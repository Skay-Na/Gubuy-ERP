//go:build integration

package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
)

func TestDebugSessionDoesNotLearnOrRecallMemory(t *testing.T) {
	for _, tt := range []struct {
		name, question string
		explicit       bool
	}{
		{name: "explicit", question: "请记住我喜欢简短回答", explicit: true},
		{name: "standalone", question: "以后回答请先给结论"},
		{name: "automatic", question: "我喜欢简短回答，8月和9月销售情况对比"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			seedAgentSalesComparison(t)
			owner := fmt.Sprintf("debug-memory-%d", time.Now().UnixNano())
			marker := "不应召回的测试专属长期偏好"
			if _, err := saveAgentMemory(owner, "preference", marker, "manual"); err != nil {
				t.Fatal(err)
			}
			script := &agentMockScript{
				Task: func(string) agentTask { return agentSalesTask() },
				MemoryCandidates: []agentMemoryCandidate{{
					Kind: "preference", Content: "用户喜欢简短回答", Evidence: tt.question, Confidence: 0.99,
				}},
			}
			runtime := agentMockRuntime(t, script)
			t.Setenv("AI_API_KEY", runtime.APIKey)
			t.Setenv("AI_MODEL", runtime.Model)
			t.Setenv("AI_API_URL", runtime.URL)
			if err := config.DB.Where("1=1").Delete(&models.AIProviderConfig{}).Error; err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("username", owner) })
			router.POST("/sessions", CreateAIAgentSession)
			router.GET("/sessions/:id", GetAIAgentSession)
			router.POST("/sessions/:id/messages", SendAIAgentMessage)
			router.GET("/trace/:message_id", GetAIAgentMessageTrace)
			response := memoryTestRequest(router, "POST", "/sessions", owner, map[string]bool{"debug": true})
			if response.Code != 200 {
				t.Fatal(response.Body.String())
			}
			var created struct {
				Data models.AIAgentSession `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			if !created.Data.Debug {
				t.Fatal("debug session was not marked")
			}
			session := created.Data
			t.Cleanup(func() {
				for _, model := range []any{&models.AIAgentMemory{}, &models.AIAgentMemoryState{}} {
					if err := config.DB.Where("owner = ?", owner).Delete(model).Error; err != nil {
						t.Error(err)
					}
				}
				if err := config.DB.Where("session_id = ?", session.ID).Delete(&models.AIAgentMessage{}).Error; err != nil {
					t.Error(err)
				}
				if err := config.DB.Delete(&session).Error; err != nil {
					t.Error(err)
				}
			})
			response = memoryTestRequest(
				router,
				http.MethodPost,
				fmt.Sprintf("/sessions/%d/messages", session.ID),
				owner,
				agentSendInput{Message: tt.question},
			)
			if response.Code != 200 || script.MemoryCalls != 0 {
				t.Fatalf("response=%s memory calls=%d", response.Body.String(), script.MemoryCalls)
			}
			for _, system := range script.Systems {
				if strings.Contains(system, marker) {
					t.Fatal("debug session recalled memory")
				}
			}
			if !tt.explicit && script.TaskCalls == 0 {
				t.Fatal("standalone/automatic message bypassed task execution")
			}
			var memories []models.AIAgentMemory
			if err := config.DB.Where("owner = ?", owner).Find(&memories).Error; err != nil {
				t.Fatal(err)
			}
			if len(memories) != 1 || memories[0].Content != marker {
				t.Fatalf("persistent memory changed: %+v", memories)
			}
			var answer struct {
				Data struct {
					Message models.AIAgentMessage `json:"message"`
				} `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
				t.Fatal(err)
			}
			response = memoryTestRequest(router, "GET", fmt.Sprintf("/trace/%d", answer.Data.Message.ID), owner, nil)
			if response.Code != 200 || !strings.Contains(response.Body.String(), `"debug":true`) {
				t.Fatalf("trace missing debug marker: %s", response.Body.String())
			}
		})
	}
}

func TestDebugSessionBlocksDrafts(t *testing.T) {
	setupAudit(t)
	session := models.AIAgentSession{Owner: "debug-draft-owner", Debug: true}
	if err := config.DB.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := config.DB.Delete(&session).Error; err != nil {
			t.Error(err)
		}
	})
	for _, tool := range []string{"draft_expense", "draft_order"} {
		_, _, action, err := agentExecuteTool(t.Context(), session, agentPlan{Tool: tool, Amount: 100, Category: "其他"})
		if err == nil || action != nil {
			t.Fatalf("debug draft allowed: %s", tool)
		}
	}
	var count int64
	err := config.DB.Model(&models.AIAgentAction{}).Where("session_id = ?", session.ID).Count(&count).Error
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("debug session created action")
	}
}
