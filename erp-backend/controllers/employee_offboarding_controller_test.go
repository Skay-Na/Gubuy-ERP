package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
)

func TestCalculateOffboardingBaseSalary(t *testing.T) {
	got, err := calculateOffboardingBaseSalary(3100, 31, 2, 20)
	if err != nil {
		t.Fatalf("calculateOffboardingBaseSalary returned error: %v", err)
	}
	if got != 2137.93 {
		t.Fatalf("calculated salary = %.2f, want 2137.93", got)
	}
}

func TestCalculateOffboardingBaseSalaryRejectsInvalidDenominator(t *testing.T) {
	if _, err := calculateOffboardingBaseSalary(3000, 31, 31, 20); err == nil {
		t.Fatal("expected invalid denominator to be rejected")
	}
}

func TestOffboardingCommissionKeepsOrderTimeSnapshotAfterCancellation(t *testing.T) {
	createdAt := time.Date(2026, time.October, 10, 14, 0, 0, 0, time.Local)
	employeeID := uint(7)
	orders := []models.Order{{
		EmployeeID: &employeeID, OrderType: 1, OrderStatus: 2,
		CommissionAmount: 125.50, ReferrerName: "自然客户", CreatedAt: createdAt,
	}}
	personal, chamber, details := offboardingCommissionForEmployee(orders, []models.Employee{{ID: employeeID}}, employeeID)
	if personal != 12550 || chamber != 0 {
		t.Fatalf("commission cents = (%d, %d), want (12550, 0)", personal, chamber)
	}
	if len(details) != 1 || !details[0].OrderDate.Equal(createdAt) || details[0].CommissionAmount != 125.50 {
		t.Fatalf("commission detail should retain the original order snapshot: %+v", details)
	}
}

func TestCountDistinctSignedInDaysUsesOnlyCheckIns(t *testing.T) {
	one := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.Local)
	three := time.Date(2026, time.October, 3, 9, 0, 0, 0, time.Local)
	marks := []models.DailyAttendance{
		{Date: "2026-10-01", CheckIn: &one},
		{Date: "2026-10-01", CheckIn: &one},
		{Date: "2026-10-02"},
		{Date: "2026-10-03", CheckIn: &three},
	}
	if got := countDistinctSignedInDays(marks); got != 2 {
		t.Fatalf("signed-in days = %d, want 2", got)
	}
}

func TestEmployeeOffboardingSettlementUsesSignInsAndOrderTimeSnapshot(t *testing.T) {
	if config.DB == nil {
		t.Fatal("audit test database is not initialized")
	}
	monthStart := time.Date(time.Now().In(time.Local).Year(), time.Now().In(time.Local).Month(), 1, 0, 0, 0, 0, time.Local)
	lastWorkingDate := offboardingDate(time.Now()).AddDate(0, 0, -1)
	if lastWorkingDate.Before(monthStart) {
		t.Skip("the first day of the month has no previous day in the same month")
	}
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())
	entryDate := monthStart
	employee := models.Employee{
		Name: "离职结算测试" + stamp, EmpNo: "OFF" + stamp,
		BaseSalary: 3100, CommissionRate: 0.1, EntryDate: &entryDate,
		EmploymentStatus: models.EmployeeStatusActive,
	}
	if err := config.DB.Create(&employee).Error; err != nil {
		t.Fatalf("create employee fixture: %v", err)
	}
	orderEmployeeID := employee.ID
	createdAt := monthStart.Add(10 * time.Hour)
	order := models.Order{
		OrderNo: "OFF-" + stamp, CustomerName: "结算测试客户", TotalAmount: 1000,
		ActualPayAmount: 1000, EmployeeID: &orderEmployeeID, ReferrerName: "测试客户",
		CommissionAmount: 125.50, OrderStatus: 2, DeliveryMethod: 1, CreatedAt: createdAt,
	}
	if err := config.DB.Create(&order).Error; err != nil {
		config.DB.Unscoped().Delete(&employee)
		t.Fatalf("create canceled order fixture: %v", err)
	}
	checkIn := monthStart.Add(9 * time.Hour)
	attendance := []models.DailyAttendance{
		{EmployeeID: employee.ID, Date: monthStart.Format("2006-01-02"), CheckIn: &checkIn},
		{EmployeeID: employee.ID, Date: monthStart.Format("2006-01-02"), CheckIn: &checkIn},
		{EmployeeID: employee.ID, Date: monthStart.AddDate(0, 0, 2).Format("2006-01-02")},
	}
	if err := config.DB.Create(&attendance).Error; err != nil {
		t.Fatalf("create attendance fixtures: %v", err)
	}
	t.Cleanup(func() {
		config.DB.Where("employee_id = ?", employee.ID).Delete(&models.DailyAttendance{})
		config.DB.Where("employee_id = ?", employee.ID).Delete(&models.EmployeeOffboardingAlert{})
		config.DB.Where("employee_id = ?", employee.ID).Delete(&models.EmployeeHandoverTask{})
		config.DB.Where("employee_id = ?", employee.ID).Delete(&models.EmployeeOffboarding{})
		config.DB.Unscoped().Delete(&models.Order{}, order.ID)
		config.DB.Unscoped().Delete(&models.Employee{}, employee.ID)
	})

	startBody, _ := json.Marshal(CreateEmployeeOffboardingRequest{
		LastWorkingDate: lastWorkingDate.Format("2006-01-02"), HolidayDays: 2,
	})
	startContext, startRecorder := employeeOffboardingTestContext("POST", "/api/admin/employees/"+fmt.Sprint(employee.ID)+"/offboardings", startBody)
	startContext.Params = gin.Params{{Key: "id", Value: fmt.Sprint(employee.ID)}}
	CreateEmployeeOffboarding(startContext)
	if startRecorder.Code != 201 {
		t.Fatalf("start offboarding status = %d, body = %s", startRecorder.Code, startRecorder.Body.String())
	}
	var startResponse struct {
		Data struct {
			Offboarding models.EmployeeOffboarding `json:"offboarding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(startRecorder.Body.Bytes(), &startResponse); err != nil {
		t.Fatalf("decode offboarding response: %v", err)
	}
	offboardingID := startResponse.Data.Offboarding.ID
	if offboardingID == 0 {
		t.Fatal("offboarding response did not include a case ID")
	}
	if err := config.DB.Model(&models.EmployeeHandoverTask{}).Where("offboarding_id = ?", offboardingID).Updates(map[string]interface{}{
		"status": "done", "notes": "测试验收", "completed_by": "test", "completed_at": time.Now(),
	}).Error; err != nil {
		t.Fatalf("complete handover tasks: %v", err)
	}

	settlementContext, settlementRecorder := employeeOffboardingTestContext("POST", "/api/admin/employee-offboardings/"+fmt.Sprint(offboardingID)+"/settlement", nil)
	settlementContext.Params = gin.Params{{Key: "id", Value: fmt.Sprint(offboardingID)}}
	ConfirmEmployeeOffboardingSettlement(settlementContext)
	if settlementRecorder.Code != 200 {
		t.Fatalf("settlement status = %d, body = %s", settlementRecorder.Code, settlementRecorder.Body.String())
	}
	var settled models.EmployeeOffboarding
	if err := config.DB.First(&settled, offboardingID).Error; err != nil {
		t.Fatalf("read settlement snapshot: %v", err)
	}
	if settled.SignedInDays != 1 {
		t.Fatalf("signed in days = %d, want 1 distinct day", settled.SignedInDays)
	}
	calendarDays := time.Date(monthStart.Year(), monthStart.Month()+1, 0, 0, 0, 0, 0, time.Local).Day()
	wantBase := float64(moneyCents(3100/float64(calendarDays-2))) / 100
	if settled.CalculatedBaseSalary != wantBase {
		t.Fatalf("base salary snapshot = %.2f, want %.2f", settled.CalculatedBaseSalary, wantBase)
	}
	if settled.CommissionSnapshot != 125.50 || settled.FinalPayableSnapshot != wantBase+125.50 {
		t.Fatalf("payable snapshots = commission %.2f / total %.2f, want 125.50 / %.2f", settled.CommissionSnapshot, settled.FinalPayableSnapshot, wantBase+125.50)
	}
	if settled.Status != "completed" {
		t.Fatalf("settlement status = %q, want completed", settled.Status)
	}

	rows, err := monthlyEmployeePayrollReport(monthStart)
	if err != nil {
		t.Fatalf("read payroll report after settlement: %v", err)
	}
	for _, row := range rows {
		if row.EmployeeID == employee.ID {
			if row.FinalSalary != settled.FinalPayableSnapshot || len(row.CommissionDetails) != 1 || row.CommissionDetails[0].OrderNo != order.OrderNo {
				t.Fatalf("payroll row does not reflect settlement snapshot: %+v", row)
			}
			return
		}
	}
	t.Fatalf("settled employee %d was missing from payroll report", employee.ID)
}

func employeeOffboardingTestContext(method, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, path, bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("username", "offboarding-test-admin")
	return context, recorder
}
