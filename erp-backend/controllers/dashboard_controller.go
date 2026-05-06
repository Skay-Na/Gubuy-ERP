package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ProductSaleStat 产品销售统计
type ProductSaleStat struct {
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Revenue  float64 `json:"revenue"`
}

// FinanceSummaryResponse 财务总览响应数据结构
type FinanceSummaryResponse struct {
	TodayRevenue        float64           `json:"today_revenue"`
	TotalExpense        float64           `json:"total_expense"`
	PendingSubsidy      float64           `json:"pending_subsidy"`
	OrderCount          int64             `json:"order_count"`
	PendingBalance      float64           `json:"pending_balance"`       // 待收尾款总额
	PendingBalanceCount int64             `json:"pending_balance_count"` // 欠款订单数
	Accounts            []models.Account  `json:"accounts"`              // 各账户余额
	TopProducts         []ProductSaleStat `json:"top_products"`          // 热销排行
}

// GetFinanceSummary 聚合查询财务统计数据
func GetFinanceSummary(c *gin.Context) {
	period := c.DefaultQuery("period", "today")
	var summary FinanceSummaryResponse

	now := time.Now()
	var startTime time.Time
	location := now.Location()

	switch period {
	case "today":
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	case "month":
		startTime = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)
	case "all":
		startTime = time.Time{} // Zero time for all
	default:
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	}

	// 1. 营业额 (Revenue) - 仅统计正常订单
	db := config.DB.Model(&models.Order{}).Where("order_status = 1")
	if !startTime.IsZero() {
		db = db.Where("created_at >= ?", startTime)
	}
	db.Select("COALESCE(SUM(total_amount), 0)").Scan(&summary.TodayRevenue)

	// 2. 支出 (Expense) - 基于财务流水表 (flow_type = 2)
	expenseDB := config.DB.Model(&models.FinancialLog{}).Where("flow_type = ?", 2)
	if !startTime.IsZero() {
		expenseDB = expenseDB.Where("created_at >= ?", startTime)
	}
	expenseDB.Select("COALESCE(SUM(amount), 0)").Scan(&summary.TotalExpense)

	// 3. 国补待回款 (PendingSubsidy) - 存量数据，不随时间周期变动
	config.DB.Model(&models.Order{}).
		Where("subsidy_status IN ? AND order_status = 1", []int{1, 2}).
		Select("COALESCE(SUM(subsidy_amount), 0)").
		Scan(&summary.PendingSubsidy)

	// 4. 订单数 (OrderCount) - 仅正常订单
	orderCountDB := config.DB.Model(&models.Order{}).Where("order_status = 1")
	if !startTime.IsZero() {
		orderCountDB = orderCountDB.Where("created_at >= ?", startTime)
	}
	orderCountDB.Count(&summary.OrderCount)

	// 5. 待收尾款总额 (PendingBalance) 和 欠款订单数 (PendingBalanceCount) - 存量数据
	config.DB.Model(&models.Order{}).
		Where("payment_status = ? AND order_status = 1 AND is_installed = ?", 1, true).
		Select("COALESCE(SUM(actual_pay_amount - deposit_amount), 0)").
		Scan(&summary.PendingBalance)

	config.DB.Model(&models.Order{}).
		Where("payment_status = ? AND order_status = 1 AND is_installed = ?", 1, true).
		Count(&summary.PendingBalanceCount)

	// 6. 各账户余额
	config.DB.Order("id asc").Find(&summary.Accounts)

	// 7. 热销排行
	topDB := config.DB.Table("order_items").
		Select("products.name as name, SUM(order_items.quantity) as quantity, SUM(order_items.quantity * order_items.unit_price) as revenue").
		Joins("JOIN products ON order_items.product_id = products.id").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("orders.order_status = 1")

	if !startTime.IsZero() {
		topDB = topDB.Where("orders.created_at >= ?", startTime)
	}

	topDB.Group("products.id").Order("quantity desc").Limit(5).Scan(&summary.TopProducts)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": summary,
	})
}
