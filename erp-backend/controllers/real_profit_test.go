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
