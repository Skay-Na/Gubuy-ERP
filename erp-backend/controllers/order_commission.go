package controllers

import "math"

// CommissionItemInput 开单循环内逐项收集的提成输入快照
type CommissionItemInput struct {
	SalesAmount           float64 // item.UnitPrice * item.Quantity（赠品为 0）
	Quantity              int
	IsGift                bool
	ProductNoCommission   bool
	CommissionFixedAmount float64 // >0 表示该商品按每台固定金额计提
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

	// 国补单保持原销售额口径；普通单把订单级减免（折扣/以旧换新/国补）按销售额比例摊入
	commissionBase := actualPayAmount
	if isSubsidy {
		commissionBase = totalAmount
	}

	// 默认部分：剔除固定提成商品后的销售额走员工比例，与原 commissionBase * min(commissionableSales,total)/total 同构
	var defaultSales float64
	for _, item := range items {
		if item.IsGift || item.ProductNoCommission || item.CommissionFixedAmount > 0 {
			continue
		}
		defaultSales += item.SalesAmount
	}
	defaultPart := 0.0
	if defaultSales > 0 {
		defaultBase := commissionBase * math.Min(defaultSales, totalAmount) / totalAmount
		defaultPart = float64(moneyCents(defaultBase*employeeRate)) / 100
	}

	// 固定提成部分：类计件工资按台计，不参与折扣分摊
	// 项级排除：赠品不发"每台固定提成"；NoCommission 商品保存时已归零，此处兜底防御
	fixedPart := 0.0
	for _, item := range items {
		if item.IsGift || item.ProductNoCommission || item.CommissionFixedAmount <= 0 {
			continue
		}
		fixedPart += float64(moneyCents(item.CommissionFixedAmount*float64(item.Quantity))) / 100
	}
	return defaultPart + fixedPart
}
