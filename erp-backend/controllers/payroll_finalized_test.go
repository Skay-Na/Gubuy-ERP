package controllers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
)

// createFinalizedCaliberEmployee 创建入职日早于薪资月的正式销售员工，
// 并补一条当月考勤（请假 0 天）——否则无考勤月份会把月初到今天的未打卡全算缺勤扣款，
// 快照应发就不再等于底薪+提成，测试无法断言。
func createFinalizedCaliberEmployee(t *testing.T, name string, baseSalary float64, month string) models.Employee {
	t.Helper()
	entry := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	employee := models.Employee{
		Name:             name,
		EmpNo:            fmt.Sprintf("FN%d%s", time.Now().UnixNano()%10000000, name),
		BaseSalary:       baseSalary,
		Role:             "sales",
		EmploymentStatus: models.EmployeeStatusActive,
		EntryDate:        &entry,
	}
	if err := config.DB.Create(&employee).Error; err != nil {
		t.Fatalf("创建定稿口径测试员工失败: %v", err)
	}
	attendance := models.MonthlyAttendance{EmployeeID: employee.ID, Month: month, LeaveDays: 0, ActualDays: 26}
	if err := config.DB.Create(&attendance).Error; err != nil {
		t.Fatalf("创建定稿口径测试考勤失败: %v", err)
	}
	return employee
}

// createPersonalCommissionOrder 创建一笔归属指定员工的个人提成订单（快照口径测试用）。
func createPersonalCommissionOrder(t *testing.T, orderNo string, employeeID uint, commission float64, createdAt time.Time) models.Order {
	t.Helper()
	order := models.Order{
		OrderNo: orderNo, CustomerName: "定稿口径客户", EmployeeID: &employeeID, OrderStatus: 1,
		OrderType: 1, TotalAmount: 1000, CommissionAmount: commission, CreatedAt: createdAt,
	}
	if err := config.DB.Create(&order).Error; err != nil {
		t.Fatalf("创建定稿口径测试订单失败: %v", err)
	}
	return order
}

// fetchPayrollHistoryFor 拉取员工工资条（payrollTestRouter）。
func fetchPayrollHistoryFor(t *testing.T, employeeID uint, year int) []PayrollHistoryMonth {
	t.Helper()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/employees/%d/payroll-history?year=%d", employeeID, year), nil)
	w := httptest.NewRecorder()
	payrollTestRouter.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("工资条查询失败: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Months []PayrollHistoryMonth `json:"months"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Data.Months
}

// TestPayrollHistoryFinalizedShowsSnapshot 定稿月工资条展示快照数字：
// 确认发放后订单再变动，应发仍显示定稿快照值（发薪那一刻的数），
// 实时回算存 live_payable 供比对，差额按"快照应发 − 实付"计。
func TestPayrollHistoryFinalizedShowsSnapshot(t *testing.T) {
	setupPayrollTest(t)
	ensureOperatingLedgerAccount(t)
	month := time.Now().Format("2006-01")
	employee := createFinalizedCaliberEmployee(t, "定稿展示员工", 5000, month)
	monthStart, _ := time.ParseInLocation("2006-01", month, time.Local)
	unique := strconv.FormatInt(time.Now().UnixNano(), 10)

	// 确认前订单：提成 100 → 快照应发 = floor(5000 + 100) = 5100
	original := createPersonalCommissionOrder(t, "FN-OLD-"+unique, employee.ID, 100, monthStart.AddDate(0, 0, 2))
	t.Cleanup(func() { config.DB.Unscoped().Delete(&original) })

	confirmW := postPayroll("/api/payroll/confirm", gin.H{
		"month": month,
		"items": []gin.H{{"employee_id": employee.ID, "amount": 5100}},
	})
	if confirmW.Code != http.StatusOK {
		t.Fatalf("工资确认失败: %d %s", confirmW.Code, confirmW.Body.String())
	}

	// 确认后再补一笔订单：实时回算 = floor(5000 + 200) = 5200 ≠ 定稿 5100
	late := createPersonalCommissionOrder(t, "FN-LATE-"+unique, employee.ID, 100, monthStart.AddDate(0, 0, 3))
	t.Cleanup(func() { config.DB.Unscoped().Delete(&late) })

	months := fetchPayrollHistoryFor(t, employee.ID, time.Now().Year())
	var row *PayrollHistoryMonth
	for i := range months {
		if months[i].Month == month {
			row = &months[i]
			break
		}
	}
	if row == nil {
		t.Fatalf("工资条缺少定稿月 %s: %+v", month, months)
	}
	if !row.Finalized {
		t.Fatalf("确认发放过的月份应标记定稿: %+v", row)
	}
	if row.Payable != 5100 {
		t.Fatalf("定稿月应发应展示快照值 5100，实际 %.2f", row.Payable)
	}
	if row.LivePayable != 5200 {
		t.Fatalf("实时回算应存 live_payable = 5200，实际 %.2f", row.LivePayable)
	}
	if row.Paid != 5100 {
		t.Fatalf("实付应为确认台账金额 5100，实际 %.2f", row.Paid)
	}
	if row.Difference != 0 {
		t.Fatalf("差额应为快照应发 − 实付 = 0，实际 %.2f", row.Difference)
	}
}

// TestDashboardPayrollMismatchTodo 看板"定稿工资变动"待办：
// ① 快照与回算一致 → 无待办；
// ② 确认后订单变动 → 出待办且金额正确；未变动员工不误报。
func TestDashboardPayrollMismatchTodo(t *testing.T) {
	setupPayrollTest(t)
	ensureOperatingLedgerAccount(t)
	month := time.Now().Format("2006-01")
	employeeA := createFinalizedCaliberEmployee(t, "定稿变动员工", 2000, month)
	employeeB := createFinalizedCaliberEmployee(t, "定稿稳定员工", 3000, month)
	monthStart, _ := time.ParseInLocation("2006-01", month, time.Local)
	unique := strconv.FormatInt(time.Now().UnixNano(), 10)

	// A 有提成 100 订单 → 确认时快照应发 = floor(2000 + 100) = 2100；B 无订单 → 3000
	orderA := createPersonalCommissionOrder(t, "MM-A-"+unique, employeeA.ID, 100, monthStart.AddDate(0, 0, 2))
	t.Cleanup(func() { config.DB.Unscoped().Delete(&orderA) })

	confirmW := postPayroll("/api/payroll/confirm", gin.H{
		"month": month,
		"items": []gin.H{
			{"employee_id": employeeA.ID, "amount": 2100},
			{"employee_id": employeeB.ID, "amount": 3000},
		},
	})
	if confirmW.Code != http.StatusOK {
		t.Fatalf("工资确认失败: %d %s", confirmW.Code, confirmW.Body.String())
	}

	// ① 无订单变动：快照与回算一致，A/B 都不出待办
	mismatches, err := finalizedPayrollMismatches()
	if err != nil {
		t.Fatalf("定稿工资比对失败: %v", err)
	}
	for _, item := range mismatches {
		if item.EmployeeID == employeeA.ID || item.EmployeeID == employeeB.ID {
			t.Fatalf("无订单变动不应出待办: %+v", item)
		}
	}

	// ② 确认后 A 补一笔提成 100 订单：回算 2200 ≠ 定稿 2100 → 出待办；B 不变 → 不误报
	lateOrder := createPersonalCommissionOrder(t, "MM-LATE-"+unique, employeeA.ID, 100, monthStart.AddDate(0, 0, 3))
	t.Cleanup(func() { config.DB.Unscoped().Delete(&lateOrder) })

	mismatches, err = finalizedPayrollMismatches()
	if err != nil {
		t.Fatalf("定稿工资比对失败: %v", err)
	}
	foundA := false
	for _, item := range mismatches {
		if item.EmployeeID == employeeA.ID {
			foundA = true
			if item.Month != month {
				t.Fatalf("待办薪资月应为 %s，实际 %s", month, item.Month)
			}
			if item.EmployeeName != employeeA.Name {
				t.Fatalf("待办应带员工姓名 %s，实际 %s", employeeA.Name, item.EmployeeName)
			}
			if math.Abs(item.FinalizedPayable-2100) > 0.01 || math.Abs(item.LivePayable-2200) > 0.01 {
				t.Fatalf("待办金额不正确（定稿 2100 / 回算 2200）: %+v", item)
			}
		}
		if item.EmployeeID == employeeB.ID {
			t.Fatalf("未变动员工不应出待办: %+v", item)
		}
	}
	if !foundA {
		t.Fatalf("订单变动后应出定稿工资待办: %+v", mismatches)
	}
}
