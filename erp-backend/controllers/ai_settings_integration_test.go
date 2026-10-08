package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
)

func TestAIProviderSettingsSaveActivateAndHideKey(t *testing.T) {
	t.Setenv("AI_CONFIG_SECRET", "integration-secret-with-at-least-24-characters")
	if err := config.DB.Where("1 = 1").Delete(&models.AIProviderConfig{}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { config.DB.Where("1 = 1").Delete(&models.AIProviderConfig{}) })
	router := gin.New()
	router.PUT("/providers/:provider", SaveAIProviderSettings)
	router.GET("/providers", GetAIProviderSettings)
	router.PUT("/active", ActivateAIProvider)

	call := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		payload, _ := json.Marshal(body)
		request := httptest.NewRequest(method, path, bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, request)
		return writer
	}
	if response := call(http.MethodPut, "/providers/deepseek", gin.H{"api_key": "sk-secret-value", "model": "deepseek-flash", "region": "cn"}); response.Code != http.StatusOK {
		t.Fatalf("save status %d: %s", response.Code, response.Body.String())
	}
	var stored models.AIProviderConfig
	if err := config.DB.Where("provider = ?", "deepseek").First(&stored).Error; err != nil || stored.EncryptedKey == "sk-secret-value" || stored.EncryptedKey == "" {
		t.Fatalf("provider key was not encrypted: %v", err)
	}
	listing := call(http.MethodGet, "/providers", nil)
	if listing.Code != http.StatusOK || strings.Contains(listing.Body.String(), "sk-secret-value") || !strings.Contains(listing.Body.String(), `"key_usable":true`) {
		t.Fatalf("unsafe settings response: %s", listing.Body.String())
	}
	if response := call(http.MethodPut, "/active", gin.H{"provider": "deepseek"}); response.Code != http.StatusOK {
		t.Fatalf("activate status %d: %s", response.Code, response.Body.String())
	}
	runtime, err := activeAIProvider()
	if err != nil || runtime.APIKey != "sk-secret-value" || runtime.URL != aiProviderURL("deepseek", "cn") {
		t.Fatalf("active runtime = %+v, %v", runtime, err)
	}
	t.Setenv("AI_API_KEY", "legacy-env-key")
	t.Setenv("AI_MODEL", "legacy-vision-model")
	if response := call(http.MethodPut, "/active", gin.H{"provider": "env"}); response.Code != http.StatusOK {
		t.Fatalf("restore env status %d: %s", response.Code, response.Body.String())
	}
	runtime, err = activeAIProvider()
	if err != nil || runtime.Provider != "env" || runtime.APIKey != "legacy-env-key" {
		t.Fatalf("env fallback = %+v, %v", runtime, err)
	}
}
