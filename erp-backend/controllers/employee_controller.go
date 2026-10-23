package controllers

import (
	"erp-backend/config"
	"erp-backend/middlewares"
	"erp-backend/models"
	"erp-backend/utils"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// GetEmployees 获取员工列表
// @Summary 获取员工列表
// @Description 获取员工列表
// @Tags Employees
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_employees
// @Router /api/employees [get]
func GetEmployees(c *gin.Context) {
	var employees []models.Employee
	query := config.DB.Model(&models.Employee{})
	includeDeparted := c.Query("include_departed") == "true"
	if includeDeparted && strings.HasPrefix(c.GetString("username"), "emp_") {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "员工端不能查看离职员工档案"})
		return
	}
	if !includeDeparted {
		today := time.Now().In(time.Local).Format("2006-01-02")
		query = query.Where("employment_status = '' OR employment_status = ? OR (employment_status = ? AND (last_working_date IS NULL OR last_working_date >= ?))",
			models.EmployeeStatusActive, models.EmployeeStatusOffboarding, today)
	}
	if err := query.Find(&employees).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "获取员工列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "查询成功",
		"data": employees,
	})
}

// CreateEmployeeRequest 创建员工请求
type CreateEmployeeRequest struct {
	Name           string     `json:"name" binding:"required"`
	Phone          string     `json:"phone"`
	EmpNo          string     `json:"emp_no" binding:"required"`
	PinCode        string     `json:"pin_code"`
	Role           string     `json:"role"`
	BaseSalary     float64    `json:"base_salary"`
	CommissionRate float64    `json:"commission_rate"`
	EntryDate      *time.Time `json:"entry_date"`
	IsOutsourced   bool       `json:"is_outsourced"`
	OutsourcedType string     `json:"outsourced_type"`
}

// CreateEmployee 创建员工
// @Summary 创建员工
// @Description 创建员工
// @Tags Employees
// @Accept json
// @Produce json
// @Param body body CreateEmployeeRequest true "员工参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_employees
// @Router /api/employees [post]
func CreateEmployee(c *gin.Context) {
	var req CreateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	pinyinStr, pinyinInitial := utils.GetPinyin(req.Name)

	employee := models.Employee{
		Name:           req.Name,
		Phone:          req.Phone,
		EmpNo:          req.EmpNo,
		PinCode:        req.PinCode,
		Role:           req.Role,
		BaseSalary:     req.BaseSalary,
		CommissionRate: req.CommissionRate,
		EntryDate:      req.EntryDate,
		IsOutsourced:   req.IsOutsourced,
		OutsourcedType: req.OutsourcedType,
		Pinyin:         pinyinStr,
		PinyinInitial:  pinyinInitial,
	}

	if err := config.DB.Create(&employee).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "创建员工失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "创建成功",
		"data": employee,
	})
}

// UpdateEmployeeRequest 更新员工请求
type UpdateEmployeeRequest struct {
	Name           string     `json:"name" binding:"required"`
	Phone          string     `json:"phone"`
	EmpNo          string     `json:"emp_no" binding:"required"`
	PinCode        string     `json:"pin_code"`
	Role           string     `json:"role"`
	BaseSalary     float64    `json:"base_salary"`
	CommissionRate float64    `json:"commission_rate"`
	EntryDate      *time.Time `json:"entry_date"`
	IsOutsourced   bool       `json:"is_outsourced"`
	OutsourcedType string     `json:"outsourced_type"`
}

// UpdateEmployee 更新员工信息
// @Summary 更新员工
// @Description 更新员工
// @Tags Employees
// @Accept json
// @Produce json
// @Param body body UpdateEmployeeRequest true "员工更新参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_employees_id
// @Router /api/employees/{id} [put]
func UpdateEmployee(c *gin.Context) {
	id := c.Param("id")
	var req UpdateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	var employee models.Employee
	if err := config.DB.First(&employee, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "员工不存在",
		})
		return
	}

	pinyinStr, pinyinInitial := utils.GetPinyin(req.Name)

	updates := map[string]interface{}{
		"name":            req.Name,
		"phone":           req.Phone,
		"emp_no":          req.EmpNo,
		"pin_code":        req.PinCode,
		"role":            req.Role,
		"base_salary":     req.BaseSalary,
		"commission_rate": req.CommissionRate,
		"entry_date":      req.EntryDate,
		"is_outsourced":   req.IsOutsourced,
		"outsourced_type": req.OutsourcedType,
		"pinyin":          pinyinStr,
		"pinyin_initial":  pinyinInitial,
	}

	if err := config.DB.Model(&employee).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "更新失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "更新成功",
	})
}

// DeleteEmployee 删除员工
// @Summary 删除员工
// @Description 删除员工
// @Tags Employees
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID delete_api_employees_id
// @Router /api/employees/{id} [delete]
func DeleteEmployee(c *gin.Context) {
	id := c.Param("id")
	var employee models.Employee
	if err := config.DB.First(&employee, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "员工不存在"})
		return
	}
	if employee.EmploymentStatus == models.EmployeeStatusOffboarding || employee.EmploymentStatus == models.EmployeeStatusDeparted {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "离职交接中的员工需保留档案，不能删除"})
		return
	}
	if err := config.DB.Delete(&models.Employee{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "删除失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "删除成功",
	})
}

type UpdateAttendanceRequest struct {
	EmployeeID uint   `json:"employee_id" binding:"required"`
	Month      string `json:"month" binding:"required"`
	LeaveDays  int    `json:"leave_days"`
}

type VerifyPinRequest struct {
	EmployeeID uint   `json:"employee_id" binding:"required"`
	PinCode    string `json:"pin_code" binding:"required"`
}

type EmployeeIDRequest struct {
	EmployeeID uint `json:"employee_id" binding:"required"`
}

func currentEmployeeScope(c *gin.Context) (*models.Employee, bool, error) {
	username := c.GetString("username")
	if !strings.HasPrefix(username, "emp_") {
		return nil, false, nil
	}

	empNo := strings.TrimPrefix(username, "emp_")
	var employee models.Employee
	if err := config.DB.Where("emp_no = ?", empNo).First(&employee).Error; err != nil {
		return nil, true, err
	}
	return &employee, true, nil
}

func enforceEmployeeScope(c *gin.Context, requestedEmployeeID uint) bool {
	employee, isEmployeeToken, err := currentEmployeeScope(c)
	if !isEmployeeToken {
		return true
	}
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工身份无效，请重新登录"})
		return false
	}
	if employee.ID != requestedEmployeeID {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "员工账号只能操作自己的记录"})
		return false
	}
	return true
}

func effectiveAttendanceEndDate(startMonth time.Time) time.Time {
	monthEnd := time.Date(startMonth.Year(), startMonth.Month()+1, 0, 0, 0, 0, 0, time.Local)
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if today.Before(startMonth) {
		return startMonth.AddDate(0, 0, -1)
	}
	if today.After(monthEnd) {
		return monthEnd
	}
	// 当月当天 17:00 前不提前算当天缺勤，避免员工还没到下班时间就被扣。
	if now.Hour() < 17 {
		return today.AddDate(0, 0, -1)
	}
	return today
}

func countAbsentDays(employee models.Employee, startMonth time.Time) int {
	employeeID := employee.ID
	firstDay := startMonth
	employmentStart := employee.CreatedAt
	if employee.EntryDate != nil {
		employmentStart = *employee.EntryDate
	}
	employmentStart = time.Date(employmentStart.Year(), employmentStart.Month(), employmentStart.Day(), 0, 0, 0, 0, time.Local)
	if employmentStart.After(firstDay) {
		firstDay = employmentStart
	}
	endDate := effectiveAttendanceEndDate(startMonth)
	if employee.LastWorkingDate != nil && employee.LastWorkingDate.Before(endDate) {
		endDate = time.Date(employee.LastWorkingDate.Year(), employee.LastWorkingDate.Month(), employee.LastWorkingDate.Day(), 0, 0, 0, 0, time.Local)
	}
	if employee.DeletedAt.Valid && employee.DeletedAt.Time.Before(endDate) {
		endDate = time.Date(employee.DeletedAt.Time.Year(), employee.DeletedAt.Time.Month(), employee.DeletedAt.Time.Day(), 0, 0, 0, 0, time.Local)
	}
	if endDate.Before(firstDay) {
		return 0
	}

	var logs []models.DailyAttendance
	if err := config.DB.Where("employee_id = ? AND date >= ? AND date <= ?", employeeID, firstDay.Format("2006-01-02"), endDate.Format("2006-01-02")).Find(&logs).Error; err != nil {
		return 0
	}

	checkedIn := make(map[string]bool, len(logs))
	for _, log := range logs {
		if log.CheckIn != nil {
			checkedIn[log.Date] = true
		}
	}

	absentDays := 0
	for day := firstDay; !day.After(endDate); day = day.AddDate(0, 0, 1) {
		if !checkedIn[day.Format("2006-01-02")] {
			absentDays++
		}
	}
	return absentDays
}

// UpdateAttendance 更新考勤
// @Summary 更新打卡信息
// @Description 更新打卡信息
// @Tags Employees
// @Accept json
// @Produce json
// @Param body body UpdateAttendanceRequest true "考勤参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_employees_attendance
// @Router /api/employees/attendance [post]
func UpdateAttendance(c *gin.Context) {
	var req UpdateAttendanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}
	if !enforceEmployeeScope(c, req.EmployeeID) {
		return
	}

	attendance := models.MonthlyAttendance{
		EmployeeID: req.EmployeeID,
		Month:      req.Month,
	}

	if err := config.DB.Where(models.MonthlyAttendance{EmployeeID: req.EmployeeID, Month: req.Month}).FirstOrCreate(&attendance).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "考勤记录获取失败: " + err.Error(),
		})
		return
	}

	if err := config.DB.Model(&attendance).Update("leave_days", req.LeaveDays).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "考勤更新失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "更新成功",
	})
}

// GetCommissionReportRequest
type GetCommissionReportRequest struct {
	Month string `form:"month" binding:"required"` // 格式 YYYY-MM
}

// CommissionReportItem 提成报表明细
type CommissionReportItem struct {
	EmployeeID         uint               `json:"employee_id"`
	EmployeeName       string             `json:"employee_name"`
	EmpNo              string             `json:"emp_no"`
	BaseSalary         float64            `json:"base_salary"`
	CommissionRate     float64            `json:"commission_rate"`
	TotalSales         float64            `json:"total_sales"`      // 个人订单商品销售额
	TotalCommission    float64            `json:"total_commission"` // 总提成
	PersonalCommission float64            `json:"personal_commission"`
	ChamberCommission  float64            `json:"chamber_commission"`
	CommissionDetails  []CommissionDetail `json:"commission_details"`
	ManualLeaveDays    int                `json:"manual_leave_days"`
	AbsentDays         int                `json:"absent_days"`
	LeaveDays          int                `json:"leave_days"` // 兼容旧字段：人工请假 + 未打卡请假
	FreeLeaveDays      int                `json:"free_leave_days"`
	DeductionDays      int                `json:"deduction_days"`
	DailySalary        float64            `json:"daily_salary"`
	Deduction          float64            `json:"deduction"`
	FinalSalary        float64            `json:"final_salary"`
}

type CommissionDetail struct {
	OrderID          uint      `json:"order_id"`
	OrderNo          string    `json:"order_no"`
	OrderDate        time.Time `json:"order_date"`
	CustomerName     string    `json:"customer_name"`
	ProductModels    []string  `json:"product_models"`
	OrderType        int       `json:"order_type"`
	OrderAmount      float64   `json:"order_amount"`
	CommissionPool   float64   `json:"commission_pool"`
	ParticipantCount int       `json:"participant_count"`
	CommissionAmount float64   `json:"commission_amount"`
}

func commissionProductModels(order models.Order) []string {
	productModels := make([]string, 0, len(order.OrderItems))
	seen := make(map[string]struct{}, len(order.OrderItems))
	for _, item := range order.OrderItems {
		name := ""
		if item.Product != nil {
			name = strings.TrimSpace(item.Product.Name)
		}
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		productModels = append(productModels, name)
	}
	return productModels
}

// CountsTowardEmployeePerformance 判断订单是否应计入员工销售业绩。
// 老板客户订单保留 employee_id 作为实际开单操作员，但业绩归属老板。
func CountsTowardEmployeePerformance(referrerName string) bool {
	return !strings.Contains(referrerName, "老板")
}

func moneyCents(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

func chamberCommissionCents(orderAmount float64) int64 {
	return moneyCents(orderAmount * 0.01)
}

func chamberCommissionPoolCents(order models.Order) int64 {
	poolCents := moneyCents(order.CommissionAmount)
	if poolCents > 0 {
		return poolCents
	}

	// Orders created before commission snapshots were introduced stored a zero
	// pool. A no-commission item marks a new order whose zero value is intentional.
	for _, item := range order.OrderItems {
		if item.NoCommission {
			return 0
		}
	}
	return chamberCommissionCents(order.TotalAmount)
}

func splitChamberCommission(poolCents int64, employeeIDs []uint) map[uint]int64 {
	shares := make(map[uint]int64, len(employeeIDs))
	if len(employeeIDs) == 0 || poolCents <= 0 {
		return shares
	}
	ids := append([]uint(nil), employeeIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	base := poolCents / int64(len(ids))
	remainder := poolCents % int64(len(ids))
	for index, id := range ids {
		shares[id] = base
		if int64(index) < remainder {
			shares[id]++
		}
	}
	return shares
}

func chamberEligibleEmployeeIDs(employees []models.Employee, orderDate time.Time) []uint {
	ids := make([]uint, 0, len(employees))
	for _, employee := range employees {
		if employee.IsOutsourced || employee.Role == "admin" || employee.Name == "老板" {
			continue
		}
		if !employee.WasEmployedAt(orderDate) {
			continue
		}
		ids = append(ids, employee.ID)
	}
	return ids
}

func employeeWorkedDuringMonth(employee models.Employee, startMonth, endMonth time.Time) bool {
	start := employee.CreatedAt
	if employee.EntryDate != nil {
		start = *employee.EntryDate
	}
	if !start.Before(endMonth) {
		return false
	}
	end := employee.EmploymentEndDate()
	return end == nil || !time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.Local).Before(startMonth)
}

func addOrderToCommissionReport(order models.Order, reportMap map[uint]*CommissionReportItem) {
	if order.OrderType == 2 || order.EmployeeID == nil || !CountsTowardEmployeePerformance(order.ReferrerName) {
		return
	}
	reportItem, ok := reportMap[*order.EmployeeID]
	if !ok {
		return
	}

	for _, item := range order.OrderItems {
		reportItem.TotalSales += item.UnitPrice * float64(item.Quantity)
	}
	commission := float64(moneyCents(order.CommissionAmount)) / 100
	reportItem.PersonalCommission = float64(moneyCents(reportItem.PersonalCommission)+moneyCents(commission)) / 100
	reportItem.TotalCommission = float64(moneyCents(reportItem.TotalCommission)+moneyCents(commission)) / 100
	if commission <= 0 {
		return
	}
	reportItem.CommissionDetails = append(reportItem.CommissionDetails, CommissionDetail{
		OrderID: order.ID, OrderNo: order.OrderNo, OrderDate: order.CreatedAt,
		CustomerName: order.CustomerName, ProductModels: commissionProductModels(order),
		OrderType: order.OrderType, OrderAmount: order.TotalAmount,
		CommissionPool: commission, ParticipantCount: 1, CommissionAmount: commission,
	})
}

func addChamberOrderToCommissionReport(order models.Order, employees []models.Employee, reportMap map[uint]*CommissionReportItem) {
	poolCents := chamberCommissionPoolCents(order)
	if poolCents <= 0 {
		return
	}
	eligibleIDs := chamberEligibleEmployeeIDs(employees, order.CreatedAt)
	shares := splitChamberCommission(poolCents, eligibleIDs)
	for _, employeeID := range eligibleIDs {
		reportItem, ok := reportMap[employeeID]
		if !ok {
			continue
		}
		shareCents := shares[employeeID]
		share := float64(shareCents) / 100
		reportItem.ChamberCommission = float64(moneyCents(reportItem.ChamberCommission)+shareCents) / 100
		reportItem.TotalCommission = float64(moneyCents(reportItem.TotalCommission)+shareCents) / 100
		reportItem.CommissionDetails = append(reportItem.CommissionDetails, CommissionDetail{
			OrderID: order.ID, OrderNo: order.OrderNo, OrderDate: order.CreatedAt,
			CustomerName: order.CustomerName, ProductModels: commissionProductModels(order),
			OrderType: order.OrderType, OrderAmount: order.TotalAmount,
			CommissionPool:   float64(poolCents) / 100,
			ParticipantCount: len(eligibleIDs), CommissionAmount: share,
		})
	}
}

// GetCommissionReport 获取月度提成报表
// @Summary 获取员工提成报表
// @Description 获取员工提成报表
// @Tags Employees
// @Accept json
// @Param month query string true "月份 (如 2026-06)"
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_employees_commission_report
// @Router /api/employees/commission-report [get]
func GetCommissionReport(c *gin.Context) {
	var req GetCommissionReportRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	// 1. 解析时间
	startMonth, err := time.ParseInLocation("2006-01", req.Month, time.Local)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "月份格式错误，应为 YYYY-MM",
		})
		return
	}
	result, err := monthlyEmployeePayrollReport(startMonth)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询月度提成报表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "查询成功",
		"data": result,
	})
}

// GetEmployeesSimple 获取员工脱敏列表 (无需认证，用于终端登录选择)
// @Summary GetEmployeesSimple
// @Description GetEmployeesSimple
// @Tags Employees
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @ID get_api_employees_simple
// @Router /api/employees/simple [get]
func GetEmployeesSimple(c *gin.Context) {
	var employees []models.Employee
	today := time.Now().In(time.Local).Format("2006-01-02")
	// 我们只查询 id, name, emp_no, is_outsourced, outsourced_type 这几列，排除底薪和密码
	if err := config.DB.Select("id, name, emp_no, is_outsourced, outsourced_type").
		Where("employment_status = '' OR employment_status = ? OR (employment_status = ? AND (last_working_date IS NULL OR last_working_date >= ?))",
			models.EmployeeStatusActive, models.EmployeeStatusOffboarding, today).
		Find(&employees).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "获取员工列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "查询成功",
		"data": employees,
	})
}

// VerifyPin 员工终端 PIN 码验证
// @Summary VerifyPin
// @Description VerifyPin
// @Tags Employees
// @Accept json
// @Produce json
// @Param body body VerifyPinRequest true "员工 PIN 登录参数"
// @Success 200 {object} APIResponse "成功响应"
// @ID post_api_employees_verify_pin
// @Router /api/employees/verify-pin [post]
func VerifyPin(c *gin.Context) {
	var req VerifyPinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var employee models.Employee
	if err := config.DB.First(&employee, req.EmployeeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "员工不存在"})
		return
	}
	if !employee.CanUseEmployeeAccess(time.Now()) {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "员工已离职，不能登录终端"})
		return
	}

	if employee.PinCode != req.PinCode {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "PIN码错误"})
		return
	}

	// 验证成功，为员工签发较长有效期的 JWT Token，避免移动收银端频繁掉登录。
	token, err := middlewares.GenerateTokenWithDuration("emp_"+employee.EmpNo, 30*24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "Token生成失败: " + err.Error()})
		return
	}

	// 脱敏后再返回给前端
	employee.PinCode = ""

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "验证成功",
		"data": gin.H{
			"token":    token,
			"employee": employee,
		},
	})
}

// GetEmployeeMeStats 获取当前员工的今日业绩快照
// @Summary 获取员工个人统计
// @Description 获取员工个人统计
// @Tags Employees
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID get_api_employees_id_stats
// @Router /api/employees/{id}/stats [get]
func GetEmployeeMeStats(c *gin.Context) {
	id := c.Param("id")
	if parsedID, err := strconv.ParseUint(id, 10, 64); err == nil {
		if !enforceEmployeeScope(c, uint(parsedID)) {
			return
		}
	}
	today := time.Now().Format("2006-01-02")

	var stats struct {
		OrderCount    int64   `json:"order_count"`
		TotalSales    float64 `json:"total_sales"`
		EstimatedComm float64 `json:"estimated_comm"`
	}

	// 1. 获取员工信息（为了提成比例）
	var employee models.Employee
	if err := config.DB.First(&employee, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "员工不存在"})
		return
	}

	// 2. 统计今日正常订单
	var orders []models.Order
	config.DB.Preload("OrderItems").
		Where("employee_id = ? AND order_status = 1 AND DATE(created_at) = ?", id, today).
		Find(&orders)

	stats.OrderCount = int64(len(orders))
	for _, order := range orders {
		var orderAmt float64
		for _, item := range order.OrderItems {
			orderAmt += item.UnitPrice * float64(item.Quantity)
		}
		stats.TotalSales += orderAmt
		if order.OrderType != 2 && CountsTowardEmployeePerformance(order.ReferrerName) {
			stats.EstimatedComm += order.CommissionAmount
		}
	}

	var chamberOrders []models.Order
	if err := config.DB.Preload("OrderItems").Where("order_type = 2 AND order_status = 1 AND DATE(created_at) = ?", today).
		Find(&chamberOrders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询商会订单失败"})
		return
	}
	if len(chamberOrders) > 0 {
		var team []models.Employee
		if err := config.DB.Unscoped().Where("is_outsourced = ?", false).Find(&team).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询员工失败"})
			return
		}
		for _, order := range chamberOrders {
			eligibleIDs := chamberEligibleEmployeeIDs(team, order.CreatedAt)
			shares := splitChamberCommission(chamberCommissionPoolCents(order), eligibleIDs)
			stats.EstimatedComm += float64(shares[employee.ID]) / 100
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": stats,
	})
}

// CheckIn 员工上班打卡
// @Summary 签到
// @Description 签到
// @Tags Employees
// @Accept json
// @Produce json
// @Param body body EmployeeIDRequest true "员工ID"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_employees_check_in
// @Router /api/employees/check-in [post]
func CheckIn(c *gin.Context) {
	var req EmployeeIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if !enforceEmployeeScope(c, req.EmployeeID) {
		return
	}

	today := time.Now().Format("2006-01-02")
	now := time.Now()

	var attendance models.DailyAttendance
	err := config.DB.Where("employee_id = ? AND date = ?", req.EmployeeID, today).First(&attendance).Error

	if err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "今日已打卡上班"})
		return
	}

	attendance = models.DailyAttendance{
		EmployeeID: req.EmployeeID,
		Date:       today,
		CheckIn:    &now,
		Status:     1, // 默认正常，可根据规定时间判断迟到
	}

	if err := config.DB.Create(&attendance).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "打卡失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "上班打卡成功", "data": attendance})
}

// CheckOut 员工下班打卡
// @Summary 签退
// @Description 签退
// @Tags Employees
// @Accept json
// @Produce json
// @Param body body EmployeeIDRequest true "员工ID"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_employees_check_out
// @Router /api/employees/check-out [post]
func CheckOut(c *gin.Context) {
	var req EmployeeIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if !enforceEmployeeScope(c, req.EmployeeID) {
		return
	}

	today := time.Now().Format("2006-01-02")
	now := time.Now()

	var attendance models.DailyAttendance
	if err := config.DB.Where("employee_id = ? AND date = ?", req.EmployeeID, today).First(&attendance).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "未找到今日上班打卡记录"})
		return
	}

	if attendance.CheckOut != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "今日已打卡下班"})
		return
	}

	if err := config.DB.Model(&attendance).Update("check_out", &now).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "打卡失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "下班打卡成功", "data": attendance})
}

// GetDailyAttendance 获取员工打卡明细
// @Summary 获取打卡记录
// @Description 获取打卡记录
// @Tags Employees
// @Accept json
// @Param employee_id query int false "员工ID (默认当前登录员工)"
// @Param month query string false "月份 (如 2026-06)"
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_employees_attendance_logs
// @Router /api/employees/attendance-logs [get]
func GetDailyAttendance(c *gin.Context) {
	employeeID := c.Query("employee_id")
	month := c.Query("month") // YYYY-MM
	if employeeID != "" {
		if parsedID, err := strconv.ParseUint(employeeID, 10, 64); err == nil {
			if !enforceEmployeeScope(c, uint(parsedID)) {
				return
			}
		}
	}

	var logs []models.DailyAttendance
	query := config.DB.Order("date desc")

	if employeeID != "" {
		query = query.Where("employee_id = ?", employeeID)
	}
	if month != "" {
		query = query.Where("date LIKE ?", month+"%")
	}

	if err := query.Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": logs})
}

// ExportEmployees 导出所有员工
// @Summary 导出员工数据
// @Description 导出员工数据
// @Tags Employees
// @Produce application/octet-stream
// @Success 200 {file} binary "Excel 文件"
// @Security ApiKeyAuth
// @ID get_api_employees_export
// @Router /api/employees/export [get]
func ExportEmployees(c *gin.Context) {
	var employees []models.Employee
	if err := config.DB.Find(&employees).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询员工失败"})
		return
	}

	headers := []string{"员工ID", "工号", "姓名", "电话", "基本底薪", "提成比例", "是否外包", "入职日期", "任职状态", "最后工作日"}
	var data [][]interface{}
	for _, e := range employees {
		isOut := "否"
		if e.IsOutsourced {
			isOut = "是"
		}
		entry := "-"
		if e.EntryDate != nil {
			entry = e.EntryDate.Format("2006-01-02")
		}
		status := e.EmploymentStatus
		if status == "" {
			status = models.EmployeeStatusActive
		}
		statusLabel := map[string]string{
			models.EmployeeStatusActive:      "在职",
			models.EmployeeStatusOffboarding: "交接中",
			models.EmployeeStatusDeparted:    "已离职",
		}[status]
		if statusLabel == "" {
			statusLabel = status
		}
		lastWorkingDate := "-"
		if e.LastWorkingDate != nil {
			lastWorkingDate = e.LastWorkingDate.Format("2006-01-02")
		}
		row := []interface{}{
			e.ID,
			e.EmpNo,
			e.Name,
			e.Phone,
			e.BaseSalary,
			e.CommissionRate,
			isOut,
			entry,
			statusLabel,
			lastWorkingDate,
		}
		data = append(data, row)
	}

	utils.ExportToExcel(c, "员工清单", headers, data)
}
