package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ledgerAccountNames 非资金账户（台账账户）名单。
// 记账规则下"经营开销"(id=7)与"历史调整台账"(id=6)只是记账口径账户，
// 不代表真实可支配资金，资金工作台的账户/现金流一律排除。
// 沿用前端既有按账户名过滤的约定（useBookkeeping/useDashboard 同款）。
var ledgerAccountNames = []string{"经营开销", "历史调整台账"}

func workspaceRound(value float64) float64 {
	return math.Round(value*100) / 100
}

// workspaceTodoItem 待办明细行（单号/客户姓名/录单日期/金额，用户规矩：必须带客户与录单日期）。
type workspaceTodoItem struct {
	OrderID      uint      `json:"order_id"`
	OrderNo      string    `json:"order_no"`
	CustomerName string    `json:"customer_name"`
	CreatedAt    time.Time `json:"created_at"`
	Amount       float64   `json:"amount"`
}

type workspaceTodo struct {
	Count  int64               `json:"count"`
	Amount float64             `json:"amount"`
	Items  []workspaceTodoItem `json:"items"`
}

// workspaceSubsidyTodo 国补待办（status=1 待提交资料 / status=2 到账待核销）。
func workspaceSubsidyTodo(status int) workspaceTodo {
	todo := workspaceTodo{Items: []workspaceTodoItem{}}
	var rows []struct {
		Count  int64
		Amount float64
	}
	config.DB.Model(&models.Order{}).
		Where("order_status = 1 AND subsidy_status = ? AND subsidy_pending_amount > 0", status).
		Select("COUNT(*) AS count, COALESCE(SUM(subsidy_pending_amount - subsidy_received_amt), 0) AS amount").
		Scan(&rows)
	if len(rows) > 0 {
		todo.Count, todo.Amount = rows[0].Count, workspaceRound(rows[0].Amount)
	}
	config.DB.Model(&models.Order{}).
		Where("order_status = 1 AND subsidy_status = ? AND subsidy_pending_amount > 0", status).
		Select("id, order_no, customer_name, created_at, COALESCE(subsidy_pending_amount - subsidy_received_amt, 0) AS amount").
		Order("created_at asc").Limit(5).
		Scan(&todo.Items)
	return todo
}

// GetFinanceWorkspace 资金工作台整页聚合数据
// @Summary 资金工作台聚合
// @Description 一次返回待办、资金、月度经营概况（含桥接对平）、最近流水与异常提醒；month 支持 YYYY-MM 或 cumulative
// @Tags Finance
// @Produce json
// @Param month query string false "月份 YYYY-MM 或 cumulative，默认当月"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_finance_workspace
// @Router /api/finance/workspace [get]
func GetFinanceWorkspace(c *gin.Context) {
	monthParam := c.DefaultQuery("month", time.Now().Format("2006-01"))
	cumulative := monthParam == "cumulative"
	var startTime, endTime time.Time
	if !cumulative {
		parsed, err := time.ParseInLocation("2006-01", monthParam, time.Now().Location())
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "month 参数应为 YYYY-MM 或 cumulative"})
			return
		}
		startTime = parsed
		endTime = startTime.AddDate(0, 1, 0).Add(-time.Nanosecond)
	} else {
		// 累计：start 取零值（命中所有历史），end 取当前之后
		startTime = time.Time{}
		endTime = time.Now().AddDate(0, 0, 1)
	}

	// ---------- 1. todos 待我处理 ----------
	subsidyToAllocate := workspaceSubsidyTodo(2)
	subsidyToSubmit := workspaceSubsidyTodo(1)

	// 尾款待收：沿用看板既有口径 actual_pay_amount - deposit_amount
	// （actual_pay 已扣除国补待回款、以旧换新与抹零，比"总额-定金-国补"更准确）。
	var balanceDue struct {
		Count  int64
		Amount float64
	}
	config.DB.Model(&models.Order{}).
		Where("payment_status = 1 AND order_status = 1").
		Select("COUNT(*) AS count, COALESCE(SUM(actual_pay_amount - deposit_amount), 0) AS amount").
		Scan(&balanceDue)

	// ---------- 2. funds 钱在哪 ----------
	var accounts []models.Account
	if err := config.DB.Where("name NOT IN ?", ledgerAccountNames).Order("id asc").Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取资金账户失败"})
		return
	}
	fundIDs := make([]uint, 0, len(accounts))
	totalBalance := 0.0
	accountViews := make([]gin.H, 0, len(accounts))
	for _, account := range accounts {
		fundIDs = append(fundIDs, account.ID)
		totalBalance += account.Balance
		accountViews = append(accountViews, gin.H{
			"id":      account.ID,
			"name":    account.Name,
			"balance": workspaceRound(account.Balance),
		})
	}

	// 公户冻结：已提交资料待回款的国补额（用户拍板口径）
	var frozen struct {
		Count  int64
		Amount float64
	}
	config.DB.Model(&models.Order{}).
		Where("subsidy_status = 2 AND order_status = 1 AND subsidy_pending_amount > 0").
		Select("COUNT(*) AS count, COALESCE(SUM(subsidy_pending_amount - subsidy_received_amt), 0) AS amount").
		Scan(&frozen)
	frozenAmount := workspaceRound(frozen.Amount)

	// ---------- 3. month 本月概况 ----------
	_, grossProfit, uncostedCount, grossErr := monthlyGrossProfitStats(startTime, endTime)
	if grossErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取毛利失败"})
		return
	}
	grossProfit = workspaceRound(grossProfit)

	// 经营开销（台账账户支出流水，工资单列）——与 GetRealProfit 完全同口径；
	// 工资按 related_no 所属月归属（月初发上月工资计入上月），非工资按 created_at
	var expenseTotal float64
	expenseQuery := config.DB.Model(&models.FinancialLog{}).
		Where("account_id = ? AND flow_type = ?", operatingExpenseAccountID, 2)
	if !cumulative {
		expenseQuery = expenseQuery.Where("created_at >= ? AND created_at <= ?", startTime, endTime)
	}
	if err := expenseQuery.Select("COALESCE(SUM(CASE WHEN category = '工资' THEN 0 ELSE amount END), 0)").
		Scan(&expenseTotal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取经营开销失败"})
		return
	}
	expenseTotal = workspaceRound(expenseTotal)
	payrollTotalRaw, payrollErr := monthlyPayrollTotal(startTime, endTime)
	if payrollErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取工资支出失败"})
		return
	}
	payrollTotal := workspaceRound(payrollTotalRaw)

	// 净利润（经营口径，扣费用不含工资）；真实净利润再扣工资
	netProfit := workspaceRound(grossProfit - expenseTotal)
	realProfit := workspaceRound(netProfit - payrollTotal)

	// 净现金流：资金账户（非台账）流入-流出；期末余额即当前资金账户合计
	var cashRow struct {
		CashIn  float64
		CashOut float64
	}
	cashQuery := config.DB.Model(&models.FinancialLog{}).Where("account_id IN ?", fundIDs)
	if !cumulative {
		cashQuery = cashQuery.Where("created_at >= ? AND created_at <= ?", startTime, endTime)
	}
	if err := cashQuery.Select(
		"COALESCE(SUM(CASE WHEN flow_type = 1 THEN amount ELSE 0 END), 0) AS cash_in, " +
			"COALESCE(SUM(CASE WHEN flow_type = 2 THEN amount ELSE 0 END), 0) AS cash_out",
	).Scan(&cashRow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取现金流失败"})
		return
	}
	cashIn := workspaceRound(cashRow.CashIn)
	cashOut := workspaceRound(cashRow.CashOut)
	netCashFlow := workspaceRound(cashIn - cashOut)

	// 本月记账笔数（问候语用，全账户口径）
	var bookedCount int64
	bookedQuery := config.DB.Model(&models.FinancialLog{})
	if !cumulative {
		bookedQuery = bookedQuery.Where("created_at >= ? AND created_at <= ?", startTime, endTime)
	}
	bookedQuery.Count(&bookedCount)

	// ---------- 4. recent_flows 最近 10 条资金流水 ----------
	var flows []models.FinancialLog
	config.DB.Preload("Account").
		Where("account_id IN ?", fundIDs).
		Order("created_at desc, id desc").Limit(10).
		Find(&flows)
	relatedNos := map[string]struct{}{}
	for _, flow := range flows {
		if flow.RelatedNo != "" {
			relatedNos[flow.RelatedNo] = struct{}{}
		}
	}
	customerByOrderNo := map[string]string{}
	if len(relatedNos) > 0 {
		keys := make([]string, 0, len(relatedNos))
		for orderNo := range relatedNos {
			keys = append(keys, orderNo)
		}
		var orders []models.Order
		config.DB.Select("order_no, customer_name").Where("order_no IN ?", keys).Find(&orders)
		for _, order := range orders {
			customerByOrderNo[order.OrderNo] = order.CustomerName
		}
	}
	recentFlows := make([]gin.H, 0, len(flows))
	for _, flow := range flows {
		accountName := ""
		if flow.Account != nil {
			accountName = flow.Account.Name
		}
		recentFlows = append(recentFlows, gin.H{
			"id":            flow.ID,
			"created_at":    flow.CreatedAt,
			"account_name":  accountName,
			"category":      flow.Category,
			"flow_type":     flow.FlowType,
			"amount":        workspaceRound(flow.Amount),
			"related_no":    flow.RelatedNo,
			"customer_name": customerByOrderNo[flow.RelatedNo],
			"remark":        flow.Remark,
		})
	}

	// ---------- 5. alerts 已知异常 ----------
	var cancelledPending struct {
		Count  int64
		Amount float64
	}
	config.DB.Model(&models.Order{}).
		Where("order_status = 2 AND subsidy_pending_amount > 0").
		Select("COUNT(*) AS count, COALESCE(SUM(subsidy_pending_amount - subsidy_received_amt), 0) AS amount").
		Scan(&cancelledPending)
	alerts := []gin.H{}
	if cancelledPending.Count > 0 {
		alerts = append(alerts, gin.H{
			"type":    "cancelled_order_with_pending_subsidy",
			"count":   cancelledPending.Count,
			"amount":  workspaceRound(cancelledPending.Amount),
			"message": fmt.Sprintf("有 %d 笔已取消订单仍挂着国补待回款 ¥%.2f，需要在数据体检中清理", cancelledPending.Count, cancelledPending.Amount),
		})
	}

	// ---------- 桥接对平：真实净利润 vs 净现金流 ----------
	bridge := gin.H{
		// 已计入毛利但尚未收现的国补（status=2 订单剩余待回款）
		"subsidy_unreceived": frozenAmount,
		// 已扣减真实净利润但不动资金账户的经营开销（台账账）
		"operating_expense": workspaceRound(expenseTotal + payrollTotal),
		"payroll":           payrollTotal,
		"net_cash_flow":     netCashFlow,
		"cash_in":           cashIn,
		"cash_out":          cashOut,
		// 差异 = 真实净利润 - 净现金流（正值一般意味着有未收现收入/台账费用未付现）
		"profit_cash_gap": workspaceRound(realProfit - netCashFlow),
	}

	monthLabel := monthParam
	if cumulative {
		monthLabel = "cumulative"
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "success",
		"data": gin.H{
			"todos": gin.H{
				"subsidy_to_allocate": subsidyToAllocate,
				"subsidy_to_submit":   subsidyToSubmit,
				"balance_due": gin.H{
					"count":  balanceDue.Count,
					"amount": workspaceRound(balanceDue.Amount),
				},
			},
			"funds": gin.H{
				"accounts":  accountViews,
				"frozen":    gin.H{"count": frozen.Count, "amount": frozenAmount},
				"available": workspaceRound(totalBalance - frozenAmount),
				"total":     workspaceRound(totalBalance),
			},
			"month": gin.H{
				"month":                     monthLabel,
				"gross_profit":              grossProfit,
				"net_profit":                netProfit,
				"expense_total":             expenseTotal,
				"payroll_total":             payrollTotal,
				"real_profit":               realProfit,
				"uncosted_order_item_count": uncostedCount,
				"net_cash_flow":             netCashFlow,
				"ending_balance":            workspaceRound(totalBalance),
				"booked_count":              bookedCount,
				"bridge":                    bridge,
			},
			"recent_flows": recentFlows,
			"alerts":       alerts,
		},
	})
}
