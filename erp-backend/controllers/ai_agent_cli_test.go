//go:build integration

package controllers

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"erp-backend/config"
	"erp-backend/middlewares"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// TestERPCLIDebugWorkflow exercises the built CLI against real handlers and a stub model.
func TestERPCLIDebugWorkflow(t *testing.T) {
	seedAgentSalesComparison(t)
	admins := []models.AdminUser{}
	if err := config.DB.Find(&admins).Error; err != nil {
		t.Fatal(err)
	}
	if err := config.DB.Where("1=1").Delete(&models.AdminUser{}).Error; err != nil {
		t.Fatal(err)
	}
	const owner = "cli_integration_admin"
	t.Cleanup(func() {
		if err := config.DB.Where("1=1").Delete(&models.AdminUser{}).Error; err != nil {
			t.Error(err)
		}
		for _, admin := range admins {
			if err := config.DB.Create(&admin).Error; err != nil {
				t.Error(err)
			}
		}
		sessions := []models.AIAgentSession{}
		if err := config.DB.Where("owner = ?", owner).Find(&sessions).Error; err != nil {
			t.Error(err)
		}
		for _, session := range sessions {
			if err := config.DB.Where("session_id = ?", session.ID).Delete(&models.AIAgentMessage{}).Error; err != nil {
				t.Error(err)
			}
			if err := config.DB.Delete(&session).Error; err != nil {
				t.Error(err)
			}
		}
	})
	password, err := bcrypt.GenerateFromPassword([]byte("cli-test-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	admin := models.AdminUser{Username: owner, Password: string(password)}
	if err := config.DB.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	script := &agentMockScript{Task: func(string) agentTask { return agentSalesTask() }}
	runtime := agentMockRuntime(t, script)
	t.Setenv("AI_API_KEY", runtime.APIKey)
	t.Setenv("AI_MODEL", runtime.Model)
	t.Setenv("AI_API_URL", runtime.URL)
	if err := config.DB.Where("1=1").Delete(&models.AIProviderConfig{}).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/api/admin/verify", VerifyAdminPassword)
	group := router.Group("/api", middlewares.AdminAuthRequired())
	group.GET("/system/ai/providers", GetAIProviderSettings)
	group.POST("/ai/agent/sessions", CreateAIAgentSession)
	group.GET("/ai/agent/sessions/:id", GetAIAgentSession)
	group.POST("/ai/agent/sessions/:id/messages", SendAIAgentMessage)
	group.GET("/ai/agent/messages/:message_id/trace", GetAIAgentMessageTrace)
	server := httptest.NewServer(router)
	defer server.Close()
	dir := t.TempDir()
	binary := filepath.Join(dir, "erpctl")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../cmd/erpctl")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v %s", err, output)
	}
	t.Setenv("ERP_TOKEN", "")
	t.Setenv("ERP_URL", server.URL)
	t.Setenv("ERP_CONFIG_DIR", filepath.Join(dir, "credentials"))
	runCLI := func(args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), binary, args...)
		if len(args) > 1 && args[1] == "login" {
			cmd.Stdin = strings.NewReader("cli-test-password\n")
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("CLI %v failed: %v %s", args, err, out)
		}
		if strings.Contains(string(out), "cli-test-password") {
			t.Fatal("password leaked")
		}
		return out
	}
	runCLI("auth", "login", "--password-stdin", "--json")
	runCLI("auth", "status", "--json")
	answer := runCLI("agent", "ask", "8月和9月销售情况对比", "--json")
	var d struct {
		Debug   bool                  `json:"debug"`
		Message models.AIAgentMessage `json:"message"`
	}
	if err := json.Unmarshal(answer, &d); err != nil {
		t.Fatal(err)
	}
	if !d.Debug || d.Message.Status != "complete" {
		t.Fatalf("answer=%s", answer)
	}
	cases := filepath.Join(dir, "cases.json")
	body := `[{"name":"sales","question":"8月和9月销售情况对比","assertions":{
 "topic":"sales","metrics":["sales_revenue","order_count"],
 "periods":[{"start":"2026-08-01","end":"2026-08-31"},{"start":"2026-09-01","end":"2026-09-30"}],
 "values":[{"metric":"sales_revenue","start":"2026-08-01","end":"2026-08-31","value":900,"tolerance":0.01},
 {"metric":"sales_revenue","start":"2026-09-01","end":"2026-09-30","value":1900,"tolerance":0.01}]}}]`
	if err := os.WriteFile(cases, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	report := runCLI("agent", "eval", "--cases", cases, "--json")
	var summary struct {
		Passed int `json:"passed"`
		Failed int `json:"failed"`
	}
	if err := json.Unmarshal(report, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 1 || summary.Failed != 0 || script.MemoryCalls != 0 {
		t.Fatalf("report=%s memory=%d", report, script.MemoryCalls)
	}
	runCLI("auth", "logout", "--json")
}
