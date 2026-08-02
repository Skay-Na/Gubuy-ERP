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

// GetAccounts 查询所有账户及余额
// @Summary 获取账户列表
// @Description 获取账户列表
// @Tags Accounts
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_accounts
// @Router /api/accounts [get]
func GetAccounts(c *gin.Context) {
	var accounts []models.Account
	if err := config.DB.Order("id asc").Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "账户查询失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "查询成功",
		"data": accounts,
	})
}

// CreateAccountRequest 创建账户请求
type CreateAccountRequest struct {
	Name    string  `json:"name" binding:"required"`
	Balance float64 `json:"balance"`
}

// CreateAccount 新增账户（如厂家余额账户）
// @Summary 创建账户
// @Description 创建账户
// @Tags Accounts
// @Accept json
// @Produce json
// @Param body body CreateAccountRequest true "账户参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_accounts
// @Router /api/accounts [post]
func CreateAccount(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	account := models.Account{
		Name:    req.Name,
		Balance: req.Balance,
	}

	if err := config.DB.Create(&account).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "账户创建失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "账户创建成功", "data": account})
}

// AccountTransferRequest 转账请求
type AccountTransferRequest struct {
	FromAccountID uint    `json:"from_account_id" binding:"required"`
	ToAccountID   uint    `json:"to_account_id" binding:"required"`
	Amount        float64 `json:"amount" binding:"required,gt=0"`
	Remark        string  `json:"remark"`
}

// TransferFunds 账户间资金划拨
// @Summary 资金划拨
// @Description 资金划拨
// @Tags Accounts
// @Accept json
// @Produce json
// @Param body body AccountTransferRequest true "资金划拨参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_accounts_transfer
// @Router /api/accounts/transfer [post]
func TransferFunds(c *gin.Context) {
	var req AccountTransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	if req.FromAccountID == req.ToAccountID {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "不能在同一个账户间转账"})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. 加锁并检查源账户（FOR UPDATE 防止并发重复扣款）
	var fromAcc models.Account
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&fromAcc, req.FromAccountID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "源账户不存在"})
		return
	}
	if fromAcc.Balance < req.Amount {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": fmt.Sprintf("源账户「%s」余额不足（当前 ¥%.2f，划拨 ¥%.2f）", fromAcc.Name, fromAcc.Balance, req.Amount)})
		return
	}

	if err := tx.Model(&fromAcc).UpdateColumn("balance", gorm.Expr("balance - ?", req.Amount)).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "扣款失败"})
		return
	}

	// 2. 加锁并增加目标账户
	var toAcc models.Account
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&toAcc, req.ToAccountID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "目标账户不存在"})
		return
	}

	if err := tx.Model(&toAcc).UpdateColumn("balance", gorm.Expr("balance + ?", req.Amount)).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "入账失败"})
		return
	}

	// 3. 记录转账历史 (流水表)
	var fromAccountUpdated models.Account
	var toAccountUpdated models.Account
	tx.First(&fromAccountUpdated, req.FromAccountID)
	tx.First(&toAccountUpdated, req.ToAccountID)

	if err := WriteFinancialLog(tx, req.FromAccountID, 3, "资金划转", req.Amount, fromAccountUpdated.Balance, "", fmt.Sprintf("转出到账户 ID %d: %s", req.ToAccountID, req.Remark)); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记录转出流水失败"})
		return
	}
	if err := WriteFinancialLog(tx, req.ToAccountID, 3, "资金划转", req.Amount, toAccountUpdated.Balance, "", fmt.Sprintf("从账户 ID %d 转入: %s", req.FromAccountID, req.Remark)); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记录转入流水失败"})
		return
	}

	record := models.TransferRecord{
		FromAccountID: req.FromAccountID,
		ToAccountID:   req.ToAccountID,
		Amount:        req.Amount,
		Remark:        req.Remark,
	}
	if err := tx.Create(&record).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记录转账历史失败"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "转账成功"})
}

// GetTransferRecords 获取转账历史
// @Summary 获取划拨记录
// @Description 获取划拨记录
// @Tags Accounts
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_accounts_transfers
// @Router /api/accounts/transfers [get]
func GetTransferRecords(c *gin.Context) {
	var records []models.TransferRecord
	if err := config.DB.Preload("FromAccount").Preload("ToAccount").Order("created_at desc").Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": records})
}

// CalibrateAccountRequest 余额校准请求
type CalibrateAccountRequest struct {
	RealBalance float64 `json:"real_balance" binding:"min=0"`
	Remark      string  `json:"remark"`
}

// CalibrateAccount 余额校准：直接将账户余额设置为真实金额
// 自动生成一条差额的"收入补录"或"余额校准支出"记录，不需要手动追踪每笔个人消费
// @Summary 账户余额校准
// @Description 账户余额校准
// @Tags Accounts
// @Accept json
// @Produce json
// @Param body body CalibrateAccountRequest true "余额校准参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_accounts_id_calibrate
// @Router /api/accounts/{id}/calibrate [put]
func CalibrateAccount(c *gin.Context) {
	id := c.Param("id")

	var req CalibrateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var account models.Account
	if err := tx.First(&account, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "账户不存在"})
		return
	}

	diff := req.RealBalance - account.Balance
	oldBalance := account.Balance

	// 更新账户余额为真实余额
	if err := tx.Model(&account).UpdateColumn("balance", req.RealBalance).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "余额更新失败"})
		return
	}

	// 生成差额的财务支出/补录记录，用于审计留痕
	accountID := account.ID
	remark := req.Remark
	if remark == "" {
		remark = "系统余额校准（个人消费或未录入流水差额抹平）"
	}

	if diff != 0 {
		flowType := 4 // 余额校准 — 不计入收入 / 支出
		amount := diff
		if amount < 0 {
			amount = -amount
		}

		if err := WriteFinancialLog(tx, accountID, flowType, "系统校准", amount, req.RealBalance, "", remark+fmt.Sprintf("（原余额 ¥%.2f → 校准后 ¥%.2f，差额 %.2f）", oldBalance, req.RealBalance, diff)); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "校准记录写入失败"})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  fmt.Sprintf("校准成功：账户「%s」余额已更新为 ¥%.2f（差额 %+.2f）", account.Name, req.RealBalance, diff),
		"data": gin.H{"old_balance": oldBalance, "new_balance": req.RealBalance, "diff": diff},
	})
}
