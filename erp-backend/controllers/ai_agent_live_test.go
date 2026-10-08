package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"
)

// Opt-in integration against the configured provider. It uses the isolated
// audit database and fabricated orders; production business data is not sent.
func TestAgentLiveBusinessQuestions(t *testing.T) {
	encrypted := os.Getenv("AI_AGENT_LIVE_ENCRYPTED_KEY")
	if encrypted == "" {
		t.Skip("set AI_AGENT_LIVE_ENCRYPTED_KEY for provider evaluation")
	}
	key, err := decryptAIKey(encrypted)
	if err != nil {
		t.Fatal("live provider key could not be decrypted")
	}
	runtime := aiProviderRuntime{Provider: "deepseek", Model: "deepseek-flash", APIKey: key, URL: "https://api.deepseek.com/chat/completions", JSONMode: true}
	seedAgentSalesComparison(t)
	seedAgentSubsidyBalance(t)
	seedAgentLivePayroll(t)
	artifacts := []map[string]any{}
	defer func() {
		if path := os.Getenv("AI_AGENT_EVAL_OUTPUT"); path != "" {
			body, _ := json.MarshalIndent(artifacts, "", "  ")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Error(err)
			}
		}
	}()
	session := models.AIAgentSession{}
	recent := []models.AIAgentMessage{}
	for _, question := range []string{"8月和9月销售情况对比", "那9月呢", "我说的是订单数量", "现在库存怎么样", "国补还有多少没有回款", "查询员工上月提成"} {
		t.Run(question, func(t *testing.T) {
			parent, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			trace := &agentRunTrace{Profile: agentProfileFor("standard", question), Calls: []agentCallTrace{}, Tools: []agentToolTrace{}}
			ctx := context.WithValue(parent, agentTraceKey{}, trace)
			out := runAgentTask(ctx, runtime, session, agentSendInput{Message: question, Thinking: "standard"}, recent)
			artifacts = append(artifacts, map[string]any{"question": question, "task": out.Task, "status": out.Status, "answer": out.Answer, "trace": trace})
			t.Logf("status=%s calls=%d tools=%d stop=%s", out.Status, len(trace.Calls), len(trace.Tools), trace.Stop)
			if out.Status != "complete" {
				t.Errorf("incomplete answer: %s", out.Answer)
			}
			if strings.Contains(out.Answer, "查询参数：") || strings.Contains(out.Answer, "\"dataset\"") {
				t.Error("raw query leaked")
			}
			if question == "8月和9月销售情况对比" {
				if len(out.Task.Periods) != 2 || !strings.Contains(out.Answer, "¥900.00") || !strings.Contains(out.Answer, "¥1900.00") {
					t.Errorf("comparison missing: %s", out.Answer)
				}
			}
			if question == "国补还有多少没有回款" {
				if out.Task.Topic != "subsidy" || len(out.Task.Periods) != 0 || !strings.Contains(out.Answer, "¥150.00") {
					t.Errorf("subsidy balance missing: %s; task=%+v", out.Answer, out.Task)
				}
			}
			if question == "查询员工上月提成" {
				if out.Task.Topic != "payroll" || out.Task.Basis != "commission_payable" || !strings.Contains(out.Answer, "应计提成") {
					t.Errorf("commission report missing: %s; task=%+v", out.Answer, out.Task)
				}
			}
			state, _ := json.Marshal(map[string]any{"task": out.Task})
			session.Context = string(state)
			recent = append(recent, models.AIAgentMessage{Role: "user", Content: question}, models.AIAgentMessage{Role: "assistant", Content: out.Answer})
		})
	}
	// Independent first-turn prompts exercise everyday phrasing without relying
	// on the previous case's session context.
	for _, question := range []string{"上个月卖得怎么样", "上个月收支情况怎么样", "未来三个月销售额大概多少"} {
		question := question
		t.Run("独立问法/"+question, func(t *testing.T) {
			parent, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			trace := &agentRunTrace{Profile: agentProfileFor("standard", question), Calls: []agentCallTrace{}, Tools: []agentToolTrace{}}
			ctx := context.WithValue(parent, agentTraceKey{}, trace)
			out := runAgentTask(ctx, runtime, models.AIAgentSession{}, agentSendInput{Message: question, Thinking: "standard"}, nil)
			artifacts = append(artifacts, map[string]any{"question": question, "task": out.Task, "status": out.Status, "answer": out.Answer, "trace": trace})
			t.Logf("status=%s task=%s/%s metrics=%v tools=%d stop=%s", out.Status, out.Task.Topic, out.Task.Operation, out.Task.Metrics, len(trace.Tools), trace.Stop)
			if out.Status != "complete" {
				t.Errorf("incomplete answer: %s", out.Answer)
			}
			if strings.Contains(out.Answer, "查询参数：") || strings.Contains(out.Answer, `"dataset"`) {
				t.Error("raw query leaked")
			}
		})
	}
}

func seedAgentLivePayroll(t *testing.T) {
	t.Helper()
	// The audit DB is disposable; remove employee fixtures left by earlier tests.
	if err := config.DB.Exec("DELETE FROM employees").Error; err != nil {
		t.Fatal("isolated payroll fixtures could not be reset")
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -1, 0)
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	employee := models.Employee{Name: "AgentLive提成员工" + stamp, EmpNo: "AL" + stamp, EntryDate: &start}
	if err := config.DB.Create(&employee).Error; err != nil {
		t.Fatal("isolated payroll fixture could not be created")
	}
}

func TestAgentLiveMemoryInference(t *testing.T) {
	encrypted := os.Getenv("AI_AGENT_LIVE_ENCRYPTED_KEY")
	if encrypted == "" {
		t.Skip("set AI_AGENT_LIVE_ENCRYPTED_KEY for provider evaluation")
	}
	key, err := decryptAIKey(encrypted)
	if err != nil {
		t.Fatal("live provider key could not be decrypted")
	}
	runtime := aiProviderRuntime{Provider: "deepseek", Model: "deepseek-flash", APIKey: key, URL: "https://api.deepseek.com/chat/completions", JSONMode: true}
	setupAudit(t)
	for index, test := range []struct {
		question   string
		wantMemory bool
	}{
		{question: "平时看报表我更关注变化原因，回答请先给结论，再用简短的中文解释", wantMemory: true},
		{question: "回答太长了，简短点", wantMemory: true},
		{question: "我主要想查上月销售额，看看多少", wantMemory: false},
	} {
		t.Run(test.question, func(t *testing.T) {
			owner := fmt.Sprintf("live-memory-%d-%d", time.Now().UnixNano(), index)
			memories := agentInferMemories(context.Background(), runtime, owner, test.question, nil)
			if (len(memories) > 0) != test.wantMemory {
				t.Fatalf("memory inference mismatch: %+v", memories)
			}
			t.Logf("saved=%d", len(memories))
		})
	}
}
