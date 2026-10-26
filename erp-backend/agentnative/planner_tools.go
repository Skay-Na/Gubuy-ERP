package agentnative

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"erp-backend/models"
	"erp-backend/services"

	"gorm.io/gorm"
)

var plannerToolDefinitions = map[string]ToolDefinition{
	"retail.product_inventory": {
		Name: "retail.product_inventory", Description: "按商品名称查询当前门店、云仓可售库存，不含样机。",
		InputSchema:  `{"type":"object","properties":{"keyword":{"type":"string","maxLength":200},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`,
		OutputSchema: `{"type":"array","items":{"product_id":"integer","name":"string","available":"integer"}}`, RequiredPermission: "retail.read", RiskLevel: "LOW", TimeoutMillis: 8000, AuditPolicy: "required",
	},
	"retail.replenishment_context": {
		Name: "retail.replenishment_context", Description: "读取商品当前可售库存及指定期间有效订单销量；用于确定性补货风险判断。",
		InputSchema:  `{"type":"object","properties":{"from":{"type":"string"},"to":{"type":"string"}},"additionalProperties":false}`,
		OutputSchema: `{"type":"array","items":{"product_id":"integer","product_name":"string","available":"integer","sales":"integer"}}`, RequiredPermission: "retail.read", RiskLevel: "LOW", TimeoutMillis: 12000, AuditPolicy: "required",
	},
	"retail.sales_summary": {
		Name: "retail.sales_summary", Description: "统计指定时间范围内有效订单的商品销量和订单数；不按商品分摊订单折扣。",
		InputSchema:  `{"type":"object","required":["from","to"],"properties":{"from":{"type":"string"},"to":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`,
		OutputSchema: `{"type":"array","items":{"product_id":"integer","product_name":"string","units":"integer","order_count":"integer"}}`, RequiredPermission: "retail.read", RiskLevel: "LOW", TimeoutMillis: 12000, AuditPolicy: "required",
	},
	"retail.sales_trend": {
		Name: "retail.sales_trend", Description: "将所选时间段与前一个等长时间段比较有效订单销量，返回销量变化最大的商品；to为排他边界。",
		InputSchema:  `{"type":"object","required":["from","to"],"properties":{"from":{"type":"string"},"to":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`,
		OutputSchema: `{"type":"array","items":{"product_id":"integer","product_name":"string","current_units":"integer","previous_units":"integer","delta_units":"integer","change_percent":"number|null"}}`, RequiredPermission: "retail.read", RiskLevel: "LOW", TimeoutMillis: 12000, AuditPolicy: "required",
	},
	"retail.inventory_sales": {
		Name: "retail.inventory_sales", Description: "将当前可售库存与指定时间范围内的有效订单销量及订单数合并，结果最多100个商品。",
		InputSchema:  `{"type":"object","required":["from","to"],"properties":{"from":{"type":"string"},"to":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`,
		OutputSchema: `{"type":"array","items":{"product_id":"integer","product_name":"string","available":"integer","units":"integer","order_count":"integer"}}`, RequiredPermission: "retail.read", RiskLevel: "LOW", TimeoutMillis: 12000, AuditPolicy: "required",
	},
	"monitor.get_signal": {
		Name: "monitor.get_signal", Description: "读取当前任务关联的经营信号和确定性监测证据。",
		InputSchema:  `{"type":"object","properties":{"signal_id":{"type":"integer","minimum":1}},"additionalProperties":false}`,
		OutputSchema: `{"type":"object"}`, RequiredPermission: "monitor.signal.read", RiskLevel: "LOW", TimeoutMillis: 8000, AuditPolicy: "required",
	},
	"replenishment.get_draft": {
		Name: "replenishment.get_draft", Description: "读取当前任务对应的内部补货建议；不会创建采购单。",
		InputSchema:  `{"type":"object","properties":{},"additionalProperties":false}`,
		OutputSchema: `{"type":"object"}`, RequiredPermission: "replenishment.draft.read", RiskLevel: "LOW", TimeoutMillis: 8000, AuditPolicy: "required",
	},
	"replenishment.create_internal_draft": {
		Name: "replenishment.create_internal_draft", Description: "创建当前监测任务的幂等内部补货草案，只供管理员审核；不下单、不付款、不改库存。",
		InputSchema:  `{"type":"object","properties":{},"additionalProperties":false}`,
		OutputSchema: `{"type":"object"}`, RequiredPermission: "replenishment.draft.create", RiskLevel: "MEDIUM", TimeoutMillis: 15000, SideEffects: true, IdempotencyPolicy: "unique task_id", AuditPolicy: "required",
	},
	"agent.get_task_status": {
		Name: "agent.get_task_status", Description: "读取当前任务自身的状态和步骤标记。",
		InputSchema:  `{"type":"object","properties":{},"additionalProperties":false}`,
		OutputSchema: `{"type":"object"}`, RequiredPermission: "agent.task.read", RiskLevel: "LOW", TimeoutMillis: 5000, AuditPolicy: "required",
	},
	"schedule.create": {
		Name: "schedule.create", Description: "创建每日库存检查计划。只支持 daily_inventory_check；默认时间为Asia/Shanghai 09:00。",
		InputSchema:  `{"type":"object","required":["task_type"],"properties":{"task_type":{"const":"daily_inventory_check"},"timezone":{"type":"string","maxLength":64},"time_of_day":{"type":"string","pattern":"^[0-2][0-9]:[0-5][0-9]$"}},"additionalProperties":false}`,
		OutputSchema: `{"type":"object"}`, RequiredPermission: "schedule.manage", RiskLevel: "LOW", TimeoutMillis: 8000, SideEffects: true, IdempotencyPolicy: "validated owner-scoped insert", AuditPolicy: "required",
	},
	"schedule.list": {
		Name: "schedule.list", Description: "列出当前用户自己的长期经营任务。",
		InputSchema:  `{"type":"object","properties":{},"additionalProperties":false}`,
		OutputSchema: `{"type":"array"}`, RequiredPermission: "schedule.read", RiskLevel: "LOW", TimeoutMillis: 8000, AuditPolicy: "required",
	},
	"schedule.history": {
		Name: "schedule.history", Description: "读取指定自有每日库存检查计划最近的真实执行历史和关联任务状态。",
		InputSchema:  `{"type":"object","required":["schedule_id"],"properties":{"schedule_id":{"type":"integer","minimum":1}},"additionalProperties":false}`,
		OutputSchema: `{"type":"array"}`, RequiredPermission: "schedule.read", RiskLevel: "LOW", TimeoutMillis: 8000, AuditPolicy: "required",
	},
	"schedule.update": {
		Name: "schedule.update", Description: "修改指定的自有每日库存检查计划时区和执行时间。",
		InputSchema:  `{"type":"object","required":["schedule_id","timezone","time_of_day"],"properties":{"schedule_id":{"type":"integer","minimum":1},"timezone":{"type":"string","maxLength":64},"time_of_day":{"type":"string"}},"additionalProperties":false}`,
		OutputSchema: `{"type":"object"}`, RequiredPermission: "schedule.manage", RiskLevel: "LOW", TimeoutMillis: 8000, SideEffects: true, IdempotencyPolicy: "set to requested configuration", AuditPolicy: "required",
	},
	"schedule.pause": {
		Name: "schedule.pause", Description: "暂停指定的自有长期任务。",
		InputSchema:  `{"type":"object","required":["schedule_id"],"properties":{"schedule_id":{"type":"integer","minimum":1}},"additionalProperties":false}`,
		OutputSchema: `{"type":"object"}`, RequiredPermission: "schedule.manage", RiskLevel: "LOW", TimeoutMillis: 8000, SideEffects: true, IdempotencyPolicy: "set paused", AuditPolicy: "required",
	},
	"schedule.resume": {
		Name: "schedule.resume", Description: "恢复指定的自有长期任务。",
		InputSchema:  `{"type":"object","required":["schedule_id"],"properties":{"schedule_id":{"type":"integer","minimum":1}},"additionalProperties":false}`,
		OutputSchema: `{"type":"object"}`, RequiredPermission: "schedule.manage", RiskLevel: "LOW", TimeoutMillis: 8000, SideEffects: true, IdempotencyPolicy: "set active", AuditPolicy: "required",
	},
	"schedule.cancel": {
		Name: "schedule.cancel", Description: "取消指定的自有长期任务。",
		InputSchema:  `{"type":"object","required":["schedule_id"],"properties":{"schedule_id":{"type":"integer","minimum":1}},"additionalProperties":false}`,
		OutputSchema: `{"type":"object"}`, RequiredPermission: "schedule.manage", RiskLevel: "LOW", TimeoutMillis: 8000, SideEffects: true, IdempotencyPolicy: "set cancelled", AuditPolicy: "required",
	},
}

func registerPlannerTools(runtime *RuntimeExecutor) {
	runtime.Tools["retail.sales_summary"] = func(ctx context.Context, _ ExecutionContext, args map[string]any) (ToolOutput, error) {
		start, end, err := plannerDateRange(args)
		if err != nil {
			return ToolOutput{}, err
		}
		items, err := (services.RetailReadService{DB: runtime.DB}).GetProductSalesSummary(ctx, start, end, plannerLimit(args))
		if err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "retail.sales_summary", Summary: fmt.Sprintf("读取 %d 个商品的有效订单销量和订单数；未分摊订单折扣", len(items)), Data: items}, nil
	}
	runtime.Tools["retail.sales_trend"] = func(ctx context.Context, _ ExecutionContext, args map[string]any) (ToolOutput, error) {
		start, end, err := plannerDateRange(args)
		if err != nil {
			return ToolOutput{}, err
		}
		items, err := (services.RetailReadService{DB: runtime.DB}).GetProductSalesTrend(ctx, start, end, plannerLimit(args))
		if err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "retail.sales_trend", Summary: fmt.Sprintf("对比 %d 个商品的相邻等长期间销量变化；下降不证明原因", len(items)), Data: items}, nil
	}
	runtime.Tools["retail.inventory_sales"] = func(ctx context.Context, _ ExecutionContext, args map[string]any) (ToolOutput, error) {
		start, end, err := plannerDateRange(args)
		if err != nil {
			return ToolOutput{}, err
		}
		items, err := (services.RetailReadService{DB: runtime.DB}).GetInventorySalesSummary(ctx, start, end, plannerLimit(args))
		if err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "retail.inventory_sales", Summary: fmt.Sprintf("读取 %d 个商品的当前可售库存与所选期间销量", len(items)), Data: items}, nil
	}
	runtime.Tools["retail.product_inventory"] = func(ctx context.Context, _ ExecutionContext, args map[string]any) (ToolOutput, error) {
		keyword, _ := args["keyword"].(string)
		var items []models.Product
		var err error
		reader := services.RetailReadService{DB: runtime.DB}
		if strings.TrimSpace(keyword) == "" {
			items, err = reader.ListProductInventory(ctx, plannerLimit(args))
		} else {
			items, err = reader.SearchProductInventory(ctx, keyword, plannerLimit(args))
		}
		if err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "retail.product_inventory", Summary: fmt.Sprintf("读取 %d 个商品的当前可售库存", len(items)), Data: items}, nil
	}
	runtime.Tools["monitor.get_signal"] = func(ctx context.Context, ec ExecutionContext, args map[string]any) (ToolOutput, error) {
		if ec.TaskID == 0 || ec.TenantID != SingleTenant {
			return ToolOutput{}, errors.New("invalid signal scope")
		}
		var task models.AgentWorkTask
		if err := runtime.DB.WithContext(ctx).Select("id", "signal_id", "related_signal_id").Where("id = ? AND tenant_id = ?", ec.TaskID, ec.TenantID).First(&task).Error; err != nil {
			return ToolOutput{}, err
		}
		signalID := task.SignalID
		if signalID == nil {
			signalID = task.RelatedSignalID
		}
		if signalID == nil {
			return ToolOutput{Name: "monitor.get_signal", Summary: "当前任务没有关联经营信号"}, nil
		}
		if requested, exists := args["signal_id"]; exists {
			id, ok := numericID(requested)
			if !ok || id != *signalID {
				return ToolOutput{}, errors.New("requested signal is outside the current task scope")
			}
		}
		var signal models.AgentSignal
		if err := runtime.DB.WithContext(ctx).Where("id = ? AND tenant_id = ?", *signalID, ec.TenantID).First(&signal).Error; err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "monitor.get_signal", Summary: "已读取当前任务关联的规则检测信号", Data: signal}, nil
	}
	runtime.Tools["replenishment.get_draft"] = func(ctx context.Context, ec ExecutionContext, _ map[string]any) (ToolOutput, error) {
		if ec.TaskID == 0 || ec.TenantID != SingleTenant {
			return ToolOutput{}, errors.New("invalid task scope")
		}
		var draft models.AgentReplenishmentDraft
		err := runtime.DB.WithContext(ctx).Where("task_id = ? AND tenant_id = ?", ec.TaskID, ec.TenantID).First(&draft).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ToolOutput{Name: "replenishment.get_draft", Summary: "当前任务没有补货草案"}, nil
		}
		if err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "replenishment.get_draft", Summary: "已读取当前任务的内部补货草案", Data: draft}, nil
	}
	runtime.Tools["agent.get_task_status"] = func(ctx context.Context, ec ExecutionContext, _ map[string]any) (ToolOutput, error) {
		if ec.TaskID == 0 || ec.TenantID != SingleTenant {
			return ToolOutput{}, errors.New("invalid task scope")
		}
		var task models.AgentWorkTask
		if err := runtime.DB.WithContext(ctx).Select("id", "kind", "status", "step", "attempts", "error_code", "updated_at").
			Where("id = ? AND tenant_id = ?", ec.TaskID, ec.TenantID).First(&task).Error; err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "agent.get_task_status", Summary: "已读取当前任务状态", Data: task}, nil
	}
	runtime.Tools["schedule.create"] = func(ctx context.Context, ec ExecutionContext, args map[string]any) (ToolOutput, error) {
		if ec.UserID == "" || ec.Principal != "admin" {
			return ToolOutput{}, errors.New("schedule owner is not authorized")
		}
		taskType, _ := args["task_type"].(string)
		timezone, _ := args["timezone"].(string)
		timeOfDay, _ := args["time_of_day"].(string)
		schedule, err := (ScheduleService{DB: runtime.DB}).Create(ctx, ScheduleInput{
			OwnerID: ec.UserID, TaskType: taskType, Timezone: timezone, TimeOfDay: timeOfDay,
			IdempotencyKey: ec.IdempotencyKey,
		})
		if err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "schedule.create", Summary: fmt.Sprintf("已创建每日库存检查，时区%s，计划%s", schedule.Timezone, schedule.ScheduleExpression), Data: schedule}, nil
	}
	runtime.Tools["schedule.list"] = func(ctx context.Context, ec ExecutionContext, _ map[string]any) (ToolOutput, error) {
		items, err := (ScheduleService{DB: runtime.DB}).List(ctx, ec.UserID)
		if err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "schedule.list", Summary: fmt.Sprintf("读取到 %d 条当前用户自己的计划", len(items)), Data: items}, nil
	}
	runtime.Tools["schedule.history"] = func(ctx context.Context, ec ExecutionContext, args map[string]any) (ToolOutput, error) {
		id, ok := numericID(args["schedule_id"])
		if !ok || ec.UserID == "" || ec.Principal != "admin" {
			return ToolOutput{}, errors.New("invalid schedule or owner scope")
		}
		items, err := (ScheduleService{DB: runtime.DB}).History(ctx, ec.UserID, id)
		if err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "schedule.history", Summary: fmt.Sprintf("读取到该计划最近 %d 次真实运行记录", len(items)), Data: items}, nil
	}
	for _, action := range []string{"pause", "resume", "cancel"} {
		action := action
		runtime.Tools["schedule."+action] = func(ctx context.Context, ec ExecutionContext, args map[string]any) (ToolOutput, error) {
			id, ok := numericID(args["schedule_id"])
			if !ok || ec.UserID == "" || ec.Principal != "admin" {
				return ToolOutput{}, errors.New("invalid schedule or owner scope")
			}
			schedule, err := (ScheduleService{DB: runtime.DB}).SetState(ctx, ec.UserID, id, action)
			if err != nil {
				return ToolOutput{}, err
			}
			return ToolOutput{Name: "schedule." + action, Summary: "已更新当前用户自己的长期任务状态", Data: schedule}, nil
		}
	}
	runtime.Tools["schedule.update"] = func(ctx context.Context, ec ExecutionContext, args map[string]any) (ToolOutput, error) {
		id, ok := numericID(args["schedule_id"])
		if !ok || ec.UserID == "" || ec.Principal != "admin" {
			return ToolOutput{}, errors.New("invalid schedule or owner scope")
		}
		timezone, _ := args["timezone"].(string)
		timeOfDay, _ := args["time_of_day"].(string)
		schedule, err := (ScheduleService{DB: runtime.DB}).Update(ctx, ec.UserID, id, timezone, timeOfDay)
		if err != nil {
			return ToolOutput{}, err
		}
		return ToolOutput{Name: "schedule.update", Summary: "已更新当前用户自己的计划时间", Data: schedule}, nil
	}
}

func validateToolInput(input ToolInput) error {
	if input.Args == nil {
		input.Args = map[string]any{}
	}
	definition, exists := plannerToolDefinitions[input.Name]
	if !exists {
		return fmt.Errorf("tool %q has no argument schema", input.Name)
	}
	switch input.Name {
	case "retail.product_inventory":
		if err := validateKeys(input.Args, "keyword", "limit"); err != nil {
			return err
		}
		if err := validateOptionalString(input.Args, "keyword", 200); err != nil {
			return err
		}
		return validateOptionalLimit(input.Args)
	case "retail.sales_summary", "retail.sales_trend", "retail.inventory_sales":
		if err := validateKeys(input.Args, "from", "to", "limit"); err != nil {
			return err
		}
		if _, _, err := plannerDateRange(input.Args); err != nil {
			return err
		}
		return validateOptionalLimit(input.Args)
	case "retail.replenishment_context":
		if err := validateKeys(input.Args, "from", "to"); err != nil {
			return err
		}
		if len(input.Args) > 0 {
			_, _, err := plannerDateRange(input.Args)
			return err
		}
		return nil
	case "monitor.get_signal":
		if err := validateKeys(input.Args, "signal_id"); err != nil {
			return err
		}
		if raw, exists := input.Args["signal_id"]; exists {
			if _, ok := numericID(raw); !ok {
				return errors.New("signal_id must be a positive integer")
			}
		}
		return nil
	case "replenishment.get_draft", "replenishment.create_internal_draft", "agent.get_task_status":
		return validateKeys(input.Args)
	case "schedule.create":
		if err := validateKeys(input.Args, "task_type", "timezone", "time_of_day"); err != nil {
			return err
		}
		if input.Args["task_type"] != "daily_inventory_check" {
			return errors.New("unsupported recurring task type")
		}
		if err := validateOptionalString(input.Args, "timezone", 64); err != nil {
			return err
		}
		return validateOptionalTime(input.Args, "time_of_day", false)
	case "schedule.list":
		return validateKeys(input.Args)
	case "schedule.history":
		if err := validateKeys(input.Args, "schedule_id"); err != nil {
			return err
		}
		if _, ok := numericID(input.Args["schedule_id"]); !ok {
			return errors.New("schedule_id must be a positive integer")
		}
		return nil
	case "schedule.update":
		if err := validateKeys(input.Args, "schedule_id", "timezone", "time_of_day"); err != nil {
			return err
		}
		if _, ok := numericID(input.Args["schedule_id"]); !ok {
			return errors.New("schedule_id must be a positive integer")
		}
		if err := validateRequiredString(input.Args, "timezone", 64); err != nil {
			return err
		}
		return validateOptionalTime(input.Args, "time_of_day", true)
	case "schedule.pause", "schedule.resume", "schedule.cancel":
		if err := validateKeys(input.Args, "schedule_id"); err != nil {
			return err
		}
		if _, ok := numericID(input.Args["schedule_id"]); !ok {
			return errors.New("schedule_id must be a positive integer")
		}
		return nil
	default:
		return fmt.Errorf("tool %q has no input validator", definition.Name)
	}
}

func validateOptionalString(args map[string]any, name string, max int) error {
	if value, exists := args[name]; exists {
		text, ok := value.(string)
		if !ok || len([]rune(text)) > max {
			return fmt.Errorf("%s must be a string of at most %d characters", name, max)
		}
	}
	return nil
}

func validateRequiredString(args map[string]any, name string, max int) error {
	text, ok := args[name].(string)
	if !ok || strings.TrimSpace(text) == "" || len([]rune(text)) > max {
		return fmt.Errorf("%s must contain 1 to %d characters", name, max)
	}
	return nil
}

func validateOptionalTime(args map[string]any, name string, required bool) error {
	value, exists := args[name]
	if !exists && !required {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf("%s must use HH:MM", name)
	}
	parsed, err := time.Parse("15:04", text)
	if err != nil || parsed.Format("15:04") != text {
		return fmt.Errorf("%s must use HH:MM", name)
	}
	return nil
}

func validateKeys(args map[string]any, keys ...string) error {
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	for key := range args {
		if !allowed[key] {
			return fmt.Errorf("unexpected argument %q", key)
		}
	}
	return nil
}

func validateOptionalLimit(args map[string]any) error {
	if value, exists := args["limit"]; exists {
		limit, ok := integerValue(value)
		if !ok || limit < 1 || limit > 100 {
			return errors.New("limit must be an integer between 1 and 100")
		}
	}
	return nil
}

func integerValue(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		if number == float64(int(number)) {
			return int(number), true
		}
	}
	return 0, false
}

func plannerLimit(args map[string]any) int {
	if limit, ok := integerValue(args["limit"]); ok {
		return limit
	}
	return 50
}

func plannerDateRange(args map[string]any) (time.Time, time.Time, error) {
	start, startOK := parsePlannerTime(args["from"])
	end, endOK := parsePlannerTime(args["to"])
	if !startOK || !endOK || !end.After(start) || end.Sub(start) > 366*24*time.Hour {
		return time.Time{}, time.Time{}, errors.New("from/to must define an exclusive range no longer than 366 days")
	}
	return start, end, nil
}

func parsePlannerTime(value any) (time.Time, bool) {
	raw, ok := value.(string)
	if !ok || len(raw) > 40 {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, true
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	return parsed, err == nil && parsed.Format("2006-01-02") == raw
}

func (r *RuntimeExecutor) availableTools(task models.AgentWorkTask) []ToolDefinition {
	principal, permissions := taskPrincipal(task)
	_ = principal
	items := make([]ToolDefinition, 0, len(plannerToolDefinitions))
	for _, definition := range plannerToolDefinitions {
		if hasPermission(permissions, definition.RequiredPermission) && r.taskCanUseTool(task, definition.Name) {
			items = append(items, definition)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (r *RuntimeExecutor) taskCanUseTool(task models.AgentWorkTask, tool string) bool {
	if task.Kind == "schedule_inventory_check" || task.Kind == "replenishment" || task.Kind == "replenishment_analysis" {
		return false
	}
	if task.Kind == "agent_analysis" {
		if strings.HasPrefix(tool, "retail.") && tool != "retail.replenishment_context" {
			return true
		}
		// 长期计划工具：availableTools 先按 RequiredPermission 过滤，
		// 只有任务 CapabilitiesJSON 显式授予 schedule.read / schedule.manage 时才可见。
		return strings.HasPrefix(tool, "schedule.") || tool == "agent.get_task_status"
	}
	return false
}

func taskPrincipal(task models.AgentWorkTask) (string, []string) {
	if task.CreatedBy == "system_monitor" {
		return "system_monitor", []string{}
	}
	if task.CreatedBy == "system_scheduler" {
		return "system_monitor", []string{}
	}
	permissions := []string{"retail.read", "agent.task.read"}
	var requested []string
	if task.CapabilitiesJSON != "" && json.Unmarshal([]byte(task.CapabilitiesJSON), &requested) == nil {
		for _, candidate := range requested {
			switch candidate {
			case "retail.read", "agent.task.read", "schedule.read", "schedule.manage":
				permissions = append(permissions, candidate)
			}
		}
	}
	return "admin", uniquePermissions(permissions)
}

func uniquePermissions(items []string) []string {
	seen := make(map[string]bool, len(items))
	unique := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			unique = append(unique, item)
		}
	}
	return unique
}
