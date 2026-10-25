package controllers

import (
	"erp-backend/config"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// ResetOrderCounters 清理测试数据并恢复计数器
// @Summary 清理订单每日计数器
// @Description 仅在开发/测试环境可用，重置订单号计数器
// @Tags System
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_dev_reset_order_counters
func ResetOrderCounters(c *gin.Context) {
	env := os.Getenv("APP_ENV")
	if env != "development" && env != "test" {
		c.JSON(http.StatusForbidden, gin.H{
			"code": 403,
			"msg":  "该接口仅在开发或测试环境下可用",
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 清理当日测试订单产生的计数器
	if err := tx.Exec("TRUNCATE TABLE order_daily_counters").Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "清理计数器失败: " + err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "测试数据和订单计数器已重置",
	})
}
