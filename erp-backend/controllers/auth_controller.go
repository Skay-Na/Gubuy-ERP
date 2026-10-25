package controllers

import (
	"erp-backend/config"
	"erp-backend/middlewares"
	"erp-backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type UpdateAdminPasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type VerifyAdminPasswordRequest struct {
	Password string `json:"password" binding:"required"`
}

type InitAdminRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// UpdateAdminPassword 修改管理员密码
// @Summary 更新管理员密码
// @Description 更新管理员密码
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body UpdateAdminPasswordRequest true "管理员密码修改参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID put_api_admin_password
// @Router /api/admin/password [put]
func UpdateAdminPassword(c *gin.Context) {
	var req UpdateAdminPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数错误")
		return
	}

	// 按当前登录用户定位管理员，避免多管理员场景下误改第一条记录
	currentUser := c.GetString("username")
	if currentUser == "" {
		FailUnauthorized(c, "未识别当前登录用户")
		return
	}
	var admin models.AdminUser
	if err := config.DB.Where("username = ?", currentUser).First(&admin).Error; err != nil {
		FailNotFound(c, "未找到当前登录的管理员账户")
		return
	}

	// 密码使用 bcrypt 对比（存量明文已在登录路径一次性升级，此处不再保留明文回退）
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(req.OldPassword)); err != nil {
		FailUnauthorized(c, "旧密码错误")
		return
	}

	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		FailInternal(c, "密码加密失败")
		return
	}

	if err := config.DB.Model(&admin).Update("password", string(hashedPassword)).Error; err != nil {
		FailInternal(c, "密码更新失败")
		return
	}

	OKWithMsg(c, "密码修改成功", nil)
}

// VerifyAdminPassword 门户入口管理员验证
// @Summary VerifyAdminPassword
// @Description VerifyAdminPassword
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body VerifyAdminPasswordRequest true "管理员登录参数"
// @Success 200 {object} APIResponse "成功响应"
// @ID post_api_admin_verify
// @Router /api/admin/verify [post]
func VerifyAdminPassword(c *gin.Context) {
	var req VerifyAdminPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数错误")
		return
	}

	var admin models.AdminUser
	if err := config.DB.First(&admin).Error; err != nil {
		FailNotFound(c, "未找到管理员账户")
		return
	}

	// 密码使用 bcrypt 对比（已确认 erp_db.admin_users 全部为 bcrypt 哈希，删除明文兼容回退）
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(req.Password)); err != nil {
		FailUnauthorized(c, "密码错误")
		return
	}

	// 生成 JWT Token
	token, err := middlewares.GenerateToken(admin.Username)
	if err != nil {
		FailInternal(c, "生成 Token 失败")
		return
	}

	OKWithMsg(c, "验证成功", gin.H{
		"token":    token,
		"username": admin.Username,
	})
}

// CheckAdminStatus 检查系统是否已初始化管理员
// @Summary CheckAdminStatus
// @Description CheckAdminStatus
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @ID get_api_admin_status
// @Router /api/admin/status [get]
func CheckAdminStatus(c *gin.Context) {
	var count int64
	if err := config.DB.Model(&models.AdminUser{}).Count(&count).Error; err != nil {
		FailInternal(c, "数据库查询失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":        200,
		"initialized": count > 0,
	})
}

// InitAdmin 初始化第一个管理员
// @Summary InitAdmin
// @Description InitAdmin
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body InitAdminRequest true "初始化管理员参数"
// @Success 200 {object} APIResponse "成功响应"
// @ID post_api_admin_init
// @Router /api/admin/init [post]
func InitAdmin(c *gin.Context) {
	var count int64
	config.DB.Model(&models.AdminUser{}).Count(&count)
	if count > 0 {
		FailBadRequest(c, "系统已初始化，不可重复操作")
		return
	}

	var req InitAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数错误")
		return
	}

	// 对初始密码进行加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		FailInternal(c, "密码加密失败")
		return
	}

	admin := models.AdminUser{
		Username: req.Username,
		Password: string(hashedPassword),
	}
	if err := config.DB.Create(&admin).Error; err != nil {
		FailInternal(c, "初始化管理员失败")
		return
	}

	OKWithMsg(c, "系统初始化成功", nil)
}
