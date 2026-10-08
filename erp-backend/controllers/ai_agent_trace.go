package controllers

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type agentProfile struct {
	Level   string        `json:"level"`
	Effort  string        `json:"effort"`
	Tokens  int           `json:"token_budget"`
	Steps   int           `json:"step_limit"`
	Timeout time.Duration `json:"-"`
}
type agentCallTrace struct {
	Model            string `json:"model"`
	Milliseconds     int64  `json:"milliseconds"`
	Finish           string `json:"finish_reason"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	ReasoningTokens  int    `json:"reasoning_tokens"`
	Error            string `json:"error,omitempty"`
}
type agentToolTrace struct {
	Plan         agentPlan `json:"plan"`
	Milliseconds int64     `json:"milliseconds"`
	Status       string    `json:"status"`
	Note         string    `json:"note,omitempty"`
}
type agentRunTrace struct {
	Profile      agentProfile     `json:"profile"`
	Calls        []agentCallTrace `json:"calls"`
	Tools        []agentToolTrace `json:"tools"`
	Stop         string           `json:"stop_reason"`
	Milliseconds int64            `json:"milliseconds"`
}
type agentTraceKey struct{}

func agentProfileFor(level, question string) agentProfile {
	if level == "" || level == "auto" {
		level = "standard"
		for _, word := range []string{"原因", "预测", "为什么", "建议", "方案"} {
			if strings.Contains(question, word) {
				level = "deep"
				break
			}
		}
	}
	switch level {
	case "quick":
		return agentProfile{Level: level, Effort: "low", Tokens: 8192, Steps: 6, Timeout: 90 * time.Second}
	case "deep":
		return agentProfile{Level: level, Effort: "max", Tokens: 32768, Steps: 14, Timeout: 180 * time.Second}
	default:
		return agentProfile{Level: "standard", Effort: "high", Tokens: 16384, Steps: 10, Timeout: 120 * time.Second}
	}
}
func agentTraceFrom(ctx context.Context) *agentRunTrace {
	trace, _ := ctx.Value(agentTraceKey{}).(*agentRunTrace)
	return trace
}
func agentTraceJSON(trace *agentRunTrace) string { b, _ := json.Marshal(trace); return string(b) }
