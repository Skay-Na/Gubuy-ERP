package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

type CategoryRequest struct {
	Name      string `json:"name" binding:"required"`
	SortOrder int    `json:"sort_order"`
}

// GetCategories 获取分类列表
// @Summary GetCategories
// @Description GetCategories
// @Tags Categories
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_categories
// @Router /api/categories [get]
func GetCategories(c *gin.Context) {
	var categories []models.Category
	if err := config.DB.Order("sort_order desc, id asc").Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": categories})
}

// CreateCategory 创建分类
// @Summary CreateCategory
// @Description CreateCategory
// @Tags Categories
// @Accept json
// @Produce json
// @Param body body CategoryRequest true "分类参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_categories
// @Router /api/categories [post]
func CreateCategory(c *gin.Context) {
	var input CategoryRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	category := models.Category{
		Name:      input.Name,
		SortOrder: input.SortOrder,
	}

	if err := config.DB.Create(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": category})
}

// UpdateCategory 更新分类
// @Summary UpdateCategory
// @Description UpdateCategory
// @Tags Categories
// @Accept json
// @Produce json
// @Param body body CategoryRequest true "分类参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_categories_id
// @Router /api/categories/{id} [put]
func UpdateCategory(c *gin.Context) {
	id := c.Param("id")
	var input CategoryRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	if err := config.DB.Model(&models.Category{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name":       input.Name,
		"sort_order": input.SortOrder,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success"})
}

// DeleteCategory 删除分类
// @Summary DeleteCategory
// @Description DeleteCategory
// @Tags Categories
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID delete_api_categories_id
// @Router /api/categories/{id} [delete]
func DeleteCategory(c *gin.Context) {
	id := c.Param("id")

	// 检查是否有商品属于该分类
	var count int64
	config.DB.Model(&models.Product{}).Where("category_id = ?", id).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "该分类下仍有商品，无法删除"})
		return
	}

	if err := config.DB.Delete(&models.Category{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "删除失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success"})
}
