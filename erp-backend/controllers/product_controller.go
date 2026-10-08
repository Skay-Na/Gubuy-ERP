package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"erp-backend/utils"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type CreateProductRequest struct {
	Name              string  `json:"name" binding:"required"`
	Remark            string  `json:"remark"`
	CategoryID        uint    `json:"category_id"`
	MarginRate        float64 `json:"margin_rate"`
	SupportCloud      bool    `json:"support_cloud"`
	NoCommission      bool    `json:"no_commission"`
	SpecialPrice      float64 `json:"special_price"`
	SpecialPriceStart *string `json:"special_price_start"`
	SpecialPriceEnd   *string `json:"special_price_end"`
}

type UpdateProductRequest struct {
	Name              string  `json:"name" binding:"required"`
	Remark            string  `json:"remark"`
	CategoryID        *uint   `json:"category_id"`
	MarginRate        float64 `json:"margin_rate"`
	SupportCloud      bool    `json:"support_cloud"`
	NoCommission      bool    `json:"no_commission"`
	SpecialPrice      float64 `json:"special_price"`
	SpecialPriceStart *string `json:"special_price_start"`
	SpecialPriceEnd   *string `json:"special_price_end"`
	AvgCost           float64 `json:"avg_cost"`
}

// GetProducts 获取商品列表
// @Summary 获取商品列表
// @Description 获取商品列表
// @Tags Product
// @Accept json
// @Produce json
// @Param keyword query string false "搜索关键字 (名称或SKU)"
// @Param category_id query int false "分类ID"
// @Param all query string false "是否拉取全部(包含无库存)"
// @Param only_special query string false "是否仅拉取特价商品(1:是)"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_products
// @Router /api/products [get]
func GetProducts(c *gin.Context) {
	keyword := c.Query("keyword")
	categoryID := c.Query("category_id")
	all := c.Query("all")
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
		db = db.Where("name LIKE ? OR sku LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	if categoryID != "" {
		db = db.Where("category_id = ?", categoryID)
	}

	if all != "1" && keyword == "" && categoryID == "" {
		db = db.Where("main_stock > 0 OR store_stock > 0 OR cloud_stock > 0 OR support_cloud = ?", true)
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
		if err := db.Offset(offset).Limit(pageSize).Find(&products).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":  200,
			"msg":   "success",
			"data":  products,
			"total": total,
		})
		return
	}

	// 否则全量拉取，保证向后兼容性
	if err := db.Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": products})
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
		SupportCloud: input.SupportCloud,
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

	// 查重：商品名称（排除自身）
	var count int64
	if err := config.DB.Model(&models.Product{}).Where("name = ? AND id != ?", input.Name, id).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if count > 0 {
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
		"support_cloud": input.SupportCloud,
		"no_commission": input.NoCommission,
		"special_price": input.SpecialPrice,
		"avg_cost":      input.AvgCost,
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

	if err := config.DB.Model(&models.Product{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
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
	var products []models.Product
	if err := config.DB.Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取商品失败"})
		return
	}

	successCount := 0
	for _, p := range products {
		var logs []models.InventoryLog
		// 仅查询采购入库(purchase)且录入了单价记录的流水
		err := config.DB.Where("product_id = ? AND log_type = ? AND unit_cost > 0", p.ID, "purchase").Find(&logs).Error
		if err != nil || len(logs) == 0 {
			// 如果没有流水记录，则尝试将 AvgCost 同步为 LatestCost
			if p.LatestCost > 0 {
				config.DB.Model(&p).Update("avg_cost", p.LatestCost)
			}
			continue
		}

		var totalQty int
		var totalAmount float64
		for _, log := range logs {
			totalQty += log.ChangeQty
			totalAmount += float64(log.ChangeQty) * log.UnitCost
		}

		if totalQty > 0 {
			avgCost := totalAmount / float64(totalQty)
			if err := config.DB.Model(&p).Update("avg_cost", avgCost).Error; err == nil {
				successCount++
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  fmt.Sprintf("重构完成，共重新核算 %d 个商品", successCount),
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
	keyword := c.Query("keyword")
	categoryID := c.Query("category_id")
	all := c.Query("all")
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
		db = db.Where("category_id = ?", categoryID)
	}

	if all != "1" && keyword == "" && categoryID == "" {
		db = db.Where("main_stock > 0 OR store_stock > 0 OR cloud_stock > 0 OR support_cloud = ?", true)
	}

	if err := db.Order("id desc").Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询商品失败"})
		return
	}

	headers := []string{"商品ID", "商品名称", "分类", "备注(SKU)", "最新进价", "平均成本", "主仓库存", "门店库存", "样机数量", "云仓库存", "支持云仓代发", "提成规则", "特价金额"}
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
		supportCloud := "否"
		if p.SupportCloud {
			supportCloud = "是"
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
			p.MainStock,
			p.StoreStock,
			p.SampleStock,
			p.CloudStock,
			supportCloud,
			commissionRule,
			p.SpecialPrice,
		}
		data = append(data, row)
	}

	utils.ExportToExcel(c, "商品清单", headers, data)
}
