package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateExpenseRequest 记账请求体
type CreateExpenseRequest struct {
	ExpenseType string  `json:"expense_type" binding:"required"`
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	Remark      string  `json:"remark"`
	AccountID   uint    `json:"account_id" binding:"required"`
}

// CreateExpense 接收 JSON 数据并插入 Expense 表，同步更新账户余额
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

	// 2. 写入财务流水
	var updatedAccount models.Account
	tx.First(&updatedAccount, req.AccountID)
	if err := WriteFinancialLog(tx, req.AccountID, 2, req.ExpenseType, req.Amount, updatedAccount.Balance, "", req.Remark); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "财务流水写入失败",
		})
		return
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
func GetExpenses(c *gin.Context) {
	var expenses []models.Expense
	if err := config.DB.Preload("Account").Order("created_at desc").Find(&expenses).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch expenses"})
		return
	}

	c.JSON(http.StatusOK, expenses)
}
