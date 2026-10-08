package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type agentMockScript struct {
	Task             func(string) agentTask
	Plans            []agentPlan
	Systems          []string
	Answer           string
	EmptyAnswer      bool
	MissingFirst     bool
	TaskCalls        int
	PlanCalls        int
	AnswerCalls      int
	MemoryCalls      int
	MemoryCandidates []agentMemoryCandidate
}

func agentMockRuntime(t *testing.T, script *agentMockScript) aiProviderRuntime {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if len(request.Messages) < 2 {
			t.Error("missing messages")
			w.WriteHeader(400)
			return
		}
		system, user := request.Messages[0].Content, request.Messages[1].Content
		script.Systems = append(script.Systems, system)
		var result any
		switch {
		case strings.Contains(system, "AGENT_MEMORY_INFER_V1"):
			script.MemoryCalls++
			result = gin.H{"candidates": script.MemoryCandidates}
		case strings.Contains(system, "AGENT_TASK_V2"):
			script.TaskCalls++
			result = script.Task(user)
		case strings.Contains(system, "AGENT_ANSWER_V2"):
			script.AnswerCalls++
			answer := script.Answer
			if answer == "" && !script.EmptyAnswer {
				answer = "已核对各期数据，变化请结合业务构成判断。"
			}
			result = gin.H{"answer": answer, "evidence_ids": []string{"E1"}, "complete": true, "missing": []string{}}
			if script.MissingFirst && script.AnswerCalls == 1 {
				result = gin.H{"answer": "还需核对商品构成", "evidence_ids": []string{"E1"}, "complete": false, "missing": []string{"商品构成"}}
			}
		default:
			index := script.PlanCalls
			script.PlanCalls++
			if index < len(script.Plans) {
				result = script.Plans[index]
			} else {
				result = agentPlan{Tool: "finish", Answer: "查询完成"}
			}
		}
		content, _ := json.Marshal(result)
		_ = json.NewEncoder(w).Encode(gin.H{"choices": []gin.H{{"message": gin.H{"content": string(content)}, "finish_reason": "stop"}}, "usage": gin.H{"prompt_tokens": 100, "completion_tokens": 50}})
	}))
	t.Cleanup(server.Close)
	return aiProviderRuntime{Provider: "deepseek", Model: "test-model", APIKey: "test", URL: server.URL, JSONMode: true}
}
func agentTestContext() (context.Context, *agentRunTrace) {
	trace := &agentRunTrace{Profile: agentProfileFor("standard", ""), Calls: []agentCallTrace{}, Tools: []agentToolTrace{}}
	return context.WithValue(context.Background(), agentTraceKey{}, trace), trace
}
func agentSalesTask() agentTask {
	return agentTask{Goal: "对比8月和9月销售情况", Topic: "sales", Operation: "compare", Metrics: []string{"sales_revenue", "order_count", "units", "average_order"}, Periods: []agentTaskPeriod{{Label: "2026年8月", Start: "2026-08-01", End: "2026-08-31"}, {Label: "2026年9月", Start: "2026-09-01", End: "2026-09-30"}}, Scope: "all"}
}
func seedAgentSalesComparison(t *testing.T) {
	t.Helper()
	setupAudit(t)
	product := ensureProduct(t, "Agent销售对比", 100, 50, 10)
	for i, month := range []time.Month{8, 9} {
		order := models.Order{OrderNo: fmt.Sprintf("AGENT-V2-%d-%d", time.Now().UnixNano(), i), OrderStatus: 1, TotalAmount: 1000 + float64(i)*1000, DiscountAmount: 100, CreatedAt: time.Date(2026, month, 10, 0, 0, 0, 0, time.Local), OrderItems: []models.OrderItem{{ProductID: product, Quantity: 2 + i, UnitPrice: 500, UnitCost: 100}}}
		if err := config.DB.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
	}
}
func TestAgentV2SalesComparisonAnswersBothMonthsWithoutRawJSON(t *testing.T) {
	seedAgentSalesComparison(t)
	script := &agentMockScript{Task: func(string) agentTask { return agentSalesTask() }}
	runtime := agentMockRuntime(t, script)
	ctx, trace := agentTestContext()
	out := runAgentTask(ctx, runtime, models.AIAgentSession{}, agentSendInput{Message: "8月和9月销售情况对比"}, nil)
	if out.Status != "complete" || !strings.Contains(out.Answer, "¥900.00") || !strings.Contains(out.Answer, "¥1900.00") || !strings.Contains(out.Answer, "111.11%") {
		t.Fatalf("comparison: %+v", out)
	}
	if strings.Contains(out.Answer, "\"dataset\"") || strings.Contains(out.Answer, "查询参数：") {
		t.Fatal("raw tool output leaked")
	}
	if script.PlanCalls != 0 || script.AnswerCalls != 1 || len(trace.Calls) != 2 {
		t.Fatalf("unexpected workflow: script=%+v trace=%+v", script, trace)
	}
	if len(out.Evidence) != 1 || len(out.Evidence[0].Periods) != 2 {
		t.Fatal("missing comparison evidence")
	}
}

func TestAgentCashflowAnalysisUsesVerifiedEmptyReportWithoutRetryingPlanner(t *testing.T) {
	setupAudit(t)
	task := agentTask{
		Goal: "查看上个月收支情况", Topic: "cashflow", Operation: "analyze",
		Metrics: []string{"cash_in", "cash_out"},
		Periods: []agentTaskPeriod{{Label: "2026年9月", Start: "2026-09-01", End: "2026-09-30"}}, Scope: "all",
	}
	script := &agentMockScript{Task: func(string) agentTask { return task }}
	ctx, trace := agentTestContext()
	out := runAgentTask(ctx, agentMockRuntime(t, script), models.AIAgentSession{}, agentSendInput{Message: "上个月收支情况怎么样"}, nil)
	if out.Status != "complete" || !strings.Contains(out.Answer, "没有找到符合口径的记录") {
		t.Fatalf("empty cashflow answer should be explicit and complete: %+v", out)
	}
	if script.PlanCalls != 0 || strings.Contains(trace.Stop, "synthesis_error") {
		t.Fatalf("verified report should not enter the generic planner: script=%+v trace=%+v", script, trace)
	}
}

func TestAgentForecastInsufficientHistoryGetsUserFacingExplanation(t *testing.T) {
	result := "数据集 orders，操作 forecast。\n查询参数：{}\n结果：" + `{"forecast_available":false,"reason":"最近 12 个完整月中只有少于 3 个有记录的月份","observed_months":2}`
	reason, months := agentForecastUnavailable(result)
	answer := agentRenderForecastUnavailable(reason, months)
	if reason == "" || months != 2 || !strings.Contains(answer, "2个有记录的完整月份") || !strings.Contains(answer, "无法给出可靠的销售额预测") {
		t.Fatalf("forecast limitation not explained: reason=%q months=%d answer=%q", reason, months, answer)
	}
	if reason, months := agentForecastUnavailable("结果：" + `{"forecast_available":true,"forecast":[{"month":"2026-10","baseline":100}]}`); reason != "" || months != 0 {
		t.Fatalf("available forecast incorrectly marked unavailable: reason=%q months=%d", reason, months)
	}
}

func TestCanonicalCashflowNetMetricEnablesStandardReport(t *testing.T) {
	task := canonicalAgentTask(agentTask{Goal: "上月收入支出结余", Topic: "cashflow", Operation: "analyze", Metrics: []string{"cash_in", "cash_out", "净收支（收入减支出）"}, Periods: []agentTaskPeriod{{Label: "2026年9月", Start: "2026-09-01", End: "2026-09-30"}}, Scope: "all"})
	if task.Metrics[2] != "net_cashflow" || !agentCanReport(task) {
		t.Fatalf("cashflow metrics did not normalize to supported report: %+v", task)
	}
}

func TestAgentV2RepairsTaskThatDropsSeptember(t *testing.T) {
	calls := 0
	runtime := agentMockRuntime(t, &agentMockScript{Task: func(string) agentTask {
		calls++
		task := agentSalesTask()
		if calls == 1 {
			task.Periods = task.Periods[:1]
			task.Operation = "lookup"
		}
		return task
	}})
	ctx, _ := agentTestContext()
	task, err := requestAgentTask(ctx, runtime, agentSendInput{Message: "8月和9月销售情况对比"}, nil, "")
	if err != nil || calls != 2 || len(task.Periods) != 2 {
		t.Fatalf("task=%+v calls=%d err=%v", task, calls, err)
	}
}
func TestAgentV2EmptySynthesisIsIncompleteAndReadable(t *testing.T) {
	seedAgentSalesComparison(t)
	script := &agentMockScript{Task: func(string) agentTask { return agentSalesTask() }, EmptyAnswer: true}
	ctx, trace := agentTestContext()
	out := runAgentTask(ctx, agentMockRuntime(t, script), models.AIAgentSession{}, agentSendInput{Message: "8月和9月销售情况对比"}, nil)
	if out.Status != "incomplete" || !strings.Contains(out.Answer, "¥1900.00") || strings.Contains(out.Answer, "\"values\"") || !strings.Contains(trace.Stop, "synthesis_error") {
		t.Fatalf("fallback=%+v trace=%+v", out, trace)
	}
}
func TestAgentV2CannotFinishWithOnlyOnePeriod(t *testing.T) {
	setupAudit(t)
	task := agentTask{Goal: "比较两月采购金额", Topic: "inbound", Operation: "compare", Periods: agentSalesTask().Periods, Metrics: []string{"采购金额"}, Scope: "all"}
	plan := agentPlan{Tool: "data_query", Data: agentDataRequest{Dataset: "orders", Operation: "aggregate", Aggregation: "count", StartDate: "2026-08-01", EndDate: "2026-08-31"}}
	script := &agentMockScript{Task: func(string) agentTask { return task }, Plans: []agentPlan{plan, {Tool: "finish"}, plan, plan}}
	ctx, trace := agentTestContext()
	out := runAgentTask(ctx, agentMockRuntime(t, script), models.AIAgentSession{}, agentSendInput{Message: "8月和9月采购对比"}, nil)
	if out.Status != "incomplete" || script.AnswerCalls != 0 || !strings.Contains(out.Answer, "2026年9月") || trace.Stop == "complete" {
		t.Fatalf("incomplete query accepted: %+v", out)
	}
}
func TestAgentV2CatalogNeverBecomesAnswer(t *testing.T) {
	setupAudit(t)
	script := &agentMockScript{Task: func(string) agentTask {
		return agentTask{Goal: "查看库存", Topic: "inventory", Operation: "lookup", Scope: "all"}
	}, Plans: []agentPlan{{Tool: "data_catalog"}, {Tool: "finish"}}}
	ctx, _ := agentTestContext()
	out := runAgentTask(ctx, agentMockRuntime(t, script), models.AIAgentSession{}, agentSendInput{Message: "库存情况"}, nil)
	if out.Status != "incomplete" || strings.Contains(out.Answer, "created_at") || len(out.Evidence) != 0 {
		t.Fatalf("catalog=%+v", out)
	}
}
func TestAgentV2PersistsTaskEvidenceAndTrace(t *testing.T) {
	seedAgentSalesComparison(t)
	if err := config.DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.AIProviderConfig{}).Error; err != nil {
		t.Fatal(err)
	}
	script := &agentMockScript{Task: func(user string) agentTask {
		task := agentSalesTask()
		if strings.Contains(user, "本轮用户原话：那9月呢") {
			if !strings.Contains(user, "上次任务") || !strings.Contains(user, "sales_revenue") {
				t.Error("semantic context missing")
			}
			task.Operation = "lookup"
			task.Periods = task.Periods[1:]
		}
		return task
	}}
	runtime := agentMockRuntime(t, script)
	t.Setenv("AI_API_KEY", runtime.APIKey)
	t.Setenv("AI_MODEL", runtime.Model)
	t.Setenv("AI_API_URL", runtime.URL)
	session := models.AIAgentSession{Owner: fmt.Sprintf("agent-v2-%d", time.Now().UnixNano())}
	if err := config.DB.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("username", session.Owner) })
	router.POST("/sessions/:id/messages", SendAIAgentMessage)
	for _, question := range []string{"8月和9月销售情况对比", "那9月呢"} {
		body, _ := json.Marshal(agentSendInput{Message: question, Thinking: "standard"})
		request := httptest.NewRequest("POST", "/sessions/"+itoa(session.ID)+"/messages", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"status":"complete"`) {
			t.Fatalf("response=%s", response.Body.String())
		}
	}
	var message models.AIAgentMessage
	if err := config.DB.Where("session_id=? AND role=?", session.ID, "assistant").Order("id DESC").First(&message).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message.Trace, "finish_reason") || !strings.Contains(message.Evidence, "2026-09") || !strings.Contains(message.Task, "sales_revenue") {
		t.Fatalf("missing audit data: %+v", message)
	}
}
func TestAgentReportZeroBaselineAndMissingPeriod(t *testing.T) {
	setupAudit(t)
	task := agentSalesTask()
	report, err := agentBuildReport(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Changes) != 0 || !report.Rows[0].Missing || !strings.Contains(agentRenderReport(report, task), "无法据此认定") {
		t.Fatalf("missing treated as zero: %+v", report)
	}
}
func TestAgentCallRecordsTruncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["reasoning_effort"] != "high" || payload["max_tokens"] != float64(16384) {
			t.Errorf("profile not sent: %+v", payload)
		}
		_ = json.NewEncoder(w).Encode(gin.H{"choices": []gin.H{{"message": gin.H{"content": "{unfinished"}, "finish_reason": "length"}}, "usage": gin.H{"prompt_tokens": 12, "completion_tokens": 16384}})
	}))
	defer server.Close()
	ctx, trace := agentTestContext()
	_, err := callAIChat(ctx, aiProviderRuntime{Provider: "deepseek", Model: "test", URL: server.URL, APIKey: "test", JSONMode: true}, "system", "user", true)
	if err != errAITruncated || len(trace.Calls) != 1 || trace.Calls[0].Finish != "length" {
		t.Fatalf("err=%v trace=%+v", err, trace)
	}
}

func TestAgentV2PayrollCorrectionAndAllEmployees(t *testing.T) {
	setupAudit(t)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	employee := models.Employee{Name: fmt.Sprintf("薪资测试%d", time.Now().UnixNano()), EmpNo: fmt.Sprint(time.Now().UnixNano()), EntryDate: &start, BaseSalary: 3200}
	if err := config.DB.Create(&employee).Error; err != nil {
		t.Fatal(err)
	}
	task := agentTask{Goal: "汇总9月应发工资", Topic: "payroll", Operation: "lookup", Basis: "payable", Scope: "all", Metrics: []string{"salary_payable"}, Periods: []agentTaskPeriod{{Label: "2026年9月", Start: "2026-09-01", End: "2026-09-30"}}}
	script := &agentMockScript{Task: func(string) agentTask { return task }}
	ctx, _ := agentTestContext()
	out := runAgentTask(ctx, agentMockRuntime(t, script), models.AIAgentSession{}, agentSendInput{Message: "我没有工资发放记录，你只看员工9月份应发的工资是多少就行"}, nil)
	if out.Status != "complete" || !strings.Contains(out.Answer, "应发工资") || strings.Contains(out.Answer, "请告诉我员工") {
		t.Fatalf("payroll correction: %+v", out)
	}
	task.Entity = employee.Name
	task.Scope = "one"
	report, err := agentBuildReport(context.Background(), task)
	if err != nil || len(report.Rows) != 1 || report.Rows[0].Records != 1 {
		t.Fatalf("individual payroll=%+v err=%v", report, err)
	}
}

func TestAgentV2RecordedPayrollDoesNotBecomePayable(t *testing.T) {
	setupAudit(t)
	task := agentTask{Goal: "9月实际发放工资", Topic: "payroll", Operation: "lookup", Basis: "paid", Scope: "all", Periods: []agentTaskPeriod{{Label: "2026年9月", Start: "2026-09-01", End: "2026-09-30"}}}
	script := &agentMockScript{Task: func(string) agentTask { return task }, Answer: "没有找到已登记的工资发放流水，因此不能核实实际支付总额。"}
	ctx, _ := agentTestContext()
	out := runAgentTask(ctx, agentMockRuntime(t, script), models.AIAgentSession{}, agentSendInput{Message: "9月实际发了多少工资"}, nil)
	if out.Status != "complete" || !strings.Contains(out.Answer, "不能核实") || !strings.Contains(out.Evidence[0].Result, "没有找到已登记") {
		t.Fatalf("paid payroll: %+v", out)
	}
}

func TestAgentCoverageRequiresEveryMetricInEveryPeriod(t *testing.T) {
	task := agentSalesTask()
	evidence := []agentEvidence{
		{Periods: task.Periods, Metrics: []string{"sales_revenue"}},
		{Periods: task.Periods[:1], Metrics: []string{"order_count", "units", "average_order"}},
	}
	missing := strings.Join(agentCoverageMissing(task, evidence), ",")
	if !strings.Contains(missing, "2026年9月的有效订单数") {
		t.Fatalf("missing=%s", missing)
	}
	if agentPeriodCovered(task.Periods[0], []agentTaskPeriod{{Start: "2026-08-01", End: "2026-08-15"}, {Start: "2026-08-17", End: "2026-08-31"}}) {
		t.Fatal("date gap accepted")
	}
	if !agentPeriodCovered(task.Periods[0], []agentTaskPeriod{{Start: "2026-08-01", End: "2026-08-15"}, {Start: "2026-08-16", End: "2026-08-31"}}) {
		t.Fatal("contiguous coverage rejected")
	}
}

func TestAgentV2SynthesisMissingReturnsToQuery(t *testing.T) {
	seedAgentSalesComparison(t)
	script := &agentMockScript{Task: func(string) agentTask { return agentSalesTask() }, MissingFirst: true, Plans: []agentPlan{{Tool: "data_query", Data: agentDataRequest{Dataset: "products", Operation: "rows", Fields: []string{"name"}, Limit: 5}}}}
	ctx, _ := agentTestContext()
	out := runAgentTask(ctx, agentMockRuntime(t, script), models.AIAgentSession{}, agentSendInput{Message: "8月和9月销售情况对比"}, nil)
	if out.Status != "complete" || script.PlanCalls != 1 || script.AnswerCalls != 2 || len(out.Evidence) != 2 {
		t.Fatalf("repair failed: %+v", out)
	}
}
func TestAgentV2ReportNumbersAllowDatesRejectInventedAmounts(t *testing.T) {
	task := agentSalesTask()
	report := agentBusinessReport{Rows: []agentReportRow{{Period: task.Periods[1], Values: map[string]float64{"order_count": 1}, Records: 1}}}
	body, _ := json.Marshal(report)
	evidence := []agentEvidence{{Plan: agentPlan{Tool: "business_report"}, Result: string(body)}}
	if !agentReportNumbersSupported("9月有1笔有效订单", task, evidence) {
		t.Fatal("valid date/count rejected")
	}
	if agentReportNumbersSupported("销售额是99999元", task, evidence) {
		t.Fatal("invented amount accepted")
	}
}
