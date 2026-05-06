package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WriteFinancialLog 在事务中写入一条财务流水记录（公用辅助函数）
// tx: 数据库事务
// accountID: 资金账户ID
// flowType: 1-收入, 2-支出, 3-内部调拨, 4-余额校准
// category: 业务分类(如: 销售货款, 采购货款, 盘亏损耗, 工资等)
// amount: 变动金额(绝对值)
// balanceAfter: 变动后账户余额快照
// relatedNo: 关联单号
// remark: 备注
func WriteFinancialLog(tx *gorm.DB, accountID uint, flowType int, category string, amount float64, balanceAfter float64, relatedNo string, remark string) error {
	log := models.FinancialLog{
		AccountID:    accountID,
		FlowType:     flowType,
		Category:     category,
		Amount:       amount,
		BalanceAfter: balanceAfter,
		RelatedNo:    relatedNo,
		Remark:       remark,
	}
	return tx.Create(&log).Error
}

// GetFinancialLogs 查询全口径财务流水
// GET /api/finance/logs?account_id=&flow_type=&category=&page=&page_size=
func GetFinancialLogs(c *gin.Context) {
	accountID := c.Query("account_id")
	flowType := c.Query("flow_type")
	category := c.Query("category")
	
	query := config.DB.
		Preload("Account").
		Order("created_at desc")

	if accountID != "" {
		query = query.Where("account_id = ?", accountID)
	}
	if flowType != "" {
		query = query.Where("flow_type = ?", flowType)
	}
	if category != "" {
		query = query.Where("category = ?", category)
	}

	var total int64
	query.Model(&models.FinancialLog{}).Count(&total)

	var offset int
	var limit int
	offset = 0
	limit = 200 // 返回最近的 200 条流水

	var logs []models.FinancialLog
	if err := query.Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询财务流水失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  200,
		"total": total,
		"data":  logs,
	})
}
