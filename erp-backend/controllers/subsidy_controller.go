package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CreateSubsidySettlementRequest struct {
	Amount    float64 `json:"amount" binding:"required,gt=0"`
	AccountID uint    `json:"account_id" binding:"required"`
	Remark    string  `json:"remark"`
}

// CreateSubsidySettlement 录入国补回款记录
// @Summary 创建国补结算单
// @Description 创建国补结算单
// @Tags Subsidy
// @Accept json
// @Produce json
// @Param body body CreateSubsidySettlementRequest true "国补结算参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_subsidy_settle
// @Router /api/subsidy/settle [post]
func CreateSubsidySettlement(c *gin.Context) {
	var input CreateSubsidySettlementRequest

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. 增加账户余额 (使用行级锁)
	var account models.Account
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&account, input.AccountID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "账户查询失败"})
		return
	}

	if err := tx.Model(&account).UpdateColumn("balance", gorm.Expr("balance + ?", input.Amount)).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新账户余额失败"})
		return
	}

	// 2. 创建回款记录
	settlement := models.SubsidySettlement{
		Amount:    input.Amount,
		AccountID: input.AccountID,
		Remark:    input.Remark,
	}
	if err := tx.Create(&settlement).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建回款记录失败"})
		return
	}

	// 3. 写入财务流水
	if err := WriteFinancialLog(tx, input.AccountID, 1, "国补回款", input.Amount, account.Balance+input.Amount, fmt.Sprintf("SUB-%d", settlement.ID), input.Remark); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "写入财务流水失败"})
		return
	}

	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": settlement})
}

// GetSubsidySettlements 获取国补回款历史
// @Summary 获取国补结算列表
// @Description 获取国补结算列表
// @Tags Subsidy
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_subsidy_settlements
// @Router /api/subsidy/settlements [get]
func GetSubsidySettlements(c *gin.Context) {
	var list []models.SubsidySettlement
	if err := config.DB.Preload("Account").Order("created_at desc").Find(&list).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取记录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": list})
}

// GetSubsidySummary 获取国补概览数据
// @Summary 获取国补数据总览
// @Description 获取国补数据总览
// @Tags Subsidy
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_subsidy_summary
// @Router /api/subsidy/summary [get]
func GetSubsidySummary(c *gin.Context) {
	var totalGenerated float64
	var totalReceived float64

	// 累计应回款：新版字段与历史回退必须逐单计算，避免新旧订单混合时漏算历史订单。
	config.DB.Model(&models.Order{}).
		Where("order_status = 1 AND subsidy_amount > 0").
		Select("COALESCE(SUM(CASE WHEN subsidy_pending_amount > 0 THEN subsidy_pending_amount ELSE subsidy_amount END), 0)").
		Scan(&totalGenerated)

	// 累计回款：历次结算之和
	config.DB.Model(&models.SubsidySettlement{}).Select("COALESCE(SUM(amount), 0)").Scan(&totalReceived)
	pendingBalance := CalculatePendingSubsidy(totalGenerated, totalReceived)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"total_generated": totalGenerated,
			"total_received":  totalReceived,
			"pending_balance": pendingBalance,
		},
	})
}
