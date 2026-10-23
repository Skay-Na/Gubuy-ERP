package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TransferRequest 调拨请求
type TransferRequest struct {
	Action   string `json:"action" binding:"required,oneof=store_to_sample sample_to_store"`
	Quantity int    `json:"quantity" binding:"required,gt=0"`
}

// TransferProduct 处理商品调拨
// @Summary TransferProduct
// @Description TransferProduct
// @Tags Products
// @Accept json
// @Produce json
// @Param body body TransferRequest true "库存调拨参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_products_id_transfer
// @Router /api/products/{id}/transfer [put]
func TransferProduct(c *gin.Context) {
	id := c.Param("id")
	var req TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	operator := InventoryOperatorFromContext(c)

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 加锁商品行（FOR UPDATE），防止并发调拨/下单时库存计算竞争
	var product models.Product
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品不存在"})
		return
	}

	var err error
	switch req.Action {
	case "store_to_sample":
		if product.StoreStock < req.Quantity {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "门店库存不足"})
			return
		}
		err = tx.Model(&product).Updates(map[string]interface{}{
			"store_stock":  gorm.Expr("store_stock - ?", req.Quantity),
			"sample_stock": gorm.Expr("sample_stock + ?", req.Quantity),
		}).Error
	case "sample_to_store":
		if product.SampleStock < req.Quantity {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "样机数量不足"})
			return
		}
		err = tx.Model(&product).Updates(map[string]interface{}{
			"sample_stock": gorm.Expr("sample_stock - ?", req.Quantity),
			"store_stock":  gorm.Expr("store_stock + ?", req.Quantity),
		}).Error
	}

	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "调拨失败"})
		return
	}

	// 记录库存流水 (双向流水)
	sourceWarehouse := 0
	destWarehouse := 0
	sourceRemark := ""
	destRemark := ""

	switch req.Action {
	case "store_to_sample":
		sourceWarehouse, destWarehouse = 2, 4
		sourceRemark, destRemark = "调拨出库 -> 样机", "调拨入库 <- 门店"
	case "sample_to_store":
		sourceWarehouse, destWarehouse = 4, 2
		sourceRemark, destRemark = "调拨出库 -> 门店", "调拨入库 <- 样机"
	}

	// 简单的获取变动前库存（基于内存对象，事务内已加锁）
	var sourceBefore, destBefore int
	if sourceWarehouse == 2 {
		sourceBefore = product.StoreStock
	} else if sourceWarehouse == 3 {
		sourceBefore = product.CloudStock
	} else if sourceWarehouse == 4 {
		sourceBefore = product.SampleStock
	}
	if destWarehouse == 2 {
		destBefore = product.StoreStock
	} else if destWarehouse == 3 {
		destBefore = product.CloudStock
	} else if destWarehouse == 4 {
		destBefore = product.SampleStock
	}

	// 写入出库流
	WriteInventoryLog(tx, product.ID, sourceWarehouse, "transfer", sourceBefore, -req.Quantity, product.LatestCost, "", sourceRemark, operator)
	// 写入入库流
	WriteInventoryLog(tx, product.ID, destWarehouse, "transfer", destBefore, req.Quantity, product.LatestCost, "", destRemark, operator)

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "调拨成功"})
}

// StocktakeRequest 盘库请求
type StocktakeRequest struct {
	WarehouseType int    `json:"warehouse_type" binding:"required,oneof=1 2 3"`
	ActualStock   int    `json:"actual_stock" binding:"min=0"`
	ExpectedStock *int   `json:"expected_stock" binding:"required"`
	Remark        string `json:"remark" binding:"max=255"`
	// StocktakeType: 1-正常损耗(盘亏计入财务), 2-业务调整(仅修正库存，不计亏损)
	StocktakeType int `json:"stocktake_type"`
}

// ProductStocktake 盘点接口
// @Summary ProductStocktake
// @Description ProductStocktake
// @Tags Products
// @Accept json
// @Produce json
// @Param body body StocktakeRequest true "库存盘点参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID post_api_products_id_stocktake
// @Router /api/products/{id}/stocktake [post]
func ProductStocktake(c *gin.Context) {
	id := c.Param("id")
	var req StocktakeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	employee, isEmployeeToken, employeeErr := currentEmployeeScope(c)
	if isEmployeeToken && employeeErr != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工身份无效，请重新登录"})
		return
	}
	if employee != nil {
		active, err := hasActiveBlindStocktake()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "检查盲盘状态失败"})
			return
		}
		if active {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "盲盘期间请从当前盘库单提交结果"})
			return
		}
	}
	operator := InventoryOperatorFromContext(c)
	if employee != nil {
		operator.EmployeeID = &employee.ID
		operator.Name = employee.Name
	}
	if req.WarehouseType == 1 {
		req.WarehouseType = 3
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 加锁商品行（FOR UPDATE），防止盘点时被并发订单覆盖库存
	var product models.Product
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品不存在"})
		return
	}

	var beforeStock int
	if req.WarehouseType == 2 {
		beforeStock = product.StoreStock
	} else if req.WarehouseType == 3 {
		beforeStock = product.CloudStock
	}

	diff := req.ActualStock - beforeStock
	if beforeStock != *req.ExpectedStock {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "库存已发生变化，请刷新后重新盘点", "current_stock": beforeStock})
		return
	}
	if diff != 0 && strings.TrimSpace(req.Remark) == "" {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "盘点数量有差异时请填写差异原因"})
		return
	}

	stocktakeType := req.StocktakeType
	if stocktakeType == 0 {
		stocktakeType = 1
	}

	if err := applyStocktake(tx, &product, req.WarehouseType, beforeStock, req.ActualStock, stocktakeType, strings.TrimSpace(req.Remark), operator, ""); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存盘点结果失败"})
		return
	}

	// 盘亏损耗不再从现金账户扣减，只在库存日志和盘点记录中呈现，遵循正确的资产减值财务规范

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "盘点完成", "difference": diff})
}

func applyStocktake(tx *gorm.DB, product *models.Product, warehouseType int, beforeStock int, actualStock int, stocktakeType int, remark string, operator InventoryOperator, relatedNo string) error {
	if err := ensureProductCostBaseline(tx, *product); err != nil {
		return fmt.Errorf("建立商品成本基准: %w", err)
	}
	updateField := "cloud_stock"
	if warehouseType == 2 {
		updateField = "store_stock"
	}
	if err := tx.Model(product).UpdateColumn(updateField, actualStock).Error; err != nil {
		return err
	}
	record := models.StocktakeRecord{
		ProductID: product.ID, WarehouseType: warehouseType, StocktakeType: stocktakeType,
		BeforeStock: beforeStock, AfterStock: actualStock, Difference: actualStock - beforeStock, Remark: remark,
	}
	if err := tx.Create(&record).Error; err != nil {
		return err
	}
	if err := WriteInventoryLog(tx, product.ID, warehouseType, "stocktake", beforeStock, actualStock-beforeStock, orderCostSnapshot(*product), relatedNo, fmt.Sprintf("库存盘点：%s", remark), operator); err != nil {
		return err
	}
	return RecalculateSingleProductCosts(tx, product.ID)
}

type ReviewPendingStocktakeRequest struct {
	ReviewRemark string `json:"review_remark" binding:"max=255"`
}

// ListPendingStocktakes returns stocktake requests for administrator review.
func ListPendingStocktakes(c *gin.Context) {
	query := config.DB.Preload("Product").Order("created_at desc, id desc")
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	var records []models.PendingStocktake
	if err := query.Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取盘点审核列表失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": records})
}

// ListMyPendingStocktakes returns stocktake requests for the authenticated employee.
func ListMyPendingStocktakes(c *gin.Context) {
	employee, isEmployeeToken, err := currentEmployeeScope(c)
	if !isEmployeeToken {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "仅员工账号可查看个人盘点申请"})
		return
	}
	if err != nil || employee == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工身份无效，请重新登录"})
		return
	}
	if !requireNoActiveBlindStocktake(c) {
		return
	}
	query := config.DB.Preload("Product").Where("operator_id = ?", employee.ID).Order("created_at desc, id desc")
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	var records []models.PendingStocktake
	if err := query.Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取我的盘点申请失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": records})
}

func ApprovePendingStocktake(c *gin.Context) {
	var req ReviewPendingStocktakeRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "审核参数错误"})
			return
		}
	}
	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "启动事务失败"})
		return
	}
	var pending models.PendingStocktake
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&pending, c.Param("id")).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "盘点申请不存在"})
		return
	}
	if pending.Status != "pending" {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "该盘点申请已处理"})
		return
	}
	var product models.Product
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, pending.ProductID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品不存在"})
		return
	}
	currentStock := product.CloudStock
	if pending.WarehouseType == 2 {
		currentStock = product.StoreStock
	}
	if currentStock != pending.ExpectedStock {
		now := time.Now()
		pending.Status = "recount_required"
		pending.ReviewRemark = "审核前发现库存已变化，已退回员工重新盘点"
		if reviewRemark := strings.TrimSpace(req.ReviewRemark); reviewRemark != "" {
			suffix := "；管理员备注："
			availableRunes := 255 - len([]rune(pending.ReviewRemark)) - len([]rune(suffix))
			reviewRemarkRunes := []rune(reviewRemark)
			if len(reviewRemarkRunes) > availableRunes {
				reviewRemarkRunes = reviewRemarkRunes[:availableRunes]
			}
			pending.ReviewRemark += suffix + string(reviewRemarkRunes)
		}
		pending.ReviewedBy = c.GetString("username")
		pending.ReviewedAt = &now
		if err := tx.Save(&pending).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新过期盘点申请失败"})
			return
		}
		if err := updateStocktakeBatchItemReview(tx, pending, "conflict"); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "退回员工重盘失败"})
			return
		}
		if err := tx.Commit().Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存过期盘点申请失败"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "库存已发生变化，盘点申请已退回员工重新盘点", "current_stock": currentStock})
		return
	}
	operator := InventoryOperator{EmployeeID: pending.OperatorID, Name: pending.OperatorName}
	if err := applyStocktake(tx, &product, pending.WarehouseType, currentStock, pending.ActualStock, pending.StocktakeType, pending.Remark, operator, fmt.Sprintf("stocktake-request-%d", pending.ID)); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "应用盘点结果失败"})
		return
	}
	now := time.Now()
	pending.Status = "approved"
	pending.ReviewRemark = req.ReviewRemark
	pending.ReviewedBy = c.GetString("username")
	pending.ReviewedAt = &now
	if err := tx.Save(&pending).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新审核状态失败"})
		return
	}
	if err := updateStocktakeBatchItemReview(tx, pending, "approved"); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新盘库单审核状态失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "提交审核失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "盘点差异已审核并应用"})
}

func RejectPendingStocktake(c *gin.Context) {
	var req ReviewPendingStocktakeRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.ReviewRemark) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请填写驳回原因"})
		return
	}
	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "启动事务失败"})
		return
	}
	var pending models.PendingStocktake
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&pending, c.Param("id")).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "盘点申请不存在"})
		return
	}
	if pending.Status != "pending" {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "该盘点申请已处理"})
		return
	}
	now := time.Now()
	pending.Status = "rejected"
	pending.ReviewRemark = strings.TrimSpace(req.ReviewRemark)
	pending.ReviewedBy = c.GetString("username")
	pending.ReviewedAt = &now
	if err := tx.Save(&pending).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存驳回结果失败"})
		return
	}
	if err := updateStocktakeBatchItemReview(tx, pending, "rejected"); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新盘库单审核状态失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "提交审核失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已驳回盘点申请"})
}
