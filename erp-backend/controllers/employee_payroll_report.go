package controllers

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"erp-backend/config"
	"erp-backend/models"
)

// chamberCommissionStart 商会订单分成起算点：2026-10 起的商会订单才计入提成
// （用户 2026-10-26 定稿：之前的商会订单不再计算）。
var chamberCommissionStart = time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)

// monthlyEmployeePayrollReport is the shared calculation behind the admin report
// and Agent's accrued salary answer. It does not represent paid cash outflows.
// 可选参数 includeDeparted：传 true 时用含离职的全量历史员工建档（工资条历史月视图用），
// 默认仅在职员工（薪资结算抽屉口径不变）。
func monthlyEmployeePayrollReport(startMonth time.Time, includeDeparted ...bool) ([]CommissionReportItem, error) {
	withDeparted := len(includeDeparted) > 0 && includeDeparted[0]
	endMonth := startMonth.AddDate(0, 1, 0)
	month := startMonth.Format("2006-01")
	var employees []models.Employee
	if err := config.DB.Where("is_outsourced = ?", false).Find(&employees).Error; err != nil {
		return nil, fmt.Errorf("查询员工: %w", err)
	}
	var historicalEmployees []models.Employee
	if err := config.DB.Unscoped().Where("is_outsourced = ?", false).Find(&historicalEmployees).Error; err != nil {
		return nil, fmt.Errorf("查询历史员工: %w", err)
	}
	roster := employees
	if withDeparted {
		roster = historicalEmployees
	}
	reportMap := make(map[uint]*CommissionReportItem, len(employees))
	for _, employee := range roster {
		if !employeeWorkedDuringMonth(employee, startMonth, endMonth) {
			continue
		}
		reportMap[employee.ID] = &CommissionReportItem{
			EmployeeID: employee.ID, EmployeeName: employee.Name, EmpNo: employee.EmpNo,
			BaseSalary: employee.BaseSalary, CommissionRate: employee.CommissionRate,
			CommissionDetails: make([]CommissionDetail, 0),
		}
	}
	var orders []models.Order
	if err := config.DB.Preload("OrderItems.Product").
		Where("created_at >= ? AND created_at < ? AND order_status = 1", startMonth, endMonth).
		Order("created_at asc, id asc").Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("查询订单: %w", err)
	}
	for _, order := range orders {
		if order.OrderType == 2 {
			if order.CreatedAt.Before(chamberCommissionStart) {
				continue // 2026-10 之前的商会订单不计分成
			}
			addChamberOrderToCommissionReport(order, historicalEmployees, reportMap)
			continue
		}
		addOrderToCommissionReport(order, reportMap)
	}
	daysInMonth := time.Date(startMonth.Year(), startMonth.Month()+1, 0, 0, 0, 0, 0, time.Local).Day()
	for _, employee := range roster {
		item, ok := reportMap[employee.ID]
		if !ok {
			continue
		}
		var attendance models.MonthlyAttendance
		config.DB.Where("employee_id = ? AND month = ?", employee.ID, month).Find(&attendance)
		item.AbsentDays = countAbsentDays(employee, startMonth)
		item.ManualLeaveDays = attendance.LeaveDays
		if attendance.ID == 0 {
			item.ManualLeaveDays = item.AbsentDays
		}
		item.LeaveDays = item.ManualLeaveDays
		item.FreeLeaveDays = 4
		item.DailySalary = item.BaseSalary / float64(daysInMonth)
		if item.LeaveDays > item.FreeLeaveDays {
			item.DeductionDays = item.LeaveDays - item.FreeLeaveDays
			item.Deduction = float64(moneyCents(item.DailySalary*float64(item.DeductionDays))) / 100
		}
		// 应发小数抹零（用户 2026-10-26 定稿）：薪资只发整数
		item.FinalSalary = math.Floor(float64(moneyCents(item.BaseSalary)+moneyCents(item.TotalCommission)-moneyCents(item.Deduction)) / 100)
	}
	var completedOffboardings []models.EmployeeOffboarding
	if err := config.DB.Where("settlement_month = ? AND status = ?", month, "completed").Find(&completedOffboardings).Error; err != nil {
		return nil, fmt.Errorf("查询离职结算快照: %w", err)
	}
	for _, settlement := range completedOffboardings {
		item, ok := reportMap[settlement.EmployeeID]
		if !ok {
			continue
		}
		item.BaseSalary = settlement.CalculatedBaseSalary
		item.PersonalCommission = settlement.PersonalCommissionSnapshot
		item.ChamberCommission = settlement.ChamberCommissionSnapshot
		item.TotalCommission = settlement.CommissionSnapshot
		item.Deduction = 0
		item.DeductionDays = 0
		item.FinalSalary = settlement.FinalPayableSnapshot
		var detailSnapshot []CommissionDetail
		if err := json.Unmarshal([]byte(settlement.CommissionDetailsSnapshot), &detailSnapshot); err == nil {
			item.CommissionDetails = detailSnapshot
		}
	}
	result := make([]CommissionReportItem, 0, len(reportMap))
	for _, item := range reportMap {
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].TotalSales == result[j].TotalSales {
			return result[i].EmployeeID < result[j].EmployeeID
		}
		return result[i].TotalSales > result[j].TotalSales
	})
	return result, nil
}
