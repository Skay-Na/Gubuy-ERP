package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetDefaultExpenseAccount 设置默认支出账户
// @Summary 设置默认支出账户
// @Description 将指定账户设为默认支出账户（全局唯一）
// @Tags Accounts
// @Accept json
// @Produce json
// @Param id path int true "账户ID"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID put_api_accounts_id_set_default_expense
// @Router /api/accounts/{id}/set-default-expense [put]
func SetDefaultExpenseAccount(c *gin.Context) {
	id := c.Param("id")

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var targetAcc models.Account
	if err := tx.First(&targetAcc, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "账户不存在",
		})
		return
	}

	// 将所有其他账户设为 false
	if err := tx.Model(&models.Account{}).Where("id != ?", id).Update("is_default_expense", false).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "重置旧默认账户失败: " + err.Error(),
		})
		return
	}

	// 将当前账户设为 true
	if err := tx.Model(&targetAcc).Update("is_default_expense", true).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "设置新默认账户失败: " + err.Error(),
		})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "设置默认账户成功",
	})
}
