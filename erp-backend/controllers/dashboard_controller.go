package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"erp-backend/services"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ProductSaleStat 产品销售统计
type ProductSaleStat struct {
	ProductID uint    `json:"product_id"`
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	Revenue   float64 `json:"revenue"`
}

// EmployeeRankStat 员工销售排名
type EmployeeRankStat struct {
	EmployeeID             uint    `json:"employee_id"`
	Name                   string  `json:"name"`
	OrderCount             int     `json:"order_count"`
	Revenue                float64 `json:"revenue"`
	GrossProfit            float64 `json:"gross_profit"`
	UncostedOrderItemCount int64   `json:"uncosted_order_item_count"`
}

// CategorySaleStat 分类销售统计
type CategorySaleStat struct {
	Name    string  `json:"name"`
	Revenue float64 `json:"revenue"`
}

// DailySaleTrend 每日销售趋势
type DailySaleTrend struct {
	Date       string  `json:"date"`
	Revenue    float64 `json:"revenue"`
	OrderCount int     `json:"order_count"`
}

// FinanceSummaryResponse 财务总览响应数据结构
type FinanceSummaryResponse struct {
	TodayRevenue             float64           `json:"today_revenue"`
	TotalIncome              float64           `json:"total_income"`
	TotalExpense             float64           `json:"total_expense"`
	PendingSubsidy           float64           `json:"pending_subsidy"`
	OrderCount               int64             `json:"order_count"`
	PendingBalance           float64           `json:"pending_balance"`       // 待收尾款总额
	PendingBalanceCount      int64             `json:"pending_balance_count"` // 欠款订单数
	Accounts                 []models.Account  `json:"accounts"`              // 各账户余额
	TopProducts              []ProductSaleStat `json:"top_products"`          // 热销排行
	GrossProfit              float64           `json:"gross_profit"`          // 销售毛利
	CostIncomplete           bool              `json:"cost_incomplete"`
	UncostedOrderItems       int64             `json:"uncosted_order_item_count"`
	PendingZeroCostSaleCount int64             `json:"pending_zero_cost_sale_count"`
	// 定稿工资变动待办：确认发放后订单有变动导致实时回算 ≠ 定稿应发
	PendingPayrollMismatchCount int64                 `json:"pending_payroll_mismatch_count"`
	PendingPayrollMismatches    []PayrollMismatchItem `json:"pending_payroll_mismatches"`
	OperatingExpense            float64               `json:"operating_expense"` // 日常经营费用，不含货款与以旧换新
	NetProfit                   float64               `json:"net_profit"`        // 经营净利润
	// 工程业务线（独立核算，不写资金账户）：按验收/结算日 accept_date 归期，仅统计已结清项目。
	// 零售订单与工程零耦合，因此单列一组字段，由前端「含工程合计」卡展示。
	EngProjectCount   int64              `json:"eng_project_count"`   // 期间内已结清的工程单数
	EngContractTotal  float64            `json:"eng_contract_total"`  // 期间内工程合同额
	EngGrossProfit    float64            `json:"eng_gross_profit"`    // 期间内工程设备毛利（合同额 − 设备成本）
	EngNetProfit      float64            `json:"eng_net_profit"`      // 期间内工程净利（再减工程费用）
	CombinedNetProfit float64            `json:"combined_net_profit"` // 经营净利润 + 工程净利
	EmployeeRankings  []EmployeeRankStat `json:"employee_rankings"`
	CategorySales     []CategorySaleStat `json:"category_sales"`
	SalesTrend        []DailySaleTrend   `json:"sales_trend"`
}

// CalculatePendingSubsidy 计算累计应回款扣除累计回款后的余额。
func CalculatePendingSubsidy(totalGenerated, totalReceived float64) float64 {
	pending := totalGenerated - totalReceived
	if pending < 0 {
		return 0
	}
	return pending
}

// CalculateGrossProfit calculates the profit directly attributable to completed sales.
// Supplier payments are intentionally excluded because the sold-goods cost is already
// recognized through the order-item cost snapshot.
func CalculateGrossProfit(
	revenue float64,
	productCost float64,
	deliveryFee float64,
	installationFee float64,
	referralFee float64,
	commissionAmount float64,
	tradeInAmount float64,
) float64 {
	return revenue - productCost - deliveryFee - installationFee - referralFee - commissionAmount - tradeInAmount
}

// CalculateNetProfit subtracts period operating expenses after direct sales costs.
func CalculateNetProfit(grossProfit, operatingExpense float64) float64 {
	return grossProfit - operatingExpense
}

// sumOperatingExpense 统计期间内经营开销总额，口径与 /api/finance/real-profit 完全一致：
// 取经营开销台账账户（accounts.id=7）的支出流水，非工资按流水发生日 created_at 归期，
// 工资按 related_no（PAY-<YYYY-MM>-<员工ID>）解析出的工资所属月归期，
// 以覆盖"月初发上月工资计入上月"的跨月情况。startTime/endTime 均为零值表示全量口径。
//
// 历史原因：此前该口径读取 models.Expense 表，但该表只在系统记账路径（POST /api/expenses）
// 有数据，全库仅 3 行以旧换新；历史开销账导入的房租、工资等 79 笔只写入了 financial_logs，
// 导致 operating_expense 长期为 0、net_profit 实际等于毛利。改造后统一走流水账户口径。
func sumOperatingExpense(startTime, endTime time.Time) (float64, error) {
	var nonPayroll float64
	nonPayrollDB := config.DB.Model(&models.FinancialLog{}).
		Where("account_id = ? AND flow_type = ?", operatingExpenseAccountID, 2)
	if !startTime.IsZero() {
		nonPayrollDB = nonPayrollDB.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		nonPayrollDB = nonPayrollDB.Where("created_at <= ?", endTime)
	}
	if err := nonPayrollDB.
		Select("COALESCE(SUM(CASE WHEN category = '工资' THEN 0 ELSE amount END), 0)").
		Scan(&nonPayroll).Error; err != nil {
		return 0, err
	}

	// 工资按所属月归期：全量口径直接合计，期间口径复用 real-profit 的归属月解析。
	var payroll float64
	if startTime.IsZero() && endTime.IsZero() {
		if err := config.DB.Model(&models.FinancialLog{}).
			Where("account_id = ? AND flow_type = ? AND category = ?", operatingExpenseAccountID, 2, "工资").
			Select("COALESCE(SUM(amount), 0)").
			Scan(&payroll).Error; err != nil {
			return 0, err
		}
	} else {
		payrollTotal, err := monthlyPayrollTotal(startTime, endTime)
		if err != nil {
			return 0, err
		}
		payroll = payrollTotal
	}
	return nonPayroll + payroll, nil
}

// resolvePeriodRange 解析 period/start_date/end_date 查询参数为统计起止时间。
// period 支持 today/month/last_month/year/custom/all；其余值按 today 处理（与历史行为一致）。
// 返回零值时间表示全量（period=all）；custom 日期无效时返回错误。
func resolvePeriodRange(c *gin.Context) (time.Time, time.Time, error) {
	period := c.DefaultQuery("period", "today")
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	now := time.Now()
	var startTime time.Time
	var endTime time.Time
	location := now.Location()

	switch period {
	case "today":
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
		endTime = startTime.AddDate(0, 0, 1).Add(-time.Nanosecond)
	case "month":
		startTime = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)
		endTime = startTime.AddDate(0, 1, 0).Add(-time.Nanosecond)
	case "last_month":
		firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)
		startTime = firstOfThisMonth.AddDate(0, -1, 0)
		endTime = firstOfThisMonth.Add(-time.Nanosecond)
	case "year":
		startTime = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, location)
		endTime = startTime.AddDate(1, 0, 0).Add(-time.Nanosecond)
	case "custom":
		start, startErr := time.ParseInLocation("2006-01-02", startDateStr, location)
		end, endErr := time.ParseInLocation("2006-01-02", endDateStr, location)
		if startErr != nil || endErr != nil || end.Before(start) {
			return time.Time{}, time.Time{}, errors.New("自定义日期范围无效")
		}
		startTime = start
		endTime = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 999999999, location)
	case "all":
		// Zero time for all
	default:
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
		endTime = startTime.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}

	return startTime, endTime, nil
}

// GetFinanceSummary 聚合查询财务统计数据
// @Summary 获取财务数据汇总
// @Description 获取财务数据汇总
// @Tags Finance
// @Accept json
// @Param period query string false "时间维度 (today, month, last_month, year, custom, all)"
// @Param start_date query string false "开始日期 (当 period=custom 时有效)"
// @Param end_date query string false "结束日期 (当 period=custom 时有效)"
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_finance_summary
// @Router /api/finance/summary [get]
func GetFinanceSummary(c *gin.Context) {
	var summary FinanceSummaryResponse

	startTime, endTime, periodErr := resolvePeriodRange(c)
	if periodErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": periodErr.Error()})
		return
	}

	// 1. 营业额 (Revenue) - 仅统计正常订单
	db := config.DB.Model(&models.Order{}).Where("order_status = 1")
	if !startTime.IsZero() {
		db = db.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		db = db.Where("created_at <= ?", endTime)
	}
	db.Select("COALESCE(SUM(total_amount - discount_amount), 0)").Scan(&summary.TodayRevenue)

	// 2. 期间现金流入/流出 - 基于财务流水表，排除成对冲销记录。
	incomeDB := config.DB.Model(&models.FinancialLog{}).
		Where("flow_type = ?", 1).
		Where("reversal_pair_id IS NULL")
	if !startTime.IsZero() {
		incomeDB = incomeDB.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		incomeDB = incomeDB.Where("created_at <= ?", endTime)
	}
	incomeDB.Select("COALESCE(SUM(amount), 0)").Scan(&summary.TotalIncome)

	expenseDB := config.DB.Model(&models.FinancialLog{}).
		Where("flow_type = ?", 2).
		Where("reversal_pair_id IS NULL")
	if !startTime.IsZero() {
		expenseDB = expenseDB.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		expenseDB = expenseDB.Where("created_at <= ?", endTime)
	}
	expenseDB.Select("COALESCE(SUM(amount), 0)").Scan(&summary.TotalExpense)

	// 3. 国补待回款：统一口径，逐单 pending 可用额 = GREATEST(subsidy_pending_amount - subsidy_received_amt, 0)。
	//    逐单 clamp（主理人已拍板），与明细钻取接口的合计完全一致，不再依赖生成/回款两个总额相减。
	var pendingSubsidyTotal float64
	config.DB.Model(&models.Order{}).
		Where("order_status = 1 AND subsidy_pending_amount > 0").
		Select("COALESCE(SUM(GREATEST(subsidy_pending_amount - subsidy_received_amt, 0)), 0)").
		Scan(&pendingSubsidyTotal)
	summary.PendingSubsidy = pendingSubsidyTotal

	// 4. 订单数 (OrderCount) - 仅正常订单
	orderCountDB := config.DB.Model(&models.Order{}).Where("order_status = 1")
	if !startTime.IsZero() {
		orderCountDB = orderCountDB.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		orderCountDB = orderCountDB.Where("created_at <= ?", endTime)
	}
	orderCountDB.Count(&summary.OrderCount)

	// 5. 待收尾款总额 (PendingBalance) 和 欠款订单数 (PendingBalanceCount) - 存量数据
	//    逐单 clamp：只统计「实收 < 定金」的欠款单。若直接对全部 payment_status=1 的单求
	//    SUM(actual_pay - deposit)，个别多付单的负差额会把别的单真实欠款冲掉，造成少报。
	config.DB.Model(&models.Order{}).
		Where("payment_status = ? AND order_status = 1 AND actual_pay_amount > deposit_amount", 1).
		Select("COALESCE(SUM(actual_pay_amount - deposit_amount), 0)").
		Scan(&summary.PendingBalance)

	config.DB.Model(&models.Order{}).
		Where("payment_status = ? AND order_status = 1 AND actual_pay_amount > deposit_amount", 1).
		Count(&summary.PendingBalanceCount)

	// 6. 各账户余额
	config.DB.Order("id asc").Find(&summary.Accounts)

	employeeIDStr := c.Query("employee_id")

	// 7. 热销排行
	topDB := config.DB.Table("order_items").
		Select("products.id as product_id, products.name as name, SUM(order_items.quantity) as quantity, SUM(order_items.quantity * order_items.unit_price) as revenue").
		Joins("JOIN products ON order_items.product_id = products.id").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("orders.order_status = 1")

	if !startTime.IsZero() {
		topDB = topDB.Where("orders.created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		topDB = topDB.Where("orders.created_at <= ?", endTime)
	}
	if employeeIDStr == "999999" {
		topDB = topDB.Where("orders.order_type = 2")
	} else if employeeIDStr == "boss" {
		topDB = topDB.Where("orders.order_type != 2 AND orders.referrer_name = ?", "老板客户")
	} else if employeeIDStr != "" {
		topDB = topDB.Where("orders.order_type != 2 AND orders.employee_id = ?", employeeIDStr)
	}

	// LIMIT 10：看板只展示 Top5，留余量；避免订单量增长后每次刷新全量拖回所有商品
	topDB.Group("products.id").Order("quantity desc").Limit(10).Scan(&summary.TopProducts)

	// 8. 销售毛利 (GrossProfit)
	var totalFees struct {
		DeliveryFee      float64
		InstallationFee  float64
		ReferralFee      float64
		CommissionAmount float64
		TradeInAmount    float64
	}
	feeDB := config.DB.Model(&models.Order{}).Where("order_status = 1")
	if !startTime.IsZero() {
		feeDB = feeDB.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		feeDB = feeDB.Where("created_at <= ?", endTime)
	}
	feeDB.Select("COALESCE(SUM(delivery_fee), 0) as delivery_fee, COALESCE(SUM(installation_fee), 0) as installation_fee, COALESCE(SUM(referral_fee), 0) as referral_fee, COALESCE(SUM(commission_amount), 0) as commission_amount, COALESCE(SUM(trade_in_amount), 0) as trade_in_amount").Scan(&totalFees)

	var totalCost float64
	costDB := config.DB.Table("order_items").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("orders.order_status = 1")
	if !startTime.IsZero() {
		costDB = costDB.Where("orders.created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		costDB = costDB.Where("orders.created_at <= ?", endTime)
	}
	if err := costDB.Select("COALESCE(SUM(order_items.unit_cost * order_items.quantity), 0)").Scan(&totalCost).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取订单成本失败"})
		return
	}
	uncostedCostDB := config.DB.Table("order_items").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("orders.order_status = 1 AND order_items.unit_cost <= 0")
	if !startTime.IsZero() {
		uncostedCostDB = uncostedCostDB.Where("orders.created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		uncostedCostDB = uncostedCostDB.Where("orders.created_at <= ?", endTime)
	}
	if err := uncostedCostDB.Count(&summary.UncostedOrderItems).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取待核订单成本数量失败"})
		return
	}
	summary.CostIncomplete = summary.UncostedOrderItems > 0

	// 看板重要事项是全量待处理提醒，不受当前经营统计期间筛选影响。
	pendingZeroCostSalesDB := config.DB.Table("order_items").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("order_items.unit_cost <= 0 AND orders.order_status = ? AND orders.deleted_at IS NULL", 1)
	if err := pendingZeroCostSalesDB.Count(&summary.PendingZeroCostSaleCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取零成本售出提醒失败"})
		return
	}

	// 看板待办：定稿工资变动（确认发放后订单有变动导致实时回算 ≠ 定稿应发），同样全量提醒。
	// 该提醒属辅助核对，读取失败时降级为空，不阻断财务总览主流程。
	if mismatches, mismatchErr := finalizedPayrollMismatches(); mismatchErr == nil {
		summary.PendingPayrollMismatchCount = int64(len(mismatches))
		summary.PendingPayrollMismatches = mismatches
	}

	summary.GrossProfit = CalculateGrossProfit(
		summary.TodayRevenue,
		totalCost,
		totalFees.DeliveryFee,
		totalFees.InstallationFee,
		totalFees.ReferralFee,
		totalFees.CommissionAmount,
		totalFees.TradeInAmount,
	)

	operatingExpense, opErr := sumOperatingExpense(startTime, endTime)
	if opErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取经营开销失败"})
		return
	}
	summary.OperatingExpense = operatingExpense
	summary.NetProfit = CalculateNetProfit(summary.GrossProfit, summary.OperatingExpense)

	// 8.5 工程业务线（独立核算）：工程单不写资金账户，故不参与上面的毛利/开销口径，
	//     这里按验收结算日 accept_date 归期单独聚合，仅统计已结清项目（status=5）。
	//     作废单（status=9）天然排除。
	var engPeriodStart, engPeriodEnd interface{}
	if !startTime.IsZero() {
		engPeriodStart = startTime
	}
	if !endTime.IsZero() {
		engPeriodEnd = endTime
	}
	engDB := config.DB.Model(&models.EngineeringProject{}).
		Where("status = ?", services.EngStatusSettled).
		Where("accept_date IS NOT NULL")
	if engPeriodStart != nil {
		engDB = engDB.Where("accept_date >= ?", engPeriodStart)
	}
	if engPeriodEnd != nil {
		engDB = engDB.Where("accept_date <= ?", engPeriodEnd)
	}
	var engAgg struct {
		ProjectCount  int64   `gorm:"column:project_count"`
		ContractTotal float64 `gorm:"column:contract_total"`
		GrossProfit   float64 `gorm:"column:gross_profit"`
		NetProfit     float64 `gorm:"column:net_profit"`
	}
	engDB.Select("COUNT(*) AS project_count, " +
		"COALESCE(SUM(contract_total), 0) AS contract_total, " +
		"COALESCE(SUM(contract_total - cost_total), 0) AS gross_profit, " +
		"COALESCE(SUM(contract_total - cost_total - expense_total), 0) AS net_profit").
		Scan(&engAgg)
	summary.EngProjectCount = engAgg.ProjectCount
	summary.EngContractTotal = engAgg.ContractTotal
	summary.EngGrossProfit = engAgg.GrossProfit
	summary.EngNetProfit = engAgg.NetProfit
	summary.CombinedNetProfit = summary.NetProfit + engAgg.NetProfit

	// 9. 员工排行 (EmployeeRankings)
	// 管理员端开单没有 employee_id，按老板客户归属；显式标记“老板客户”的订单也归入老板客户。
	// “老板客”是虚拟销售账号（outsourced_type='boss'），其订单归入老板客户统计，不算外协。
	bossOrderExpr := "orders.referrer_name = '老板客户' OR orders.employee_id IS NULL OR employees.outsourced_type = 'boss'"
	bossRankExpr := "IF(orders.order_type = 2, '商会订单', IF(" + bossOrderExpr + ", '老板客户', employees.name))"
	employeeRankIDExpr := "IF(orders.order_type = 2, 999999, IF(" + bossOrderExpr + ", 0, orders.employee_id))"
	// revenue / gross_profit 必须与销售额 KPI、销售趋势同一口径：
	// 营收取净额（total_amount − discount_amount），毛利再扣商品成本与五项费用（含以旧换新）。
	// 此前营收用 total_amount、毛利漏扣 trade_in_amount，导致员工排行合计与顶部 KPI 对不上。
	rankDB := config.DB.Table("orders").
		Select(employeeRankIDExpr + " as employee_id, " + bossRankExpr + " as name, COUNT(orders.id) as order_count, SUM(orders.total_amount - orders.discount_amount) as revenue, SUM(orders.total_amount - orders.discount_amount - COALESCE(cost_tb.total_cost, 0) - orders.delivery_fee - orders.installation_fee - orders.referral_fee - orders.commission_amount - orders.trade_in_amount) as gross_profit, SUM(COALESCE(cost_tb.uncosted_count, 0)) as uncosted_order_item_count").
		Joins("LEFT JOIN employees ON orders.employee_id = employees.id").
		Joins("LEFT JOIN (SELECT order_id, SUM(unit_cost * quantity) as total_cost, SUM(CASE WHEN unit_cost <= 0 THEN 1 ELSE 0 END) as uncosted_count FROM order_items GROUP BY order_id) as cost_tb ON cost_tb.order_id = orders.id").
		Where("orders.order_status = 1").
		// 排除所有非 boss 的外协（install/delivery/未来新类型），否则新增师傅类型时其订单会被静默算成销售；
		// employee_id 为空的订单由 bossOrderExpr 的 orders.employee_id IS NULL 兜住，不能依赖 NULL = 0。
		Where("(orders.order_type = 2 OR " + bossOrderExpr + " OR employees.is_outsourced = 0)")
	if !startTime.IsZero() {
		rankDB = rankDB.Where("orders.created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		rankDB = rankDB.Where("orders.created_at <= ?", endTime)
	}
	if employeeIDStr == "999999" {
		rankDB = rankDB.Where("orders.order_type = 2")
	} else if employeeIDStr == "boss" {
		rankDB = rankDB.Where("orders.order_type != 2 AND (orders.referrer_name = ? OR orders.employee_id IS NULL OR employees.outsourced_type = ?)", "老板客户", "boss")
	} else if employeeIDStr != "" {
		rankDB = rankDB.Where("orders.order_type != 2 AND orders.employee_id = ?", employeeIDStr)
	}
	rankDB.Group(employeeRankIDExpr + ", " + bossRankExpr).Order("revenue desc").Scan(&summary.EmployeeRankings)

	// 10. 分类销售 (CategorySales)
	catDB := config.DB.Table("order_items").
		Select("COALESCE(categories.name, '未分类') as name, SUM(order_items.quantity * order_items.unit_price) as revenue").
		Joins("JOIN products ON order_items.product_id = products.id").
		Joins("LEFT JOIN categories ON products.category_id = categories.id").
		Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("orders.order_status = 1")
	if !startTime.IsZero() {
		catDB = catDB.Where("orders.created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		catDB = catDB.Where("orders.created_at <= ?", endTime)
	}
	// LIMIT 10：同热销排行，看板只展示 Top5
	catDB.Group("products.category_id").Order("revenue desc").Limit(10).Scan(&summary.CategorySales)

	// 11. 销售趋势 (SalesTrend)
	// revenue 口径与销售额 KPI 保持一致：SUM(total_amount - discount_amount)，否则有折扣单时趋势图会高于 KPI
	trendDB := config.DB.Table("orders").
		Select("DATE(created_at) as date, SUM(total_amount - discount_amount) as revenue, COUNT(id) as order_count").
		Where("order_status = 1")
	if !startTime.IsZero() {
		trendDB = trendDB.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		trendDB = trendDB.Where("created_at <= ?", endTime)
	}
	trendDB.Group("DATE(created_at)").Order("date asc").Scan(&summary.SalesTrend)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": summary,
	})
}
