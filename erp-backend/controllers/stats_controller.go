package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// MonthlyStatsResponse 月度统计响应
type MonthlyStatsResponse struct {
	Month                  string  `json:"month"`
	Revenue                float64 `json:"revenue"`
	GrossProfit            float64 `json:"gross_profit"`
	UncostedOrderItemCount int64   `json:"uncosted_order_item_count"`
	OrderCount             int64   `json:"order_count"`
	Expense                float64 `json:"expense"`
}

// monthlyGrossProfitStats 计算指定月份区间的销货毛利（GetMonthlyStats 口径）：
// 毛利 = 销售额 - 订单商品成本快照 - 订单费用（配送/安装/渠道/提成）。
// GetMonthlyStats 与真实净利润工具共用，保证两处口径永远一致。
func monthlyGrossProfitStats(startTime, endTime time.Time) (revenue, grossProfit float64, uncostedCount int64, err error) {
	if err = config.DB.Model(&models.Order{}).
		Where("order_status = 1 AND created_at >= ? AND created_at <= ?", startTime, endTime).
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&revenue).Error; err != nil {
		return
	}

	var totalItemCost float64
	if err = config.DB.Table("order_items").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("orders.order_status = 1 AND orders.created_at >= ? AND orders.created_at <= ?", startTime, endTime).
		Select("COALESCE(SUM(order_items.quantity * order_items.unit_cost), 0)").
		Scan(&totalItemCost).Error; err != nil {
		return
	}
	if err = config.DB.Table("order_items").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("orders.order_status = 1 AND orders.created_at >= ? AND orders.created_at <= ? AND order_items.unit_cost <= 0", startTime, endTime).
		Count(&uncostedCount).Error; err != nil {
		return
	}

	var totalFees float64
	if err = config.DB.Model(&models.Order{}).
		Where("order_status = 1 AND created_at >= ? AND created_at <= ?", startTime, endTime).
		Select("COALESCE(SUM(delivery_fee + installation_fee + referral_fee + commission_amount), 0)").
		Scan(&totalFees).Error; err != nil {
		return
	}

	grossProfit = revenue - totalItemCost - totalFees
	return
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

	// Gross Profit = Revenue - TotalItemCost - TotalOrderFees（与真实净利润共用同一口径）
	_, grossProfit, uncostedCount, grossErr := monthlyGrossProfitStats(startTime, endTime)
	if grossErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取毛利数据失败"})
		return
	}
	stats.GrossProfit = grossProfit
	stats.UncostedOrderItemCount = uncostedCount

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

// RealProfitBreakdownItem 真实净利润费用分类明细
type RealProfitBreakdownItem struct {
	Category  string  `json:"category"`
	Amount    float64 `json:"amount"`
	IsPayroll bool    `json:"is_payroll"`
}

// RealProfitResponse 真实净利润响应
type RealProfitResponse struct {
	Month                 string                    `json:"month"`
	GrossProfit           float64                   `json:"gross_profit"`
	ExpenseTotal          float64                   `json:"expense_total"`
	PayrollTotal          float64                   `json:"payroll_total"`
	RealProfit            float64                   `json:"real_profit"`
	UncostedOrderItemCount int64                    `json:"uncosted_order_item_count"`
	Breakdown             []RealProfitBreakdownItem `json:"breakdown"`
}

// GetRealProfit 真实净利润：本月销货毛利 - 本月经营开销 - 本月工资。
// 经营开销与工资均取经营开销台账账户（accounts.id=7）的支出流水口径，
// 与既有经营开销账（导入历史）完全一致；工资单独列示便于核对。
// @Summary 获取真实净利润
// @Description 毛利(月报口径) - 经营开销 - 工资，附费用分类明细
// @Tags Stats
// @Produce json
// @Param month query string true "月份 (如 2026-10)"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_finance_real_profit
// @Router /api/finance/real-profit [get]
func GetRealProfit(c *gin.Context) {
	monthStr := c.Query("month")
	if monthStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "month parameter is required, e.g. 2026-10"})
		return
	}
	startTime, err := time.ParseInLocation("2006-01", monthStr, time.Now().Location())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid month format, use YYYY-MM"})
		return
	}
	endTime := startTime.AddDate(0, 1, 0).Add(-time.Nanosecond)

	// 1. 毛利（GetMonthlyStats 既有口径）
	_, grossProfit, uncostedCount, grossErr := monthlyGrossProfitStats(startTime, endTime)
	if grossErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取毛利数据失败"})
		return
	}

	// 2. 本月经营开销发生额（经营开销台账支出流水，工资部分拆出单独列示）
	var expenseTotal float64
	if err := config.DB.Model(&models.FinancialLog{}).
		Where("account_id = ? AND flow_type = ? AND created_at >= ? AND created_at <= ?", operatingExpenseAccountID, 2, startTime, endTime).
		Select("COALESCE(SUM(CASE WHEN category = '工资' THEN 0 ELSE amount END), 0)").
		Scan(&expenseTotal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取经营开销失败"})
		return
	}
	var payrollTotal float64
	if err := config.DB.Model(&models.FinancialLog{}).
		Where("account_id = ? AND flow_type = ? AND category = ? AND created_at >= ? AND created_at <= ?", operatingExpenseAccountID, 2, "工资", startTime, endTime).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&payrollTotal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取工资支出失败"})
		return
	}

	// 3. 费用分类明细（工资单独列）
	var categoryRows []struct {
		Category string
		Amount   float64
	}
	if err := config.DB.Model(&models.FinancialLog{}).
		Where("account_id = ? AND flow_type = ? AND created_at >= ? AND created_at <= ?", operatingExpenseAccountID, 2, startTime, endTime).
		Select("category, COALESCE(SUM(amount), 0) AS amount").
		Group("category").Order("amount DESC").
		Scan(&categoryRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取费用分类明细失败"})
		return
	}
	breakdown := make([]RealProfitBreakdownItem, 0, len(categoryRows))
	for _, row := range categoryRows {
		breakdown = append(breakdown, RealProfitBreakdownItem{
			Category:  row.Category,
			Amount:    math.Round(row.Amount*100) / 100,
			IsPayroll: row.Category == "工资",
		})
	}

	result := RealProfitResponse{
		Month:                  monthStr,
		GrossProfit:            math.Round(grossProfit*100) / 100,
		ExpenseTotal:           math.Round(expenseTotal*100) / 100,
		PayrollTotal:           math.Round(payrollTotal*100) / 100,
		RealProfit:             math.Round((grossProfit-expenseTotal-payrollTotal)*100) / 100,
		UncostedOrderItemCount: uncostedCount,
		Breakdown:              breakdown,
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "success",
		"data": result,
	})
}
