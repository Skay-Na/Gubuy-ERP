package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
)

type CategoryRequest struct {
	Name      string `json:"name" binding:"required"`
	ParentID  *uint  `json:"parent_id"`
	SortOrder int    `json:"sort_order"`
}

// validateParent 校验父分类：必须存在且本身是一级分类（仅支持两级）
func validateParent(parentID *uint, selfID uint) *string {
	if parentID == nil || *parentID == 0 {
		return nil
	}
	if selfID != 0 && *parentID == selfID {
		msg := "父分类不能是自身"
		return &msg
	}
	var parent models.Category
	if err := config.DB.First(&parent, *parentID).Error; err != nil {
		msg := "父分类不存在"
		return &msg
	}
	if parent.ParentID != nil && *parent.ParentID != 0 {
		msg := "仅支持两级分类，二级分类下不能再建子分类"
		return &msg
	}
	return nil
}

// categoryWithSales 带历史销量的分类响应结构（with_sales=1 时使用）
type categoryWithSales struct {
	models.Category
	SalesQty int64 `json:"sales_qty"`
}

// categorySalesQtyMap 统计各分类商品的历史成交数量合计。
// 口径与财务销售分析保持一致：order_items JOIN orders，排除已取消订单（order_status = 1 为正常单）。
func categorySalesQtyMap() map[uint]int64 {
	var rows []struct {
		CategoryID uint
		Qty        int64
	}
	config.DB.Table("order_items oi JOIN orders o ON oi.order_id = o.id").
		Joins("JOIN products p ON p.id = oi.product_id").
		Where("o.order_status = 1").
		Select("p.category_id AS category_id, COALESCE(SUM(oi.quantity), 0) AS qty").
		Group("p.category_id").
		Scan(&rows)
	result := make(map[uint]int64, len(rows))
	for _, row := range rows {
		result[row.CategoryID] = row.Qty
	}
	return result
}

// GetCategories 获取分类列表（平铺返回，子分类紧跟其父分类，前端据此组树）
// @Summary GetCategories
// @Description GetCategories
// @Tags Categories
// @Accept json
// @Produce json
// @Param with_sales query string false "传1时为每个分类附带销量 sales_qty（本身+二级子分类商品成交数量合计，排除已取消订单），并按销量降序排列"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_categories
// @Router /api/categories [get]
func GetCategories(c *gin.Context) {
	withSales := c.Query("with_sales") == "1"

	var categories []models.Category
	if err := config.DB.Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}

	// 一级分类分组；二级分类紧跟父分类。
	// （SQL 单条 ORDER BY 无法按"父分类的 sort_order"排一级，改在应用层组序）
	lessCategory := func(a, b models.Category) bool {
		if a.SortOrder != b.SortOrder {
			return a.SortOrder > b.SortOrder
		}
		return a.ID < b.ID
	}

	// 不带 with_sales：保持原有行为（一级按 sort_order desc、id asc）
	if !withSales {
		byParent := make(map[uint][]models.Category)
		var roots []models.Category
		for _, cat := range categories {
			if cat.ParentID == nil || *cat.ParentID == 0 {
				roots = append(roots, cat)
			} else {
				p := *cat.ParentID
				byParent[p] = append(byParent[p], cat)
			}
		}
		sort.Slice(roots, func(i, j int) bool { return lessCategory(roots[i], roots[j]) })

		ordered := make([]models.Category, 0, len(categories))
		for _, root := range roots {
			ordered = append(ordered, root)
			kids := byParent[root.ID]
			sort.Slice(kids, func(i, j int) bool { return lessCategory(kids[i], kids[j]) })
			ordered = append(ordered, kids...)
		}

		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": ordered})
		return
	}

	// with_sales=1：一级销量 = 分类本身 + 其二级子分类商品的销量合计
	sales := categorySalesQtyMap()
	childQty := make(map[uint]int64)
	for _, cat := range categories {
		if cat.ParentID != nil && *cat.ParentID != 0 {
			childQty[*cat.ParentID] += sales[cat.ID]
		}
	}

	items := make([]categoryWithSales, 0, len(categories))
	for _, cat := range categories {
		qty := sales[cat.ID]
		if cat.ParentID == nil || *cat.ParentID == 0 {
			qty += childQty[cat.ID]
		}
		items = append(items, categoryWithSales{Category: cat, SalesQty: qty})
	}

	lessWithSales := func(a, b categoryWithSales) bool {
		if a.SalesQty != b.SalesQty {
			return a.SalesQty > b.SalesQty
		}
		if a.SortOrder != b.SortOrder {
			return a.SortOrder > b.SortOrder
		}
		return a.ID < b.ID
	}

	byParent := make(map[uint][]categoryWithSales)
	var roots []categoryWithSales
	for _, item := range items {
		if item.ParentID == nil || *item.ParentID == 0 {
			roots = append(roots, item)
		} else {
			p := *item.ParentID
			byParent[p] = append(byParent[p], item)
		}
	}
	sort.Slice(roots, func(i, j int) bool { return lessWithSales(roots[i], roots[j]) })

	ordered := make([]categoryWithSales, 0, len(categories))
	for _, root := range roots {
		ordered = append(ordered, root)
		kids := byParent[root.ID]
		sort.Slice(kids, func(i, j int) bool { return lessWithSales(kids[i], kids[j]) })
		ordered = append(ordered, kids...)
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": ordered})
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

	if msg := validateParent(input.ParentID, 0); msg != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": *msg})
		return
	}

	category := models.Category{
		Name:      input.Name,
		ParentID:  input.ParentID,
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

	var category models.Category
	if err := config.DB.First(&category, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "分类不存在"})
		return
	}

	if msg := validateParent(input.ParentID, category.ID); msg != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": *msg})
		return
	}

	updates := map[string]interface{}{
		"name":       input.Name,
		"sort_order": input.SortOrder,
	}
	if input.ParentID == nil || *input.ParentID == 0 {
		updates["parent_id"] = nil
	} else {
		updates["parent_id"] = *input.ParentID
	}

	if err := config.DB.Model(&models.Category{}).Where("id = ?", id).Updates(updates).Error; err != nil {
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

	// 检查是否有子分类
	var childCount int64
	config.DB.Model(&models.Category{}).Where("parent_id = ?", id).Count(&childCount)
	if childCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "该分类下仍有子分类，请先删除或移走子分类"})
		return
	}

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
