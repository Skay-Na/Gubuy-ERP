package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/middlewares"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
)

// payrollTestRouter 独立测试路由：确认接口挂 OperationHistoryMiddleware，
// 验证工资确认动作真实生成可撤销的操作历史。
var (
	payrollRouterOnce sync.Once
	payrollTestRouter *gin.Engine
)

func setupPayrollTest(t *testing.T) {
	t.Helper()
	setupAudit(t)
	payrollRouterOnce.Do(func() {
		gin.SetMode(gin.TestMode)
		payrollTestRouter = gin.New()
		payrollTestRouter.POST("/api/payroll/confirm", middlewares.OperationHistoryMiddleware(), ConfirmPayroll)
		payrollTestRouter.GET("/api/finance/real-profit", GetRealProfit)
	})
}

// ensureOperatingLedgerAccount 确保经营开销台账账户存在（隔离测试库默认只有四个种子账户）。
func ensureOperatingLedgerAccount(t *testing.T) uint {
	t.Helper()
	var account models.Account
	if err := config.DB.Where("name = ?", "经营开销").First(&account).Error; err == nil {
		config.DB.Model(&account).Update("balance", 0)
		return account.ID
	}
	account = models.Account{ID: operatingExpenseAccountID, Name: "经营开销"}
	if err := config.DB.Create(&account).Error; err != nil {
		t.Fatalf("创建经营开销账户失败: %v", err)
	}
	return account.ID
}

// ensurePayrollEmployee 创建一个测试员工，返回员工 ID。
func ensurePayrollEmployee(t *testing.T, name string, status string) uint {
	t.Helper()
	employee := models.Employee{
		Name:             name,
		EmpNo:            fmt.Sprintf("PAY%s%d", status, time.Now().UnixNano()%10000000),
		BaseSalary:       2000,
		Role:             "sales",
		EmploymentStatus: status,
	}
	if err := config.DB.Create(&employee).Error; err != nil {
		t.Fatalf("创建测试员工失败: %v", err)
	}
	return employee.ID
}

func postPayroll(path string, body interface{}) *httptest.ResponseRecorder {
	jsonValue, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", path, bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	payrollTestRouter.ServeHTTP(w, req)
	return w
}

func payrollConfirmationCount(t *testing.T, employeeID uint, month string) int64 {
	t.Helper()
	var count int64
	config.DB.Model(&models.PayrollConfirmation{}).Where("employee_id = ? AND month = ?", employeeID, month).Count(&count)
	return count
}

// TestPayrollConfirmBookingsAndRejectsDuplicate 工资确认入账 + 幂等：
// 二次确认同月同员工被拒（409），不重复入账；同员工跨月可再次确认。
func TestPayrollConfirmBookingsAndRejectsDuplicate(t *testing.T) {
	setupPayrollTest(t)
	ledgerID := ensureOperatingLedgerAccount(t)
	employeeA := ensurePayrollEmployee(t, "工资测试甲", models.EmployeeStatusActive)
	employeeB := ensurePayrollEmployee(t, "工资测试乙", models.EmployeeStatusOffboarding)
	employeeDeparted := ensurePayrollEmployee(t, "工资测试离职", models.EmployeeStatusDeparted)
	month := time.Now().Format("2006-01")

	// 1. 正常确认两名员工
	w := postPayroll("/api/payroll/confirm", gin.H{
		"month": month,
		"items": []gin.H{
			{"employee_id": employeeA, "amount": 3000},
			{"employee_id": employeeB, "amount": 1500.555},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("工资确认应成功: %d %s", w.Code, w.Body.String())
	}

	// 确认记录落库
	if got := payrollConfirmationCount(t, employeeA, month); got != 1 {
		t.Fatalf("员工甲应有 1 条确认记录，实际 %d", got)
	}
	var confirmation models.PayrollConfirmation
	config.DB.Where("employee_id = ? AND month = ?", employeeB, month).First(&confirmation)
	if confirmation.Amount != 1500.56 {
		t.Fatalf("实发金额应保留两位小数 1500.56，实际 %.2f", confirmation.Amount)
	}

	// 经营开销账户记两笔"工资"支出，不进真实资金账户
	var ledger models.Account
	config.DB.First(&ledger, ledgerID)
	if balance := ledger.Balance; balance != -4500.56 {
		t.Fatalf("经营开销账户余额应为 -4500.56，实际 %.2f", balance)
	}
	var logCount int64
	config.DB.Model(&models.FinancialLog{}).
		Where("account_id = ? AND flow_type = 2 AND category = ?", ledgerID, "工资").Count(&logCount)
	if logCount != 2 {
		t.Fatalf("应写入 2 条工资支出流水，实际 %d", logCount)
	}
	var financialLog models.FinancialLog
	config.DB.Where("account_id = ? AND category = ? AND related_no = ?", ledgerID, "工资", fmt.Sprintf("PAY-%s-%d", month, employeeA)).First(&financialLog)
	if financialLog.Amount != 3000 {
		t.Fatalf("工资流水金额应为 3000，实际 %.2f", financialLog.Amount)
	}

	// 2. 幂等：同月同员工二次确认被拒
	duplicate := postPayroll("/api/payroll/confirm", gin.H{
		"month": month,
		"items": []gin.H{{"employee_id": employeeA, "amount": 3000}},
	})
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("二次确认应返回 409: %d %s", duplicate.Code, duplicate.Body.String())
	}
	var dupResp struct {
		Msg string `json:"msg"`
	}
	_ = json.Unmarshal(duplicate.Body.Bytes(), &dupResp)
	if !strings.Contains(dupResp.Msg, "已确认") {
		t.Fatalf("错误信息应说明已确认过，实际: %s", dupResp.Msg)
	}
	if got := payrollConfirmationCount(t, employeeA, month); got != 1 {
		t.Fatalf("被拒绝后确认记录不应增加，实际 %d", got)
	}
	config.DB.First(&ledger, ledgerID)
	if ledger.Balance != -4500.56 {
		t.Fatalf("被拒绝后经营开销余额不应变化，实际 %.2f", ledger.Balance)
	}

	// 3. 离职员工不能确认
	departed := postPayroll("/api/payroll/confirm", gin.H{
		"month": month,
		"items": []gin.H{{"employee_id": employeeDeparted, "amount": 1000}},
	})
	if departed.Code != http.StatusConflict {
		t.Fatalf("离职员工确认应返回 409: %d %s", departed.Code, departed.Body.String())
	}

	// 4. 同员工跨月可再次确认，且生成可撤销操作历史（增量断言：
	// operation_histories 不随 resetAuditDB 清空，不能做全量计数）
	nextMonth := time.Now().AddDate(0, 1, 0).Format("2006-01")
	var historyBefore int64
	config.DB.Model(&models.OperationHistory{}).Where("title = ?", "确认工资发放").Count(&historyBefore)
	next := postPayroll("/api/payroll/confirm", gin.H{
		"month": nextMonth,
		"items": []gin.H{{"employee_id": employeeA, "amount": 2800}},
	})
	if next.Code != http.StatusOK {
		t.Fatalf("跨月确认应成功: %d %s", next.Code, next.Body.String())
	}
	var historyAfter int64
	config.DB.Model(&models.OperationHistory{}).Where("title = ?", "确认工资发放").Count(&historyAfter)
	if historyAfter != historyBefore+1 {
		t.Fatalf("工资确认应新增 1 条操作历史，实际 %d -> %d", historyBefore, historyAfter)
	}
	if got := payrollConfirmationCount(t, employeeA, nextMonth); got != 1 {
		t.Fatalf("跨月确认后应有 1 条确认记录，实际 %d", got)
	}
}
