package services

// 工程模块 · 业务算法层（纯函数，无 DB / 无 HTTP 依赖）
//
// 归属任务卡：T03（docs/tasks/2026-10-29-engineering-module-task-cards.md）
// 设计基线：docs/tech/2026-10-29-engineering-module-tech-design.md §3 状态机、§4 金额算法
//
// 为什么坚持"纯函数"：
//  1. 金额口径只有一处：合同额、净利、逾期、毛利率全部在这里算，controller 只做取数与落库；
//  2. 可以脱离数据库跑单测，30+ 用例秒级验证，改口径不会悄悄改坏账；
//  3. 本文件禁止 import gorm / gin / net/http —— 出现即评审退回。
//
// 三条隔离声明（与 models/engineering.go 一致）：
//  1. 本文件不涉及 orders / order_items；
//  2. 本文件不涉及 accounts / financial_logs，任何金额计算都不产生账户流水；
//  3. 工程费用只进"项目净利"，不进经营开销。

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// 状态与动作枚举（唯一事实来源，前端不要另建字典）
// ---------------------------------------------------------------------------

const (
	EngStatusFollowing = 1 // 跟进中
	EngStatusSigned    = 2 // 已签约
	EngStatusSupplying = 3 // 供货中
	EngStatusAccepted  = 4 // 已验收
	EngStatusSettled   = 5 // 已结清
	EngStatusVoided    = 9 // 已作废
)

// EngStatusAction 状态流转动作
type EngStatusAction string

const (
	EngActionSign   EngStatusAction = "sign"   // 跟进中 → 已签约
	EngActionAccept EngStatusAction = "accept" // 供货中 → 已验收
	EngActionRevert EngStatusAction = "revert" // 退一步
	EngActionVoid   EngStatusAction = "void"   // 任意态 → 已作废（已结清除外）
)

// EngExpenseCategories 工程费用七类，顺序即前端展示顺序。
// 固定不可自定义：开放自定义半年后会出现"安装费/安装人工/安装工人工资"三个同义类目，汇总失效。
var EngExpenseCategories = []int{1, 2, 3, 4, 5, 6, 7}

// ---------------------------------------------------------------------------
// 4.1 精度处理（统一入口，禁止散落）
// ---------------------------------------------------------------------------

// RoundMoney 金额归一到 2 位小数。所有落库金额、所有对外展示的计算金额都必须过这里。
func RoundMoney(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

// moneyEpsilon 金额比较容差。
// 注意：容差 0.005，意味着"差 1 分钱"会被判为不等 —— 这正是我们要的行为，
// 合同额与节点合计差 0.01 元也必须拦住，否则账上永远差一分。
const moneyEpsilon = 0.005

// MoneyEqual 金额比较（禁止用 == 比较浮点金额）
func MoneyEqual(a, b float64) bool {
	return math.Abs(a-b) < moneyEpsilon
}

// cents 金额转"分"，用于精确比较与差额计算，避免浮点直接相减出现 0.009999999
func cents(v float64) int64 {
	return int64(math.Round(v * 100))
}

// FormatMoneyCN 金额格式化：¥860,000.00（用于报错文案，前端另有展示格式化函数）
func FormatMoneyCN(v float64) string {
	neg := v < 0
	c := cents(math.Abs(v))
	whole := c / 100
	frac := c % 100
	s := fmt.Sprintf("%d", whole)
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	out := fmt.Sprintf("¥%s.%02d", b.String(), frac)
	if neg {
		return "-" + out
	}
	return out
}

// ---------------------------------------------------------------------------
// 4.2 合同总额重算（后端唯一权威，前端只做预览）
// ---------------------------------------------------------------------------

// EngItemAmount 参与金额汇总的清单行（只含计算所需字段，与 models 解耦）
type EngItemAmount struct {
	Quantity  int
	UnitPrice float64
	UnitCost  float64
}

// EngAmountSummary 清单汇总结果
type EngAmountSummary struct {
	ItemsTotal float64 // 设备总额 = Σ round(数量 × 单价)
	CostTotal  float64 // 设备成本 = Σ round(数量 × 成本单价)
	OrderedQty int     // 订购台数 = Σ 数量
}

// CalcItemSummary 汇总清单：行金额逐行四舍五入后再累加，与前端展示的"行小计"永远一致。
func CalcItemSummary(items []EngItemAmount) EngAmountSummary {
	var s EngAmountSummary
	for _, it := range items {
		qty := it.Quantity
		if qty < 0 {
			qty = 0
		}
		s.ItemsTotal += RoundMoney(float64(qty) * it.UnitPrice)
		s.CostTotal += RoundMoney(float64(qty) * it.UnitCost)
		s.OrderedQty += qty
	}
	s.ItemsTotal = RoundMoney(s.ItemsTotal)
	s.CostTotal = RoundMoney(s.CostTotal)
	return s
}

// CalcContractTotal 合同总额 = 设备总额 + 安装费 + 运输费 + 其他费用 − 折让。
// 折让为正数表示让利；结果允许为负（零元赠送、倒贴安装等真实场景），不做下限截断。
func CalcContractTotal(itemsTotal, installFee, freightFee, otherFee, discount float64) float64 {
	return RoundMoney(itemsTotal + installFee + freightFee + otherFee - discount)
}

// ---------------------------------------------------------------------------
// 4.5 盈利口径（两层：设备毛利 / 项目净利）
// ---------------------------------------------------------------------------

// EngProfit 项目盈利指标
type EngProfit struct {
	GrossProfit     float64 // 设备毛利 = 合同额 − 设备成本
	NetProfit       float64 // 项目净利 = 合同额 − 设备成本 − 本项目费用
	GrossMarginRate float64 // 设备毛利率 %
	NetMarginRate   float64 // 净利率 %
}

// CalcProfitability 一次算出四层口径，避免各处各算一遍导致数字互相打架。
// 合同额为 0 时两个毛利率均返回 0（不 panic、不产生 NaN）。
func CalcProfitability(contractTotal, costTotal, expenseTotal float64) EngProfit {
	contractTotal = RoundMoney(contractTotal)
	costTotal = RoundMoney(costTotal)
	expenseTotal = RoundMoney(expenseTotal)

	p := EngProfit{
		GrossProfit: RoundMoney(contractTotal - costTotal),
		NetProfit:   RoundMoney(contractTotal - costTotal - expenseTotal),
	}
	if cents(contractTotal) != 0 {
		p.GrossMarginRate = RoundMoney(p.GrossProfit / contractTotal * 100)
		p.NetMarginRate = RoundMoney(p.NetProfit / contractTotal * 100)
	}
	return p
}

// CalcGrossProfit 设备毛利（只减进货成本，与零售毛利同算法）
func CalcGrossProfit(contractTotal, costTotal float64) float64 {
	return RoundMoney(RoundMoney(contractTotal) - RoundMoney(costTotal))
}

// CalcNetProfit 项目净利（老板看的那个数：再减本项目实际发生的工程费用）
func CalcNetProfit(contractTotal, costTotal, expenseTotal float64) float64 {
	return RoundMoney(RoundMoney(contractTotal) - RoundMoney(costTotal) - RoundMoney(expenseTotal))
}

// CalcEquipMarginRate 设备毛利率 %；合同额为 0 时返回 0
func CalcEquipMarginRate(contractTotal, costTotal float64) float64 {
	return CalcProfitability(contractTotal, costTotal, 0).GrossMarginRate
}

// CalcNetMarginRate 净利率 %；合同额为 0 时返回 0
func CalcNetMarginRate(contractTotal, costTotal, expenseTotal float64) float64 {
	return CalcProfitability(contractTotal, costTotal, expenseTotal).NetMarginRate
}

// CalcAggregateMarginRate 汇总口径的毛利率：Σ利润 / Σ合同额 × 100。
// 汇总卡必须用它（而不是把各项目毛利率再平均）—— 平均值会被小项目拉偏，
// 金额加权才是老板真正想问的"整体赚了几个点"。
func CalcAggregateMarginRate(profitTotal, contractTotal float64) float64 {
	if cents(RoundMoney(contractTotal)) == 0 {
		return 0
	}
	return RoundMoney(RoundMoney(profitTotal) / RoundMoney(contractTotal) * 100)
}

// ---------------------------------------------------------------------------
// 4.3 合同额 = 节点合计（三道强制点共用的同一道校验）
// ---------------------------------------------------------------------------

// EngTermPlan 付款节点计划（校验用最小字段集）
type EngTermPlan struct {
	Name          string
	PlannedAmount float64
	PlannedDate   *time.Time
}

// SumPlannedAmount 节点计划金额合计
func SumPlannedAmount(terms []EngTermPlan) float64 {
	var sum float64
	for _, t := range terms {
		sum += RoundMoney(t.PlannedAmount)
	}
	return RoundMoney(sum)
}

// ValidateContractBalance 校验「节点合计 == 合同总额」。
//
// 用"分"为单位比较，差额文案带具体金额，让用户知道差多少、怎么补。
// 调用点（任何一个漏掉，就会出现"合同改了、节点没改"的脏数据）：
//  1. PUT /projects/:id/items（改清单导致合同额变化）
//  2. PUT /projects/:id/payment-terms（改节点）
//  3. PUT /projects/:id（改安装费/运输费/其他/折让）
//  4. PUT /projects/:id/status 的 sign 动作（最终把关）
func ValidateContractBalance(contractTotal float64, terms []EngTermPlan) error {
	sum := SumPlannedAmount(terms)
	if cents(sum) == cents(RoundMoney(contractTotal)) {
		return nil
	}
	diff := RoundMoney(sum - RoundMoney(contractTotal))
	dir := "多"
	if diff < 0 {
		dir = "少"
	}
	return fmt.Errorf("付款节点合计 %s 与合同总额 %s 不一致，%s了 %s",
		FormatMoneyCN(sum), FormatMoneyCN(contractTotal), dir, FormatMoneyCN(math.Abs(diff)))
}

// ---------------------------------------------------------------------------
// 3 状态机
// ---------------------------------------------------------------------------

// transitionTable 状态转移矩阵（技术方案 §3.1，唯一事实来源）。
// key = 当前状态，value = 动作 → 目标状态。
var transitionTable = map[int]map[EngStatusAction]int{
	EngStatusFollowing: {
		EngActionSign: EngStatusSigned,
		EngActionVoid: EngStatusVoided,
	},
	EngStatusSigned: {
		EngActionRevert: EngStatusFollowing,
		EngActionVoid:   EngStatusVoided,
	},
	EngStatusSupplying: {
		EngActionRevert: EngStatusSigned,
		EngActionAccept: EngStatusAccepted,
		EngActionVoid:   EngStatusVoided,
	},
	EngStatusAccepted: {
		EngActionVoid: EngStatusVoided, // 4 → 5 由回款自动触发，不开放手工 settle
	},
	EngStatusSettled: {}, // 已结清：不允许任何流转（含作废）
	EngStatusVoided:  {}, // 已作废：终态
}

// NextStatus 返回目标状态；不允许流转时返回 -1。
func NextStatus(from int, action EngStatusAction) int {
	if m, ok := transitionTable[from]; ok {
		if to, ok := m[action]; ok {
			return to
		}
	}
	return -1
}

// CanTransit 校验流转是否允许，返回可直接展示给用户的中文原因。
func CanTransit(from int, action EngStatusAction) error {
	if NextStatus(from, action) > 0 {
		return nil
	}
	cur := EngStatusLabel(from)
	switch action {
	case EngActionSign:
		return fmt.Errorf("只有「跟进中」的项目可以签约，当前是「%s」", cur)
	case EngActionAccept:
		return fmt.Errorf("只有「供货中」的项目可以验收，当前是「%s」", cur)
	case EngActionRevert:
		if from == EngStatusSettled {
			return errors.New("已结清的项目不能回退，如需调整请先作废后重新建档")
		}
		if from == EngStatusVoided {
			return errors.New("已作废的项目不能回退")
		}
		return fmt.Errorf("「%s」状态不支持回退", cur)
	case EngActionVoid:
		if from == EngStatusSettled {
			return errors.New("已结清的项目不允许作废")
		}
		return fmt.Errorf("「%s」状态不允许作废", cur)
	default:
		return fmt.Errorf("不支持的操作：%s", string(action))
	}
}

// EngSignGuard 签约前置条件（全部满足才允许）
type EngSignGuard struct {
	ItemCount     int        // 清单行数
	ContractTotal float64    // 合同总额
	TermCount     int        // 节点数
	TermPlanned   float64    // 节点计划金额合计
	SignDate      *time.Time // 签约日期
}

// CheckSignGuard 校验签约条件，顺序与用户填写顺序一致，报错指向"下一步该填什么"。
func CheckSignGuard(g EngSignGuard) error {
	if g.ItemCount <= 0 {
		return errors.New("请先录入设备清单，再签约")
	}
	if g.ContractTotal <= 0 {
		return fmt.Errorf("合同总额为 %s，无法签约，请检查清单单价与费用项", FormatMoneyCN(RoundMoney(g.ContractTotal)))
	}
	if g.TermCount <= 0 {
		return errors.New("请先设置付款节点，再签约")
	}
	if cents(RoundMoney(g.TermPlanned)) != cents(RoundMoney(g.ContractTotal)) {
		diff := RoundMoney(RoundMoney(g.TermPlanned) - RoundMoney(g.ContractTotal))
		dir := "多"
		if diff < 0 {
			dir = "少"
		}
		return fmt.Errorf("付款节点合计 %s 与合同总额 %s 不一致，%s了 %s，签约前必须调平",
			FormatMoneyCN(g.TermPlanned), FormatMoneyCN(g.ContractTotal), dir, FormatMoneyCN(math.Abs(diff)))
	}
	if g.SignDate == nil {
		return errors.New("请填写签约日期")
	}
	return nil
}

// EngAcceptGuard 验收前置条件
type EngAcceptGuard struct {
	OrderedQty int
	ShippedQty int
	AcceptDate *time.Time
}

// CheckAcceptGuard 验收：必须全部发完货并填写验收日期。
func CheckAcceptGuard(g EngAcceptGuard) error {
	if g.OrderedQty <= 0 {
		return errors.New("该项目没有设备清单，无法验收")
	}
	if g.ShippedQty < g.OrderedQty {
		return fmt.Errorf("还有 %d 台未出库（共 %d 台，已发 %d 台），全部发货后才能验收",
			g.OrderedQty-g.ShippedQty, g.OrderedQty, g.ShippedQty)
	}
	if g.AcceptDate == nil {
		return errors.New("请填写验收日期")
	}
	return nil
}

// CheckVoidGuard 作废：原因至少 5 个字，已结清不允许作废。
func CheckVoidGuard(status int, reason string) error {
	if status == EngStatusSettled {
		return errors.New("已结清的项目不允许作废")
	}
	if status == EngStatusVoided {
		return errors.New("该项目已作废，无需重复操作")
	}
	if len([]rune(strings.TrimSpace(reason))) < 5 {
		return errors.New("作废原因至少填 5 个字，便于日后追溯")
	}
	return nil
}

// ---------------------------------------------------------------------------
// 4.4 逾期判定（派生，不落库）
// ---------------------------------------------------------------------------

// EngTermDue 参与逾期计算的节点
type EngTermDue struct {
	PlannedAmount  float64
	ReceivedAmount float64
	PlannedDate    *time.Time
}

// EngOverdue 逾期汇总
type EngOverdue struct {
	Amount float64 // 逾期未收金额
	Days   int     // 最大逾期天数
	Count  int     // 逾期节点数
}

// CalcOverdue 逾期 = 计划金额 > 已收金额 且 计划回款日 < 今天。
// 已收满的节点不算逾期（哪怕日期已过），这是"客户提前付清"的正常情况。
func CalcOverdue(terms []EngTermDue, today time.Time) EngOverdue {
	var r EngOverdue
	t := dateOnly(today)
	for _, term := range terms {
		if term.PlannedDate == nil {
			continue
		}
		planned := RoundMoney(term.PlannedAmount)
		received := RoundMoney(term.ReceivedAmount)
		if planned-received < moneyEpsilon { // 已收满
			continue
		}
		pd := dateOnly(*term.PlannedDate)
		if !pd.Before(t) { // 未到期
			continue
		}
		r.Amount = RoundMoney(r.Amount + (planned - received))
		r.Count++
		if d := int(t.Sub(pd).Hours() / 24); d > r.Days {
			r.Days = d
		}
	}
	return r
}

// ---------------------------------------------------------------------------
// 回款与出库的数量/金额闸门
// ---------------------------------------------------------------------------

// CheckReceiptAmount 回款登记：不允许把单个节点收超（收多了要么改节点计划，要么挂到下个节点）。
// 冲销传负数金额时同样走这里：负数不会超上限，但会校验冲销额度（见 CheckReverseAmount）。
func CheckReceiptAmount(termPlanned, termReceived, amount float64) error {
	if amount <= 0 {
		return errors.New("回款金额必须大于 0")
	}
	remain := RoundMoney(RoundMoney(termPlanned) - RoundMoney(termReceived))
	if cents(RoundMoney(amount)) > cents(remain) {
		return fmt.Errorf("本次回款 %s 超过该节点剩余应收 %s，请调整金额或先修改节点计划",
			FormatMoneyCN(amount), FormatMoneyCN(remain))
	}
	return nil
}

// CheckReverseAmount 冲销：不能冲得比该节点已收还多（否则节点会变成负数已收）。
func CheckReverseAmount(termReceived, reverseAmount float64) error {
	if reverseAmount <= 0 {
		return errors.New("冲销金额必须大于 0")
	}
	if cents(RoundMoney(reverseAmount)) > cents(RoundMoney(termReceived)) {
		return fmt.Errorf("冲销金额 %s 超过该节点已收 %s，无法冲销",
			FormatMoneyCN(reverseAmount), FormatMoneyCN(termReceived))
	}
	return nil
}

// CheckShipmentQty 出库：不允许超过该行未发数量。
func CheckShipmentQty(orderedQty, shippedQty, qty int) error {
	if qty <= 0 {
		return errors.New("出库数量必须大于 0")
	}
	remain := orderedQty - shippedQty
	if qty > remain {
		return fmt.Errorf("本次出库 %d 台超过该行未发数量 %d 台（订购 %d 台，已发 %d 台）",
			qty, remain, orderedQty, shippedQty)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 金额锁定判定
// ---------------------------------------------------------------------------

// EngAmountLocked 已签约及以后（除作废）不允许随意改动金额结构：
// 合同额、清单、节点、费用项都要走"平账校验"，不能静默改。
func EngAmountLocked(status int) bool {
	return status == EngStatusSigned || status == EngStatusSupplying || status == EngStatusAccepted || status == EngStatusSettled
}

// EngEditable 作废项目只读
func EngEditable(status int) bool {
	return status != EngStatusVoided
}

// ---------------------------------------------------------------------------
// 字典（后端唯一副本，响应里附带 xxx_label 供前端直接展示）
// ---------------------------------------------------------------------------

// EngStatusLabel 状态文案
func EngStatusLabel(status int) string {
	switch status {
	case EngStatusFollowing:
		return "跟进中"
	case EngStatusSigned:
		return "已签约"
	case EngStatusSupplying:
		return "供货中"
	case EngStatusAccepted:
		return "已验收"
	case EngStatusSettled:
		return "已结清"
	case EngStatusVoided:
		return "已作废"
	default:
		return "未知"
	}
}

// EngProjectTypeLabel 项目类型文案
func EngProjectTypeLabel(t int) string {
	switch t {
	case 1:
		return "住宅精装"
	case 2:
		return "商业综合体"
	case 3:
		return "酒店公寓"
	case 4:
		return "政企学校"
	case 5:
		return "其他"
	default:
		return "其他"
	}
}

// EngReceiptMethodLabel 回款方式文案
func EngReceiptMethodLabel(m int) string {
	switch m {
	case 1:
		return "银行转账"
	case 2:
		return "承兑汇票"
	case 3:
		return "现金"
	case 4:
		return "支票"
	case 5:
		return "其他"
	default:
		return "其他"
	}
}

// EngWarehouseLabel 出库仓文案
func EngWarehouseLabel(w int) string {
	switch w {
	case 2:
		return "门店"
	case 3:
		return "云仓"
	default:
		return "未知仓"
	}
}

// EngExpenseCategoryLabel 费用类别文案
func EngExpenseCategoryLabel(c int) string {
	switch c {
	case 1:
		return "安装人工"
	case 2:
		return "运输物流"
	case 3:
		return "辅材耗材"
	case 4:
		return "现场施工"
	case 5:
		return "差旅招待"
	case 6:
		return "外包服务"
	case 7:
		return "其他"
	default:
		return "其他"
	}
}

// EngPayStatusLabel 费用付款状态文案
func EngPayStatusLabel(s int) string {
	if s == 0 {
		return "未付"
	}
	return "已付"
}

// EngPayMethodLabel 费用付款方式文案（仅备查，不产生账户流水）
func EngPayMethodLabel(m int) string {
	switch m {
	case 0:
		return "未指定"
	case 1:
		return "微信"
	case 2:
		return "支付宝"
	case 3:
		return "公户"
	case 4:
		return "现金"
	case 5:
		return "其他"
	default:
		return "未指定"
	}
}

// ---------------------------------------------------------------------------
// 日期工具（业务日期统一 YYYY-MM-DD，由后端格式化，前端不再 slice(0,10)）
// ---------------------------------------------------------------------------

// dateOnly 截断到当天零点（本地时区语义：逾期按自然日比较）
func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// ParseBusinessDate 解析 YYYY-MM-DD；空串返回 nil（不报错，表示"未填"）
func ParseBusinessDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return nil, fmt.Errorf("日期格式应为 YYYY-MM-DD：%s", s)
	}
	return &t, nil
}

// FormatBusinessDate 输出 YYYY-MM-DD；nil 返回 nil（JSON 出 null）
func FormatBusinessDate(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}
