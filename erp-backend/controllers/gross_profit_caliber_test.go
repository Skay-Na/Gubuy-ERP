package controllers

import (
	"math"
	"strconv"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"
)

// TestMonthlyGrossProfitDeductsDiscountAndTradeIn 毛利口径统一（2026-10-26 用户定稿）：
// 以财务总览 CalculateGrossProfit 口径为准——收入 = 订单总额 − 手动折扣，
// 毛利再单独扣除以旧换新折抵，真实净利润不再比看板虚高。
func TestMonthlyGrossProfitDeductsDiscountAndTradeIn(t *testing.T) {
	setupPayrollTest(t)
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Nanosecond)
	unique := strconv.FormatInt(time.Now().UnixNano(), 10)

	// 甲单：总额 1000、折扣 102、以旧换新 600（无成本无费用）→ 收入 898，毛利 298
	// 乙单：总额 500、无折扣无以旧换新 → 收入 500，毛利 500
	// 合计：收入 1398，毛利 1398 - 600 = 798
	discounted := models.Order{
		OrderNo: "GP-DISCOUNT-" + unique, CustomerName: "毛利口径客户甲", OrderStatus: 1, OrderType: 1,
		TotalAmount: 1000, DiscountAmount: 102, TradeInAmount: 600,
		CreatedAt: monthStart.AddDate(0, 0, 2),
	}
	plain := models.Order{
		OrderNo: "GP-PLAIN-" + unique, CustomerName: "毛利口径客户乙", OrderStatus: 1, OrderType: 1,
		TotalAmount: 500,
		CreatedAt:   monthStart.AddDate(0, 0, 3),
	}
	for _, order := range []*models.Order{&discounted, &plain} {
		if err := config.DB.Create(order).Error; err != nil {
			t.Fatalf("创建毛利口径测试订单失败: %v", err)
		}
	}
	t.Cleanup(func() {
		config.DB.Unscoped().Delete(&discounted)
		config.DB.Unscoped().Delete(&plain)
	})

	revenue, grossProfit, _, err := monthlyGrossProfitStats(monthStart, monthEnd)
	if err != nil {
		t.Fatalf("毛利统计失败: %v", err)
	}
	if math.Abs(revenue-1398) > 0.01 {
		t.Fatalf("收入应扣除手动折扣为 1398，实际 %.2f", revenue)
	}
	if math.Abs(grossProfit-798) > 0.01 {
		t.Fatalf("毛利应扣除折扣与以旧换新为 798，实际 %.2f", grossProfit)
	}
}
