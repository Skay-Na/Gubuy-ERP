package services

// 工程模块 · 算法层单测（T03）
//
// 跑法：cd erp-backend && go test ./services/ -v -run TestEngineering
//
// 覆盖五类：① 金额精度与汇总 ② 合同额平账 ③ 状态机 ④ 逾期 ⑤ 盈利口径
// 原则：每个用例都对应一条真实业务场景，不做"为了覆盖率"的空断言。

import (
	"math"
	"strings"
	"testing"
	"time"
)

func d(s string) *time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		panic(err)
	}
	return &t
}

// ---------------------------------------------------------------------------
// ① 金额精度
// ---------------------------------------------------------------------------

func TestEngineeringRoundMoney(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"四舍五入到分_进位", 0.005, 0.01},
		{"四舍五入到分_舍去", 1.234, 1.23},
		{"四舍五入到分_进位2", 1.235, 1.24},
		{"负数进位", -1.235, -1.24},
		{"整数不变", 860000, 860000},
		{"零", 0, 0},
		{"浮点累加误差抹平", 0.1 + 0.2, 0.3},
		{"NaN归零", math.NaN(), 0},
		{"正无穷归零", math.Inf(1), 0},
	}
	for _, c := range cases {
		if got := RoundMoney(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: RoundMoney(%v) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}

func TestEngineeringMoneyEqual(t *testing.T) {
	// 容差 0.005：差 1 分钱必须判为"不等"，这是平账校验的底线
	if !MoneyEqual(860000.00, 860000.00) {
		t.Error("同额应相等")
	}
	if MoneyEqual(860000.00, 860000.01) {
		t.Error("差 0.01 必须判为不等（否则账上永远差一分）")
	}
	if !MoneyEqual(0.1+0.2, 0.3) {
		t.Error("浮点累加误差在容差内应判为相等")
	}
}

func TestEngineeringCalcItemSummary(t *testing.T) {
	// 三行：5999.99×3、1280.005×2（行金额先四舍五入）、0 元赠品行
	items := []EngItemAmount{
		{Quantity: 3, UnitPrice: 5999.99, UnitCost: 4800.00},
		{Quantity: 2, UnitPrice: 1280.00, UnitCost: 1000.00},
		{Quantity: 1, UnitPrice: 0, UnitCost: 0},
	}
	got := CalcItemSummary(items)
	// 17999.97 + 2560.00 + 0
	if !MoneyEqual(got.ItemsTotal, 20559.97) {
		t.Errorf("ItemsTotal = %v, want 20559.97", got.ItemsTotal)
	}
	// 14400 + 2000 + 0
	if !MoneyEqual(got.CostTotal, 16400.00) {
		t.Errorf("CostTotal = %v, want 16400.00", got.CostTotal)
	}
	if got.OrderedQty != 6 {
		t.Errorf("OrderedQty = %v, want 6", got.OrderedQty)
	}
}

func TestEngineeringCalcItemSummaryNegativeQtyIgnored(t *testing.T) {
	got := CalcItemSummary([]EngItemAmount{{Quantity: -5, UnitPrice: 100}})
	if got.ItemsTotal != 0 || got.OrderedQty != 0 {
		t.Errorf("负数数量应被忽略，got %+v", got)
	}
}

func TestEngineeringCalcItemSummaryEmpty(t *testing.T) {
	got := CalcItemSummary(nil)
	if got.ItemsTotal != 0 || got.CostTotal != 0 || got.OrderedQty != 0 {
		t.Errorf("空清单应全 0，got %+v", got)
	}
}

func TestEngineeringCalcContractTotal(t *testing.T) {
	// 设备 690000 + 安装 80000 + 运输 30000 + 其他 0 − 折让 0
	if got := CalcContractTotal(690000, 80000, 30000, 0, 0); !MoneyEqual(got, 800000) {
		t.Errorf("got %v want 800000", got)
	}
	// 折让大于合计：允许为负（零元赠送单场景）
	if got := CalcContractTotal(1000, 0, 0, 0, 1500); !MoneyEqual(got, -500) {
		t.Errorf("折让大于合计应允许负数，got %v", got)
	}
	// 小数位：6999.995 + 0.1
	if got := CalcContractTotal(6999.99, 0.005, 0, 0, 0); !MoneyEqual(got, 7000.00) {
		t.Errorf("got %v want 7000.00", got)
	}
}

func TestEngineeringFormatMoneyCN(t *testing.T) {
	cases := map[float64]string{
		860000:  "¥860,000.00",
		0:       "¥0.00",
		1234.5:  "¥1,234.50",
		1234567: "¥1,234,567.00",
		-500:    "-¥500.00",
	}
	for in, want := range cases {
		if got := FormatMoneyCN(in); got != want {
			t.Errorf("FormatMoneyCN(%v) = %q, want %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// ② 合同额 = 节点合计
// ---------------------------------------------------------------------------

func TestEngineeringValidateContractBalanceOK(t *testing.T) {
	terms := []EngTermPlan{
		{Name: "预付款", PlannedAmount: 258000},
		{Name: "到货款", PlannedAmount: 430000},
		{Name: "质保金", PlannedAmount: 172000},
	}
	if err := ValidateContractBalance(860000, terms); err != nil {
		t.Errorf("合计相等应通过，got %v", err)
	}
}

func TestEngineeringValidateContractBalanceOffByCent(t *testing.T) {
	// 差 1 分钱也必须拦住 —— 这是这套账最容易悄悄出错的地方
	terms := []EngTermPlan{{PlannedAmount: 860000.00}, {PlannedAmount: 0.01}}
	err := ValidateContractBalance(860000.00, terms)
	if err == nil {
		t.Fatal("差 0.01 应报错")
	}
	if !strings.Contains(err.Error(), "不一致") || !strings.Contains(err.Error(), "多") {
		t.Errorf("文案应含差额方向，got %q", err.Error())
	}
	if !strings.Contains(err.Error(), FormatMoneyCN(0.01)) {
		t.Errorf("文案应含具体差额 ¥0.01，got %q", err.Error())
	}
}

func TestEngineeringValidateContractBalanceShort(t *testing.T) {
	terms := []EngTermPlan{{PlannedAmount: 500000}, {PlannedAmount: 350000}}
	err := ValidateContractBalance(860000, terms)
	if err == nil {
		t.Fatal("节点少了 10000 应报错")
	}
	if !strings.Contains(err.Error(), "少") || !strings.Contains(err.Error(), "¥10,000.00") {
		t.Errorf("文案应说明少了 1 万，got %q", err.Error())
	}
}

func TestEngineeringValidateContractBalanceNoTerms(t *testing.T) {
	if err := ValidateContractBalance(860000, nil); err == nil {
		t.Error("没有节点时合同额不可能平账，应报错")
	}
}

func TestEngineeringSumPlannedAmount(t *testing.T) {
	terms := []EngTermPlan{
		{PlannedAmount: 0.005}, {PlannedAmount: 0.005}, {PlannedAmount: 100.00},
	}
	// 每项先归一到 0.01 + 0.01 + 100 = 100.02
	if got := SumPlannedAmount(terms); !MoneyEqual(got, 100.02) {
		t.Errorf("got %v want 100.02", got)
	}
}

// ---------------------------------------------------------------------------
// ③ 状态机
// ---------------------------------------------------------------------------

func TestEngineeringNextStatus(t *testing.T) {
	cases := []struct {
		from   int
		action EngStatusAction
		want   int
	}{
		{EngStatusFollowing, EngActionSign, EngStatusSigned},
		{EngStatusSigned, EngActionRevert, EngStatusFollowing},
		{EngStatusSupplying, EngActionRevert, EngStatusSigned},
		{EngStatusSupplying, EngActionAccept, EngStatusAccepted},
		{EngStatusFollowing, EngActionVoid, EngStatusVoided},
		{EngStatusSigned, EngActionVoid, EngStatusVoided},
		{EngStatusSupplying, EngActionVoid, EngStatusVoided},
		{EngStatusAccepted, EngActionVoid, EngStatusVoided},
	}
	for _, c := range cases {
		if got := NextStatus(c.from, c.action); got != c.want {
			t.Errorf("NextStatus(%d,%s) = %d, want %d", c.from, c.action, got, c.want)
		}
	}
}

func TestEngineeringCanTransitIllegal(t *testing.T) {
	cases := []struct {
		name   string
		from   int
		action EngStatusAction
		msgHas string
	}{
		{"跟进中不能直接验收", EngStatusFollowing, EngActionAccept, "只有「供货中」"},
		{"跟进中不能回退", EngStatusFollowing, EngActionRevert, "不支持回退"},
		{"跟进中不能重复签约外的动作", EngStatusSigned, EngActionSign, "只有「跟进中」"},
		{"已签约不能跳验收", EngStatusSigned, EngActionAccept, "只有「供货中」"},
		{"已验收不能回退", EngStatusAccepted, EngActionRevert, "不支持回退"},
		{"已结清不能作废", EngStatusSettled, EngActionVoid, "不允许作废"},
		{"已结清不能回退", EngStatusSettled, EngActionRevert, "不能回退"},
		{"已作废是终态_不能回退", EngStatusVoided, EngActionRevert, "已作废"},
		{"已作废不能作废", EngStatusVoided, EngActionVoid, "不允许作废"},
		{"未知动作", EngStatusFollowing, EngStatusAction("settle"), "不支持的操作"},
	}
	for _, c := range cases {
		err := CanTransit(c.from, c.action)
		if err == nil {
			t.Errorf("%s: 应报错", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.msgHas) {
			t.Errorf("%s: 文案应含 %q，got %q", c.name, c.msgHas, err.Error())
		}
	}
}

func TestEngineeringCanTransitLegal(t *testing.T) {
	cases := []struct {
		from   int
		action EngStatusAction
	}{
		{EngStatusFollowing, EngActionSign},
		{EngStatusFollowing, EngActionVoid},
		{EngStatusSigned, EngActionRevert},
		{EngStatusSupplying, EngActionRevert},
		{EngStatusSupplying, EngActionAccept},
	}
	for _, c := range cases {
		if err := CanTransit(c.from, c.action); err != nil {
			t.Errorf("CanTransit(%d,%s) 应通过，got %v", c.from, c.action, err)
		}
	}
}

func TestEngineeringSignGuard(t *testing.T) {
	base := EngSignGuard{
		ItemCount: 3, ContractTotal: 860000, TermCount: 3, TermPlanned: 860000, SignDate: d("2026-10-29"),
	}
	if err := CheckSignGuard(base); err != nil {
		t.Errorf("完整条件应通过，got %v", err)
	}

	noItem := base
	noItem.ItemCount = 0
	if err := CheckSignGuard(noItem); err == nil || !strings.Contains(err.Error(), "设备清单") {
		t.Errorf("无清单应拦，got %v", err)
	}

	zeroAmount := base
	zeroAmount.ContractTotal = 0
	if err := CheckSignGuard(zeroAmount); err == nil || !strings.Contains(err.Error(), "合同总额") {
		t.Errorf("合同额为 0 应拦，got %v", err)
	}

	noTerm := base
	noTerm.TermCount = 0
	if err := CheckSignGuard(noTerm); err == nil || !strings.Contains(err.Error(), "付款节点") {
		t.Errorf("无节点应拦，got %v", err)
	}

	unbalanced := base
	unbalanced.TermPlanned = 850000
	if err := CheckSignGuard(unbalanced); err == nil || !strings.Contains(err.Error(), "调平") {
		t.Errorf("节点不平应拦，got %v", err)
	}

	noDate := base
	noDate.SignDate = nil
	if err := CheckSignGuard(noDate); err == nil || !strings.Contains(err.Error(), "签约日期") {
		t.Errorf("无签约日期应拦，got %v", err)
	}
}

func TestEngineeringAcceptGuard(t *testing.T) {
	base := EngAcceptGuard{OrderedQty: 86, ShippedQty: 86, AcceptDate: d("2026-10-29")}
	if err := CheckAcceptGuard(base); err != nil {
		t.Errorf("发完货应通过，got %v", err)
	}

	partial := base
	partial.ShippedQty = 52
	if err := CheckAcceptGuard(partial); err == nil || !strings.Contains(err.Error(), "34 台未出库") {
		t.Errorf("未发完应拦并报缺口，got %v", err)
	}

	noItems := base
	noItems.OrderedQty, noItems.ShippedQty = 0, 0
	if err := CheckAcceptGuard(noItems); err == nil || !strings.Contains(err.Error(), "没有设备清单") {
		t.Errorf("无清单应拦，got %v", err)
	}

	noDate := base
	noDate.AcceptDate = nil
	if err := CheckAcceptGuard(noDate); err == nil || !strings.Contains(err.Error(), "验收日期") {
		t.Errorf("无验收日期应拦，got %v", err)
	}
}

func TestEngineeringVoidGuard(t *testing.T) {
	if err := CheckVoidGuard(EngStatusSupplying, "甲方资金链断裂，项目终止"); err != nil {
		t.Errorf("正常作废应通过，got %v", err)
	}
	if err := CheckVoidGuard(EngStatusSettled, "甲方资金链断裂"); err == nil {
		t.Error("已结清不允许作废")
	}
	if err := CheckVoidGuard(EngStatusVoided, "重复操作"); err == nil {
		t.Error("已作废不允许重复作废")
	}
	if err := CheckVoidGuard(EngStatusSupplying, "不想做了"); err == nil {
		t.Error("原因不足 5 字应拦")
	}
	if err := CheckVoidGuard(EngStatusSupplying, "   "); err == nil {
		t.Error("空白原因应拦")
	}
	// 中文按字数计：5 个字刚好通过
	if err := CheckVoidGuard(EngStatusSupplying, "项目取消掉"); err != nil {
		t.Errorf("5 个字应通过，got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ④ 逾期
// ---------------------------------------------------------------------------

func TestEngineeringCalcOverdue(t *testing.T) {
	today := *d("2026-10-29")
	terms := []EngTermDue{
		{PlannedAmount: 258000, ReceivedAmount: 258000, PlannedDate: d("2026-09-01")}, // 已收满，不算逾期
		{PlannedAmount: 430000, ReceivedAmount: 330000, PlannedDate: d("2026-10-17")}, // 逾期 12 天，欠 10 万
		{PlannedAmount: 172000, ReceivedAmount: 0, PlannedDate: d("2026-12-31")},      // 未到期
		{PlannedAmount: 50000, ReceivedAmount: 0, PlannedDate: nil},                   // 无日期不计
	}
	got := CalcOverdue(terms, today)
	if !MoneyEqual(got.Amount, 100000) {
		t.Errorf("逾期金额 = %v, want 100000", got.Amount)
	}
	if got.Days != 12 {
		t.Errorf("逾期天数 = %v, want 12", got.Days)
	}
	if got.Count != 1 {
		t.Errorf("逾期节点数 = %v, want 1", got.Count)
	}
}

func TestEngineeringCalcOverdueCrossMonth(t *testing.T) {
	today := *d("2026-03-01")
	terms := []EngTermDue{{PlannedAmount: 100, ReceivedAmount: 0, PlannedDate: d("2026-02-01")}}
	got := CalcOverdue(terms, today)
	if got.Days != 28 {
		t.Errorf("跨月逾期天数 = %v, want 28（2026 年 2 月 28 天）", got.Days)
	}
}

func TestEngineeringCalcOverdueCrossYear(t *testing.T) {
	today := *d("2026-01-05")
	terms := []EngTermDue{{PlannedAmount: 100, ReceivedAmount: 0, PlannedDate: d("2025-12-26")}}
	got := CalcOverdue(terms, today)
	if got.Days != 10 {
		t.Errorf("跨年逾期天数 = %v, want 10", got.Days)
	}
	if !MoneyEqual(got.Amount, 100) {
		t.Errorf("跨年逾期金额 = %v, want 100", got.Amount)
	}
}

func TestEngineeringCalcOverdueDueToday(t *testing.T) {
	today := *d("2026-10-29")
	terms := []EngTermDue{{PlannedAmount: 100, ReceivedAmount: 0, PlannedDate: d("2026-10-29")}}
	got := CalcOverdue(terms, today)
	if got.Count != 0 || got.Amount != 0 {
		t.Errorf("当天到期的节点不算逾期，got %+v", got)
	}
}

func TestEngineeringCalcOverduePartialCent(t *testing.T) {
	today := *d("2026-10-29")
	terms := []EngTermDue{{PlannedAmount: 100, ReceivedAmount: 99.99, PlannedDate: d("2026-10-01")}}
	got := CalcOverdue(terms, today)
	if !MoneyEqual(got.Amount, 0.01) {
		t.Errorf("差 1 分也算逾期，金额应 0.01，got %v", got.Amount)
	}
}

func TestEngineeringCalcOverdueEmpty(t *testing.T) {
	got := CalcOverdue(nil, time.Now())
	if got.Amount != 0 || got.Days != 0 || got.Count != 0 {
		t.Errorf("空节点应全 0，got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// ⑤ 盈利口径
// ---------------------------------------------------------------------------

func TestEngineeringCalcProfitabilityRealCase(t *testing.T) {
	// 技术方案 DoD 指定的真实案例：合同 860,000 − 成本 690,000 − 费用 46,800 = 净利 123,200
	p := CalcProfitability(860000, 690000, 46800)
	if !MoneyEqual(p.GrossProfit, 170000) {
		t.Errorf("设备毛利 = %v, want 170000", p.GrossProfit)
	}
	if !MoneyEqual(p.NetProfit, 123200) {
		t.Errorf("项目净利 = %v, want 123200", p.NetProfit)
	}
	if !MoneyEqual(p.GrossMarginRate, 19.77) {
		t.Errorf("设备毛利率 = %v, want 19.77", p.GrossMarginRate)
	}
	if !MoneyEqual(p.NetMarginRate, 14.33) {
		t.Errorf("净利率 = %v, want 14.33", p.NetMarginRate)
	}
}

func TestEngineeringCalcProfitabilityNegative(t *testing.T) {
	p := CalcProfitability(100000, 90000, 30000)
	if !MoneyEqual(p.GrossProfit, 10000) {
		t.Errorf("设备毛利 = %v, want 10000", p.GrossProfit)
	}
	if !MoneyEqual(p.NetProfit, -20000) {
		t.Errorf("净利应为负 -20000，got %v", p.NetProfit)
	}
	if !MoneyEqual(p.NetMarginRate, -20) {
		t.Errorf("净利率 = %v, want -20", p.NetMarginRate)
	}
}

func TestEngineeringCalcProfitabilityZeroContract(t *testing.T) {
	p := CalcProfitability(0, 5000, 1000)
	if p.GrossMarginRate != 0 || p.NetMarginRate != 0 {
		t.Errorf("合同额为 0 时两个毛利率都必须返回 0（不能 NaN），got %+v", p)
	}
	if math.IsNaN(p.NetMarginRate) || math.IsInf(p.NetMarginRate, 0) {
		t.Error("合同额为 0 时禁止出现 NaN/Inf")
	}
	if !MoneyEqual(p.NetProfit, -6000) {
		t.Errorf("净利 = %v, want -6000", p.NetProfit)
	}
}

func TestEngineeringCalcNetProfitPublicExpenseExcluded(t *testing.T) {
	// 项目费用 46,800 已扣除；公共费用不在此函数内（口径落点在 recalcExpenseTotal）
	if got := CalcNetProfit(860000, 690000, 46800); !MoneyEqual(got, 123200) {
		t.Errorf("CalcNetProfit = %v, want 123200", got)
	}
	if got := CalcNetProfit(860000, 690000, 0); !MoneyEqual(got, 170000) {
		t.Errorf("无费用时净利 = 设备毛利 = %v, want 170000", got)
	}
}

func TestEngineeringCalcEquipMarginRate(t *testing.T) {
	if got := CalcEquipMarginRate(860000, 690000); !MoneyEqual(got, 19.77) {
		t.Errorf("got %v want 19.77", got)
	}
	if got := CalcEquipMarginRate(0, 0); got != 0 {
		t.Errorf("合同额 0 时返回 0，got %v", got)
	}
}

// ---------------------------------------------------------------------------
// 回款 / 出库闸门
// ---------------------------------------------------------------------------

func TestEngineeringCheckReceiptAmount(t *testing.T) {
	if err := CheckReceiptAmount(258000, 0, 258000); err != nil {
		t.Errorf("刚好收满应通过，got %v", err)
	}
	err := CheckReceiptAmount(258000, 200000, 60000)
	if err == nil || !strings.Contains(err.Error(), "超过该节点剩余应收") {
		t.Errorf("超收应拦，got %v", err)
	}
	if err := CheckReceiptAmount(258000, 0, 0); err == nil {
		t.Error("0 元回款应拦")
	}
	if err := CheckReceiptAmount(258000, 0, -100); err == nil {
		t.Error("负数回款应拦（冲销走单独接口）")
	}
	// 差 1 分不允许
	if err := CheckReceiptAmount(100, 0, 100.01); err == nil {
		t.Error("超 1 分也应拦")
	}
}

func TestEngineeringCheckReverseAmount(t *testing.T) {
	if err := CheckReverseAmount(258000, 258000); err != nil {
		t.Errorf("全额冲销应通过，got %v", err)
	}
	if err := CheckReverseAmount(258000, 300000); err == nil {
		t.Error("冲销超过已收应拦")
	}
	if err := CheckReverseAmount(258000, 0); err == nil {
		t.Error("冲销 0 应拦")
	}
}

func TestEngineeringCheckShipmentQty(t *testing.T) {
	if err := CheckShipmentQty(10, 4, 6); err != nil {
		t.Errorf("刚好发完应通过，got %v", err)
	}
	err := CheckShipmentQty(10, 4, 7)
	if err == nil || !strings.Contains(err.Error(), "超过该行未发数量 6 台") {
		t.Errorf("超发应拦并报缺口，got %v", err)
	}
	if err := CheckShipmentQty(10, 4, 0); err == nil {
		t.Error("0 台出库应拦")
	}
}

// ---------------------------------------------------------------------------
// 字典与日期工具
// ---------------------------------------------------------------------------

func TestEngineeringLabels(t *testing.T) {
	statuses := map[int]string{
		EngStatusFollowing: "跟进中", EngStatusSigned: "已签约", EngStatusSupplying: "供货中",
		EngStatusAccepted: "已验收", EngStatusSettled: "已结清", EngStatusVoided: "已作废", 0: "未知",
	}
	for k, v := range statuses {
		if got := EngStatusLabel(k); got != v {
			t.Errorf("EngStatusLabel(%d) = %q, want %q", k, got, v)
		}
	}
	if EngExpenseCategoryLabel(1) != "安装人工" || EngExpenseCategoryLabel(7) != "其他" || EngExpenseCategoryLabel(99) != "其他" {
		t.Error("费用类别文案不符")
	}
	if EngPayStatusLabel(1) != "已付" || EngPayStatusLabel(0) != "未付" {
		t.Error("付款状态文案不符")
	}
	if EngPayMethodLabel(3) != "公户" || EngPayMethodLabel(0) != "未指定" {
		t.Error("付款方式文案不符")
	}
	if EngWarehouseLabel(2) != "门店" || EngWarehouseLabel(3) != "云仓" {
		t.Error("仓库文案不符")
	}
	if EngReceiptMethodLabel(1) != "银行转账" || EngProjectTypeLabel(4) != "政企学校" {
		t.Error("回款方式/项目类型文案不符")
	}
	if len(EngExpenseCategories) != 7 {
		t.Errorf("费用类别必须恒为 7 类，got %d", len(EngExpenseCategories))
	}
}

func TestEngineeringBusinessDate(t *testing.T) {
	got, err := ParseBusinessDate("2026-10-29")
	if err != nil || got == nil || got.Format("2006-01-02") != "2026-10-29" {
		t.Errorf("解析失败：%v %v", got, err)
	}
	if v, err := ParseBusinessDate(""); v != nil || err != nil {
		t.Error("空串应返回 nil 且不报错")
	}
	if _, err := ParseBusinessDate("2026/10/29"); err == nil {
		t.Error("错误格式应报错")
	}
	if s := FormatBusinessDate(got); s == nil || *s != "2026-10-29" {
		t.Errorf("格式化失败：%v", s)
	}
	if s := FormatBusinessDate(nil); s != nil {
		t.Error("nil 日期应输出 nil（JSON 出 null）")
	}
}

func TestEngineeringAmountLocked(t *testing.T) {
	if EngAmountLocked(EngStatusFollowing) {
		t.Error("跟进中不应锁定金额")
	}
	for _, s := range []int{EngStatusSigned, EngStatusSupplying, EngStatusAccepted, EngStatusSettled} {
		if !EngAmountLocked(s) {
			t.Errorf("状态 %d 应锁定金额（走平账校验）", s)
		}
	}
	if EngEditable(EngStatusVoided) {
		t.Error("作废项目应只读")
	}
	if !EngEditable(EngStatusSupplying) {
		t.Error("供货中应可编辑备注等非金额字段")
	}
}
