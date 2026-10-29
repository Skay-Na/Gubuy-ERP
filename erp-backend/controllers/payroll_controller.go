package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// operatingExpenseAccountID 经营开销台账账户（非真实资金账户）。
// 记账规则：非进货成本的一切花销（含工资）记经营开销账户，不进资金账户流水。
const operatingExpenseAccountID = 7

const operatingExpenseAccountName = "经营开销"

type PayrollConfirmItem struct {
	EmployeeID uint    `json:"employee_id" binding:"required"`
	Amount     float64 `json:"amount" binding:"required,gt=0"`
}

type PayrollConfirmRequest struct {
	Month string               `json:"month" binding:"required"`
	Items []PayrollConfirmItem `json:"items" binding:"required,min=1,dive"`
}

// resolveOperatingExpenseAccount 定位经营开销台账账户：优先固定 ID=7，
// 找不到时按账户名"经营开销"兜底（隔离测试库可能没有固定 ID）。
func resolveOperatingExpenseAccount(tx *gorm.DB) (models.Account, error) {
	var account models.Account
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, operatingExpenseAccountID).Error
	if err == nil {
		return account, nil
	}
	if !strings.Contains(err.Error(), "record not found") {
		return account, fmt.Errorf("经营开销账户读取失败")
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("name = ?", operatingExpenseAccountName).First(&account).Error; err != nil {
		return account, fmt.Errorf("经营开销账户(id=%d 或名称「%s」)不存在，请先创建", operatingExpenseAccountID, operatingExpenseAccountName)
	}
	return account, nil
}

// ConfirmPayroll 确认本月工资已发放
// @Summary 确认工资发放
// @Description 单事务内校验员工在职并防重复确认，写 payroll_confirmations 并在经营开销账户记"工资"支出流水（不进资金账户流水）。
// @Tags Payroll
// @Accept json
// @Produce json
// @Param body body PayrollConfirmRequest true "工资确认参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_payroll_confirm
// @Router /api/payroll/confirm [post]
func ConfirmPayroll(c *gin.Context) {
	var req PayrollConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	if _, err := time.ParseInLocation("2006-01", req.Month, time.Now().Location()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "月份格式错误，应为 YYYY-MM"})
		return
	}

	// 规整金额并拒绝请求内重复员工
	amountByEmployee := map[uint]float64{}
	employeeIDs := make([]uint, 0, len(req.Items))
	for _, item := range req.Items {
		amount := math.Round(item.Amount*100) / 100
		if amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "实发工资金额必须大于 0"})
			return
		}
		if _, exists := amountByEmployee[item.EmployeeID]; exists {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "同一员工在同一次确认中重复提交"})
			return
		}
		amountByEmployee[item.EmployeeID] = amount
		employeeIDs = append(employeeIDs, item.EmployeeID)
	}
	sort.Slice(employeeIDs, func(i, j int) bool { return employeeIDs[i] < employeeIDs[j] })

	type payrollBooking struct {
		employee models.Employee
		amount   float64
		snapshot string
	}
	var bookings []payrollBooking
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		// 1. 经营开销台账账户
		ledgerAccount, err := resolveOperatingExpenseAccount(tx)
		if err != nil {
			return err
		}

		// 2. 幂等：同月同员工已确认过则拒绝（唯一索引 uk_employee_month 兜底）
		var existing []models.PayrollConfirmation
		if err := tx.Where("month = ? AND employee_id IN ?", req.Month, employeeIDs).Find(&existing).Error; err != nil {
			return fmt.Errorf("查询工资确认记录失败")
		}
		if len(existing) > 0 {
			var employeeIDsExisting []uint
			for _, record := range existing {
				employeeIDsExisting = append(employeeIDsExisting, record.EmployeeID)
			}
			var names []string
			tx.Model(&models.Employee{}).Where("id IN ?", employeeIDsExisting).Pluck("name", &names)
			return fmt.Errorf("员工 %s 的 %s 工资已确认过发放，不能重复确认", strings.Join(names, "、"), req.Month)
		}

		// 3. 逐条校验员工存在且在职（离职员工不能确认新工资）
		for _, employeeID := range employeeIDs {
			var employee models.Employee
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&employee, employeeID).Error; err != nil {
				return fmt.Errorf("员工 ID %d 不存在", employeeID)
			}
			if employee.EmploymentStatus == models.EmployeeStatusDeparted {
				return fmt.Errorf("员工 %s 已离职，不能确认工资发放", employee.Name)
			}
			bookings = append(bookings, payrollBooking{employee: employee, amount: amountByEmployee[employeeID], snapshot: payrollSnapshotForMonth(req.Month, employeeID)})
		}

		// 4. 写确认记录 + 经营开销"工资"支出流水（不进资金账户流水）
		operator := c.GetString("username")
		now := time.Now()
		balance := ledgerAccount.Balance
		for _, booking := range bookings {
			confirmation := models.PayrollConfirmation{
				EmployeeID: booking.employee.ID,
				Month:      req.Month,
				Amount:     booking.amount,
				Snapshot:   booking.snapshot,
				CreatedBy:  operator,
				CreatedAt:  now,
			}
			if err := tx.Create(&confirmation).Error; err != nil {
				return fmt.Errorf("写入工资确认记录失败")
			}
			balance = math.Round((balance-booking.amount)*100) / 100
			if err := tx.Model(&models.Account{}).Where("id = ?", ledgerAccount.ID).
				UpdateColumn("balance", gorm.Expr("balance - ?", booking.amount)).Error; err != nil {
				return fmt.Errorf("经营开销账户记账失败")
			}
			relatedNo := fmt.Sprintf("PAY-%s-%d", req.Month, booking.employee.ID)
			remark := fmt.Sprintf("%s 工资 %s", req.Month, booking.employee.Name)
			if err := WriteFinancialLog(tx, ledgerAccount.ID, 2, "工资", booking.amount, balance, relatedNo, remark, now); err != nil {
				return fmt.Errorf("经营开销工资流水写入失败")
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": err.Error()})
		return
	}

	total := 0.0
	for _, amount := range amountByEmployee {
		total += amount
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  fmt.Sprintf("已确认 %d 名员工 %s 工资发放，合计 ¥%.2f", len(bookings), req.Month, total),
	})
}

// GetPayrollConfirmations 工资确认记录列表
// @Summary 获取工资确认记录
// @Description 可按月份筛选，含员工姓名/岗位，供薪资抽屉展示确认台账
// @Tags Payroll
// @Produce json
// @Param month query string false "月份 YYYY-MM"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_payroll_confirmations
// @Router /api/payroll/confirmations [get]
func GetPayrollConfirmations(c *gin.Context) {
	query := config.DB.Table("payroll_confirmations").
		Select("payroll_confirmations.id, payroll_confirmations.employee_id, payroll_confirmations.month, " +
			"payroll_confirmations.amount, payroll_confirmations.created_by, payroll_confirmations.created_at, " +
			"employees.name AS employee_name, employees.emp_no, employees.role AS employee_role").
		Joins("LEFT JOIN employees ON employees.id = payroll_confirmations.employee_id")

	if month := strings.TrimSpace(c.Query("month")); month != "" {
		if _, err := time.ParseInLocation("2006-01", month, time.Now().Location()); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "月份格式错误，应为 YYYY-MM"})
			return
		}
		query = query.Where("payroll_confirmations.month = ?", month)
	}

	records := []gin.H{}
	rows := []map[string]interface{}{}
	if err := query.Order("payroll_confirmations.created_at desc, payroll_confirmations.id desc").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取工资确认记录失败"})
		return
	}
	for _, row := range rows {
		records = append(records, gin.H{
			"id":            row["id"],
			"employee_id":   row["employee_id"],
			"employee_name": row["employee_name"],
			"emp_no":        row["emp_no"],
			"employee_role": row["employee_role"],
			"month":         row["month"],
			"amount":        row["amount"],
			"created_by":    row["created_by"],
			"created_at":    row["created_at"],
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": records})
}
