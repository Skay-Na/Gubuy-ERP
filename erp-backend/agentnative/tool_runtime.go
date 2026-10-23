package agentnative

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"erp-backend/models"
	"erp-backend/services"

	"gorm.io/gorm"
)

// ExecutionContext carries trusted identity and cancellation data without coupling tools to Gin.
type ExecutionContext struct {
	UserID         string
	Principal      string
	PermissionSet  []string
	TenantID       string
	TaskID         uint
	LeaseVersion   uint64
	ToolCallID     string
	IdempotencyKey string
	Timeout        time.Duration
}

type ToolInput struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type ToolOutput struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Data    any    `json:"data,omitempty"`
}

type ToolHandler func(context.Context, ExecutionContext, map[string]any) (ToolOutput, error)

type ToolExecutor interface {
	Execute(context.Context, ExecutionContext, ToolInput) (ToolOutput, error)
}

// RuntimeExecutor is the single registry and policy boundary for durable and request-scoped tools.
type RuntimeExecutor struct {
	DB      *gorm.DB
	Monitor MonitorService
	Tools   map[string]ToolHandler
	Planner AgentPlanner
	Limits  PlannerLimits
	Now     func() time.Time
	OnEvent func(RuntimeEvent)
}

func (r *RuntimeExecutor) now() time.Time {
	if r != nil && r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

var ErrTaskCancelled = errors.New("agent task cancelled")

var toolPermissions = map[string]string{
	"retail.product_inventory":            "retail.read",
	"retail.replenishment_context":        "retail.read",
	"retail.sales_summary":                "retail.read",
	"retail.sales_trend":                  "retail.read",
	"retail.inventory_sales":              "retail.read",
	"monitor.get_signal":                  "monitor.signal.read",
	"replenishment.get_draft":             "replenishment.draft.read",
	"replenishment.create_internal_draft": "replenishment.draft.create",
	"agent.get_task_status":               "agent.task.read",
	"schedule.create":                     "schedule.manage",
	"schedule.list":                       "schedule.read",
	"schedule.history":                    "schedule.read",
	"schedule.update":                     "schedule.manage",
	"schedule.pause":                      "schedule.manage",
	"schedule.resume":                     "schedule.manage",
	"schedule.cancel":                     "schedule.manage",
}

func NewRuntimeExecutor(db *gorm.DB) *RuntimeExecutor {
	r := &RuntimeExecutor{DB: db, Monitor: MonitorService{DB: db}, Tools: map[string]ToolHandler{}}
	r.Tools["retail.product_inventory"] = func(ctx context.Context, ec ExecutionContext, args map[string]any) (ToolOutput, error) {
		keyword, _ := args["keyword"].(string)
		limit := 20
		if configured, ok := integerValue(args["limit"]); ok {
			limit = configured
		}
		items, err := (services.RetailReadService{DB: db}).SearchProductInventory(ctx, keyword, limit)
		if err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "retail.product_inventory", Summary: fmt.Sprintf("读取 %d 个商品库存记录", len(items)), Data: items}, nil
	}
	r.Tools["retail.replenishment_context"] = func(ctx context.Context, _ ExecutionContext, args map[string]any) (ToolOutput, error) {
		from, to := time.Now().AddDate(0, 0, -30), time.Now()
		if raw, ok := args["from"].(string); ok && raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return ToolOutput{}, fmt.Errorf("parse from time: %w", err)
			}
			from = parsed
		}
		if raw, ok := args["to"].(string); ok && raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return ToolOutput{}, fmt.Errorf("parse to time: %w", err)
			}
			to = parsed
		}
		items, err := (services.RetailReadService{DB: db}).GetReplenishmentContext(ctx, from, to)
		if err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "retail.replenishment_context", Summary: fmt.Sprintf("读取 %d 个商品的库存和销量证据", len(items)), Data: items}, nil
	}
	r.Tools["monitor.get_signal"] = func(ctx context.Context, ec ExecutionContext, args map[string]any) (ToolOutput, error) {
		id, ok := numericID(args["signal_id"])
		if !ok || ec.TenantID != SingleTenant {
			return ToolOutput{}, errors.New("invalid signal scope")
		}
		var signal models.AgentSignal
		if err := db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, ec.TenantID).First(&signal).Error; err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "monitor.get_signal", Summary: "已读取规则检测信号及其库存/销量证据", Data: signal}, nil
	}
	r.Tools["replenishment.create_internal_draft"] = func(ctx context.Context, ec ExecutionContext, _ map[string]any) (ToolOutput, error) {
		if ec.TaskID == 0 || ec.TenantID != SingleTenant || !hasPermission(ec.PermissionSet, "replenishment.draft.create") {
			return ToolOutput{}, errors.New("not authorized to create internal replenishment draft")
		}
		var task models.AgentWorkTask
		if err := db.WithContext(ctx).Where("id = ? AND tenant_id = ?", ec.TaskID, ec.TenantID).First(&task).Error; err != nil {
			return ToolOutput{}, err
		}
		if err := r.Monitor.createDraft(ctx, task, ec.LeaseVersion); err != nil {
			return ToolOutput{}, err
		}
		var draft models.AgentReplenishmentDraft
		if err := db.WithContext(ctx).Where("task_id = ? AND tenant_id = ?", task.ID, ec.TenantID).First(&draft).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ToolOutput{Name: "replenishment.create_internal_draft", Summary: "库存已恢复，不需要补货草案"}, nil
			}
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "replenishment.create_internal_draft", Summary: "已保存可人工审核的内部补货草案", Data: draft}, nil
	}
	registerPlannerTools(r)
	return r
}

func (r *RuntimeExecutor) Execute(ctx context.Context, ec ExecutionContext, input ToolInput) (ToolOutput, error) {
	if r == nil || r.DB == nil {
		return ToolOutput{}, errors.New("agent runtime unavailable")
	}
	if ec.Principal != "admin" && ec.Principal != "system_monitor" {
		return ToolOutput{}, errors.New("execution principal is not allowed")
	}
	if ec.TenantID != SingleTenant {
		return ToolOutput{}, errors.New("tenant scope is not allowed")
	}
	if ec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, ec.Timeout)
		defer cancel()
	}
	handler, exists := r.Tools[input.Name]
	if !exists {
		return ToolOutput{}, fmt.Errorf("unknown tool %q", input.Name)
	}
	permission, registered := toolPermissions[input.Name]
	if !registered || !hasPermission(ec.PermissionSet, permission) {
		return ToolOutput{}, fmt.Errorf("principal is not authorized for tool %q", input.Name)
	}
	if err := validateToolInput(input); err != nil {
		return ToolOutput{}, fmt.Errorf("invalid arguments for tool %q: %w", input.Name, err)
	}
	if ec.TaskID > 0 && ec.LeaseVersion > 0 {
		if err := r.checkTaskLease(ctx, models.AgentWorkTask{ID: ec.TaskID, LeaseVersion: ec.LeaseVersion}); err != nil {
			return ToolOutput{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return ToolOutput{}, err
	}
	started := time.Now()
	callID := ec.ToolCallID
	if callID == "" {
		callID = fmt.Sprintf("tool-%d", started.UnixNano())
	} else {
		callID = fmt.Sprintf("%s-%d", callID, started.UnixNano())
	}
	var taskID *uint
	if ec.TaskID > 0 {
		taskID = &ec.TaskID
	}
	audit := models.AgentToolCallAudit{
		TaskID: taskID, CallID: callID, IdempotencyKey: ec.IdempotencyKey,
		TenantID: ec.TenantID, UserID: ec.UserID, Principal: ec.Principal,
		ToolName: input.Name, InputSummary: summarizeInput(input), Status: "RUNNING", StartedAt: started,
	}
	if err := r.DB.WithContext(ctx).Create(&audit).Error; err != nil {
		return ToolOutput{}, fmt.Errorf("persist tool audit: %w", err)
	}
	output, err := handler(ctx, ec, input.Args)
	finished := time.Now()
	if err != nil {
		message := err.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		if auditErr := r.DB.WithContext(ctx).Model(&audit).Updates(map[string]any{"status": "FAILED", "finished_at": finished, "error_message": message}).Error; auditErr != nil {
			return ToolOutput{}, fmt.Errorf("tool %s failed: %w (audit update: %v)", input.Name, err, auditErr)
		}
		return ToolOutput{}, fmt.Errorf("tool %s: %w", input.Name, err)
	}
	outputSummary := output.Summary
	if len(outputSummary) > 500 {
		outputSummary = outputSummary[:500]
	}
	if err := r.DB.WithContext(ctx).Model(&audit).Updates(map[string]any{"status": "SUCCEEDED", "finished_at": finished, "output_summary": outputSummary}).Error; err != nil {
		return ToolOutput{}, fmt.Errorf("persist tool result audit: %w", err)
	}
	return output, nil
}

func hasPermission(permissions []string, permission string) bool {
	for _, candidate := range permissions {
		if candidate == permission {
			return true
		}
	}
	return false
}

func numericID(value any) (uint, bool) {
	switch number := value.(type) {
	case float64:
		if number > 0 && number == float64(uint(number)) {
			return uint(number), true
		}
	case uint:
		return number, number > 0
	case int:
		return uint(number), number > 0
	}
	return 0, false
}

// ExecutePersistedTask runs a bounded, explicit workflow. Successful tool calls are read from durable
// step rows after restart; the side-effecting draft tool is itself uniquely keyed by TaskID.
func (r *RuntimeExecutor) ExecutePersistedTask(ctx context.Context, task models.AgentWorkTask, now time.Time) error {
	if task.Kind != "replenishment" && task.Kind != "replenishment_analysis" {
		return errors.New("unsupported durable task kind")
	}
	if task.SignalID == nil {
		return errors.New("replenishment task is missing its signal")
	}
	steps := []ToolInput{
		{Name: "monitor.get_signal", Args: map[string]any{"signal_id": *task.SignalID}},
		{Name: "retail.replenishment_context", Args: map[string]any{}},
		{Name: "replenishment.create_internal_draft", Args: map[string]any{}},
	}
	principal := "system_monitor"
	permissions := []string{"retail.read", "monitor.signal.read", "replenishment.draft.create"}
	if task.CreatedBy != "system_monitor" {
		principal = "admin"
	}
	for index, call := range steps {
		if err := r.checkTaskLease(ctx, task); err != nil {
			return err
		}
		var saved models.AgentWorkTaskStep
		if err := r.DB.WithContext(ctx).Where("task_id = ? AND sequence = ?", task.ID, index+1).First(&saved).Error; err == nil && saved.Status == "SUCCEEDED" {
			continue
		}
		key := fmt.Sprintf("agent-task:%d:step:%d", task.ID, index+1)
		step := models.AgentWorkTaskStep{TaskID: task.ID, Sequence: index + 1, StepType: "tool", ToolName: call.Name, InputSummary: summarizeInput(call), Status: "RUNNING", StartedAt: &now, IdempotencyKey: key}
		if err := r.DB.WithContext(ctx).Where("task_id = ? AND sequence = ?", task.ID, index+1).First(&step).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			if err := r.createTaskStep(ctx, task, &step); err != nil {
				return fmt.Errorf("persist step %d: %w", index+1, err)
			}
		} else if err != nil {
			return fmt.Errorf("load step %d: %w", index+1, err)
		}
		if step.Status == "SUCCEEDED" {
			continue
		}
		step.Status, step.StartedAt = "RUNNING", &now
		if err := r.updateTaskStep(ctx, task, step, map[string]any{"status": step.Status, "started_at": now, "retry_count": gorm.Expr("retry_count + 1"), "error_message": ""}); err != nil {
			return fmt.Errorf("start step %d: %w", index+1, err)
		}
		output, err := r.Execute(ctx, ExecutionContext{UserID: task.CreatedBy, Principal: principal, PermissionSet: permissions, TenantID: task.TenantID, TaskID: task.ID, LeaseVersion: task.LeaseVersion, ToolCallID: key, IdempotencyKey: key, Timeout: 30 * time.Second}, call)
		finished := time.Now()
		if err != nil {
			message := err.Error()
			if len(message) > 500 {
				message = message[:500]
			}
			_ = r.updateTaskStep(ctx, task, step, map[string]any{"status": "FAILED", "finished_at": finished, "error_message": message})
			return fmt.Errorf("execute step %d: %w", index+1, err)
		}
		result, marshalErr := json.Marshal(output)
		if marshalErr != nil {
			return fmt.Errorf("encode step result: %w", marshalErr)
		}
		if err := r.updateTaskStep(ctx, task, step, map[string]any{"status": "SUCCEEDED", "finished_at": finished, "result_summary": string(result), "error_message": ""}); err != nil {
			return fmt.Errorf("save step %d result: %w", index+1, err)
		}
	}
	return r.completeFixedTask(ctx, task)
}

// ReconcileCompletedTasks closes the crash window between the draft transaction and step result write.
func (r *RuntimeExecutor) ReconcileCompletedTasks(ctx context.Context) error {
	var steps []models.AgentWorkTaskStep
	if err := r.DB.WithContext(ctx).
		Joins("JOIN agent_work_tasks ON agent_work_tasks.id = agent_work_task_steps.task_id").
		Where("agent_work_tasks.tenant_id = ? AND agent_work_tasks.status = ? AND agent_work_task_steps.status = ? AND agent_work_task_steps.tool_name = ?",
			SingleTenant, "SUCCEEDED", "RUNNING", "replenishment.create_internal_draft").
		Where("agent_work_tasks.kind NOT IN ?", []string{"replenishment", "replenishment_analysis", "schedule_inventory_check"}).
		Where("agent_work_tasks.created_by NOT IN ?", []string{"system_monitor", "system_scheduler"}).
		Find(&steps).Error; err != nil {
		return fmt.Errorf("find completed tasks with unfinished draft steps: %w", err)
	}
	for _, step := range steps {
		var draft models.AgentReplenishmentDraft
		err := r.DB.WithContext(ctx).Where("task_id = ? AND tenant_id = ?", step.TaskID, SingleTenant).First(&draft).Error
		summary := "规则复核后库存已恢复，无需创建补货草案"
		if err == nil {
			encoded, marshalErr := json.Marshal(ToolOutput{Name: step.ToolName, Summary: "已恢复内部补货草案执行结果", Data: draft})
			if marshalErr != nil {
				return fmt.Errorf("encode recovered draft step: %w", marshalErr)
			}
			summary = string(encoded)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read recovered draft: %w", err)
		}
		if err := r.DB.WithContext(ctx).Model(&step).Updates(map[string]any{"status": "SUCCEEDED", "finished_at": time.Now(), "result_summary": summary, "error_message": ""}).Error; err != nil {
			return fmt.Errorf("reconcile task step %d: %w", step.ID, err)
		}
	}
	return nil
}

func summarizeInput(input ToolInput) string {
	data, err := json.Marshal(input.Args)
	if err != nil {
		return strings.TrimSpace(input.Name)
	}
	text := string(data)
	if len(text) > 500 {
		text = text[:500]
	}
	return text
}
