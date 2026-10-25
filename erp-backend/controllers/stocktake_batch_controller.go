package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errStocktakeBatchItemNotEditable = errors.New("stocktake batch item is not editable")
var errStocktakeBatchNotActive = errors.New("stocktake batch is not active")
var errStocktakeBatchHasInventoryEffects = errors.New("stocktake batch has inventory effects or review records")

// CreateStocktakeBatchRequest selects the warehouse and product filters for a blind-count batch.
type CreateStocktakeBatchRequest struct {
	WarehouseType int    `json:"warehouse_type" binding:"required,oneof=2 3"`
	CategoryID    string `json:"category_id" binding:"max=20"`
	Keyword       string `json:"keyword" binding:"max=255"`
}

// StocktakeBatchItemInput contains an employee's count for one snapshotted product.
type StocktakeBatchItemInput struct {
	ID          uint   `json:"id" binding:"required,gt=0"`
	ActualStock *int   `json:"actual_stock"`
	Remark      string `json:"remark" binding:"max=255"`
}

// UpdateStocktakeBatchItemsRequest saves counts and the adjustment nature for a batch.
type UpdateStocktakeBatchItemsRequest struct {
	Items         []StocktakeBatchItemInput `json:"items" binding:"required,min=1,dive"`
	StocktakeType int                       `json:"stocktake_type" binding:"required,oneof=1 2"`
}

type ConfirmStocktakeBatchPrintRequest struct {
	Confirmed bool `json:"confirmed"`
}

type stocktakeBatchSubmissionResult struct {
	ItemID      uint   `json:"item_id"`
	ProductName string `json:"product_name"`
	Status      string `json:"status"`
	Difference  int    `json:"difference"`
	Message     string `json:"message,omitempty"`
}

// CreateStocktakeBatch 创建盘点批次
// @Summary 创建盘点批次
// @Description 创建盘点批次
// @Tags StocktakeBatch
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_stocktake_batches
// @Router /api/stocktake-batches [post]
// CreateStocktakeBatch snapshots all products matching the current keyword/category filters.
func CreateStocktakeBatch(c *gin.Context) {
	var req CreateStocktakeBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "盘库单参数错误"})
		return
	}

	employee, isEmployeeToken, err := currentEmployeeScope(c)
	if !isEmployeeToken {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "仅员工账号可创建盘库单"})
		return
	}
	if err != nil || employee == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工身份无效，请重新登录"})
		return
	}
	active, err := hasActiveBlindStocktake()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "检查现有盘库任务失败"})
		return
	}
	if active {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "当前已有进行中的纸质盘库单，请先完成现有任务"})
		return
	}

	query := config.DB.Model(&models.Product{})
	keyword := strings.TrimSpace(req.Keyword)
	categoryID := strings.TrimSpace(req.CategoryID)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR sku LIKE ?", like, like)
	}
	categoryName := ""
	if categoryID != "" {
		id, parseErr := strconv.ParseUint(categoryID, 10, 64)
		if parseErr != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "商品分类无效"})
			return
		}
		query = query.Where("category_id = ?", id)
		var category models.Category
		categoryErr := config.DB.Select("name").First(&category, id).Error
		if categoryErr == nil {
			categoryName = category.Name
		} else if !errors.Is(categoryErr, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取商品分类失败"})
			return
		}
	}

	products := []models.Product{}
	if err := query.Order("name asc, id asc").Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取盘库商品失败"})
		return
	}
	if len(products) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "当前筛选范围没有商品"})
		return
	}

	operator := InventoryOperator{Name: employee.Name, EmployeeID: &employee.ID}
	now := time.Now()
	batchNo, err := newStocktakeBatchNo(now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "生成盘库单号失败"})
		return
	}
	scopeLabelParts := []string{}
	if categoryName != "" {
		scopeLabelParts = append(scopeLabelParts, "分类："+categoryName)
	} else if categoryID != "" {
		scopeLabelParts = append(scopeLabelParts, "指定分类")
	}
	if keyword != "" {
		keywordLabel := []rune(keyword)
		if len(keywordLabel) > 80 {
			keywordLabel = keywordLabel[:80]
		}
		scopeLabelParts = append(scopeLabelParts, "关键词："+string(keywordLabel))
	}
	scopeLabel := "全部商品"
	if len(scopeLabelParts) > 0 {
		scopeLabel = strings.Join(scopeLabelParts, "　")
	}
	batch := models.StocktakeBatch{
		BatchNo:       batchNo,
		WarehouseType: req.WarehouseType,
		ScopeLabel:    scopeLabel,
		OperatorID:    operator.EmployeeID,
		OperatorName:  operator.Name,
		StocktakeType: 1,
		Status:        "in_progress",
	}
	items := make([]models.StocktakeBatchItem, 0, len(products))
	for _, product := range products {
		stock := product.CloudStock
		if req.WarehouseType == 2 {
			stock = product.StoreStock
		}
		remark := ""
		if product.Remark != nil {
			remark = *product.Remark
		}
		items = append(items, models.StocktakeBatchItem{
			ProductID:     product.ID,
			ProductName:   product.Name,
			ProductRemark: remark,
			ExpectedStock: stock,
			Status:        "uncounted",
		})
	}

	err = config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].BatchID = batch.ID
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建盘库单失败"})
		return
	}
	batch.Items = items
	c.JSON(http.StatusCreated, gin.H{"code": 201, "data": batch})
}

func newStocktakeBatchNo(now time.Time) (string, error) {
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("generate stocktake batch suffix: %w", err)
	}
	return fmt.Sprintf("PD%s-%s", now.Format("20060102-150405"), hex.EncodeToString(suffix)), nil
}

// ListMyStocktakeBatches 获取我的盘点批次列表
// @Summary 获取我的盘点批次列表
// @Description 获取我的盘点批次列表
// @Tags StocktakeBatch
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_stocktake_batches
// @Router /api/stocktake-batches [get]
// ListMyStocktakeBatches lists recent batches owned by the authenticated employee.
func ListMyStocktakeBatches(c *gin.Context) {
	employee, ok := stocktakeBatchEmployee(c)
	if !ok {
		return
	}
	blindMode, err := hasActiveBlindStocktake()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "检查盘库状态失败"})
		return
	}
	batches := []models.StocktakeBatch{}
	if err := config.DB.Where("operator_id = ?", employee.ID).
		Order("created_at desc, id desc").Limit(100).Find(&batches).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取盘库单列表失败"})
		return
	}

	data := make([]gin.H, 0, len(batches))
	for _, batch := range batches {
		var itemCount int64
		var countedCount int64
		if err := config.DB.Model(&models.StocktakeBatchItem{}).
			Where("batch_id = ?", batch.ID).Count(&itemCount).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取盘库单统计失败"})
			return
		}
		if err := config.DB.Model(&models.StocktakeBatchItem{}).
			Where("batch_id = ? AND actual_stock IS NOT NULL", batch.ID).Count(&countedCount).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取盘库单统计失败"})
			return
		}
		data = append(data, gin.H{
			"id": batch.ID, "batch_no": batch.BatchNo, "warehouse_type": batch.WarehouseType,
			"scope_label": batch.ScopeLabel, "operator_name": batch.OperatorName,
			"stocktake_type": batch.StocktakeType, "print_count": batch.PrintCount,
			"printed_at": batch.PrintedAt, "status": batch.Status, "created_at": batch.CreatedAt,
			"item_count": itemCount, "counted_count": countedCount,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": data, "blind_stocktake_mode": blindMode})
}

// ListAdminStocktakeBatches 管理端获取全部盘点批次
// @Summary 管理端获取全部盘点批次
// @Description 管理端获取全部盘点批次
// @Tags StocktakeBatch
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_admin_stocktake_batches
// @Router /api/admin/stocktake-batches [get]
// ListAdminStocktakeBatches lists recent stocktake batches for administrator management.
func ListAdminStocktakeBatches(c *gin.Context) {
	batches := []models.StocktakeBatch{}
	if err := config.DB.Order("created_at desc, id desc").Limit(200).Find(&batches).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取盘库单列表失败"})
		return
	}

	type batchItemCounts struct {
		BatchID      uint  `gorm:"column:batch_id"`
		ItemCount    int64 `gorm:"column:item_count"`
		CountedCount int64 `gorm:"column:counted_count"`
	}
	counts := []batchItemCounts{}
	if len(batches) > 0 {
		batchIDs := make([]uint, 0, len(batches))
		for _, batch := range batches {
			batchIDs = append(batchIDs, batch.ID)
		}
		if err := config.DB.Model(&models.StocktakeBatchItem{}).
			Select("batch_id, COUNT(*) AS item_count, COUNT(actual_stock) AS counted_count").
			Where("batch_id IN ?", batchIDs).Group("batch_id").Scan(&counts).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取盘库单统计失败"})
			return
		}
	}
	countsByBatchID := make(map[uint]batchItemCounts, len(counts))
	for _, count := range counts {
		countsByBatchID[count.BatchID] = count
	}

	data := make([]gin.H, 0, len(batches))
	for _, batch := range batches {
		count := countsByBatchID[batch.ID]
		data = append(data, gin.H{
			"id": batch.ID, "batch_no": batch.BatchNo, "warehouse_type": batch.WarehouseType,
			"scope_label": batch.ScopeLabel, "operator_name": batch.OperatorName,
			"status": batch.Status, "created_at": batch.CreatedAt,
			"item_count": count.ItemCount, "counted_count": count.CountedCount,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": data})
}

// TerminateStocktakeBatch 终止盘点批次
// @Summary 终止盘点批次
// @Description 终止盘点批次
// @Tags StocktakeBatch
// @Param id path int true "id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_admin_stocktake_batches_id_terminate
// @Router /api/admin/stocktake-batches/{id}/terminate [post]
// TerminateStocktakeBatch ends an active batch while preserving its count history.
func TerminateStocktakeBatch(c *gin.Context) {
	batchID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || batchID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "盘库单编号无效"})
		return
	}

	err = config.DB.Transaction(func(tx *gorm.DB) error {
		var batch models.StocktakeBatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&batch, batchID).Error; err != nil {
			return err
		}
		if batch.Status != "in_progress" {
			return errStocktakeBatchNotActive
		}
		return tx.Model(&batch).Update("status", "terminated").Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "盘库单不存在"})
		return
	}
	if errors.Is(err, errStocktakeBatchNotActive) {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "只有进行中的盘库单可以终止"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "终止盘库单失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "盘库单已终止，已录入信息已保留"})
}

// DeleteStocktakeBatch 删除盘点批次
// @Summary 删除盘点批次
// @Description 删除盘点批次
// @Tags StocktakeBatch
// @Param id path int true "id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID delete_api_admin_stocktake_batches_id
// @Router /api/admin/stocktake-batches/{id} [delete]
// DeleteStocktakeBatch removes a batch only when it has no inventory changes or linked review records.
func DeleteStocktakeBatch(c *gin.Context) {
	batchID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || batchID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "盘库单编号无效"})
		return
	}

	err = config.DB.Transaction(func(tx *gorm.DB) error {
		var batch models.StocktakeBatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&batch, batchID).Error; err != nil {
			return err
		}

		var affectedItems int64
		if err := tx.Model(&models.StocktakeBatchItem{}).
			Where("batch_id = ? AND status IN ?", batch.ID, []string{"applied", "pending_review", "approved"}).
			Count(&affectedItems).Error; err != nil {
			return err
		}
		if affectedItems > 0 {
			return errStocktakeBatchHasInventoryEffects
		}

		itemIDs := tx.Model(&models.StocktakeBatchItem{}).Select("id").Where("batch_id = ?", batch.ID)
		var linkedReviews int64
		if err := tx.Model(&models.PendingStocktake{}).
			Where("stocktake_batch_item_id IN (?)", itemIDs).Count(&linkedReviews).Error; err != nil {
			return err
		}
		if linkedReviews > 0 {
			return errStocktakeBatchHasInventoryEffects
		}

		if err := tx.Where("batch_id = ?", batch.ID).Delete(&models.StocktakeBatchItem{}).Error; err != nil {
			return err
		}
		return tx.Delete(&batch).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "盘库单不存在"})
		return
	}
	if errors.Is(err, errStocktakeBatchHasInventoryEffects) {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "该盘库单已产生库存调整或审核记录，不能删除；可以终止未完成的盘库单"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "删除盘库单失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "盘库单及其清点信息已删除"})
}

// GetStocktakeBatch 获取盘点批次详情
// @Summary 获取盘点批次详情
// @Description 获取盘点批次详情
// @Tags StocktakeBatch
// @Param id path int true "id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_stocktake_batches_id
// @Router /api/stocktake-batches/{id} [get]
// GetStocktakeBatch returns a batch and its product snapshots without exposing book quantities.
func GetStocktakeBatch(c *gin.Context) {
	batch, ok := ownedStocktakeBatch(c, true)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": batch})
}

// MarkStocktakeBatchPrinted 记录盘点批次打印
// @Summary 记录盘点批次打印
// @Description 记录盘点批次打印
// @Tags StocktakeBatch
// @Param id path int true "id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_stocktake_batches_id_print
// @Router /api/stocktake-batches/{id}/print [post]
// MarkStocktakeBatchPrinted records every print/reprint action for the task.
func MarkStocktakeBatchPrinted(c *gin.Context) {
	var req ConfirmStocktakeBatchPrintRequest
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirmed {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请在确认纸单已成功打印后记录"})
		return
	}
	batch, ok := ownedStocktakeBatch(c, false)
	if !ok {
		return
	}
	now := time.Now()
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		var current models.StocktakeBatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, batch.ID).Error; err != nil {
			return err
		}
		if current.Status != "in_progress" {
			return errStocktakeBatchNotActive
		}
		updates := map[string]interface{}{"print_count": gorm.Expr("print_count + 1"), "printed_at": &now}
		return tx.Model(&current).Updates(updates).Error
	})
	if errors.Is(err, errStocktakeBatchNotActive) {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "盘库单已结束，不能继续打印"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记录打印状态失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已记录打印"})
}

// UpdateStocktakeBatchItems 保存盘点批次明细
// @Summary 保存盘点批次明细
// @Description 保存盘点批次明细
// @Tags StocktakeBatch
// @Param id path int true "id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID put_api_stocktake_batches_id_items
// @Router /api/stocktake-batches/{id}/items [put]
// UpdateStocktakeBatchItems saves employee-entered counts without exposing book stock.
func UpdateStocktakeBatchItems(c *gin.Context) {
	var req UpdateStocktakeBatchItemsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "盘点数量参数错误"})
		return
	}
	batch, ok := ownedStocktakeBatch(c, false)
	if !ok {
		return
	}
	if batch.Status != "in_progress" {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": stocktakeBatchInactiveMessage(batch.Status)})
		return
	}

	ids := make([]uint, 0, len(req.Items))
	for _, input := range req.Items {
		if input.ActualStock != nil && *input.ActualStock < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "实盘数量不能小于 0"})
			return
		}
		ids = append(ids, input.ID)
	}
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		var current models.StocktakeBatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, batch.ID).Error; err != nil {
			return err
		}
		if current.Status != "in_progress" {
			return errStocktakeBatchNotActive
		}
		rows := []models.StocktakeBatchItem{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("batch_id = ? AND id IN ?", batch.ID, ids).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(req.Items) {
			return gorm.ErrRecordNotFound
		}
		byID := make(map[uint]models.StocktakeBatchItem, len(rows))
		for _, row := range rows {
			byID[row.ID] = row
		}
		for _, input := range req.Items {
			row, exists := byID[input.ID]
			if !exists {
				return gorm.ErrRecordNotFound
			}
			if row.Status != "uncounted" {
				return errStocktakeBatchItemNotEditable
			}
			row.ActualStock = input.ActualStock
			row.Remark = strings.TrimSpace(input.Remark)
			if row.ActualStock == nil {
				row.Remark = ""
			}
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.StocktakeBatch{}).Where("id = ?", batch.ID).
			Update("stocktake_type", req.StocktakeType).Error
	})
	if err != nil {
		if errors.Is(err, errStocktakeBatchNotActive) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "盘库单已结束，不能继续录入"})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "盘库单商品不存在"})
			return
		}
		if errors.Is(err, errStocktakeBatchItemNotEditable) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "该商品已提交或需要先刷新后重新盘点"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存盘点数量失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "盘点数量已保存"})
}

// SubmitStocktakeBatch 提交盘点批次
// @Summary 提交盘点批次
// @Description 提交盘点批次
// @Tags StocktakeBatch
// @Param id path int true "id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_stocktake_batches_id_submit
// @Router /api/stocktake-batches/{id}/submit [post]
// SubmitStocktakeBatch applies counted items through the existing inventory review rules.
func SubmitStocktakeBatch(c *gin.Context) {
	batch, ok := ownedStocktakeBatch(c, false)
	if !ok {
		return
	}
	if batch.Status != "in_progress" {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": stocktakeBatchInactiveMessage(batch.Status)})
		return
	}
	items := []models.StocktakeBatchItem{}
	if err := config.DB.Where("batch_id = ? AND actual_stock IS NOT NULL AND status = ?", batch.ID, "uncounted").
		Order("id asc").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取盘点结果失败"})
		return
	}
	if len(items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请先填写已清点商品的实盘数量"})
		return
	}

	results := make([]stocktakeBatchSubmissionResult, 0, len(items))
	for _, item := range items {
		result := submitStocktakeBatchItem(batch, item.ID)
		results = append(results, result)
	}
	if err := refreshStocktakeBatchStatus(batch.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新盘库单状态失败", "results": results})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"results": results}})
}

func submitStocktakeBatchItem(batch models.StocktakeBatch, itemID uint) stocktakeBatchSubmissionResult {
	result := stocktakeBatchSubmissionResult{ItemID: itemID}
	tx := config.DB.Begin()
	if tx.Error != nil {
		result.Status = "error"
		result.Message = "启动库存事务失败"
		return result
	}
	var currentBatch models.StocktakeBatch
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&currentBatch, batch.ID).Error; err != nil {
		tx.Rollback()
		result.Status = "error"
		result.Message = "盘库单不存在"
		return result
	}
	if currentBatch.Status != "in_progress" {
		tx.Rollback()
		result.Status = "error"
		result.Message = "盘库单已结束，不能继续提交"
		return result
	}
	var item models.StocktakeBatchItem
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, itemID).Error; err != nil {
		tx.Rollback()
		result.Status = "error"
		result.Message = "盘库商品不存在"
		return result
	}
	if item.BatchID != batch.ID || item.ActualStock == nil {
		tx.Rollback()
		result.Status = item.Status
		result.Message = "该商品尚未录入实盘数量"
		return result
	}
	result.ProductName = item.ProductName
	if item.Status != "uncounted" {
		tx.Rollback()
		result.Status = item.Status
		return result
	}
	if *item.ActualStock < 0 {
		tx.Rollback()
		result.Status = "error"
		result.Message = "实盘数量不能小于 0"
		return result
	}
	difference := *item.ActualStock - item.ExpectedStock
	if difference != 0 && strings.TrimSpace(item.Remark) == "" {
		tx.Rollback()
		result.Status = "error"
		result.Message = "数量有差异，请填写差异说明"
		return result
	}

	var product models.Product
	productErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, item.ProductID).Error
	if errors.Is(productErr, gorm.ErrRecordNotFound) {
		item.Status = "conflict"
		if saveErr := tx.Save(&item).Error; saveErr != nil {
			tx.Rollback()
			result.Status = "error"
			result.Message = "商品不存在且冲突状态保存失败"
			return result
		}
		if commitErr := tx.Commit().Error; commitErr != nil {
			result.Status = "error"
			result.Message = "保存商品冲突失败"
			return result
		}
		result.Status = item.Status
		result.Message = "商品已删除或不可用"
		return result
	}
	if productErr != nil {
		tx.Rollback()
		result.Status = "error"
		result.Message = "读取当前库存失败"
		return result
	}

	currentStock := product.CloudStock
	if batch.WarehouseType == 2 {
		currentStock = product.StoreStock
	}
	if currentStock != item.ExpectedStock {
		item.Status = "conflict"
		if err := tx.Save(&item).Error; err != nil {
			tx.Rollback()
			result.Status = "error"
			result.Message = "保存库存冲突失败"
			return result
		}
		if err := tx.Commit().Error; err != nil {
			result.Status = "error"
			result.Message = "保存库存冲突失败"
			return result
		}
		result.Status = item.Status
		result.Message = "盘点期间库存已变化，请重新清点"
		return result
	}

	stocktakeType := batch.StocktakeType
	if stocktakeType == 0 {
		stocktakeType = 1
	}
	relatedNo := fmt.Sprintf("%s-%d", batch.BatchNo, item.ID)
	operator := InventoryOperator{EmployeeID: batch.OperatorID, Name: batch.OperatorName}
	if err := applyStocktake(
		tx,
		&product,
		batch.WarehouseType,
		currentStock,
		*item.ActualStock,
		stocktakeType,
		strings.TrimSpace(item.Remark),
		operator,
		relatedNo,
	); err != nil {
		tx.Rollback()
		result.Status = "error"
		result.Message = "保存盘点结果失败"
		return result
	}
	item.Difference = difference
	item.Status = "applied"

	if err := tx.Save(&item).Error; err != nil {
		tx.Rollback()
		result.Status = "error"
		result.Message = "更新盘库商品状态失败"
		return result
	}
	if err := tx.Commit().Error; err != nil {
		result.Status = "error"
		result.Message = "提交盘点结果失败"
		return result
	}
	result.Status = item.Status
	result.Difference = difference
	return result
}

// RefreshStocktakeBatchItem 刷新盘点批次单项库存快照
// @Summary 刷新盘点批次单项库存快照
// @Description 刷新盘点批次单项库存快照
// @Tags StocktakeBatch
// @Param id path int true "id"
// @Param item_id path int true "item_id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_stocktake_batches_id_items_item_id_refresh
// @Router /api/stocktake-batches/{id}/items/{item_id}/refresh [post]
// RefreshStocktakeBatchItem resets a conflicted item to the latest snapshot for recounting.
func RefreshStocktakeBatchItem(c *gin.Context) {
	batch, ok := ownedStocktakeBatch(c, false)
	if !ok {
		return
	}
	itemID, err := strconv.ParseUint(c.Param("item_id"), 10, 64)
	if err != nil || itemID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "盘库商品编号无效"})
		return
	}
	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "启动库存事务失败"})
		return
	}
	var currentBatch models.StocktakeBatch
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&currentBatch, batch.ID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "盘库单不存在"})
		return
	}
	if currentBatch.Status != "in_progress" {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "盘库单已结束，不能继续重盘"})
		return
	}
	var item models.StocktakeBatchItem
	itemErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("batch_id = ?", batch.ID).First(&item, itemID).Error
	if errors.Is(itemErr, gorm.ErrRecordNotFound) {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "盘库商品不存在"})
		return
	}
	if itemErr != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取盘库商品失败"})
		return
	}
	if item.Status != "conflict" {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "只有库存冲突商品可以刷新后重新清点"})
		return
	}
	var product models.Product
	productErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, item.ProductID).Error
	if errors.Is(productErr, gorm.ErrRecordNotFound) {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品已删除或不可用"})
		return
	}
	if productErr != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取当前库存失败"})
		return
	}
	item.ExpectedStock = product.CloudStock
	if batch.WarehouseType == 2 {
		item.ExpectedStock = product.StoreStock
	}
	item.ActualStock = nil
	item.Remark = ""
	item.Difference = 0
	item.PendingStocktakeID = nil
	item.Status = "uncounted"
	if err := tx.Save(&item).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "刷新盘点快照失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存盘点快照失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "库存快照已刷新，请重新清点该商品"})
}

func ownedStocktakeBatch(c *gin.Context, withItems bool) (models.StocktakeBatch, bool) {
	employee, ok := stocktakeBatchEmployee(c)
	if !ok {
		return models.StocktakeBatch{}, false
	}
	var batch models.StocktakeBatch
	query := config.DB.Where("operator_id = ?", employee.ID)
	if withItems {
		query = query.Preload("Items", func(db *gorm.DB) *gorm.DB {
			return db.Order("id asc")
		})
	}
	if err := query.First(&batch, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "盘库单不存在"})
		return models.StocktakeBatch{}, false
	}
	return batch, true
}

func stocktakeBatchEmployee(c *gin.Context) (*models.Employee, bool) {
	employee, isEmployeeToken, err := currentEmployeeScope(c)
	if !isEmployeeToken {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "仅员工账号可操作盘库单"})
		return nil, false
	}
	if err != nil || employee == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工身份无效，请重新登录"})
		return nil, false
	}
	return employee, true
}

// employeeBlindStocktakeMode reports whether any paper stocktake is unfinished.
// Product stock stays hidden from employee accounts until all counted items,
// including differences waiting for review, are resolved.
func employeeBlindStocktakeMode(c *gin.Context) (bool, bool) {
	employee, isEmployeeToken, err := currentEmployeeScope(c)
	if !isEmployeeToken {
		return false, true
	}
	if err != nil || employee == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工身份无效，请重新登录"})
		return false, false
	}
	active, err := hasActiveBlindStocktake()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "检查盲盘状态失败"})
		return false, false
	}
	return active, true
}

func hasActiveBlindStocktake() (bool, error) {
	var count int64
	err := config.DB.Model(&models.StocktakeBatch{}).
		Where("status = ?", "in_progress").
		Count(&count).Error
	return count > 0, err
}

func requireNoActiveBlindStocktake(c *gin.Context) bool {
	active, ok := employeeBlindStocktakeMode(c)
	if !ok {
		return false
	}
	if active {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "盲盘期间暂不可查看库存明细"})
		return false
	}
	return true
}

func refreshStocktakeBatchStatus(batchID uint) error {
	return config.DB.Transaction(func(tx *gorm.DB) error {
		return refreshStocktakeBatchStatusInTransaction(tx, batchID)
	})
}

func updateStocktakeBatchItemReview(tx *gorm.DB, pending models.PendingStocktake, status string) error {
	if pending.StocktakeBatchItemID == nil {
		return nil
	}
	var item models.StocktakeBatchItem
	if err := tx.Select("id", "batch_id").First(&item, *pending.StocktakeBatchItemID).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{
		"status": status, "difference": pending.Difference,
	}
	if status == "conflict" {
		updates["pending_stocktake_id"] = nil
	}
	if err := tx.Model(&item).Updates(updates).Error; err != nil {
		return err
	}
	return refreshStocktakeBatchStatusInTransaction(tx, item.BatchID)
}

func refreshStocktakeBatchStatusInTransaction(tx *gorm.DB, batchID uint) error {
	if batchID == 0 {
		return nil
	}
	var batch models.StocktakeBatch
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "status").First(&batch, batchID).Error; err != nil {
		return err
	}
	if batch.Status == "terminated" {
		return nil
	}
	var unfinished int64
	if err := tx.Model(&models.StocktakeBatchItem{}).
		Where("batch_id = ? AND status NOT IN ?", batchID, []string{"applied", "approved", "rejected"}).
		Count(&unfinished).Error; err != nil {
		return err
	}
	status := "in_progress"
	if unfinished == 0 {
		status = "completed"
	}
	return tx.Model(&models.StocktakeBatch{}).Where("id = ?", batchID).Update("status", status).Error
}

func stocktakeBatchInactiveMessage(status string) string {
	if status == "terminated" {
		return "该盘库单已被管理员终止"
	}
	return "该盘库单已完成"
}
