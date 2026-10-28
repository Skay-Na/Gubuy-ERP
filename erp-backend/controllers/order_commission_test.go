package controllers

import (
	"math"
	"testing"
)

func commItem(sales float64, qty int, mode int, rate, fixed float64) CommissionItemInput {
	return CommissionItemInput{
		SalesAmount:           sales,
		Quantity:              qty,
		CommissionMode:        mode,
		CommissionRate:        rate,
		CommissionFixedAmount: fixed,
	}
}

func assertCommission(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s: calcOrderCommission() = %.4f, want %.4f", name, got, want)
	}
}

// 用例1：全 mode=0 与旧公式 commissionableOrderAmount 逐分一致（回归基线）
func TestCalcOrderCommissionMode0MatchesLegacyFormula(t *testing.T) {
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
				items = append(items, commItem(sales, 1, 0, 0, 0))
				items[i].ProductNoCommission = noComm
			}
			want := float64(moneyCents(commissionableOrderAmount(tc.totalAmount, tc.actualPayAmount, commissionableSales, tc.isSubsidy)*tc.employeeRate)) / 100
			got := calcOrderCommission(items, tc.employeeRate, true, tc.totalAmount, tc.actualPayAmount, tc.isSubsidy)
			assertCommission(t, tc.name, got, want)
		})
	}
}

// 用例2：全 mode=1，不同 rate 两项，逐项取分后求和
func TestCalcOrderCommissionMode1TwoRates(t *testing.T) {
	items := []CommissionItemInput{
		commItem(400, 1, 1, 0.05, 0),
		commItem(600, 1, 1, 0.02, 0),
	}
	// 400*1*0.05=20.00 + 600*1*0.02=12.00
	assertCommission(t, "mode1", calcOrderCommission(items, 0.03, true, 1000, 1000, false), 32.00)
}

// 用例3：mode=1 与 mode=0 混合，默认部分只按 defaultSales 摊
func TestCalcOrderCommissionMixedMode0And1(t *testing.T) {
	items := []CommissionItemInput{
		commItem(500, 1, 0, 0, 0),
		commItem(500, 1, 1, 0.10, 0),
	}
	// 默认：800*500/1000=400 → 400*0.03=12.00；自定义：500*0.8*0.10=40.00
	assertCommission(t, "mixed", calcOrderCommission(items, 0.03, true, 1000, 800, false), 52.00)
}

// 用例4：mode=2 多台，qty=3 fixed=50，与成交价无关
func TestCalcOrderCommissionMode2MultiQty(t *testing.T) {
	items := []CommissionItemInput{commItem(1000, 3, 2, 0, 50)}
	// 大额折扣下仍为 150
	assertCommission(t, "mode2 qty3", calcOrderCommission(items, 0.05, true, 1000, 700, false), 150.00)
}

// 用例5：mode=2 商品 + 同商品赠品行，赠品不发固定提成
func TestCalcOrderCommissionGiftSkipsFixedCommission(t *testing.T) {
	normal := commItem(200, 2, 2, 0, 50)
	gift := commItem(0, 1, 2, 0, 50)
	gift.IsGift = true
	assertCommission(t, "gift fixed", calcOrderCommission([]CommissionItemInput{normal, gift}, 0.05, true, 200, 200, false), 100.00)
}

// 用例6：普通单带折扣，mode=1 按 K=actualPay/total 缩水
func TestCalcOrderCommissionMode1ShrinksWithDiscount(t *testing.T) {
	items := []CommissionItemInput{commItem(1000, 1, 1, 0.1, 0)}
	assertCommission(t, "k factor", calcOrderCommission(items, 0.05, true, 1000, 900, false), 90.00)
}

// 用例7：国补单 K=1，mode=1 按原销售额口径
func TestCalcOrderCommissionSubsidyKeepsSalesBasis(t *testing.T) {
	items := []CommissionItemInput{commItem(600, 1, 1, 0.1, 0)}
	assertCommission(t, "subsidy", calcOrderCommission(items, 0.05, true, 1000, 850, true), 60.00)
}

// 用例8：mode=2 + 大额折扣，固定金额不缩水
func TestCalcOrderCommissionMode2NotAffectedByDiscount(t *testing.T) {
	items := []CommissionItemInput{commItem(1000, 1, 2, 0, 20)}
	assertCommission(t, "fixed discount", calcOrderCommission(items, 0.05, true, 1000, 100, false), 20.00)
}

// 用例9：no_commission=true 但 mode 残留非 0（防御），两部分都不计
func TestCalcOrderCommissionNoCommissionResidualMode(t *testing.T) {
	residual := commItem(500, 1, 1, 0.5, 0)
	residual.ProductNoCommission = true
	normal := commItem(500, 1, 0, 0, 0)
	assertCommission(t, "residual mode", calcOrderCommission([]CommissionItemInput{residual, normal}, 0.03, true, 1000, 1000, false), 15.00)
}

// 用例10：无员工返回 0
func TestCalcOrderCommissionNoEmployee(t *testing.T) {
	items := []CommissionItemInput{commItem(1000, 1, 1, 0.1, 0)}
	assertCommission(t, "no employee", calcOrderCommission(items, 0, false, 1000, 1000, false), 0)
}

// 用例11：全额折扣 actualPayAmount=0，mode=1 为 0，mode=2 照发
func TestCalcOrderCommissionFullDiscount(t *testing.T) {
	items := []CommissionItemInput{
		commItem(500, 1, 1, 0.1, 0),
		commItem(500, 2, 2, 0, 30),
	}
	assertCommission(t, "full discount", calcOrderCommission(items, 0.05, true, 1000, 0, false), 60.00)
}

// 用例12：mode=1 逐项取分，无累计漂移（3×0.17 元×0.03 逐项各进 1 分）
func TestCalcOrderCommissionMode1CentRounding(t *testing.T) {
	items := []CommissionItemInput{
		commItem(0.17, 1, 1, 0.03, 0),
		commItem(0.17, 1, 1, 0.03, 0),
		commItem(0.17, 1, 1, 0.03, 0),
	}
	// 逐项：round(0.51分)=1分 ×3 = 3分；汇总取分会得到 2 分（0.51*0.03 元）
	assertCommission(t, "cent rounding", calcOrderCommission(items, 0, true, 0.51, 0.51, false), 0.03)
}

// 用例13：赠品 mode=1 商品，销售额 0 贡献 0
func TestCalcOrderCommissionGiftMode1(t *testing.T) {
	gift := commItem(0, 1, 1, 0.5, 0)
	gift.IsGift = true
	assertCommission(t, "gift mode1", calcOrderCommission([]CommissionItemInput{gift}, 0.05, true, 100, 100, false), 0)
}

// 用例14：customSales=0 且 defaultSales>0，等价旧逻辑
func TestCalcOrderCommissionDefaultOnlyEqualsLegacy(t *testing.T) {
	items := []CommissionItemInput{commItem(600, 1, 0, 0, 0)}
	want := float64(moneyCents(commissionableOrderAmount(1000, 900, 600, false)*0.03)) / 100
	assertCommission(t, "default only", calcOrderCommission(items, 0.03, true, 1000, 900, false), want)
}

// 用例15：defaultSales=0 且全部 mode=2
func TestCalcOrderCommissionFixedOnly(t *testing.T) {
	items := []CommissionItemInput{
		commItem(600, 1, 2, 0, 30),
		commItem(400, 2, 2, 0, 10),
	}
	assertCommission(t, "fixed only", calcOrderCommission(items, 0.05, true, 1000, 900, false), 50.00)
}

// 用例16：min(defaultSales,totalAmount) 钳制，病态输入不放大提成
func TestCalcOrderCommissionClampsDefaultSales(t *testing.T) {
	items := []CommissionItemInput{commItem(2000, 1, 0, 0, 0)}
	assertCommission(t, "clamp", calcOrderCommission(items, 0.1, true, 1000, 1000, false), 100.00)
}
