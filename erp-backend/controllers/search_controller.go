package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RelatedSuggestion 搜索建议返回结构
type RelatedSuggestion struct {
	Value string `json:"value"` // 填入 related_no 的实际值
	Label string `json:"label"` // 下拉展示的文本
	Type  string `json:"type"`  // employee 或 order
}

// SearchRelated 搜索关联的人员或单号
// @Summary 搜索关联人员或单号
// @Description 根据拼音、首字母、汉字搜索员工，或根据单号、客户姓名搜索订单
// @Tags Search
// @Accept json
// @Produce json
// @Param q query string true "搜索关键词"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_search_related
// @Router /api/search/related [get]
func SearchRelated(c *gin.Context) {
	q := c.Query("q")
	var suggestions []RelatedSuggestion
	likeQ := "%" + q + "%"

	// 1. 搜员工 (汉字、拼音、首字母)
	var employees []models.Employee
	config.DB.Where("name LIKE ? OR pinyin LIKE ? OR pinyin_initial LIKE ?", likeQ, likeQ, likeQ).Limit(10).Find(&employees)
	for _, emp := range employees {
		suggestions = append(suggestions, RelatedSuggestion{
			Value: emp.Name, // 对于人员，我们把名字作为关联单号记录下来，也可以加上工号
			Label: "员工：" + emp.Name + " (" + emp.Phone + ")",
			Type:  "employee",
		})
	}

	// 2. 搜订单 (单号、客户名、客户名拼音、首字母)
	var orders []models.Order
	config.DB.Where("order_no LIKE ? OR customer_name LIKE ? OR customer_pinyin LIKE ? OR customer_pinyin_initial LIKE ?", likeQ, likeQ, likeQ, likeQ).Limit(10).Find(&orders)
	for _, o := range orders {
		suggestions = append(suggestions, RelatedSuggestion{
			Value: o.OrderNo,
			Label: "订单：" + o.OrderNo + " - " + o.CustomerName,
			Type:  "order",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "搜索成功",
		"data": suggestions,
	})
}
