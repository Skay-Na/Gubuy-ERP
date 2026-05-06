package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetProducts 获取商品列表
func GetProducts(c *gin.Context) {
	keyword := c.Query("keyword")
	categoryID := c.Query("category_id")
	all := c.Query("all")
	var products []models.Product
	db := config.DB.Preload("Category")

	if keyword != "" {
		db = db.Where("name LIKE ? OR sku LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	if categoryID != "" {
		db = db.Where("category_id = ?", categoryID)
	}

	if all != "1" && keyword == "" && categoryID == "" {
		db = db.Where("main_stock > 0 OR store_stock > 0 OR cloud_stock > 0 OR support_cloud = ?", true)
	}

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
		SKU          string  `json:"sku" binding:"required"`
		CategoryID   uint    `json:"category_id"`
		MarginRate   float64 `json:"margin_rate"`
		SupportCloud bool    `json:"support_cloud"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	product := models.Product{
		Name:         input.Name,
		SKU:          input.SKU,
		CategoryID:   input.CategoryID,
		MarginRate:   input.MarginRate,
		SupportCloud: input.SupportCloud,
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
		SKU          string  `json:"sku" binding:"required"`
		CategoryID   uint    `json:"category_id"`
		MarginRate   float64 `json:"margin_rate"`
		SupportCloud bool    `json:"support_cloud"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	updates := map[string]interface{}{
		"name":          input.Name,
		"sku":           input.SKU,
		"category_id":   input.CategoryID,
		"margin_rate":   input.MarginRate,
		"support_cloud": input.SupportCloud,
	}

	if err := config.DB.Model(&models.Product{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success"})
}
