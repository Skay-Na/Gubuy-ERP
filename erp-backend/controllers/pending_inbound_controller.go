package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

type CreatePendingInboundRequest struct {
	ProductID     uint   `json:"product_id" binding:"required"`
	WarehouseType int    `json:"warehouse_type" binding:"required,oneof=1 2 3"`
	Quantity      int    `json:"quantity" binding:"required,gt=0"`
	SubmitterName string `json:"submitter_name"`
	Remark        string `json:"remark"`
}

// CreatePendingInbound records a staff inventory application without affecting saleable stock or cost.
// @Summary 创建待确认入库
// @Tags System
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID create_pending_inbound
// @Router /api/pending-inbounds [post]
func CreatePendingInbound(c *gin.Context) {
	var req CreatePendingInboundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误", "error": err.Error()})
		return
	}

	submitterName := req.SubmitterName
	if submitterName == "" {
		submitterName = c.GetString("username")
	}
	if submitterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "缺少申请人信息"})
		return
	}

	record := models.PendingInbound{
		ProductID:     req.ProductID,
		WarehouseType: req.WarehouseType,
		Quantity:      req.Quantity,
		SubmitterName: submitterName,
		Remark:        req.Remark,
		Status:        "pending",
	}
	if err := config.DB.Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建待确认入库失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "提交入库申请成功", "data": record})
}

type ApprovePendingInboundRequest struct {
	UnitCost  float64 `json:"unit_cost" binding:"required,gt=0"`
	AccountID uint    `json:"account_id" binding:"required"`
}

// ApprovePendingInbound converts an approved application into a normal purchase inbound atomically.
// @Summary 确认待入库单
// @Tags System
// @Accept json
// @Produce json
// @Param id path int true "编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID approve_pending_inbound
// @Router /api/pending-inbounds/{id}/approve [post]
func ApprovePendingInbound(c *gin.Context) {
	var req ApprovePendingInboundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误(需要大于 0 的入库单价和支出账户)", "error": err.Error()})
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

	var pending models.PendingInbound
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&pending, c.Param("id")).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "找不到待确认入库申请"})
		return
	}
	if pending.Status != "pending" {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "只能审核待确认状态的申请"})
		return
	}

	var product models.Product
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, pending.ProductID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "商品不存在"})
		return
	}
	var account models.Account
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, req.AccountID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "支出账户不存在"})
		return
	}

	totalCost := req.UnitCost * float64(pending.Quantity)
	if account.Balance < totalCost {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "账户余额不足"})
		return
	}

	beforeStock := 0
	currentTotalStock := product.MainStock + product.StoreStock + product.CloudStock
	switch pending.WarehouseType {
	case 1:
		beforeStock = product.MainStock
		product.MainStock += pending.Quantity
	case 2:
		beforeStock = product.StoreStock
		product.StoreStock += pending.Quantity
	case 3:
		beforeStock = product.CloudStock
		product.CloudStock += pending.Quantity
	}

	oldAvgCost := product.AvgCost
	if oldAvgCost == 0 && product.LatestCost > 0 {
		oldAvgCost = product.LatestCost
	}
	newAvgCost := req.UnitCost
	if currentTotalStock > 0 {
		newAvgCost = (float64(currentTotalStock)*oldAvgCost + totalCost) / float64(currentTotalStock+pending.Quantity)
	}
	product.AvgCost = newAvgCost
	product.LatestCost = req.UnitCost
	if err := tx.Save(&product).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新商品库存失败"})
		return
	}

	accountID := req.AccountID
	inboundRecord := models.InboundRecord{
		ProductID: pending.ProductID,
		Quantity:  pending.Quantity,
		UnitCost:  req.UnitCost,
		TotalCost: totalCost,
		AccountID: &accountID,
	}
	if err := tx.Create(&inboundRecord).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建入库记录失败"})
		return
	}
	if err := tx.Model(&account).UpdateColumn("balance", account.Balance-totalCost).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新账户余额失败"})
		return
	}
	account.Balance -= totalCost
	if err := WriteFinancialLog(tx, accountID, 2, "采购货款", totalCost, account.Balance, "", fmt.Sprintf("员工申请入库：申请人 %s，商品 ID: %d，数量: %d，成本价: ¥%.2f", pending.SubmitterName, product.ID, pending.Quantity, req.UnitCost)); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记录财务流水失败"})
		return
	}
	if err := WriteInventoryLog(tx, product.ID, pending.WarehouseType, "purchase", beforeStock, pending.Quantity, req.UnitCost, fmt.Sprintf("%d", inboundRecord.ID), fmt.Sprintf("员工申请入库通过：%s", pending.Remark)); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记录库存日志失败"})
		return
	}

	pending.Status = "approved"
	if err := tx.Save(&pending).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新申请状态失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "提交事务失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "入库申请已通过审核"})
}

// @Summary 列出待确认入库
// @Tags System
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_pending_inbounds
// @Router /api/pending-inbounds [get]
func ListPendingInbounds(c *gin.Context) {
	status := c.Query("status")
	var records []models.PendingInbound
	query := config.DB.Preload("Product").Order("created_at desc")
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取待确认入库列表失败", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": records})
}

// @Summary 驳回待入库单
// @Tags System
// @Accept json
// @Produce json
// @Param id path int true "编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID reject_pending_inbound
// @Router /api/pending-inbounds/{id}/reject [post]
func RejectPendingInbound(c *gin.Context) {
	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "启动事务失败"})
		return
	}
	var pending models.PendingInbound
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&pending, c.Param("id")).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "找不到待确认入库申请"})
		return
	}
	if pending.Status != "pending" {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "只能拒绝待确认状态的申请"})
		return
	}
	if err := tx.Model(&pending).Update("status", "rejected").Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新申请状态失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "提交事务失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "入库申请已拒绝"})
}
