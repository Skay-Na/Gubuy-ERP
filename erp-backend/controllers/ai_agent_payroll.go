package controllers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"
)

var agentChineseMonth = regexp.MustCompile(`(?:(20\d{2})年)?(1[0-2]|0?[1-9])月份?`)

func agentQuestionMonth(message string) (string, string) {
	if month := agentMonthPattern.FindString(message); month != "" {
		return month, ""
	}
	if period := agentPeriodFromMessage(message); period != "" {
		return "", period
	}
	parts := agentChineseMonth.FindStringSubmatch(message)
	if len(parts) == 0 {
		return "", ""
	}
	year := time.Now().Year()
	if parts[1] != "" {
		year, _ = strconv.Atoi(parts[1])
	}
	month, _ := strconv.Atoi(parts[2])
	return fmt.Sprintf("%04d-%02d", year, month), ""
}

func agentPayrollTopic(message string) string {
	if strings.Contains(message, "工资") || strings.Contains(message, "薪资") {
		return "salary"
	}
	if strings.Contains(message, "提成") || strings.Contains(message, "佣金") {
		return "commission"
	}
	return ""
}

func agentHasOtherBusinessTopic(message string) bool {
	for _, topic := range []string{
		"订单", "销售", "销量", "营业额", "营收", "收入", "利润", "毛利", "库存", "商品", "采购", "入库",
		"客户", "应收", "尾款", "回款", "国补", "考勤", "流水", "账户", "记账", "报销", "费用", "支出", "退货", "转账",
	} {
		if strings.Contains(message, topic) {
			return true
		}
	}
	return false
}

func agentPayrollBasis(message string) string {
	// An explicit correction to the reporting basis wins over incidental mentions
	// of payment records (for example, “没有发放记录，只看应发工资”).
	for _, word := range []string{"应发", "应计", "预计", "报表", "应付"} {
		if strings.Contains(message, word) {
			return "payable"
		}
	}
	for _, word := range []string{"实际发放", "已发", "实发", "支付了", "已付", "发了", "发放", "付款", "发出去"} {
		if strings.Contains(message, word) {
			return "paid"
		}
	}
	return ""
}

// Resolve a short payroll follow-up from the user's own earlier questions.
// Explicit current subject, reporting basis and month always take precedence.
func agentPayrollQuestion(message string, recent []models.AIAgentMessage) (string, string, string, string) {
	topic := agentPayrollTopic(message)
	basis := agentPayrollBasis(message)
	month, period := agentQuestionMonth(message)
	shortFollowUp := topic == "" || agentMeansAllEmployees(message)
	for index := len(recent) - 1; index >= 0; index-- {
		if recent[index].Role != "user" {
			continue
		}
		previous := recent[index].Content
		previousTopic := agentPayrollTopic(previous)
		if previousTopic == "" {
			if agentHasOtherBusinessTopic(previous) {
				break
			}
			continue
		}
		if topic == "" {
			topic = previousTopic
		}
		if (shortFollowUp || basis == "") && agentPayrollBasis(previous) != "" {
			// “我上个月发了多少” inherits an explicit prior correction to
			// the payable basis, while a new complete payroll question does not.
			if basis == "" || (shortFollowUp && agentPayrollBasis(previous) == "payable") {
				basis = agentPayrollBasis(previous)
			}
		}
		if month == "" && period == "" {
			month, period = agentQuestionMonth(previous)
		}
		break
	}
	return topic, basis, month, period
}

func agentPayrollAnswer(message string, recent []models.AIAgentMessage) (string, string, bool, error) {
	// A newly named business subject starts a new request, even when an earlier
	// conversation turn was about payroll.
	if agentPayrollTopic(message) == "" && agentHasOtherBusinessTopic(message) {
		return "", "", false, nil
	}
	topic, basis, month, period := agentPayrollQuestion(message, recent)
	if topic == "" {
		if strings.Contains(message, "发了多少") || strings.Contains(message, "发出去多少") {
			return "你想问工资、提成，还是其他支出？如果没有登记付款记录，也可以按员工月报计算应发工资。", "", true, nil
		}
		return "", "", false, nil
	}
	if month == "" && period == "" {
		return "", "", false, nil
	}
	if !agentMeansAllEmployees(message) {
		var employees []models.Employee
		if err := config.DB.Select("name", "emp_no").Find(&employees).Error; err != nil {
			return "", "", true, err
		}
		for _, employee := range employees {
			if len([]rune(employee.Name)) >= 2 && strings.Contains(message, employee.Name) {
				return "", "", false, nil
			}
			if employee.EmpNo != "" && strings.Contains(message, "工号"+employee.EmpNo) {
				return "", "", false, nil
			}
		}
	}
	start, validation := agentMonthStart(month, period)
	if validation != "" {
		return validation, "", true, nil
	}
	if basis == "paid" {
		answer, err := agentRecordedPayoutForMonth(topic, start)
		return answer, "财务支出流水", true, err
	}
	answer, err := agentExpectedPayrollForMonth(topic, start)
	return answer, "员工月度提成报表口径", true, err
}

// Calculate the all-employee amount from the same monthly report used by the UI.
func agentExpectedPayrollForMonth(topic string, start time.Time) (string, error) {
	report, err := monthlyEmployeePayrollReport(start)
	if err != nil {
		return "", err
	}
	var base, commission, deduction, salary int64
	for _, item := range report {
		base += moneyCents(item.BaseSalary)
		commission += moneyCents(item.TotalCommission)
		deduction += moneyCents(item.Deduction)
		salary += moneyCents(item.FinalSalary)
	}
	label := start.Format("2006-01")
	if len(report) == 0 {
		return label + "没有月度薪资报表中的正式员工记录，无法计算应发金额。", nil
	}
	if topic == "salary" {
		answer := fmt.Sprintf("%s全体 %d 位正式员工的应发工资合计 ¥%.2f：底薪 ¥%.2f + 应计提成 ¥%.2f - 请假扣款 ¥%.2f。",
			label, len(report), float64(salary)/100, float64(base)/100, float64(commission)/100, float64(deduction)/100)
		if start.Year() == time.Now().Year() && start.Month() == time.Now().Month() {
			answer += "本月尚未结束，这是按目前订单和考勤计算的预计值，月底前会变化。"
		}
		return answer + "数据来源：员工月度提成报表；应发金额不等于已实际付款。", nil
	}
	answer := fmt.Sprintf("%s全体 %d 位正式员工的应计提成合计 ¥%.2f。", label, len(report), float64(commission)/100)
	if start.Year() == time.Now().Year() && start.Month() == time.Now().Month() {
		answer += "本月尚未结束，订单变化会影响最终提成。"
	}
	return answer + "数据来源：员工月度提成报表；应计提成不等于已实际付款。", nil
}
