package controllers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type aiProviderSpec struct {
	Name         string `json:"name"`
	DefaultModel string `json:"default_model"`
	CNURL        string `json:"-"`
	GlobalURL    string `json:"-"`
	JSONMode     bool   `json:"-"`
}

var aiProviderCatalog = map[string]aiProviderSpec{
	"deepseek": {Name: "DeepSeek", DefaultModel: "deepseek-flash", CNURL: "https://api.deepseek.com/chat/completions", JSONMode: true},
	"minimax":  {Name: "MiniMax", DefaultModel: "MiniMax-M3", CNURL: "https://api.minimaxi.com/v1/chat/completions", GlobalURL: "https://api.minimax.io/v1/chat/completions"},
	"mimo":     {Name: "小米 MiMo", DefaultModel: "mimo-v2.6-flash", CNURL: "https://api.xiaomimimo.com/v1/chat/completions", JSONMode: true},
	"qwen":     {Name: "千问", DefaultModel: "qwen3-vl-plus", CNURL: "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", GlobalURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1/chat/completions", JSONMode: true},
}

var aiModelNamePattern = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,100}$`)

type aiProviderRuntime struct {
	Provider string
	Model    string
	APIKey   string
	URL      string
	JSONMode bool
}

func aiEncryptionKey() ([]byte, error) {
	secret := os.Getenv("AI_CONFIG_SECRET")
	if secret == "" {
		secret = os.Getenv("JWT_SECRET")
	}
	if len(secret) < 24 || secret == "erp_wonchon_default_secret_key_2026_please_change" || secret == "please_change_this_secret" {
		return nil, errors.New("请先在服务端配置至少 24 位的 AI_CONFIG_SECRET 或安全的 JWT_SECRET")
	}
	hash := sha256.Sum256([]byte("erpwonchon-ai-provider-key-v1:" + secret))
	return hash[:], nil
}

func encryptAIKey(plain string) (string, error) {
	key, err := aiEncryptionKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptAIKey(encoded string) (string, error) {
	key, err := aiEncryptionKey()
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < aead.NonceSize() {
		return "", errors.New("密钥数据无效")
	}
	plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], nil)
	if err != nil {
		return "", errors.New("无法解密 AI 密钥：当前服务端加密密钥与保存时不一致。请恢复原 AI_CONFIG_SECRET/JWT_SECRET，或在系统设置重新输入服务商 API Key 并保存")
	}
	return string(plain), nil
}

func aiProviderURL(provider, region string) string {
	spec := aiProviderCatalog[provider]
	if region == "global" && spec.GlobalURL != "" {
		return spec.GlobalURL
	}
	return spec.CNURL
}

func activeAIProvider() (aiProviderRuntime, error) {
	var runtime aiProviderRuntime
	if config.DB != nil {
		var saved models.AIProviderConfig
		err := config.DB.Where("active = ?", true).First(&saved).Error
		if err == nil {
			spec, ok := aiProviderCatalog[saved.Provider]
			if !ok {
				return runtime, errors.New("当前 AI 服务商无效")
			}
			key, err := decryptAIKey(saved.EncryptedKey)
			if err != nil {
				return runtime, err
			}
			return aiProviderRuntime{Provider: saved.Provider, Model: saved.Model, APIKey: key, URL: aiProviderURL(saved.Provider, saved.Region), JSONMode: spec.JSONMode}, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return runtime, err
		}
	}
	runtime = aiProviderRuntime{Provider: "env", APIKey: os.Getenv("AI_API_KEY"), Model: os.Getenv("AI_MODEL"), URL: os.Getenv("AI_API_URL"), JSONMode: true}
	if runtime.URL == "" {
		runtime.URL = "https://api.openai.com/v1/chat/completions"
	}
	if runtime.APIKey == "" || runtime.Model == "" {
		return runtime, errors.New("请在系统设置中配置并启用 AI 服务商")
	}
	return runtime, nil
}

type aiProviderSummary struct {
	Provider     string `json:"provider"`
	Name         string `json:"name"`
	Model        string `json:"model"`
	DefaultModel string `json:"default_model"`
	Region       string `json:"region"`
	HasKey       bool   `json:"has_key"`
	KeyUsable    bool   `json:"key_usable"`
	Active       bool   `json:"active"`
}

// GetAIProviderSettings returns presence flags but never credentials.
// @Summary 查询 AI 服务配置
// @Tags System
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID get_api_system_ai_providers
// @Router /api/system/ai/providers [get]
func GetAIProviderSettings(c *gin.Context) {
	var saved []models.AIProviderConfig
	if err := config.DB.Select("provider", "model", "region", "active", "encrypted_key").Find(&saved).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "AI 配置读取失败"})
		return
	}
	byProvider := make(map[string]models.AIProviderConfig, len(saved))
	active := "env"
	for _, item := range saved {
		byProvider[item.Provider] = item
		if item.Active {
			active = item.Provider
		}
	}
	providers := make([]aiProviderSummary, 0, 4)
	for _, id := range []string{"deepseek", "minimax", "mimo", "qwen"} {
		spec := aiProviderCatalog[id]
		item := byProvider[id]
		region := item.Region
		if region == "" {
			region = "cn"
		}
		model := item.Model
		if model == "" {
			model = spec.DefaultModel
		}
		_, keyError := decryptAIKey(item.EncryptedKey)
		providers = append(providers, aiProviderSummary{Provider: id, Name: spec.Name, Model: model, DefaultModel: spec.DefaultModel, Region: region, HasKey: item.EncryptedKey != "", KeyUsable: item.EncryptedKey != "" && keyError == nil, Active: item.Active})
	}
	_, encryptionError := aiEncryptionKey()
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{
		"providers":        providers,
		"active":           active,
		"env_configured":   os.Getenv("AI_API_KEY") != "" && os.Getenv("AI_MODEL") != "",
		"env_model":        os.Getenv("AI_MODEL"),
		"encryption_ready": encryptionError == nil,
	}})
}

type saveAIProviderRequest struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
	Region string `json:"region"`
}

// SaveAIProviderSettings creates or updates one encrypted provider credential.
// @Summary 保存 AI 服务配置
// @Tags System
// @Accept json
// @Produce json
// @Param provider path string true "服务商"
// @Param body body saveAIProviderRequest true "配置"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID put_api_system_ai_providers_provider
// @Router /api/system/ai/providers/{provider} [put]
func SaveAIProviderSettings(c *gin.Context) {
	provider := c.Param("provider")
	spec, ok := aiProviderCatalog[provider]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "不支持的 AI 服务商"})
		return
	}
	var req saveAIProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "配置格式无效"})
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.Model == "" {
		req.Model = spec.DefaultModel
	}
	if !aiModelNamePattern.MatchString(req.Model) || len(req.APIKey) > 4096 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "模型名称或密钥长度无效"})
		return
	}
	if req.Region == "" {
		req.Region = "cn"
	}
	if req.Region != "cn" && (req.Region != "global" || spec.GlobalURL == "") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "不支持的服务区域"})
		return
	}
	var existing models.AIProviderConfig
	err := config.DB.Where("provider = ?", provider).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取配置失败"})
		return
	}
	if req.APIKey == "" && existing.EncryptedKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请填写 API Key"})
		return
	}
	if req.APIKey == "" {
		if _, err := decryptAIKey(existing.EncryptedKey); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "旧密钥无法解密，请重新输入 API Key"})
			return
		}
	}
	if req.APIKey != "" {
		encrypted, err := encryptAIKey(req.APIKey)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": err.Error()})
			return
		}
		existing.EncryptedKey = encrypted
	}
	existing.Provider = provider
	existing.Model = req.Model
	existing.Region = req.Region
	if err := config.DB.Save(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "配置保存失败"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": fmt.Sprintf("%s 配置已保存", spec.Name)})
}

type activateAIProviderRequest struct {
	Provider string `json:"provider" binding:"required"`
}

// ActivateAIProvider selects the provider used by image import.
// @Summary 切换 AI 服务商
// @Tags System
// @Accept json
// @Produce json
// @Param body body activateAIProviderRequest true "服务商"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID put_api_system_ai_active
// @Router /api/system/ai/active [put]
func ActivateAIProvider(c *gin.Context) {
	var req activateAIProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请选择 AI 服务商"})
		return
	}
	if req.Provider != "env" {
		if _, ok := aiProviderCatalog[req.Provider]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "不支持的 AI 服务商"})
			return
		}
	}
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		if req.Provider != "env" {
			var target models.AIProviderConfig
			if err := tx.Where("provider = ?", req.Provider).First(&target).Error; err != nil || target.EncryptedKey == "" {
				return errors.New("请先保存该服务商的 API Key")
			}
			if _, err := decryptAIKey(target.EncryptedKey); err != nil {
				return errors.New("该服务商密钥无法解密，请重新输入 API Key")
			}
		}
		if err := tx.Model(&models.AIProviderConfig{}).Where("active = ?", true).Update("active", false).Error; err != nil {
			return err
		}
		if req.Provider == "env" {
			return nil
		}
		return tx.Model(&models.AIProviderConfig{}).Where("provider = ?", req.Provider).Update("active", true).Error
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已切换 AI 服务商"})
}

// DeleteAIProviderKey removes a saved key and returns to env fallback if active.
// @Summary 删除 AI 服务密钥
// @Tags System
// @Produce json
// @Param provider path string true "服务商"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID delete_api_system_ai_providers_provider
// @Router /api/system/ai/providers/{provider} [delete]
func DeleteAIProviderKey(c *gin.Context) {
	provider := c.Param("provider")
	if _, ok := aiProviderCatalog[provider]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "不支持的 AI 服务商"})
		return
	}
	if err := config.DB.Where("provider = ?", provider).Delete(&models.AIProviderConfig{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "删除密钥失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "密钥已删除"})
}
