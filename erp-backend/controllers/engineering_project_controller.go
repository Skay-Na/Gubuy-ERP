package controllers

// 工程模块 · 项目主档控制器
//
// 归属任务卡：
//   - T04：ListEngineeringProjects / GetEngineeringSummary（本文件已实现）
//   - T06：CreateEngineeringProject / GetEngineeringProject / UpdateEngineeringProject /
//     DeleteEngineeringProject / UpdateEngineeringProjectStatus
//
// 铁律（docs/tasks/2026-10-29-engineering-module-task-cards.md §0.2 H1）：
// 本文件不得出现对 orders / order_items / financial_logs / accounts 的任何写操作。

import (
	"erp-backend/config"
	"erp-backend/models"
	"erp-backend/services"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---------------------------------------------------------------------------
// 列表（T04）
// ---------------------------------------------------------------------------

// engProjectFilter 列表筛选条件（与契约 §3.2 一一对应）
type engProjectFilter struct {
	Keyword     string
	Statuses    []int
	ProjectType int
	OwnerID     uint
	SignStart   *time.Time
	SignEnd     *time.Time
	OnlyOverdue bool
	OnlyPending bool // 仅返回待回款 > 0 的项目（汇总卡 2 点击 → 筛"待回款>0"）
	IncludeVoid bool
}

// applyEngProjectFilter 统一构造筛选条件。
// 用闭包而非复用同一个 *gorm.DB：GORM 的链式 Statement 在 Scan 之后会残留，
// 复用会导致 count 查询带上 select/order（本项目历史上踩过，务必各自调用一次）。
func applyEngProjectFilter(db *gorm.DB, f engProjectFilter) *gorm.DB {
	db = db.Where("p.deleted_at IS NULL")
	if f.Keyword != "" {
		like := engLikePattern(f.Keyword)
		db = db.Where(`(p.project_no LIKE ? OR p.name LIKE ? OR p.client_name LIKE ? OR p.client_contact LIKE ? OR p.client_phone LIKE ?)`,
			like, like, like, like, like)
	}
	if len(f.Statuses) > 0 {
		db = db.Where("p.status IN ?", f.Statuses)
	} else if !f.IncludeVoid {
		db = db.Where("p.status <> ?", services.EngStatusVoided)
	}
	if f.ProjectType > 0 {
		db = db.Where("p.project_type = ?", f.ProjectType)
	}
	if f.OwnerID > 0 {
		db = db.Where("p.owner_employee_id = ?", f.OwnerID)
	}
	if f.SignStart != nil {
		db = db.Where("p.sign_date >= ?", *f.SignStart)
	}
	if f.SignEnd != nil {
		db = db.Where("p.sign_date <= ?", *f.SignEnd)
	}
	if f.OnlyOverdue {
		// 用 EXISTS 子查询而不是"先拉全量再内存过滤"，后者项目一多必然慢
		db = db.Where(`EXISTS (SELECT 1 FROM engineering_payment_terms t
			WHERE t.project_id = p.id AND t.planned_date IS NOT NULL
			  AND t.planned_date < CURDATE() AND t.planned_amount > t.received_amount)`)
	}
	if f.OnlyPending {
		// 与 /summary 的 pending_amount 同口径：状态 2~4 且合同额 > 已实收
		db = db.Where("p.status IN ? AND p.contract_total > p.received_amount",
			[]int{services.EngStatusSigned, services.EngStatusSupplying, services.EngStatusAccepted})
	}
	return db
}

// engTermAggSubquery 节点的逾期/下次到期聚合子查询（列表用一次 JOIN 批量算，禁止逐行查= N+1）
func engTermAggSubquery() *gorm.DB {
	return config.DB.Table("engineering_payment_terms").
		Select(`project_id,
			SUM(CASE WHEN planned_amount > received_amount AND planned_date IS NOT NULL AND planned_date < CURDATE()
			         THEN planned_amount - received_amount ELSE 0 END) AS overdue_amount,
			MAX(CASE WHEN planned_amount > received_amount AND planned_date IS NOT NULL AND planned_date < CURDATE()
			         THEN DATEDIFF(CURDATE(), planned_date) ELSE 0 END) AS overdue_days,
			MIN(CASE WHEN planned_amount > received_amount AND planned_date IS NOT NULL AND planned_date >= CURDATE()
			         THEN planned_date END) AS next_due_date`).
		Group("project_id")
}

// engProjectListRow 列表查询原始行（派生字段在 DTO 层算，保证口径来自 services 唯一入口）
type engProjectListRow struct {
	ID              uint       `gorm:"column:id"`
	ProjectNo       string     `gorm:"column:project_no"`
	Name            string     `gorm:"column:name"`
	ProjectType     int        `gorm:"column:project_type"`
	ClientName      string     `gorm:"column:client_name"`
	ClientContact   string     `gorm:"column:client_contact"`
	ClientPhone     string     `gorm:"column:client_phone"`
	OwnerEmployeeID *uint      `gorm:"column:owner_employee_id"`
	OwnerName       string     `gorm:"column:owner_name"`
	ContractTotal   float64    `gorm:"column:contract_total"`
	EstimatedAmount float64    `gorm:"column:estimated_amount"`
	CostTotal       float64    `gorm:"column:cost_total"`
	ExpenseTotal    float64    `gorm:"column:expense_total"`
	ReceivedAmount  float64    `gorm:"column:received_amount"`
	Status          int        `gorm:"column:status"`
	OrderedQty      int        `gorm:"column:ordered_qty"`
	ShippedQty      int        `gorm:"column:shipped_qty"`
	SignDate        *time.Time `gorm:"column:sign_date"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
	OverdueAmount   float64    `gorm:"column:overdue_amount"`
	OverdueDays     int        `gorm:"column:overdue_days"`
	NextDueDate     *time.Time `gorm:"column:next_due_date"`
}

// engProjectOrderBy 排序白名单（禁止把 query 参数直接拼进 ORDER BY）
func engProjectOrderBy(sortBy, order string) string {
	dir := "DESC"
	if strings.EqualFold(order, "asc") {
		dir = "ASC"
	}
	expr := "p.updated_at"
	switch sortBy {
	case "contract_total":
		expr = "p.contract_total"
	case "pending_amount":
		expr = "(p.contract_total - p.received_amount)"
	case "next_due_date":
		// 没有下次到期日的排最后
		expr = "t.next_due_date IS NULL, t.next_due_date"
	case "net_margin_rate":
		expr = "CASE WHEN p.contract_total = 0 THEN 0 ELSE (p.contract_total - p.cost_total - p.expense_total) / p.contract_total END"
	}
	return expr + " " + dir
}

// ListEngineeringProjects 工程列表 GET /api/admin/engineering/projects（T04）
// @Summary 工程列表（筛选 + 排序 + 分页）
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param keyword query string false "关键词：项目编号/项目名/甲方单位"
// @Param status query int false "项目状态，可重复传多个（1 跟进中 2 已签约 3 供货中 4 已验收 5 已结算 9 已作废）"
// @Param project_type query int false "项目类型（1 家用 2 商用 3 工程 4 政企）"
// @Param owner_id query int false "负责人员工 ID"
// @Param only_overdue query bool false "只看有逾期节点的项目"
// @Param only_pending query bool false "只看待回款>0 的项目（与汇总卡同口径）"
// @Param include_void query bool false "是否包含已作废项目"
// @Param sign_start query string false "签约起始日期 YYYY-MM-DD"
// @Param sign_end query string false "签约截止日期 YYYY-MM-DD"
// @Param sort_by query string false "排序字段（created_at/contract_total/pending_amount/next_due_date 等）"
// @Param order query string false "排序方向 asc/desc"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID list_engineering_projects
// @Router /api/admin/engineering/projects [get]
func ListEngineeringProjects(c *gin.Context) {
	page, pageSize := engPageParams(c)

	f := engProjectFilter{
		Keyword:     engTrimKeyword(c.Query("keyword")),
		Statuses:    engIntListQuery(c, "status"),
		ProjectType: engIntQuery(c, "project_type"),
		OwnerID:     uint(engIntQuery(c, "owner_id")),
		OnlyOverdue: engBoolQuery(c, "only_overdue"),
		OnlyPending: engBoolQuery(c, "only_pending"),
		IncludeVoid: engBoolQuery(c, "include_void"),
	}
	var err error
	if f.SignStart, err = services.ParseBusinessDate(c.Query("sign_start")); err != nil {
		FailBadRequest(c, "签约起始日期格式错误，应为 YYYY-MM-DD")
		return
	}
	if f.SignEnd, err = services.ParseBusinessDate(c.Query("sign_end")); err != nil {
		FailBadRequest(c, "签约截止日期格式错误，应为 YYYY-MM-DD")
		return
	}

	var total int64
	if err := applyEngProjectFilter(config.DB.Table("engineering_projects AS p"), f).Count(&total).Error; err != nil {
		FailInternal(c, "工程列表总数查询失败："+err.Error())
		return
	}

	rows := make([]engProjectListRow, 0, pageSize)
	if total > 0 {
		err := applyEngProjectFilter(config.DB.Table("engineering_projects AS p"), f).
			Select(`p.id, p.project_no, p.name, p.project_type, p.client_name, p.client_contact, p.client_phone,
			        p.owner_employee_id, COALESCE(e.name, '') AS owner_name,
			        p.contract_total, p.estimated_amount, p.cost_total, p.expense_total, p.received_amount,
			        p.status, p.ordered_qty, p.shipped_qty, p.sign_date, p.updated_at,
			        COALESCE(t.overdue_amount, 0) AS overdue_amount,
			        COALESCE(t.overdue_days, 0) AS overdue_days,
			        t.next_due_date`).
			Joins("LEFT JOIN (?) AS t ON t.project_id = p.id", engTermAggSubquery()).
			Joins("LEFT JOIN employees AS e ON e.id = p.owner_employee_id").
			Order(engProjectOrderBy(c.Query("sort_by"), c.Query("order"))).
			Offset((page - 1) * pageSize).Limit(pageSize).
			Scan(&rows).Error
		if err != nil {
			FailInternal(c, "工程列表查询失败："+err.Error())
			return
		}
	}

	list := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		list = append(list, buildEngProjectListItem(r))
	}

	// 顶层 total 必须恒返回（APIResponse.Total 带 omitempty，total=0 时会被省略，
	// 而契约要求列表接口始终有 total，故此处用独立响应结构）
	c.JSON(http.StatusOK, struct {
		Code  int         `json:"code"`
		Msg   string      `json:"msg"`
		Data  interface{} `json:"data"`
		Total int64       `json:"total"`
	}{Code: ErrCodeOK, Msg: "success", Data: gin.H{"list": list}, Total: total})
}

// buildEngProjectListItem 单行 DTO：所有派生金额都过 services 的唯一口径函数
func buildEngProjectListItem(r engProjectListRow) gin.H {
	contract := services.RoundMoney(r.ContractTotal)
	received := services.RoundMoney(r.ReceivedAmount)
	pending := services.RoundMoney(contract - received)
	if pending < 0 {
		pending = 0
	}
	// 已作废：合同额与已收留作历史参考，但「待回款 / 逾期」必须归零。
	// 否则勾选「含已作废」时会看到一笔永远追不回的待回款并标红，与 /summary 口径（只算 1~4）不一致。
	isVoided := r.Status == services.EngStatusVoided
	overdueAmount := services.RoundMoney(r.OverdueAmount)
	overdueDays := r.OverdueDays
	if isVoided {
		pending = 0
		overdueAmount = 0
		overdueDays = 0
	}
	ratio := 0
	if contract > 0 {
		ratio = int(services.RoundMoney(received / contract * 100))
		if ratio > 100 {
			ratio = 100
		}
	}
	p := services.CalcProfitability(contract, r.CostTotal, r.ExpenseTotal)

	return gin.H{
		"id":                 r.ID,
		"project_no":         r.ProjectNo,
		"name":               r.Name,
		"project_type":       r.ProjectType,
		"project_type_label": services.EngProjectTypeLabel(r.ProjectType),
		"client_name":        r.ClientName,
		"client_contact":     r.ClientContact,
		"client_phone":       r.ClientPhone,
		"owner_employee_id":  r.OwnerEmployeeID,
		"owner_name":         r.OwnerName,
		"contract_total":     contract,
		"estimated_amount":   services.RoundMoney(r.EstimatedAmount),
		"cost_total":         services.RoundMoney(r.CostTotal),
		"expense_total":      services.RoundMoney(r.ExpenseTotal),
		"net_profit":         p.NetProfit,
		"net_margin_rate":    p.NetMarginRate,
		"received_amount":    received,
		"pending_amount":     pending,
		"received_ratio":     ratio,
		"status":             r.Status,
		"status_label":       services.EngStatusLabel(r.Status),
		"ordered_qty":        r.OrderedQty,
		"shipped_qty":        r.ShippedQty,
		"overdue_amount":     overdueAmount,
		"overdue_days":       overdueDays,
		"next_due_date":      engDatePtr(r.NextDueDate),
		"sign_date":          engDatePtr(r.SignDate),
		"updated_at":         r.UpdatedAt,
	}
}

// ---------------------------------------------------------------------------
// 汇总卡（T04）
// ---------------------------------------------------------------------------

// engProjectAggRow 项目维度聚合
type engProjectAggRow struct {
	ActiveCount         int64   `gorm:"column:active_count"`
	ActiveAmount        float64 `gorm:"column:active_amount"`
	PendingAmount       float64 `gorm:"column:pending_amount"`
	PendingProjectCount int64   `gorm:"column:pending_project_count"`
	SignedContractTotal float64 `gorm:"column:signed_contract_total"`
	GrossProfit         float64 `gorm:"column:gross_profit"`
	TotalNetProfit      float64 `gorm:"column:total_net_profit"`
}

// engOverdueAggRow 逾期聚合
type engOverdueAggRow struct {
	OverdueAmount    float64 `gorm:"column:overdue_amount"`
	OverdueTermCount int64   `gorm:"column:overdue_term_count"`
}

// engYearReceiptRow 本年回款聚合
type engYearReceiptRow struct {
	YearReceived     float64 `gorm:"column:year_received"`
	YearReceiptCount int64   `gorm:"column:year_receipt_count"`
}

// engYearExpenseRow 本年工程费用聚合（含公共费用）
type engYearExpenseRow struct {
	YearExpense      float64 `gorm:"column:year_expense"`
	YearExpenseCount int64   `gorm:"column:year_expense_count"`
}

// GetEngineeringSummary 汇总卡 GET /api/admin/engineering/summary（T04）
//
// 口径全部在 SQL 里聚合，不把项目拉到内存里循环：
//   - active_*    状态 1/2/3，状态 1 无合同时用预计额兜底
//   - pending_*   状态 1~4 的 (合同额 − 已实收)，负数按 0
//   - overdue_*   未收满且已过期节点（作废项目不计）
//   - year_*      本自然年回款（含冲销负数）/ 本自然年工程费用（含公共费用）
//   - margin      仅状态 2~5，设备毛利与项目净利两套口径
//
// @Summary 工程汇总（6 张汇总卡）
// @Tags 工程管理
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_engineering_summary
// @Router /api/admin/engineering/summary [get]
func GetEngineeringSummary(c *gin.Context) {
	var agg engProjectAggRow
	if err := config.DB.Table("engineering_projects").
		Select(`SUM(CASE WHEN status IN (1,2,3) THEN 1 ELSE 0 END) AS active_count,
		        SUM(CASE WHEN status IN (1,2,3) THEN (CASE WHEN contract_total > 0 THEN contract_total ELSE estimated_amount END) ELSE 0 END) AS active_amount,
		        SUM(CASE WHEN status IN (1,2,3,4) AND contract_total > received_amount THEN contract_total - received_amount ELSE 0 END) AS pending_amount,
		        SUM(CASE WHEN status IN (1,2,3,4) AND contract_total > received_amount THEN 1 ELSE 0 END) AS pending_project_count,
		        SUM(CASE WHEN status IN (2,3,4,5) THEN contract_total ELSE 0 END) AS signed_contract_total,
		        SUM(CASE WHEN status IN (2,3,4,5) THEN contract_total - cost_total ELSE 0 END) AS gross_profit,
		        SUM(CASE WHEN status IN (2,3,4,5) THEN contract_total - cost_total - expense_total ELSE 0 END) AS total_net_profit`).
		Where("deleted_at IS NULL").
		Scan(&agg).Error; err != nil {
		FailInternal(c, "工程汇总查询失败："+err.Error())
		return
	}

	var overdue engOverdueAggRow
	if err := config.DB.Table("engineering_payment_terms AS t").
		Select(`COALESCE(SUM(t.planned_amount - t.received_amount), 0) AS overdue_amount, COUNT(*) AS overdue_term_count`).
		Joins("JOIN engineering_projects AS p ON p.id = t.project_id AND p.deleted_at IS NULL AND p.status <> ?", services.EngStatusVoided).
		Where("t.planned_amount > t.received_amount AND t.planned_date IS NOT NULL AND t.planned_date < CURDATE()").
		Scan(&overdue).Error; err != nil {
		FailInternal(c, "工程逾期汇总失败："+err.Error())
		return
	}

	var receipt engYearReceiptRow
	if err := config.DB.Table("engineering_receipts AS r").
		Select(`COALESCE(SUM(r.amount), 0) AS year_received, COUNT(*) AS year_receipt_count`).
		Joins("JOIN engineering_projects AS p ON p.id = r.project_id AND p.deleted_at IS NULL AND p.status <> ?", services.EngStatusVoided).
		Where("YEAR(r.payment_date) = YEAR(CURDATE())").
		Scan(&receipt).Error; err != nil {
		FailInternal(c, "工程回款汇总失败："+err.Error())
		return
	}

	var expense engYearExpenseRow
	if err := config.DB.Table("engineering_expenses").
		Select(`COALESCE(SUM(amount), 0) AS year_expense, COUNT(*) AS year_expense_count`).
		Where("deleted_at IS NULL AND YEAR(expense_date) = YEAR(CURDATE())").
		Scan(&expense).Error; err != nil {
		FailInternal(c, "工程费用汇总失败："+err.Error())
		return
	}

	// 汇总口径：Σ利润 / Σ合同额（金额加权），不使用"各项目毛利率平均"
	grossRate := services.CalcAggregateMarginRate(agg.GrossProfit, agg.SignedContractTotal)
	netRate := services.CalcAggregateMarginRate(agg.TotalNetProfit, agg.SignedContractTotal)

	OK(c, gin.H{
		"active_count":          agg.ActiveCount,
		"active_amount":         services.RoundMoney(agg.ActiveAmount),
		"pending_amount":        services.RoundMoney(agg.PendingAmount),
		"pending_project_count": agg.PendingProjectCount,
		"overdue_amount":        services.RoundMoney(overdue.OverdueAmount),
		"overdue_term_count":    overdue.OverdueTermCount,
		"year_received":         services.RoundMoney(receipt.YearReceived),
		"year_receipt_count":    receipt.YearReceiptCount,
		"avg_margin_rate":       grossRate,
		"gross_profit":          services.RoundMoney(agg.GrossProfit),
		"year_expense":          services.RoundMoney(expense.YearExpense),
		"year_expense_count":    expense.YearExpenseCount,
		"avg_net_margin_rate":   netRate,
		"total_net_profit":      services.RoundMoney(agg.TotalNetProfit),
	})
}

// ---------------------------------------------------------------------------
// 主档 CRUD（T06）
// ---------------------------------------------------------------------------

// engPhoneRe 联系电话：手机号或座机（契约 §3.3）
var engPhoneRe = regexp.MustCompile(`^(1[3-9]\d{9}|0\d{2,3}-?\d{7,8})$`)

// engProjectCreateReq 建档请求（必填项按契约校验）
type engProjectCreateReq struct {
	ProjectNo       string   `json:"project_no"`
	Name            string   `json:"name"`
	ProjectType     int      `json:"project_type"`
	ClientName      string   `json:"client_name"`
	ClientContact   string   `json:"client_contact"`
	ClientPhone     string   `json:"client_phone"`
	Address         string   `json:"address"`
	OwnerEmployeeID *uint    `json:"owner_employee_id"`
	ContractNo      string   `json:"contract_no"`
	EstimatedAmount *float64 `json:"estimated_amount"`
	SignDate        *string  `json:"sign_date"`
	Remark          string   `json:"remark"`
	ClientRequestID *string  `json:"client_request_id"`
}

// validate 逐字段校验，报错文案直接可展示（指明是哪个字段）
func (r *engProjectCreateReq) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	r.ClientName = strings.TrimSpace(r.ClientName)
	r.ClientContact = strings.TrimSpace(r.ClientContact)
	r.ClientPhone = strings.TrimSpace(r.ClientPhone)

	if engRuneLen(r.Name) == 0 || engRuneLen(r.Name) > 100 {
		return errors.New("项目名称需在 1~100 字之间")
	}
	if r.ProjectType < 1 || r.ProjectType > 5 {
		return errors.New("项目类型不正确")
	}
	if engRuneLen(r.ClientName) == 0 || engRuneLen(r.ClientName) > 150 {
		return errors.New("甲方单位需在 1~150 字之间")
	}
	if engRuneLen(r.ClientContact) == 0 || engRuneLen(r.ClientContact) > 50 {
		return errors.New("甲方联系人需在 1~50 字之间")
	}
	if !engPhoneRe.MatchString(r.ClientPhone) {
		return errors.New("联系电话格式不正确")
	}
	if engRuneLen(r.Address) > 255 {
		return errors.New("项目地址不能超过 255 字")
	}
	if engRuneLen(r.ContractNo) > 50 {
		return errors.New("甲方合同号不能超过 50 字")
	}
	if engRuneLen(r.Remark) > 500 {
		return errors.New("备注不能超过 500 字")
	}
	if r.OwnerEmployeeID == nil || *r.OwnerEmployeeID == 0 {
		return errors.New("请选择项目负责人")
	}
	if r.EstimatedAmount != nil && *r.EstimatedAmount < 0 {
		return errors.New("预计合同额不能为负数")
	}
	if r.ClientRequestID != nil && engRuneLen(*r.ClientRequestID) > 64 {
		return errors.New("幂等键不能超过 64 字")
	}
	return nil
}

// engProjectUpdateReq 更新请求：全部指针，"传入即覆盖、未传保持不变"
type engProjectUpdateReq struct {
	Name            *string  `json:"name"`
	ProjectType     *int     `json:"project_type"`
	ClientName      *string  `json:"client_name"`
	ClientContact   *string  `json:"client_contact"`
	ClientPhone     *string  `json:"client_phone"`
	Address         *string  `json:"address"`
	OwnerEmployeeID *uint    `json:"owner_employee_id"`
	ContractNo      *string  `json:"contract_no"`
	EstimatedAmount *float64 `json:"estimated_amount"`
	SignDate        *string  `json:"sign_date"`
	Remark          *string  `json:"remark"`
	InstallFee      *float64 `json:"install_fee"`
	FreightFee      *float64 `json:"freight_fee"`
	OtherFee        *float64 `json:"other_fee"`
	DiscountAmount  *float64 `json:"discount_amount"`
}

// engStatusReq 状态流转请求
type engStatusReq struct {
	Action     string  `json:"action"`
	SignDate   *string `json:"sign_date"`
	AcceptDate *string `json:"accept_date"`
	ContractNo *string `json:"contract_no"`
	Reason     string  `json:"reason"`
}

// engGenerateProjectNo 生成当日流水号 GC-YYYYMMDD-NNN。
// 刻意不用 MAX(id)+1：并发下必撞号；这里取当日已有最大编号 +1，唯一索引兜底重试。
func engGenerateProjectNo() (string, error) {
	prefix := "GC-" + time.Now().Format("20060102") + "-"
	var maxNo string
	if err := config.DB.Model(&models.EngineeringProject{}).Unscoped().
		Where("project_no LIKE ?", prefix+"%").
		Select("COALESCE(MAX(project_no), '')").
		Scan(&maxNo).Error; err != nil {
		return "", err
	}
	seq := 1
	if len(maxNo) > len(prefix) {
		if n, err := strconv.Atoi(maxNo[len(prefix):]); err == nil {
			seq = n + 1
		}
	}
	return fmt.Sprintf("%s%03d", prefix, seq), nil
}

// engIsDuplicateErr MySQL 1062 唯一键冲突
func engIsDuplicateErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Duplicate entry") || strings.Contains(err.Error(), "1062")
}

// CreateEngineeringProject 建档 POST /api/admin/engineering/projects（T06）
// @Summary 创建工程项目
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param body body engProjectCreateReq true "创建参数（client_request_id 幂等）"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID create_engineering_project
// @Router /api/admin/engineering/projects [post]
func CreateEngineeringProject(c *gin.Context) {
	var req engProjectCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数格式错误："+err.Error())
		return
	}
	if err := req.validate(); err != nil {
		FailBadRequest(c, err.Error())
		return
	}

	// 幂等：同一 client_request_id 重复提交只建一单
	if req.ClientRequestID != nil && strings.TrimSpace(*req.ClientRequestID) != "" {
		var existed models.EngineeringProject
		if err := config.DB.Where("client_request_id = ?", *req.ClientRequestID).First(&existed).Error; err == nil {
			c.JSON(http.StatusConflict, APIResponse{
				Code: ErrDuplicated,
				Msg:  "该项目已创建，请勿重复提交",
				Data: gin.H{"id": existed.ID, "project_no": existed.ProjectNo},
			})
			return
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			FailInternal(c, "幂等校验失败："+err.Error())
			return
		}
	}

	// 负责人必须存在且未删除
	var owner models.Employee
	if err := config.DB.Select("id", "name").First(&owner, *req.OwnerEmployeeID).Error; err != nil {
		FailBadRequest(c, "负责人不存在，请重新选择")
		return
	}

	estimated := 0.0
	if req.EstimatedAmount != nil {
		estimated = services.RoundMoney(*req.EstimatedAmount)
	}
	signDate, err := services.ParseBusinessDate(derefString(req.SignDate))
	if err != nil {
		FailBadRequest(c, "签约日期格式错误，应为 YYYY-MM-DD")
		return
	}
	if signDate != nil && signDate.After(engToday()) {
		FailBadRequest(c, "签约日期不能晚于今天")
		return
	}

	// 显式传入编号时校验唯一并直接返回 409；自动生成时撞号重试 3 次
	explicitNo := strings.TrimSpace(req.ProjectNo) != ""
	if explicitNo {
		if engRuneLen(req.ProjectNo) > 50 {
			FailBadRequest(c, "项目编号不能超过 50 字")
			return
		}
		var cnt int64
		if err := config.DB.Model(&models.EngineeringProject{}).Unscoped().
			Where("project_no = ?", req.ProjectNo).Count(&cnt).Error; err != nil {
			FailInternal(c, "项目编号校验失败："+err.Error())
			return
		}
		if cnt > 0 {
			FailDuplicated(c, "项目编号已被占用")
			return
		}
	}

	project := models.EngineeringProject{
		ProjectNo:       strings.TrimSpace(req.ProjectNo),
		Name:            req.Name,
		ProjectType:     req.ProjectType,
		ClientName:      req.ClientName,
		ClientContact:   req.ClientContact,
		ClientPhone:     req.ClientPhone,
		Address:         req.Address,
		OwnerEmployeeID: req.OwnerEmployeeID,
		ContractNo:      strings.TrimSpace(req.ContractNo),
		SignDate:        signDate,
		EstimatedAmount: estimated,
		Remark:          strings.TrimSpace(req.Remark),
		Status:          services.EngStatusFollowing,
		ClientRequestID: req.ClientRequestID,
	}
	// 费用项默认 0，合同额 = 0 + 0 + 0 + 0 − 0
	project.ContractTotal = services.CalcContractTotal(0, 0, 0, 0, 0)

	var createErr error
	for attempt := 0; attempt < 3; attempt++ {
		if project.ProjectNo == "" {
			no, err := engGenerateProjectNo()
			if err != nil {
				FailInternal(c, "项目编号生成失败，请重试")
				return
			}
			project.ProjectNo = no
		}
		createErr = config.DB.Create(&project).Error
		if createErr == nil {
			break
		}
		if !engIsDuplicateErr(createErr) {
			FailInternal(c, "建档失败："+createErr.Error())
			return
		}
		if explicitNo {
			FailDuplicated(c, "项目编号已被占用")
			return
		}
		project.ProjectNo = "" // 撞号：重新取号再试
	}
	if createErr != nil {
		FailInternal(c, "项目编号生成失败，请重试")
		return
	}

	OKWithMsg(c, "项目已创建", gin.H{"id": project.ID, "project_no": project.ProjectNo})
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// GetEngineeringProject 详情 GET /api/admin/engineering/projects/:id（T06）
//
// 一次返回主档 + 清单 + 节点 + 回款 + 出库 + 费用（本项目）+ 费用汇总，共 7 次单表查询，
// 不存在逐行查询（无 N+1）。公共费用刻意不返回：项目净利不含公共费用。
// @Summary 工程详情
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param id path int true "项目 ID"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_engineering_project
// @Router /api/admin/engineering/projects/{id} [get]
func GetEngineeringProject(c *gin.Context) {
	id, ok := engIDParam(c, "id")
	if !ok {
		return
	}

	var row struct {
		models.EngineeringProject
		OwnerName string `gorm:"column:owner_name"`
	}
	if err := config.DB.Table("engineering_projects AS p").
		Select("p.*, COALESCE(e.name, '') AS owner_name").
		Joins("LEFT JOIN employees AS e ON e.id = p.owner_employee_id").
		Where("p.id = ? AND p.deleted_at IS NULL", id).
		Scan(&row).Error; err != nil {
		FailInternal(c, "项目查询失败："+err.Error())
		return
	}
	if row.ID == 0 {
		FailNotFound(c, "项目不存在")
		return
	}
	p := row.EngineeringProject

	// 清单
	type itemRow struct {
		ID          uint    `gorm:"column:id"`
		ProductID   uint    `gorm:"column:product_id"`
		ProductName string  `gorm:"column:product_name"`
		ProductSKU  string  `gorm:"column:product_sku"`
		Quantity    int     `gorm:"column:quantity"`
		UnitPrice   float64 `gorm:"column:unit_price"`
		UnitCost    float64 `gorm:"column:unit_cost"`
		ShippedQty  int     `gorm:"column:shipped_qty"`
		Remark      string  `gorm:"column:remark"`
		SortOrder   int     `gorm:"column:sort_order"`
	}
	var itemRows []itemRow
	if err := config.DB.Table("engineering_project_items AS i").
		Select(`i.id, i.product_id, COALESCE(pr.name, '') AS product_name, COALESCE(pr.sku, '') AS product_sku,
		        i.quantity, i.unit_price, i.unit_cost, i.shipped_qty, i.remark, i.sort_order`).
		Joins("LEFT JOIN products AS pr ON pr.id = i.product_id").
		Where("i.project_id = ?", id).
		Order("i.sort_order ASC, i.id ASC").
		Scan(&itemRows).Error; err != nil {
		FailInternal(c, "设备清单查询失败："+err.Error())
		return
	}

	// 付款节点（按计划日排序，未填日期的排最后）
	var terms []models.EngineeringPaymentTerm
	if err := config.DB.Where("project_id = ?", id).
		Order("planned_date IS NULL, planned_date ASC, sort_order ASC, id ASC").
		Find(&terms).Error; err != nil {
		FailInternal(c, "付款节点查询失败："+err.Error())
		return
	}

	// 回款（带节点名）
	type receiptRow struct {
		ID                uint      `gorm:"column:id"`
		TermID            uint      `gorm:"column:term_id"`
		TermName          string    `gorm:"column:term_name"`
		Amount            float64   `gorm:"column:amount"`
		PaymentDate       time.Time `gorm:"column:payment_date"`
		Method            int       `gorm:"column:method"`
		BillNo            string    `gorm:"column:bill_no"`
		Remark            string    `gorm:"column:remark"`
		ReversedReceiptID *uint     `gorm:"column:reversed_receipt_id"`
		OperatorName      string    `gorm:"column:operator_name"`
		CreatedAt         time.Time `gorm:"column:created_at"`
	}
	var receiptRows []receiptRow
	if err := config.DB.Table("engineering_receipts AS r").
		Select(`r.id, r.term_id, COALESCE(t.name, '') AS term_name, r.amount, r.payment_date, r.method,
		        r.bill_no, r.remark, r.reversed_receipt_id, r.operator_name, r.created_at`).
		Joins("LEFT JOIN engineering_payment_terms AS t ON t.id = r.term_id").
		Where("r.project_id = ?", id).
		Order("r.payment_date DESC, r.id DESC").
		Scan(&receiptRows).Error; err != nil {
		FailInternal(c, "回款记录查询失败："+err.Error())
		return
	}

	// 出库（带商品名）
	type shipmentRow struct {
		ID            uint      `gorm:"column:id"`
		ItemID        uint      `gorm:"column:item_id"`
		ProductName   string    `gorm:"column:product_name"`
		Quantity      int       `gorm:"column:quantity"`
		WarehouseType int       `gorm:"column:warehouse_type"`
		ShipDate      time.Time `gorm:"column:ship_date"`
		TrackingNo    string    `gorm:"column:tracking_no"`
		Remark        string    `gorm:"column:remark"`
		AllowNegative bool      `gorm:"column:allow_negative"`
		OperatorName  string    `gorm:"column:operator_name"`
		CreatedAt     time.Time `gorm:"column:created_at"`
	}
	var shipmentRows []shipmentRow
	if err := config.DB.Table("engineering_shipments AS s").
		Select(`s.id, s.item_id, COALESCE(pr.name, '') AS product_name, s.quantity, s.warehouse_type,
		        s.ship_date, s.tracking_no, s.remark, s.allow_negative, s.operator_name, s.created_at`).
		Joins("LEFT JOIN products AS pr ON pr.id = s.product_id").
		Where("s.project_id = ?", id).
		Order("s.ship_date DESC, s.id DESC").
		Scan(&shipmentRows).Error; err != nil {
		FailInternal(c, "出库记录查询失败："+err.Error())
		return
	}

	// 费用（只含本项目，公共费用不进来）
	var expenses []models.EngineeringExpense
	if err := config.DB.Where("project_id = ?", id).
		Order("expense_date DESC, id DESC").Find(&expenses).Error; err != nil {
		FailInternal(c, "工程费用查询失败："+err.Error())
		return
	}

	today := engToday()
	profit := services.CalcProfitability(p.ContractTotal, p.CostTotal, p.ExpenseTotal)
	overdue := services.CalcOverdue(termsToDue(terms), today)

	// 清单 DTO
	itemsDTO := make([]gin.H, 0, len(itemRows))
	for _, it := range itemRows {
		amount := services.RoundMoney(float64(it.Quantity) * it.UnitPrice)
		costAmount := services.RoundMoney(float64(it.Quantity) * it.UnitCost)
		itemsDTO = append(itemsDTO, gin.H{
			"id":           it.ID,
			"product_id":   it.ProductID,
			"product_name": it.ProductName,
			"product_sku":  it.ProductSKU,
			"quantity":     it.Quantity,
			"unit_price":   services.RoundMoney(it.UnitPrice),
			"unit_cost":    services.RoundMoney(it.UnitCost),
			"amount":       amount,
			"cost_amount":  costAmount,
			"gross_profit": services.RoundMoney(amount - costAmount),
			"margin_rate":  services.CalcAggregateMarginRate(amount-costAmount, amount),
			"shipped_qty":  it.ShippedQty,
			"remark":       it.Remark,
			"sort_order":   it.SortOrder,
			"cost_missing": it.UnitCost <= 0,
		})
	}

	// 节点 DTO
	termsDTO := make([]gin.H, 0, len(terms))
	for _, t := range terms {
		planned := services.RoundMoney(t.PlannedAmount)
		received := services.RoundMoney(t.ReceivedAmount)
		pending := services.RoundMoney(planned - received)
		if pending < 0 {
			pending = 0
		}
		status, label, overdueDays := "unpaid", "未收", 0
		switch {
		case pending <= 0:
			status, label = "paid", "已收"
		case t.PlannedDate != nil && t.PlannedDate.Before(today):
			status, label = "overdue", "逾期"
			overdueDays = int(today.Sub(*t.PlannedDate).Hours() / 24)
			if overdueDays < 0 {
				overdueDays = 0
			}
		case received > 0:
			status, label = "partial", "部分收"
		}
		var ratio interface{}
		if t.Ratio != nil {
			ratio = services.RoundMoney(*t.Ratio)
		}
		termsDTO = append(termsDTO, gin.H{
			"id":              t.ID,
			"name":            t.Name,
			"ratio":           ratio,
			"planned_amount":  planned,
			"planned_date":    engDatePtr(t.PlannedDate),
			"received_amount": received,
			"pending_amount":  pending,
			"status":          status,
			"status_label":    label,
			"overdue_days":    overdueDays,
		})
	}

	// 回款 DTO
	receiptsDTO := make([]gin.H, 0, len(receiptRows))
	for _, r := range receiptRows {
		receiptsDTO = append(receiptsDTO, gin.H{
			"id":                  r.ID,
			"term_id":             r.TermID,
			"term_name":           r.TermName,
			"amount":              services.RoundMoney(r.Amount),
			"payment_date":        r.PaymentDate.Format("2006-01-02"),
			"method":              r.Method,
			"method_label":        services.EngReceiptMethodLabel(r.Method),
			"bill_no":             r.BillNo,
			"remark":              r.Remark,
			"is_reversal":         r.Amount < 0,
			"reversed_receipt_id": r.ReversedReceiptID,
			"operator_name":       r.OperatorName,
			"created_at":          r.CreatedAt,
		})
	}

	// 出库 DTO
	shipmentsDTO := make([]gin.H, 0, len(shipmentRows))
	for _, s := range shipmentRows {
		shipmentsDTO = append(shipmentsDTO, gin.H{
			"id":              s.ID,
			"item_id":         s.ItemID,
			"product_name":    s.ProductName,
			"quantity":        s.Quantity,
			"warehouse_type":  s.WarehouseType,
			"warehouse_label": services.EngWarehouseLabel(s.WarehouseType),
			"ship_date":       s.ShipDate.Format("2006-01-02"),
			"tracking_no":     s.TrackingNo,
			"remark":          s.Remark,
			"allow_negative":  s.AllowNegative,
			"operator_name":   s.OperatorName,
			"created_at":      s.CreatedAt,
		})
	}

	// 费用 DTO + 汇总
	expenseDTO := make([]gin.H, 0, len(expenses))
	var expTotal, expPaid float64
	byCategory := map[int]float64{}
	for _, e := range expenses {
		expTotal += e.Amount
		if e.PayStatus == 1 {
			expPaid += e.Amount
		}
		byCategory[e.Category] += e.Amount
		expenseDTO = append(expenseDTO, gin.H{
			"id":               e.ID,
			"project_id":       e.ProjectID,
			"project_name":     p.Name,
			"category":         e.Category,
			"category_label":   services.EngExpenseCategoryLabel(e.Category),
			"amount":           services.RoundMoney(e.Amount),
			"expense_date":     e.ExpenseDate.Format("2006-01-02"),
			"pay_status":       e.PayStatus,
			"pay_status_label": services.EngPayStatusLabel(e.PayStatus),
			"pay_method":       e.PayMethod,
			"pay_method_label": services.EngPayMethodLabel(e.PayMethod),
			"paid_date":        engDatePtr(e.PaidDate),
			"payee":            e.Payee,
			"bill_no":          e.BillNo,
			"remark":           e.Remark,
			"operator_name":    e.OperatorName,
			"created_at":       e.CreatedAt,
		})
	}
	byCategoryDTO := make([]gin.H, 0, len(services.EngExpenseCategories))
	for _, c := range services.EngExpenseCategories {
		byCategoryDTO = append(byCategoryDTO, gin.H{
			"category":       c,
			"category_label": services.EngExpenseCategoryLabel(c),
			"amount":         services.RoundMoney(byCategory[c]),
		})
	}

	pending := services.RoundMoney(p.ContractTotal - p.ReceivedAmount)
	if pending < 0 {
		pending = 0
	}
	receivedRatio := 0
	if p.ContractTotal > 0 {
		receivedRatio = int(services.RoundMoney(p.ReceivedAmount / p.ContractTotal * 100))
		if receivedRatio > 100 {
			receivedRatio = 100
		}
	}
	shippedRatio := 0
	if p.OrderedQty > 0 {
		shippedRatio = int(services.RoundMoney(float64(p.ShippedQty) / float64(p.OrderedQty) * 100))
		if shippedRatio > 100 {
			shippedRatio = 100
		}
	}

	OK(c, gin.H{
		"id":                 p.ID,
		"project_no":         p.ProjectNo,
		"name":               p.Name,
		"project_type":       p.ProjectType,
		"project_type_label": services.EngProjectTypeLabel(p.ProjectType),
		"client_name":        p.ClientName,
		"client_contact":     p.ClientContact,
		"client_phone":       p.ClientPhone,
		"address":            p.Address,
		"owner_employee_id":  p.OwnerEmployeeID,
		"owner_name":         row.OwnerName,
		"contract_no":        p.ContractNo,
		"sign_date":          engDatePtr(p.SignDate),
		"accept_date":        engDatePtr(p.AcceptDate),
		"settled_at":         engDateTime(p.SettledAt),
		"estimated_amount":   services.RoundMoney(p.EstimatedAmount),
		"items_total":        services.RoundMoney(p.ItemsTotal),
		"install_fee":        services.RoundMoney(p.InstallFee),
		"freight_fee":        services.RoundMoney(p.FreightFee),
		"other_fee":          services.RoundMoney(p.OtherFee),
		"discount_amount":    services.RoundMoney(p.DiscountAmount),
		"contract_total":     services.RoundMoney(p.ContractTotal),
		"cost_total":         services.RoundMoney(p.CostTotal),
		"expense_total":      services.RoundMoney(p.ExpenseTotal),
		"gross_profit":       profit.GrossProfit,
		"margin_rate":        profit.GrossMarginRate,
		"net_profit":         profit.NetProfit,
		"net_margin_rate":    profit.NetMarginRate,
		"received_amount":    services.RoundMoney(p.ReceivedAmount),
		"pending_amount":     pending,
		"received_ratio":     receivedRatio,
		"ordered_qty":        p.OrderedQty,
		"shipped_qty":        p.ShippedQty,
		"shipped_ratio":      shippedRatio,
		"status":             p.Status,
		"status_label":       services.EngStatusLabel(p.Status),
		"overdue_amount":     overdue.Amount,
		"overdue_term_count": overdue.Count,
		"void_reason":        p.VoidReason,
		"voided_at":          p.VoidedAt,
		"remark":             p.Remark,
		"created_at":         p.CreatedAt,
		"updated_at":         p.UpdatedAt,
		"items":              itemsDTO,
		"terms":              termsDTO,
		"receipts":           receiptsDTO,
		"shipments":          shipmentsDTO,
		"expenses":           expenseDTO,
		"expense_summary": gin.H{
			"total":       services.RoundMoney(expTotal),
			"paid":        services.RoundMoney(expPaid),
			"unpaid":      services.RoundMoney(expTotal - expPaid),
			"count":       len(expenses),
			"by_category": byCategoryDTO,
		},
	})
}

// termsToDue 把节点转成逾期计算入参
func termsToDue(terms []models.EngineeringPaymentTerm) []services.EngTermDue {
	out := make([]services.EngTermDue, 0, len(terms))
	for _, t := range terms {
		out = append(out, services.EngTermDue{
			PlannedAmount:  t.PlannedAmount,
			ReceivedAmount: t.ReceivedAmount,
			PlannedDate:    t.PlannedDate,
		})
	}
	return out
}

// UpdateEngineeringProject 更新主档 PUT /api/admin/engineering/projects/:id（T06）
//
// 只改主档字段；清单、节点、回款、出库、费用一律不在此接口改。
// @Summary 修改工程项目
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param id path int true "项目 ID"
// @Param body body engProjectUpdateReq true "修改参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID update_engineering_project
// @Router /api/admin/engineering/projects/{id} [put]
func UpdateEngineeringProject(c *gin.Context) {
	id, ok := engIDParam(c, "id")
	if !ok {
		return
	}
	var req engProjectUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数格式错误："+err.Error())
		return
	}

	var p models.EngineeringProject
	if err := config.DB.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			FailNotFound(c, "项目不存在")
			return
		}
		FailInternal(c, "项目查询失败："+err.Error())
		return
	}
	if p.Status == services.EngStatusVoided {
		FailBadRequest(c, "项目已作废，不可编辑")
		return
	}

	// 已结清：只允许改联系方式与备注（R10）
	amountTouched := req.InstallFee != nil || req.FreightFee != nil || req.OtherFee != nil ||
		req.DiscountAmount != nil || req.EstimatedAmount != nil
	mainTouched := req.Name != nil || req.ProjectType != nil || req.ClientName != nil ||
		req.Address != nil || req.OwnerEmployeeID != nil || req.ContractNo != nil || req.SignDate != nil
	if p.Status == services.EngStatusSettled && (amountTouched || mainTouched) {
		FailBadRequest(c, "项目已结清，金额字段不可修改")
		return
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if engRuneLen(name) == 0 || engRuneLen(name) > 100 {
			FailBadRequest(c, "项目名称需在 1~100 字之间")
			return
		}
		updates["name"] = name
	}
	if req.ProjectType != nil {
		if *req.ProjectType < 1 || *req.ProjectType > 5 {
			FailBadRequest(c, "项目类型不正确")
			return
		}
		updates["project_type"] = *req.ProjectType
	}
	if req.ClientName != nil {
		v := strings.TrimSpace(*req.ClientName)
		if engRuneLen(v) == 0 || engRuneLen(v) > 150 {
			FailBadRequest(c, "甲方单位需在 1~150 字之间")
			return
		}
		updates["client_name"] = v
	}
	if req.ClientContact != nil {
		v := strings.TrimSpace(*req.ClientContact)
		if engRuneLen(v) == 0 || engRuneLen(v) > 50 {
			FailBadRequest(c, "甲方联系人需在 1~50 字之间")
			return
		}
		updates["client_contact"] = v
	}
	if req.ClientPhone != nil {
		v := strings.TrimSpace(*req.ClientPhone)
		if !engPhoneRe.MatchString(v) {
			FailBadRequest(c, "联系电话格式不正确")
			return
		}
		updates["client_phone"] = v
	}
	if req.Address != nil {
		if engRuneLen(*req.Address) > 255 {
			FailBadRequest(c, "项目地址不能超过 255 字")
			return
		}
		updates["address"] = strings.TrimSpace(*req.Address)
	}
	if req.OwnerEmployeeID != nil {
		var owner models.Employee
		if err := config.DB.Select("id").First(&owner, *req.OwnerEmployeeID).Error; err != nil {
			FailBadRequest(c, "负责人不存在，请重新选择")
			return
		}
		updates["owner_employee_id"] = *req.OwnerEmployeeID
	}
	if req.ContractNo != nil {
		if engRuneLen(*req.ContractNo) > 50 {
			FailBadRequest(c, "甲方合同号不能超过 50 字")
			return
		}
		updates["contract_no"] = strings.TrimSpace(*req.ContractNo)
	}
	if req.Remark != nil {
		if engRuneLen(*req.Remark) > 500 {
			FailBadRequest(c, "备注不能超过 500 字")
			return
		}
		updates["remark"] = strings.TrimSpace(*req.Remark)
	}
	if req.EstimatedAmount != nil {
		if *req.EstimatedAmount < 0 {
			FailBadRequest(c, "预计合同额不能为负数")
			return
		}
		updates["estimated_amount"] = services.RoundMoney(*req.EstimatedAmount)
	}
	if req.SignDate != nil {
		sd, err := services.ParseBusinessDate(*req.SignDate)
		if err != nil {
			FailBadRequest(c, "签约日期格式错误，应为 YYYY-MM-DD")
			return
		}
		if sd != nil && sd.After(engToday()) {
			FailBadRequest(c, "签约日期不能晚于今天")
			return
		}
		updates["sign_date"] = sd
	}
	for name, ptr := range map[string]*float64{
		"install_fee":     req.InstallFee,
		"freight_fee":     req.FreightFee,
		"other_fee":       req.OtherFee,
		"discount_amount": req.DiscountAmount,
	} {
		if ptr == nil {
			continue
		}
		if *ptr < 0 {
			FailBadRequest(c, "费用项不能为负数")
			return
		}
		updates[name] = services.RoundMoney(*ptr)
	}

	// 费用项变化会改合同额：已签约及以后必须仍与节点合计相等（第三道强制点）
	install, freight, other, discount := p.InstallFee, p.FreightFee, p.OtherFee, p.DiscountAmount
	if v, ok := updates["install_fee"].(float64); ok {
		install = v
	}
	if v, ok := updates["freight_fee"].(float64); ok {
		freight = v
	}
	if v, ok := updates["other_fee"].(float64); ok {
		other = v
	}
	if v, ok := updates["discount_amount"].(float64); ok {
		discount = v
	}
	newContract := services.CalcContractTotal(p.ItemsTotal, install, freight, other, discount)
	contractChanged := !services.MoneyEqual(newContract, p.ContractTotal)

	if contractChanged && services.EngAmountLocked(p.Status) {
		var plans []services.EngTermPlan
		var termRows []models.EngineeringPaymentTerm
		if err := config.DB.Where("project_id = ?", id).Find(&termRows).Error; err != nil {
			FailInternal(c, "付款节点查询失败："+err.Error())
			return
		}
		for _, t := range termRows {
			plans = append(plans, services.EngTermPlan{Name: t.Name, PlannedAmount: t.PlannedAmount, PlannedDate: t.PlannedDate})
		}
		if err := services.ValidateContractBalance(newContract, plans); err != nil {
			Fail(c, http.StatusConflict, ErrDuplicated,
				fmt.Sprintf("合同总额与付款节点合计相差 %s，请同时调整付款节点",
					services.FormatMoneyCN(services.RoundMoney(newContract-services.SumPlannedAmount(plans)))))
			return
		}
	}
	if contractChanged {
		updates["contract_total"] = newContract
	}

	if len(updates) > 0 {
		if err := config.DB.Model(&p).Updates(updates).Error; err != nil {
			FailInternal(c, "项目更新失败："+err.Error())
			return
		}
	}

	finalContract := p.ContractTotal
	if v, ok := updates["contract_total"].(float64); ok {
		finalContract = v
	}
	pending := services.RoundMoney(finalContract - p.ReceivedAmount)
	if pending < 0 {
		pending = 0
	}
	OKWithMsg(c, "项目已更新", gin.H{
		"id":             p.ID,
		"contract_total": services.RoundMoney(finalContract),
		"pending_amount": pending,
	})
}

// DeleteEngineeringProject 删除 DELETE /api/admin/engineering/projects/:id（T06）
//
// 仅跟进中且无回款、无出库的项目可删；其余提示"请先作废"，避免删掉有账的项目。
// @Summary 删除工程项目（软删）
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param id path int true "项目 ID"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID delete_engineering_project
// @Router /api/admin/engineering/projects/{id} [delete]
func DeleteEngineeringProject(c *gin.Context) {
	id, ok := engIDParam(c, "id")
	if !ok {
		return
	}

	var p models.EngineeringProject
	if err := config.DB.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			FailNotFound(c, "项目不存在")
			return
		}
		FailInternal(c, "项目查询失败："+err.Error())
		return
	}

	if p.Status != services.EngStatusFollowing {
		FailBadRequest(c, "仅跟进中且无回款、无出库的项目可删除，该项目请先作废")
		return
	}
	var receiptCnt, shipmentCnt, expenseCnt int64
	if err := config.DB.Model(&models.EngineeringReceipt{}).Where("project_id = ?", id).Count(&receiptCnt).Error; err != nil {
		FailInternal(c, "回款记录校验失败："+err.Error())
		return
	}
	if err := config.DB.Model(&models.EngineeringShipment{}).Where("project_id = ?", id).Count(&shipmentCnt).Error; err != nil {
		FailInternal(c, "出库记录校验失败："+err.Error())
		return
	}
	if err := config.DB.Model(&models.EngineeringExpense{}).Where("project_id = ?", id).Count(&expenseCnt).Error; err != nil {
		FailInternal(c, "费用记录校验失败："+err.Error())
		return
	}
	if receiptCnt+shipmentCnt+expenseCnt > 0 {
		FailBadRequest(c, "仅跟进中且无回款、无出库的项目可删除，该项目请先作废")
		return
	}

	// 清单与节点跟着主档一起软删（业务上不可分离）
	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", id).Delete(&models.EngineeringProjectItem{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", id).Delete(&models.EngineeringPaymentTerm{}).Error; err != nil {
			return err
		}
		return tx.Delete(&p).Error
	}); err != nil {
		FailInternal(c, "项目删除失败："+err.Error())
		return
	}

	OKWithMsg(c, "项目已删除", gin.H{})
}

// UpdateEngineeringProjectStatus 状态流转 PUT /api/admin/engineering/projects/:id/status（T06）
//
// 全程在一个事务内，先 SELECT ... FOR UPDATE 锁项目行，保证并发下状态只推进一次。
// @Summary 工程状态流转（签约/供货/验收/结算/作废/恢复）
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param id path int true "项目 ID"
// @Param body body engStatusReq true "流转参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID update_engineering_project_status
// @Router /api/admin/engineering/projects/{id}/status [put]
func UpdateEngineeringProjectStatus(c *gin.Context) {
	id, ok := engIDParam(c, "id")
	if !ok {
		return
	}
	var req engStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数格式错误："+err.Error())
		return
	}
	action := services.EngStatusAction(strings.TrimSpace(req.Action))

	var result gin.H
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		var p models.EngineeringProject
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, id).Error; err != nil {
			return err
		}
		if err := services.CanTransit(p.Status, action); err != nil {
			return err
		}

		switch action {
		case services.EngActionSign:
			signDate, err := services.ParseBusinessDate(derefString(req.SignDate))
			if err != nil {
				return errors.New("签约日期格式错误，应为 YYYY-MM-DD")
			}
			var itemCnt int64
			var qtySum struct{ Ordered int }
			if err := tx.Model(&models.EngineeringProjectItem{}).Where("project_id = ?", id).Count(&itemCnt).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.EngineeringProjectItem{}).
				Select("COALESCE(SUM(quantity), 0) AS ordered").
				Where("project_id = ?", id).Scan(&qtySum).Error; err != nil {
				return err
			}
			var termRows []models.EngineeringPaymentTerm
			if err := tx.Where("project_id = ?", id).Find(&termRows).Error; err != nil {
				return err
			}
			termPlanned := 0.0
			for _, t := range termRows {
				termPlanned += t.PlannedAmount
			}
			if err := services.CheckSignGuard(services.EngSignGuard{
				ItemCount:     int(itemCnt),
				ContractTotal: p.ContractTotal,
				TermCount:     len(termRows),
				TermPlanned:   termPlanned,
				SignDate:      signDate,
			}); err != nil {
				return err
			}
			updates := map[string]interface{}{"status": services.EngStatusSigned, "sign_date": signDate}
			if req.ContractNo != nil {
				if engRuneLen(*req.ContractNo) > 50 {
					return errors.New("甲方合同号不能超过 50 字")
				}
				updates["contract_no"] = strings.TrimSpace(*req.ContractNo)
			}
			if err := tx.Model(&p).Updates(updates).Error; err != nil {
				return err
			}
			result = gin.H{
				"id": p.ID, "status": services.EngStatusSigned,
				"status_label": services.EngStatusLabel(services.EngStatusSigned),
				"sign_date":    engDatePtr(signDate),
			}

		case services.EngActionAccept:
			acceptDate, err := services.ParseBusinessDate(derefString(req.AcceptDate))
			if err != nil {
				return errors.New("验收日期格式错误，应为 YYYY-MM-DD")
			}
			var sum struct{ Ordered, Shipped int }
			if err := tx.Model(&models.EngineeringProjectItem{}).
				Select("COALESCE(SUM(quantity),0) AS ordered, COALESCE(SUM(shipped_qty),0) AS shipped").
				Where("project_id = ?", id).Scan(&sum).Error; err != nil {
				return err
			}
			if err := services.CheckAcceptGuard(services.EngAcceptGuard{
				OrderedQty: sum.Ordered, ShippedQty: sum.Shipped, AcceptDate: acceptDate,
			}); err != nil {
				return err
			}
			// 验收即满足「已验收 → 已结清」的前置条件：若此刻已实收满合同额，
			// 同事务直接进已结清并写结清时间（SRS §4.1 状态表：4 → 5 由收满自动触发）。
			nextStatus := services.EngStatusAccepted
			acceptUpdates := map[string]interface{}{"status": nextStatus, "accept_date": acceptDate}
			var settledNow bool
			if p.ContractTotal > 0 &&
				services.RoundMoney(p.ReceivedAmount) >= services.RoundMoney(p.ContractTotal)-0.004 {
				now := time.Now()
				nextStatus = services.EngStatusSettled
				settledNow = true
				acceptUpdates["status"] = nextStatus
				acceptUpdates["settled_at"] = &now
			}
			if err := tx.Model(&p).Updates(acceptUpdates).Error; err != nil {
				return err
			}
			result = gin.H{
				"id": p.ID, "status": nextStatus,
				"status_label": services.EngStatusLabel(nextStatus),
				"accept_date":  engDatePtr(acceptDate),
				"settled":      settledNow,
			}

		case services.EngActionRevert:
			if engRuneLen(strings.TrimSpace(req.Reason)) < 5 {
				return errors.New("回退原因至少填 5 个字，便于日后追溯")
			}
			target := services.NextStatus(p.Status, action)
			if target <= 0 {
				return errors.New("当前状态不允许回退")
			}
			// 3 → 2：有出库记录就不能退，否则状态与库存事实背离
			if p.Status == services.EngStatusSupplying {
				var shipmentCnt int64
				if err := tx.Model(&models.EngineeringShipment{}).Where("project_id = ?", id).Count(&shipmentCnt).Error; err != nil {
					return err
				}
				if shipmentCnt > 0 {
					return errors.New("项目已有出库记录，不可回退到已签约")
				}
			}
			if err := tx.Model(&p).Updates(map[string]interface{}{
				"status": target, "remark": appendRemark(p.Remark, "回退："+strings.TrimSpace(req.Reason)),
			}).Error; err != nil {
				return err
			}
			result = gin.H{
				"id": p.ID, "status": target, "status_label": services.EngStatusLabel(target),
			}

		case services.EngActionVoid:
			if err := services.CheckVoidGuard(p.Status, req.Reason); err != nil {
				return err
			}
			now := time.Now()
			if err := tx.Model(&p).Updates(map[string]interface{}{
				"status": services.EngStatusVoided, "void_reason": strings.TrimSpace(req.Reason), "voided_at": &now,
			}).Error; err != nil {
				return err
			}
			result = gin.H{
				"id": p.ID, "status": services.EngStatusVoided,
				"status_label": services.EngStatusLabel(services.EngStatusVoided),
			}

		default:
			return errors.New("不支持的操作：" + req.Action)
		}
		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			FailNotFound(c, "项目不存在")
		case engIsConflictErr(err):
			FailDuplicated(c, err.Error())
		default:
			FailBadRequest(c, err.Error())
		}
		return
	}

	OKWithMsg(c, "状态已更新", result)
}

// appendRemark 在备注尾部追加一行审计说明（保留原备注，不覆盖用户内容）
func appendRemark(origin, add string) string {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return add
	}
	merged := origin + "\n" + add
	if engRuneLen(merged) > 500 {
		r := []rune(merged)
		return string(r[len(r)-500:])
	}
	return merged
}
