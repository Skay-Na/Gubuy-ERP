package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"erp-backend/config"
	"erp-backend/models"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestAgentPlanRepairsMalformedModelResponse(t *testing.T) {
	responses := []string{
		"我来看看。",
		"<think>已分析</think>\n```json\n{\"tool\":\"data_query\",\"data\":{\"dataset\":\"orders\",\"operation\":\"aggregate\",\"aggregation\":\"count\"}}\n```",
	}
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		content := responses[calls]
		calls++
		_ = json.NewEncoder(w).Encode(gin.H{"choices": []gin.H{{"message": gin.H{"content": content}}}})
	}))
	defer model.Close()
	plan, err := requestAgentPlan(context.Background(), aiProviderRuntime{
		Provider: "env", Model: "test-model", APIKey: "test-key", URL: model.URL, JSONMode: true,
	}, agentInstructions, "分析订单")
	if err != nil || calls != 2 || plan.Tool != "data_query" || plan.Data.Dataset != "orders" {
		t.Fatalf("repaired plan = %+v, calls=%d, err=%v", plan, calls, err)
	}
}

func TestRequestAgentPlanRetriesEmptyJSONWithoutJSONMode(t *testing.T) {
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		content := ""
		if calls == 2 {
			if _, present := request["response_format"]; present {
				t.Error("retry should disable JSON mode after empty content")
			}
			content = `{"tool":"finish","answer":"已恢复"}`
		}
		_ = json.NewEncoder(w).Encode(gin.H{"choices": []gin.H{{"message": gin.H{"content": content}}}})
	}))
	defer model.Close()
	plan, err := requestAgentPlan(context.Background(), aiProviderRuntime{
		Provider: "deepseek", Model: "deepseek-flash", APIKey: "test-key", URL: model.URL, JSONMode: true,
	}, agentInstructions, "查询数据")
	if err != nil || calls != 2 || plan.Tool != "finish" || plan.Answer != "已恢复" {
		t.Fatalf("empty-response retry = %+v, calls=%d, err=%v", plan, calls, err)
	}
}

func TestAgentPlannerConversationKeepsCorrectionWithoutRawEvidence(t *testing.T) {
	recent := []models.AIAgentMessage{
		{Role: "user", Content: "上个月订单有多少"},
		{Role: "assistant", Content: "订单 12 笔。\n\n查询依据：\n数据库字段 balance,created_at,id"},
		{Role: "user", Content: "我说的是销售额，不是订单数量"},
	}
	context := agentPlannerConversation(recent)
	if !strings.Contains(context, "我说的是销售额") || strings.Contains(context, "balance,created_at") {
		t.Fatalf("planner context = %s", context)
	}
}

func TestAgentExpenseConfirmationIsOneUse(t *testing.T) {
	setupAudit(t)
	if err := config.DB.Exec("DELETE FROM ai_agent_actions").Error; err != nil {
		t.Fatal(err)
	}
	if err := config.DB.Exec("DELETE FROM ai_agent_sessions").Error; err != nil {
		t.Fatal(err)
	}
	var account models.Account
	if err := config.DB.First(&account).Error; err != nil {
		t.Fatal(err)
	}
	if err := config.DB.Model(&account).Update("balance", 1000).Error; err != nil {
		t.Fatal(err)
	}
	session := models.AIAgentSession{Owner: "agent-test"}
	if err := config.DB.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	action := models.AIAgentAction{SessionID: session.ID, Owner: session.Owner, Kind: "expense", Payload: "{}", Status: "pending", ExpiresAt: time.Now().Add(time.Hour)}
	if err := config.DB.Create(&action).Error; err != nil {
		t.Fatal(err)
	}
	idempotencyKey := "chat:test-expense-confirmation-" + itoa(action.ID)
	workTask := models.AgentWorkTask{
		TenantID: "single", Kind: "chat_request", Status: "WAITING_APPROVAL", Step: "awaiting_confirmation",
		Attempts: 1, CreatedBy: session.Owner, IdempotencyKey: &idempotencyKey,
	}
	if err := config.DB.Create(&workTask).Error; err != nil {
		t.Fatal(err)
	}
	actionID := action.ID
	workTaskID := workTask.ID
	if err := config.DB.Create(&models.AIAgentMessage{
		SessionID: session.ID, Role: "assistant", Content: "待确认", ActionID: &actionID, TaskID: &workTaskID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("username", "agent-test") })
	router.POST("/actions/:id/confirm", ConfirmAIAgentAction)
	requestBody, _ := json.Marshal(gin.H{"entry_type": 2, "category": "房租", "amount": 80, "account_id": account.ID})
	request := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/actions/"+itoa(action.ID)+"/confirm", bytes.NewReader(requestBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		return recorder
	}
	if response := request(); response.Code != http.StatusOK {
		t.Fatalf("first confirmation: %d %s", response.Code, response.Body.String())
	}
	if response := request(); response.Code != http.StatusConflict {
		t.Fatalf("repeat confirmation: %d %s", response.Code, response.Body.String())
	}
	if err := config.DB.First(&account, account.ID).Error; err != nil {
		t.Fatal(err)
	}
	if account.Balance != 920 {
		t.Fatalf("balance = %.2f, want 920", account.Balance)
	}
	var count int64
	if err := config.DB.Model(&models.FinancialLog{}).Where("category = ?", "房租").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("entries = %d, want 1", count)
	}
	if err := config.DB.First(&workTask, workTask.ID).Error; err != nil {
		t.Fatal(err)
	}
	if workTask.Status != "SUCCEEDED" || workTask.Step != "confirmed" {
		t.Fatalf("confirmed chat task = %s/%s, want SUCCEEDED/confirmed", workTask.Status, workTask.Step)
	}
	if err := config.DB.Model(&models.AgentActivity{}).Where("task_id = ? AND event = ?", workTask.ID, "chat_action_confirmed").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("chat task confirmation activities = %d, want 1", count)
	}
}

func itoa(id uint) string { return strconv.FormatUint(uint64(id), 10) }

func TestAgentOrderConfirmationIsOneUse(t *testing.T) {
	setupAudit(t)
	productID := ensureProduct(t, "Agent确认测试商品", 100, 60, 5)
	session := models.AIAgentSession{Owner: "agent-order-test"}
	if err := config.DB.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	action := models.AIAgentAction{SessionID: session.ID, Owner: session.Owner, Kind: "order", Payload: "{}", Status: "pending", ExpiresAt: time.Now().Add(time.Hour)}
	if err := config.DB.Create(&action).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("username", session.Owner) })
	router.POST("/orders", CreateOrder)
	body, _ := json.Marshal(gin.H{"agent_action_id": action.ID, "items": []gin.H{{"product_id": productID, "quantity": 1, "unit_price": 100}}, "customer_name": "Agent客户", "payment_method": 1, "delivery_method": 2})
	request := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		return recorder
	}
	if response := request(); response.Code != 200 {
		t.Fatalf("first order confirmation: %d %s", response.Code, response.Body.String())
	}
	if response := request(); response.Code != 409 {
		t.Fatalf("repeat order confirmation: %d %s", response.Code, response.Body.String())
	}
	var count int64
	if err := config.DB.Model(&models.Order{}).Where("customer_name = ?", "Agent客户").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("order count = %d, want 1", count)
	}
}

func TestAgentAnswersEmployeeCommissionFromReportRules(t *testing.T) {
	setupAudit(t)
	month := time.Now().AddDate(0, -1, 0)
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.Local)
	name := "Agent提成员工" + strconv.FormatInt(time.Now().UnixNano(), 10)
	employee := models.Employee{Name: name, EmpNo: name, EntryDate: &start}
	if err := config.DB.Create(&employee).Error; err != nil {
		t.Fatal(err)
	}
	productID := ensureProduct(t, "Agent提成商品", 100, 60, 5)
	order := models.Order{
		OrderNo:      "AGENT-COMMISSION-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		CustomerName: "提成客户", EmployeeID: &employee.ID, OrderStatus: 1,
		OrderType: 1, CommissionAmount: 12.34, CreatedAt: start.AddDate(0, 0, 2),
		OrderItems: []models.OrderItem{{ProductID: productID, UnitPrice: 100, Quantity: 2}},
	}
	if err := config.DB.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	got, err := agentEmployeeCommission(name, start.Format("2006-01"), "")
	if err != nil || !strings.Contains(got, "提成：¥12.34") || !strings.Contains(got, "销售额 ¥200.00") {
		t.Fatalf("commission = %q, err = %v", got, err)
	}
	orders, err := agentEmployeeOrders(name, "last_month")
	if err != nil || !strings.Contains(orders, order.OrderNo) {
		t.Fatalf("employee orders = %q, err = %v", orders, err)
	}
	if _, message, err := agentFindEmployee("不存在的员工"); err != nil || !strings.Contains(message, "没有找到员工") {
		t.Fatalf("missing employee message = %q, err = %v", message, err)
	}
	if help := agentSystemHelp("员工提成"); !strings.Contains(help, "月度提成报表") {
		t.Fatalf("employee help = %q", help)
	}
	plan, err := normalizeAgentEmployeePlan(agentPlan{Tool: "customer_orders", Customer: name},
		"她上个月提成多少", []models.AIAgentMessage{{Role: "user", Content: name + "是我的员工"}})
	if err != nil || plan.Tool != "employee_commission" || plan.Employee != name || plan.Period != "last_month" {
		t.Fatalf("normalized plan = %+v, err = %v", plan, err)
	}
	missingName, err := normalizeAgentEmployeePlan(agentPlan{Tool: "finish"}, "查询员工上月提成", nil)
	if err != nil || missingName.Tool != "employee_commission" {
		t.Fatalf("unnamed employee plan = %+v, err = %v", missingName, err)
	}
	if helpPlan := normalizeAgentHelpPlan(agentPlan{Tool: "finish"}, "员工提成报表在哪里"); helpPlan.Tool != "system_help" {
		t.Fatalf("help plan = %+v", helpPlan)
	}
}

// TestAgentCommissionSkipsChamberOrdersBeforeStartDate AI 店长提成口径：
// 2026-10 之前的商会订单不计分成（chamberCommissionStart），
// 2026-10 起的商会订单正常按在职正式员工分摊，与员工月度提成报表一致。
func TestAgentCommissionSkipsChamberOrdersBeforeStartDate(t *testing.T) {
	setupAudit(t)
	// 报表按"当月在职正式员工"分摊商会池，先清历史用例残留员工保证分母确定。
	if err := config.DB.Unscoped().Where("is_outsourced = ?", false).Delete(&models.Employee{}).Error; err != nil {
		t.Fatalf("清理残留员工失败: %v", err)
	}
	entryDate := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	unique := strconv.FormatInt(time.Now().UnixNano(), 10)
	employee := models.Employee{
		Name: "商会口径员工" + unique, EmpNo: "CH-" + unique,
		Role: "sales", EntryDate: &entryDate, BaseSalary: 1000,
	}
	if err := config.DB.Create(&employee).Error; err != nil {
		t.Fatal(err)
	}
	legacyOrder := models.Order{
		OrderNo: "AGENT-CHAMBER-OLD-" + unique, CustomerName: "商会老单客户", OrderStatus: 1,
		OrderType: 2, TotalAmount: 100, ActualPayAmount: 100,
		CreatedAt: time.Date(2026, 8, 15, 12, 0, 0, 0, time.Local),
	}
	if err := config.DB.Create(&legacyOrder).Error; err != nil {
		t.Fatal(err)
	}
	activeOrder := models.Order{
		OrderNo: "AGENT-CHAMBER-NEW-" + unique, CustomerName: "商会新单客户", OrderStatus: 1,
		OrderType: 2, TotalAmount: 100, ActualPayAmount: 100,
		CreatedAt: time.Date(2026, 10, 10, 12, 0, 0, 0, time.Local),
	}
	if err := config.DB.Create(&activeOrder).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		config.DB.Unscoped().Delete(&legacyOrder)
		config.DB.Unscoped().Delete(&activeOrder)
		config.DB.Unscoped().Delete(&employee)
	})

	// 2026-08：商会老单早于起计点 → 提成为 0，不出现分成明细
	answer, err := agentEmployeeCommission(employee.Name, "2026-08", "")
	if err != nil {
		t.Fatalf("legacy month answer: %v", err)
	}
	if !strings.Contains(answer, "提成：¥0.00") || !strings.Contains(answer, "商会订单分摊 ¥0.00") {
		t.Fatalf("legacy chamber order must not pay commission: %q", answer)
	}

	// 2026-10：新商会单正常分摊（池 100×1%=1 元，唯一在职员工独得 1.00）
	answer, err = agentEmployeeCommission(employee.Name, "2026-10", "")
	if err != nil {
		t.Fatalf("active month answer: %v", err)
	}
	if !strings.Contains(answer, "商会订单分摊 ¥1.00") {
		t.Fatalf("active chamber order should pay commission: %q", answer)
	}
}

func TestAgentCommissionFollowUpExpandsToAllEmployees(t *testing.T) {
	setupAudit(t)
	month := time.Now().AddDate(0, -1, 0)
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.Local)
	unique := strconv.FormatInt(time.Now().UnixNano(), 10)
	first := models.Employee{Name: "甲" + unique, EmpNo: "A" + unique, EntryDate: &start}
	second := models.Employee{Name: "乙" + unique, EmpNo: "B" + unique, EntryDate: &start}
	for _, employee := range []*models.Employee{&first, &second} {
		if err := config.DB.Create(employee).Error; err != nil {
			t.Fatal(err)
		}
	}
	for index, employee := range []*models.Employee{&first, &second} {
		order := models.Order{
			OrderNo:      "AGENT-ALL-" + unique + "-" + strconv.Itoa(index),
			CustomerName: "提成客户", EmployeeID: &employee.ID, OrderStatus: 1,
			OrderType: 1, CommissionAmount: float64(10 * (index + 1)), CreatedAt: start.AddDate(0, 0, 2),
		}
		if err := config.DB.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
	}
	plan, err := normalizeAgentEmployeePlan(agentPlan{Tool: "finish"}, "我问所有人的", []models.AIAgentMessage{
		{Role: "user", Content: first.Name + "上个月提成多少"},
		{Role: "assistant", Content: "个人提成已查询"},
		{Role: "user", Content: "我问所有人的"},
	})
	if err != nil || plan.Tool != "employee_commission" || plan.Employee != "all" || plan.Period != "last_month" {
		t.Fatalf("follow-up plan = %+v, err = %v", plan, err)
	}
	directPlan, err := normalizeAgentEmployeePlan(agentPlan{Tool: "finish"}, "上个月员工提成一共有多少", nil)
	if err != nil || directPlan.Tool != "employee_commission" || directPlan.Employee != "all" || directPlan.Period != "last_month" {
		t.Fatalf("direct total plan = %+v, err = %v", directPlan, err)
	}
	monthPlan, err := normalizeAgentEmployeePlan(agentPlan{Tool: "finish"}, "我问所有人的", []models.AIAgentMessage{
		{Role: "user", Content: first.Name + "在 2026-09 的提成是多少"},
		{Role: "user", Content: "我问所有人的"},
	})
	if err != nil || monthPlan.Tool != "employee_commission" || monthPlan.Employee != "all" || monthPlan.Month != "2026-09" {
		t.Fatalf("explicit-month follow-up plan = %+v, err = %v", monthPlan, err)
	}
	answer, err := agentEmployeeCommission(plan.Employee, plan.Month, plan.Period)
	if err != nil || !strings.Contains(answer, "提成合计：") || !strings.Contains(answer, first.Name) || !strings.Contains(answer, second.Name) {
		t.Fatalf("all-employee commission = %q, err = %v", answer, err)
	}
	if strings.Contains(answer, "请告诉我员工姓名或工号") {
		t.Fatalf("unexpected employee-name prompt: %q", answer)
	}
}
