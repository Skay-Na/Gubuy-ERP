package controllers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
)

// PayrollPaidEntry 工资条"实付"的构成明细：一笔工资流水的溯源信息。
type PayrollPaidEntry struct {
	FinancialLogID uint      `json:"financial_log_id"`
	RelatedNo      string    `json:"related_no"`
	Remark         string    `json:"remark"`
	Amount         float64   `json:"amount"`
	PaidAt         time.Time `json:"paid_at"`
}

// PayrollHistoryMonth 工资条单月结构。
// 应发(Payable)=按当前订单数据实时回算（与薪资结算抽屉同口径）；
// 实付(Paid)=确认发放台账为准，历史手工月按"工资"流水归属汇总；
// 差额(Difference)=应发-实付，用于暴露"提成后补、工资先发"类历史差异。
type PayrollHistoryMonth struct {
	Month              string             `json:"month"`
	BaseSalary         float64            `json:"base_salary"`
	PersonalCommission float64            `json:"personal_commission"`
	ChamberCommission  float64            `json:"chamber_commission"`
	TotalCommission    float64            `json:"total_commission"`
	LeaveDays          int                `json:"leave_days"`
	FreeLeaveDays      int                `json:"free_leave_days"`
	DeductionDays      int                `json:"deduction_days"`
	Deduction          float64            `json:"deduction"`
	DailySalary        float64            `json:"daily_salary"`
	Payable            float64            `json:"payable"`
	Paid               float64            `json:"paid"`
	PaidSource         string             `json:"paid_source"` // confirmation/ledger/none
	PaidEntries        []PayrollPaidEntry `json:"paid_entries"`
	Difference         float64            `json:"difference"`
	Status             string             `json:"status"` // confirmed/paid/unpaid
	Finalized          bool               `json:"finalized"`
	AttendanceRecorded bool               `json:"attendance_recorded"` // 当月是否有人工考勤记录，无记录时不计缺勤扣款
	Snapshot           json.RawMessage    `json:"snapshot,omitempty"`
	CommissionDetails  []CommissionDetail `json:"commission_details"`
}

// payrollMonthMarker 匹配备注/单号中的薪资月标记："2026-08工资"、"2026-07薪资"、"2026-08提成"。
// 手工补录的工资流水只要在备注里写清"<薪资月>工资/薪资/提成"，就能被精确归属。
var payrollMonthMarker = regexp.MustCompile(`20\d{2}-\d{2}(?:工资|薪资|提成)`)

// payrollSalaryMonth 归属一笔"工资"流水的薪资所属月：
//  1. related_no 前缀 PAY-YYYY-MM-*（确认发放台账写入）→ 该月
//  2. 备注中首个"YYYY-MM工资/薪资/提成"标记 → 该月
//  3. 兜底：实付月的上一个月（店内惯例：每月月初发放上月工资）
func payrollSalaryMonth(log models.FinancialLog) string {
	if relatedNo := strings.TrimSpace(log.RelatedNo); strings.HasPrefix(relatedNo, "PAY-") && len(relatedNo) >= len("PAY-2006-01") {
		if candidate := relatedNo[len("PAY-"):][:7]; validPayrollMonth(candidate) {
			return candidate
		}
	}
	if marker := payrollMonthMarker.FindString(log.Remark); len(marker) >= 7 {
		return marker[:7]
	}
	return log.CreatedAt.AddDate(0, -1, 0).Format("2006-01")
}

func validPayrollMonth(candidate string) bool {
	_, err := time.ParseInLocation("2006-01", candidate, time.Now().Location())
	return err == nil
}

// resolveOperatingLedgerAccountID 只读定位经营开销台账账户：优先固定 ID=7，按名称兜底。
func resolveOperatingLedgerAccountID() (uint, error) {
	var account models.Account
	if err := config.DB.First(&account, operatingExpenseAccountID).Error; err == nil {
		return account.ID, nil
	}
	if err := config.DB.Where("name = ?", operatingExpenseAccountName).First(&account).Error; err != nil {
		return 0, fmt.Errorf("经营开销账户(id=%d 或名称「%s」)不存在", operatingExpenseAccountID, operatingExpenseAccountName)
	}
	return account.ID, nil
}

// GetPayrollHistory 员工工资条（按年逐月）
// @Summary 员工工资条历史
// @Description 返回该员工指定年份逐月工资条：应发（实时回算，含逐笔提成订单明细）、实付（确认发放台账/工资流水归属，含构成明细）、差额与状态；确认发放过的月份附带定稿快照。
// @Tags Payroll
// @Produce json
// @Param id path int true "员工ID"
// @Param year query int false "年份，默认当前年"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_employees_id_payroll_history
// @Router /api/employees/{id}/payroll-history [get]
func GetPayrollHistory(c *gin.Context) {
	employeeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || employeeID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "员工ID错误"})
		return
	}
	year := time.Now().Year()
	if raw := strings.TrimSpace(c.Query("year")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err != nil || parsed < 2000 || parsed > 2100 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "年份格式错误"})
			return
		} else {
			year = parsed
		}
	}

	var employee models.Employee
	if err := config.DB.Unscoped().First(&employee, employeeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "员工不存在"})
		return
	}
	if employee.IsOutsourced {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "外包人员按次结算，不生成工资条"})
		return
	}

	ledgerAccountID, err := resolveOperatingLedgerAccountID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}

	// 确认发放台账（跨全量月份，键=薪资月）
	var confirmations []models.PayrollConfirmation
	if err := config.DB.Where("employee_id = ?", employee.ID).
		Where("month >= ? AND month <= ?", fmt.Sprintf("%d-01", year), fmt.Sprintf("%d-12", year)).
		Find(&confirmations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询工资确认记录失败"})
		return
	}
	confirmationByMonth := make(map[string]models.PayrollConfirmation, len(confirmations))
	for _, record := range confirmations {
		confirmationByMonth[record.Month] = record
	}

	// 月度考勤记录：没有人工考勤的月份（多为历史月）不能把“没打卡”当全月缺勤扣款
	var attendanceMonths []string
	if err := config.DB.Model(&models.MonthlyAttendance{}).
		Where("employee_id = ?", employee.ID).
		Where("month >= ? AND month <= ?", fmt.Sprintf("%d-01", year), fmt.Sprintf("%d-12", year)).
		Pluck("month", &attendanceMonths).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询月度考勤失败"})
		return
	}
	attendanceRecorded := make(map[string]bool, len(attendanceMonths))
	for _, month := range attendanceMonths {
		attendanceRecorded[month] = true
	}

	// 工资流水窗口：薪资年 1 月起至次年 3 月底（12 月工资最晚 1~2 月发放也兜住）
	windowStart := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
	windowEnd := time.Date(year+1, 3, 1, 0, 0, 0, 0, time.Local)
	var wageLogs []models.FinancialLog
	if err := config.DB.
		Where("account_id = ? AND flow_type = 2 AND category = ? AND (counterparty = ? OR remark LIKE ?) AND created_at >= ? AND created_at < ?",
			ledgerAccountID, "工资", employee.Name, "%"+employee.Name+"%", windowStart, windowEnd).
		Order("created_at asc, id asc").Find(&wageLogs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询工资流水失败"})
		return
	}
	paidEntriesByMonth := make(map[string][]PayrollPaidEntry)
	for _, log := range wageLogs {
		month := payrollSalaryMonth(log)
		if !strings.HasPrefix(month, strconv.Itoa(year)+"-") {
			continue
		}
		paidEntriesByMonth[month] = append(paidEntriesByMonth[month], PayrollPaidEntry{
			FinancialLogID: log.ID, RelatedNo: log.RelatedNo, Remark: log.Remark,
			Amount: log.Amount, PaidAt: log.CreatedAt,
		})
	}

	months := make([]PayrollHistoryMonth, 0, 12)
	currentMonth := time.Now().Format("2006-01")
	for m := 1; m <= 12; m++ {
		monthStart := time.Date(year, time.Month(m), 1, 0, 0, 0, 0, time.Local)
		monthKey := monthStart.Format("2006-01")

		item, hasReport := findCommissionReportItem(monthStart, employee.ID)
		confirmation, hasConfirmation := confirmationByMonth[monthKey]
		entries := paidEntriesByMonth[monthKey]
		if !hasReport && !hasConfirmation && len(entries) == 0 {
			continue
		}

		if entries == nil {
			entries = []PayrollPaidEntry{}
		}
		record := PayrollHistoryMonth{
			Month:             monthKey,
			PaidEntries:       entries,
			PaidSource:        "none",
			Status:            "unpaid",
			CommissionDetails: make([]CommissionDetail, 0),
		}
		if hasReport {
			record.BaseSalary = item.BaseSalary
			record.PersonalCommission = item.PersonalCommission
			record.ChamberCommission = item.ChamberCommission
			record.TotalCommission = item.TotalCommission
			record.LeaveDays = item.LeaveDays
			record.FreeLeaveDays = item.FreeLeaveDays
			record.DeductionDays = item.DeductionDays
			record.Deduction = item.Deduction
			record.DailySalary = item.DailySalary
			record.Payable = item.FinalSalary
			record.CommissionDetails = item.CommissionDetails
			record.AttendanceRecorded = attendanceRecorded[monthKey]
			if !record.AttendanceRecorded {
				// 无人工考勤记录：不计“没打卡=缺勤”的推定扣款，应发=底薪+提成
				record.LeaveDays = 0
				record.DeductionDays = 0
				record.Deduction = 0
				record.Payable = math.Floor(record.BaseSalary + record.TotalCommission)
			}
		}
		if hasConfirmation {
			record.Paid = confirmation.Amount
			record.PaidSource = "confirmation"
			record.Status = "confirmed"
			if strings.TrimSpace(confirmation.Snapshot) != "" {
				var snapshot json.RawMessage
				if json.Unmarshal([]byte(confirmation.Snapshot), &snapshot) == nil {
					record.Snapshot = snapshot
					record.Finalized = true
				}
			}
		} else if len(entries) > 0 {
			total := 0.0
			for _, entry := range entries {
				total += entry.Amount
			}
			record.Paid = float64(moneyCents(total)) / 100
			record.PaidSource = "ledger"
			record.Status = "paid"
		}
		record.Difference = float64(moneyCents(record.Payable-record.Paid)) / 100
		// 未到发薪（薪资月 >= 当前月：M 月工资在 M+1 月初发）→ 不算"未发"，差额不显示
		if record.Status == "unpaid" && monthKey >= currentMonth {
			record.Status = "upcoming"
			record.Difference = 0
		}
		months = append(months, record)
	}
	sort.Slice(months, func(i, j int) bool { return months[i].Month > months[j].Month })

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{
		"employee": gin.H{
			"id": employee.ID, "name": employee.Name, "emp_no": employee.EmpNo,
			"base_salary": employee.BaseSalary, "commission_rate": employee.CommissionRate,
			"employment_status": employee.EmploymentStatus, "entry_date": employee.EntryDate,
			"last_working_date": employee.LastWorkingDate,
		},
		"year":   year,
		"months": months,
	}})
}

// findCommissionReportItem 复用月度薪资报表口径，取指定员工当月的报表项（含离职员工的历史月份）。
func findCommissionReportItem(monthStart time.Time, employeeID uint) (CommissionReportItem, bool) {
	items, err := monthlyEmployeePayrollReport(monthStart, true)
	if err != nil {
		return CommissionReportItem{}, false
	}
	for _, item := range items {
		if item.EmployeeID == employeeID {
			return item, true
		}
	}
	return CommissionReportItem{}, false
}

// payrollSnapshotForMonth 生成指定员工当月的薪资结构快照 JSON（确认发放时定稿用）。
// 报表计算失败不阻断发放，返回空串（工资条展示实时估算）。
func payrollSnapshotForMonth(month string, employeeID uint) string {
	monthStart, err := time.ParseInLocation("2006-01", month, time.Now().Location())
	if err != nil {
		return ""
	}
	items, err := monthlyEmployeePayrollReport(monthStart, true)
	if err != nil {
		return ""
	}
	for _, item := range items {
		if item.EmployeeID != employeeID {
			continue
		}
		if payload, err := json.Marshal(item); err == nil {
			return string(payload)
		}
	}
	return ""
}
