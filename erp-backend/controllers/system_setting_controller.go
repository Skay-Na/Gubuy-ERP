package controllers

import (
	"net/http"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
)

// allowedSystemSettings 可被接口读写的设置键白名单。
var allowedSystemSettings = map[string]bool{
	models.SettingOrderBackdateEnabled: true,
}

// GetSystemSettings 读取系统设置（管理员与员工均可读）。
// GET /api/system-settings?keys=a,b；不带 keys 返回全部白名单项。
func GetSystemSettings(c *gin.Context) {
	query := config.DB.Where("setting_key IN ?", mapKeys(allowedSystemSettings))
	if keys := c.Query("keys"); keys != "" {
		requested := splitAndTrim(keys)
		filtered := make([]string, 0, len(requested))
		for _, k := range requested {
			if allowedSystemSettings[k] {
				filtered = append(filtered, k)
			}
		}
		if len(filtered) == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 200, "data": []models.SystemSetting{}})
			return
		}
		query = config.DB.Where("setting_key IN ?", filtered)
	}
	var settings []models.SystemSetting
	if err := query.Find(&settings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取系统设置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": settings})
}

// UpdateSystemSetting 更新系统设置（仅管理员）。
// PUT /api/system-settings/:key  body: {"value": "true", "remark": "可选"}
func UpdateSystemSetting(c *gin.Context) {
	key := c.Param("key")
	if !allowedSystemSettings[key] {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "不支持的设置项"})
		return
	}
	var req struct {
		Value  string `json:"value" binding:"required,max=255"`
		Remark string `json:"remark" binding:"max=255"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	var setting models.SystemSetting
	err := config.DB.Where("setting_key = ?", key).First(&setting).Error
	if err != nil {
		setting = models.SystemSetting{SettingKey: key, Value: req.Value, Remark: req.Remark}
		if err := config.DB.Create(&setting).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存设置失败"})
			return
		}
	} else {
		updates := map[string]interface{}{"value": req.Value}
		if req.Remark != "" {
			updates["remark"] = req.Remark
		}
		if err := config.DB.Model(&setting).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存设置失败"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保存", "data": setting})
}

// IsOrderBackdateEnabledForEmployee 员工 token 开单时是否允许补录开单时间。
func IsOrderBackdateEnabledForEmployee() bool {
	var setting models.SystemSetting
	if err := config.DB.Where("setting_key = ?", models.SettingOrderBackdateEnabled).First(&setting).Error; err != nil {
		return false
	}
	return models.IsTruthyValue(setting.Value)
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func splitAndTrim(s string) []string {
	out := make([]string, 0)
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	return out
}
