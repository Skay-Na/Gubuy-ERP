package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// InboundRequest 入库请求体
type InboundRequest struct {
	ProductID      uint    `json:"product_id" binding:"required"`
	Quantity       int     `json:"quantity" binding:"required,gt=0"`
	UnitCost       float64 `json:"unit_cost" binding:"gte=0"`
	AccountID      *uint   `json:"account_id"`
	WarehouseType  int     `json:"warehouse_type" binding:"required,oneof=1 2 3"`
	SettlementType string  `json:"settlement_type"`
}

// CreateInbound 处理商品入库，同步更新账户余额（支出）
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
	settlement, err := normalizeInboundSettlement(req.SettlementType, req.AccountID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	// 1. 开启事务
	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 2. 现款采购验证账户和余额；卖后结算仅增加库存，不产生现金支出。
	var account models.Account
	totalCost := float64(req.Quantity) * req.UnitCost
	if settlement.Type == SettlementTypeCashPurchase {
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&account, *settlement.AccountID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "支付账户不存在"})
			return
		}
		if account.Balance < totalCost {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": fmt.Sprintf("账户余额不足 (当前: ¥%.2f, 需支付: ¥%.2f)", account.Balance, totalCost)})
			return
		}
	}

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

	// 4. 更新对应的商品记录 (库存累加 + 最新进价覆盖)
	var product models.Product
	if err := tx.First(&product, req.ProductID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  fmt.Sprintf("商品 ID %d 不存在", req.ProductID),
		})
		return
	}

	// 计算移动加权平均成本
	currentTotalStock := product.MainStock + product.StoreStock + product.CloudStock

	// 处理初始数据：如果 AvgCost 是 0，则假定现有库存的成本为 LatestCost
	oldAvgCost := product.AvgCost
	if oldAvgCost == 0 && product.LatestCost > 0 {
		oldAvgCost = product.LatestCost
	}

	newAvgCost := req.UnitCost
	if currentTotalStock > 0 {
		newAvgCost = (float64(currentTotalStock)*oldAvgCost + float64(req.Quantity)*req.UnitCost) / float64(currentTotalStock+req.Quantity)
	}

	updates := map[string]interface{}{}
	if req.WarehouseType == 1 {
		updates["main_stock"] = product.MainStock + req.Quantity
	} else if req.WarehouseType == 2 {
		updates["store_stock"] = product.StoreStock + req.Quantity
	} else if req.WarehouseType == 3 {
		updates["cloud_stock"] = product.CloudStock + req.Quantity
	}
	updates["latest_cost"] = req.UnitCost
	updates["avg_cost"] = newAvgCost

	if err := tx.Model(&product).Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "商品库存或价格更新失败",
		})
		return
	}

	if settlement.Type == SettlementTypeCashPurchase {
		// 5. 更新账户余额（采购支出，减少余额）
		if err := tx.Model(&account).UpdateColumn("balance", gorm.Expr("balance - ?", totalCost)).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "账户余额扣除失败"})
			return
		}

		// 5.5 同步记录财务明细，修正仪表盘总支出不平的问题
		var updatedAccount models.Account
		tx.First(&updatedAccount, *settlement.AccountID)
		if err := WriteFinancialLog(tx, *settlement.AccountID, 2, "采购货款", totalCost, updatedAccount.Balance, "", fmt.Sprintf("采购入库 商品 ID: %d, 入库数量: %d件, 成本价: ¥%.2f", req.ProductID, req.Quantity, req.UnitCost)); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "财务流水写入失败"})
			return
		}
	}

	// 5.6 记录库存流水
	beforeQty := 0
	if req.WarehouseType == 1 {
		beforeQty = product.MainStock
	} else if req.WarehouseType == 2 {
		beforeQty = product.StoreStock
	} else if req.WarehouseType == 3 {
		beforeQty = product.CloudStock
	}

	if err := WriteInventoryLog(tx, req.ProductID, req.WarehouseType, "purchase", beforeQty, req.Quantity, req.UnitCost, fmt.Sprintf("%d", inboundRecord.ID), fmt.Sprintf("采购入库：数量 %d", req.Quantity)); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "库存流水写入失败",
		})
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
		"msg":  "采购入库成功，账户余额已更新",
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

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. 查询目标库存流水记录
	var log models.InventoryLog
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&log, logID).Error; err != nil {
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

	// 查询商品主体信息
	var product models.Product
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&product, inboundRecord.ProductID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "关联商品不存在",
		})
		return
	}

	// 4. 校验对应仓库的当前可用库存是否充足
	var currentStock int
	if log.WarehouseType == 1 {
		currentStock = product.MainStock
	} else if log.WarehouseType == 2 {
		currentStock = product.StoreStock
	} else if log.WarehouseType == 3 {
		currentStock = product.CloudStock
	}

	if currentStock < inboundRecord.Quantity {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  fmt.Sprintf("可用库存不足，无法撤销此入库记录 (当前 %s 库存: %d件, 需扣减: %d件)", getWarehouseNameString(log.WarehouseType), currentStock, inboundRecord.Quantity),
		})
		return
	}

	// 5. 原路扣减库存
	updates := map[string]interface{}{}
	if log.WarehouseType == 1 {
		updates["main_stock"] = product.MainStock - inboundRecord.Quantity
	} else if log.WarehouseType == 2 {
		updates["store_stock"] = product.StoreStock - inboundRecord.Quantity
	} else if log.WarehouseType == 3 {
		updates["cloud_stock"] = product.CloudStock - inboundRecord.Quantity
	}

	// 6. 原路退回采购款项到原账户
	if inboundRecord.AccountID != nil {
		var account models.Account
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&account, *inboundRecord.AccountID).Error; err == nil {
			// 退款
			if err := tx.Model(&account).UpdateColumn("balance", gorm.Expr("balance + ?", inboundRecord.TotalCost)).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "账户退款余额增加失败"})
				return
			}

			// 记录冲销财务日志
			var updatedAccount models.Account
			tx.First(&updatedAccount, *inboundRecord.AccountID)
			if err := WriteFinancialLog(tx, *inboundRecord.AccountID, 1, "采购退货红冲", inboundRecord.TotalCost, updatedAccount.Balance, "", fmt.Sprintf("撤销采购入库：商品 ID: %d, 撤销数量: %d件, 原路退还货款: ¥%.2f", inboundRecord.ProductID, inboundRecord.Quantity, inboundRecord.TotalCost)); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "采购红冲财务流水日志写入失败"})
				return
			}
		}
	}

	// 7. 写入退库库存审计流水
	if err := WriteInventoryLog(tx, inboundRecord.ProductID, log.WarehouseType, "cancel", currentStock, -inboundRecord.Quantity, inboundRecord.UnitCost, log.RelatedNo, fmt.Sprintf("入库撤销(红冲)：原进价 ¥%.2f", inboundRecord.UnitCost)); err != nil {
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

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "入库流水红冲成功，货款已退回原付款账户，成本已重新校准！",
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

// RecalculateSingleProductCosts 在事务中为指定商品重新核算移动加权平均成本及最新进价
func RecalculateSingleProductCosts(tx *gorm.DB, productID uint) error {
	var p models.Product
	if err := tx.First(&p, productID).Error; err != nil {
		return err
	}

	var logs []models.InventoryLog
	err := tx.Where("product_id = ? AND log_type IN ? AND change_qty > 0 AND unit_cost > 0", p.ID, []string{"purchase", "stocktake"}).Find(&logs).Error
	if err != nil {
		return err
	}

	// 如果所有入库日志均被红冲撤销，归零成本参数
	if len(logs) == 0 {
		return tx.Model(&p).Updates(map[string]interface{}{
			"avg_cost":    0.0,
			"latest_cost": 0.0,
		}).Error
	}

	var totalQty int
	var totalAmount float64
	for _, log := range logs {
		totalQty += log.ChangeQty
		totalAmount += float64(log.ChangeQty) * log.UnitCost
	}

	var latestLog models.InventoryLog
	latestCost := 0.0
	tx.Where("product_id = ? AND log_type IN ? AND change_qty > 0 AND unit_cost > 0", p.ID, []string{"purchase", "stocktake"}).Order("created_at desc").First(&latestLog)
	if latestLog.ID > 0 {
		latestCost = latestLog.UnitCost
	}

	avgCost := 0.0
	if totalQty > 0 {
		avgCost = totalAmount / float64(totalQty)
	}

	return tx.Model(&p).Updates(map[string]interface{}{
		"avg_cost":    avgCost,
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
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var log models.InventoryLog
	if err := tx.First(&log, logID).Error; err != nil {
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
			if err := tx.First(&inbound, inboundRecordID).Error; err == nil {
				inbound.UnitCost = req.NewPrice
				inbound.TotalCost = float64(inbound.Quantity) * req.NewPrice
				tx.Save(&inbound)
			}
		}
	}

	// 3. 重新核算商品的移动加权平均成本和最新价格
	if err := RecalculateSingleProductCosts(tx, log.ProductID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "成本重算失败"})
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
