package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
)

var (
	payrollHistoryRouterOnce sync.Once
	payrollHistoryTestRouter *gin.Engine
)

func setupPayrollHistoryTest(t *testing.T) {
	t.Helper()
	setupAudit(t)
	payrollHistoryRouterOnce.Do(func() {
		gin.SetMode(gin.TestMode)
		payrollHistoryTestRouter = gin.New()
		payrollHistoryTestRouter.GET("/api/employees/:id/payroll-history", GetPayrollHistory)
		payrollHistoryTestRouter.POST("/api/payroll/confirm", ConfirmPayroll)
	})
}

func getPayrollHistory(t *testing.T, employeeID uint, year int) map[string]interface{} {
	t.Helper()
	path := fmt.Sprintf("/api/employees/%d/payroll-history?year=%d", employeeID, year)
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	payrollHistoryTestRouter.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("工资条接口返回 %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Employee map[string]interface{} `json:"employee"`
			Year     int                    `json:"year"`
			Months   []PayrollHistoryMonth  `json:"months"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Code != 200 {
		t.Fatalf("业务码非 200: %d", resp.Code)
	}
	return map[string]interface{}{"months": resp.Data.Months}
}

func findHistoryMonth(t *testing.T, payload map[string]interface{}, month string) PayrollHistoryMonth {
	t.Helper()
	months := payload["months"].([]PayrollHistoryMonth)
	for _, record := range months {
		if record.Month == month {
			return record
		}
	}
	t.Fatalf("工资条缺少月份 %s，实际月份: %v", month, func() (out []string) {
		for _, record := range months {
			out = append(out, record.Month)
		}
		return
	}())
	return PayrollHistoryMonth{}
}

func TestPayrollHistory_ShowsPayablePaidDifference(t *testing.T) {
	setupPayrollHistoryTest(t)
	ledgerID := ensureOperatingLedgerAccount(t)

	suffix := time.Now().UnixNano() % 10000000
	entryDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	employee := models.Employee{
		Name:             fmt.Sprintf("工资条员工%d", suffix),
		EmpNo:            fmt.Sprintf("PH%d", suffix),
		BaseSalary:       2000,
		Role:             "sales",
		CommissionRate:   0.01,
		EmploymentStatus: models.EmployeeStatusActive,
		EntryDate:        &entryDate,
	}
	if err := config.DB.Create(&employee).Error; err != nil {
		t.Fatalf("创建员工失败: %v", err)
	}

	// 2026-08 个人订单，提成 100 → 8月应发 2100
	employeeID := employee.ID
	order := models.Order{
		OrderNo:          fmt.Sprintf("PHTEST%012d", suffix),
		CustomerName:     "工资条客户",
		TotalAmount:      5000,
		CommissionAmount: 100,
		OrderStatus:      1,
		OrderType:        1,
		PaymentStatus:    2,
		PaymentMethod:    1,
		DeliveryMethod:   2,
		EmployeeID:       &employeeID,
		CreatedAt:        time.Date(2026, 8, 15, 10, 0, 0, 0, time.Local),
		UpdatedAt:        time.Date(2026, 8, 15, 10, 0, 0, 0, time.Local),
	}
	if err := config.DB.Create(&order).Error; err != nil {
		t.Fatalf("创建订单失败: %v", err)
	}

	// 月度考勤：请假 0 天，避免无打卡被按全月缺勤扣款
	attendance := models.MonthlyAttendance{EmployeeID: employee.ID, Month: "2026-08", LeaveDays: 0}
	if err := config.DB.Create(&attendance).Error; err != nil {
		t.Fatalf("创建考勤记录失败: %v", err)
	}

	// 手工工资流水：2026-07 薪资 2018，实付日 2026-08-01（备注带薪资月标记）
	wageLog := models.FinancialLog{
		AccountID:    ledgerID,
		FlowType:     2,
		Category:     "工资",
		Amount:       2018,
		BalanceAfter: -2018,
		RelatedNo:    fmt.Sprintf("EXPPH%010d", suffix),
		Remark:       "2026-07工资：底薪2000+商会分成18.32=2018.32抹零2018",
		Counterparty: employee.Name,
		SourceType:   "manual",
		CreatedAt:    time.Date(2026, 8, 1, 12, 0, 0, 0, time.Local),
	}
	if err := config.DB.Create(&wageLog).Error; err != nil {
		t.Fatalf("创建工资流水失败: %v", err)
	}

	// 未确认阶段：7月实付来自台账；8月应发2100未确认
	first := getPayrollHistory(t, employee.ID, 2026)
	july := findHistoryMonth(t, first, "2026-07")
	if july.Paid != 2018 || july.PaidSource != "ledger" || july.Status != "paid" {
		t.Fatalf("7月实付归属错误: paid=%.2f source=%s status=%s", july.Paid, july.PaidSource, july.Status)
	}
	if len(july.PaidEntries) != 1 || july.PaidEntries[0].Amount != 2018 {
		t.Fatalf("7月实付构成错误: %+v", july.PaidEntries)
	}
	august := findHistoryMonth(t, first, "2026-08")
	if august.Payable != 2100 {
		t.Fatalf("8月应发应为 2100（底薪2000+提成100），实际 %.2f", august.Payable)
	}
	if august.Status != "unpaid" || august.Paid != 0 {
		t.Fatalf("8月未确认状态错误: status=%s paid=%.2f", august.Status, august.Paid)
	}
	if len(august.CommissionDetails) != 1 || august.CommissionDetails[0].CommissionAmount != 100 {
		t.Fatalf("8月提成明细错误: %+v", august.CommissionDetails)
	}

	// 确认 8 月发放 → 定稿快照 + 实付 2100 + 差额 0
	confirmBody := gin.H{"month": "2026-08", "items": []gin.H{
		{"employee_id": employee.ID, "amount": 2100},
	}}
	jsonValue, _ := json.Marshal(confirmBody)
	req, _ := http.NewRequest("POST", "/api/payroll/confirm", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	payrollHistoryTestRouter.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("确认发放失败 %d: %s", w.Code, w.Body.String())
	}

	second := getPayrollHistory(t, employee.ID, 2026)
	augustConfirmed := findHistoryMonth(t, second, "2026-08")
	if augustConfirmed.Status != "confirmed" || !augustConfirmed.Finalized {
		t.Fatalf("8月确认状态错误: status=%s finalized=%v", augustConfirmed.Status, augustConfirmed.Finalized)
	}
	if augustConfirmed.Paid != 2100 || augustConfirmed.Difference != 0 {
		t.Fatalf("8月确认后实付/差额错误: paid=%.2f diff=%.2f", augustConfirmed.Paid, augustConfirmed.Difference)
	}
	if len(augustConfirmed.Snapshot) == 0 {
		t.Fatalf("确认发放应写入定稿快照")
	}
	// 确认发放的 PAY- 流水应归属 2026-08 而非重复计入 7 月
	if julyAfter := findHistoryMonth(t, second, "2026-07"); julyAfter.Paid != 2018 {
		t.Fatalf("确认 8 月后 7 月实付不应变化: %.2f", julyAfter.Paid)
	}
}
