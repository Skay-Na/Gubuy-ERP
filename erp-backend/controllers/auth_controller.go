package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// UpdateAdminPassword 修改管理员密码
func UpdateAdminPassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var admin models.AdminUser
	if err := config.DB.First(&admin).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "未找到管理员账户"})
		return
	}

	// 这里暂时使用明文对比（后续可引入 bcrypt）
	if admin.Password != req.OldPassword {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "旧密码错误"})
		return
	}

	if err := config.DB.Model(&admin).Update("password", req.NewPassword).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "密码更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "密码修改成功"})
}

// VerifyAdminPassword 门户入口管理员验证
func VerifyAdminPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var admin models.AdminUser
	if err := config.DB.First(&admin).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "未找到管理员账户"})
		return
	}

	if admin.Password != req.Password {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "密码错误"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "验证成功"})
}
