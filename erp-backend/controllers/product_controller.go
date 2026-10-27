package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"erp-backend/utils"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CreateProductRequest struct {
	Name              string  `json:"name" binding:"required"`
	Remark            string  `json:"remark"`
	CategoryID        uint    `json:"category_id"`
	MarginRate        float64 `json:"margin_rate"`
	NoCommission      bool    `json:"no_commission"`
	SpecialPrice      float64 `json:"special_price"`
	SpecialPriceStart *string `json:"special_price_start"`
	SpecialPriceEnd   *string `json:"special_price_end"`
}

type UpdateProductRequest struct {
	Name              string   `json:"name" binding:"required"`
	Remark            string   `json:"remark"`
	CategoryID        *uint    `json:"category_id"`
	MarginRate        float64  `json:"margin_rate"`
	NoCommission      bool     `json:"no_commission"`
	SpecialPrice      float64  `json:"special_price"`
	SpecialPriceStart *string  `json:"special_price_start"`
	SpecialPriceEnd   *string  `json:"special_price_end"`
	AvgCost           *float64 `json:"avg_cost"`
}

// GetProducts 获取商品列表
// @Summary 获取商品列表
// @Description 获取商品列表
// @Tags Product
// @Accept json
// @Produce json
// @Param keyword query string false "搜索关键字 (名称或SKU)"
// @Param category_id query int false "分类ID"
// @Param stock_filter query string false "库存筛选：store/cloud/available/none"
// @Param only_special query string false "是否仅拉取特价商品(1:是)"
// @Param order_by query string false "排序：sales_qty 按历史销量降序（排除已取消订单），同时每个商品附带 sales_qty 字段"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_products
// @Router /api/products [get]
func GetProducts(c *gin.Context) {
	blindStocktakeMode, ok := employeeBlindStocktakeMode(c)
	if !ok {
		return
	}
	keyword := c.Query("keyword")
	categoryID := c.Query("category_id")
	stockFilter := c.Query("stock_filter")
	orderBy := c.Query("order_by")
	orderBySales := orderBy == "sales_qty"
	if blindStocktakeMode {
		// Inventory-based filters would reveal whether a product has stock even
		// when the quantities themselves are removed from the response.
		stockFilter = ""
	}
	onlySpecial := c.Query("only_special")
	pageStr := c.Query("page")
	pageSizeStr := c.Query("page_size")

	var products []models.Product
	db := config.DB.Preload("Category")

	if onlySpecial == "1" {
		now := time.Now()
		db = db.Where("special_price > 0 AND special_price_start <= ? AND special_price_end >= ?", now, now)
	}

	if keyword != "" {
		// 商品备注存在 sku 列（模型 Remark->column:sku），name/sku 已覆盖名称+备注搜索
		db = db.Where("(name LIKE ? OR sku LIKE ?)", "%"+keyword+"%", "%"+keyword+"%")
	}

	if categoryID != "" {
		// 选中一级大分类时，包含其下所有二级分类的商品
		db = db.Where("category_id = ? OR category_id IN (SELECT id FROM categories WHERE parent_id = ? AND deleted_at IS NULL)", categoryID, categoryID)
	}
	switch stockFilter {
	case "store":
		db = db.Where("store_stock > 0")
	case "cloud":
		db = db.Where("cloud_stock > 0")
	case "available":
		db = db.Where("(store_stock > 0 OR sample_stock > 0 OR cloud_stock > 0)")
	case "none":
		db = db.Where("store_stock <= 0 AND cloud_stock <= 0")
	}

	// 如果传了 page 参数，则执行真分页
	if pageStr != "" {
		page, _ := strconv.Atoi(pageStr)
		if page <= 0 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(pageSizeStr)
		if pageSize <= 0 {
			pageSize = 15
		}

		var total int64
		if err := db.Model(&models.Product{}).Count(&total).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
			return
		}

		offset := (page - 1) * pageSize
		// order_by=sales_qty：销量聚合做成 JOIN 计算列（JOIN 在 Count 之后才加，不影响 total）
		if orderBySales {
			var rows []productWithSales
			if err := productSalesQuery(db).Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"code":                 200,
				"msg":                  "success",
				"data":                 productsWithSalesForEmployeeStocktake(rows, blindStocktakeMode),
				"total":                total,
				"blind_stocktake_mode": blindStocktakeMode,
			})
			return
		}
		if err := db.Offset(offset).Limit(pageSize).Find(&products).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":                 200,
			"msg":                  "success",
			"data":                 productsForEmployeeStocktake(products, blindStocktakeMode),
			"total":                total,
			"blind_stocktake_mode": blindStocktakeMode,
		})
		return
	}

	// 否则全量拉取，保证向后兼容性
	if orderBySales {
		var rows []productWithSales
		if err := productSalesQuery(db).Find(&rows).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":                 200,
			"msg":                  "success",
			"data":                 productsWithSalesForEmployeeStocktake(rows, blindStocktakeMode),
			"blind_stocktake_mode": blindStocktakeMode,
		})
		return
	}

	if err := db.Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":                 200,
		"msg":                  "success",
		"data":                 productsForEmployeeStocktake(products, blindStocktakeMode),
		"blind_stocktake_mode": blindStocktakeMode,
	})
}

// productWithSales 带历史销量的商品行（order_by=sales_qty 时返回，销量为 JSON 平铺字段）
type productWithSales struct {
	models.Product
	SalesQty int64 `json:"sales_qty"`
}

// productSalesJoin 销量聚合子查询：历史成交数量合计，口径同财务销售分析（排除已取消订单 order_status=1）
const productSalesJoin = "LEFT JOIN (SELECT oi.product_id AS pid, SUM(oi.quantity) AS qty " +
	"FROM order_items oi JOIN orders o ON o.id = oi.order_id " +
	"WHERE o.order_status = 1 GROUP BY oi.product_id) sales_agg ON sales_agg.pid = products.id"

const productSalesSelect = "products.*, COALESCE(sales_agg.qty, 0) AS sales_qty"

// productSalesQuery 构建带销量计算列的商品查询（order_by=sales_qty 时使用，分页/全量两分支共用）。
// 必须显式指定 Model：全量拉取分支的 db 从未经过 Count，若不设 Model，
// GORM 会按目标结构体 productWithSales 推断表名（product_with_sales，不存在）导致 500。
func productSalesQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&models.Product{}).
		Joins(productSalesJoin).
		Select(productSalesSelect).
		Order("sales_qty DESC, products.id ASC")
}

// productsWithSalesForEmployeeStocktake 盲盘模式下隐藏库存字段（销量字段一并隐藏，保持盲盘口径）
func productsWithSalesForEmployeeStocktake(rows []productWithSales, blindStocktakeMode bool) interface{} {
	if !blindStocktakeMode {
		return rows
	}
	redacted := make([]productWithoutStock, len(rows))
	for i, row := range rows {
		redacted[i] = productWithoutStock{Product: row.Product}
	}
	return redacted
}

type productWithoutStock struct {
	models.Product
	MainStock   *int `json:"main_stock,omitempty"`
	StoreStock  *int `json:"store_stock,omitempty"`
	SampleStock *int `json:"sample_stock,omitempty"`
	CloudStock  *int `json:"cloud_stock,omitempty"`
}

func productsForEmployeeStocktake(products []models.Product, blindStocktakeMode bool) interface{} {
	if !blindStocktakeMode {
		return products
	}
	redacted := make([]productWithoutStock, len(products))
	for i, product := range products {
		redacted[i] = productWithoutStock{Product: product}
	}
	return redacted
}

// CreateProduct 创建商品
// @Summary 创建商品
// @Description 创建商品
// @Tags Product
// @Accept json
// @Produce json
// @Param body body CreateProductRequest true "商品信息"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_products
// @Router /api/products [post]
func CreateProduct(c *gin.Context) {
	var input CreateProductRequest

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	// 查重：商品名称
	var count int64
	if err := config.DB.Model(&models.Product{}).Where("name = ?", input.Name).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "商品名称已存在"})
		return
	}

	product := models.Product{
		Name:         input.Name,
		CategoryID:   input.CategoryID,
		MarginRate:   input.MarginRate,
		SupportCloud: true,
		NoCommission: input.NoCommission,
		SpecialPrice: input.SpecialPrice,
	}

	if input.SpecialPriceStart != nil && *input.SpecialPriceStart != "" {
		t, err := parseTime(*input.SpecialPriceStart)
		if err == nil {
			product.SpecialPriceStart = &t
		}
	}
	if input.SpecialPriceEnd != nil && *input.SpecialPriceEnd != "" {
		t, err := parseTime(*input.SpecialPriceEnd)
		if err == nil {
			product.SpecialPriceEnd = &t
		}
	}

	if input.Remark != "" {
		product.Remark = &input.Remark
	}

	if err := config.DB.Create(&product).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": product})
}

// UpdateProduct 更新商品信息
// @Summary 更新商品
// @Description 更新商品
// @Tags Products
// @Accept json
// @Produce json
// @Param body body UpdateProductRequest true "商品更新参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_products_id
// @Router /api/products/{id} [put]
func UpdateProduct(c *gin.Context) {
	id := c.Param("id")
	var input UpdateProductRequest

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if input.AvgCost != nil && *input.AvgCost < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "平均成本不能小于 0"})
		return
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": tx.Error.Error()})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var product models.Product
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品不存在"})
		return
	}

	// 查重：商品名称（排除自身）
	var count int64
	if err := tx.Model(&models.Product{}).Where("name = ? AND id != ?", input.Name, id).Count(&count).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if count > 0 {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "商品名称已存在"})
		return
	}

	var remarkPtr *string
	if input.Remark != "" {
		remarkPtr = &input.Remark
	}

	updates := map[string]interface{}{
		"name":          input.Name,
		"sku":           remarkPtr,
		"margin_rate":   input.MarginRate,
		"support_cloud": true,
		"no_commission": input.NoCommission,
		"special_price": input.SpecialPrice,
	}
	costChanged := false
	if input.AvgCost != nil {
		updates["avg_cost"] = *input.AvgCost
		costChanged = int64(math.Round(product.AvgCost*100)) != int64(math.Round(*input.AvgCost*100))
	}
	if input.CategoryID != nil {
		updates["category_id"] = *input.CategoryID
	}

	if input.SpecialPriceStart != nil && *input.SpecialPriceStart != "" {
		if t, err := parseTime(*input.SpecialPriceStart); err == nil {
			updates["special_price_start"] = t
		}
	} else {
		updates["special_price_start"] = nil
	}

	if input.SpecialPriceEnd != nil && *input.SpecialPriceEnd != "" {
		if t, err := parseTime(*input.SpecialPriceEnd); err == nil {
			updates["special_price_end"] = t
		}
	} else {
		updates["special_price_end"] = nil
	}

	if err := tx.Model(&product).Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if costChanged {
		if err := tx.Create(&models.HistoricalCostRepair{
			ProductID:        product.ID,
			UnitCost:         *input.AvgCost,
			PreviousUnitCost: product.AvgCost,
			StockQty:         productTotalStock(product),
			CostBaseline:     true,
			Remark:           "商品档案手动校准平均成本",
		}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存商品成本校准记录失败"})
			return
		}
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存商品失败：" + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success"})
}

// RecalculateAllAvgCosts 根据历史流水重新核算所有商品的平均成本
// @Summary 重新核算所有平均成本
// @Description 重新核算所有平均成本
// @Tags Products
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_products_recalculate_costs
// @Router /api/products/recalculate-costs [post]
func RecalculateAllAvgCosts(c *gin.Context) {
	var productIDs []uint
	if err := config.DB.Model(&models.Product{}).Order("id asc").Pluck("id", &productIDs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取商品失败"})
		return
	}

	successCount := 0
	failed := make([]gin.H, 0)
	for _, productID := range productIDs {
		tx := config.DB.Begin()
		if tx.Error != nil {
			failed = append(failed, gin.H{"product_id": productID, "error": tx.Error.Error()})
			continue
		}
		var product models.Product
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, productID).Error
		if err == nil {
			err = RecalculateSingleProductCosts(tx, productID)
		}
		if err != nil {
			tx.Rollback()
			failed = append(failed, gin.H{"product_id": productID, "error": err.Error()})
			continue
		}
		if err := tx.Commit().Error; err != nil {
			failed = append(failed, gin.H{"product_id": productID, "error": err.Error()})
			continue
		}
		successCount++
	}

	if len(failed) > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"code": 409,
			"msg":  fmt.Sprintf("已核算 %d 个商品，%d 个商品因成本流水不完整或库存不一致而跳过", successCount, len(failed)),
			"data": gin.H{"recalculated_count": successCount, "failed": failed},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  fmt.Sprintf("核算完成，共重新核算 %d 个商品", successCount),
		"data": gin.H{"recalculated_count": successCount, "failed": failed},
	})
}

// 辅助函数：解析多种时间格式
func parseTime(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	// 尝试带本地时区的解析
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("could not parse time: %s", s)
}

// ExportProducts 导出商品
// @Summary 导出商品数据
// @Description 导出商品数据
// @Tags Products
// @Produce application/octet-stream
// @Param keyword query string false "搜索关键字"
// @Param category_id query int false "分类ID"
// @Param all query string false "传1返回全部商品"
// @Param only_special query string false "传1仅返回特价商品"
// @Success 200 {file} binary "Excel 文件"
// @Security ApiKeyAuth
// @ID get_api_products_export
// @Router /api/products/export [get]
func ExportProducts(c *gin.Context) {
	if !requireNoActiveBlindStocktake(c) {
		return
	}
	keyword := c.Query("keyword")
	categoryID := c.Query("category_id")
	onlySpecial := c.Query("only_special")

	var products []models.Product
	db := config.DB.Preload("Category")

	if onlySpecial == "1" {
		now := time.Now()
		db = db.Where("special_price > 0 AND special_price_start <= ? AND special_price_end >= ?", now, now)
	}

	if keyword != "" {
		db = db.Where("name LIKE ? OR sku LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	if categoryID != "" {
		// 选中一级大分类时，包含其下所有二级分类的商品
		db = db.Where("category_id = ? OR category_id IN (SELECT id FROM categories WHERE parent_id = ? AND deleted_at IS NULL)", categoryID, categoryID)
	}

	if err := db.Order("id desc").Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询商品失败"})
		return
	}

	headers := []string{"商品ID", "商品名称", "分类", "备注(SKU)", "最新进价", "平均成本", "门店库存", "样机数量", "云仓库存", "提成规则", "特价金额"}
	var data [][]interface{}
	for _, p := range products {
		catName := "-"
		if p.Category != nil {
			catName = p.Category.Name
		}
		sku := ""
		if p.Remark != nil {
			sku = *p.Remark
		}
		commissionRule := "正常提成"
		if p.NoCommission {
			commissionRule = "无提成"
		}

		row := []interface{}{
			p.ID,
			p.Name,
			catName,
			sku,
			p.LatestCost,
			p.AvgCost,
			p.StoreStock,
			p.SampleStock,
			p.CloudStock,
			commissionRule,
			p.SpecialPrice,
		}
		data = append(data, row)
	}

	utils.ExportToExcel(c, "商品清单", headers, data)
}
