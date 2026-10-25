package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateExpenseRequest 记账请求体
type CreateExpenseRequest struct {
	ExpenseType     string  `json:"expense_type" binding:"required"`
	Amount          float64 `json:"amount" binding:"required,gt=0"`
	Remark          string  `json:"remark"`
	AccountID       uint    `json:"account_id"`
	OccurredAt      string  `json:"occurred_at"`
	RelatedNo       string  `json:"related_no"`
	ClientRequestID string  `json:"client_request_id"`
}

// refundCapEpsilon 金额比较容差（分），避免浮点累积误差导致临界退款被误拒。
const refundCapEpsilon = 0.005

// sumOrderRefunds 统计订单已累计退款金额，覆盖两条记账路径且不重复计数：
//  1. 支出表驱动（POST /api/expenses）：退款写入 expenses，同时写入 source_type=system 的财务流水；
//  2. 手工记账驱动（POST /api/bookkeeping/entries）：退款只写入 source_type=manual 的财务流水。
//
// 因此累计退款 = SUM(expenses 中退款) + SUM(financial_logs 中 manual 来源的退款支出)。
func sumOrderRefunds(tx *gorm.DB, orderNo string) (float64, error) {
	var expenseTotal float64
	if err := tx.Model(&models.Expense{}).
		Where("related_no = ? AND expense_type LIKE ?", orderNo, "%退款%").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&expenseTotal).Error; err != nil {
		return 0, err
	}
	var bookkeepingTotal float64
	if err := tx.Model(&models.FinancialLog{}).
		Where("related_no = ? AND flow_type = ? AND source_type = ? AND category LIKE ? AND reversal_pair_id IS NULL",
			orderNo, bookkeepingExpense, "manual", "%退款%").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&bookkeepingTotal).Error; err != nil {
		return 0, err
	}
	return expenseTotal + bookkeepingTotal, nil
}

// checkRefundLimit 退款防重上限校验：当支出类型为退款且关联订单时，
// 校验"已有累计退款 + 本次金额"不超过订单实付金额（ActualPayAmount）。
// related_no 不是订单号时不做校验，保持历史行为兼容。
func checkRefundLimit(tx *gorm.DB, relatedNo string, expenseType string, amount float64) error {
	if !strings.Contains(expenseType, "退款") || strings.TrimSpace(relatedNo) == "" {
		return nil
	}
	var order models.Order
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", relatedNo).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("查询退款关联订单失败: %w", err)
	}
	return checkRefundLimitForOrder(tx, order, amount)
}

// checkRefundLimitForOrder 基于已锁定的订单校验退款上限（供支出表与手工记账两条路径共用）。
func checkRefundLimitForOrder(tx *gorm.DB, order models.Order, amount float64) error {
	existing, err := sumOrderRefunds(tx, order.OrderNo)
	if err != nil {
		return fmt.Errorf("统计订单累计退款失败: %w", err)
	}
	if existing+amount > order.ActualPayAmount+refundCapEpsilon {
		return fmt.Errorf("该订单累计退款 %.2f 元，超过实付 %.2f 元", existing+amount, order.ActualPayAmount)
	}
	return nil
}

// CreateExpense 接收 JSON 数据并插入 Expense 表，同步更新账户余额
// @Summary 创建支出
// @Description 创建支出
// @Tags Expenses
// @Accept json
// @Produce json
// @Param body body CreateExpenseRequest true "支出参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_expenses
// @Router /api/expenses [post]
func CreateExpense(c *gin.Context) {
	var req CreateExpenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "请求参数错误: " + err.Error(),
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 记账幂等：请求携带 client_request_id 时，事务内先按该键查询，
	// 已存在支出记录则直接返回（duplicated:true），不重复扣余额、不重复写流水。
	req.ClientRequestID = strings.TrimSpace(req.ClientRequestID)
	if req.ClientRequestID != "" {
		var existing models.Expense
		idempotencyErr := tx.Where("client_request_id = ?", req.ClientRequestID).First(&existing).Error
		if idempotencyErr == nil {
			tx.Rollback()
			c.JSON(http.StatusOK, gin.H{
				"code":       200,
				"msg":        "该请求已提交过，返回已有支出记录，未重复记账",
				"duplicated": true,
				"data":       existing,
			})
			return
		}
		if !errors.Is(idempotencyErr, gorm.ErrRecordNotFound) {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "幂等键查询失败"})
			return
		}
	}

	// 退款防重：累计退款超过订单实付金额时拒绝，防止超额退款
	if err := checkRefundLimit(tx, req.RelatedNo, req.ExpenseType, req.Amount); err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  err.Error(),
		})
		return
	}

	// 如果未提供 AccountID，查找默认账户
	if req.AccountID == 0 {
		var defaultAcc models.Account
		if err := tx.Where("is_default_expense = ?", true).First(&defaultAcc).Error; err == nil {
			req.AccountID = defaultAcc.ID
		} else {
			// 如果没有显式设置默认账户，取第一个账户
			var firstAcc models.Account
			if err := tx.First(&firstAcc).Error; err == nil {
				req.AccountID = firstAcc.ID
			} else {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"code": 400,
					"msg":  "系统暂无任何资金账户，请先创建账户",
				})
				return
			}
		}
	}

	// 加锁并验证账户（FOR UPDATE 防止并发支出导致余额计算错误）
	var account models.Account
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, req.AccountID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "账户不存在",
		})
		return
	}

	// 检查余额是否充足（仅作提示，不强制拦截，允许账面出现负值用于校准）
	if account.Balance < req.Amount {
		// 仅警告，不阻止，因为余额是"参考值"而非严格银行余额
		_ = fmt.Sprintf("账户「%s」账面余额不足，将记录为负值，请使用一键校准功能对齐真实余额", account.Name)
	}

	// 1. 更新账户余额（支出，减少余额）
	if err := tx.Model(&account).UpdateColumn("balance", gorm.Expr("balance - ?", req.Amount)).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "账户余额扣除失败",
		})
		return
	}

	var occurredAt time.Time
	if req.OccurredAt != "" {
		parsedTime, err := time.ParseInLocation("2006-01-02 15:04:05", req.OccurredAt, time.Local)
		if err == nil {
			occurredAt = parsedTime
		} else {
			// 如果解析失败则回退到当前时间，或者也可以返回错误
			occurredAt = time.Now()
		}
	} else {
		occurredAt = time.Now()
	}

	// 1.5. 写入支出明细表
	expense := models.Expense{
		ExpenseType: req.ExpenseType,
		Amount:      req.Amount,
		Remark:      req.Remark,
		AccountID:   &req.AccountID,
		RelatedNo:   req.RelatedNo,
		CreatedAt:   occurredAt,
	}
	if req.ClientRequestID != "" {
		expense.ClientRequestID = &req.ClientRequestID
	}
	if err := tx.Create(&expense).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "支出明细记录创建失败",
		})
		return
	}

	// 2. 写入财务流水
	var updatedAccount models.Account
	tx.First(&updatedAccount, req.AccountID)
	if err := WriteFinancialLog(tx, req.AccountID, 2, req.ExpenseType, req.Amount, updatedAccount.Balance, req.RelatedNo, req.Remark, occurredAt); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "财务流水写入失败",
		})
		return
	}
	if strings.Contains(req.ExpenseType, "退款") && req.RelatedNo != "" {
		var linkedOrder models.Order
		if err := tx.Where("order_no = ?", req.RelatedNo).First(&linkedOrder).Error; err == nil {
			var financialLog models.FinancialLog
			if err := tx.Where("related_no = ? AND account_id = ? AND category = ?", linkedOrder.OrderNo, req.AccountID, req.ExpenseType).Order("id DESC").First(&financialLog).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询退款流水失败"})
				return
			}
			if err := postExitOrderRefundAlert(tx, linkedOrder, req.ExpenseType, req.Amount, &financialLog.ID, time.Now()); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "离职后订单退款告警写入失败，支出操作已回滚"})
				return
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询退款关联订单失败"})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "支出记录已保存，账户余额已更新",
	})
}

// GetExpenses 按时间倒序查询所有支出记录
// @Summary 获取支出列表
// @Description 获取支出列表
// @Tags Expenses
// @Accept json
// @Produce json
// @Param expense_type query string false "支出类型"
// @Param start_date query string false "开始日期"
// @Param end_date query string false "结束日期"
// @Success 200 {array} models.Expense "支出列表"
// @Security ApiKeyAuth
// @ID get_api_expenses
// @Router /api/expenses [get]
func GetExpenses(c *gin.Context) {
	expenseType := c.Query("expense_type")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := config.DB.Preload("Account").Order("created_at desc")
	if expenseType != "" {
		query = query.Where("expense_type = ?", expenseType)
	}
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("created_at <= ?", endDate)
	}

	var expenses []models.Expense
	if err := query.Find(&expenses).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch expenses"})
		return
	}

	c.JSON(http.StatusOK, expenses)
}

// UpdateExpense 修改支出记录并同步处理账户和流水
// @Summary 修改支出
// @Description 修改支出，自动处理旧账冲销和新账扣除
// @Tags Expenses
// @Accept json
// @Produce json
// @Param id path int true "支出ID"
// @Param body body CreateExpenseRequest true "支出参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID put_api_expenses
// @Router /api/expenses/{id} [put]
func UpdateExpense(c *gin.Context) {
	expenseID := c.Param("id")
	var req CreateExpenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "请求参数错误: " + err.Error(),
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var expense models.Expense
	if err := tx.First(&expense, expenseID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "未找到该支出记录",
		})
		return
	}

	// 确定新的时间
	var occurredAt time.Time
	if req.OccurredAt != "" {
		parsedTime, err := time.ParseInLocation("2006-01-02 15:04:05", req.OccurredAt, time.Local)
		if err == nil {
			occurredAt = parsedTime
		} else {
			occurredAt = expense.CreatedAt
		}
	} else {
		occurredAt = expense.CreatedAt
	}

	// 1. 冲销旧账户（如果原来有账户的话）
	if expense.AccountID != nil {
		var oldAccount models.Account
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&oldAccount, *expense.AccountID).Error; err == nil {
			// 加回原来的金额
			if err := tx.Model(&oldAccount).UpdateColumn("balance", gorm.Expr("balance + ?", expense.Amount)).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "旧账回滚失败"})
				return
			}
			tx.First(&oldAccount, *expense.AccountID)
			// 写回滚流水（类别为冲销）
			if err := WriteFinancialLog(tx, oldAccount.ID, 1, "支出修改-冲销旧账", expense.Amount, oldAccount.Balance, expense.RelatedNo, fmt.Sprintf("原支出类型: %s", expense.ExpenseType), time.Now()); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "旧账流水写入失败"})
				return
			}
		}
	}

	// 2. 扣除新账户
	newAccountID := req.AccountID
	if newAccountID == 0 {
		var defaultAcc models.Account
		if err := tx.Where("is_default_expense = ?", true).First(&defaultAcc).Error; err == nil {
			newAccountID = defaultAcc.ID
		} else {
			var firstAcc models.Account
			if err := tx.First(&firstAcc).Error; err == nil {
				newAccountID = firstAcc.ID
			} else {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "系统暂无任何资金账户"})
				return
			}
		}
	}

	var newAccount models.Account
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&newAccount, newAccountID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "新账户不存在"})
		return
	}

	if newAccount.Balance < req.Amount {
		_ = fmt.Sprintf("账户「%s」账面余额不足，将记录为负值", newAccount.Name)
	}

	if err := tx.Model(&newAccount).UpdateColumn("balance", gorm.Expr("balance - ?", req.Amount)).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "新账户扣款失败"})
		return
	}

	tx.First(&newAccount, newAccountID)
	if err := WriteFinancialLog(tx, newAccountID, 2, req.ExpenseType, req.Amount, newAccount.Balance, req.RelatedNo, req.Remark, occurredAt); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "新账流水写入失败"})
		return
	}
	if strings.Contains(req.ExpenseType, "退款") && req.RelatedNo != "" {
		var linkedOrder models.Order
		if err := tx.Where("order_no = ?", req.RelatedNo).First(&linkedOrder).Error; err == nil {
			var financialLog models.FinancialLog
			if err := tx.Where("related_no = ? AND account_id = ? AND category = ?", linkedOrder.OrderNo, newAccountID, req.ExpenseType).Order("id DESC").First(&financialLog).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询退款流水失败"})
				return
			}
			if err := postExitOrderRefundAlert(tx, linkedOrder, req.ExpenseType, req.Amount, &financialLog.ID, time.Now()); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "离职后订单退款告警写入失败，支出操作已回滚"})
				return
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询退款关联订单失败"})
			return
		}
	}

	// 3. 更新Expense记录
	updates := map[string]interface{}{
		"expense_type": req.ExpenseType,
		"amount":       req.Amount,
		"remark":       req.Remark,
		"account_id":   newAccountID,
		"related_no":   req.RelatedNo,
		"created_at":   occurredAt,
	}

	if err := tx.Model(&expense).Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "支出明细更新失败"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "支出记录已修改",
	})
}
