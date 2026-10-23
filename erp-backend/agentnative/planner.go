package agentnative

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"erp-backend/models"

	"gorm.io/gorm"
)

const (
	DecisionCallTool          = "CALL_TOOL"
	DecisionFinish            = "FINISH"
	DecisionRequestClarify    = "REQUEST_CLARIFICATION"
	DecisionRequestApproval   = "REQUEST_APPROVAL"
	DecisionFail              = "FAIL"
	plannerMaxResponseBytes   = 8 * 1024
	plannerMaxGoalRunes       = 1200
	plannerMaxContextBytes    = 24 * 1024
	plannerMaxToolResultBytes = 6 * 1024
)

// ToolDefinition is the server-owned description shown to a planner. Only
// registered definitions can be dispatched by RuntimeExecutor.
type ToolDefinition struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	InputSchema        string `json:"input_schema"`
	OutputSchema       string `json:"output_schema"`
	RequiredPermission string `json:"required_permission"`
	RiskLevel          string `json:"risk_level"`
	TimeoutMillis      int64  `json:"timeout_millis"`
	SideEffects        bool   `json:"side_effects"`
	IdempotencyPolicy  string `json:"idempotency_policy"`
	AuditPolicy        string `json:"audit_policy"`
}

type PlannerEvidence struct {
	Sequence int             `json:"sequence"`
	ToolName string          `json:"tool_name"`
	Summary  string          `json:"summary"`
	Data     json.RawMessage `json:"data,omitempty"`
}

type PlannerRequest struct {
	Goal     string            `json:"goal"`
	Now      string            `json:"now"`
	Tools    []ToolDefinition  `json:"tools"`
	Evidence []PlannerEvidence `json:"evidence"`
	Call     int               `json:"call"`
	Feedback string            `json:"feedback,omitempty"`
}

type PlannerDecision struct {
	Type      string         `json:"type"`
	Tool      string         `json:"tool,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Answer    string         `json:"answer,omitempty"`
	Reason    string         `json:"reason,omitempty"`
}

type PlannerUsage struct {
	Provider         string
	Model            string
	PromptTokens     int
	CompletionTokens int
	EstimatedTokens  int
	TokenUsageKnown  bool
	Latency          time.Duration
}

// RuntimeEvent exposes bounded execution metadata for diagnostics and tests.
type RuntimeEvent struct {
	TaskID   uint
	Stage    string
	ToolName string
}

func (r *RuntimeExecutor) emit(event RuntimeEvent) {
	if r != nil && r.OnEvent != nil {
		r.OnEvent(event)
	}
}

// AgentPlanner adapts an existing provider to the runtime's constrained JSON
// action protocol. Implementations must honor ctx and bound response size.
type AgentPlanner interface {
	Decide(context.Context, PlannerRequest) (PlannerDecision, PlannerUsage, error)
}

type PlannerLimits struct {
	MaxPlannerCalls int
	MaxToolCalls    int
	MaxRunTime      time.Duration
	MaxTokens       int
	MaxRetries      int
	MaxAnswerRunes  int
}

func DefaultPlannerLimits() PlannerLimits {
	return PlannerLimits{
		MaxPlannerCalls: 8,
		MaxToolCalls:    12,
		MaxRunTime:      4 * time.Minute,
		MaxTokens:       16000,
		MaxRetries:      1,
		MaxAnswerRunes:  3000,
	}
}

type plannerTaskInput struct {
	Goal string `json:"goal"`
}

type plannerTaskResult struct {
	Status string `json:"status"`
	Answer string `json:"answer,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// ExecutePlannerTask performs a durable, bounded decision/tool loop. A restart
// rebuilds context from completed step rows and never re-runs successful reads.
func (r *RuntimeExecutor) ExecutePlannerTask(ctx context.Context, task models.AgentWorkTask, now time.Time) error {
	if r == nil || r.DB == nil {
		return errors.New("agent runtime unavailable")
	}
	var input plannerTaskInput
	if len(task.InputJSON) == 0 || len(task.InputJSON) > 8*1024 || json.Unmarshal([]byte(task.InputJSON), &input) != nil {
		return errors.New("planner task input is invalid")
	}
	input.Goal = strings.TrimSpace(input.Goal)
	if input.Goal == "" || len([]rune(input.Goal)) > plannerMaxGoalRunes {
		return errors.New("planner task goal is empty or too long")
	}
	limits := r.Limits
	defaults := DefaultPlannerLimits()
	if limits.MaxPlannerCalls <= 0 {
		limits.MaxPlannerCalls = defaults.MaxPlannerCalls
	}
	if limits.MaxToolCalls <= 0 {
		limits.MaxToolCalls = defaults.MaxToolCalls
	}
	if limits.MaxRunTime <= 0 {
		limits.MaxRunTime = defaults.MaxRunTime
	}
	if limits.MaxTokens <= 0 {
		limits.MaxTokens = defaults.MaxTokens
	}
	if limits.MaxRetries <= 0 {
		limits.MaxRetries = defaults.MaxRetries
	}
	if limits.MaxAnswerRunes <= 0 {
		limits.MaxAnswerRunes = defaults.MaxAnswerRunes
	}
	runCtx, cancel := context.WithTimeout(ctx, limits.MaxRunTime)
	defer cancel()
	if task.Kind == "schedule_inventory_check" {
		if err := r.runScheduledRuleCheck(runCtx, task); err != nil {
			return fmt.Errorf("scheduled deterministic inventory check: %w", err)
		}
	}
	if r.Planner == nil {
		if task.Kind == "schedule_inventory_check" {
			return r.completePlannerTask(runCtx, task, plannerTaskResult{
				Status: "PARTIAL", Answer: "已完成确定性库存规则检查；LLM Planner 当前不可用，因此没有生成语义分析。", Reason: "provider_unavailable",
			}, now)
		}
		return r.completePlannerTask(runCtx, task, plannerTaskResult{
			Status: "FAILED", Answer: "当前配置的 LLM Provider 不可用；未完成语义分析。", Reason: "provider_unavailable",
		}, now)
	}
	var steps []models.AgentWorkTaskStep
	if err := r.DB.WithContext(runCtx).Where("task_id = ?", task.ID).Order("sequence").Find(&steps).Error; err != nil {
		return fmt.Errorf("load planner task history: %w", err)
	}
	evidence := make([]PlannerEvidence, 0, len(steps))
	sequence := 0
	toolStepCount := 0
	for _, step := range steps {
		if step.Sequence > sequence {
			sequence = step.Sequence
		}
		if step.StepType == "tool" {
			toolStepCount++
		}
		if step.StepType != "tool" || step.Status != "SUCCEEDED" {
			continue
		}
		var output ToolOutput
		if json.Unmarshal([]byte(step.ResultSummary), &output) != nil {
			continue
		}
		data, _ := json.Marshal(output.Data)
		if len(data) > plannerMaxToolResultBytes {
			data = json.RawMessage(`{"truncated":true}`)
		}
		evidence = append(evidence, PlannerEvidence{
			Sequence: step.Sequence, ToolName: output.Name,
			Summary: truncateRunes(output.Summary, 300), Data: data,
		})
	}
	feedback := ""
	plannerCalls := 0
	toolCalls := toolStepCount
	usedTokens := 0
	var pendingDecision *PlannerDecision
	var pendingToolStep *models.AgentWorkTaskStep
	for index, step := range steps {
		if step.StepType == "planner" && step.Status == "SUCCEEDED" {
			plannerCalls++
			var decision PlannerDecision
			if json.Unmarshal([]byte(step.ResultSummary), &decision) != nil {
				continue
			}
			switch {
			case decision.Type == DecisionCallTool && index+1 == len(steps):
				pendingDecision = &decision
			case decision.Type == DecisionCallTool && index+1 < len(steps) && steps[index+1].Sequence == step.Sequence+1 &&
				steps[index+1].ToolName == decision.Tool && steps[index+1].Status != "SUCCEEDED":
				pendingDecision = &decision
				pendingToolStep = &steps[index+1]
			case index+1 == len(steps):
				pendingDecision = &decision
			}
		}
	}
	var savedCalls []models.AgentPlannerCall
	if err := r.DB.WithContext(runCtx).Where("task_id = ?", task.ID).Order("call_number").Find(&savedCalls).Error; err != nil {
		return fmt.Errorf("load planner call history: %w", err)
	}
	for _, call := range savedCalls {
		if call.CallNumber > plannerCalls {
			plannerCalls = call.CallNumber
		}
		if call.TokenUsageKnown {
			usedTokens += call.PromptTokens + call.CompletionTokens
		} else {
			usedTokens += call.EstimatedTokens
		}
	}
	for pendingDecision != nil || (plannerCalls < limits.MaxPlannerCalls && toolCalls < limits.MaxToolCalls) {
		if err := r.checkTaskLease(runCtx, task); err != nil {
			return err
		}
		if r.Planner == nil && pendingDecision == nil {
			return errors.New("LLM planner unavailable")
		}
		contextBytes, _ := json.Marshal(evidence)
		for len(contextBytes) > plannerMaxContextBytes && len(evidence) > 0 {
			evidence = evidence[1:]
			contextBytes, _ = json.Marshal(evidence)
		}
		request := PlannerRequest{
			Goal: input.Goal, Now: now.Format(time.RFC3339), Tools: r.availableTools(task), Evidence: evidence,
			Call: plannerCalls + 1, Feedback: truncateRunes(feedback, 500),
		}
		var decision PlannerDecision
		if pendingDecision != nil {
			decision = *pendingDecision
			pendingDecision = nil
		} else {
			encodedRequest, _ := json.Marshal(request)
			estimatedPrompt := estimateTokens(len(encodedRequest))
			if usedTokens+estimatedPrompt+1536 > limits.MaxTokens {
				break
			}
			started := time.Now()
			var usage PlannerUsage
			var planErr error
			decision, usage, planErr = r.Planner.Decide(runCtx, request)
			usage.Latency = time.Since(started)
			usage.TokenUsageKnown = usage.PromptTokens+usage.CompletionTokens > 0
			usage.EstimatedTokens = usage.PromptTokens + usage.CompletionTokens
			if !usage.TokenUsageKnown {
				usage.EstimatedTokens = estimatedPrompt + 1536
			}
			plannerCalls++
			callNumber := plannerCalls
			if planErr != nil {
				if err := r.persistPlannerFailure(runCtx, task, callNumber, usage, planErr); err != nil {
					return err
				}
				if plannerErrorClass(planErr) == "invalid_output" && plannerCalls <= limits.MaxRetries {
					feedback = "上一次模型输出不是有效的结构化决策；请只返回符合协议的 JSON。"
					continue
				}
				if task.Kind == "schedule_inventory_check" {
					return r.completePlannerTask(runCtx, task, plannerTaskResult{
						Status: "PARTIAL", Answer: "已完成确定性库存规则检查；LLM Planner 暂不可用，语义分析未完成。", Reason: plannerErrorClass(planErr),
					}, now)
				}
				return r.completePlannerTask(runCtx, task, plannerTaskResult{
					Status: "FAILED", Answer: "LLM Planner 调用失败；未生成分析结论。", Reason: truncateRunes(planErr.Error(), 500),
				}, now)
			}
			if err := validatePlannerDecision(decision, limits.MaxAnswerRunes); err != nil {
				if saveErr := r.persistPlannerFailure(runCtx, task, callNumber, usage, err); saveErr != nil {
					return saveErr
				}
				if plannerCalls <= limits.MaxRetries {
					feedback = "上一次结构化决策无效；只可选择工具目录中列出的工具并提供符合参数约束的 JSON。"
					continue
				}
				return r.completePlannerTask(runCtx, task, plannerTaskResult{
					Status: "FAILED", Answer: "LLM Planner 多次返回无效决策；未执行相关操作。", Reason: truncateRunes(err.Error(), 500),
				}, now)
			}
			callTokens := usage.EstimatedTokens
			if usage.TokenUsageKnown {
				callTokens = usage.PromptTokens + usage.CompletionTokens
			}
			if usedTokens+callTokens > limits.MaxTokens {
				if err := r.persistPlannerFailure(runCtx, task, callNumber, usage, errors.New("planner token budget exceeded after provider usage report")); err != nil {
					return err
				}
				return r.completePlannerTask(runCtx, task, plannerTaskResult{
					Status: "PARTIAL", Answer: "已保存此前取得的证据；本次模型调用达到服务端 Token 预算，未执行其行动建议。", Reason: "token_budget_exceeded",
				}, now)
			}
			if err := r.persistPlannerDecision(runCtx, task, callNumber, sequence+1, decision, usage, now); err != nil {
				return err
			}
			sequence++
			usedTokens += callTokens
		}
		switch decision.Type {
		case DecisionCallTool:
			tool := ToolInput{Name: decision.Tool, Args: decision.Arguments}
			if err := validateToolInput(tool); err != nil {
				feedback = "工具参数不符合服务端约束；请调整查询，不能改变权限或注册表。"
				continue
			}
			if !r.taskCanUseTool(task, tool.Name) {
				return fmt.Errorf("task is not authorized for tool %q", tool.Name)
			}
			var step models.AgentWorkTaskStep
			if pendingToolStep != nil {
				step = *pendingToolStep
				pendingToolStep = nil
				if err := r.updateTaskStep(runCtx, task, step, map[string]any{
					"status": "RUNNING", "started_at": now, "retry_count": gorm.Expr("retry_count + 1"), "error_message": "",
				}); err != nil {
					return fmt.Errorf("resume planner tool step: %w", err)
				}
			} else {
				r.emit(RuntimeEvent{TaskID: task.ID, Stage: "before_tool_step", ToolName: tool.Name})
				key := fmt.Sprintf("agent-task:%d:planner-step:%d", task.ID, sequence)
				step = models.AgentWorkTaskStep{
					TaskID: task.ID, Sequence: sequence + 1, StepType: "tool", ToolName: tool.Name,
					InputSummary: summarizeInput(tool), Status: "RUNNING", StartedAt: timePtr(now), IdempotencyKey: key,
				}
				if err := r.createTaskStep(runCtx, task, &step); err != nil {
					return fmt.Errorf("persist planner tool step: %w", err)
				}
				sequence++
				toolCalls++
			}
			r.emit(RuntimeEvent{TaskID: task.ID, Stage: "before_tool_execution", ToolName: tool.Name})
			key := step.IdempotencyKey
			principal, permissions := taskPrincipal(task)
			timeout := 20 * time.Second
			if definition, ok := plannerToolDefinitions[tool.Name]; ok && definition.TimeoutMillis > 0 {
				timeout = time.Duration(definition.TimeoutMillis) * time.Millisecond
			}
			output, executeErr := r.Execute(runCtx, ExecutionContext{
				UserID: task.CreatedBy, Principal: principal, PermissionSet: permissions,
				TenantID: task.TenantID, TaskID: task.ID, LeaseVersion: task.LeaseVersion, ToolCallID: key,
				IdempotencyKey: key, Timeout: timeout,
			}, tool)
			finished := time.Now()
			r.emit(RuntimeEvent{TaskID: task.ID, Stage: "after_tool_execution", ToolName: tool.Name})
			if executeErr != nil {
				message := truncateRunes(executeErr.Error(), 500)
				if updateErr := r.updateTaskStep(runCtx, task, step, map[string]any{
					"status": "FAILED", "finished_at": finished, "error_message": message,
				}); updateErr != nil {
					return updateErr
				}
				feedback = "工具未能完成查询；根据缺失或错误调整下一步，不要声称已经执行成功。"
				continue
			}
			encoded, marshalErr := json.Marshal(output)
			if marshalErr != nil {
				return fmt.Errorf("encode planner tool result: %w", marshalErr)
			}
			if len(encoded) > plannerMaxToolResultBytes {
				if rows := reflect.ValueOf(output.Data); rows.IsValid() && rows.Kind() == reflect.Slice {
					for rows.Len() > 0 && len(encoded) > plannerMaxToolResultBytes {
						rows = rows.Slice(0, rows.Len()-1)
						output.Data = rows.Interface()
						encoded, _ = json.Marshal(output)
					}
				}
				if len(encoded) > plannerMaxToolResultBytes {
					output.Summary = truncateRunes(output.Summary, 250)
					output.Data = map[string]any{"truncated": true, "summary": output.Summary}
					encoded, _ = json.Marshal(output)
				}
			}
			if err := r.savePlannerToolResult(runCtx, task, step, encoded, finished); err != nil {
				return err
			}
			var saved ToolOutput
			if err := json.Unmarshal(encoded, &saved); err == nil {
				data, _ := json.Marshal(saved.Data)
				evidence = append(evidence, PlannerEvidence{Sequence: step.Sequence, ToolName: saved.Name, Summary: saved.Summary, Data: data})
			}
			feedback = ""
		case DecisionFinish:
			return r.completePlannerTask(runCtx, task, plannerTaskResult{Status: "SUCCEEDED", Answer: strings.TrimSpace(decision.Answer)}, now)
		case DecisionRequestClarify:
			return r.completePlannerTask(runCtx, task, plannerTaskResult{Status: "WAITING_CLARIFICATION", Answer: strings.TrimSpace(decision.Answer)}, now)
		case DecisionRequestApproval:
			return r.completePlannerTask(runCtx, task, plannerTaskResult{Status: "WAITING_APPROVAL", Answer: strings.TrimSpace(decision.Answer)}, now)
		case DecisionFail:
			return r.completePlannerTask(runCtx, task, plannerTaskResult{Status: "FAILED", Reason: truncateRunes(decision.Answer, 500)}, now)
		}
	}
	answer := "已保存目前可核对的证据，但任务达到服务端执行预算上限，结果为部分完成。"
	return r.completePlannerTask(runCtx, task, plannerTaskResult{Status: "PARTIAL", Answer: answer, Reason: "execution_budget_exceeded"}, now)
}

func estimateTokens(byteCount int) int {
	if byteCount <= 0 {
		return 0
	}
	return (byteCount + 2) / 3
}

func validatePlannerDecision(decision PlannerDecision, maxAnswerRunes int) error {
	if len([]byte(mustJSON(decision))) > plannerMaxResponseBytes {
		return errors.New("planner decision exceeds response limit")
	}
	if len([]rune(decision.Reason)) > 300 || len([]rune(decision.Answer)) > maxAnswerRunes {
		return errors.New("planner decision text exceeds limit")
	}
	switch decision.Type {
	case DecisionCallTool:
		if decision.Tool == "" || decision.Answer != "" {
			return errors.New("CALL_TOOL requires a tool and no answer")
		}
	case DecisionFinish, DecisionRequestClarify, DecisionRequestApproval:
		if strings.TrimSpace(decision.Answer) == "" || decision.Tool != "" {
			return errors.New("terminal decision requires an answer and no tool")
		}
	case DecisionFail:
		if strings.TrimSpace(decision.Answer) == "" {
			return errors.New("FAIL requires a reason")
		}
	default:
		return fmt.Errorf("unsupported decision type %q", decision.Type)
	}
	return nil
}

func mustJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
