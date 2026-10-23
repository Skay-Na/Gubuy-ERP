package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

type ApprovePendingInboundRequest struct {
	UnitCost float64 `json:"unit_cost" binding:"required,gt=0"`
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
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请输入大于 0 的入库成本", "error": err.Error()})
		return
	}
	operator := InventoryOperatorFromContext(c)

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
	if pending.WarehouseType == 1 {
		pending.WarehouseType = 3
	}

	var product models.Product
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, pending.ProductID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "商品不存在"})
		return
	}
	if err := ensureProductCostBaseline(tx, product); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "建立商品成本基准失败"})
		return
	}
	totalCost := req.UnitCost * float64(pending.Quantity)

	beforeStock := 0
	switch pending.WarehouseType {
	case 2:
		beforeStock = product.StoreStock
		product.StoreStock += pending.Quantity
	case 3:
		beforeStock = product.CloudStock
		product.CloudStock += pending.Quantity
	}

	if err := tx.Save(&product).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新商品库存失败"})
		return
	}

	inboundRecord := models.InboundRecord{
		ProductID:      pending.ProductID,
		Quantity:       pending.Quantity,
		UnitCost:       req.UnitCost,
		TotalCost:      totalCost,
		SettlementType: SettlementTypeSaleSettlement,
	}
	if err := tx.Create(&inboundRecord).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建入库记录失败"})
		return
	}
	if err := WriteInventoryLog(tx, product.ID, pending.WarehouseType, "purchase", beforeStock, pending.Quantity, req.UnitCost, fmt.Sprintf("%d", inboundRecord.ID), fmt.Sprintf("员工申请入库通过（申请人：%s）：%s", pending.SubmitterName, pending.Remark), operator); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记录库存日志失败"})
		return
	}
	if err := RecalculateSingleProductCosts(tx, product.ID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "成本重算失败：" + err.Error()})
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
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "入库申请已通过，已记录库存成本，未扣资金账户"})
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

// ListMyPendingInbounds returns only applications submitted by the authenticated employee.
func ListMyPendingInbounds(c *gin.Context) {
	employee, isEmployeeToken, err := currentEmployeeScope(c)
	if !isEmployeeToken {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "仅员工账号可查看个人申请"})
		return
	}
	if err != nil || employee == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工身份无效，请重新登录"})
		return
	}
	if !requireNoActiveBlindStocktake(c) {
		return
	}
	query := config.DB.Preload("Product").Where("submitter_employee_id = ?", employee.ID).Order("created_at desc, id desc")
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	var records []models.PendingInbound
	if err := query.Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取我的入库申请失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": records})
}

type RejectPendingInboundRequest struct {
	Reason string `json:"reason" binding:"required,max=255"`
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
	var req RejectPendingInboundRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Reason) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请填写拒绝原因"})
		return
	}
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
	pending.Status = "rejected"
	pending.RejectionReason = strings.TrimSpace(req.Reason)
	if err := tx.Save(&pending).Error; err != nil {
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
