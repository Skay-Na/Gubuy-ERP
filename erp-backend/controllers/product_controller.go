package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// GetProducts 获取商品列表
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
func CreateProduct(c *gin.Context) {
	var input struct {
		Name         string  `json:"name" binding:"required"`
		Remark       string  `json:"remark"`
		CategoryID   uint    `json:"category_id"`
		MarginRate   float64 `json:"margin_rate"`
		SupportCloud      bool       `json:"support_cloud"`
		SpecialPrice      float64    `json:"special_price"`
		SpecialPriceStart *string `json:"special_price_start"`
		SpecialPriceEnd   *string `json:"special_price_end"`
	}

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
func UpdateProduct(c *gin.Context) {
	id := c.Param("id")
	var input struct {
		Name         string  `json:"name" binding:"required"`
		Remark       string  `json:"remark"`
		CategoryID   uint    `json:"category_id"`
		MarginRate   float64 `json:"margin_rate"`
		SupportCloud      bool       `json:"support_cloud"`
		SpecialPrice      float64    `json:"special_price"`
		SpecialPriceStart *string `json:"special_price_start"`
		SpecialPriceEnd   *string `json:"special_price_end"`
		AvgCost           float64 `json:"avg_cost"`
	}

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
		"category_id":   input.CategoryID,
		"margin_rate":   input.MarginRate,
		"support_cloud": input.SupportCloud,
		"special_price": input.SpecialPrice,
		"avg_cost":      input.AvgCost,
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
