package controllers

import (
	"fmt"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"
)

// A payout is a cash outflow, not an accrued payroll or commission figure.
// Only explicit payout wording and a resolvable month use this deterministic path.
func agentRecordedPayout(message string) (string, bool, error) {
	asksPayout := strings.Contains(message, "发了") || strings.Contains(message, "发放") ||
		strings.Contains(message, "支付了") || strings.Contains(message, "实发")
	if !asksPayout {
		return "", false, nil
	}
	asksCommission := strings.Contains(message, "提成") || strings.Contains(message, "佣金")
	asksSalary := strings.Contains(message, "工资") || strings.Contains(message, "薪资")
	if !asksCommission && !asksSalary {
		return "", false, nil
	}
	period := agentPeriodFromMessage(message)
	month := agentMonthPattern.FindString(message)
	if month == "" && period == "" {
		return "", false, nil
	}
	start, validation := agentMonthStart(month, period)
	if validation != "" {
		return validation, true, nil
	}
	topic := "salary"
	if asksCommission {
		topic = "commission"
	}
	answer, err := agentRecordedPayoutForMonth(topic, start)
	return answer, true, err
}

func agentRecordedPayoutForMonth(topic string, start time.Time) (string, error) {
	end := start.AddDate(0, 1, 0)
	kind := "工资"
	query := config.DB.Model(&models.FinancialLog{}).
		Where("flow_type = ? AND reversal_pair_id IS NULL AND created_at >= ? AND created_at < ?", 2, start, end)
	if topic == "commission" {
		kind = "员工提成"
		query = query.Where("(category IN ? OR category LIKE ? OR category LIKE ?) AND category NOT LIKE ?",
			[]string{"员工提成", "提成", "提成发放", "业务提成"}, "%员工%提成%", "%业务%提成%", "%渠道%")
	} else {
		query = query.Where("(category LIKE ? OR category LIKE ?) AND category NOT LIKE ?", "%工资%", "%薪资%", "%提成%")
	}
	var total struct {
		Count  int64   `gorm:"column:count"`
		Amount float64 `gorm:"column:amount"`
	}
	if err := query.Select("COUNT(*) AS count, COALESCE(SUM(amount),0) AS amount").Scan(&total).Error; err != nil {
		return "", err
	}
	label := start.Format("2006-01")
	if total.Count == 0 {
		return fmt.Sprintf("%s的财务支出流水中没有找到已登记的%s发放记录，因此无法核实实际发放总额。月度员工报表中的应计提成或应发薪资不能当作已付款。", label, kind), nil
	}
	return fmt.Sprintf("%s财务支出流水中登记的%s发放合计 ¥%.2f，共 %d 笔（已排除成对冲销流水）。这是系统已登记的支出；若有线下付款未记账，实际发放额可能更高。", label, kind, total.Amount, total.Count), nil
}
