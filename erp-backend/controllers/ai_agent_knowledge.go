package controllers

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"
)

var agentMonthPattern = regexp.MustCompile(`20\d{2}-(?:0[1-9]|1[0-2])`)

func agentFindEmployee(name string) (models.Employee, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 {
		return models.Employee{}, "请告诉我员工姓名或工号。", nil
	}
	var employees []models.Employee
	if err := config.DB.Where("name = ? OR emp_no = ?", name, name).Limit(10).Find(&employees).Error; err != nil {
		return models.Employee{}, "", err
	}
	if len(employees) == 0 {
		pattern := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(name) + "%"
		if err := config.DB.Where("name LIKE ? OR emp_no LIKE ?", pattern, pattern).Limit(10).Find(&employees).Error; err != nil {
			return models.Employee{}, "", err
		}
	}
	switch len(employees) {
	case 0:
		return models.Employee{}, fmt.Sprintf("没有找到员工“%s”，请核对姓名或工号。", name), nil
	case 1:
		return employees[0], "", nil
	default:
		return models.Employee{}, fmt.Sprintf("找到多位与“%s”匹配的员工，请用工号指定。", name), nil
	}
}

func agentEmployeeProfile(name string) (string, error) {
	employee, message, err := agentFindEmployee(name)
	if err != nil || message != "" {
		return message, err
	}
	return fmt.Sprintf("%s是系统员工，工号 %s，岗位 %s，当前提成比例 %.2f%%。员工管理中的月度提成报表可查看个人及商会订单提成，也可以直接问我指定月份的提成。", employee.Name, employee.EmpNo, employee.Role, employee.CommissionRate*100), nil
}

func normalizeAgentEmployeePlan(plan agentPlan, message string, recent []models.AIAgentMessage) (agentPlan, error) {
	if plan.Tool == "data_query" || plan.Tool == "data_catalog" {
		return plan, nil
	}
	tasksCommission := strings.Contains(message, "提成") || strings.Contains(message, "佣金")
	tasksPerformance := strings.Contains(message, "业绩") || strings.Contains(message, "跟单")
	assertsEmployee := strings.Contains(message, "是我的员工") || strings.Contains(message, "是员工")
	allEmployees := agentMeansAllEmployees(message) || strings.EqualFold(strings.TrimSpace(plan.Employee), "all")
	if employee := strings.TrimSpace(plan.Employee); employee != "" && !strings.EqualFold(employee, "all") && strings.Contains(message, employee) {
		allEmployees = false
	}
	if allEmployees && !tasksCommission {
		for index := len(recent) - 1; index >= 0; index-- {
			if recent[index].Role != "user" || recent[index].Content == message {
				continue
			}
			previous := recent[index].Content
			tasksCommission = strings.Contains(previous, "提成") || strings.Contains(previous, "佣金")
			if tasksCommission && agentPeriodFromMessage(message) == "" && agentMonthPattern.FindString(message) == "" {
				if previousMonth := agentMonthPattern.FindString(previous); previousMonth != "" {
					plan.Month = previousMonth
					plan.Period = ""
				} else if previousPeriod := agentPeriodFromMessage(previous); previousPeriod != "" {
					plan.Month = ""
					plan.Period = previousPeriod
				}
			}
			break
		}
	}
	if !tasksCommission && !tasksPerformance && !assertsEmployee {
		return plan, nil
	}
	if allEmployees && tasksCommission {
		plan.Tool, plan.Employee = "employee_commission", "all"
		if period := agentPeriodFromMessage(message); period != "" {
			plan.Period, plan.Month = period, ""
		} else if month := agentMonthPattern.FindString(message); month != "" {
			plan.Month, plan.Period = month, ""
		}
		return plan, nil
	}
	var employees []models.Employee
	if err := config.DB.Select("id", "name", "emp_no").Find(&employees).Error; err != nil {
		return plan, err
	}
	name := ""
	for index := len(recent) - 1; index >= 0 && name == ""; index-- {
		if recent[index].Role != "user" {
			continue
		}
		for _, employee := range employees {
			if employee.Name != "" && strings.Contains(recent[index].Content, employee.Name) {
				name = employee.Name
				break
			}
		}
	}
	if name == "" && tasksCommission && strings.Contains(message, "员工") {
		plan.Tool, plan.Employee = "employee_commission", ""
		return plan, nil
	}
	if name == "" {
		name = strings.TrimSpace(plan.Employee)
	}
	if name == "" {
		return plan, nil
	}
	plan.Employee = name
	switch {
	case tasksCommission:
		plan.Tool = "employee_commission"
	case tasksPerformance:
		plan.Tool = "employee_orders"
	case assertsEmployee:
		plan.Tool = "employee_profile"
	}
	if strings.Contains(message, "上个月") || strings.Contains(message, "上月") {
		plan.Period, plan.Month = "last_month", ""
	}
	return plan, nil
}

func agentMeansAllEmployees(message string) bool {
	for _, phrase := range []string{"所有人", "所有员工", "全部员工", "全体员工", "全员", "每个员工", "员工总提成", "提成总额", "提成合计"} {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	mentionsStaff := strings.Contains(message, "员工") || strings.Contains(message, "大家")
	mentionsCommission := strings.Contains(message, "提成") || strings.Contains(message, "佣金")
	mentionsTotal := strings.Contains(message, "一共") || strings.Contains(message, "总共") || strings.Contains(message, "合计")
	return mentionsStaff && mentionsCommission && mentionsTotal
}

func agentPeriodFromMessage(message string) string {
	switch {
	case strings.Contains(message, "上个月"), strings.Contains(message, "上月"):
		return "last_month"
	case strings.Contains(message, "这个月"), strings.Contains(message, "本月"):
		return "this_month"
	default:
		return ""
	}
}

func normalizeAgentHelpPlan(plan agentPlan, message string) agentPlan {
	if plan.Tool != "finish" {
		return plan
	}
	tasksHow := strings.Contains(message, "怎么") || strings.Contains(message, "如何") || strings.Contains(message, "在哪") || strings.Contains(message, "入口") || strings.Contains(message, "有什么功能")
	if tasksHow {
		plan.Tool, plan.Category = "system_help", message
	}
	return plan
}

func agentMonthStart(month, period string) (time.Time, string) {
	month = strings.TrimSpace(month)
	if month != "" {
		start, err := time.ParseInLocation("2006-01", month, time.Local)
		if err != nil || start.Format("2006-01") != month {
			return time.Time{}, "月份请用 YYYY-MM 格式，例如 2026-09。"
		}
		return start, ""
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	if period == "last_month" {
		start = start.AddDate(0, -1, 0)
	}
	return start, ""
}

func agentEmployeeCommission(name, month, period string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(name), "all") || agentMeansAllEmployees(name) {
		return agentAllEmployeeCommissions(month, period)
	}
	employee, message, err := agentFindEmployee(name)
	if err != nil || message != "" {
		return message, err
	}
	start, message := agentMonthStart(month, period)
	if message != "" {
		return message, nil
	}
	end := start.AddDate(0, 1, 0)
	if employee.IsOutsourced {
		return fmt.Sprintf("%s是外包人员，当前月度员工提成报表不包含外包人员。", employee.Name), nil
	}
	if !employeeWorkedDuringMonth(employee, start, end) {
		return fmt.Sprintf("%s在%s不属于在职月份，月度提成报表没有该员工记录。", employee.Name, start.Format("2006-01")), nil
	}
	var historicalEmployees []models.Employee
	if err := config.DB.Unscoped().Where("is_outsourced = ?", false).Find(&historicalEmployees).Error; err != nil {
		return "", err
	}
	var orders []models.Order
	if err := config.DB.Preload("OrderItems.Product").Where("created_at >= ? AND created_at < ? AND order_status = 1", start, end).Order("created_at asc, id asc").Find(&orders).Error; err != nil {
		return "", err
	}
	item := &CommissionReportItem{EmployeeID: employee.ID, EmployeeName: employee.Name, CommissionRate: employee.CommissionRate, CommissionDetails: []CommissionDetail{}}
	report := map[uint]*CommissionReportItem{employee.ID: item}
	for _, order := range orders {
		if order.OrderType == 2 {
			addChamberOrderToCommissionReport(order, historicalEmployees, report)
		} else {
			addOrderToCommissionReport(order, report)
		}
	}
	return fmt.Sprintf("%s（工号 %s）%s提成：¥%.2f。个人订单提成 ¥%.2f，商会订单分摊 ¥%.2f；个人订单商品销售额 ¥%.2f，提成明细 %d 笔。口径与员工管理的月度提成报表一致：仅有效订单，普通订单按开单员工归属，老板客户订单不计个人业绩，商会订单按在职正式员工分摊。", employee.Name, employee.EmpNo, start.Format("2006-01"), item.TotalCommission, item.PersonalCommission, item.ChamberCommission, item.TotalSales, len(item.CommissionDetails)), nil
}

func agentAllEmployeeCommissions(month, period string) (string, error) {
	start, message := agentMonthStart(month, period)
	if message != "" {
		return message, nil
	}
	end := start.AddDate(0, 1, 0)
	var employees []models.Employee
	if err := config.DB.Where("is_outsourced = ?", false).Order("id asc").Find(&employees).Error; err != nil {
		return "", err
	}
	var historical []models.Employee
	if err := config.DB.Unscoped().Where("is_outsourced = ?", false).Find(&historical).Error; err != nil {
		return "", err
	}
	reports := make(map[uint]*CommissionReportItem)
	for _, employee := range employees {
		if employeeWorkedDuringMonth(employee, start, end) {
			reports[employee.ID] = &CommissionReportItem{EmployeeID: employee.ID, EmployeeName: employee.Name, EmpNo: employee.EmpNo, CommissionRate: employee.CommissionRate, CommissionDetails: []CommissionDetail{}}
		}
	}
	var orders []models.Order
	if err := config.DB.Preload("OrderItems.Product").Where("created_at >= ? AND created_at < ? AND order_status = 1", start, end).Order("created_at asc, id asc").Find(&orders).Error; err != nil {
		return "", err
	}
	for _, order := range orders {
		if order.OrderType == 2 {
			addChamberOrderToCommissionReport(order, historical, reports)
		} else {
			addOrderToCommissionReport(order, reports)
		}
	}
	var total float64
	lines := []string{}
	for _, employee := range employees {
		item := reports[employee.ID]
		if item == nil {
			continue
		}
		total += item.TotalCommission
		lines = append(lines, fmt.Sprintf("%s（工号 %s）：¥%.2f", item.EmployeeName, item.EmpNo, item.TotalCommission))
	}
	return fmt.Sprintf("%s所有员工提成合计：¥%.2f。按员工明细：%s。仅含在职正式员工，口径与员工管理月度提成报表一致。", start.Format("2006-01"), total, strings.Join(lines, "；")), nil
}

func agentEmployeeOrders(name, period string) (string, error) {
	employee, message, err := agentFindEmployee(name)
	if err != nil || message != "" {
		return message, err
	}
	start, message := agentMonthStart("", period)
	if message != "" {
		return message, nil
	}
	end := start.AddDate(0, 1, 0)
	var orders []models.Order
	if err := config.DB.Where("employee_id = ? AND created_at >= ? AND created_at < ? AND order_status = 1", employee.ID, start, end).Order("created_at desc, id desc").Limit(50).Find(&orders).Error; err != nil {
		return "", err
	}
	if len(orders) == 0 {
		return fmt.Sprintf("%s在%s没有有效跟单订单。", employee.Name, start.Format("2006-01")), nil
	}
	lines := make([]string, 0, len(orders))
	for _, order := range orders {
		lines = append(lines, fmt.Sprintf("%s %s 客户%s 金额¥%.2f", order.CreatedAt.Format("2006-01-02"), order.OrderNo, order.CustomerName, order.TotalAmount))
	}
	return fmt.Sprintf("%s在%s的有效跟单订单（最多显示50笔）：%s", employee.Name, start.Format("2006-01"), strings.Join(lines, "；")), nil
}

func agentSystemHelp(category string) string {
	if strings.Contains(category, "提成") || strings.Contains(category, "员工") {
		return "员工月度提成报表可在员工管理页面查看；选择月份后可查看个人订单提成、商会订单分摊及明细。也可以直接询问某位员工或所有员工指定月份的提成。"
	}
	return "可以从系统菜单进入销售、库存、财务、员工和国补等模块；告诉我你要完成的业务，我可以查询相关数据或说明入口。"
}
