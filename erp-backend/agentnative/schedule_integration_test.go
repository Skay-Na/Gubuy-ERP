package agentnative

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"erp-backend/models"
)

type finishPlanner struct{ calls int }

func (p *finishPlanner) Decide(context.Context, PlannerRequest) (PlannerDecision, PlannerUsage, error) {
	p.calls++
	return PlannerDecision{Type: DecisionFinish, Answer: "库存检查已完成；本次仅执行只读分析。"}, PlannerUsage{Provider: "mock", Model: "schedule-test", PromptTokens: 10, CompletionTokens: 10}, nil
}

func TestNextDailyRunResolvesDSTAndRejectsInvalidInput(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 8, 1, 0, 0, 0, newYork)
	next, err := NextDailyRun(now, "America/New_York", "02:30")
	if err != nil {
		t.Fatal(err)
	}
	if next.In(newYork).Format("2006-01-02 15:04") != "2026-03-08 03:00" {
		t.Fatalf("spring-forward next run = %s, want first valid minute at/after 02:30", next.In(newYork))
	}
	fallbackNow := time.Date(2026, 11, 1, 0, 30, 0, 0, newYork)
	fallback, err := NextDailyRun(fallbackNow, "America/New_York", "01:30")
	if err != nil || !fallback.After(fallbackNow) || fallback.In(newYork).Format("15:04") != "01:30" {
		t.Fatalf("fall-back next run = %s, err=%v", fallback, err)
	}
	if _, err := NextDailyRun(now, "No/Such_Zone", "09:00"); err == nil {
		t.Fatal("invalid timezone accepted")
	}
	if _, err := NextDailyRun(now, "Asia/Shanghai", "9:00"); err == nil {
		t.Fatal("non-padded local time accepted")
	}
}

func TestPersistentSchedulerIsDurableAndMultiWorkerSafe(t *testing.T) {
	db := openAgentNativeIntegrationDB(t)
	now := time.Date(2026, 10, 11, 8, 58, 0, 0, time.Local)
	owner := fmt.Sprintf("schedule-owner-%d", time.Now().UnixNano())
	service := ScheduleService{DB: db, Now: func() time.Time { return now }}
	schedule, err := service.Create(context.Background(), ScheduleInput{
		OwnerID: owner, TaskType: "daily_inventory_check", Timezone: "Asia/Shanghai", TimeOfDay: "09:00", IdempotencyKey: "first-request",
	})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := service.Create(context.Background(), ScheduleInput{
		OwnerID: owner, TaskType: "daily_inventory_check", Timezone: "Asia/Shanghai", TimeOfDay: "09:00", IdempotencyKey: "second-request",
	})
	if err != nil || duplicate.ID != schedule.ID {
		t.Fatalf("semantic duplicate schedule = %+v err=%v", duplicate, err)
	}
	if schedule.NextRunAt.In(time.FixedZone("CST", 8*60*60)).Format("15:04") != "09:00" {
		t.Fatalf("next run local time = %s, want 09:00", schedule.NextRunAt)
	}
	due := schedule.NextRunAt.Add(10 * time.Second)
	services := []ScheduleService{{DB: db}, {DB: db}}
	counts := make([]int, len(services))
	var errs [2]error
	var wait sync.WaitGroup
	for index := range services {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			counts[i], errs[i] = services[i].DispatchDue(context.Background(), due)
		}(index)
	}
	wait.Wait()
	if errs[0] != nil || errs[1] != nil || counts[0]+counts[1] != 1 {
		t.Fatalf("competing schedule workers counts=%v errs=%v", counts, errs)
	}
	var runs []models.AgentScheduleRun
	if err := db.Where("schedule_id = ?", schedule.ID).Find(&runs).Error; err != nil || len(runs) != 1 || runs[0].LinkedTaskID == nil {
		t.Fatalf("schedule runs = %+v err=%v", runs, err)
	}
	var task models.AgentWorkTask
	if err := db.First(&task, *runs[0].LinkedTaskID).Error; err != nil {
		t.Fatal(err)
	}
	if task.Kind != "schedule_inventory_check" || task.CreatedBy != "system_scheduler" || task.Status != "CREATED" {
		t.Fatalf("scheduled runtime task = %+v", task)
	}
	_, permissions := taskPrincipal(task)
	for _, forbidden := range []string{"replenishment.draft.create", "schedule.manage", "payments.write", "orders.write"} {
		if hasPermission(permissions, forbidden) {
			t.Fatalf("scheduled task unexpectedly has %q", forbidden)
		}
	}
	if runtime := NewRuntimeExecutor(db); runtime.taskCanUseTool(task, "replenishment.create_internal_draft") {
		t.Fatal("scheduled task can use internal replenishment draft tool")
	}

	planner := &finishPlanner{}
	worker := MonitorService{DB: db, Planner: planner}
	if processed, err := worker.ProcessPending(context.Background()); err != nil || processed != 1 {
		t.Fatalf("scheduled task runtime processing = %d, err=%v", processed, err)
	}
	if planner.calls != 1 {
		t.Fatalf("scheduled planner calls = %d, want 1", planner.calls)
	}
	var completed models.AgentWorkTask
	if err := db.First(&completed, task.ID).Error; err != nil || completed.Status != "SUCCEEDED" {
		t.Fatalf("scheduled task completion = %+v err=%v", completed, err)
	}
	if err := db.Where("schedule_id = ?", schedule.ID).First(&runs[0]).Error; err != nil || runs[0].Status != "SUCCEEDED" || runs[0].ActualStartedAt == nil || runs[0].FinishedAt == nil {
		t.Fatalf("scheduled run history = %+v err=%v", runs[0], err)
	}
	history, err := service.History(context.Background(), owner, schedule.ID)
	if err != nil || len(history) != 1 || history[0].LinkedTaskID == nil || history[0].Status != "SUCCEEDED" {
		t.Fatalf("owner-scoped schedule history = %+v err=%v", history, err)
	}
	if foreignHistory, err := service.History(context.Background(), owner+"-other", schedule.ID); err != nil || len(foreignHistory) != 0 {
		t.Fatalf("cross-owner schedule history = %+v err=%v", foreignHistory, err)
	}

	updated, err := (ScheduleService{DB: db, Now: func() time.Time { return now }}).Update(context.Background(), owner, schedule.ID, "Asia/Shanghai", "10:30")
	if err != nil || updated.ScheduleExpression != "daily 10:30" || updated.NextRunAt.In(time.FixedZone("CST", 8*60*60)).Format("15:04") != "10:30" {
		t.Fatalf("updated schedule = %+v err=%v", updated, err)
	}
	paused, err := (ScheduleService{DB: db, Now: func() time.Time { return now }}).SetState(context.Background(), owner, schedule.ID, "pause")
	if err != nil || paused.Enabled || paused.Status != "paused" {
		t.Fatalf("paused schedule = %+v err=%v", paused, err)
	}
	if count, err := (ScheduleService{DB: db}).DispatchDue(context.Background(), updated.NextRunAt.Add(time.Minute)); err != nil || count != 0 {
		t.Fatalf("paused schedule dispatched: count=%d err=%v", count, err)
	}
	resumed, err := (ScheduleService{DB: db, Now: func() time.Time { return now }}).SetState(context.Background(), owner, schedule.ID, "resume")
	if err != nil || !resumed.Enabled || resumed.Status != "active" {
		t.Fatalf("resumed schedule = %+v err=%v", resumed, err)
	}
	newService := ScheduleService{DB: db, Now: func() time.Time { return now }}
	items, err := newService.List(context.Background(), owner)
	if err != nil || len(items) != 1 || items[0].ID != schedule.ID {
		t.Fatalf("schedule after service reconstruction = %+v err=%v", items, err)
	}
	if _, err := newService.SetState(context.Background(), owner, schedule.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
}
