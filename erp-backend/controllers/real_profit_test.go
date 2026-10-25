package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
)

func getRealProfit(month string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("GET", "/api/finance/real-profit?month="+month, nil)
	w := httptest.NewRecorder()
	payrollTestRouter.ServeHTTP(w, req)
	return w
}

// TestRealProfitCalculation 真实净利润 = 毛利 - 经营开销 - 工资。
func TestRealProfitCalculation(t *testing.T) {
	setupPayrollTest(t)
	ensureOperatingLedgerAccount(t)
	month := time.Now().Format("2006-01")

	// 毛利：单价 2000、成本 1000 的商品卖一件 → 毛利 1000
	productID := ensureProduct(t, "真实净利润测试商品", 2000, 1000, 100)
	orderW := postJSON("/api/orders", gin.H{
		"items":            []gin.H{{"product_id": productID, "quantity": 1, "unit_price": 2000}},
		"payment_method":   1,
		"delivery_method":  2,
		"customer_name":    "利润测试客户",
		"customer_phone":   "13500000000",
		"customer_address": "利润测试地址",
	})
	if orderW.Code != http.StatusOK {
		t.Fatalf("下单失败: %d %s", orderW.Code, orderW.Body.String())
	}

	// 工资：走确认接口入经营开销账
	employeeID := ensurePayrollEmployee(t, "利润测试员工", models.EmployeeStatusActive)
	confirmW := postPayroll("/api/payroll/confirm", gin.H{
		"month": month,
		"items": []gin.H{{"employee_id": employeeID, "amount": 3000}},
	})
	if confirmW.Code != http.StatusOK {
		t.Fatalf("工资确认失败: %d %s", confirmW.Code, confirmW.Body.String())
	}

	// 其他经营开销：电费 500
	config.DB.Create(&models.FinancialLog{
		AccountID: operatingExpenseAccountID, FlowType: 2, Category: "电费",
		Amount: 500, BalanceAfter: -3500, Remark: "测试电费",
		CreatedAt: time.Now(),
	})

	w := getRealProfit(month)
	if w.Code != http.StatusOK {
		t.Fatalf("真实净利润查询失败: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data RealProfitResponse `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	result := resp.Data

	if margin := result.GrossProfit; margin != 1000 {
		t.Fatalf("毛利应为 1000，实际 %.2f", margin)
	}
	if result.ExpenseTotal != 500 {
		t.Fatalf("经营开销应为 500（不含工资），实际 %.2f", result.ExpenseTotal)
	}
	if result.PayrollTotal != 3000 {
		t.Fatalf("工资应为 3000，实际 %.2f", result.PayrollTotal)
	}
	if result.RealProfit != -2500 {
		t.Fatalf("真实净利润应为 -2500，实际 %.2f", result.RealProfit)
	}
	foundPayrollLine, foundUtilityLine := false, false
	for _, item := range result.Breakdown {
		if item.Category == "工资" && item.IsPayroll && item.Amount == 3000 {
			foundPayrollLine = true
		}
		if item.Category == "电费" && !item.IsPayroll && item.Amount == 500 {
			foundUtilityLine = true
		}
	}
	if !foundPayrollLine || !foundUtilityLine {
		t.Fatalf("分类明细应含工资(单独列)与电费: %+v", result.Breakdown)
	}
}

// TestRealProfitPayrollMonthFromRelatedNo 工资按 related_no 的所属月归属：
// 11 月初发放、related_no 为 PAY-2026-10-xxx 的工资计入 10 月而非 11 月；
// 解析失败（历史导入 EXP 前缀）的工资回退按流水 created_at 月份归属。
func TestRealProfitPayrollMonthFromRelatedNo(t *testing.T) {
	setupPayrollTest(t)
	ensureOperatingLedgerAccount(t)
	thisMonth := time.Now().Format("2006-01")
	lastMonth := time.Now().AddDate(0, -1, 0).Format("2006-01")

	// 11 月初发放的上月工资：流水 created_at 在本月，related_no 归属上月
	config.DB.Create(&models.FinancialLog{
		AccountID: operatingExpenseAccountID, FlowType: 2, Category: "工资",
		Amount: 800, BalanceAfter: -800, RelatedNo: fmt.Sprintf("PAY-%s-999", lastMonth),
		Remark: "上月工资本月发", CreatedAt: time.Now(),
	})
	// 历史导入格式（EXP 前缀，无法解析所属月）：回退按 created_at 归属本月
	config.DB.Create(&models.FinancialLog{
		AccountID: operatingExpenseAccountID, FlowType: 2, Category: "工资",
		Amount: 200, BalanceAfter: -1000, RelatedNo: "EXP20260101001",
		Remark: "导入历史工资", CreatedAt: time.Now(),
	})

	// 本月：只应包含回退口径的 200，不含归属上月的 800
	current := getRealProfit(thisMonth)
	if current.Code != http.StatusOK {
		t.Fatalf("本月真实净利润查询失败: %d %s", current.Code, current.Body.String())
	}
	var currentResp struct {
		Data RealProfitResponse `json:"data"`
	}
	_ = json.Unmarshal(current.Body.Bytes(), &currentResp)
	if currentResp.Data.PayrollTotal != 200 {
		t.Fatalf("本月工资应为 200（上月工资已按所属月划走），实际 %.2f", currentResp.Data.PayrollTotal)
	}

	// 上月：应包含 PAY-上月 前缀的 800
	previous := getRealProfit(lastMonth)
	if previous.Code != http.StatusOK {
		t.Fatalf("上月真实净利润查询失败: %d %s", previous.Code, previous.Body.String())
	}
	var previousResp struct {
		Data RealProfitResponse `json:"data"`
	}
	_ = json.Unmarshal(previous.Body.Bytes(), &previousResp)
	if previousResp.Data.PayrollTotal != 800 {
		t.Fatalf("上月工资应为 800（月初发上月工资计入干活月），实际 %.2f", previousResp.Data.PayrollTotal)
	}
	if previousResp.Data.RealProfit != -800 {
		t.Fatalf("上月真实净利润应为 -800（毛利 0 - 开销 0 - 工资 800），实际 %.2f", previousResp.Data.RealProfit)
	}
}
