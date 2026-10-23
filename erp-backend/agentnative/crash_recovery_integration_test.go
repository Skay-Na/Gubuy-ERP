package agentnative

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"erp-backend/models"
	mysqldsn "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type oneActionPlanner struct {
	tool  string
	args  map[string]any
	calls int
}

func (p *oneActionPlanner) Decide(context.Context, PlannerRequest) (PlannerDecision, PlannerUsage, error) {
	p.calls++
	usage := PlannerUsage{Provider: "mock", Model: "crash-test", PromptTokens: 20, CompletionTokens: 10}
	if p.tool == "" {
		return PlannerDecision{Type: DecisionFinish, Answer: "已从恢复后的持久化证据完成。"}, usage, nil
	}
	if p.calls == 1 {
		return PlannerDecision{Type: DecisionCallTool, Tool: p.tool, Arguments: p.args}, usage, nil
	}
	return PlannerDecision{Type: DecisionFinish, Answer: "已从恢复后的持久化证据完成。"}, usage, nil
}

// TestCrashWorkerProcessHelper is run as a separate OS process by the crash
// test. The parent kills it at a persisted runtime boundary.
func TestCrashWorkerProcessHelper(t *testing.T) {
	if os.Getenv("AGENT_CRASH_HELPER") != "1" {
		return
	}
	stage := os.Getenv("AGENT_CRASH_STAGE")
	var db *gorm.DB
	if stage == "during_tool_execution" {
		db = openAgentNativeIntegrationDBWithoutMigration(t)
	} else {
		db = openAgentNativeIntegrationDB(t)
	}
	marker := os.Getenv("AGENT_CRASH_MARKER")
	if stage == "scheduler_claimed" {
		scheduleID, err := strconv.ParseUint(os.Getenv("AGENT_CRASH_SCHEDULE_ID"), 10, 32)
		if err != nil {
			t.Fatal(err)
		}
		count, err := (ScheduleService{DB: db}).DispatchDue(context.Background(), time.Now())
		if err != nil || count != 1 {
			t.Fatalf("dispatch due schedule: count=%d err=%v", count, err)
		}
		var run models.AgentScheduleRun
		if err := db.Where("schedule_id = ?", scheduleID).First(&run).Error; err != nil || run.LinkedTaskID == nil {
			t.Fatalf("claimed schedule run: %+v err=%v", run, err)
		}
		if err := os.WriteFile(marker, []byte("schedule_claimed"), 0600); err != nil {
			t.Fatal(err)
		}
		select {}
	}
	taskID, err := strconv.ParseUint(os.Getenv("AGENT_CRASH_TASK_ID"), 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	var task models.AgentWorkTask
	if err := db.First(&task, taskID).Error; err != nil {
		t.Fatal(err)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(os.Getenv("AGENT_CRASH_TOOL_ARGS")), &args); err != nil {
		t.Fatal(err)
	}
	planner := &oneActionPlanner{tool: os.Getenv("AGENT_CRASH_TOOL"), args: args}
	service := MonitorService{
		DB: db, Planner: planner, LeaseDuration: 350 * time.Millisecond,
		OnRuntimeEvent: func(event RuntimeEvent) {
			eventStage := stage
			if stage == "during_tool_execution" {
				eventStage = "before_tool_execution"
			}
			if event.TaskID == task.ID && event.Stage == eventStage {
				if err := os.WriteFile(marker, []byte(event.ToolName), 0600); err != nil {
					t.Errorf("write crash marker: %v", err)
				}
				if stage != "during_tool_execution" {
					select {}
				}
			}
		},
	}
	if _, err := service.processPending(context.Background(), uint(taskID)); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerProcessCrashRecoversPlannerAndIdempotentDraft(t *testing.T) {
	db := openAgentNativeIntegrationDB(t)
	product := models.Product{Name: fmt.Sprintf("crash-product-%d", time.Now().UnixNano()), CloudStock: 1, LatestCost: 10}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&product)

	lowTask := createCrashTask(t, db, "agent_analysis", product, "retail.product_inventory", map[string]any{"keyword": product.Name})
	runCrashWorkerAndRecover(t, db, lowTask, "before_tool_step", "retail.product_inventory", map[string]any{"keyword": product.Name})
	assertToolStepCount(t, db, lowTask.ID, "retail.product_inventory", 1)

	duringReadTask := createCrashTask(t, db, "agent_analysis", product, "retail.product_inventory", map[string]any{"keyword": product.Name})
	runCrashWorkerAndRecover(t, db, duringReadTask, "during_tool_execution", "retail.product_inventory", map[string]any{"keyword": product.Name})
	assertToolStepCount(t, db, duringReadTask.ID, "retail.product_inventory", 1)

	now := time.Now()
	signal := models.AgentSignal{
		TenantID: SingleTenant, Fingerprint: fmt.Sprintf("crash-signal-%d", now.UnixNano()), SignalType: "low_stock",
		Severity: "medium", ProductID: product.ID,
		MetricsJSON: fmt.Sprintf(`{"product_id":%d,"available":1,"sales_30_days":10,"threshold":3}`, product.ID),
		Evidence:    "crash recovery fixture", Status: "open", DetectedAt: now,
	}
	if err := db.Create(&signal).Error; err != nil {
		t.Fatal(err)
	}
	draftTask := createCrashTask(t, db, "replenishment_analysis", product, "replenishment.create_internal_draft", map[string]any{})
	draftTask.SignalID = &signal.ID
	if err := db.Model(&draftTask).Update("signal_id", signal.ID).Error; err != nil {
		t.Fatal(err)
	}
	runCrashWorkerAndRecover(t, db, draftTask, "after_tool_execution", "replenishment.create_internal_draft", map[string]any{})
	assertToolStepCount(t, db, draftTask.ID, "replenishment.create_internal_draft", 1)
	var drafts int64
	if err := db.Model(&models.AgentReplenishmentDraft{}).Where("task_id = ?", draftTask.ID).Count(&drafts).Error; err != nil {
		t.Fatal(err)
	}
	if drafts != 1 {
		t.Fatalf("internal drafts after real process restart = %d, want 1", drafts)
	}

	scheduleOwner := fmt.Sprintf("crash-scheduler-owner-%d", time.Now().UnixNano())
	schedule, err := (ScheduleService{DB: db}).Create(context.Background(), ScheduleInput{
		OwnerID: scheduleOwner, TaskType: "daily_inventory_check", Timezone: "Asia/Shanghai", TimeOfDay: "09:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&schedule).Update("next_run_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	runSchedulerCrashAndRecover(t, db, schedule)
}

func createCrashTask(t *testing.T, db *gorm.DB, kind string, product models.Product, tool string, args map[string]any) models.AgentWorkTask {
	t.Helper()
	input, _ := json.Marshal(plannerTaskInput{Goal: "从当前任务权限内检查商品 " + product.Name})
	key := fmt.Sprintf("crash-task:%s:%d", tool, time.Now().UnixNano())
	task := models.AgentWorkTask{
		TenantID: SingleTenant, Kind: kind, CreatedBy: "crash-test-admin", Status: "CREATED", Step: "queued",
		IdempotencyKey: &key, InputJSON: string(input),
	}
	if kind == "replenishment_analysis" {
		task.CreatedBy = "system_monitor"
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	return task
}

func runCrashWorkerAndRecover(t *testing.T, db *gorm.DB, task models.AgentWorkTask, stage, tool string, args map[string]any) {
	t.Helper()
	var lockConn interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
		Close() error
	}
	lockHeld := false
	releaseTableLock := func() {
		if !lockHeld || lockConn == nil {
			return
		}
		_, _ = lockConn.ExecContext(context.Background(), "UNLOCK TABLES")
		_ = lockConn.Close()
		lockHeld = false
	}
	if stage == "during_tool_execution" {
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		lockConn, err = sqlDB.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := lockConn.ExecContext(context.Background(), "LOCK TABLES products WRITE"); err != nil {
			_ = lockConn.Close()
			t.Fatal(err)
		}
		lockHeld = true
		defer releaseTableLock()
	}
	marker := t.TempDir() + "/worker-reached-boundary"
	encodedArgs, _ := json.Marshal(args)
	command := exec.Command(os.Args[0], "-test.run=^TestCrashWorkerProcessHelper$")
	command.Env = append(os.Environ(),
		"AGENT_CRASH_HELPER=1", fmt.Sprintf("AGENT_CRASH_TASK_ID=%d", task.ID),
		"AGENT_CRASH_STAGE="+stage, "AGENT_CRASH_TOOL="+tool,
		"AGENT_CRASH_TOOL_ARGS="+string(encodedArgs), "AGENT_CRASH_MARKER="+marker,
	)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if command.ProcessState != nil {
			t.Fatalf("crash worker exited before boundary: %s", output.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("worker did not reach %s: %s", stage, output.String())
	}
	if stage == "during_tool_execution" {
		deadline := time.Now().Add(5 * time.Second)
		var waiting int64
		for time.Now().Before(deadline) {
			if err := db.Raw("SELECT COUNT(*) FROM information_schema.processlist WHERE INFO LIKE '%products%' AND STATE LIKE '%lock%'").Scan(&waiting).Error; err == nil && waiting > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if waiting == 0 {
			_ = command.Process.Kill()
			_ = command.Wait()
			t.Fatalf("read-only product query did not block inside tool execution: %s", output.String())
		}
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	releaseTableLock()
	var plannerCalls, runningToolSteps int64
	db.Model(&models.AgentPlannerCall{}).Where("task_id = ?", task.ID).Count(&plannerCalls)
	db.Model(&models.AgentWorkTaskStep{}).Where("task_id = ? AND step_type = ?", task.ID, "tool").Count(&runningToolSteps)
	if plannerCalls != 1 {
		t.Fatalf("persisted planner decisions before process kill = %d, want 1", plannerCalls)
	}
	if stage == "before_tool_step" && runningToolSteps != 0 {
		t.Fatalf("tool steps before start boundary = %d, want 0", runningToolSteps)
	}
	if stage == "after_tool_execution" || stage == "during_tool_execution" {
		var step models.AgentWorkTaskStep
		if err := db.Where("task_id = ? AND step_type = ?", task.ID, "tool").First(&step).Error; err != nil || step.Status != "RUNNING" {
			t.Fatalf("step before recovery = %+v err=%v, want RUNNING", step, err)
		}
	}
	var current models.AgentWorkTask
	if err := db.First(&current, task.ID).Error; err != nil || current.Status != "RUNNING" {
		t.Fatalf("task after process kill = %+v err=%v", current, err)
	}
	if current.LeaseUntil != nil {
		wait := time.Until(*current.LeaseUntil)
		if wait > 0 {
			time.Sleep(wait + 80*time.Millisecond)
		}
	}
	recoveryPlanner := &oneActionPlanner{}
	service := MonitorService{DB: db, Planner: recoveryPlanner, LeaseDuration: 350 * time.Millisecond}
	if count, err := service.processPending(context.Background(), task.ID); err != nil || count != 1 {
		t.Fatalf("restart worker: processed=%d err=%v", count, err)
	}
	if recoveryPlanner.calls != 1 {
		t.Fatalf("recovery planner calls = %d, want one post-recovery finish call (pending decision reused)", recoveryPlanner.calls)
	}
	var completed models.AgentWorkTask
	if err := db.First(&completed, task.ID).Error; err != nil || completed.Status != "SUCCEEDED" {
		t.Fatalf("recovered task = %+v err=%v", completed, err)
	}
}

func runSchedulerCrashAndRecover(t *testing.T, db *gorm.DB, schedule models.AgentSchedule) {
	t.Helper()
	marker := t.TempDir() + "/scheduler-claimed"
	command := exec.Command(os.Args[0], "-test.run=^TestCrashWorkerProcessHelper$")
	command.Env = append(os.Environ(), "AGENT_CRASH_HELPER=1", "AGENT_CRASH_STAGE=scheduler_claimed",
		fmt.Sprintf("AGENT_CRASH_SCHEDULE_ID=%d", schedule.ID), "AGENT_CRASH_MARKER="+marker)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if command.ProcessState != nil {
			t.Fatalf("scheduler process exited before claim: %s", output.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("scheduler did not persist its claim: %s", output.String())
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	var run models.AgentScheduleRun
	if err := db.Where("schedule_id = ?", schedule.ID).First(&run).Error; err != nil || run.Status != "QUEUED" || run.LinkedTaskID == nil {
		t.Fatalf("schedule run after scheduler process kill = %+v err=%v", run, err)
	}
	var runCount int64
	if err := db.Model(&models.AgentScheduleRun{}).Where("schedule_id = ?", schedule.ID).Count(&runCount).Error; err != nil || runCount != 1 {
		t.Fatalf("schedule runs after process kill = %d err=%v, want one", runCount, err)
	}
	worker := MonitorService{DB: db, Planner: &finishPlanner{}}
	if processed, err := worker.processPending(context.Background(), *run.LinkedTaskID); err != nil || processed != 1 {
		t.Fatalf("recover claimed schedule task: processed=%d err=%v", processed, err)
	}
	if err := db.Where("schedule_id = ?", schedule.ID).First(&run).Error; err != nil || run.Status != "SUCCEEDED" {
		t.Fatalf("schedule run after worker recovery = %+v err=%v", run, err)
	}
	var tasks int64
	if err := db.Model(&models.AgentWorkTask{}).Where("id = ?", *run.LinkedTaskID).Count(&tasks).Error; err != nil || tasks != 1 {
		t.Fatalf("linked runtime tasks after recovery = %d err=%v, want one", tasks, err)
	}
}

func openAgentNativeIntegrationDBWithoutMigration(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("ERP_AGENT_NATIVE_TEST_DSN")
	parsed, err := mysqldsn.ParseDSN(dsn)
	if err != nil || !strings.HasSuffix(parsed.DBName, "_test") {
		t.Skip("set ERP_AGENT_NATIVE_TEST_DSN to an isolated MySQL database ending in _test")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func assertToolStepCount(t *testing.T, db *gorm.DB, taskID uint, tool string, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&models.AgentWorkTaskStep{}).Where("task_id = ? AND tool_name = ? AND status = ?", taskID, tool, "SUCCEEDED").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("successful %s steps = %d, want %d", tool, count, want)
	}
}
