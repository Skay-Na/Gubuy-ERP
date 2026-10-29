package controllers

// 工程模块 · 工程费用控制器（独立成账的核心）
//
// 归属任务卡：T07（台账查询 / 新增 / 编辑 / 删除 + expense_total 同事务重算）
//
// 隔离声明（代码评审一级拦截项，SRS R11 / 任务卡 H1）：
//  1. 本文件不得出现 financial_logs / accounts / account_id / 经营开销(account_id=7) 的任何写操作；
//  2. pay_method 里的「公户」只是备查标记，不得扣减账户余额；
//  3. 系统已存在经营开销接口 POST/PUT /api/admin/expenses（controllers.CreateExpense /
//     UpdateExpense）；工程费用必须走 /api/admin/engineering/expenses，两套账不可混用。

import (
	"erp-backend/config"
	"erp-backend/models"
	"erp-backend/services"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// engExpenseFilter 费用台账筛选（契约 §3.13）
type engExpenseFilter struct {
	ProjectID  *uint
	OnlyPublic bool
	Categories []int
	DateStart  *time.Time
	DateEnd    *time.Time
	PayStatus  *int
	Keyword    string
}

// applyEngExpenseFilter 统一筛选条件（闭包：各条查询各调一次，不复用同一个 *gorm.DB）。
// alias 为表别名（列表关联 projects 时传 "ex"）；不传则用裸列名。
// 必须带前缀，否则 JOIN projects 后 deleted_at / created_at 会报 ambiguous。
func applyEngExpenseFilter(db *gorm.DB, f engExpenseFilter, alias string) *gorm.DB {
	col := func(name string) string {
		if alias == "" {
			return name
		}
		return alias + "." + name
	}
	db = db.Where(col("deleted_at") + " IS NULL")
	switch {
	case f.OnlyPublic:
		db = db.Where(col("project_id") + " IS NULL")
	case f.ProjectID != nil:
		db = db.Where(col("project_id")+" = ?", *f.ProjectID)
	}
	if len(f.Categories) > 0 {
		db = db.Where(col("category")+" IN ?", f.Categories)
	}
	if f.DateStart != nil {
		db = db.Where(col("expense_date")+" >= ?", *f.DateStart)
	}
	if f.DateEnd != nil {
		db = db.Where(col("expense_date")+" <= ?", *f.DateEnd)
	}
	if f.PayStatus != nil {
		db = db.Where(col("pay_status")+" = ?", *f.PayStatus)
	}
	if f.Keyword != "" {
		like := engLikePattern(f.Keyword)
		db = db.Where("("+col("payee")+" LIKE ? OR "+col("remark")+" LIKE ? OR "+col("project_id")+` IN (
			SELECT id FROM engineering_projects WHERE name LIKE ? AND deleted_at IS NULL))`,
			like, like, like)
	}
	return db
}

// engExpenseSummaryRow 汇总行
type engExpenseSummaryRow struct {
	Total     float64 `gorm:"column:total"`
	Paid      float64 `gorm:"column:paid"`
	Unpaid    float64 `gorm:"column:unpaid"`
	YearTotal float64 `gorm:"column:year_total"`
	Cnt       int64   `gorm:"column:cnt"`
}

// engExpenseCategoryRow 类别分布行
type engExpenseCategoryRow struct {
	Category int     `gorm:"column:category"`
	Amount   float64 `gorm:"column:amount"`
}

// engExpenseOrderBy 排序白名单
func engExpenseOrderBy(sortBy, order string) string {
	dir := "DESC"
	if strings.EqualFold(order, "asc") {
		dir = "ASC"
	}
	switch sortBy {
	case "amount":
		return "ex.amount " + dir + ", ex.id DESC"
	case "created_at":
		return "ex.created_at " + dir + ", ex.id DESC"
	default:
		return "ex.expense_date " + dir + ", ex.id DESC"
	}
}

// ListEngineeringExpenses 工程费用台账 GET /api/admin/engineering/expenses（T07）
//
// 返回 3 块：list（当前页明细）+ summary（当前筛选范围汇总）+ by_category（恒 7 项）。
// summary 的 year_total 是固定口径（本自然年，不受筛选影响），前端卡片要用它。
// @Summary 工程费用台账
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param keyword query string false "关键词：收款方/备注"
// @Param category query int false "费用类别，可重复传多个（1 安装人工 2 运输物流 3 辅材耗材 4 现场施工 5 差旅招待 6 外包服务 7 其他）"
// @Param project_id query string false "所属项目 ID；传 none 只看公共费用"
// @Param date_start query string false "发生日期起 YYYY-MM-DD"
// @Param date_end query string false "发生日期止 YYYY-MM-DD"
// @Param pay_status query int false "付款状态（0 未付 1 已付）"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID list_engineering_expenses
// @Router /api/admin/engineering/expenses [get]
func ListEngineeringExpenses(c *gin.Context) {
	page, pageSize := engPageParams(c)

	f := engExpenseFilter{
		Categories: engIntListQuery(c, "category"),
		Keyword:    engTrimKeyword(c.Query("keyword")),
	}
	pidRaw := strings.TrimSpace(c.Query("project_id"))
	if strings.EqualFold(pidRaw, "none") {
		f.OnlyPublic = true
	} else if pidRaw != "" {
		var pid uint
		if _, err := fmt.Sscanf(pidRaw, "%d", &pid); err != nil || pid == 0 {
			FailBadRequest(c, "项目参数不正确")
			return
		}
		f.ProjectID = &pid
	}
	var err error
	if f.DateStart, err = services.ParseBusinessDate(c.Query("date_start")); err != nil {
		FailBadRequest(c, "开始日期格式错误，应为 YYYY-MM-DD")
		return
	}
	if f.DateEnd, err = services.ParseBusinessDate(c.Query("date_end")); err != nil {
		FailBadRequest(c, "结束日期格式错误，应为 YYYY-MM-DD")
		return
	}
	if v := strings.TrimSpace(c.Query("pay_status")); v != "" {
		var ps int
		if _, err := fmt.Sscanf(v, "%d", &ps); err != nil || (ps != 0 && ps != 1) {
			FailBadRequest(c, "付款状态参数不正确")
			return
		}
		f.PayStatus = &ps
	}

	var total int64
	if err := applyEngExpenseFilter(config.DB.Table("engineering_expenses AS ex"), f, "ex").Count(&total).Error; err != nil {
		FailInternal(c, "费用总数查询失败："+err.Error())
		return
	}

	type expenseRow struct {
		ID           uint       `gorm:"column:id"`
		ProjectID    *uint      `gorm:"column:project_id"`
		ProjectNo    string     `gorm:"column:project_no"`
		ProjectName  string     `gorm:"column:project_name"`
		Category     int        `gorm:"column:category"`
		Amount       float64    `gorm:"column:amount"`
		ExpenseDate  time.Time  `gorm:"column:expense_date"`
		PayStatus    int        `gorm:"column:pay_status"`
		PayMethod    int        `gorm:"column:pay_method"`
		PaidDate     *time.Time `gorm:"column:paid_date"`
		Payee        string     `gorm:"column:payee"`
		BillNo       string     `gorm:"column:bill_no"`
		Remark       string     `gorm:"column:remark"`
		OperatorName string     `gorm:"column:operator_name"`
		CreatedAt    time.Time  `gorm:"column:created_at"`
	}
	rows := make([]expenseRow, 0, pageSize)
	if total > 0 {
		err := applyEngExpenseFilter(config.DB.Table("engineering_expenses AS ex"), f, "ex").
			Select(`ex.id, ex.project_id, COALESCE(p.project_no, '') AS project_no, COALESCE(p.name, '') AS project_name,
			        ex.category, ex.amount, ex.expense_date, ex.pay_status, ex.pay_method, ex.paid_date,
			        ex.payee, ex.bill_no, ex.remark, ex.operator_name, ex.created_at`).
			Joins("LEFT JOIN engineering_projects AS p ON p.id = ex.project_id AND p.deleted_at IS NULL").
			Order(engExpenseOrderBy(c.Query("sort_by"), c.Query("order"))).
			Offset((page - 1) * pageSize).Limit(pageSize).
			Scan(&rows).Error
		if err != nil {
			FailInternal(c, "费用列表查询失败："+err.Error())
			return
		}
	}

	list := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		list = append(list, gin.H{
			"id":               r.ID,
			"project_id":       r.ProjectID,
			"project_no":       r.ProjectNo,
			"project_name":     r.ProjectName,
			"is_public":        r.ProjectID == nil,
			"category":         r.Category,
			"category_label":   services.EngExpenseCategoryLabel(r.Category),
			"amount":           services.RoundMoney(r.Amount),
			"expense_date":     r.ExpenseDate.Format("2006-01-02"),
			"pay_status":       r.PayStatus,
			"pay_status_label": services.EngPayStatusLabel(r.PayStatus),
			"pay_method":       r.PayMethod,
			"pay_method_label": engPayMethodLabelOrDash(r.PayMethod),
			"paid_date":        engDatePtr(r.PaidDate),
			"payee":            r.Payee,
			"bill_no":          r.BillNo,
			"remark":           r.Remark,
			"operator_name":    r.OperatorName,
			"created_at":       r.CreatedAt,
		})
	}

	// 汇总（当前筛选范围）
	var s engExpenseSummaryRow
	if err := applyEngExpenseFilter(config.DB.Table("engineering_expenses"), f, "").
		Select(`COALESCE(SUM(amount), 0) AS total,
		        COALESCE(SUM(CASE WHEN pay_status = 1 THEN amount ELSE 0 END), 0) AS paid,
		        COALESCE(SUM(CASE WHEN pay_status = 0 THEN amount ELSE 0 END), 0) AS unpaid,
		        COALESCE(SUM(CASE WHEN YEAR(expense_date) = YEAR(CURDATE()) THEN amount ELSE 0 END), 0) AS year_total,
		        COUNT(*) AS cnt`).
		Scan(&s).Error; err != nil {
		FailInternal(c, "费用汇总失败："+err.Error())
		return
	}

	// 本年合计（固定口径，不受筛选影响）
	var fixed struct {
		YearTotal float64 `gorm:"column:year_total"`
	}
	if err := config.DB.Table("engineering_expenses").
		Select("COALESCE(SUM(amount), 0) AS year_total").
		Where("deleted_at IS NULL AND YEAR(expense_date) = YEAR(CURDATE())").
		Scan(&fixed).Error; err != nil {
		FailInternal(c, "本年费用汇总失败："+err.Error())
		return
	}

	// 类别分布（重新构造查询，不复用上面那条）
	var catRows []engExpenseCategoryRow
	if err := applyEngExpenseFilter(config.DB.Table("engineering_expenses"), f, "").
		Select("category, SUM(amount) AS amount").
		Group("category").Scan(&catRows).Error; err != nil {
		FailInternal(c, "费用类别分布查询失败："+err.Error())
		return
	}
	catMap := make(map[int]float64, len(catRows))
	for _, cr := range catRows {
		catMap[cr.Category] = cr.Amount
	}
	byCategory := make([]gin.H, 0, len(services.EngExpenseCategories))
	for _, cat := range services.EngExpenseCategories {
		byCategory = append(byCategory, gin.H{
			"category":       cat,
			"category_label": services.EngExpenseCategoryLabel(cat),
			"amount":         services.RoundMoney(catMap[cat]),
		})
	}

	c.JSON(http.StatusOK, struct {
		Code  int         `json:"code"`
		Msg   string      `json:"msg"`
		Data  interface{} `json:"data"`
		Total int64       `json:"total"`
	}{Code: ErrCodeOK, Msg: "success", Total: total, Data: gin.H{
		"list": list,
		"summary": gin.H{
			"total":      services.RoundMoney(s.Total),
			"paid":       services.RoundMoney(s.Paid),
			"unpaid":     services.RoundMoney(s.Unpaid),
			"count":      s.Cnt,
			"year_total": services.RoundMoney(fixed.YearTotal),
		},
		"by_category": byCategory,
	}})
}

// engPayMethodLabelOrDash 付款方式为 0（未指定）时显示 —（契约要求）
func engPayMethodLabelOrDash(m int) string {
	if m == 0 {
		return "—"
	}
	return services.EngPayMethodLabel(m)
}

// ---------------------------------------------------------------------------
// 费用写操作
// ---------------------------------------------------------------------------

type engExpenseReq struct {
	ProjectID       *uint   `json:"project_id"`
	Category        int     `json:"category"`
	Amount          float64 `json:"amount"`
	ExpenseDate     string  `json:"expense_date"`
	PayStatus       int     `json:"pay_status"`
	PayMethod       int     `json:"pay_method"`
	PaidDate        *string `json:"paid_date"`
	Payee           string  `json:"payee"`
	BillNo          string  `json:"bill_no"`
	Remark          string  `json:"remark"`
	ClientRequestID *string `json:"client_request_id"`
}

// validate 费用字段校验（新增与编辑共用，避免两处规则分叉）
func (r *engExpenseReq) validate() (*time.Time, *time.Time, error) {
	if r.Category < 1 || r.Category > 7 {
		return nil, nil, errors.New("费用类别不正确")
	}
	amount := services.RoundMoney(r.Amount)
	if amount <= 0 {
		return nil, nil, errors.New("费用金额必须大于 0")
	}
	if amount > 9999999.99 {
		return nil, nil, errors.New("费用金额不能超过 9,999,999.99")
	}
	if r.PayStatus != 0 && r.PayStatus != 1 {
		return nil, nil, errors.New("付款状态不正确")
	}
	if r.PayStatus == 1 && (r.PayMethod < 0 || r.PayMethod > 5) {
		return nil, nil, errors.New("请选择付款方式")
	}
	if r.PayStatus == 1 && r.PayMethod == 0 {
		return nil, nil, errors.New("请选择付款方式")
	}
	if engRuneLen(r.Payee) > 100 || engRuneLen(r.BillNo) > 50 || engRuneLen(r.Remark) > 200 {
		return nil, nil, errors.New("收款方、票据号或备注过长")
	}
	if r.Category == 7 && strings.TrimSpace(r.Remark) == "" {
		return nil, nil, errors.New("「其他」类别请填写备注，说明这笔钱花在哪")
	}
	expenseDate, err := services.ParseBusinessDate(r.ExpenseDate)
	if err != nil || expenseDate == nil {
		return nil, nil, errors.New("发生日期格式错误，应为 YYYY-MM-DD")
	}
	if expenseDate.After(engToday()) {
		return nil, nil, errors.New("发生日期不能晚于今天")
	}
	paidDate, err := services.ParseBusinessDate(derefString(r.PaidDate))
	if err != nil {
		return nil, nil, errors.New("付款日期格式错误，应为 YYYY-MM-DD")
	}
	if r.PayStatus == 1 && paidDate == nil {
		paidDate = expenseDate // 已付但没填付款日期时默认等于发生日期
	}
	if paidDate != nil && paidDate.After(engToday()) {
		return nil, nil, errors.New("付款日期不能晚于今天")
	}
	return expenseDate, paidDate, nil
}

// recalcProjectExpenseTotal 重算某项目的费用合计并写回主表（唯一入口，禁止各处手写 SUM）
func recalcProjectExpenseTotal(tx *gorm.DB, projectID uint) (float64, error) {
	var sum struct {
		Total float64 `gorm:"column:total"`
	}
	if err := tx.Model(&models.EngineeringExpense{}).
		Select("COALESCE(SUM(amount), 0) AS total").
		Where("project_id = ?", projectID).
		Scan(&sum).Error; err != nil {
		return 0, err
	}
	total := services.RoundMoney(sum.Total)
	if err := tx.Model(&models.EngineeringProject{}).Where("id = ?", projectID).
		UpdateColumn("expense_total", total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// engProjectNetProfitSnapshot 组装费用写操作响应里的净利三件套；公共费用返回 nil
func engProjectNetProfitSnapshot(tx *gorm.DB, projectID uint, expenseTotal float64) (gin.H, error) {
	var p struct {
		ContractTotal float64 `gorm:"column:contract_total"`
		CostTotal     float64 `gorm:"column:cost_total"`
	}
	if err := tx.Model(&models.EngineeringProject{}).
		Select("contract_total, cost_total").Where("id = ?", projectID).Scan(&p).Error; err != nil {
		return nil, err
	}
	profit := services.CalcProfitability(p.ContractTotal, p.CostTotal, expenseTotal)
	return gin.H{
		"project_expense_total": expenseTotal,
		"net_profit":            profit.NetProfit,
		"net_margin_rate":       profit.NetMarginRate,
	}, nil
}

// CreateEngineeringExpense 新增费用 POST /api/admin/engineering/expenses（T07）
// @Summary 新增工程费用
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param body body engExpenseReq true "费用参数（project_id 为空=公共费用）"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID create_engineering_expense
// @Router /api/admin/engineering/expenses [post]
func CreateEngineeringExpense(c *gin.Context) {
	var req engExpenseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数格式错误："+err.Error())
		return
	}
	expenseDate, paidDate, err := req.validate()
	if err != nil {
		FailBadRequest(c, err.Error())
		return
	}
	if req.ClientRequestID != nil && engRuneLen(*req.ClientRequestID) > 64 {
		FailBadRequest(c, "幂等键不能超过 64 字")
		return
	}

	// 幂等
	if req.ClientRequestID != nil && strings.TrimSpace(*req.ClientRequestID) != "" {
		var cnt int64
		if err := config.DB.Model(&models.EngineeringExpense{}).
			Where("client_request_id = ?", *req.ClientRequestID).Count(&cnt).Error; err != nil {
			FailInternal(c, "幂等校验失败："+err.Error())
			return
		}
		if cnt > 0 {
			FailDuplicated(c, "该笔费用已记录")
			return
		}
	}

	operator := InventoryOperatorFromContext(c)

	var result gin.H
	err = config.DB.Transaction(func(tx *gorm.DB) error {
		if req.ProjectID != nil {
			var p models.EngineeringProject
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, *req.ProjectID).Error; err != nil {
				return errors.New("项目不存在")
			}
			if p.Status == services.EngStatusVoided {
				return errors.New("项目已作废，不可新增费用")
			}
		}

		expense := models.EngineeringExpense{
			ProjectID: req.ProjectID, Category: req.Category, Amount: services.RoundMoney(req.Amount),
			ExpenseDate: *expenseDate, PayStatus: req.PayStatus, PayMethod: req.PayMethod,
			PaidDate: paidDate, Payee: strings.TrimSpace(req.Payee), BillNo: strings.TrimSpace(req.BillNo),
			Remark: strings.TrimSpace(req.Remark), OperatorID: operator.EmployeeID, OperatorName: operator.Name,
			ClientRequestID: req.ClientRequestID,
		}
		if err := tx.Create(&expense).Error; err != nil {
			if engIsDuplicateErr(err) {
				return errors.New("该笔费用已记录")
			}
			return err
		}

		if req.ProjectID == nil {
			// 公共费用：没有项目可重算，净利字段返回 null（前端据此跳过刷新）
			result = gin.H{
				"expense_id":            expense.ID,
				"project_expense_total": nil,
				"net_profit":            nil,
				"net_margin_rate":       nil,
			}
			return nil
		}

		total, err := recalcProjectExpenseTotal(tx, *req.ProjectID)
		if err != nil {
			return err
		}
		snap, err := engProjectNetProfitSnapshot(tx, *req.ProjectID, total)
		if err != nil {
			return err
		}
		snap["expense_id"] = expense.ID
		result = snap
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
	OKWithMsg(c, "费用已记录", result)
}

// UpdateEngineeringExpense 编辑费用 PUT /api/admin/engineering/expenses/:eid（T07）
//
// 若 project_id 从 A 改为 B，A 与 B 的 expense_total 都要重算；按 ID 升序锁两行，避免死锁。
// @Summary 修改工程费用
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param eid path int true "费用 ID"
// @Param body body engExpenseReq true "费用参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID update_engineering_expense
// @Router /api/admin/engineering/expenses/{eid} [put]
func UpdateEngineeringExpense(c *gin.Context) {
	eid, ok := engIDParam(c, "eid")
	if !ok {
		return
	}
	var req engExpenseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数格式错误："+err.Error())
		return
	}
	expenseDate, paidDate, err := req.validate()
	if err != nil {
		FailBadRequest(c, err.Error())
		return
	}

	var result gin.H
	err = config.DB.Transaction(func(tx *gorm.DB) error {
		var expense models.EngineeringExpense
		if err := tx.First(&expense, eid).Error; err != nil {
			return errors.New("费用不存在")
		}
		oldProjectID := expense.ProjectID

		// 涉及的项目行按 ID 升序加锁
		lockIDs := make([]uint, 0, 2)
		if oldProjectID != nil {
			lockIDs = append(lockIDs, *oldProjectID)
		}
		if req.ProjectID != nil && (oldProjectID == nil || *req.ProjectID != *oldProjectID) {
			lockIDs = append(lockIDs, *req.ProjectID)
		}
		if len(lockIDs) > 1 && lockIDs[0] > lockIDs[1] {
			lockIDs[0], lockIDs[1] = lockIDs[1], lockIDs[0]
		}
		for _, pid := range lockIDs {
			var p models.EngineeringProject
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, pid).Error; err != nil {
				return errors.New("项目不存在")
			}
			if p.Status == services.EngStatusVoided {
				if req.ProjectID != nil && pid == *req.ProjectID {
					return errors.New("项目已作废，不可挂载费用")
				}
				return errors.New("项目已作废，不可修改费用")
			}
		}

		if err := tx.Model(&models.EngineeringExpense{}).Where("id = ?", eid).Updates(map[string]interface{}{
			"project_id":   req.ProjectID,
			"category":     req.Category,
			"amount":       services.RoundMoney(req.Amount),
			"expense_date": expenseDate,
			"pay_status":   req.PayStatus,
			"pay_method":   req.PayMethod,
			"paid_date":    paidDate,
			"payee":        strings.TrimSpace(req.Payee),
			"bill_no":      strings.TrimSpace(req.BillNo),
			"remark":       strings.TrimSpace(req.Remark),
		}).Error; err != nil {
			return err
		}

		// 老项目必重算（金额可能变了、也可能被移走）
		if oldProjectID != nil {
			if _, err := recalcProjectExpenseTotal(tx, *oldProjectID); err != nil {
				return err
			}
		}
		if req.ProjectID == nil {
			result = gin.H{
				"expense_id":            eid,
				"project_expense_total": nil,
				"net_profit":            nil,
				"net_margin_rate":       nil,
			}
			return nil
		}
		total, err := recalcProjectExpenseTotal(tx, *req.ProjectID)
		if err != nil {
			return err
		}
		snap, err := engProjectNetProfitSnapshot(tx, *req.ProjectID, total)
		if err != nil {
			return err
		}
		snap["expense_id"] = eid
		result = snap
		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			FailNotFound(c, "费用不存在")
		default:
			FailBadRequest(c, err.Error())
		}
		return
	}
	OKWithMsg(c, "费用已更新", result)
}

// DeleteEngineeringExpense 删除费用（软删除） DELETE /api/admin/engineering/expenses/:eid（T07）
// @Summary 删除工程费用（软删）
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param eid path int true "费用 ID"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID delete_engineering_expense
// @Router /api/admin/engineering/expenses/{eid} [delete]
func DeleteEngineeringExpense(c *gin.Context) {
	eid, ok := engIDParam(c, "eid")
	if !ok {
		return
	}

	var result gin.H
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		var expense models.EngineeringExpense
		if err := tx.First(&expense, eid).Error; err != nil {
			return errors.New("费用不存在")
		}
		projectID := expense.ProjectID
		if projectID != nil {
			var p models.EngineeringProject
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, *projectID).Error; err != nil {
				return errors.New("项目不存在")
			}
			if p.Status == services.EngStatusVoided {
				return errors.New("项目已作废，不可删除费用")
			}
		}
		if err := tx.Delete(&models.EngineeringExpense{}, eid).Error; err != nil {
			return err
		}
		if projectID == nil {
			result = gin.H{
				"expense_id":            eid,
				"project_expense_total": nil,
				"net_profit":            nil,
			}
			return nil
		}
		total, err := recalcProjectExpenseTotal(tx, *projectID)
		if err != nil {
			return err
		}
		snap, err := engProjectNetProfitSnapshot(tx, *projectID, total)
		if err != nil {
			return err
		}
		snap["expense_id"] = eid
		result = snap
		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			FailNotFound(c, "费用不存在")
		default:
			FailBadRequest(c, err.Error())
		}
		return
	}
	OKWithMsg(c, "费用已删除", result)
}
