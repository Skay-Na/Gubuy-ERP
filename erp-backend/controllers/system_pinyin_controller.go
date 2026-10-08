package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"erp-backend/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

// InitPinyinData 为历史数据刷入拼音索引
// @Summary 初始化拼音索引
// @Tags System
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
func InitPinyinData(c *gin.Context) {
	// 1. 处理员工
	var employees []models.Employee
	config.DB.Find(&employees)
	for _, emp := range employees {
		if emp.Pinyin == "" {
			pStr, pInit := utils.GetPinyin(emp.Name)
			config.DB.Model(&emp).Updates(map[string]interface{}{
				"pinyin":         pStr,
				"pinyin_initial": pInit,
			})
		}
	}

	// 2. 处理订单
	var orders []models.Order
	config.DB.Find(&orders)
	for _, order := range orders {
		if order.CustomerPinyin == "" {
			pStr, pInit := utils.GetPinyin(order.CustomerName)
			config.DB.Model(&order).Updates(map[string]interface{}{
				"customer_pinyin":         pStr,
				"customer_pinyin_initial": pInit,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "拼音索引初始化完成",
	})
}
