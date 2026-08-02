package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// MonthlyStatsResponse 月度统计响应
type MonthlyStatsResponse struct {
	Month       string  `json:"month"`
	Revenue     float64 `json:"revenue"`
	GrossProfit float64 `json:"gross_profit"`
	OrderCount  int64   `json:"order_count"`
	Expense     float64 `json:"expense"`
}

// GetMonthlyStats 获取指定月份聚合报表
// @Summary 获取月度统计报表
// @Description 获取月度统计报表
// @Tags Stats
// @Accept json
// @Produce json
// @Param month query string true "月份 (如 2026-06)"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_stats_monthly
// @Router /api/stats/monthly [get]
func GetMonthlyStats(c *gin.Context) {
	monthStr := c.Query("month")
	if monthStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "month parameter is required, e.g. 2026-06"})
		return
	}

	location := time.Now().Location()
	t, err := time.ParseInLocation("2006-01", monthStr, location)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid month format, use YYYY-MM"})
		return
	}

	startTime := t
	endTime := startTime.AddDate(0, 1, 0).Add(-time.Nanosecond)

	var stats MonthlyStatsResponse
	stats.Month = monthStr

	// 1. Revenue & OrderCount
	config.DB.Model(&models.Order{}).
		Where("order_status = 1 AND created_at >= ? AND created_at <= ?", startTime, endTime).
		Select("COALESCE(SUM(total_amount), 0) as revenue, COUNT(id) as order_count").
		Row().Scan(&stats.Revenue, &stats.OrderCount)

	// Gross Profit = Revenue - TotalItemCost - TotalOrderFees
	var totalItemCost float64
	config.DB.Table("order_items").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("orders.order_status = 1 AND orders.created_at >= ? AND orders.created_at <= ?", startTime, endTime).
		Select("COALESCE(SUM(order_items.quantity * order_items.unit_cost), 0)").
		Row().Scan(&totalItemCost)

	var totalFees float64
	config.DB.Model(&models.Order{}).
		Where("order_status = 1 AND created_at >= ? AND created_at <= ?", startTime, endTime).
		Select("COALESCE(SUM(delivery_fee + installation_fee + referral_fee + commission_amount), 0)").
		Row().Scan(&totalFees)

	stats.GrossProfit = stats.Revenue - totalItemCost - totalFees

	// 2. Expense
	config.DB.Model(&models.FinancialLog{}).
		Where("flow_type = 2 AND created_at >= ? AND created_at <= ?", startTime, endTime).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&stats.Expense)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "success",
		"data": stats,
	})
}
