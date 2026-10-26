package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errInvalidOffboardingPayroll = errors.New("invalid offboarding payroll inputs")

type CreateEmployeeOffboardingRequest struct {
	LastWorkingDate     string `json:"last_working_date" binding:"required"`
	HolidayDays         int    `json:"holiday_days" binding:"gte=0"`
	SuccessorEmployeeID *uint  `json:"successor_employee_id"`
	Notes               string `json:"notes"`
}

type UpdateEmployeeHandoverTaskRequest struct {
	Done  *bool  `json:"done" binding:"required"`
	Notes string `json:"notes"`
}

type ResolveEmployeeOffboardingAlertRequest struct {
	Resolution string `json:"resolution" binding:"required"`
}

type offboardingChecklistItem struct {
	key   string
	type_ string
	title string
}

var defaultOffboardingChecklist = []offboardingChecklistItem{
	{key: "inventory", type_: "inventory", title: "核对门店、云仓和样机库存"},
	{key: "cash", type_: "cash", title: "核对经手收款、报销和待办账目"},
	{key: "assets", type_: "assets", title: "归还设备、钥匙和业务资料"},
	{key: "access", type_: "access", title: "移交系统及第三方业务账号"},
	{key: "customers", type_: "customers", title: "移交客户关系和未结事项"},
}

func offboardingDate(value time.Time) time.Time {
	return time.Date(value.In(time.Local).Year(), value.In(time.Local).Month(), value.In(time.Local).Day(), 0, 0, 0, 0, time.Local)
}

func validateActiveEmployeeAssignment(tx *gorm.DB, employeeID uint, at time.Time) error {
	if employeeID == 0 {
		return errors.New("请选择有效员工")
	}
	var employee models.Employee
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&employee, employeeID).Error; err != nil {
		return fmt.Errorf("员工不存在: %w", err)
	}
	if !employee.CanUseEmployeeAccess(at) {
		return errors.New("该员工已离职或超过最后工作日，不能继续指派")
	}
	return nil
}

func calculateOffboardingBaseSalary(baseSalary float64, calendarDays, holidayDays, signedInDays int) (float64, error) {
	workingDays := calendarDays - holidayDays
	if baseSalary < 0 || calendarDays <= 0 || holidayDays < 0 || workingDays <= 0 || signedInDays < 0 || signedInDays > calendarDays {
		return 0, errInvalidOffboardingPayroll
	}
	return float64(moneyCents(baseSalary*float64(signedInDays)/float64(workingDays))) / 100, nil
}

func countSignedInDays(tx *gorm.DB, employee models.Employee, monthStart, lastWorkingDate time.Time) (int, error) {
	firstDay := offboardingDate(monthStart)
	employeeStart := employee.CreatedAt
	if employee.EntryDate != nil {
		employeeStart = *employee.EntryDate
	}
	employeeStart = offboardingDate(employeeStart)
	if employeeStart.After(firstDay) {
		firstDay = employeeStart
	}
	lastDay := offboardingDate(lastWorkingDate)
	if lastDay.Before(firstDay) {
		return 0, nil
	}
	var attendance []models.DailyAttendance
	if err := tx.Where("employee_id = ? AND date >= ? AND date <= ? AND check_in IS NOT NULL",
		employee.ID, firstDay.Format("2006-01-02"), lastDay.Format("2006-01-02")).Find(&attendance).Error; err != nil {
		return 0, fmt.Errorf("query signed attendance: %w", err)
	}
	return countDistinctSignedInDays(attendance), nil
}

func countDistinctSignedInDays(attendance []models.DailyAttendance) int {
	days := make(map[string]struct{}, len(attendance))
	for _, mark := range attendance {
		if mark.CheckIn != nil {
			days[mark.Date] = struct{}{}
		}
	}
	return len(days)
}

// CreateEmployeeOffboarding 发起员工离职交接
// @Summary 发起员工离职交接
// @Description 发起员工离职交接
// @Tags EmployeeOffboarding
// @Param id path int true "id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_employees_id_offboardings
// @Router /api/employees/{id}/offboardings [post]
func CreateEmployeeOffboarding(c *gin.Context) {
	employeeID := c.Param("id")
	var req CreateEmployeeOffboardingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请填写最后工作日和有效假期天数"})
		return
	}
	lastWorkingDate, err := time.ParseInLocation("2006-01-02", req.LastWorkingDate, time.Local)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "最后工作日格式应为 YYYY-MM-DD"})
		return
	}
	lastWorkingDate = offboardingDate(lastWorkingDate)
	calendarDays := time.Date(lastWorkingDate.Year(), lastWorkingDate.Month()+1, 0, 0, 0, 0, 0, time.Local).Day()
	if req.HolidayDays >= calendarDays {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "假期天数必须小于当月天数"})
		return
	}
	parsedID, err := strconv.ParseUint(employeeID, 10, 64)
	if err != nil || parsedID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "员工编号无效"})
		return
	}
	id := uint(parsedID)

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "无法启动离职交接事务"})
		return
	}
	defer tx.Rollback()
	var employee models.Employee
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&employee, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "员工不存在"})
		return
	}
	if employee.IsOutsourced {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "外包人员请走合作结算流程"})
		return
	}
	if employee.EmploymentStatus != "" && employee.EmploymentStatus != models.EmployeeStatusActive {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "该员工已有离职流程或已离职"})
		return
	}
	if employee.EntryDate != nil && lastWorkingDate.Before(offboardingDate(*employee.EntryDate)) {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "最后工作日不能早于入职日期"})
		return
	}
	if req.SuccessorEmployeeID != nil {
		if *req.SuccessorEmployeeID == employee.ID {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "接手人不能是离职员工本人"})
			return
		}
		var successor models.Employee
		if err := tx.First(&successor, *req.SuccessorEmployeeID).Error; err != nil || !successor.CanUseEmployeeAccess(time.Now()) {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请选择一名在职接手人"})
			return
		}
	}

	caseRecord := models.EmployeeOffboarding{
		EmployeeID: employee.ID, EmployeeName: employee.Name, EmpNo: employee.EmpNo,
		LastWorkingDate: lastWorkingDate, SuccessorEmployeeID: req.SuccessorEmployeeID,
		Status: "in_progress", HolidayDays: req.HolidayDays, CalendarDays: calendarDays,
		BaseSalarySnapshot: employee.BaseSalary, CreatedBy: c.GetString("username"), Notes: strings.TrimSpace(req.Notes),
	}
	if err := tx.Create(&caseRecord).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建离职交接单失败"})
		return
	}
	if err := tx.Model(&employee).Updates(map[string]interface{}{
		"employment_status": models.EmployeeStatusOffboarding,
		"last_working_date": lastWorkingDate,
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新员工任职状态失败"})
		return
	}

	for _, item := range defaultOffboardingChecklist {
		if err := createOffboardingTask(tx, caseRecord, employee, nil, item.key, item.type_, item.title, req.SuccessorEmployeeID); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建交接清单失败"})
			return
		}
	}
	var orders []models.Order
	if err := tx.Where("order_status = ? AND (employee_id = ? OR delivery_person_id = ? OR installer_id = ?)",
		1, employee.ID, employee.ID, employee.ID).Order("created_at asc, id asc").Find(&orders).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取员工未结订单失败"})
		return
	}
	for _, order := range orders {
		needsFollowup := order.EmployeeID != nil && *order.EmployeeID == employee.ID &&
			(order.PaymentStatus == 1 || order.SubsidyStatus == 1 || order.SubsidyStatus == 2 || orderNeedsFulfillment(order))
		if order.DeliveryPersonID != nil && *order.DeliveryPersonID == employee.ID && orderNeedsFulfillment(order) {
			needsFollowup = true
		}
		if order.InstallerID != nil && *order.InstallerID == employee.ID && orderNeedsFulfillment(order) {
			needsFollowup = true
		}
		if !needsFollowup {
			continue
		}
		task := models.EmployeeHandoverTask{
			OffboardingID: caseRecord.ID, EmployeeID: employee.ID, OrderID: &order.ID,
			TaskKey: fmt.Sprintf("order:%d:followup", order.ID), TaskType: "order_followup",
			Title:              fmt.Sprintf("跟进订单 %s：核对尾款、国补及履约状态", order.OrderNo),
			AssigneeEmployeeID: req.SuccessorEmployeeID, Status: "pending",
		}
		if err := tx.Create(&task).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建订单交接任务失败"})
			return
		}
		if orderNeedsFulfillment(order) {
			if err := createPendingFulfillmentAlert(tx, caseRecord, employee, order); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建履约告警失败"})
				return
			}
		}
	}
	var tasks []models.EmployeeHandoverTask
	if err := tx.Where("offboarding_id = ?", caseRecord.ID).Order("id asc").Find(&tasks).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取交接任务失败，离职交接未提交"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "离职交接提交失败"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": 200, "msg": "离职交接已发起", "data": gin.H{"offboarding": caseRecord, "tasks": tasks}})
}

func createOffboardingTask(tx *gorm.DB, caseRecord models.EmployeeOffboarding, employee models.Employee, orderID *uint, key, taskType, title string, successorID *uint) error {
	task := models.EmployeeHandoverTask{
		OffboardingID: caseRecord.ID, EmployeeID: employee.ID, OrderID: orderID,
		TaskKey: key, TaskType: taskType, Title: title, AssigneeEmployeeID: successorID, Status: "pending",
	}
	if err := tx.Create(&task).Error; err != nil {
		return fmt.Errorf("create task %s: %w", key, err)
	}
	return nil
}

func orderNeedsFulfillment(order models.Order) bool {
	return order.OrderStatus == 1 && order.DeliveryMethod != 1 && (!order.IsDelivered || !order.IsInstalled)
}

func createPendingFulfillmentAlert(tx *gorm.DB, caseRecord models.EmployeeOffboarding, employee models.Employee, order models.Order) error {
	message := "离职员工关联订单尚未完成配送或安装，请确认接手人并持续跟进"
	commission, err := employeeCommissionForOrder(tx, order, employee.ID)
	if err != nil {
		return err
	}
	alert := models.EmployeeOffboardingAlert{
		OffboardingID: caseRecord.ID, EmployeeID: employee.ID, EmployeeName: employee.Name,
		OrderID: order.ID, SourceKey: fmt.Sprintf("pending:%d:%d", caseRecord.ID, order.ID),
		AlertType: "pending_fulfillment", Severity: "high", Status: "open", OrderNo: order.OrderNo,
		CommissionAmount: commission, Message: message, OccurredAt: time.Now(),
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&alert).Error; err != nil {
		return fmt.Errorf("save pending fulfillment alert: %w", err)
	}
	return nil
}

func attachOrderToActiveOffboarding(tx *gorm.DB, order models.Order, employeeID *uint) error {
	if employeeID == nil || *employeeID == 0 {
		return nil
	}
	var employee models.Employee
	if err := tx.Unscoped().First(&employee, *employeeID).Error; err != nil {
		return fmt.Errorf("load seller for handover: %w", err)
	}
	if employee.EmploymentStatus != models.EmployeeStatusOffboarding || employee.LastWorkingDate == nil || !employee.WasEmployedAt(order.CreatedAt) {
		return nil
	}
	var caseRecord models.EmployeeOffboarding
	if err := tx.Where("employee_id = ? AND status = ?", employee.ID, "in_progress").Order("id desc").First(&caseRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load active offboarding case: %w", err)
	}
	needsFollowup := order.PaymentStatus == 1 || order.SubsidyStatus == 1 || order.SubsidyStatus == 2 || orderNeedsFulfillment(order)
	if !needsFollowup {
		return nil
	}
	task := models.EmployeeHandoverTask{
		OffboardingID: caseRecord.ID, EmployeeID: employee.ID, OrderID: &order.ID,
		TaskKey: fmt.Sprintf("order:%d:followup", order.ID), TaskType: "order_followup",
		Title:              fmt.Sprintf("跟进订单 %s：核对尾款、国补及履约状态", order.OrderNo),
		AssigneeEmployeeID: caseRecord.SuccessorEmployeeID, Status: "pending",
	}
	if err := tx.Create(&task).Error; err != nil {
		return fmt.Errorf("create handover task for new order %s: %w", order.OrderNo, err)
	}
	if orderNeedsFulfillment(order) {
		if err := createPendingFulfillmentAlert(tx, caseRecord, employee, order); err != nil {
			return err
		}
	}
	return nil
}

func employeeCommissionForOrder(tx *gorm.DB, order models.Order, employeeID uint) (float64, error) {
	if order.OrderType != 2 {
		if order.EmployeeID != nil && *order.EmployeeID == employeeID && CountsTowardEmployeePerformance(order.ReferrerName) {
			return float64(moneyCents(order.CommissionAmount)) / 100, nil
		}
		return 0, nil
	}
	var employees []models.Employee
	if err := tx.Unscoped().Where("is_outsourced = ?", false).Find(&employees).Error; err != nil {
		return 0, fmt.Errorf("load chamber commission roster: %w", err)
	}
	eligibleIDs := chamberEligibleEmployeeIDs(employees, order.CreatedAt)
	shares := splitChamberCommission(chamberCommissionPoolCents(order), eligibleIDs)
	return float64(shares[employeeID]) / 100, nil
}

// ListEmployeeOffboardingAlerts 获取离职订单提醒
// @Summary 获取离职订单提醒
// @Description 获取离职订单提醒
// @Tags EmployeeOffboarding
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_employee_offboarding_alerts
// @Router /api/employee-offboarding-alerts [get]
func ListEmployeeOffboardingAlerts(c *gin.Context) {
	var alerts []models.EmployeeOffboardingAlert
	if err := config.DB.Where("status = ?", "open").Order("occurred_at desc, id desc").Limit(100).Find(&alerts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "离职相关告警加载失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": alerts})
}

// GetEmployeeOffboarding 获取员工离职交接详情
// @Summary 获取员工离职交接详情
// @Description 获取员工离职交接详情
// @Tags EmployeeOffboarding
// @Param id path int true "id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_employees_id_offboardings
// @Router /api/employees/{id}/offboardings [get]
func GetEmployeeOffboarding(c *gin.Context) {
	var employee models.Employee
	if err := config.DB.First(&employee, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "员工不存在"})
		return
	}
	var caseRecord models.EmployeeOffboarding
	err := config.DB.Where("employee_id = ?", employee.ID).Order("id desc").First(&caseRecord).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusOK, gin.H{"code": 200, "data": nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "离职交接记录加载失败"})
		return
	}
	var tasks []models.EmployeeHandoverTask
	if err := config.DB.Where("offboarding_id = ?", caseRecord.ID).Order("id asc").Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "交接任务加载失败"})
		return
	}
	var alerts []models.EmployeeOffboardingAlert
	if err := config.DB.Where("offboarding_id = ?", caseRecord.ID).Order("occurred_at desc, id desc").Find(&alerts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "离职告警加载失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"offboarding": caseRecord, "tasks": tasks, "alerts": alerts}})
}

// UpdateEmployeeHandoverTask 更新离职交接任务
// @Summary 更新离职交接任务
// @Description 更新离职交接任务
// @Tags EmployeeOffboarding
// @Param id path int true "id"
// @Param task_id path int true "task_id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID put_api_employee_offboardings_id_tasks_task_id
// @Router /api/employee-offboardings/{id}/tasks/{task_id} [put]
func UpdateEmployeeHandoverTask(c *gin.Context) {
	var req UpdateEmployeeHandoverTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Done == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请提交任务完成状态"})
		return
	}
	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "无法启动交接任务事务"})
		return
	}
	defer func() {
		if tx.Error != nil {
			tx.Rollback()
		}
	}()
	var caseRecord models.EmployeeOffboarding
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&caseRecord, c.Param("id")).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "离职交接单不存在"})
		return
	}
	if caseRecord.Status != "in_progress" {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "交接单已确认结算，任务不能修改"})
		return
	}
	var task models.EmployeeHandoverTask
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND offboarding_id = ?", c.Param("task_id"), caseRecord.ID).First(&task).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "交接任务不存在"})
		return
	}
	updates := map[string]interface{}{"notes": strings.TrimSpace(req.Notes)}
	if *req.Done {
		if strings.TrimSpace(req.Notes) == "" {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "完成交接任务时请填写验收说明"})
			return
		}
		now := time.Now()
		updates["status"] = "done"
		updates["completed_by"] = c.GetString("username")
		updates["completed_at"] = now
	} else {
		updates["status"] = "pending"
		updates["completed_by"] = ""
		updates["completed_at"] = nil
	}
	if err := tx.Model(&task).Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新交接任务失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "交接任务提交失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "交接任务已更新"})
}

// ConfirmEmployeeOffboardingSettlement 确认离职结算
// @Summary 确认离职结算
// @Description 确认离职结算
// @Tags EmployeeOffboarding
// @Param id path int true "id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_employee_offboardings_id_settlement
// @Router /api/employee-offboardings/{id}/settlement [post]
func ConfirmEmployeeOffboardingSettlement(c *gin.Context) {
	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "无法启动薪资结算事务"})
		return
	}
	defer func() {
		if tx.Error != nil {
			tx.Rollback()
		}
	}()
	var caseRecord models.EmployeeOffboarding
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&caseRecord, c.Param("id")).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "离职交接单不存在"})
		return
	}
	if caseRecord.Status != "in_progress" {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "该离职交接单已确认或已完成"})
		return
	}
	today := offboardingDate(time.Now())
	if !caseRecord.LastWorkingDate.Before(today) {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "最后工作日结束后才能确认离职结算"})
		return
	}
	var pendingTasks int64
	if err := tx.Model(&models.EmployeeHandoverTask{}).Where("offboarding_id = ? AND status <> ?", caseRecord.ID, "done").Count(&pendingTasks).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取未完成交接任务失败"})
		return
	}
	if pendingTasks > 0 {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": fmt.Sprintf("还有 %d 项交接任务未验收", pendingTasks)})
		return
	}
	var employee models.Employee
	if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&employee, caseRecord.EmployeeID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "员工档案不存在，无法结算"})
		return
	}
	monthStart := time.Date(caseRecord.LastWorkingDate.Year(), caseRecord.LastWorkingDate.Month(), 1, 0, 0, 0, 0, time.Local)
	signedDays, err := countSignedInDays(tx, employee, monthStart, caseRecord.LastWorkingDate)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取离职月签到记录失败"})
		return
	}
	calendarDays := time.Date(caseRecord.LastWorkingDate.Year(), caseRecord.LastWorkingDate.Month()+1, 0, 0, 0, 0, 0, time.Local).Day()
	baseSalary, err := calculateOffboardingBaseSalary(caseRecord.BaseSalarySnapshot, calendarDays, caseRecord.HolidayDays, signedDays)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "离职底薪参数无效，请检查假期天数和考勤"})
		return
	}
	personalCommissionCents, chamberCommissionCents, commissionDetails, err := calculateOffboardingCommission(tx, employee, monthStart, caseRecord.LastWorkingDate)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "计算离职月提成失败"})
		return
	}
	personalCommission := float64(personalCommissionCents) / 100
	chamberCommission := float64(chamberCommissionCents) / 100
	commission := personalCommission + chamberCommission
	commission = float64(moneyCents(commission)) / 100
	commissionDetailsJSON, err := json.Marshal(commissionDetails)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存离职提成明细快照失败"})
		return
	}
	finalPayable := float64(moneyCents(baseSalary+commission)) / 100
	now := time.Now()
	updates := map[string]interface{}{
		"status": "completed", "calendar_days": calendarDays, "signed_in_days": signedDays,
		"calculated_base_salary": baseSalary, "personal_commission_snapshot": personalCommission,
		"chamber_commission_snapshot": chamberCommission, "commission_snapshot": commission,
		"commission_details_snapshot": string(commissionDetailsJSON),
		"final_payable_snapshot":      finalPayable, "settlement_month": caseRecord.LastWorkingDate.Format("2006-01"),
		"settlement_confirmed_at": now, "settlement_confirmed_by": c.GetString("username"),
		"completed_at": now, "completed_by": c.GetString("username"),
	}
	if err := tx.Model(&caseRecord).Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存离职结算快照失败"})
		return
	}
	if err := tx.Model(&employee).Updates(map[string]interface{}{"employment_status": models.EmployeeStatusDeparted}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新员工离职状态失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "离职薪资结算提交失败"})
		return
	}
	caseRecord.Status = "completed"
	caseRecord.CalendarDays = calendarDays
	caseRecord.SignedInDays = signedDays
	caseRecord.CalculatedBaseSalary = baseSalary
	caseRecord.PersonalCommissionSnapshot = personalCommission
	caseRecord.ChamberCommissionSnapshot = chamberCommission
	caseRecord.CommissionSnapshot = commission
	caseRecord.CommissionDetailsSnapshot = string(commissionDetailsJSON)
	caseRecord.FinalPayableSnapshot = finalPayable
	caseRecord.SettlementMonth = caseRecord.LastWorkingDate.Format("2006-01")
	caseRecord.SettlementConfirmedAt = &now
	caseRecord.SettlementConfirmedBy = c.GetString("username")
	caseRecord.CompletedAt = &now
	caseRecord.CompletedBy = c.GetString("username")
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "离职交接与应付金额已确认；实际付款仍以财务流水为准", "data": caseRecord})
}

// calculateOffboardingCommission snapshots commissions from orders created
// during the employee's final month through the last working date. The order
// creation time controls attribution; later cancellation or refund is surfaced
// as a separate post-exit alert and does not silently erase this snapshot.
func calculateOffboardingCommission(tx *gorm.DB, employee models.Employee, monthStart, lastWorkingDate time.Time) (int64, int64, []CommissionDetail, error) {
	end := offboardingDate(lastWorkingDate).AddDate(0, 0, 1)
	var orders []models.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("OrderItems.Product").
		Where("created_at >= ? AND created_at < ?", monthStart, end).
		Order("created_at asc, id asc").Find(&orders).Error; err != nil {
		return 0, 0, nil, fmt.Errorf("query orders for offboarding commission: %w", err)
	}
	var employees []models.Employee
	if err := tx.Unscoped().Where("is_outsourced = ?", false).Find(&employees).Error; err != nil {
		return 0, 0, nil, fmt.Errorf("query employees for offboarding commission: %w", err)
	}
	personalCents, chamberCents, details := offboardingCommissionForEmployee(orders, employees, employee.ID)
	return personalCents, chamberCents, details, nil
}

func offboardingCommissionForEmployee(orders []models.Order, employees []models.Employee, employeeID uint) (int64, int64, []CommissionDetail) {
	var personalCents, chamberCents int64
	details := make([]CommissionDetail, 0)
	for _, order := range orders {
		if order.OrderType == 2 {
			if order.CreatedAt.Before(chamberCommissionStart) {
				continue // 2026-10 之前的商会订单不计分成（与月度提成报表口径一致）
			}
			eligibleIDs := chamberEligibleEmployeeIDs(employees, order.CreatedAt)
			shares := splitChamberCommission(chamberCommissionPoolCents(order), eligibleIDs)
			share := shares[employeeID]
			chamberCents += share
			if share > 0 {
				details = append(details, CommissionDetail{
					OrderID: order.ID, OrderNo: order.OrderNo, OrderDate: order.CreatedAt,
					CustomerName: order.CustomerName, ProductModels: commissionProductModels(order),
					OrderType: order.OrderType, OrderAmount: order.TotalAmount,
					CommissionPool:   float64(chamberCommissionPoolCents(order)) / 100,
					ParticipantCount: len(eligibleIDs), CommissionAmount: float64(share) / 100,
				})
			}
			continue
		}
		if order.EmployeeID != nil && *order.EmployeeID == employeeID && CountsTowardEmployeePerformance(order.ReferrerName) {
			commission := moneyCents(order.CommissionAmount)
			personalCents += commission
			if commission > 0 {
				details = append(details, CommissionDetail{
					OrderID: order.ID, OrderNo: order.OrderNo, OrderDate: order.CreatedAt,
					CustomerName: order.CustomerName, ProductModels: commissionProductModels(order),
					OrderType: order.OrderType, OrderAmount: order.TotalAmount,
					CommissionPool: float64(commission) / 100, ParticipantCount: 1,
					CommissionAmount: float64(commission) / 100,
				})
			}
		}
	}
	return personalCents, chamberCents, details
}

// ResolveEmployeeOffboardingAlert 处理离职订单提醒
// @Summary 处理离职订单提醒
// @Description 处理离职订单提醒
// @Tags EmployeeOffboarding
// @Param id path int true "id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_employee_offboarding_alerts_id_resolve
// @Router /api/employee-offboarding-alerts/{id}/resolve [post]
func ResolveEmployeeOffboardingAlert(c *gin.Context) {
	var req ResolveEmployeeOffboardingAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Resolution) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请填写复核处理说明"})
		return
	}
	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "无法启动告警复核事务"})
		return
	}
	defer tx.Rollback()
	var alert models.EmployeeOffboardingAlert
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&alert, c.Param("id")).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "离职订单告警不存在"})
		return
	}
	if alert.Status != "open" || alert.AlertType == "pending_fulfillment" {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "该告警已处理或需等待订单履约完成"})
		return
	}
	now := time.Now()
	if err := tx.Model(&alert).Updates(map[string]interface{}{
		"status": "resolved", "resolved_by": c.GetString("username"), "resolved_at": now,
		"resolution": strings.TrimSpace(req.Resolution),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存告警复核结果失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "告警复核提交失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "告警复核已记录；结算调整需另行登记", "data": gin.H{"id": alert.ID, "status": "resolved"}})
}

func PostExitOrderChangeAlerts(tx *gorm.DB, order models.Order, alertType string, refundAmount float64, financialLogID *uint, occurredAt time.Time) error {
	impacted := map[uint]float64{}
	if order.OrderType == 2 {
		var employees []models.Employee
		if err := tx.Unscoped().Where("is_outsourced = ?", false).Find(&employees).Error; err != nil {
			return fmt.Errorf("load employees for chamber commission: %w", err)
		}
		eligibleIDs := chamberEligibleEmployeeIDs(employees, order.CreatedAt)
		for employeeID, cents := range splitChamberCommission(chamberCommissionPoolCents(order), eligibleIDs) {
			impacted[employeeID] = float64(cents) / 100
		}
	} else if order.EmployeeID != nil && CountsTowardEmployeePerformance(order.ReferrerName) {
		impacted[*order.EmployeeID] = float64(moneyCents(order.CommissionAmount)) / 100
	}
	if len(impacted) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(impacted))
	for id := range impacted {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, employeeID := range ids {
		var employee models.Employee
		if err := tx.Unscoped().First(&employee, employeeID).Error; err != nil {
			return fmt.Errorf("load affected employee %d: %w", employeeID, err)
		}
		if employee.LastWorkingDate == nil || occurredAt.In(time.Local).Format("2006-01-02") <= employee.LastWorkingDate.In(time.Local).Format("2006-01-02") {
			continue
		}
		var caseRecord models.EmployeeOffboarding
		if err := tx.Where("employee_id = ? AND status <> ?", employee.ID, "cancelled").Order("id desc").First(&caseRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return fmt.Errorf("load offboarding case for employee %d: %w", employee.ID, err)
		}
		sourceID := alertType
		message := "离职后关联订单发生取消或红冲，需复核该订单提成及离职结算"
		if alertType == "order_refund" {
			sourceID = fmt.Sprintf("refund-%d", valueOrZero(financialLogID))
			message = "离职后关联订单登记了退款，需复核该订单提成及离职结算"
		} else if alertType == "order_cancelled" {
			message = "离职后关联订单被取消，需复核该订单提成及离职结算"
		} else if alertType == "order_reversed" {
			message = "离职后关联订单被红冲，需复核该订单提成及离职结算"
		}
		alert := models.EmployeeOffboardingAlert{
			OffboardingID: caseRecord.ID, EmployeeID: employee.ID, EmployeeName: employee.Name,
			OrderID: order.ID, FinancialLogID: financialLogID,
			SourceKey: fmt.Sprintf("%s:%d:%d", sourceID, order.ID, employee.ID),
			AlertType: alertType, Severity: "high", Status: "open", OrderNo: order.OrderNo,
			RefundAmount: float64(moneyCents(refundAmount)) / 100, CommissionAmount: impacted[employee.ID],
			Message: message, OccurredAt: occurredAt,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&alert).Error; err != nil {
			return fmt.Errorf("save post-exit order alert: %w", err)
		}
	}
	return nil
}

func valueOrZero(value *uint) uint {
	if value == nil {
		return 0
	}
	return *value
}

func resolvePendingFulfillmentAlert(tx *gorm.DB, orderID uint) error {
	var order models.Order
	if err := tx.First(&order, orderID).Error; err != nil {
		return fmt.Errorf("load order %d to refresh handover alert: %w", orderID, err)
	}
	if order.OrderStatus != 2 && order.DeliveryMethod != 1 && (!order.IsDelivered || !order.IsInstalled) {
		return nil
	}
	now := time.Now()
	if err := tx.Model(&models.EmployeeOffboardingAlert{}).
		Where("order_id = ? AND alert_type = ? AND status = ?", order.ID, "pending_fulfillment", "open").
		Updates(map[string]interface{}{"status": "resolved", "resolved_by": "system", "resolved_at": now, "resolution": "订单已取消或履约完成"}).Error; err != nil {
		return fmt.Errorf("resolve completed fulfillment alert: %w", err)
	}
	return nil
}
