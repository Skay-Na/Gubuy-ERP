package agentnative

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"erp-backend/models"

	mysqldsn "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestMonitorWorkflowIsolatedMySQL(t *testing.T) {
	dsn := os.Getenv("ERP_AGENT_NATIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("set ERP_AGENT_NATIVE_TEST_DSN for an isolated test database")
	}
	parsed, err := mysqldsn.ParseDSN(dsn)
	if err != nil || !strings.HasSuffix(parsed.DBName, "_test") {
		t.Fatal("ERP_AGENT_NATIVE_TEST_DSN must point to a database ending in _test")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Product{}, &models.Order{}, &models.OrderItem{},
		&models.AgentSignal{}, &models.AgentGoal{}, &models.AgentWorkTask{}, &models.AgentWorkTaskStep{}, &models.AgentToolCallAudit{},
		&models.AgentPlannerCall{}, &models.AgentSchedule{}, &models.AgentScheduleRun{},
		&models.AgentReplenishmentDraft{}, &models.AgentActivity{},
	); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.Local)
	product := models.Product{Name: "test item", CloudStock: 1, LatestCost: 20}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	order := models.Order{OrderNo: fmt.Sprintf("agent-native-test-%d", time.Now().UnixNano()), OrderStatus: 1, CreatedAt: now.Add(-24 * time.Hour)}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.OrderItem{OrderID: order.ID, ProductID: product.ID, Quantity: 10, UnitPrice: 30, UnitCost: 20}).Error; err != nil {
		t.Fatal(err)
	}
	service := MonitorService{DB: db, Now: func() time.Time { return now }}
	ctx := context.Background()
	chatRunID := fmt.Sprintf("chat-test-%d", time.Now().UnixNano())
	chatTask, err := PersistChatTask(db, ChatTaskInput{
		RunID: chatRunID, Actor: "admin_test", InputJSON: `{"topic":"inventory"}`,
		OutputJSON: `{"status":"complete"}`, Status: "SUCCEEDED", Step: "completed",
	})
	if err != nil || chatTask.SignalID != nil || chatTask.CreatedBy != "admin_test" || chatTask.IdempotencyKey == nil {
		t.Fatalf("persist chat task: task=%+v err=%v", chatTask, err)
	}
	if _, err := PersistChatTask(db, ChatTaskInput{
		RunID: chatRunID, Actor: "admin_test", InputJSON: `{}`, OutputJSON: `{}`,
		Status: "SUCCEEDED", Step: "completed",
	}); err == nil {
		t.Fatal("duplicate chat idempotency key unexpectedly succeeded")
	}
	if count, err := service.Scan(ctx); err != nil || count != 1 {
		t.Fatalf("first scan: count=%d err=%v", count, err)
	}
	if count, err := service.Scan(ctx); err != nil || count != 0 {
		t.Fatalf("deduplicated scan: count=%d err=%v", count, err)
	}
	var firstTask models.AgentWorkTask
	if err := db.Order("id DESC").First(&firstTask).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := service.ProcessPending(ctx); err != nil || count != 1 {
		t.Fatalf("process task: count=%d err=%v", count, err)
	}
	var draft models.AgentReplenishmentDraft
	if err := db.Where("task_id = ?", firstTask.ID).First(&draft).Error; err != nil {
		t.Fatal(err)
	}
	if draft.ProductID != product.ID || draft.Quantity != 4 || draft.Status != "suggested" {
		t.Fatalf("unexpected draft: %+v", draft)
	}
	var steps []models.AgentWorkTaskStep
	if err := db.Where("task_id = ?", firstTask.ID).Order("sequence").Find(&steps).Error; err != nil {
		t.Fatal(err)
	}
	if len(steps) != 3 {
		t.Fatalf("durable workflow has %d steps, want 3", len(steps))
	}
	for _, step := range steps {
		if step.Status != "SUCCEEDED" || step.ToolName == "" || step.IdempotencyKey == "" || step.FinishedAt == nil {
			t.Fatalf("incomplete durable step: %+v", step)
		}
	}
	var auditCount int64
	if err := db.Model(&models.AgentToolCallAudit{}).Where("task_id = ? AND status = ?", firstTask.ID, "SUCCEEDED").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 3 {
		t.Fatalf("audited tool calls = %d, want 3", auditCount)
	}
	if err := db.Model(&models.AgentWorkTaskStep{}).Where("task_id = ? AND sequence = ?", firstTask.ID, 3).Updates(map[string]any{"status": "RUNNING", "result_summary": ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewRuntimeExecutor(db).ReconcileCompletedTasks(ctx); err != nil {
		t.Fatalf("reconcile crash window: %v", err)
	}
	if err := db.Where("task_id = ? AND sequence = ? AND status = ?", firstTask.ID, 3, "SUCCEEDED").First(&steps[2]).Error; err != nil {
		t.Fatalf("recovered completed draft step: %v", err)
	}
	approved, err := service.ReviewDraft(ctx, DraftReviewInput{
		DraftID: draft.ID, Decision: "approve", Quantity: 5, Note: "本周优先补货", Actor: "admin_test", Now: now,
	})
	if err != nil || approved.Status != "approved" || approved.Quantity != 5 || approved.ReviewedBy != "admin_test" {
		t.Fatalf("review draft: result=%+v err=%v", approved, err)
	}
	if _, err := service.ReviewDraft(ctx, DraftReviewInput{
		DraftID: draft.ID, Decision: "approve", Quantity: 5, Actor: "admin_test", Now: now,
	}); !errors.Is(err, ErrDraftNotReviewable) {
		t.Fatalf("duplicate review error = %v, want ErrDraftNotReviewable", err)
	}
	if count, err := service.ProcessPending(ctx); err != nil || count != 0 {
		t.Fatalf("repeated processing: count=%d err=%v", count, err)
	}
	var draftCount, orderCount int64
	db.Model(&models.AgentReplenishmentDraft{}).Where("task_id = ?", firstTask.ID).Count(&draftCount)
	db.Model(&models.Order{}).Where("order_no = ?", order.OrderNo).Count(&orderCount)
	if draftCount != 1 || orderCount != 1 {
		t.Fatalf("workflow duplicated a draft or wrote an order: drafts=%d orders=%d", draftCount, orderCount)
	}
	var current models.Product
	if err := db.First(&current, product.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.CloudStock != 1 {
		t.Fatalf("monitor changed cloud stock: %d", current.CloudStock)
	}

	// Simulate process restart after a persisted claim and an expired lease.
	lease := now.Add(-time.Minute)
	secondSignal := models.AgentSignal{
		TenantID: SingleTenant, Fingerprint: fmt.Sprintf("restart-test-%d", time.Now().UnixNano()), SignalType: "low_stock",
		Severity: "medium", ProductID: product.ID,
		MetricsJSON: fmt.Sprintf(`{"product_id":%d,"available":1,"sales_30_days":10,"threshold":3}`, product.ID),
		Evidence:    "test", Status: "open", DetectedAt: now,
	}
	if err := db.Create(&secondSignal).Error; err != nil {
		t.Fatal(err)
	}
	second := models.AgentWorkTask{
		TenantID: SingleTenant, SignalID: &secondSignal.ID, Kind: "replenishment", CreatedBy: "system_monitor",
		Status: "RUNNING", Step: "draft", Attempts: 1, LeaseUntil: &lease,
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := service.ProcessPending(ctx); err != nil || count != 1 {
		t.Fatalf("resume expired lease: count=%d err=%v", count, err)
	}
	if err := db.Model(&product).Update("cloud_stock", 20).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := service.Scan(ctx); err != nil || count != 0 {
		t.Fatalf("scan after stock recovery: count=%d err=%v", count, err)
	}
	var recovered models.AgentSignal
	if firstTask.SignalID == nil {
		t.Fatal("monitor task is missing signal ID")
	}
	if err := db.First(&recovered, *firstTask.SignalID).Error; err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "resolved" {
		t.Fatalf("signal was not resolved after stock recovery: %s", recovered.Status)
	}
	if err := db.First(&draft, draft.ID).Error; err != nil {
		t.Fatal(err)
	}
	if draft.Status != "approved" {
		t.Fatalf("stock recovery overwrote human review status: %s", draft.Status)
	}
	var recoveredDraft models.AgentReplenishmentDraft
	if err := db.Where("task_id = ?", second.ID).First(&recoveredDraft).Error; err != nil {
		t.Fatal(err)
	}
	if recoveredDraft.Status != "resolved" {
		t.Fatalf("unreviewed draft was not closed after stock recovery: %s", recoveredDraft.Status)
	}
}
