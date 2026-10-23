package agentnative

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"erp-backend/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrTaskLeaseLost = errors.New("agent task lease lost")

func (r *RuntimeExecutor) checkTaskLease(ctx context.Context, task models.AgentWorkTask) error {
	var current models.AgentWorkTask
	if err := r.DB.WithContext(ctx).Select("id", "status", "lease_version", "lease_until").First(&current, task.ID).Error; err != nil {
		return fmt.Errorf("reload task lease: %w", err)
	}
	return validateTaskLease(current, task, r.now())
}

func validateTaskLease(current, expected models.AgentWorkTask, now time.Time) error {
	if current.Status == "CANCELLED" {
		return ErrTaskCancelled
	}
	if current.Status != "RUNNING" || current.LeaseVersion != expected.LeaseVersion || current.LeaseUntil == nil || !current.LeaseUntil.After(now) {
		return ErrTaskLeaseLost
	}
	return nil
}

func (r *RuntimeExecutor) updateTaskStep(ctx context.Context, task models.AgentWorkTask, step models.AgentWorkTaskStep, updates map[string]any) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.AgentWorkTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "status", "lease_version", "lease_until").First(&current, task.ID).Error; err != nil {
			return err
		}
		if err := validateTaskLease(current, task, r.now()); err != nil {
			return err
		}
		result := tx.Model(&models.AgentWorkTaskStep{}).Where("id = ? AND task_id = ?", step.ID, task.ID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTaskLeaseLost
		}
		return nil
	})
}

func (r *RuntimeExecutor) createTaskStep(ctx context.Context, task models.AgentWorkTask, step *models.AgentWorkTaskStep) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.AgentWorkTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "status", "lease_version", "lease_until").First(&current, task.ID).Error; err != nil {
			return err
		}
		if err := validateTaskLease(current, task, r.now()); err != nil {
			return err
		}
		return tx.Create(step).Error
	})
}

func watchTaskLease(parent context.Context, db *gorm.DB, task models.AgentWorkTask, now func() time.Time) (context.Context, func()) {
	if now == nil {
		now = time.Now
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		nextRenewal := now().Add(time.Minute)
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				var current struct {
					Status       string
					LeaseVersion uint64
					LeaseUntil   *time.Time
				}
				checkCtx, checkCancel := context.WithTimeout(context.Background(), 2*time.Second)
				err := db.WithContext(checkCtx).Model(&models.AgentWorkTask{}).
					Select("status", "lease_version", "lease_until").Where("id = ?", task.ID).Take(&current).Error
				checkCancel()
				if err != nil || current.Status != "RUNNING" || current.LeaseVersion != task.LeaseVersion || current.LeaseUntil == nil || !current.LeaseUntil.After(now()) {
					cancel()
					return
				}
				if now().After(nextRenewal) {
					newLease := now().Add(2 * time.Minute)
					renewCtx, renewCancel := context.WithTimeout(context.Background(), 2*time.Second)
					updated := db.WithContext(renewCtx).Model(&models.AgentWorkTask{}).
						Where("id = ? AND status = ? AND lease_version = ? AND lease_until > ?", task.ID, "RUNNING", task.LeaseVersion, now()).
						Update("lease_until", newLease)
					renewCancel()
					if updated.Error != nil || updated.RowsAffected != 1 {
						cancel()
						return
					}
					runCtx, runCancel := context.WithTimeout(context.Background(), 2*time.Second)
					_ = db.WithContext(runCtx).Model(&models.AgentScheduleRun{}).
						Where("linked_task_id = ? AND status = ?", task.ID, "RUNNING").Update("lease_until", newLease).Error
					runCancel()
					nextRenewal = now().Add(time.Minute)
				}
			}
		}
	}()
	return ctx, func() {
		close(done)
		cancel()
	}
}

func (r *RuntimeExecutor) runScheduledRuleCheck(ctx context.Context, task models.AgentWorkTask) error {
	if err := r.checkTaskLease(ctx, task); err != nil {
		return err
	}
	var saved models.AgentWorkTaskStep
	key := fmt.Sprintf("agent-task:%d:rule-scan", task.ID)
	findErr := r.DB.WithContext(ctx).Where("task_id = ? AND idempotency_key = ?", task.ID, key).First(&saved).Error
	if findErr == nil && saved.Status == "SUCCEEDED" {
		return nil
	}
	if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return findErr
	}
	if errors.Is(findErr, gorm.ErrRecordNotFound) {
		var maximum int
		if err := r.DB.WithContext(ctx).Model(&models.AgentWorkTaskStep{}).Where("task_id = ?", task.ID).Select("COALESCE(MAX(sequence), 0)").Scan(&maximum).Error; err != nil {
			return err
		}
		saved = models.AgentWorkTaskStep{
			TaskID: task.ID, Sequence: maximum + 1, StepType: "rule_scan", ToolName: "monitor.rule_scan",
			InputSummary: "执行确定性的低库存规则扫描；结果不依赖 LLM",
			Status:       "RUNNING", StartedAt: timePtr(r.now()), IdempotencyKey: key,
		}
		if err := r.createTaskStep(ctx, task, &saved); err != nil {
			return err
		}
	}
	started := r.now()
	if err := r.updateTaskStep(ctx, task, saved, map[string]any{
		"status": "RUNNING", "started_at": started, "retry_count": gorm.Expr("retry_count + 1"), "error_message": "",
	}); err != nil {
		return err
	}
	signals, err := r.Monitor.Scan(ctx)
	if err != nil {
		return err
	}
	monitor := r.Monitor
	monitor.Planner = r.Planner
	processed, err := monitor.ProcessPending(ctx)
	if err != nil {
		return err
	}
	result, _ := json.Marshal(map[string]int{"new_signals": signals, "processed_tasks": processed})
	return r.savePlannerToolResult(ctx, task, saved, result, r.now())
}

func (r *RuntimeExecutor) persistPlannerDecision(ctx context.Context, task models.AgentWorkTask, callNumber, sequence int, decision PlannerDecision, usage PlannerUsage, now time.Time) error {
	if err := validatePlannerDecision(decision, DefaultPlannerLimits().MaxAnswerRunes); err != nil {
		return err
	}
	encoded, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("encode planner decision: %w", err)
	}
	key := fmt.Sprintf("agent-task:%d:planner:%d", task.ID, callNumber)
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.AgentWorkTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "status", "lease_version", "lease_until").First(&current, task.ID).Error; err != nil {
			return err
		}
		if err := validateTaskLease(current, task, r.now()); err != nil {
			return err
		}
		call := models.AgentPlannerCall{
			TaskID: task.ID, CallNumber: callNumber, Provider: truncateRunes(usage.Provider, 32),
			Model: truncateRunes(usage.Model, 100), DecisionType: decision.Type,
			ToolName: truncateRunes(decision.Tool, 80), ReasonSummary: truncateRunes(decision.Reason, 500),
			Status: "SUCCEEDED", PromptTokens: usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens, EstimatedTokens: usage.EstimatedTokens,
			TokenUsageKnown: usage.TokenUsageKnown, LatencyMillis: usage.Latency.Milliseconds(),
		}
		if err := tx.Create(&call).Error; err != nil {
			return fmt.Errorf("save planner call: %w", err)
		}
		step := models.AgentWorkTaskStep{
			TaskID: task.ID, Sequence: sequence, StepType: "planner", ToolName: "planner.decision",
			InputSummary: "已发送受限任务目标与工具目录；工具结果按数据传入",
			Status:       "SUCCEEDED", StartedAt: timePtr(now), FinishedAt: timePtr(r.now()),
			ResultSummary: string(encoded), IdempotencyKey: key,
		}
		if err := tx.Create(&step).Error; err != nil {
			return fmt.Errorf("save planner decision step: %w", err)
		}
		return nil
	})
}

func (r *RuntimeExecutor) persistPlannerFailure(ctx context.Context, task models.AgentWorkTask, callNumber int, usage PlannerUsage, cause error) error {
	message := truncateRunes(cause.Error(), 500)
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.AgentWorkTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "status", "lease_version", "lease_until").First(&current, task.ID).Error; err != nil {
			return err
		}
		if err := validateTaskLease(current, task, r.now()); err != nil {
			return err
		}
		return tx.Create(&models.AgentPlannerCall{
			TaskID: task.ID, CallNumber: callNumber,
			Provider: truncateRunes(usage.Provider, 32), Model: truncateRunes(usage.Model, 100),
			DecisionType: "INVALID", Status: "FAILED", ErrorClass: plannerErrorClass(cause),
			PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
			EstimatedTokens: usage.EstimatedTokens, TokenUsageKnown: usage.TokenUsageKnown,
			LatencyMillis: usage.Latency.Milliseconds(),
		}).Error
	})
	if err != nil {
		return fmt.Errorf("persist planner failure (%s): %w", message, err)
	}
	return nil
}

func plannerErrorClass(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case strings.Contains(message, "429"), strings.Contains(message, "rate limit"):
		return "rate_limited"
	case strings.Contains(message, "invalid planner decision"), strings.Contains(message, "invalid planner json"), strings.Contains(message, "planner response"):
		return "invalid_output"
	case strings.Contains(message, "provider") || strings.Contains(message, "model"):
		return "provider_unavailable"
	default:
		return "execution_error"
	}
}

func (r *RuntimeExecutor) savePlannerToolResult(ctx context.Context, task models.AgentWorkTask, step models.AgentWorkTaskStep, result []byte, finished time.Time) error {
	if len(result) > plannerMaxToolResultBytes {
		return errors.New("tool result exceeds persistence limit")
	}
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.AgentWorkTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "status", "lease_version", "lease_until").First(&current, task.ID).Error; err != nil {
			return err
		}
		if err := validateTaskLease(current, task, r.now()); err != nil {
			return err
		}
		updated := tx.Model(&models.AgentWorkTaskStep{}).
			Where("id = ? AND task_id = ? AND status = ?", step.ID, task.ID, "RUNNING").
			Updates(map[string]any{"status": "SUCCEEDED", "finished_at": finished, "result_summary": string(result), "error_message": ""})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrTaskLeaseLost
		}
		return nil
	})
}

func (r *RuntimeExecutor) completePlannerTask(ctx context.Context, task models.AgentWorkTask, result plannerTaskResult, now time.Time) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode planner task result: %w", err)
	}
	finished := r.now()
	stepName := "completed"
	messageStatus := "complete"
	switch result.Status {
	case "WAITING_CLARIFICATION":
		stepName, messageStatus = "clarification", "clarification"
	case "WAITING_APPROVAL":
		stepName, messageStatus = "awaiting_approval", "approval"
	case "PARTIAL":
		stepName, messageStatus = "partial", "incomplete"
		if strings.Contains(result.Reason, "budget") {
			stepName = "budget_reached"
		} else if result.Reason != "" {
			stepName = "planner_unavailable"
		}
	case "FAILED":
		stepName, messageStatus = "planner_failed", "incomplete"
	}
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.AgentWorkTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "status", "lease_version", "lease_until", "chat_message_id").First(&current, task.ID).Error; err != nil {
			return err
		}
		if err := validateTaskLease(current, task, r.now()); err != nil {
			return err
		}
		updated := tx.Model(&models.AgentWorkTask{}).
			Where("id = ? AND status = ? AND lease_version = ?", task.ID, "RUNNING", task.LeaseVersion).
			Updates(map[string]any{"status": result.Status, "step": stepName, "output_json": string(encoded), "lease_until": nil})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrTaskLeaseLost
		}
		if task.Kind == "schedule_inventory_check" {
			if err := tx.Model(&models.AgentScheduleRun{}).
				Where("linked_task_id = ? AND status IN ?", task.ID, []string{"QUEUED", "RUNNING"}).
				Updates(map[string]any{"status": result.Status, "finished_at": finished, "error": truncateRunes(result.Reason, 500), "lease_until": nil}).Error; err != nil {
				return fmt.Errorf("finish schedule run: %w", err)
			}
		}
		if current.ChatMessageID != nil {
			messageContent := result.Answer
			if messageContent == "" {
				messageContent = result.Reason
			}
			if err := tx.Model(&models.AIAgentMessage{}).Where("id = ?", *current.ChatMessageID).
				Updates(map[string]any{"content": messageContent, "status": messageStatus}).Error; err != nil {
				return fmt.Errorf("update planner chat response: %w", err)
			}
		}
		_ = now
		return nil
	})
}

func (r *RuntimeExecutor) completeFixedTask(ctx context.Context, task models.AgentWorkTask) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.AgentWorkTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "status", "lease_version", "lease_until").First(&current, task.ID).Error; err != nil {
			return err
		}
		if err := validateTaskLease(current, task, r.now()); err != nil {
			return err
		}
		updated := tx.Model(&models.AgentWorkTask{}).
			Where("id = ? AND status = ? AND lease_version = ?", task.ID, "RUNNING", task.LeaseVersion).
			Updates(map[string]any{"status": "SUCCEEDED", "step": "draft_ready", "lease_until": nil, "error_code": ""})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrTaskLeaseLost
		}
		return nil
	})
}

func timePtr(value time.Time) *time.Time { return &value }
