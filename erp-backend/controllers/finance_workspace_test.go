package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
)

// getWorkspace 调用工作台聚合接口
func getWorkspace(month string) *httptest.ResponseRecorder {
	path := "/api/finance/workspace"
	if month != "" {
		path += "?month=" + month
	}
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	payrollTestRouter.ServeHTTP(w, req)
	return w
}

func workspaceData(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("workspace 查询应成功: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析 workspace 响应失败: %v", err)
	}
	return resp.Data
}

// TestFinanceWorkspaceFrozenTodosAndAlerts 校验冻结口径、待办口径与异常提醒。
func TestFinanceWorkspaceFrozenTodosAndAlerts(t *testing.T) {
	setupPayrollTest(t)
	month := time.Now().Format("2006-01")

	// 台账账户不应出现在资金账户里
	ensureOperatingLedgerAccount(t)

	// 国补到账待核销：status=2，pending 100 已核 20 → 剩 80
	allocateOrder := models.Order{
		OrderNo: time.Now().Format("20060102150405") + "WSA1", OrderStatus: 1,
		SubsidyStatus: 2, SubsidyPendingAmount: 100, SubsidyReceivedAmt: 20,
		PaymentStatus: 2, TotalAmount: 1000,
	}
	config.DB.Create(&allocateOrder)

	// 待提交资料：status=1，pending 50
	submitOrder := models.Order{
		OrderNo: time.Now().Format("20060102150405") + "WSS1", OrderStatus: 1,
		SubsidyStatus: 1, SubsidyPendingAmount: 50, SubsidyReceivedAmt: 0,
		PaymentStatus: 2, TotalAmount: 500,
	}
	config.DB.Create(&submitOrder)

	// 尾款待收：仅付定金
	depositOrder := models.Order{
		OrderNo: time.Now().Format("20060102150405") + "WSB1", OrderStatus: 1,
		PaymentStatus: 1, TotalAmount: 800, ActualPayAmount: 800, DepositAmount: 300,
	}
	config.DB.Create(&depositOrder)

	// 异常：已取消订单仍挂 pending 30
	cancelledOrder := models.Order{
		OrderNo: time.Now().Format("20060102150405") + "WSC1", OrderStatus: 2,
		SubsidyStatus: 2, SubsidyPendingAmount: 30, SubsidyReceivedAmt: 0,
		PaymentStatus: 2, TotalAmount: 300,
	}
	config.DB.Create(&cancelledOrder)

	data := workspaceData(t, getWorkspace(""))

	// funds：台账账户被排除，冻结 = 80
	funds := data["funds"].(map[string]interface{})
	accounts := funds["accounts"].([]interface{})
	for _, item := range accounts {
		account := item.(map[string]interface{})
		if account["name"] == "经营开销" || account["name"] == "历史调整台账" {
			t.Fatalf("资金账户列表不应包含台账账户: %v", account)
		}
	}
	frozen := funds["frozen"].(map[string]interface{})
	if frozen["count"].(float64) != 1 {
		t.Fatalf("冻结笔数应为 1，实际 %v", frozen["count"])
	}
	if frozen["amount"].(float64) != 80 {
		t.Fatalf("冻结金额应为 80，实际 %v", frozen["amount"])
	}

	// todos
	todos := data["todos"].(map[string]interface{})
	toAllocate := todos["subsidy_to_allocate"].(map[string]interface{})
	if toAllocate["count"].(float64) != 1 || toAllocate["amount"].(float64) != 80 {
		t.Fatalf("国补待核销应为 1 笔 80，实际 %v %v", toAllocate["count"], toAllocate["amount"])
	}
	allocateItems := toAllocate["items"].([]interface{})
	if len(allocateItems) != 1 {
		t.Fatalf("待核销明细应为 1 条，实际 %d", len(allocateItems))
	}
	firstItem := allocateItems[0].(map[string]interface{})
	if firstItem["customer_name"] == nil || firstItem["created_at"] == nil {
		t.Fatalf("待办明细必须带客户姓名与录单日期: %v", firstItem)
	}
	toSubmit := todos["subsidy_to_submit"].(map[string]interface{})
	if toSubmit["count"].(float64) != 1 || toSubmit["amount"].(float64) != 50 {
		t.Fatalf("待提交资料应为 1 笔 50，实际 %v %v", toSubmit["count"], toSubmit["amount"])
	}
	balanceDue := todos["balance_due"].(map[string]interface{})
	if balanceDue["amount"].(float64) != 500 {
		t.Fatalf("尾款待收应为 500（实付 800 - 定金 300），实际 %v", balanceDue["amount"])
	}

	// alerts：已取消订单挂 pending
	alerts := data["alerts"].([]interface{})
	if len(alerts) != 1 {
		t.Fatalf("应有一条已取消挂 pending 告警，实际 %d", len(alerts))
	}
	alert := alerts[0].(map[string]interface{})
	if alert["count"].(float64) != 1 || alert["amount"].(float64) != 30 {
		t.Fatalf("告警应为 1 笔 30，实际 %v %v", alert["count"], alert["amount"])
	}

	// 本月概况的记账笔数存在（当前月）
	monthData := workspaceData(t, getWorkspace(month))["month"].(map[string]interface{})
	if monthData["booked_count"].(float64) < 0 {
		t.Fatalf("booked_count 不应为负")
	}
}

// TestFinanceWorkspaceBridgeReconcile 桥接对平：
// 真实净利润 - 净现金流 = profit_cash_gap，且桥接项数值正确。
func TestFinanceWorkspaceBridgeReconcile(t *testing.T) {
	setupPayrollTest(t)
	ensureOperatingLedgerAccount(t)
	month := time.Now().Format("2006-01")

	// 毛利 1000：售价 2000、成本 1000
	productID := ensureProduct(t, "工作台桥接测试商品", 2000, 1000, 100)
	if w := postJSON("/api/orders", gin.H{
		"items":            []gin.H{{"product_id": productID, "quantity": 1, "unit_price": 2000}},
		"payment_method":   1,
		"delivery_method":  2,
		"customer_name":    "桥接测试客户",
		"customer_phone":   "13400000000",
		"customer_address": "桥接地址",
	}); w.Code != http.StatusOK {
		t.Fatalf("下单失败: %d %s", w.Code, w.Body.String())
	}

	// 工资 3000（确认入经营开销账）+ 电费 500（直接入台账）
	employeeID := ensurePayrollEmployee(t, "桥接测试员工", models.EmployeeStatusActive)
	if w := postPayroll("/api/payroll/confirm", gin.H{
		"month": month,
		"items": []gin.H{{"employee_id": employeeID, "amount": 3000}},
	}); w.Code != http.StatusOK {
		t.Fatalf("工资确认失败: %d %s", w.Code, w.Body.String())
	}
	config.DB.Create(&models.FinancialLog{
		AccountID: operatingExpenseAccountID, FlowType: 2, Category: "电费",
		Amount: 500, BalanceAfter: -3500, Remark: "桥接测试电费", CreatedAt: time.Now(),
	})

	data := workspaceData(t, getWorkspace(month))
	monthData := data["month"].(map[string]interface{})

	if monthData["gross_profit"].(float64) != 1000 {
		t.Fatalf("毛利应为 1000，实际 %v", monthData["gross_profit"])
	}
	if monthData["expense_total"].(float64) != 500 {
		t.Fatalf("经营开销（不含工资）应为 500，实际 %v", monthData["expense_total"])
	}
	if monthData["payroll_total"].(float64) != 3000 {
		t.Fatalf("工资应为 3000，实际 %v", monthData["payroll_total"])
	}
	// 净利润 = 1000 - 500 = 500；真实净利润 = 500 - 3000 = -2500
	if monthData["net_profit"].(float64) != 500 {
		t.Fatalf("净利润应为 500，实际 %v", monthData["net_profit"])
	}
	if monthData["real_profit"].(float64) != -2500 {
		t.Fatalf("真实净利润应为 -2500，实际 %v", monthData["real_profit"])
	}
	// 净现金流：卖货收款 2000 进支付宝，台账费用不动现金流 → 2000
	if monthData["net_cash_flow"].(float64) != 2000 {
		t.Fatalf("净现金流应为 2000，实际 %v", monthData["net_cash_flow"])
	}

	// 桥接对平：gap = real_profit - net_cash_flow
	bridge := monthData["bridge"].(map[string]interface{})
	gap := bridge["profit_cash_gap"].(float64)
	expectedGap := monthData["real_profit"].(float64) - monthData["net_cash_flow"].(float64)
	if gap != expectedGap {
		t.Fatalf("桥接差异应对平 real_profit - net_cash_flow = %v，实际 %v", expectedGap, gap)
	}
	if bridge["operating_expense"].(float64) != 3500 {
		t.Fatalf("桥接经营开销（含工资）应为 3500，实际 %v", bridge["operating_expense"])
	}
	if bridge["payroll"].(float64) != 3000 {
		t.Fatalf("桥接工资应为 3000，实际 %v", bridge["payroll"])
	}

	// 累计档可用
	cumulativeData := workspaceData(t, getWorkspace("cumulative"))
	if cumulativeData["month"].(map[string]interface{})["month"] != "cumulative" {
		t.Fatal("cumulative 档 month 标识应为 cumulative")
	}
}
