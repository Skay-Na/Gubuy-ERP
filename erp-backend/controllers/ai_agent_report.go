package controllers

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"
)

type agentReportRow struct {
	Period                 agentTaskPeriod    `json:"period"`
	Values                 map[string]float64 `json:"values"`
	Records                int64              `json:"records"`
	Missing                bool               `json:"missing"`
	Partial                bool               `json:"partial_period"`
	CostIncomplete         bool               `json:"cost_incomplete"`
	UncostedOrderItemCount int64              `json:"uncosted_order_item_count,omitempty"`
}
type agentReportChange struct {
	Metric     string   `json:"metric"`
	From       string   `json:"from"`
	To         string   `json:"to"`
	Difference float64  `json:"difference"`
	Percent    *float64 `json:"percent,omitempty"`
}
type agentBusinessReport struct {
	Topic   string              `json:"topic"`
	Basis   string              `json:"basis"`
	Rows    []agentReportRow    `json:"rows"`
	Changes []agentReportChange `json:"changes"`
}

var agentMetricLabels = map[string]string{
	"sales_revenue": "折后销售额", "order_count": "有效订单数", "units": "销售件数", "average_order": "客单价",
	"stock_units": "可用库存件数", "stock_value": "库存平均成本估值", "sample_stock": "样机件数", "main_stock": "主仓件数", "store_stock": "门店件数", "cloud_stock": "云仓件数", "unvalued_count": "缺少平均成本的商品数",
	"gross_profit": "订单毛利", "uncosted_order_item_count": "成本待核订单明细数", "cash_in": "已登记收入", "cash_out": "已登记支出", "net_cashflow": "净收支",
	"subsidy_due": "累计应回款", "subsidy_received": "累计已回款", "subsidy_pending": "尚未回款",
	"salary_payable": "应发工资", "commission_payable": "应计提成", "base_salary": "底薪", "deduction": "请假扣款",
	"salary_paid": "实发工资", "commission_paid": "实发提成",
}

func agentCanReport(task agentTask) bool {
	if len(task.Metrics) == 0 || task.Operation == "forecast" {
		return false
	}
	if task.Topic == "subsidy" && len(task.Periods) == 0 && task.Entity == "" && task.Scope != "one" {
		for _, metric := range task.Metrics {
			switch metric {
			case "subsidy_due", "subsidy_received", "subsidy_pending":
			default:
				return false
			}
		}
		return true
	}
	if task.Topic == "inventory" && len(task.Periods) == 0 && task.Entity == "" && task.Scope != "one" {
		for _, metric := range task.Metrics {
			switch metric {
			case "stock_units", "stock_value", "sample_stock", "main_stock", "store_stock", "cloud_stock", "unvalued_count":
			default:
				return false
			}
		}
		return true
	}
	if len(task.Periods) == 0 || task.Operation == "forecast" {
		return false
	}
	if task.Topic != "payroll" && (task.Entity != "" || task.Scope == "one") {
		return false
	}
	switch task.Topic {
	case "sales":
		for _, metric := range task.Metrics {
			switch metric {
			case "sales_revenue", "order_count", "units", "average_order", "gross_profit":
			default:
				return false
			}
		}
		return true
	case "cashflow":
		for _, metric := range task.Metrics {
			if metric != "cash_in" && metric != "cash_out" && metric != "net_cashflow" {
				return false
			}
		}
		return true
	case "payroll":
		return task.Basis == "payable" || task.Basis == "commission_payable"
	}
	return false
}

func agentBuildReport(ctx context.Context, task agentTask) (agentBusinessReport, error) {
	report := agentBusinessReport{Topic: task.Topic, Rows: []agentReportRow{}, Changes: []agentReportChange{}}
	periods := task.Periods
	if task.Topic == "inventory" || task.Topic == "subsidy" {
		today := time.Now().Format("2006-01-02")
		label := "当前库存"
		if task.Topic == "subsidy" {
			label = "截至目前"
		}
		periods = []agentTaskPeriod{{Label: label, Start: today, End: today}}
	}
	for _, period := range periods {
		start, _ := time.ParseInLocation("2006-01-02", period.Start, time.Local)
		end, _ := time.ParseInLocation("2006-01-02", period.End, time.Local)
		row := agentReportRow{Period: period, Values: map[string]float64{}, Partial: !end.Before(time.Now().Truncate(24 * time.Hour))}
		switch task.Topic {
		case "subsidy":
			report.Basis = "沿用国补概览口径：有效国补订单的应回款金额按新字段优先、旧订单回退国补金额逐单累计；减去已登记的国补结算回款；余额最低为零。"
			overview, err := readSubsidyOverview(ctx)
			if err != nil {
				return report, err
			}
			row.Records = overview.OrderCount + overview.ReceiptCount
			row.Partial = false
			row.Values = map[string]float64{
				"subsidy_due": overview.TotalGenerated, "subsidy_received": overview.TotalReceived,
				"subsidy_pending": overview.PendingBalance,
			}
		case "inventory":
			report.Basis = "当前商品库存快照；可用件数=门店+云仓，不含样机；估值按平均成本乘全部库存（含样机），缺少平均成本的商品未计入估值。"
			var total struct {
				Count                                              int64
				Units, Value, Sample, Main, Store, Cloud, Unvalued float64
			}
			err := config.DB.WithContext(ctx).Model(&models.Product{}).Select(`COUNT(*) AS count,
				COALESCE(SUM(store_stock+cloud_stock),0) AS units,
				COALESCE(SUM((store_stock+cloud_stock+sample_stock)*avg_cost),0) AS value,
				COALESCE(SUM(sample_stock),0) AS sample,0 AS main,
    COALESCE(SUM(store_stock),0) AS store,COALESCE(SUM(cloud_stock),0) AS cloud,
				COALESCE(SUM(CASE WHEN avg_cost<=0 AND store_stock+cloud_stock+sample_stock>0 THEN 1 ELSE 0 END),0) AS unvalued`).Scan(&total).Error
			if err != nil {
				return report, err
			}
			row.Records = total.Count
			row.Partial = false
			row.Values = map[string]float64{"stock_units": total.Units, "stock_value": agentRound(total.Value), "sample_stock": total.Sample, "main_stock": total.Main, "store_stock": total.Store, "cloud_stock": total.Cloud, "unvalued_count": total.Unvalued}

		case "sales":
			report.Basis = "按订单发生日期统计有效订单；销售额=订单金额−折扣；客单价=折后销售额÷订单数；订单毛利按现有订单费用和成本快照计算，尚未扣经营费用。若存在成本为 0 的历史订单明细，该期间毛利不提供，需先核实成本。"
			var total struct {
				Count                                                                int64
				Revenue, Cost, Delivery, Installation, Referral, Commission, TradeIn float64
			}
			query := config.DB.WithContext(ctx).Model(&models.Order{}).Where("order_status=1 AND created_at>=? AND created_at<?", start, end.AddDate(0, 0, 1))
			err := query.Select(`COUNT(*) AS count, COALESCE(SUM(total_amount-discount_amount),0) AS revenue,
    COALESCE(SUM(delivery_fee),0) AS delivery, COALESCE(SUM(installation_fee),0) AS installation,
    COALESCE(SUM(referral_fee),0) AS referral, COALESCE(SUM(commission_amount),0) AS commission,
    COALESCE(SUM(trade_in_amount),0) AS trade_in`).Scan(&total).Error
			if err != nil {
				return report, err
			}
			var items struct {
				Units float64
				Cost  float64
			}
			err = config.DB.WithContext(ctx).Table("order_items oi").Joins("JOIN orders o ON o.id=oi.order_id").
				Where("o.order_status=1 AND o.deleted_at IS NULL AND o.created_at>=? AND o.created_at<?", start, end.AddDate(0, 0, 1)).
				Select("COALESCE(SUM(oi.quantity),0) AS units, COALESCE(SUM(oi.unit_cost*oi.quantity),0) AS cost").Scan(&items).Error
			if err != nil {
				return report, err
			}
			err = config.DB.WithContext(ctx).Table("order_items oi").Joins("JOIN orders o ON o.id=oi.order_id").
				Where("o.order_status=1 AND o.deleted_at IS NULL AND o.created_at>=? AND o.created_at<? AND oi.unit_cost<=0", start, end.AddDate(0, 0, 1)).
				Count(&row.UncostedOrderItemCount).Error
			if err != nil {
				return report, err
			}
			row.CostIncomplete = row.UncostedOrderItemCount > 0
			row.Records = total.Count
			row.Values["sales_revenue"] = agentRound(total.Revenue)
			row.Values["order_count"] = float64(total.Count)
			row.Values["units"] = items.Units
			row.Values["average_order"] = 0
			if total.Count > 0 {
				row.Values["average_order"] = agentRound(total.Revenue / float64(total.Count))
			}
			if !row.CostIncomplete {
				row.Values["gross_profit"] = agentRound(CalculateGrossProfit(total.Revenue, items.Cost, total.Delivery, total.Installation, total.Referral, total.Commission, total.TradeIn))
			}
		case "cashflow":
			report.Basis = "按财务流水发生日期统计已登记收支，排除成对冲销；现金流不等于利润。"
			var rows []struct {
				FlowType int
				Amount   float64
				Count    int64
			}
			err := config.DB.WithContext(ctx).Model(&models.FinancialLog{}).
				Where("reversal_pair_id IS NULL AND created_at>=? AND created_at<?", start, end.AddDate(0, 0, 1)).
				Select("flow_type, COALESCE(SUM(amount),0) AS amount, COUNT(*) AS count").Group("flow_type").Scan(&rows).Error
			if err != nil {
				return report, err
			}
			row.Values["cash_in"], row.Values["cash_out"] = 0, 0
			for _, value := range rows {
				if value.FlowType == 1 {
					row.Values["cash_in"] = agentRound(value.Amount)
				}
				if value.FlowType == 2 {
					row.Values["cash_out"] = agentRound(value.Amount)
				}
				row.Records += value.Count
			}
			row.Values["net_cashflow"] = agentRound(row.Values["cash_in"] - row.Values["cash_out"])
		case "payroll":
			if start.Day() != 1 || end.Format("2006-01-02") != start.AddDate(0, 1, -1).Format("2006-01-02") {
				return report, fmt.Errorf("工资报表需要完整自然月")
			}
			report.Basis = "应计提成沿用员工管理月度提成报表：只计有效订单，普通订单按开单员工归属，老板客户订单不计个人业绩，商会订单按在职正式员工分摊。"
			for _, metric := range task.Metrics {
				if metric == "salary_payable" {
					report.Basis = "正式员工应发工资=底薪+应计提成−请假扣款；提成沿用员工管理月度提成报表。应发不等于已付款。"
					break
				}
			}
			payroll, err := monthlyEmployeePayrollReport(start)
			if err != nil {
				return report, err
			}
			if task.Entity != "" {
				employee, message, err := agentFindEmployee(task.Entity)
				if err != nil {
					return report, err
				}
				if message != "" {
					return report, fmt.Errorf("%s", message)
				}
				selected := payroll[:0]
				for _, item := range payroll {
					if item.EmployeeID == employee.ID {
						selected = append(selected, item)
					}
				}
				payroll = selected
			}
			row.Records = int64(len(payroll))
			cents := map[string]int64{"salary_payable": 0, "commission_payable": 0, "base_salary": 0, "deduction": 0}
			for _, item := range payroll {
				cents["salary_payable"] += moneyCents(item.FinalSalary)
				cents["commission_payable"] += moneyCents(item.TotalCommission)
				cents["base_salary"] += moneyCents(item.BaseSalary)
				cents["deduction"] += moneyCents(item.Deduction)
			}
			for metric, value := range cents {
				row.Values[metric] = float64(value) / 100
			}
		default:
			return report, fmt.Errorf("没有该业务的统一报表")
		}
		row.Missing = row.Records == 0
		report.Rows = append(report.Rows, row)
	}
	sort.SliceStable(report.Rows, func(i, j int) bool { return report.Rows[i].Period.Start < report.Rows[j].Period.Start })
	for i := 1; i < len(report.Rows); i++ {
		before, after := report.Rows[i-1], report.Rows[i]
		if before.Missing || after.Missing {
			continue
		}
		for key, value := range after.Values {
			previous, ok := before.Values[key]
			if !ok {
				continue
			}
			change := agentReportChange{Metric: key, From: before.Period.Label, To: after.Period.Label, Difference: agentRound(value - previous)}
			if previous != 0 {
				percent := agentRound((value - previous) / math.Abs(previous) * 100)
				change.Percent = &percent
			}
			report.Changes = append(report.Changes, change)
		}
	}
	sort.SliceStable(report.Changes, func(i, j int) bool {
		if report.Changes[i].To == report.Changes[j].To {
			return report.Changes[i].Metric < report.Changes[j].Metric
		}
		return report.Changes[i].To < report.Changes[j].To
	})
	return report, nil
}
func agentRound(value float64) float64 { return math.Round(value*100) / 100 }

func agentRenderReport(report agentBusinessReport, task agentTask) string {
	metrics := task.Metrics
	if len(metrics) == 0 {
		switch task.Topic {
		case "sales":
			metrics = []string{"sales_revenue", "order_count", "units", "average_order"}
		case "cashflow":
			metrics = []string{"cash_in", "cash_out"}
		case "payroll":
			metrics = []string{"salary_payable", "commission_payable"}
		case "subsidy":
			metrics = []string{"subsidy_pending", "subsidy_due", "subsidy_received"}
		}
	}
	lines := []string{}
	for _, row := range report.Rows {
		if row.Missing {
			lines = append(lines, row.Period.Label+"：没有找到符合口径的记录，无法据此认定实际业务为零。")
			continue
		}
		parts := []string{}
		for _, metric := range metrics {
			if value, ok := row.Values[metric]; ok {
				parts = append(parts, agentMetricLabels[metric]+" "+agentMetricValue(metric, value))
			}
		}
		line := row.Period.Label + "：" + strings.Join(parts, "，") + "。"
		if row.Partial {
			line += "该期间尚未结束，数值截至目前。"
		}
		if row.CostIncomplete {
			line += fmt.Sprintf("另有 %d 条订单明细成本待核，毛利暂不提供。", row.UncostedOrderItemCount)
		}
		lines = append(lines, line)
	}
	for _, change := range report.Changes {
		wanted := false
		for _, metric := range metrics {
			if metric == change.Metric {
				wanted = true
			}
		}
		if !wanted {
			continue
		}
		line := fmt.Sprintf("%s较%s的%s变化 %s", change.To, change.From, agentMetricLabels[change.Metric], agentMetricValue(change.Metric, change.Difference))
		if change.Percent != nil {
			line += fmt.Sprintf("（%+.2f%%）", *change.Percent)
		} else {
			line += "（基期为零，不计算百分比）"
		}
		lines = append(lines, line+"。")
	}
	lines = append(lines, "口径："+report.Basis)
	return strings.Join(lines, "\n")
}
func agentMetricValue(metric string, value float64) string {
	if metric == "order_count" || metric == "units" || strings.HasSuffix(metric, "_stock") || strings.HasSuffix(metric, "_count") || metric == "stock_units" {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("¥%.2f", value)
}
