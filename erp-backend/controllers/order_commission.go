package controllers

import "math"

// CommissionItemInput 开单循环内逐项收集的提成输入快照
type CommissionItemInput struct {
	SalesAmount           float64 // item.UnitPrice * item.Quantity（赠品为 0）
	Quantity              int
	IsGift                bool
	ProductNoCommission   bool
	CommissionMode        int     // 0/1/2
	CommissionRate        float64
	CommissionFixedAmount float64
}

// calcOrderCommission 纯函数：输入订单项快照 + 金额信息 + 员工比例，输出订单提成（元，两位小数）。
// 不依赖 DB / gin，可单测。整单级判定（商会/老板客户）留在调用方。
func calcOrderCommission(
	items []CommissionItemInput,
	employeeRate float64, // 员工不存在时传 0
	hasEmployee bool,
	totalAmount, actualPayAmount float64,
	isSubsidy bool,
) float64 {
	if !hasEmployee {
		return 0
	}

	// 分摊因子 K：普通单把订单级减免（折扣/以旧换新/国补）按销售额比例摊入；国补单保持原销售额口径
	commissionBase := actualPayAmount
	if isSubsidy {
		commissionBase = totalAmount
	}
	k := 0.0
	if totalAmount > 0 {
		k = commissionBase / totalAmount
	}

	// 默认部分：仅 mode=0 商品参与，与原 commissionBase * min(commissionableSales,total)/total 完全同构
	var defaultSales float64
	for _, item := range items {
		if item.IsGift || item.ProductNoCommission || item.CommissionMode != 0 {
			continue
		}
		defaultSales += item.SalesAmount
	}
	defaultPart := 0.0
	if defaultSales > 0 {
		defaultBase := commissionBase * math.Min(defaultSales, totalAmount) / totalAmount
		defaultPart = float64(moneyCents(defaultBase * employeeRate)) / 100
	}

	// 自定义部分：mode=1 逐项按分取整累加；mode=2 固定金额不参与折扣分摊（类计件工资，与成交价无关）
	customPart := 0.0
	for _, item := range items {
		// 项级排除：赠品销售额为 0 且不发"每台固定提成"；NoCommission 无论 mode 整体排除（保存时已归零，此处兜底防御）
		if item.IsGift || item.ProductNoCommission {
			continue
		}
		switch item.CommissionMode {
		case 1:
			customPart += float64(moneyCents(item.SalesAmount*k*item.CommissionRate)) / 100
		case 2:
			customPart += float64(moneyCents(item.CommissionFixedAmount*float64(item.Quantity))) / 100
		}
	}
	return defaultPart + customPart
}
