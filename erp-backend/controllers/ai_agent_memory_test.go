package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
)

func memoryTestRequest(router *gin.Engine, method, path, owner string, body any) *httptest.ResponseRecorder {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Owner", owner)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestAgentAutomaticMemoryInfersWithoutRememberCommand(t *testing.T) {
	setupAudit(t)
	owner := fmt.Sprintf("memory-auto-%d", time.Now().UnixNano())
	question := "我喜欢简短回答，先给结论"
	script := &agentMockScript{MemoryCandidates: []agentMemoryCandidate{{Kind: "preference", Content: "用户偏好简短回答并先给结论", Evidence: question, Confidence: 0.95}}}
	memories := agentInferMemories(context.Background(), agentMockRuntime(t, script), owner, question, nil)
	if len(memories) != 1 || memories[0].Source != "inferred" || memories[0].Evidence != question {
		t.Fatalf("automatic inference failed: %+v", memories)
	}
	recalled, err := agentRecallMemory(owner, "工资报表")
	if err != nil || !strings.Contains(recalled, "先给结论") {
		t.Fatalf("inference not recalled: %q, %v", recalled, err)
	}
}

func TestAgentAutomaticMemoryRejectsUnfoundedAndTemporaryCandidates(t *testing.T) {
	setupAudit(t)
	owner := fmt.Sprintf("memory-reject-%d", time.Now().UnixNano())
	question := "我喜欢简短回答，先给结论"
	script := &agentMockScript{MemoryCandidates: []agentMemoryCandidate{
		{Kind: "profile", Content: "用户是老板", Evidence: "我是老板", Confidence: 0.99},
		{Kind: "context", Content: "本月工资一万元", Evidence: question, Confidence: 0.99},
	}}
	if memories := agentInferMemories(context.Background(), agentMockRuntime(t, script), owner, question, nil); len(memories) != 0 {
		t.Fatalf("invalid candidates persisted: %+v", memories)
	}
}

func TestAgentAutomaticMemoryReplacementRespectsOwnerAndManualEntries(t *testing.T) {
	setupAudit(t)
	owner := fmt.Sprintf("memory-replace-%d", time.Now().UnixNano())
	manual, err := saveAgentMemory(owner, "preference", "回答要详细", "manual")
	if err != nil {
		t.Fatal(err)
	}
	candidate := agentMemoryCandidate{Kind: "preference", Content: "用户喜欢简洁回答", Evidence: "我喜欢简洁回答", Confidence: 0.95, ReplaceID: manual.ID}
	if _, _, err := saveInferredAgentMemory(owner, candidate); err == nil {
		t.Fatal("manual entry overwritten")
	}
	candidate.ReplaceID = 0
	inferred, _, err := saveInferredAgentMemory(owner, candidate)
	if err != nil {
		t.Fatal(err)
	}
	candidate.ReplaceID = inferred.ID
	candidate.Content = "用户喜欢先给结论再展开解释"
	if _, _, err := saveInferredAgentMemory(owner+"-other", candidate); err == nil {
		t.Fatal("other owner's entry overwritten")
	}
	if updated, changed, err := saveInferredAgentMemory(owner, candidate); err != nil || !changed || updated.ID != inferred.ID {
		t.Fatalf("inferred entry not updated: %+v, %v", updated, err)
	}
}

func TestAgentAutomaticMemoryDoesNotUseFailedOrDraftHistory(t *testing.T) {
	draft := uint(1)
	recent := []models.AIAgentMessage{
		{Role: "user", Content: "我负责财务"}, {Role: "assistant", Status: "incomplete"},
		{Role: "user", Content: "我喜欢采购草稿"}, {Role: "assistant", Status: "complete", ActionID: &draft},
		{Role: "user", Content: "我负责门店经营"}, {Role: "assistant", Status: "complete"},
	}
	lines := agentMemoryUserLines(recent, "我喜欢先给结论")
	if len(lines) != 2 || strings.Contains(strings.Join(lines, " "), "财务") || strings.Contains(strings.Join(lines, " "), "采购草稿") {
		t.Fatalf("invalid history included: %v", lines)
	}
}

func TestAgentAutomaticMemoryDeletionExcludesEarlierQuotes(t *testing.T) {
	setupAudit(t)
	owner := fmt.Sprintf("memory-forget-%d", time.Now().UnixNano())
	state := models.AIAgentMemoryState{Owner: owner, ForgotBefore: time.Now()}
	if err := config.DB.Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	recent := []models.AIAgentMessage{{Role: "user", Content: "我喜欢简短回答", CreatedAt: time.Now().Add(-time.Hour)}, {Role: "assistant", Status: "complete", CreatedAt: time.Now().Add(-time.Hour)}}
	script := &agentMockScript{MemoryCandidates: []agentMemoryCandidate{{Kind: "preference", Content: "用户喜欢简短回答", Evidence: "我喜欢简短回答", Confidence: 0.95}}}
	memories := agentInferMemories(context.Background(), agentMockRuntime(t, script), owner, "我希望回答自然一些", recent)
	if len(memories) != 0 {
		t.Fatalf("forgotten old preference relearned: %+v", memories)
	}
}

func TestAgentAutomaticMemoryStandalonePreferenceEndpoint(t *testing.T) {
	setupAudit(t)
	owner := fmt.Sprintf("memory-endpoint-%d", time.Now().UnixNano())
	session := models.AIAgentSession{Owner: owner}
	if err := config.DB.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	question := "以后回答请先给结论"
	script := &agentMockScript{MemoryCandidates: []agentMemoryCandidate{{Kind: "preference", Content: "用户希望回答先给结论", Evidence: question, Confidence: 0.95}}}
	runtime := agentMockRuntime(t, script)
	t.Setenv("AI_API_KEY", runtime.APIKey)
	t.Setenv("AI_MODEL", runtime.Model)
	t.Setenv("AI_API_URL", runtime.URL)
	if err := config.DB.Where("1=1").Delete(&models.AIProviderConfig{}).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("username", owner) })
	router.POST("/sessions/:id/messages", SendAIAgentMessage)
	response := memoryTestRequest(router, http.MethodPost, fmt.Sprintf("/sessions/%d/messages", session.ID), owner, agentSendInput{Message: question})
	if response.Code != 200 || !strings.Contains(response.Body.String(), "memory_updates") || script.TaskCalls != 0 || script.MemoryCalls != 1 {
		t.Fatalf("natural preference not accepted: %s", response.Body.String())
	}
	response = memoryTestRequest(router, http.MethodPost, fmt.Sprintf("/sessions/%d/messages", session.ID), owner, agentSendInput{Message: question})
	if response.Code != 200 || !strings.Contains(response.Body.String(), "已保存在长期记忆") || script.TaskCalls != 0 {
		t.Fatalf("repeated preference not acknowledged: %s", response.Body.String())
	}
}

func TestAgentAutomaticMemorySkipsFailedAndDraftResponses(t *testing.T) {
	for _, test := range []struct {
		name, question string
		draft          bool
	}{
		{name: "failed", question: "我喜欢简短回答，8月和9月销售情况对比"},
		{name: "draft", question: "我希望新增一笔支出草稿", draft: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			seedAgentSalesComparison(t)
			owner := fmt.Sprintf("memory-skip-%d", time.Now().UnixNano())
			session := models.AIAgentSession{Owner: owner}
			if err := config.DB.Create(&session).Error; err != nil {
				t.Fatal(err)
			}
			script := &agentMockScript{EmptyAnswer: true, Task: func(string) agentTask { return agentSalesTask() }}
			if test.draft {
				script.Task = func(string) agentTask { return agentTask{Goal: "新增支出", Topic: "write", Operation: "write"} }
				script.Plans = []agentPlan{{Tool: "draft_expense", Amount: 100, Category: "其他"}}
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
			router.POST("/sessions/:id/messages", SendAIAgentMessage)
			response := memoryTestRequest(router, http.MethodPost, fmt.Sprintf("/sessions/%d/messages", session.ID), owner, agentSendInput{Message: test.question})
			if response.Code != 200 || script.MemoryCalls != 0 {
				t.Fatalf("unconfirmed outcome learned: %s calls=%d", response.Body.String(), script.MemoryCalls)
			}
		})
	}
}

func TestAgentPersistentMemoryOwnerIsolationAndManagement(t *testing.T) {
	setupAudit(t)
	owner := fmt.Sprintf("memory-owner-%d", time.Now().UnixNano())
	other := owner + "-other"
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("username", c.GetHeader("X-Test-Owner")) })
	router.GET("/memories", ListAIAgentMemories)
	router.POST("/memories", CreateAIAgentMemory)
	router.PUT("/memories/:memory_id", UpdateAIAgentMemory)
	router.DELETE("/memories/:memory_id", DeleteAIAgentMemory)

	created := memoryTestRequest(router, http.MethodPost, "/memories", owner, agentMemoryInput{Kind: "preference", Content: "回答销售问题时先给结论"})
	if created.Code != http.StatusOK {
		t.Fatalf("create: %s", created.Body.String())
	}
	var result struct {
		Data models.AIAgentMemory `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	id := result.Data.ID
	path := fmt.Sprintf("/memories/%d", id)
	if response := memoryTestRequest(router, http.MethodGet, "/memories", other, nil); response.Code != http.StatusOK || strings.Contains(response.Body.String(), "回答销售") {
		t.Fatalf("other owner can read memory: %s", response.Body.String())
	}
	if response := memoryTestRequest(router, http.MethodPut, path, other, agentMemoryInput{Kind: "profile", Content: "伪造资料"}); response.Code != http.StatusNotFound {
		t.Fatalf("other owner can update memory: %s", response.Body.String())
	}
	if response := memoryTestRequest(router, http.MethodDelete, path, other, nil); response.Code != http.StatusNotFound {
		t.Fatalf("other owner can delete memory: %s", response.Body.String())
	}
	if response := memoryTestRequest(router, http.MethodPut, path, owner, agentMemoryInput{Kind: "preference", Content: "回答工资问题时先给结论"}); response.Code != http.StatusOK {
		t.Fatalf("update: %s", response.Body.String())
	}
	if recalled, err := agentRecallMemory(owner, "工资情况"); err != nil || !strings.Contains(recalled, "回答工资问题") {
		t.Fatalf("updated memory not recalled: %q, %v", recalled, err)
	}
	if response := memoryTestRequest(router, http.MethodDelete, path, owner, nil); response.Code != http.StatusOK {
		t.Fatalf("delete: %s", response.Body.String())
	}
	if recalled, err := agentRecallMemory(owner, "工资情况"); err != nil || recalled != "" {
		t.Fatalf("deleted memory recalled: %q, %v", recalled, err)
	}
}

func TestAgentPersistentMemoryExplicitSaveAndRetrieval(t *testing.T) {
	setupAudit(t)
	owner := fmt.Sprintf("memory-chat-%d", time.Now().UnixNano())
	session := models.AIAgentSession{Owner: owner}
	if err := config.DB.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("username", owner) })
	router.POST("/sessions/:id/messages", SendAIAgentMessage)
	response := memoryTestRequest(router, http.MethodPost, fmt.Sprintf("/sessions/%d/messages", session.ID), owner, agentSendInput{Message: "记住：我喜欢回答简短，先给结论"})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "已记住") {
		t.Fatalf("explicit save: %s", response.Body.String())
	}
	recalled, err := agentRecallMemory(owner, "9月销售额是多少")
	if err != nil || !strings.Contains(recalled, "先给结论") {
		t.Fatalf("preference not available across sessions: %q, %v", recalled, err)
	}
	if other, err := agentRecallMemory(owner+"-other", "9月销售额是多少"); err != nil || other != "" {
		t.Fatalf("memory leaked: %q, %v", other, err)
	}
	if _, err := agentMemoryContent("请记住我的 API key 是 abc"); err == nil {
		t.Fatal("credential-like memory accepted")
	}
}

func TestAgentPersistentMemoryRetrievesRelevantContextOnly(t *testing.T) {
	setupAudit(t)
	owner := fmt.Sprintf("memory-search-%d", time.Now().UnixNano())
	for _, content := range []string{"采购入库验收要核对供应商", "工资报表习惯先看应发工资"} {
		if _, err := saveAgentMemory(owner, "context", content, "manual"); err != nil {
			t.Fatal(err)
		}
	}
	recalled, err := agentRecallMemory(owner, "这个月工资报表")
	if err != nil || !strings.Contains(recalled, "工资报表") || strings.Contains(recalled, "采购入库") {
		t.Fatalf("unrelated memory selected: %q, %v", recalled, err)
	}
}

func TestAgentPersistentMemoryReachesNewSessionTaskUnderstanding(t *testing.T) {
	seedAgentSalesComparison(t)
	owner := fmt.Sprintf("memory-prompt-%d", time.Now().UnixNano())
	if _, err := saveAgentMemory(owner, "preference", "回答销售问题先给结论", "manual"); err != nil {
		t.Fatal(err)
	}
	seen := false
	script := &agentMockScript{Task: func(user string) agentTask {
		seen = strings.Contains(user, "回答销售问题先给结论") && strings.Contains(user, "低优先级背景资料")
		return agentSalesTask()
	}}
	ctx, _ := agentTestContext()
	out := runAgentTask(ctx, agentMockRuntime(t, script), models.AIAgentSession{Owner: owner}, agentSendInput{Message: "8月和9月销售情况对比"}, nil)
	if out.Status != "complete" || !seen {
		t.Fatalf("new session did not use persistent preference: status=%s, seen=%t", out.Status, seen)
	}
}
