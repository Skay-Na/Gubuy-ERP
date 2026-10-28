package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ProfitBreakdownResponse 利润计算过程汇总响应。
// 各字段口径与 GetFinanceSummary 完全一致，供看板利润过程区展示公式链路。
type ProfitBreakdownResponse struct {
	SalesRevenue           float64 `json:"sales_revenue"`            // 销售额：正常订单 SUM(total_amount - discount_amount)
	ProductCost            float64 `json:"product_cost"`             // 货品成本：order_items.unit_cost × quantity
	DeliveryFee            float64 `json:"delivery_fee"`             // 配送费
	InstallationFee        float64 `json:"installation_fee"`         // 安装费
	ReferralFee            float64 `json:"referral_fee"`             // 介绍人提成
	CommissionAmount       float64 `json:"commission_amount"`        // 商会提成
	TradeInAmount          float64 `json:"trade_in_amount"`          // 以旧换新折抵
	GrossProfit            float64 `json:"gross_profit"`             // 毛利 = 销售额 - 货品成本 - 五项费用
	OperatingExpense       float64 `json:"operating_expense"`        // 经营开销：Expense 中排除 采购货款/以旧换新
	NetProfit              float64 `json:"net_profit"`               // 经营净利润 = 毛利 - 经营开销
	OrderCount             int64   `json:"order_count"`              // 期间正常订单数
	CostIncomplete         bool    `json:"cost_incomplete"`          // 是否存在待核算成本明细
	UncostedOrderItemCount int64   `json:"uncosted_order_item_count"` // 待核算成本明细条数（unit_cost <= 0）
}

// BreakdownOrderRow 订单类明细行（销售额/费用/待收尾款/国补/员工订单/趋势区间）。
// created_at 输出 DATE_FORMAT 后的纯日期串（YYYY-MM-DD）。
type BreakdownOrderRow struct {
	ID           uint    `json:"id"`
	OrderNo      string  `json:"order_no"`
	CustomerName string  `json:"customer_name"`
	CreatedAt    string  `json:"created_at"`
	Amount       float64 `json:"amount"`
	// 可选上下文字段（仅特定类型填充，前端按类型选择性展示）
	ActualPayAmount float64 `json:"actual_pay_amount,omitempty"` // 待收尾款：客户实际应付
	DepositAmount   float64 `json:"deposit_amount,omitempty"`    // 待收尾款：已付定金
	SubsidyPending  float64 `json:"subsidy_pending,omitempty"`   // 国补：待回款金额
	SubsidyReceived float64 `json:"subsidy_received,omitempty"`  // 国补：已核销回款
}

// BreakdownCostRow 按单分组的货品成本明细行。
type BreakdownCostRow struct {
	ID            uint    `json:"id"`
	OrderNo       string  `json:"order_no"`
	CustomerName  string  `json:"customer_name"`
	CreatedAt     string  `json:"created_at"`
	Amount        float64 `json:"amount"`        // 单成本小计：SUM(unit_cost × quantity)
	UncostedCount int64   `json:"uncosted_count"` // 该单内待核算明细件数
}

// BreakdownExpenseRow 经营开销明细行。
type BreakdownExpenseRow struct {
	ID          uint    `json:"id"`
	CreatedAt   string  `json:"created_at"`
	ExpenseType string  `json:"expense_type"`
	Amount      float64 `json:"amount"`
	Remark      string  `json:"remark"`
}

// BreakdownLogRow 账户流水明细行。
type BreakdownLogRow struct {
	ID           uint    `json:"id"`
	CreatedAt    string  `json:"created_at"`
	FlowType     int     `json:"flow_type"` // 1-收入, 2-支出
	Category     string  `json:"category"`
	Amount       float64 `json:"amount"`
	BalanceAfter float64 `json:"balance_after"`
	Counterparty string  `json:"counterparty"`
	RelatedNo    string  `json:"related_no"`
	Remark       string  `json:"remark"`
}

// BreakdownDetailResponse 明细钻取响应。
// TotalAmount 为后端不分页的 SUM（WHERE 与明细完全一致），前端合计行直接绑定该值。
type BreakdownDetailResponse struct {
	List        interface{} `json:"list"`
	Total       int64       `json:"total"`
	Page        int         `json:"page"`
	PageSize    int         `json:"page_size"`
	TotalAmount float64     `json:"total_amount"`
	Title       string      `json:"title"`
}

// breakdownTypeTitles 明细类型 → 标题。列配置由前端 drillColumns.js 维护。
var breakdownTypeTitles = map[string]string{
	"revenue":          "销售额明细",
	"cost":             "货品成本明细",
	"delivery":         "配送费明细",
	"installation":     "安装费明细",
	"referral":         "介绍人提成明细",
	"commission":       "商会提成明细",
	"trade_in":         "以旧换新折抵明细",
	"operating_expense": "经营开销明细",
	"pending_balance":  "待收尾款明细",
	"pending_subsidy":  "国补待回款明细",
	"employee_orders":  "员工订单明细",
	"account_logs":     "账户流水明细",
	"trend_orders":     "趋势区间订单明细",
}

// feeColumnByType 五项费用类型 → 订单表列名（白名单，避免拼接注入）。
var feeColumnByType = map[string]string{
	"delivery":     "delivery_fee",
	"installation": "installation_fee",
	"referral":     "referral_fee",
	"commission":   "commission_amount",
	"trade_in":     "trade_in_amount",
}

// pendingSubsidyExpr 逐单国补可用待回款表达式（与 GetFinanceSummary 国补卡片口径一致）。
const pendingSubsidyExpr = "GREATEST(subsidy_pending_amount - subsidy_received_amt, 0)"

// orderPeriodScope 按期间过滤的正常订单基础查询（与 GetFinanceSummary 各分段口径一致）。
func orderPeriodScope(startTime, endTime time.Time) *gorm.DB {
	db := config.DB.Model(&models.Order{}).Where("order_status = 1")
	if !startTime.IsZero() {
		db = db.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		db = db.Where("created_at <= ?", endTime)
	}
	return db
}

// parsePositiveInt 解析正整数查询参数，非法或小于 1 时返回默认值。
func parsePositiveInt(raw string, defaultValue int) int {
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return defaultValue
	}
	return value
}

// GetProfitBreakdown 利润计算过程汇总
// @Summary 获取利润计算过程各组成项金额
// @Description 返回销售额/货品成本/五项费用/毛利/经营开销/净利润，口径与 finance/summary 完全一致
// @Tags Finance
// @Accept json
// @Param period query string false "时间维度 (today, month, last_month, year, custom, all)"
// @Param start_date query string false "开始日期 (当 period=custom 时有效)"
// @Param end_date query string false "结束日期 (当 period=custom 时有效)"
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_finance_profit_breakdown
// @Router /api/finance/profit-breakdown [get]
func GetProfitBreakdown(c *gin.Context) {
	startTime, endTime, periodErr := resolvePeriodRange(c)
	if periodErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": periodErr.Error()})
		return
	}

	result, err := buildProfitBreakdown(startTime, endTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取利润计算过程失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": result,
	})
}

// buildProfitBreakdown 按期间计算利润公式各组成项。
// 全部 SQL 逐条复刻 GetFinanceSummary 对应分段，保证与卡片数字分毫不差。
func buildProfitBreakdown(startTime, endTime time.Time) (*ProfitBreakdownResponse, error) {
	result := &ProfitBreakdownResponse{}

	// 销售额（SUM(total_amount - discount_amount)，order_status=1）
	if err := orderPeriodScope(startTime, endTime).
		Select("COALESCE(SUM(total_amount - discount_amount), 0)").
		Scan(&result.SalesRevenue).Error; err != nil {
		return nil, err
	}

	// 订单数
	if err := orderPeriodScope(startTime, endTime).
		Count(&result.OrderCount).Error; err != nil {
		return nil, err
	}

	// 五项费用
	var totalFees struct {
		DeliveryFee      float64
		InstallationFee  float64
		ReferralFee      float64
		CommissionAmount float64
		TradeInAmount    float64
	}
	if err := orderPeriodScope(startTime, endTime).
		Select("COALESCE(SUM(delivery_fee), 0) as delivery_fee, COALESCE(SUM(installation_fee), 0) as installation_fee, COALESCE(SUM(referral_fee), 0) as referral_fee, COALESCE(SUM(commission_amount), 0) as commission_amount, COALESCE(SUM(trade_in_amount), 0) as trade_in_amount").
		Scan(&totalFees).Error; err != nil {
		return nil, err
	}
	result.DeliveryFee = totalFees.DeliveryFee
	result.InstallationFee = totalFees.InstallationFee
	result.ReferralFee = totalFees.ReferralFee
	result.CommissionAmount = totalFees.CommissionAmount
	result.TradeInAmount = totalFees.TradeInAmount

	// 货品成本（order_items.unit_cost × quantity）
	costDB := config.DB.Table("order_items").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("orders.order_status = 1")
	if !startTime.IsZero() {
		costDB = costDB.Where("orders.created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		costDB = costDB.Where("orders.created_at <= ?", endTime)
	}
	if err := costDB.Select("COALESCE(SUM(order_items.unit_cost * order_items.quantity), 0)").
		Scan(&result.ProductCost).Error; err != nil {
		return nil, err
	}

	// 待核算成本明细（unit_cost <= 0）
	uncostedDB := config.DB.Table("order_items").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("orders.order_status = 1 AND order_items.unit_cost <= 0")
	if !startTime.IsZero() {
		uncostedDB = uncostedDB.Where("orders.created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		uncostedDB = uncostedDB.Where("orders.created_at <= ?", endTime)
	}
	if err := uncostedDB.Count(&result.UncostedOrderItemCount).Error; err != nil {
		return nil, err
	}
	result.CostIncomplete = result.UncostedOrderItemCount > 0

	// 经营开销（排除 采购货款/以旧换新）
	operatingExpenseDB := config.DB.Model(&models.Expense{}).
		Where("expense_type NOT IN ?", []string{"采购货款", "以旧换新"})
	if !startTime.IsZero() {
		operatingExpenseDB = operatingExpenseDB.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		operatingExpenseDB = operatingExpenseDB.Where("created_at <= ?", endTime)
	}
	if err := operatingExpenseDB.Select("COALESCE(SUM(amount), 0)").
		Scan(&result.OperatingExpense).Error; err != nil {
		return nil, err
	}

	result.GrossProfit = CalculateGrossProfit(
		result.SalesRevenue,
		result.ProductCost,
		result.DeliveryFee,
		result.InstallationFee,
		result.ReferralFee,
		result.CommissionAmount,
		result.TradeInAmount,
	)
	result.NetProfit = CalculateNetProfit(result.GrossProfit, result.OperatingExpense)

	return result, nil
}

// GetBreakdownDetail 明细钻取
// @Summary 按类型返回看板明细列表（分页，附后端全量合计）
// @Description type: revenue/cost/delivery/installation/referral/commission/trade_in/operating_expense/pending_balance/pending_subsidy/employee_orders/account_logs/trend_orders
// @Tags Finance
// @Accept json
// @Param type path string true "明细类型"
// @Param period query string false "时间维度 (today, month, last_month, year, custom, all)"
// @Param start_date query string false "开始日期 (当 period=custom 时有效)"
// @Param end_date query string false "结束日期 (当 period=custom 时有效)"
// @Param page query int false "页码 (默认 1)"
// @Param page_size query int false "每页条数 (默认 50，最大 200)"
// @Param employee_id query string false "员工筛选：真实ID / boss=老板客户 / 999999=商会订单"
// @Param account_id query int false "账户ID (account_logs 类型)"
// @Param only_uncosted query bool false "仅含待核算成本的单 (cost 类型)"
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_finance_breakdown
// @Router /api/finance/breakdown/{type} [get]
func GetBreakdownDetail(c *gin.Context) {
	breakdownType := c.Param("type")
	title, known := breakdownTypeTitles[breakdownType]
	if !known {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "未知的明细类型"})
		return
	}

	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("page_size"), 50)
	if pageSize > 200 {
		pageSize = 200
	}

	// pending_balance / pending_subsidy 为全量存量口径，忽略 period 参数（与卡片分毫不差）。
	var startTime, endTime time.Time
	if breakdownType != "pending_balance" && breakdownType != "pending_subsidy" {
		var periodErr error
		startTime, endTime, periodErr = resolvePeriodRange(c)
		if periodErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": periodErr.Error()})
			return
		}
	}

	var (
		list        interface{}
		total       int64
		totalAmount float64
		err         error
	)

	switch breakdownType {
	case "revenue", "trend_orders":
		// trend_orders：趋势柱覆盖区间（前端传 period=custom + 起止日期），必须显式传区间。
		if breakdownType == "trend_orders" && startTime.IsZero() {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "趋势区间明细需要 custom 期间与起止日期"})
			return
		}
		// 销售额口径与 GetFinanceSummary 一致（Model 触发软删除过滤）；
		// trend_orders 对齐 sales_trend 的 Table("orders") 原生口径（趋势聚合本身未过滤软删除）。
		list, total, totalAmount, err = orderAmountBreakdown(startTime, endTime, "", nil, page, pageSize, breakdownType == "trend_orders")
	case "cost":
		onlyUncosted := c.Query("only_uncosted") == "true" || c.Query("only_uncosted") == "1"
		list, total, totalAmount, err = costBreakdown(startTime, endTime, onlyUncosted, page, pageSize)
	case "delivery", "installation", "referral", "commission", "trade_in":
		list, total, totalAmount, err = feeBreakdown(feeColumnByType[breakdownType], startTime, endTime, page, pageSize)
	case "operating_expense":
		list, total, totalAmount, err = operatingExpenseBreakdown(startTime, endTime, page, pageSize)
	case "pending_balance":
		list, total, totalAmount, err = pendingBalanceBreakdown(page, pageSize)
	case "pending_subsidy":
		list, total, totalAmount, err = pendingSubsidyBreakdown(page, pageSize)
	case "employee_orders":
		list, total, totalAmount, err = employeeOrdersBreakdown(c.Query("employee_id"), startTime, endTime, page, pageSize)
	case "account_logs":
		list, total, totalAmount, err = accountLogsBreakdown(c, startTime, endTime, page, pageSize)
	}

	if err != nil {
		var paramErr *invalidParamError
		if errors.As(err, &paramErr) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": paramErr.msg})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取明细失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": BreakdownDetailResponse{
			List:        list,
			Total:       total,
			Page:        page,
			PageSize:    pageSize,
			TotalAmount: totalAmount,
			Title:       title,
		},
	})
}

// orderAmountBreakdown 订单金额类明细：行金额与合计口径 = total_amount - discount_amount。
// useRawTable 用于 trend_orders：对齐 sales_trend 的 Table("orders") 原生 SQL 口径。
func orderAmountBreakdown(startTime, endTime time.Time, extraWhere string, extraArgs []interface{}, page, pageSize int, useRawTable bool) ([]BreakdownOrderRow, int64, float64, error) {
	scope := func() *gorm.DB {
		var db *gorm.DB
		if useRawTable {
			db = config.DB.Table("orders").Where("order_status = 1")
		} else {
			db = config.DB.Model(&models.Order{}).Where("order_status = 1")
		}
		if !startTime.IsZero() {
			db = db.Where("created_at >= ?", startTime)
		}
		if !endTime.IsZero() {
			db = db.Where("created_at <= ?", endTime)
		}
		if extraWhere != "" {
			db = db.Where(extraWhere, extraArgs...)
		}
		return db
	}

	var rows []BreakdownOrderRow
	if err := scope().
		Select("id, order_no, customer_name, DATE_FORMAT(created_at, '%Y-%m-%d') as created_at, (total_amount - discount_amount) as amount").
		Order("created_at desc, id desc").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, 0, err
	}

	var total int64
	if err := scope().Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}

	var totalAmount float64
	if err := scope().Select("COALESCE(SUM(total_amount - discount_amount), 0)").Scan(&totalAmount).Error; err != nil {
		return nil, 0, 0, err
	}
	return rows, total, totalAmount, nil
}

// costScope 货品成本按单分组子查询（订单/客户/成本小计/待核算件数）。
// onlyUncosted=true 时仅保留含待核算明细的单（HAVING 与合计口径一致）。
func costScope(startTime, endTime time.Time, onlyUncosted bool) *gorm.DB {
	sub := config.DB.Table("order_items").
		Select("order_items.order_id, SUM(order_items.unit_cost * order_items.quantity) AS total_cost, SUM(CASE WHEN order_items.unit_cost <= 0 THEN 1 ELSE 0 END) AS uncosted_count").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("orders.order_status = 1")
	if !startTime.IsZero() {
		sub = sub.Where("orders.created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		sub = sub.Where("orders.created_at <= ?", endTime)
	}
	sub = sub.Group("order_items.order_id")
	if onlyUncosted {
		sub = sub.Having("uncosted_count > 0")
	}
	return config.DB.Table("(?) AS cost_orders", sub)
}

// costBreakdown 货品成本明细：按单分组，行金额 = 单成本小计，合计 = SUM(total_cost)。
func costBreakdown(startTime, endTime time.Time, onlyUncosted bool, page, pageSize int) ([]BreakdownCostRow, int64, float64, error) {
	var rows []BreakdownCostRow
	if err := costScope(startTime, endTime, onlyUncosted).
		Select("orders.id, orders.order_no, orders.customer_name, DATE_FORMAT(orders.created_at, '%Y-%m-%d') as created_at, cost_orders.total_cost as amount, cost_orders.uncosted_count").
		Joins("JOIN orders ON orders.id = cost_orders.order_id").
		Order("orders.created_at desc, orders.id desc").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, 0, err
	}

	var total int64
	if err := costScope(startTime, endTime, onlyUncosted).Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}

	var totalAmount float64
	if err := costScope(startTime, endTime, onlyUncosted).
		Select("COALESCE(SUM(cost_orders.total_cost), 0)").Scan(&totalAmount).Error; err != nil {
		return nil, 0, 0, err
	}
	return rows, total, totalAmount, nil
}

// feeBreakdown 五项费用明细：仅返回对应字段 > 0 的订单，行金额 = 该字段。
func feeBreakdown(column string, startTime, endTime time.Time, page, pageSize int) ([]BreakdownOrderRow, int64, float64, error) {
	scope := func() *gorm.DB {
		db := config.DB.Model(&models.Order{}).
			Where("order_status = 1 AND " + column + " > 0")
		if !startTime.IsZero() {
			db = db.Where("created_at >= ?", startTime)
		}
		if !endTime.IsZero() {
			db = db.Where("created_at <= ?", endTime)
		}
		return db
	}

	var rows []BreakdownOrderRow
	if err := scope().
		Select("id, order_no, customer_name, DATE_FORMAT(created_at, '%Y-%m-%d') as created_at, "+column+" as amount").
		Order("created_at desc, id desc").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, 0, err
	}

	var total int64
	if err := scope().Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}

	var totalAmount float64
	if err := scope().Select("COALESCE(SUM(" + column + "), 0)").Scan(&totalAmount).Error; err != nil {
		return nil, 0, 0, err
	}
	return rows, total, totalAmount, nil
}

// operatingExpenseBreakdown 经营开销逐笔明细（Expense 中排除 采购货款/以旧换新）。
func operatingExpenseBreakdown(startTime, endTime time.Time, page, pageSize int) ([]BreakdownExpenseRow, int64, float64, error) {
	scope := func() *gorm.DB {
		db := config.DB.Model(&models.Expense{}).
			Where("expense_type NOT IN ?", []string{"采购货款", "以旧换新"})
		if !startTime.IsZero() {
			db = db.Where("created_at >= ?", startTime)
		}
		if !endTime.IsZero() {
			db = db.Where("created_at <= ?", endTime)
		}
		return db
	}

	var rows []BreakdownExpenseRow
	if err := scope().
		Select("id, DATE_FORMAT(created_at, '%Y-%m-%d') as created_at, expense_type, amount, remark").
		Order("created_at desc, id desc").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, 0, err
	}

	var total int64
	if err := scope().Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}

	var totalAmount float64
	if err := scope().Select("COALESCE(SUM(amount), 0)").Scan(&totalAmount).Error; err != nil {
		return nil, 0, 0, err
	}
	return rows, total, totalAmount, nil
}

// pendingBalanceBreakdown 待收尾款全量存量明细（忽略期间）：
// 口径与 GetFinanceSummary 一致：payment_status=1 AND order_status=1，金额 = actual_pay_amount - deposit_amount。
func pendingBalanceBreakdown(page, pageSize int) ([]BreakdownOrderRow, int64, float64, error) {
	scope := func() *gorm.DB {
		return config.DB.Model(&models.Order{}).
			Where("payment_status = ? AND order_status = 1", 1)
	}

	var rows []BreakdownOrderRow
	if err := scope().
		Select("id, order_no, customer_name, DATE_FORMAT(created_at, '%Y-%m-%d') as created_at, (actual_pay_amount - deposit_amount) as amount, actual_pay_amount, deposit_amount").
		Order("created_at desc, id desc").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, 0, err
	}

	var total int64
	if err := scope().Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}

	var totalAmount float64
	if err := scope().Select("COALESCE(SUM(actual_pay_amount - deposit_amount), 0)").Scan(&totalAmount).Error; err != nil {
		return nil, 0, 0, err
	}
	return rows, total, totalAmount, nil
}

// pendingSubsidyBreakdown 国补待回款全量存量明细（忽略期间）：
// 逐单可用待回款 = GREATEST(subsidy_pending_amount - subsidy_received_amt, 0)，与卡片口径一致。
func pendingSubsidyBreakdown(page, pageSize int) ([]BreakdownOrderRow, int64, float64, error) {
	scope := func() *gorm.DB {
		return config.DB.Model(&models.Order{}).
			Where("order_status = 1 AND subsidy_pending_amount > 0")
	}

	var rows []BreakdownOrderRow
	if err := scope().
		Select("id, order_no, customer_name, DATE_FORMAT(created_at, '%Y-%m-%d') as created_at, "+pendingSubsidyExpr+" as amount, subsidy_pending_amount, subsidy_received_amt").
		Order("created_at desc, id desc").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, 0, err
	}

	var total int64
	if err := scope().Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}

	var totalAmount float64
	if err := scope().Select("COALESCE(SUM("+pendingSubsidyExpr+"), 0)").Scan(&totalAmount).Error; err != nil {
		return nil, 0, 0, err
	}
	return rows, total, totalAmount, nil
}

// employeeOrdersBreakdown 员工订单明细：归属表达式与 GetFinanceSummary 员工排行完全一致。
// employeeID 取值：'boss'=老板客户（含管理员开单无员工ID），'999999'=商会订单，其余为真实员工ID。
func employeeOrdersBreakdown(employeeID string, startTime, endTime time.Time, page, pageSize int) ([]BreakdownOrderRow, int64, float64, error) {
	if employeeID == "" {
		return nil, 0, 0, &invalidParamError{msg: "缺少员工归属参数"}
	}
	if employeeID != "boss" && employeeID != "999999" {
		if _, convErr := strconv.Atoi(employeeID); convErr != nil {
			return nil, 0, 0, &invalidParamError{msg: "员工归属参数无效"}
		}
	}

	// 与排行一致：管理端开单无 employee_id 按"老板客户"归属，显式标记也归入。
	bossOrderExpr := "orders.referrer_name = '老板客户' OR orders.employee_id IS NULL"
	scope := func() *gorm.DB {
		db := config.DB.Table("orders").
			Joins("LEFT JOIN employees ON orders.employee_id = employees.id").
			Where("orders.order_status = 1").
			Where("(orders.order_type = 2 OR " + bossOrderExpr + " OR employees.is_outsourced = ?)", false)
		if !startTime.IsZero() {
			db = db.Where("orders.created_at >= ?", startTime)
		}
		if !endTime.IsZero() {
			db = db.Where("orders.created_at <= ?", endTime)
		}
		switch employeeID {
		case "999999":
			db = db.Where("orders.order_type = 2")
		case "boss":
			db = db.Where("orders.order_type != 2 AND (orders.referrer_name = ? OR orders.employee_id IS NULL)", "老板客户")
		default:
			db = db.Where("orders.order_type != 2 AND orders.employee_id = ?", employeeID)
		}
		return db
	}

	var rows []BreakdownOrderRow
	if err := scope().
		Select("orders.id, orders.order_no, orders.customer_name, DATE_FORMAT(orders.created_at, '%Y-%m-%d') as created_at, (orders.total_amount - orders.discount_amount) as amount").
		Order("orders.created_at desc, orders.id desc").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, 0, err
	}

	var total int64
	if err := scope().Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}

	var totalAmount float64
	if err := scope().Select("COALESCE(SUM(orders.total_amount - orders.discount_amount), 0)").Scan(&totalAmount).Error; err != nil {
		return nil, 0, 0, err
	}
	return rows, total, totalAmount, nil
}

type invalidParamError struct{ msg string }

func (e *invalidParamError) Error() string { return e.msg }

// accountLogsBreakdown 账户流水明细：FinancialLog（1收2支，排除成对冲销），支持 account_id 过滤。
func accountLogsBreakdown(c *gin.Context, startTime, endTime time.Time, page, pageSize int) ([]BreakdownLogRow, int64, float64, error) {
	accountIDStr := c.Query("account_id")
	var accountID int64
	if accountIDStr != "" {
		parsed, convErr := strconv.ParseInt(accountIDStr, 10, 64)
		if convErr != nil || parsed < 1 {
			return nil, 0, 0, &invalidParamError{msg: "账户参数无效"}
		}
		accountID = parsed
	}

	scope := func() *gorm.DB {
		db := config.DB.Model(&models.FinancialLog{}).
			Where("flow_type IN ? AND reversal_pair_id IS NULL", []int{1, 2})
		if accountID > 0 {
			db = db.Where("account_id = ?", accountID)
		}
		if !startTime.IsZero() {
			db = db.Where("created_at >= ?", startTime)
		}
		if !endTime.IsZero() {
			db = db.Where("created_at <= ?", endTime)
		}
		return db
	}

	var rows []BreakdownLogRow
	if err := scope().
		Select("id, DATE_FORMAT(created_at, '%Y-%m-%d') as created_at, flow_type, category, amount, balance_after, counterparty, related_no, remark").
		Order("created_at desc, id desc").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, 0, err
	}

	var total int64
	if err := scope().Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}

	var totalAmount float64
	if err := scope().Select("COALESCE(SUM(amount), 0)").Scan(&totalAmount).Error; err != nil {
		return nil, 0, 0, err
	}
	return rows, total, totalAmount, nil
}
