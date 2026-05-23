package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CreateSubsidySettlement 录入国补回款记录
func CreateSubsidySettlement(c *gin.Context) {
	var input struct {
		Amount    float64 `json:"amount" binding:"required,gt=0"`
		AccountID uint    `json:"account_id" binding:"required"`
		Remark    string  `json:"remark"`
	}

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
func GetSubsidySettlements(c *gin.Context) {
	var list []models.SubsidySettlement
	if err := config.DB.Preload("Account").Order("created_at desc").Find(&list).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取记录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": list})
}

// GetSubsidySummary 获取国补概览数据
func GetSubsidySummary(c *gin.Context) {
	var totalGenerated float64
	var totalReceived float64

	// 累计产生：所有正常订单的国补金额之和
	config.DB.Model(&models.Order{}).Where("order_status = 1").Select("COALESCE(SUM(subsidy_amount), 0)").Scan(&totalGenerated)

	// 累计回款：历次结算之和
	config.DB.Model(&models.SubsidySettlement{}).Select("COALESCE(SUM(amount), 0)").Scan(&totalReceived)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"total_generated": totalGenerated,
			"total_received":  totalReceived,
			"pending_balance": totalGenerated - totalReceived,
		},
	})
}
