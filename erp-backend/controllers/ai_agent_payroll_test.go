package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"
)

func TestAgentPayrollQuestionUsesCorrectionAndNaturalMonth(t *testing.T) {
	now := time.Now()
	wantMonth := fmt.Sprintf("%04d-09", now.Year())
	correction := "我没有工资发放记录，你只看员工9月份应发的工资是多少就行"
	topic, basis, month, period := agentPayrollQuestion(correction, nil)
	if topic != "salary" || basis != "payable" || month != wantMonth || period != "" {
		t.Fatalf("correction: topic=%q basis=%q month=%q period=%q", topic, basis, month, period)
	}
	recent := []models.AIAgentMessage{{Role: "user", Content: "我上个月发了多少工资"}, {Role: "assistant", Content: "没有付款记录"}, {Role: "user", Content: correction}}
	topic, basis, month, period = agentPayrollQuestion("我上个月发了多少", recent)
	if topic != "salary" || basis != "payable" || month != "" || period != "last_month" {
		t.Fatalf("follow-up: topic=%q basis=%q month=%q period=%q", topic, basis, month, period)
	}
	topic, basis, _, period = agentPayrollQuestion("我上个月发了多少工资", nil)
	if topic != "salary" || basis != "paid" || period != "last_month" {
		t.Fatalf("new paid question: topic=%q basis=%q period=%q", topic, basis, period)
	}
}

func TestAgentPayrollCanonicalizesChineseCommissionBasisForReport(t *testing.T) {
	task := canonicalAgentTask(agentTask{
		Goal: "查询所有员工2026年9月的提成", Topic: "payroll", Operation: "lookup",
		Metrics: []string{"commission_payable"},
		Periods: []agentTaskPeriod{{Label: "2026年9月", Start: "2026-09-01", End: "2026-09-30"}},
		Scope:   "all", Basis: "应计提成（commission_payable）",
	})
	if task.Basis != "commission_payable" || !agentCanReport(task) {
		t.Fatalf("commission report task was not canonicalized: %+v", task)
	}
	paid := canonicalAgentTask(agentTask{
		Goal: "查询已发提成", Topic: "payroll", Operation: "lookup",
		Metrics: []string{"commission_paid"}, Scope: "all", Basis: "实际已发提成",
	})
	if paid.Basis != "commission_paid" || agentCanReport(paid) {
		t.Fatalf("actual paid commission was routed to payable report: %+v", paid)
	}
	corrected := canonicalAgentTask(agentTask{
		Topic: "payroll", Metrics: []string{"commission_payable"}, Basis: "实际发放的提成",
	})
	if corrected.Basis != "commission_paid" || corrected.Metrics[0] != "commission_paid" {
		t.Fatalf("explicit payment basis did not override inferred payable metric: %+v", corrected)
	}
}

func TestAgentPayrollBasisCannotSwitchToPaidWithoutExplicitUserWording(t *testing.T) {
	commission := agentNormalizePayrollBasisForQuestion(agentTask{Topic: "payroll", Metrics: []string{"commission_paid"}, Basis: "commission_paid"}, "查询员工上月提成")
	if commission.Basis != "commission_payable" || commission.Metrics[0] != "commission_payable" {
		t.Fatalf("model-inferred commission payout was not normalized: %+v", commission)
	}
	salary := agentNormalizePayrollBasisForQuestion(agentTask{Topic: "payroll", Metrics: []string{"salary_paid"}, Basis: "paid"}, "上个月工资情况")
	if salary.Basis != "payable" || salary.Metrics[0] != "salary_payable" {
		t.Fatalf("model-inferred salary payout was not normalized: %+v", salary)
	}
	for _, question := range []string{"上月提成实际发了多少", "员工实发工资是多少"} {
		task := agentNormalizePayrollBasisForQuestion(canonicalAgentTask(agentTask{
			Topic: "payroll", Metrics: []string{"commission_paid"}, Basis: "commission_paid",
		}), question)
		if task.Basis != "commission_paid" {
			t.Fatalf("explicit payout wording was discarded for %q: %+v", question, task)
		}
	}
}

func TestAgentPayrollAllCommissionRunsWithoutEmployeeName(t *testing.T) {
	setupAudit(t)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	unique := strconv.FormatInt(time.Now().UnixNano(), 10)
	employee := models.Employee{Name: "全员提成" + unique, EmpNo: "AC" + unique, EntryDate: &start}
	if err := config.DB.Create(&employee).Error; err != nil {
		t.Fatal(err)
	}
	plan := agentPlan{Tool: "employee_commission", Month: "2026-09"}
	result, _, _, err := agentExecuteTool(context.Background(), models.AIAgentSession{}, plan)
	if err != nil || strings.Contains(result, "请告诉我员工姓名") || !strings.Contains(result, "所有员工提成合计：¥") {
		t.Fatalf("all employee commission query: %q, %v", result, err)
	}
}

func TestAgentPayrollCommissionUsesCanonicalReportWithoutPlanner(t *testing.T) {
	setupAudit(t)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	employee := models.Employee{Name: "提成报表员工" + stamp, EmpNo: "CR" + stamp, EntryDate: &start}
	if err := config.DB.Create(&employee).Error; err != nil {
		t.Fatal(err)
	}
	task := agentTask{
		Goal: "查询所有员工2026年9月的应计提成", Topic: "payroll", Operation: "lookup",
		Metrics: []string{"commission_payable"}, Periods: []agentTaskPeriod{{
			Label: "2026年9月", Start: "2026-09-01", End: "2026-09-30",
		}}, Scope: "all", Basis: "应计提成（commission_payable）",
	}
	script := &agentMockScript{Task: func(string) agentTask { return task }}
	ctx, trace := agentTestContext()
	out := runAgentTask(ctx, agentMockRuntime(t, script), models.AIAgentSession{}, agentSendInput{Message: "查询员工上月提成"}, nil)
	if out.Status != "complete" || script.PlanCalls != 0 || !strings.Contains(out.Answer, "应计提成") || trace.Tools[0].Plan.Tool != "business_report" {
		t.Fatalf("commission report did not run deterministically: status=%s plans=%d answer=%q trace=%+v", out.Status, script.PlanCalls, out.Answer, trace.Tools)
	}
}

func TestAgentPayrollDoesNotHijackOtherBusinessQuestions(t *testing.T) {
	recent := []models.AIAgentMessage{{Role: "user", Content: "上个月发了多少工资"}}
	for _, question := range []string{"上个月订单有多少", "上个月销售额呢", "库存现在怎么样", "上个月支出了多少"} {
		if answer, source, handled, err := agentPayrollAnswer(question, recent); err != nil || handled {
			t.Fatalf("question %q hijacked: answer=%q source=%q handled=%v err=%v", question, answer, source, handled, err)
		}
	}
	recent = append(recent, models.AIAgentMessage{Role: "user", Content: "这个月订单销售额是多少"})
	if answer, source, handled, err := agentPayrollAnswer("那上个月呢", recent); err != nil || handled {
		t.Fatalf("new order context was hijacked by old payroll question: answer=%q source=%q handled=%v err=%v", answer, source, handled, err)
	}
}

func TestAgentPayrollCorrectionAnswersAllEmployees(t *testing.T) {
	setupAudit(t)
	start := time.Date(time.Now().Year(), 9, 1, 0, 0, 0, 0, time.Local)
	unique := strconv.FormatInt(time.Now().UnixNano(), 10)
	employee := models.Employee{Name: "月报薪资" + unique, EmpNo: "PM" + unique, EntryDate: &start, BaseSalary: 3200}
	if err := config.DB.Create(&employee).Error; err != nil {
		t.Fatal(err)
	}
	report, err := monthlyEmployeePayrollReport(start)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, row := range report {
		total += moneyCents(row.FinalSalary)
	}
	correction := "我没有工资发放记录，你只看员工9月份应发的工资是多少就行"
	answer, source, handled, err := agentPayrollAnswer(correction, nil)
	if err != nil || !handled || source != "员工月度提成报表口径" || !strings.Contains(answer, fmt.Sprintf("应发工资合计 ¥%.2f", float64(total)/100)) {
		t.Fatalf("correction answer=%q source=%q handled=%v err=%v", answer, source, handled, err)
	}
	recent := []models.AIAgentMessage{{Role: "user", Content: correction}}
	answer, source, handled, err = agentPayrollAnswer("我上个月发了多少", recent)
	if err != nil || !handled || source != "员工月度提成报表口径" || !strings.Contains(answer, "应发工资合计") {
		t.Fatalf("follow-up answer=%q source=%q handled=%v err=%v", answer, source, handled, err)
	}
	answer, source, handled, err = agentPayrollAnswer("我上个月发了多少", nil)
	if err != nil || !handled || source != "" || !strings.Contains(answer, "工资、提成") {
		t.Fatalf("ambiguous answer=%q source=%q handled=%v err=%v", answer, source, handled, err)
	}
}
