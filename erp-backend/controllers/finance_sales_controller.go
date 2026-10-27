package controllers

import (
	"erp-backend/config"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// salesAnalysisRound 金额统一保留两位
func salesAnalysisRound(value float64) float64 {
	return math.Round(value*100) / 100
}

// ---------- 响应结构 ----------

// salesEmployeeRow 员工业绩排行行。
// 口径：销售额/成本/毛利取订单明细（商品口径，含赠品成本）；订单数/提成为订单级。
type salesEmployeeRow struct {
	EmployeeID   *uint   `json:"employee_id"`
	EmployeeName string  `json:"employee_name"`
	OrderCount   int64   `json:"order_count"`
	SalesAmount  float64 `json:"sales_amount"`
	CostAmount   float64 `json:"cost_amount"`
	GrossProfit  float64 `json:"gross_profit"`
	Commission   float64 `json:"commission"`
}

// salesCategoryRow 品类销售排行行
type salesCategoryRow struct {
	CategoryID   *uint   `json:"category_id"`
	CategoryName string  `json:"category_name"`
	OrderCount   int64   `json:"order_count"`
	Qty          int64   `json:"qty"`
	SalesAmount  float64 `json:"sales_amount"`
	CostAmount   float64 `json:"cost_amount"`
	GrossProfit  float64 `json:"gross_profit"`
}

// salesProductRow 热销商品行
type salesProductRow struct {
	ProductID    uint    `json:"product_id"`
	ProductName  string  `json:"product_name"`
	CategoryName string  `json:"category_name"`
	Qty          int64   `json:"qty"`
	SalesAmount  float64 `json:"sales_amount"`
	GrossProfit  float64 `json:"gross_profit"`
}

// salesEmployeeCategoryRow 员工×品类交叉行（哪个员工哪个品类卖了多少）
type salesEmployeeCategoryRow struct {
	EmployeeID  *uint   `json:"employee_id"`
	CategoryID  *uint   `json:"category_id"`
	Qty         int64   `json:"qty"`
	SalesAmount float64 `json:"sales_amount"`
}

// salesPaymentRow 支付方式分布行
type salesPaymentRow struct {
	PaymentMethod int     `json:"payment_method"`
	OrderCount    int64   `json:"order_count"`
	Amount        float64 `json:"amount"`
}

// salesSummary 销售分析总览（环比仅单月模式返回）
type salesSummary struct {
	OrderCount        int64    `json:"order_count"`
	SalesAmount       float64  `json:"sales_amount"`
	AvgTicket         float64  `json:"avg_ticket"`
	GrossProfit       float64  `json:"gross_profit"`
	SubsidyOrderCount int64    `json:"subsidy_order_count"`
	SubsidyAmount     float64  `json:"subsidy_amount"`
	TradeInAmount     float64  `json:"trade_in_amount"`
	RefundAmount      float64  `json:"refund_amount"`
	CancelCount       int64    `json:"cancel_count"`
	PrevOrderCount    *int64   `json:"prev_order_count,omitempty"`
	PrevSalesAmount   *float64 `json:"prev_sales_amount,omitempty"`
	PrevGrossProfit   *float64 `json:"prev_gross_profit,omitempty"`
}

var salesPaymentMethodNames = map[int]string{
	1: "支付宝",
	2: "微信",
	3: "公户",
	4: "混合支付",
	5: "现金",
}

// GetFinanceSalesAnalysis 财务面板销售分析聚合
// @Summary 销售分析聚合（员工/品类/商品/支付方式）
// @Description 一次返回指定月份（或 cumulative 累计）的员工业绩排行、品类排行、员工×品类交叉、热销商品 TOP、支付方式分布与总览环比
// @Tags Finance
// @Produce json
// @Param month query string false "月份 YYYY-MM 或 cumulative，默认当月"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_finance_sales_analysis
// @Router /api/finance/sales-analysis [get]
func GetFinanceSalesAnalysis(c *gin.Context) {
	monthParam := c.DefaultQuery("month", time.Now().Format("2006-01"))
	cumulative := monthParam == "cumulative"
	var startTime, endTime, prevStart, prevEnd time.Time
	if !cumulative {
		parsed, err := time.ParseInLocation("2006-01", monthParam, time.Now().Location())
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "month 参数应为 YYYY-MM 或 cumulative"})
			return
		}
		startTime = parsed
		endTime = startTime.AddDate(0, 1, 0).Add(-time.Nanosecond)
		// 注意：不能用 endTime.AddDate(0,-1,0)——30 天月份会溢出（如 10-31 减一月 → 9-31 → 规范化为 10-01），
		// 导致环比窗口吞掉下月 1 日。必须从上月 1 号重新推算月末。
		prevStart = startTime.AddDate(0, -1, 0)
		prevEnd = prevStart.AddDate(0, 1, 0).Add(-time.Nanosecond)
	} else {
		startTime = time.Time{}
		endTime = time.Now().AddDate(0, 0, 1)
	}

	// 明细级统计的公共 WHERE：只统计正常订单
	itemJoin := "order_items oi JOIN orders o ON oi.order_id = o.id"
	itemWhere := "o.order_status = 1 AND o.created_at >= ? AND o.created_at <= ?"

	// ---------- 1. 员工业绩 ----------
	// 1a. 商品口径（明细聚合）
	var employeeItemRows []struct {
		EmployeeID  *uint
		Name        string
		EmpNo       string
		SalesAmount float64
		CostAmount  float64
	}
	if err := config.DB.Table(itemJoin).
		Joins("LEFT JOIN employees e ON e.id = o.employee_id").
		Where(itemWhere, startTime, endTime).
		Select("o.employee_id AS employee_id, COALESCE(e.name, '未分配') AS name, COALESCE(e.emp_no, '') AS emp_no, " +
			"COALESCE(SUM(oi.unit_price * oi.quantity), 0) AS sales_amount, " +
			"COALESCE(SUM(oi.unit_cost * oi.quantity), 0) AS cost_amount").
		Group("o.employee_id, e.name, e.emp_no").
		Scan(&employeeItemRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取员工业绩失败"})
		return
	}
	// 1b. 订单级（订单数 + 提成）
	var employeeOrderRows []struct {
		EmployeeID *uint
		OrderCount int64
		Commission float64
	}
	if err := config.DB.Table("orders o").
		Where(itemWhere, startTime, endTime).
		Select("o.employee_id AS employee_id, COUNT(*) AS order_count, COALESCE(SUM(o.commission_amount), 0) AS commission").
		Group("o.employee_id").
		Scan(&employeeOrderRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取员工业绩失败"})
		return
	}
	orderByEmp := map[uint]struct {
		Count      int64
		Commission float64
	}{}
	var nilEmpOrderCount int64
	for _, row := range employeeOrderRows {
		if row.EmployeeID != nil {
			orderByEmp[*row.EmployeeID] = struct {
				Count      int64
				Commission float64
			}{row.OrderCount, row.Commission}
		} else {
			nilEmpOrderCount = row.OrderCount
		}
	}
	// 老板客（BOSS001）业绩单独返回，不与员工混排
	var bossRow *salesEmployeeRow
	var bossEmployeeID *uint
	employees := make([]salesEmployeeRow, 0, len(employeeItemRows))
	for _, row := range employeeItemRows {
		item := salesEmployeeRow{
			EmployeeID:   row.EmployeeID,
			EmployeeName: row.Name,
			SalesAmount:  salesAnalysisRound(row.SalesAmount),
			CostAmount:   salesAnalysisRound(row.CostAmount),
			GrossProfit:  salesAnalysisRound(row.SalesAmount - row.CostAmount),
		}
		if row.EmployeeID != nil {
			if orderStat, ok := orderByEmp[*row.EmployeeID]; ok {
				item.OrderCount = orderStat.Count
				item.Commission = salesAnalysisRound(orderStat.Commission)
			}
		} else {
			item.OrderCount = nilEmpOrderCount
		}
		if row.EmpNo == "BOSS001" {
			boss := item
			bossRow = &boss
			bossEmployeeID = row.EmployeeID
			continue
		}
		employees = append(employees, item)
	}
	// 按销售额降序
	for i := 0; i < len(employees); i++ {
		for j := i + 1; j < len(employees); j++ {
			if employees[j].SalesAmount > employees[i].SalesAmount {
				employees[i], employees[j] = employees[j], employees[i]
			}
		}
	}

	// ---------- 2. 品类排行 ----------
	var categoryRows []salesCategoryRow
	if err := config.DB.Table(itemJoin).
		Joins("JOIN products p ON p.id = oi.product_id").
		Joins("LEFT JOIN categories cat ON cat.id = p.category_id").
		Where(itemWhere, startTime, endTime).
		Select("p.category_id AS category_id, COALESCE(cat.name, '未分类') AS category_name, " +
			"COUNT(DISTINCT o.id) AS order_count, COALESCE(SUM(oi.quantity), 0) AS qty, " +
			"COALESCE(SUM(oi.unit_price * oi.quantity), 0) AS sales_amount, " +
			"COALESCE(SUM(oi.unit_cost * oi.quantity), 0) AS cost_amount").
		Group("p.category_id, cat.name").
		Scan(&categoryRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取品类统计失败"})
		return
	}
	for i := range categoryRows {
		categoryRows[i].SalesAmount = salesAnalysisRound(categoryRows[i].SalesAmount)
		categoryRows[i].CostAmount = salesAnalysisRound(categoryRows[i].CostAmount)
		categoryRows[i].GrossProfit = salesAnalysisRound(categoryRows[i].SalesAmount - categoryRows[i].CostAmount)
	}
	for i := 0; i < len(categoryRows); i++ {
		for j := i + 1; j < len(categoryRows); j++ {
			if categoryRows[j].SalesAmount > categoryRows[i].SalesAmount {
				categoryRows[i], categoryRows[j] = categoryRows[j], categoryRows[i]
			}
		}
	}

	// ---------- 3. 员工×品类交叉 ----------
	var crossRows []salesEmployeeCategoryRow
	if err := config.DB.Table(itemJoin).
		Joins("JOIN products p ON p.id = oi.product_id").
		Where(itemWhere, startTime, endTime).
		Select("o.employee_id AS employee_id, p.category_id AS category_id, " +
			"COALESCE(SUM(oi.quantity), 0) AS qty, COALESCE(SUM(oi.unit_price * oi.quantity), 0) AS sales_amount").
		Group("o.employee_id, p.category_id").
		Scan(&crossRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取员工品类交叉数据失败"})
		return
	}
	for i := range crossRows {
		crossRows[i].SalesAmount = salesAnalysisRound(crossRows[i].SalesAmount)
	}
	// 交叉表只保留员工（老板客单独展示，不进热力图）
	if bossEmployeeID != nil {
		filtered := crossRows[:0]
		for _, row := range crossRows {
			if row.EmployeeID == nil || *row.EmployeeID != *bossEmployeeID {
				filtered = append(filtered, row)
			}
		}
		crossRows = filtered
	}

	// ---------- 4. 热销商品 TOP10 ----------
	var topProducts []salesProductRow
	if err := config.DB.Table(itemJoin).
		Joins("JOIN products p ON p.id = oi.product_id").
		Joins("LEFT JOIN categories cat ON cat.id = p.category_id").
		Where(itemWhere, startTime, endTime).
		Select("oi.product_id AS product_id, p.name AS product_name, COALESCE(cat.name, '未分类') AS category_name, " +
			"COALESCE(SUM(oi.quantity), 0) AS qty, COALESCE(SUM(oi.unit_price * oi.quantity), 0) AS sales_amount, " +
			"COALESCE(SUM((oi.unit_price - oi.unit_cost) * oi.quantity), 0) AS gross_profit").
		Group("oi.product_id, p.name, cat.name").
		Order("sales_amount DESC").Limit(10).
		Scan(&topProducts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取热销商品失败"})
		return
	}
	for i := range topProducts {
		topProducts[i].SalesAmount = salesAnalysisRound(topProducts[i].SalesAmount)
		topProducts[i].GrossProfit = salesAnalysisRound(topProducts[i].GrossProfit)
	}

	// ---------- 5. 支付方式分布 ----------
	var paymentRows []salesPaymentRow
	if err := config.DB.Table("orders o").
		Where(itemWhere, startTime, endTime).
		Select("o.payment_method, COUNT(*) AS order_count, COALESCE(SUM(o.total_amount - o.discount_amount), 0) AS amount").
		Group("o.payment_method").
		Scan(&paymentRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取支付方式分布失败"})
		return
	}
	for i := range paymentRows {
		paymentRows[i].Amount = salesAnalysisRound(paymentRows[i].Amount)
	}

	// ---------- 6. 总览 + 环比 ----------
	summary := salesSummary{}
	var summaryRow struct {
		OrderCount    int64
		SalesAmount   float64
		SubsidyCount  int64
		SubsidyAmount float64
		TradeInAmount float64
	}
	if err := config.DB.Table("orders o").
		Where(itemWhere, startTime, endTime).
		Select("COUNT(*) AS order_count, COALESCE(SUM(o.total_amount - o.discount_amount), 0) AS sales_amount, " +
			"COALESCE(SUM(CASE WHEN o.subsidy_status > 0 THEN 1 ELSE 0 END), 0) AS subsidy_count, " +
			"COALESCE(SUM(o.subsidy_amount), 0) AS subsidy_amount, " +
			"COALESCE(SUM(o.trade_in_amount), 0) AS trade_in_amount").
		Scan(&summaryRow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取销售总览失败"})
		return
	}
	_, grossProfit, _, grossErr := monthlyGrossProfitStats(startTime, endTime)
	if grossErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取毛利失败"})
		return
	}
	// 本期取消单（参考值）
	config.DB.Table("orders o").
		Where("o.order_status = 2 AND o.created_at >= ? AND o.created_at <= ?", startTime, endTime).
		Count(&summary.CancelCount)
	// 本期退款金额（资金流水退款红冲，资金账户口径，排除台账账户）
	// 退款类分类：订单取消退款 / 订单红冲 / 退定金 / 手工记账退款等
	var refundAmount float64
	config.DB.Table("financial_logs fl").
		Joins("JOIN accounts a ON a.id = fl.account_id").
		Where("fl.flow_type = 2 AND (fl.category LIKE ? OR fl.category LIKE ? OR fl.category LIKE ?) AND fl.created_at >= ? AND fl.created_at <= ? AND a.name NOT IN ?",
			"%退款%", "%红冲%", "%退定金%", startTime, endTime, ledgerAccountNames).
		Select("COALESCE(SUM(fl.amount), 0)").
		Scan(&refundAmount)

	summary.OrderCount = summaryRow.OrderCount
	summary.SalesAmount = salesAnalysisRound(summaryRow.SalesAmount)
	if summaryRow.OrderCount > 0 {
		summary.AvgTicket = salesAnalysisRound(summaryRow.SalesAmount / float64(summaryRow.OrderCount))
	}
	summary.GrossProfit = salesAnalysisRound(grossProfit)
	summary.SubsidyOrderCount = summaryRow.SubsidyCount
	summary.SubsidyAmount = salesAnalysisRound(summaryRow.SubsidyAmount)
	summary.TradeInAmount = salesAnalysisRound(summaryRow.TradeInAmount)
	summary.RefundAmount = salesAnalysisRound(refundAmount)

	if !cumulative {
		var prevRow struct {
			OrderCount  int64
			SalesAmount float64
		}
		if err := config.DB.Table("orders o").
			Where("o.order_status = 1 AND o.created_at >= ? AND o.created_at <= ?", prevStart, prevEnd).
			Select("COUNT(*) AS order_count, COALESCE(SUM(o.total_amount - o.discount_amount), 0) AS sales_amount").
			Scan(&prevRow).Error; err == nil {
			_, prevGross, _, prevGrossErr := monthlyGrossProfitStats(prevStart, prevEnd)
			if prevGrossErr == nil {
				summary.PrevOrderCount = &prevRow.OrderCount
				prevSales := salesAnalysisRound(prevRow.SalesAmount)
				summary.PrevSalesAmount = &prevSales
				prevGrossRounded := salesAnalysisRound(prevGross)
				summary.PrevGrossProfit = &prevGrossRounded
			}
		}
	}

	// 未设分类的在售商品数（含分类被删导致失联的），提醒用户去商品管理补分类
	var uncategorizedCount int64
	config.DB.Table("products p").
		Joins("LEFT JOIN categories cat2 ON cat2.id = p.category_id AND cat2.deleted_at IS NULL").
		Where("p.deleted_at IS NULL AND (p.category_id IS NULL OR p.category_id = 0 OR cat2.id IS NULL)").
		Count(&uncategorizedCount)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "success",
		"data": gin.H{
			"month":                  monthParam,
			"summary":                summary,
			"employees":              employees,
			"boss":                   bossRow,
			"categories":             categoryRows,
			"employee_category":      crossRows,
			"top_products":           topProducts,
			"payment_methods":        paymentRows,
			"payment_names":          salesPaymentMethodNames,
			"uncategorized_products": uncategorizedCount,
		},
	})
}
