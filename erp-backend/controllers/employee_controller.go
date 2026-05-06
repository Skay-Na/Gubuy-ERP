package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// GetEmployees 获取员工列表
func GetEmployees(c *gin.Context) {
	var employees []models.Employee
	if err := config.DB.Find(&employees).Error; err != nil {
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
	BaseSalary     float64    `json:"base_salary"`
	CommissionRate float64    `json:"commission_rate"`
	EntryDate      *time.Time `json:"entry_date"`
}

// CreateEmployee 创建员工
func CreateEmployee(c *gin.Context) {
	var req CreateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	employee := models.Employee{
		Name:           req.Name,
		Phone:          req.Phone,
		EmpNo:          req.EmpNo,
		PinCode:        req.PinCode,
		BaseSalary:     req.BaseSalary,
		CommissionRate: req.CommissionRate,
		EntryDate:      req.EntryDate,
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
	BaseSalary     float64    `json:"base_salary"`
	CommissionRate float64    `json:"commission_rate"`
	EntryDate      *time.Time `json:"entry_date"`
}

// UpdateEmployee 更新员工信息
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

	updates := map[string]interface{}{
		"name":            req.Name,
		"phone":           req.Phone,
		"emp_no":          req.EmpNo,
		"pin_code":        req.PinCode,
		"base_salary":     req.BaseSalary,
		"commission_rate": req.CommissionRate,
		"entry_date":      req.EntryDate,
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
func DeleteEmployee(c *gin.Context) {
	id := c.Param("id")
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

// UpdateAttendance 更新考勤
func UpdateAttendance(c *gin.Context) {
	var req UpdateAttendanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
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
	EmployeeID      uint    `json:"employee_id"`
	EmployeeName    string  `json:"employee_name"`
	EmpNo           string  `json:"emp_no"`
	BaseSalary      float64 `json:"base_salary"`
	CommissionRate  float64 `json:"commission_rate"`
	TotalSales      float64 `json:"total_sales"`      // 实际计算提成的销售额
	TotalCommission float64 `json:"total_commission"` // 总提成
	LeaveDays       int     `json:"leave_days"`
	Deduction       float64 `json:"deduction"`
	FinalSalary     float64 `json:"final_salary"`
}

// GetCommissionReport 获取月度提成报表
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
	startMonth, err := time.Parse("2006-01", req.Month)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "月份格式错误，应为 YYYY-MM",
		})
		return
	}
	endMonth := startMonth.AddDate(0, 1, 0)

	// 2. 查询员工信息 (包含已删除的员工，避免历史记录丢失)
	var employees []models.Employee
	if err := config.DB.Unscoped().Find(&employees).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "查询员工失败",
		})
		return
	}

	reportMap := make(map[uint]*CommissionReportItem)
	for _, emp := range employees {
		reportMap[emp.ID] = &CommissionReportItem{
			EmployeeID:     emp.ID,
			EmployeeName:   emp.Name,
			EmpNo:          emp.EmpNo,
			BaseSalary:     emp.BaseSalary,
			CommissionRate: emp.CommissionRate,
			TotalSales:     0,
			TotalCommission: 0,
		}
	}

	// 3. 查询在此期间开单的订单及明细
	//    只统计有 employee_id 的订单并且 payment_status=2 (已结全款)，或者不考虑状态计算当月开单业绩？根据常规按开新单时间或者结款时间，这里简单按下单时间计算。
	var orders []models.Order
	if err := config.DB.Preload("OrderItems").
		Where("created_at >= ? AND created_at < ? AND employee_id IS NOT NULL AND order_status = 1", startMonth, endMonth).
		Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "查询订单失败: " + err.Error(),
		})
		return
	}

	// 4. 计算业绩与提成
	for _, order := range orders {
		if order.EmployeeID == nil {
			continue
		}
		reportItem, ok := reportMap[*order.EmployeeID]
		if !ok {
			continue // 员工可能已被删除或者异常
		}

		var orderSales float64
		for _, item := range order.OrderItems {
			// 由于赠品的 UnitPrice == 0，此处累加也是 0，符合业务逻辑
			orderSales += item.UnitPrice * float64(item.Quantity)
		}
		
		reportItem.TotalSales += orderSales
		reportItem.TotalCommission += orderSales * reportItem.CommissionRate
	}

	daysInMonth := time.Date(startMonth.Year(), startMonth.Month()+1, 0, 0, 0, 0, 0, time.Local).Day()

	for _, emp := range employees {
		reportItem := reportMap[emp.ID]
		var attendance models.MonthlyAttendance
		config.DB.Where("employee_id = ? AND month = ?", emp.ID, req.Month).First(&attendance)
		
		reportItem.LeaveDays = attendance.LeaveDays
		reportItem.Deduction = 0
		if reportItem.LeaveDays > 4 {
			reportItem.Deduction = (reportItem.BaseSalary / float64(daysInMonth)) * float64(reportItem.LeaveDays - 4)
		}
		reportItem.FinalSalary = reportItem.BaseSalary + reportItem.TotalCommission - reportItem.Deduction
	}

	// 5. 整理返回数组
	result := make([]CommissionReportItem, 0, len(reportMap))
	for _, item := range reportMap {
		result = append(result, *item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "查询成功",
		"data": result,
	})
}

// VerifyPin 员工终端 PIN 码验证
func VerifyPin(c *gin.Context) {
	var req struct {
		EmployeeID uint   `json:"employee_id" binding:"required"`
		PinCode    string `json:"pin_code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var employee models.Employee
	if err := config.DB.First(&employee, req.EmployeeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "员工不存在"})
		return
	}

	if employee.PinCode != req.PinCode {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "PIN码错误"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "验证成功",
		"data": employee,
	})
}

// GetEmployeeMeStats 获取当前员工的今日业绩快照
func GetEmployeeMeStats(c *gin.Context) {
	id := c.Param("id")
	today := time.Now().Format("2006-01-02")
	
	var stats struct {
		OrderCount      int64   `json:"order_count"`
		TotalSales      float64 `json:"total_sales"`
		EstimatedComm   float64 `json:"estimated_comm"`
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
	}
	stats.EstimatedComm = stats.TotalSales * employee.CommissionRate

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": stats,
	})
}
