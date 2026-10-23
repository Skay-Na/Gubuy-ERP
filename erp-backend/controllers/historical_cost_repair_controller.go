package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const historicalCostOutlierMultiplier = 5

type HistoricalCostRepairRequest struct {
	LogID            uint    `json:"log_id"`
	OrderItemID      uint    `json:"order_item_id"`
	ProductID        uint    `json:"product_id"`
	CandidateType    string  `json:"candidate_type"`
	UnitCost         float64 `json:"unit_cost" binding:"required,gt=0"`
	RepairOrderCosts bool    `json:"repair_order_costs"`
	OutlierConfirm   string  `json:"outlier_confirm"`
	Remark           string  `json:"remark"`
	Confirm          string  `json:"confirm" binding:"required,eq=REPAIR"`
}

type HistoricalCostRepairCandidate struct {
	models.InventoryLog
	CandidateID        string  `json:"candidate_id"`
	CandidateType      string  `json:"candidate_type"`
	OrderItemID        uint    `json:"order_item_id,omitempty"`
	OrderNo            string  `json:"order_no,omitempty"`
	OrderUnitPrice     float64 `json:"order_unit_price,omitempty"`
	OrderQuantity      int     `json:"order_quantity,omitempty"`
	StockQty           int     `json:"stock_qty,omitempty"`
	ZeroCostOrderCount int64   `json:"zero_cost_order_count"`
	CostReference      float64 `json:"cost_reference"`
}

func historicalCostReference(db *gorm.DB, product models.Product, productID uint) (float64, error) {
	reference := math.Max(product.AvgCost, product.LatestCost)
	var maxOrderPrice float64
	err := db.Model(&models.OrderItem{}).
		Select("COALESCE(MAX(unit_price), 0)").
		Where("product_id = ? AND unit_cost <= 0", productID).
		Scan(&maxOrderPrice).Error
	if err != nil {
		return 0, err
	}

	return math.Max(reference, maxOrderPrice), nil
}

func isHistoricalCostOutlier(unitCost, costReference float64) bool {
	return costReference > 0 && unitCost > costReference*historicalCostOutlierMultiplier
}

func isHistoricalZeroCostInbound(log models.InventoryLog) bool {
	return log.UnitCost <= 0 && log.ChangeQty > 0 && (log.LogType == "purchase" || log.LogType == "stocktake")
}

func countHistoricalZeroCostOrderItems(db *gorm.DB, productID uint) (int64, error) {
	var orderCount int64
	err := db.Model(&models.OrderItem{}).
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("order_items.product_id = ? AND order_items.unit_cost <= 0 AND orders.order_status = ? AND orders.deleted_at IS NULL", productID, 1).
		Count(&orderCount).Error
	return orderCount, err
}

// ListHistoricalCostRepairCandidates returns zero-cost inbound logs, order lines,
// and product records that need an explicit cost baseline.
// @Summary 列出历史成本修复候选
// @Tags System
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_historical_cost_repair_candidates
// @Router /api/system/historical-cost-repair/candidates [get]
func ListHistoricalCostRepairCandidates(c *gin.Context) {
	var logs []models.InventoryLog
	if err := config.DB.Preload("Product").
		Where("unit_cost <= 0 AND change_qty > 0 AND log_type IN ?", []string{"purchase", "stocktake"}).
		Order("created_at asc, id asc").Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取历史零成本记录失败"})
		return
	}

	candidates := make([]HistoricalCostRepairCandidate, 0, len(logs))
	productsWithRepairableLogs := make(map[uint]struct{}, len(logs))
	for _, log := range logs {
		productsWithRepairableLogs[log.ProductID] = struct{}{}
		orderCount, err := countHistoricalZeroCostOrderItems(config.DB, log.ProductID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取受影响订单数量失败"})
			return
		}
		if orderCount == 0 {
			continue
		}

		product := models.Product{}
		if log.Product != nil {
			product = *log.Product
		}
		costReference, err := historicalCostReference(config.DB, product, log.ProductID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取成本核验参考失败"})
			return
		}

		candidates = append(candidates, HistoricalCostRepairCandidate{
			InventoryLog:       log,
			CandidateID:        fmt.Sprintf("log:%d", log.ID),
			CandidateType:      "inventory_log",
			StockQty:           productTotalStock(product),
			ZeroCostOrderCount: orderCount,
			CostReference:      costReference,
		})
	}

	var orderItems []models.OrderItem
	if err := config.DB.Preload("Product").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("order_items.unit_cost <= 0 AND orders.order_status = ? AND orders.deleted_at IS NULL", 1).
		Order("order_items.order_id asc, order_items.id asc").Find(&orderItems).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取零成本订单明细失败"})
		return
	}
	orderIDs := make([]uint, 0, len(orderItems))
	seenOrderIDs := make(map[uint]struct{}, len(orderItems))
	for _, item := range orderItems {
		if _, exists := seenOrderIDs[item.OrderID]; exists {
			continue
		}
		seenOrderIDs[item.OrderID] = struct{}{}
		orderIDs = append(orderIDs, item.OrderID)
	}
	ordersByID := make(map[uint]models.Order, len(orderIDs))
	if len(orderIDs) > 0 {
		var orders []models.Order
		if err := config.DB.Select("id", "order_no", "created_at").Where("id IN ? AND order_status = ?", orderIDs, 1).Find(&orders).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取零成本订单信息失败"})
			return
		}
		for _, order := range orders {
			ordersByID[order.ID] = order
		}
	}
	for _, item := range orderItems {
		product := models.Product{}
		if item.Product != nil {
			product = *item.Product
		}
		costReference, err := historicalCostReference(config.DB, product, item.ProductID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取订单成本核验参考失败"})
			return
		}
		order := ordersByID[item.OrderID]
		createdAt := order.CreatedAt
		if createdAt.IsZero() {
			createdAt = item.CreatedAt
		}
		candidates = append(candidates, HistoricalCostRepairCandidate{
			InventoryLog: models.InventoryLog{
				ProductID: item.ProductID,
				Product:   item.Product,
				LogType:   "sale",
				ChangeQty: item.Quantity,
				RelatedNo: order.OrderNo,
				CreatedAt: createdAt,
			},
			CandidateID:        fmt.Sprintf("order_item:%d", item.ID),
			CandidateType:      "order_item",
			OrderItemID:        item.ID,
			OrderNo:            order.OrderNo,
			OrderUnitPrice:     item.UnitPrice,
			OrderQuantity:      item.Quantity,
			ZeroCostOrderCount: 1,
			CostReference:      costReference,
		})
	}

	var productsWithInboundHistory []uint
	if err := config.DB.Model(&models.InventoryLog{}).
		Distinct("product_id").
		Where("change_qty > 0 AND log_type IN ?", []string{"purchase", "stocktake"}).
		Pluck("product_id", &productsWithInboundHistory).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取商品入库历史失败"})
		return
	}
	inboundProductIDs := make(map[uint]struct{}, len(productsWithInboundHistory))
	for _, productID := range productsWithInboundHistory {
		inboundProductIDs[productID] = struct{}{}
	}

	var products []models.Product
	if err := config.DB.
		Where("avg_cost <= 0 AND latest_cost <= 0").
		Order("created_at asc, id asc").Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取零成本商品失败"})
		return
	}
	for i := range products {
		product := &products[i]
		if _, hasRepairableLog := productsWithRepairableLogs[product.ID]; hasRepairableLog {
			continue
		}
		if _, hasInboundHistory := inboundProductIDs[product.ID]; hasInboundHistory {
			continue
		}
		orderCount, err := countHistoricalZeroCostOrderItems(config.DB, product.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取受影响订单数量失败"})
			return
		}
		if orderCount == 0 {
			continue
		}
		costReference, err := historicalCostReference(config.DB, *product, product.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取成本核验参考失败"})
			return
		}
		candidates = append(candidates, HistoricalCostRepairCandidate{
			InventoryLog: models.InventoryLog{
				ProductID: product.ID,
				Product:   product,
				CreatedAt: product.CreatedAt,
			},
			CandidateID:        fmt.Sprintf("product:%d", product.ID),
			CandidateType:      "product",
			StockQty:           productTotalStock(*product),
			ZeroCostOrderCount: orderCount,
			CostReference:      costReference,
		})
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": candidates})
}

// RepairHistoricalCost repairs exactly one selected product, inventory log, or order line.
// @Summary 修复历史成本
// @Tags System
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID repair_historical_cost
// @Router /api/system/historical-cost-repair [post]
func RepairHistoricalCost(c *gin.Context) {
	var req HistoricalCostRepairRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请求参数错误：请填写大于 0 的成本并输入 REPAIR 确认"})
		return
	}
	if req.RepairOrderCosts {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "订单成本必须逐笔核实，请从待修复订单明细中单独修复"})
		return
	}
	validProduct := req.CandidateType == "product" && req.ProductID != 0 && req.LogID == 0 && req.OrderItemID == 0
	validLog := req.CandidateType == "inventory_log" && req.LogID != 0 && req.ProductID == 0 && req.OrderItemID == 0
	validOrderItem := req.CandidateType == "order_item" && req.OrderItemID != 0 && req.ProductID == 0 && req.LogID == 0
	if !validProduct && !validLog && !validOrderItem {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "修复目标无效，请刷新待修复列表后重试"})
		return
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "启动成本修复事务失败"})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var product models.Product
	var audit models.HistoricalCostRepair
	var repairedOrders int64
	var repairedSales int64
	productID := req.ProductID

	switch {
	case validProduct:
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, req.ProductID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "找不到商品"})
			return
		}
		if product.AvgCost > 0 || product.LatestCost > 0 {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "该商品成本已更新，请刷新待修复列表"})
			return
		}
		var inboundCount int64
		if err := tx.Model(&models.InventoryLog{}).
			Where("product_id = ? AND change_qty > 0 AND log_type IN ?", product.ID, []string{"purchase", "stocktake"}).
			Count(&inboundCount).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取商品入库历史失败"})
			return
		}
		if inboundCount > 0 {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "该商品已有入库流水，请刷新列表并修复具体流水"})
			return
		}
		costReference, err := historicalCostReference(tx, product, product.ID)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取成本核验参考失败"})
			return
		}
		if rejectHistoricalCostOutlier(c, tx, req, costReference) {
			return
		}
		if err := tx.Model(&product).Update("avg_cost", req.UnitCost).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新商品成本失败"})
			return
		}
		audit = models.HistoricalCostRepair{
			ProductID:        product.ID,
			UnitCost:         req.UnitCost,
			PreviousUnitCost: product.AvgCost,
			StockQty:         productTotalStock(product),
			CostBaseline:     true,
			Remark:           repairRemark(req.Remark, "管理员商品档案零成本补录"),
		}
	case validLog:
		var log models.InventoryLog
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&log, req.LogID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "找不到库存流水"})
			return
		}
		if !isHistoricalZeroCostInbound(log) {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "该流水已变化或不是可修复的历史零成本入库记录"})
			return
		}
		productID = log.ProductID
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, productID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "找不到关联商品"})
			return
		}
		costReference, err := historicalCostReference(tx, product, productID)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取成本核验参考失败"})
			return
		}
		if rejectHistoricalCostOutlier(c, tx, req, costReference) {
			return
		}
		previousCost := log.UnitCost
		log.UnitCost = req.UnitCost
		log.Remark = fmt.Sprintf("%s（历史成本补录：¥%.2f）", log.Remark, req.UnitCost)
		if err := tx.Save(&log).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新库存成本失败"})
			return
		}
		if log.LogType == "purchase" && log.RelatedNo != "" {
			var inboundID uint
			if _, err := fmt.Sscanf(log.RelatedNo, "%d", &inboundID); err == nil && inboundID > 0 {
				var inbound models.InboundRecord
				inboundErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&inbound, inboundID).Error
				if inboundErr == nil {
					inbound.UnitCost = req.UnitCost
					inbound.TotalCost = float64(inbound.Quantity) * req.UnitCost
					if err := tx.Save(&inbound).Error; err != nil {
						tx.Rollback()
						c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "同步修复入库单成本失败"})
						return
					}
				} else if inboundErr != gorm.ErrRecordNotFound {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取关联入库单失败"})
					return
				}
			}
		}
		if err := RecalculateSingleProductCosts(tx, productID); err != nil {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "成本流水已回滚，当前历史不能安全重算：" + err.Error()})
			return
		}
		audit = models.HistoricalCostRepair{
			InventoryLogID:     &log.ID,
			ProductID:          productID,
			UnitCost:           req.UnitCost,
			PreviousUnitCost:   previousCost,
			RepairedOrderCount: 0,
			RepairedSaleCount:  0,
			Remark:             repairRemark(req.Remark, "管理员历史入库成本补录"),
		}
	case validOrderItem:
		var item models.OrderItem
		if err := tx.First(&item, req.OrderItemID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "找不到订单明细"})
			return
		}
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, item.OrderID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "找不到关联订单"})
			return
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, item.ID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "订单明细已变化，请刷新待修复列表"})
			return
		}
		if item.UnitCost > 0 {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "该订单成本已补录，请刷新待修复列表"})
			return
		}
		var productForReference models.Product
		if err := tx.First(&productForReference, item.ProductID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "找不到订单商品"})
			return
		}
		costReference, err := historicalCostReference(tx, productForReference, item.ProductID)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取订单成本核验参考失败"})
			return
		}
		if rejectHistoricalCostOutlier(c, tx, req, costReference) {
			return
		}
		result := tx.Model(&models.OrderItem{}).Where("id = ? AND unit_cost <= 0", item.ID).Update("unit_cost", req.UnitCost)
		if result.Error != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "修复订单成本失败"})
			return
		}
		if result.RowsAffected != 1 {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "订单成本已被其他操作更新，请刷新待修复列表"})
			return
		}
		logResult := tx.Model(&models.InventoryLog{}).
			Where("product_id = ? AND log_type = ? AND related_no = ? AND unit_cost <= 0", item.ProductID, "sale", order.OrderNo).
			Update("unit_cost", req.UnitCost)
		if logResult.Error != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "同步该订单销售流水成本失败"})
			return
		}
		productID = item.ProductID
		repairedOrders = 1
		repairedSales = logResult.RowsAffected
		orderItemID := item.ID
		audit = models.HistoricalCostRepair{
			OrderItemID:        &orderItemID,
			ProductID:          productID,
			OrderNo:            order.OrderNo,
			UnitCost:           req.UnitCost,
			PreviousUnitCost:   item.UnitCost,
			RepairedOrderCount: repairedOrders,
			RepairedSaleCount:  repairedSales,
			Remark:             repairRemark(req.Remark, "管理员逐笔修复订单成本"),
		}
	}

	if err := tx.Create(&audit).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存修复审计记录失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "提交历史成本修复失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "成本修复完成；库存数量和账户余额未变动", "data": gin.H{
		"repaired_order_count": repairedOrders,
		"repaired_sale_count":  repairedSales,
	}})
}

func repairRemark(remark, fallback string) string {
	if remark == "" {
		return fallback
	}
	return remark
}

func rejectHistoricalCostOutlier(c *gin.Context, tx *gorm.DB, req HistoricalCostRepairRequest, costReference float64) bool {
	if isHistoricalCostOutlier(req.UnitCost, costReference) && req.OutlierConfirm != fmt.Sprintf("%.2f", req.UnitCost) {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": fmt.Sprintf("补录单价超过核验参考的 %d 倍，请重新核对凭证并输入确认金额", historicalCostOutlierMultiplier)})
		return true
	}
	return false
}
