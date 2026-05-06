package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ResetSystem 恢复出厂设置 - 清空所有业务数据
func ResetSystem(c *gin.Context) {
	// 这里可以加一个简单的校验，或者从 Header 传一个 RESET 标记
	var req struct {
		Confirm string `json:"confirm" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Confirm != "RESET" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请正确输入确认码"})
		return
	}

	// 开启事务执行清空
	tx := config.DB.Begin()

	// 清空所有业务表 (按照依赖关系逆序删除或直接 TRUNCATE)
	// 注意：MySQL 中如果有外键约束，TRUNCATE 可能会失败，可以使用 DELETE
	tables := []interface{}{
		&models.FinancialLog{},
		&models.InventoryLog{},
		&models.TransferRecord{},
		&models.StocktakeRecord{},
		&models.InboundRecord{},
		&models.Expense{},
		&models.OrderItem{},
		&models.Order{},
		&models.Product{},
		&models.Category{},
		&models.MonthlyAttendance{},
		&models.Employee{},
		&models.Account{},
		&models.AdminUser{},
	}

	for _, table := range tables {
		if err := tx.Unscoped().Where("1 = 1").Delete(table).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "系统重置失败: " + err.Error()})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "系统已成功恢复出厂设置"})
}
