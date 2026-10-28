package controllers

import (
	"math"
	"testing"
)

func commItem(sales float64, qty int, fixed float64) CommissionItemInput {
	return CommissionItemInput{
		SalesAmount:           sales,
		Quantity:              qty,
		CommissionFixedAmount: fixed,
	}
}

func assertCommission(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s: calcOrderCommission() = %.4f, want %.4f", name, got, want)
	}
}

// 用例1：无固定提成商品时与旧公式 commissionableOrderAmount 逐分一致（回归基线，oracle 对拍）
func TestCalcOrderCommissionDefaultMatchesLegacyFormula(t *testing.T) {
	cases := []struct {
		name            string
		totalAmount     float64
		actualPayAmount float64
		itemSales       []float64
		noCommIdx       int // -1 表示全部可计提成
		isSubsidy       bool
		employeeRate    float64
	}{
		{name: "全可计提成无折扣", totalAmount: 1000, actualPayAmount: 1000, itemSales: []float64{400, 600}, noCommIdx: -1, employeeRate: 0.03},
		{name: "折扣按比例摊入", totalAmount: 1000, actualPayAmount: 900, itemSales: []float64{400, 600}, noCommIdx: 1, employeeRate: 0.03},
		{name: "国补单保持原销售额口径", totalAmount: 1000, actualPayAmount: 850, itemSales: []float64{400, 600}, noCommIdx: 1, isSubsidy: true, employeeRate: 0.03},
		{name: "全部排除", totalAmount: 1000, actualPayAmount: 900, itemSales: []float64{1000}, noCommIdx: 0, employeeRate: 0.03},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := make([]CommissionItemInput, 0, len(tc.itemSales))
			var commissionableSales float64
			for i, sales := range tc.itemSales {
				noComm := i == tc.noCommIdx
				if !noComm {
					commissionableSales += sales
				}
				items = append(items, commItem(sales, 1, 0))
				items[i].ProductNoCommission = noComm
			}
			want := float64(moneyCents(commissionableOrderAmount(tc.totalAmount, tc.actualPayAmount, commissionableSales, tc.isSubsidy)*tc.employeeRate)) / 100
			got := calcOrderCommission(items, tc.employeeRate, true, tc.totalAmount, tc.actualPayAmount, tc.isSubsidy)
			assertCommission(t, tc.name, got, want)
		})
	}
}

// 用例2：固定提成+默认提成混算（折扣单）。默认部分按剔除固定商品后的销售额摊折扣，固定部分原额照发
func TestCalcOrderCommissionMixedFixedAndDefault(t *testing.T) {
	items := []CommissionItemInput{
		commItem(500, 1, 0),  // 默认
		commItem(300, 1, 20), // 固定 ¥20/台
		commItem(200, 2, 10), // 固定 ¥10/台 ×2
	}
	// total=1000, actualPay=800：默认 800×500/1000×0.03=12.00；固定 20+20=40.00
	assertCommission(t, "mixed", calcOrderCommission(items, 0.03, true, 1000, 800, false), 52.00)
}

// 用例3：国补单默认部分保持原销售额口径，固定部分不受影响
func TestCalcOrderCommissionSubsidyBasis(t *testing.T) {
	items := []CommissionItemInput{
		commItem(600, 1, 0),
		commItem(400, 1, 30),
	}
	// total=1000, actualPay=850，国补：默认 1000×600/1000×0.05=30.00；固定 30.00
	assertCommission(t, "subsidy", calcOrderCommission(items, 0.05, true, 1000, 850, true), 60.00)
}

// 用例4：固定提成商品的赠品行不发提成（普通台照发）
func TestCalcOrderCommissionGiftSkipsFixedCommission(t *testing.T) {
	normal := commItem(200, 2, 50)
	gift := commItem(0, 1, 50)
	gift.IsGift = true
	assertCommission(t, "gift fixed", calcOrderCommission([]CommissionItemInput{normal, gift}, 0.05, true, 200, 200, false), 100.00)
}

// 用例5：no_commission=true 但固定金额残留非 0（防御），默认与固定都不计
func TestCalcOrderCommissionNoCommissionResidualFixed(t *testing.T) {
	residual := commItem(500, 1, 50)
	residual.ProductNoCommission = true
	normal := commItem(500, 1, 0)
	assertCommission(t, "residual fixed", calcOrderCommission([]CommissionItemInput{residual, normal}, 0.03, true, 1000, 1000, false), 15.00)
}

// 用例6：全额折扣 actualPayAmount=0，默认部分为 0，固定提成照发
func TestCalcOrderCommissionFullDiscountFixedStillPaid(t *testing.T) {
	items := []CommissionItemInput{
		commItem(500, 1, 0),
		commItem(500, 2, 30),
	}
	assertCommission(t, "full discount", calcOrderCommission(items, 0.05, true, 1000, 0, false), 60.00)
}

// 用例7：无员工返回 0
func TestCalcOrderCommissionNoEmployee(t *testing.T) {
	items := []CommissionItemInput{commItem(1000, 1, 20)}
	assertCommission(t, "no employee", calcOrderCommission(items, 0, false, 1000, 1000, false), 0)
}

// 用例8：min(defaultSales,totalAmount) 钳制，病态输入不放大提成
func TestCalcOrderCommissionClampsDefaultSales(t *testing.T) {
	items := []CommissionItemInput{commItem(2000, 1, 0)}
	assertCommission(t, "clamp", calcOrderCommission(items, 0.1, true, 1000, 1000, false), 100.00)
}
