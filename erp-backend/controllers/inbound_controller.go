package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// InboundRequest 入库请求体
type InboundRequest struct {
	ProductID      uint    `json:"product_id" binding:"required"`
	Quantity       int     `json:"quantity" binding:"required,gt=0"`
	UnitCost       float64 `json:"unit_cost" binding:"required,gt=0"`
	AccountID      *uint   `json:"account_id"`
	WarehouseType  int     `json:"warehouse_type" binding:"required,oneof=1 2 3"`
	SettlementType string  `json:"settlement_type"`
}

// CreateInbound records inventory quantity and cost basis without moving money.
// @Summary 创建入库单
// @Description 创建入库单
// @Tags Inbounds
// @Accept json
// @Produce json
// @Param body body InboundRequest true "入库参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_inbounds
// @Router /api/inbounds [post]
func CreateInbound(c *gin.Context) {
	var req InboundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "请求参数格式错误: " + err.Error(),
		})
		return
	}
	operator := InventoryOperatorFromContext(c)
	if req.WarehouseType == 1 {
		req.WarehouseType = 3
	}
	settlement, err := normalizeInboundSettlement(req.SettlementType, req.AccountID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	// 1. 开启事务
	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "启动事务失败"})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Product rows are always locked before account rows across inventory flows.
	var product models.Product
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, req.ProductID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": fmt.Sprintf("商品 ID %d 不存在", req.ProductID)})
		return
	}
	if err := ensureProductCostBaseline(tx, product); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "建立商品成本基准失败"})
		return
	}

	// Procurement is an inventory/cost record, not a cash payment.
	totalCost := float64(req.Quantity) * req.UnitCost

	// 3. 计算并创建入库明细记录
	inboundRecord := models.InboundRecord{
		ProductID:      req.ProductID,
		Quantity:       req.Quantity,
		UnitCost:       req.UnitCost,
		TotalCost:      totalCost,
		SettlementType: settlement.Type,
		AccountID:      settlement.AccountID,
	}

	if err := tx.Create(&inboundRecord).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "入库记录保存失败",
		})
		return
	}

	// 4. 更新对应的商品库存；成本统一从有效成本流水重算。
	updates := map[string]interface{}{}
	if req.WarehouseType == 2 {
		updates["store_stock"] = product.StoreStock + req.Quantity
	} else if req.WarehouseType == 3 {
		updates["cloud_stock"] = product.CloudStock + req.Quantity
	}
	if err := tx.Model(&product).Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "商品库存或价格更新失败",
		})
		return
	}

	// Record the inventory cost event; sale orders snapshot the resulting cost for profit.
	beforeQty := 0
	if req.WarehouseType == 2 {
		beforeQty = product.StoreStock
	} else if req.WarehouseType == 3 {
		beforeQty = product.CloudStock
	}

	if err := WriteInventoryLog(tx, req.ProductID, req.WarehouseType, "purchase", beforeQty, req.Quantity, req.UnitCost, fmt.Sprintf("%d", inboundRecord.ID), fmt.Sprintf("采购入库：数量 %d", req.Quantity), operator); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "库存流水写入失败",
		})
		return
	}
	if err := RecalculateSingleProductCosts(tx, req.ProductID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "成本重算失败：" + err.Error()})
		return
	}

	// 6. 提交事务
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "事务提交失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "商品入库成功，已记录库存成本，本次未扣资金账户",
		"data": inboundRecord,
	})
}

// CancelInboundByLog 撤销某笔采购入库流水（一键红冲）
// POST /api/inbounds/cancel-by-log/:log_id
// @Summary 撤销入库记录
// @Description 撤销入库记录
// @Tags Inbounds
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param log_id path int true "路径资源ID"
// @ID post_api_inbounds_cancel_by_log_log_id
// @Router /api/inbounds/cancel-by-log/{log_id} [post]
func CancelInboundByLog(c *gin.Context) {
	logID := c.Param("log_id")
	operator := InventoryOperatorFromContext(c)

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. 查询目标库存流水记录
	var log models.InventoryLog
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&log, logID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "对应的库存流水不存在",
		})
		return
	}

	// 2. 检查流水是否为采购入库类型且关联了入库单
	if log.LogType != "purchase" || log.RelatedNo == "" {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "此流水类型不是采购入库，或缺少关联的入库记录，无法撤销",
		})
		return
	}

	// 解析 RelatedNo 获得 InboundRecord 的 ID
	var inboundRecordID uint
	if _, err := fmt.Sscanf(log.RelatedNo, "%d", &inboundRecordID); err != nil || inboundRecordID == 0 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "流水关联的入库单号格式错误，无法撤销",
		})
		return
	}

	// 3. 查询入库单记录
	var inboundRecord models.InboundRecord
	if err := tx.First(&inboundRecord, inboundRecordID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "关联的采购入库单记录已被删除或不存在",
		})
		return
	}
	warehouseType := log.WarehouseType
	if warehouseType == 1 {
		warehouseType = 3
	}

	// 查询商品主体信息
	var product models.Product
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, inboundRecord.ProductID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "关联商品不存在",
		})
		return
	}
	if err := ensureProductCostBaseline(tx, product); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "建立商品成本基准失败"})
		return
	}
	refundedPayment := false

	// 4. 校验对应仓库的当前可用库存是否充足
	var currentStock int
	if warehouseType == 2 {
		currentStock = product.StoreStock
	} else if warehouseType == 3 {
		currentStock = product.CloudStock
	}

	if currentStock < inboundRecord.Quantity {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  fmt.Sprintf("可用库存不足，无法撤销此入库记录 (当前 %s 库存: %d件, 需扣减: %d件)", getWarehouseNameString(warehouseType), currentStock, inboundRecord.Quantity),
		})
		return
	}

	// 5. 原路扣减库存
	updates := map[string]interface{}{}
	if warehouseType == 2 {
		updates["store_stock"] = product.StoreStock - inboundRecord.Quantity
	} else if warehouseType == 3 {
		updates["cloud_stock"] = product.CloudStock - inboundRecord.Quantity
	}

	// Refund only legacy records that explicitly represent a real cash purchase.
	if inboundRecord.SettlementType == SettlementTypeCashPurchase && inboundRecord.AccountID != nil {
		var account models.Account
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, *inboundRecord.AccountID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "原采购付款账户不存在，无法安全撤销"})
			return
		}
		if err := tx.Model(&account).UpdateColumn("balance", gorm.Expr("balance + ?", inboundRecord.TotalCost)).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "账户退款余额增加失败"})
			return
		}

		var updatedAccount models.Account
		if err := tx.First(&updatedAccount, *inboundRecord.AccountID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取退款后账户余额失败"})
			return
		}
		if err := WriteFinancialLog(tx, *inboundRecord.AccountID, 1, "采购退货红冲", inboundRecord.TotalCost, updatedAccount.Balance, "", fmt.Sprintf("撤销采购入库：商品 ID: %d, 撤销数量: %d件, 原路退还货款: ¥%.2f", inboundRecord.ProductID, inboundRecord.Quantity, inboundRecord.TotalCost)); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "采购红冲财务流水日志写入失败"})
			return
		}
		refundedPayment = true
	}

	// 7. 写入退库库存审计流水
	if err := WriteInventoryLog(tx, inboundRecord.ProductID, warehouseType, "cancel", currentStock, -inboundRecord.Quantity, inboundRecord.UnitCost, log.RelatedNo, fmt.Sprintf("入库撤销(红冲)：原进价 ¥%.2f", inboundRecord.UnitCost), operator); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "采购红冲库存日志写入失败"})
		return
	}

	// 8. 将原 "purchase" 记录标记为 "purchase_void" 排除出后续成本加权计算
	if err := tx.Model(&log).Update("log_type", "purchase_void").Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "废除历史入库日志状态失败"})
		return
	}

	// 9. 更新商品库存数量
	if err := tx.Model(&product).Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "商品库存数据更新失败"})
		return
	}

	// 10. 删除该入库单实体
	if err := tx.Delete(&inboundRecord).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "采购入库实体销毁失败"})
		return
	}

	// 11. 自动重新核算该商品的移动加权平均成本与最新进价
	if err := RecalculateSingleProductCosts(tx, inboundRecord.ProductID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "重新核算移动加权平均成本失败: " + err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	message := "入库已撤销，库存和成本已重新核算"
	if refundedPayment {
		message = "入库已撤销，旧采购款已退回原付款账户，库存和成本已重新核算"
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  message,
	})
}

// 辅助函数：根据类型获取仓库名称
func getWarehouseNameString(wType int) string {
	switch wType {
	case 1:
		return "主仓"
	case 2:
		return "门店"
	case 3:
		return "云仓"
	default:
		return "未知仓库"
	}
}

func productTotalStock(product models.Product) int {
	return product.MainStock + product.StoreStock + product.CloudStock + product.SampleStock
}

// ensureProductCostBaseline records the current stock and known average as the
// opening balance for future event-based recalculations. This avoids replaying
// incomplete legacy logs over a cost that was already calibrated.
func ensureProductCostBaseline(tx *gorm.DB, product models.Product) error {
	var count int64
	if err := tx.Model(&models.HistoricalCostRepair{}).
		Where("product_id = ? AND cost_baseline = ?", product.ID, true).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	cost := product.AvgCost
	if cost <= 0 {
		cost = product.LatestCost
	}
	if cost <= 0 {
		return nil
	}
	return tx.Create(&models.HistoricalCostRepair{
		ProductID:        product.ID,
		UnitCost:         cost,
		PreviousUnitCost: product.AvgCost,
		StockQty:         productTotalStock(product),
		CostBaseline:     true,
		Remark:           "自动建立库存成本期初基准",
	}).Error
}

type productCostEvent struct {
	createdAt time.Time
	log       *models.InventoryLog
	baseline  *models.HistoricalCostRepair
}

func addCostToStock(quantity int, averageCost float64, changeQty int, unitCost float64) (int, float64) {
	newQuantity := quantity + changeQty
	if quantity <= 0 {
		return newQuantity, unitCost
	}
	return newQuantity, (float64(quantity)*averageCost + float64(changeQty)*unitCost) / float64(newQuantity)
}

// RecalculateSingleProductCosts replays the canonical inventory cost events
// from the latest audited baseline. Transfers do not change total stock value.
func RecalculateSingleProductCosts(tx *gorm.DB, productID uint) error {
	var p models.Product
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, productID).Error; err != nil {
		return err
	}

	var latestBaseline models.HistoricalCostRepair
	baselineErr := tx.Where("product_id = ? AND cost_baseline = ?", p.ID, true).
		Order("created_at desc, id desc").First(&latestBaseline).Error
	hasBaseline := baselineErr == nil
	if baselineErr != nil && baselineErr != gorm.ErrRecordNotFound {
		return baselineErr
	}

	var logs []models.InventoryLog
	err := tx.Where("product_id = ? AND log_type IN ?", p.ID, []string{"purchase", "purchase_void", "sale", "cancel", "stocktake"}).
		Order("created_at asc, id asc").Find(&logs).Error
	if err != nil {
		return err
	}

	var auditBaselines []models.HistoricalCostRepair
	if hasBaseline {
		auditBaselines = append(auditBaselines, latestBaseline)
	}
	events := make([]productCostEvent, 0, len(logs)+len(auditBaselines))
	for i := range auditBaselines {
		baseline := &auditBaselines[i]
		events = append(events, productCostEvent{createdAt: baseline.CreatedAt, baseline: baseline})
	}
	for i := range logs {
		log := &logs[i]
		if log.ChangeQty == 0 {
			continue
		}
		if hasBaseline && log.CreatedAt.Before(latestBaseline.CreatedAt) {
			continue
		}
		events = append(events, productCostEvent{createdAt: log.CreatedAt, log: log})
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].createdAt.Equal(events[j].createdAt) {
			// A baseline is written before the inventory event that follows it.
			return events[i].baseline != nil && events[j].baseline == nil
		}
		return events[i].createdAt.Before(events[j].createdAt)
	})

	quantity := 0
	averageCost := 0.0
	if hasBaseline {
		quantity = latestBaseline.StockQty
		averageCost = latestBaseline.UnitCost
	}
	latestCost := p.LatestCost
	var latestValidInboundCost float64
	hasInboundHistory := false
	hasActiveInbound := false
	for _, log := range logs {
		if log.LogType == "purchase_void" {
			hasInboundHistory = true
			continue
		}
		if log.ChangeQty > 0 && (log.LogType == "purchase" || log.LogType == "stocktake") {
			hasInboundHistory = true
			hasActiveInbound = true
			if log.UnitCost > 0 {
				latestValidInboundCost = log.UnitCost
			}
		}
	}
	if latestValidInboundCost > 0 {
		latestCost = latestValidInboundCost
	} else if hasInboundHistory && !hasActiveInbound {
		latestCost = 0
	}

	for _, event := range events {
		if event.baseline != nil {
			quantity = event.baseline.StockQty
			averageCost = event.baseline.UnitCost
			continue
		}
		log := event.log
		if log == nil || log.LogType == "purchase_void" || log.LogType == "transfer" || log.ChangeQty == 0 {
			continue
		}
		switch {
		case (log.LogType == "purchase" || log.LogType == "stocktake" || log.LogType == "cancel") && log.ChangeQty > 0:
			if log.UnitCost <= 0 {
				// Preserve unpriced inventory events without assigning an unknown value.
				quantity += log.ChangeQty
				continue
			}
			if quantity > 0 && averageCost <= 0 {
				return fmt.Errorf("库存流水 #%d 增加库存前已有 %d 件无成本基准库存，请先补录成本", log.ID, quantity)
			}
			quantity, averageCost = addCostToStock(quantity, averageCost, log.ChangeQty, log.UnitCost)
		case (log.LogType == "sale" || log.LogType == "stocktake" || log.LogType == "cancel") && log.ChangeQty < 0:
			if quantity < -log.ChangeQty {
				return fmt.Errorf("库存流水 #%d 扣减 %d 件时，成本历史仅有 %d 件；请先核对库存流水", log.ID, -log.ChangeQty, quantity)
			}
			quantity += log.ChangeQty
		default:
			return fmt.Errorf("库存流水 #%d 类型或数量异常：%s / %d", log.ID, log.LogType, log.ChangeQty)
		}
	}

	if len(events) == 0 {
		if !hasInboundHistory {
			return nil
		}
		averageCost = 0
	}
	currentStock := productTotalStock(p)
	if quantity != currentStock {
		return fmt.Errorf("成本流水推算库存为 %d 件，与商品当前库存 %d 件不一致；请先校准库存基准", quantity, currentStock)
	}

	return tx.Model(&p).Updates(map[string]interface{}{
		"avg_cost":    averageCost,
		"latest_cost": latestCost,
	}).Error
}

// UpdateLogPriceRequest 更改入库价格请求体
type UpdateLogPriceRequest struct {
	NewPrice float64 `json:"new_price" binding:"required,gt=0"`
}

// UpdateInventoryLogPrice 更改过去入库价格的临时补救功能
// @Summary 更新入库记录单价
// @Description 更新入库记录单价
// @Tags Inbounds
// @Accept json
// @Produce json
// @Param body body UpdateLogPriceRequest true "入库单价更新参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param log_id path int true "路径资源ID"
// @ID post_api_inbounds_update_price_log_id
// @Router /api/inbounds/update-price/{log_id} [post]
func UpdateInventoryLogPrice(c *gin.Context) {
	logIDStr := c.Param("log_id")
	var logID uint
	if _, err := fmt.Sscanf(logIDStr, "%d", &logID); err != nil || logID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "无效的流水ID"})
		return
	}

	var req UpdateLogPriceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "启动事务失败"})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var log models.InventoryLog
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&log, logID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "找不到该库存流水"})
		return
	}

	// 限制只能修改 purchase/purchase_void 流水
	if log.LogType != "purchase" && log.LogType != "purchase_void" {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "只允许修改采购入库流水的价格"})
		return
	}

	// 1. 更新流水价格
	oldPrice := log.UnitCost
	log.UnitCost = req.NewPrice
	log.Remark = fmt.Sprintf("%s (进价由 ¥%.2f 手动修正为 ¥%.2f)", log.Remark, oldPrice, req.NewPrice)
	if err := tx.Save(&log).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新库存流水失败"})
		return
	}

	// 2. 如果存在关联的原始入库记录，一并修正其单价与总成本
	if log.RelatedNo != "" {
		var inboundRecordID uint
		if _, err := fmt.Sscanf(log.RelatedNo, "%d", &inboundRecordID); err == nil && inboundRecordID > 0 {
			var inbound models.InboundRecord
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&inbound, inboundRecordID).Error; err == nil {
				inbound.UnitCost = req.NewPrice
				inbound.TotalCost = float64(inbound.Quantity) * req.NewPrice
				if err := tx.Save(&inbound).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "同步更新入库单成本失败"})
					return
				}
			} else if err != gorm.ErrRecordNotFound {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取关联入库单失败"})
				return
			}
		}
	}

	// 3. 重新核算商品的移动加权平均成本和最新价格
	if err := RecalculateSingleProductCosts(tx, log.ProductID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "成本重算失败：" + err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "提交事务失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "价格修正及成本重算成功！",
	})
}
