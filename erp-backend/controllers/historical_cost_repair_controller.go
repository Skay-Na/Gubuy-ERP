package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

type HistoricalCostRepairRequest struct {
	LogID            uint    `json:"log_id" binding:"required"`
	UnitCost         float64 `json:"unit_cost" binding:"required,gt=0"`
	RepairOrderCosts bool    `json:"repair_order_costs"`
	Remark           string  `json:"remark"`
	Confirm          string  `json:"confirm" binding:"required,eq=REPAIR"`
}

type HistoricalCostRepairCandidate struct {
	models.InventoryLog
	ZeroCostOrderCount int64 `json:"zero_cost_order_count"`
}

func isHistoricalZeroCostInbound(log models.InventoryLog) bool {
	return log.UnitCost == 0 && log.ChangeQty > 0 && (log.LogType == "purchase" || log.LogType == "stocktake")
}

// ListHistoricalCostRepairCandidates returns only historic positive stock records that are missing a cost.
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
		Where("unit_cost = 0 AND change_qty > 0 AND log_type IN ?", []string{"purchase", "stocktake"}).
		Order("created_at asc").
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取历史零成本记录失败"})
		return
	}

	candidates := make([]HistoricalCostRepairCandidate, 0, len(logs))
	for _, log := range logs {
		var orderCount int64
		config.DB.Model(&models.OrderItem{}).Where("product_id = ? AND unit_cost = 0", log.ProductID).Count(&orderCount)
		candidates = append(candidates, HistoricalCostRepairCandidate{InventoryLog: log, ZeroCostOrderCount: orderCount})
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": candidates})
}

// RepairHistoricalCost repairs one selected historic inbound record without changing stock quantities or account balances.
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

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var log models.InventoryLog
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&log, req.LogID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "找不到库存流水"})
		return
	}
	if !isHistoricalZeroCostInbound(log) {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "该流水不是可修复的历史零成本增加库存记录"})
		return
	}

	remark := req.Remark
	if remark == "" {
		remark = "管理员历史成本补录"
	}
	log.UnitCost = req.UnitCost
	log.Remark = fmt.Sprintf("%s（历史零成本补录：¥%.2f）", log.Remark, req.UnitCost)
	if err := tx.Save(&log).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新库存成本失败"})
		return
	}

	var repairedOrders int64
	var repairedSales int64
	if req.RepairOrderCosts {
		result := tx.Model(&models.OrderItem{}).Where("product_id = ? AND unit_cost = 0", log.ProductID).Update("unit_cost", req.UnitCost)
		if result.Error != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "修复订单成本失败"})
			return
		}
		repairedOrders = result.RowsAffected

		result = tx.Model(&models.InventoryLog{}).Where("product_id = ? AND log_type = ? AND unit_cost = 0", log.ProductID, "sale").Update("unit_cost", req.UnitCost)
		if result.Error != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "修复销售流水成本失败"})
			return
		}
		repairedSales = result.RowsAffected
	}

	if err := RecalculateSingleProductCosts(tx, log.ProductID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "重算商品成本失败"})
		return
	}

	audit := models.HistoricalCostRepair{
		InventoryLogID:     log.ID,
		ProductID:          log.ProductID,
		UnitCost:           req.UnitCost,
		RepairedOrderCount: repairedOrders,
		RepairedSaleCount:  repairedSales,
		Remark:             remark,
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

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "历史成本修复完成；库存数量和账户余额未变动", "data": gin.H{
		"repaired_order_count": repairedOrders,
		"repaired_sale_count":  repairedSales,
	}})
}
