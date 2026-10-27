package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"erp-backend/utils"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type InventoryOperator struct {
	EmployeeID *uint
	Name       string
}

// InventoryOperatorFromContext derives the operator from the verified JWT identity.
func InventoryOperatorFromContext(c *gin.Context) InventoryOperator {
	username := c.GetString("username")
	operator := InventoryOperator{Name: username}
	if !strings.HasPrefix(username, "emp_") {
		return operator
	}

	empNo := strings.TrimPrefix(username, "emp_")
	var employee models.Employee
	if err := config.DB.Select("id", "name").Where("emp_no = ?", empNo).First(&employee).Error; err != nil {
		operator.Name = "员工（工号：" + empNo + "）"
		return operator
	}

	operator.EmployeeID = &employee.ID
	operator.Name = employee.Name
	return operator
}

// WriteInventoryLog 在事务中写入一条库存流水记录（公用辅助函数）
// tx: 当前事务（保证原子性，流水与主操作同事务回滚）
// productID: 商品ID
// warehouseType: 1-主仓, 2-门店, 3-云仓, 4-样机
// logType: purchase / sale / transfer / stocktake / cancel
// beforeQty: 操作前库存
// changeQty: 变动量（正=增加，负=减少）
// relatedNo: 关联单号（如订单号）
// remark: 说明
func WriteInventoryLog(tx *gorm.DB, productID uint, warehouseType int, logType string, beforeQty int, changeQty int, unitCost float64, relatedNo string, remark string, operators ...InventoryOperator) error {
	log := models.InventoryLog{
		ProductID:     productID,
		WarehouseType: warehouseType,
		LogType:       logType,
		ChangeQty:     changeQty,
		BeforeQty:     beforeQty,
		AfterQty:      beforeQty + changeQty,
		UnitCost:      unitCost,
		RelatedNo:     relatedNo,
		Remark:        remark,
	}
	if len(operators) > 0 {
		log.OperatorID = operators[0].EmployeeID
		log.OperatorName = operators[0].Name
	}
	return tx.Create(&log).Error
}

// applyInventoryLogScopeFilters 库存流水的范围过滤：商品分类（选中一级时含二级分类的商品）与日期区间
func applyInventoryLogScopeFilters(query *gorm.DB, c *gin.Context) *gorm.DB {
	categoryID := c.Query("category_id")
	if categoryID != "" {
		query = query.Joins("JOIN products ON inventory_logs.product_id = products.id").
			Where("(products.category_id = ? OR products.category_id IN (SELECT id FROM categories WHERE parent_id = ? AND deleted_at IS NULL))", categoryID, categoryID)
	}
	startDate := c.Query("start_date")
	if startDate != "" {
		query = query.Where("inventory_logs.created_at >= ?", startDate+" 00:00:00")
	}
	endDate := c.Query("end_date")
	if endDate != "" {
		query = query.Where("inventory_logs.created_at <= ?", endDate+" 23:59:59")
	}
	return query
}

// GetInventoryLogs 查询指定商品的库存流水记录
// GET /api/products/{id}/logs?warehouse_type=&page=&page_size=
// @Summary 获取指定商品库存流水
// @Description 获取指定商品库存流水
// @Tags Products
// @Accept json
// @Param warehouse_type query int false "仓库类型 1主仓 2门店 3云仓 4样机"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID get_api_products_id_logs
// @Router /api/products/{id}/logs [get]
func GetInventoryLogs(c *gin.Context) {
	if !requireNoActiveBlindStocktake(c) {
		return
	}
	productID := c.Param("id")
	warehouseType := c.Query("warehouse_type")
	logType := c.Query("log_type")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	query := config.DB.
		Preload("Product").
		Where("product_id = ?", productID).
		Order("created_at desc, id desc")

	if warehouseType != "" {
		query = query.Where("warehouse_type = ?", warehouseType)
	}
	if logType != "" {
		query = query.Where("log_type = ?", logType)
	}

	var total int64
	if err := query.Model(&models.InventoryLog{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "统计库存流水失败"})
		return
	}

	offset := (page - 1) * pageSize

	var logs []models.InventoryLog
	if err := query.Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"data":      logs,
	})
}

// GetGlobalInventoryLogs 查询全局库存流水记录（不限商品）
// GET /api/inventory/logs?warehouse_type=&log_type=&product_id=&page=&page_size=
// @Summary 获取全局库存流水
// @Description 获取全局库存流水
// @Tags Inventory
// @Accept json
// @Param warehouse_type query int false "仓库类型"
// @Param log_type query string false "流水类型 (purchase/sale/transfer等)"
// @Param product_id query int false "商品ID"
// @Param operator_id query int false "操作员工ID"
// @Param keyword query string false "模糊搜索关键字"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_inventory_logs
// @Router /api/inventory/logs [get]
func GetGlobalInventoryLogs(c *gin.Context) {
	if !requireNoActiveBlindStocktake(c) {
		return
	}
	warehouseType := c.Query("warehouse_type")
	logType := c.Query("log_type")
	productID := c.Query("product_id")
	keyword := c.Query("keyword")
	operatorID := c.Query("operator_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	query := config.DB.
		Model(&models.InventoryLog{}).
		Preload("Product").
		Order("created_at desc, id desc")

	if warehouseType != "" {
		query = query.Where("warehouse_type = ?", warehouseType)
	}
	if logType != "" {
		query = query.Where("log_type = ?", logType)
	}
	if productID != "" {
		query = query.Where("product_id = ?", productID)
	}
	if operatorID != "" {
		query = query.Where("operator_id = ?", operatorID)
	}
	if keyword != "" {
		k := "%" + keyword + "%"
		// 需要联表查询商品名称和备注
		query = query.Joins("JOIN products ON inventory_logs.product_id = products.id").
			Where("(products.name LIKE ? OR products.sku LIKE ? OR inventory_logs.remark LIKE ? OR inventory_logs.related_no LIKE ?)", k, k, k, k)
	}
	query = applyInventoryLogScopeFilters(query, c)

	var total int64
	query.Model(&models.InventoryLog{}).Count(&total)

	offset := (page - 1) * pageSize

	var logs []models.InventoryLog
	if err := query.Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  200,
		"total": total,
		"data":  logs,
	})
}

// ExportInventoryLogs 导出出入库日志
// @Summary 导出库存流水
// @Description 导出库存流水
// @Tags Inventory
// @Produce application/octet-stream
// @Param warehouse_type query int false "仓库类型"
// @Param log_type query string false "流水类型"
// @Param product_id query int false "商品ID"
// @Param operator_id query int false "操作员工ID"
// @Param keyword query string false "搜索关键字"
// @Success 200 {file} binary "Excel 文件"
// @Security ApiKeyAuth
// @ID get_api_inventory_logs_export
// @Router /api/inventory/logs/export [get]
func ExportInventoryLogs(c *gin.Context) {
	if !requireNoActiveBlindStocktake(c) {
		return
	}
	warehouseType := c.Query("warehouse_type")
	logType := c.Query("log_type")
	productID := c.Query("product_id")
	keyword := c.Query("keyword")
	operatorID := c.Query("operator_id")

	query := config.DB.Model(&models.InventoryLog{}).Preload("Product").Order("created_at desc, id desc")

	if warehouseType != "" {
		query = query.Where("warehouse_type = ?", warehouseType)
	}
	if logType != "" {
		query = query.Where("log_type = ?", logType)
	}
	if productID != "" {
		query = query.Where("product_id = ?", productID)
	}
	if operatorID != "" {
		query = query.Where("operator_id = ?", operatorID)
	}
	if keyword != "" {
		k := "%" + keyword + "%"
		query = query.Joins("JOIN products ON inventory_logs.product_id = products.id").
			Where("(products.name LIKE ? OR products.sku LIKE ? OR inventory_logs.remark LIKE ? OR inventory_logs.related_no LIKE ?)", k, k, k, k)
	}
	query = applyInventoryLogScopeFilters(query, c)

	var logs []models.InventoryLog
	if err := query.Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询日志失败"})
		return
	}

	headers := []string{"日志ID", "商品名称", "操作人", "仓库类型", "记录类型", "变动数量", "变动前", "变动后", "单价", "总价", "关联单号", "备注", "记录时间"}
	var data [][]interface{}
	for _, l := range logs {
		prodName := "-"
		if l.Product != nil {
			prodName = l.Product.Name
		}
		operatorName := l.OperatorName
		if operatorName == "" {
			operatorName = "历史记录"
		}
		wh := "主仓"
		if l.WarehouseType == 2 {
			wh = "门店"
		} else if l.WarehouseType == 3 {
			wh = "云仓"
		} else if l.WarehouseType == 4 {
			wh = "样机"
		}
		lt := "系统调整"
		if l.LogType == "purchase" {
			lt = "采购入库"
		} else if l.LogType == "sale" {
			lt = "销售出库"
		} else if l.LogType == "transfer" {
			lt = "库存调拨"
		} else if l.LogType == "stocktake" {
			lt = "盘点调整"
		} else if l.LogType == "cancel" {
			lt = "取消退库"
		}

		row := []interface{}{
			l.ID,
			prodName,
			operatorName,
			wh,
			lt,
			l.ChangeQty,
			l.BeforeQty,
			l.AfterQty,
			l.UnitCost,
			l.UnitCost * float64(l.ChangeQty),
			l.RelatedNo,
			l.Remark,
			l.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		data = append(data, row)
	}

	utils.ExportToExcel(c, "出入库日志", headers, data)
}
