package agentnative

import (
	"context"
	"encoding/json"
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

type adaptiveMockPlanner struct {
	productName string
	branch      string
	calls       int
}

func (p *adaptiveMockPlanner) Decide(_ context.Context, request PlannerRequest) (PlannerDecision, PlannerUsage, error) {
	p.calls++
	usage := PlannerUsage{Provider: "mock", Model: "adaptive-test", PromptTokens: 100, CompletionTokens: 20}
	if p.calls == 1 {
		end, err := time.Parse(time.RFC3339, request.Now)
		if err != nil {
			return PlannerDecision{}, usage, err
		}
		return PlannerDecision{Type: DecisionCallTool, Tool: "retail.sales_trend", Arguments: map[string]any{
			"from": end.AddDate(0, 0, -14).Format("2006-01-02"), "to": end.Format("2006-01-02"), "limit": 100,
		}}, usage, nil
	}
	if p.calls == 2 {
		return PlannerDecision{Type: DecisionCallTool, Tool: "retail.product_inventory", Arguments: map[string]any{
			"keyword": p.productName, "limit": 10,
		}}, usage, nil
	}
	if p.calls == 3 {
		if len(request.Evidence) < 2 {
			return PlannerDecision{}, usage, fmt.Errorf("inventory evidence missing")
		}
		var rows []struct {
			StoreStock int `json:"store_stock"`
			CloudStock int `json:"cloud_stock"`
		}
		if err := json.Unmarshal(request.Evidence[len(request.Evidence)-1].Data, &rows); err != nil || len(rows) != 1 {
			return PlannerDecision{}, usage, fmt.Errorf("decode inventory evidence: %w", err)
		}
		if rows[0].StoreStock+rows[0].CloudStock < 10 {
			p.branch = "low_stock"
			return PlannerDecision{Type: DecisionCallTool, Tool: "retail.replenishment_context", Arguments: map[string]any{}}, usage, nil
		}
		p.branch = "high_stock"
		end, _ := time.Parse(time.RFC3339, request.Now)
		return PlannerDecision{Type: DecisionCallTool, Tool: "retail.sales_summary", Arguments: map[string]any{
			"from": end.AddDate(0, 0, -14).Format("2006-01-02"), "to": end.Format("2006-01-02"),
		}}, usage, nil
	}
	return PlannerDecision{Type: DecisionFinish, Answer: "已根据销量变化和当前可售库存完成分析，结论只描述相关事实。"}, usage, nil
}

func TestPlannerDynamicallyBranchesOnIsolatedMySQLEvidence(t *testing.T) {
	db := openAgentNativeIntegrationDB(t)
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.Local)
	lowName := fmt.Sprintf("planner-low-%d", time.Now().UnixNano())
	highName := fmt.Sprintf("planner-high-%d", time.Now().UnixNano())
	low := models.Product{Name: lowName, StoreStock: 1, CloudStock: 0}
	high := models.Product{Name: highName, StoreStock: 80, CloudStock: 20}
	if err := db.Create(&low).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&high).Error; err != nil {
		t.Fatal(err)
	}
	createTrendOrder := func(product models.Product, day time.Time, quantity int) {
		t.Helper()
		order := models.Order{OrderNo: fmt.Sprintf("planner-%d", time.Now().UnixNano()), OrderStatus: 1, CreatedAt: day}
		if err := db.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&models.OrderItem{OrderID: order.ID, ProductID: product.ID, Quantity: quantity, UnitPrice: 10}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, product := range []models.Product{low, high} {
		createTrendOrder(product, now.AddDate(0, 0, -21), 10)
		createTrendOrder(product, now.AddDate(0, 0, -7), 2)
	}
	defer db.Where("product_id IN ?", []uint{low.ID, high.ID}).Delete(&models.OrderItem{})
	defer db.Where("order_no LIKE ?", "planner-%").Delete(&models.Order{})
	defer db.Delete(&low)
	defer db.Delete(&high)

	for _, test := range []struct {
		name, product, branch, expectedTool string
	}{
		{name: "decline with low stock", product: lowName, branch: "low_stock", expectedTool: "retail.replenishment_context"},
		{name: "decline with ample stock", product: highName, branch: "high_stock", expectedTool: "retail.sales_summary"},
	} {
		t.Run(test.name, func(t *testing.T) {
			lease := now.Add(time.Minute)
			input, _ := json.Marshal(plannerTaskInput{Goal: "分析销量下降商品 " + test.product})
			task := models.AgentWorkTask{
				TenantID: SingleTenant, Kind: "agent_analysis", Status: "RUNNING", Step: "planner",
				CreatedBy: "planner-test-admin", LeaseVersion: 1, LeaseUntil: &lease, InputJSON: string(input),
			}
			if err := db.Create(&task).Error; err != nil {
				t.Fatal(err)
			}
			planner := &adaptiveMockPlanner{productName: test.product}
			runtime := NewRuntimeExecutor(db)
			runtime.Planner = planner
			runtime.Now = func() time.Time { return now }
			if err := runtime.ExecutePlannerTask(context.Background(), task, now); err != nil {
				t.Fatal(err)
			}
			if planner.branch != test.branch {
				t.Fatalf("adaptive branch = %q, want %q", planner.branch, test.branch)
			}
			var completed models.AgentWorkTask
			if err := db.First(&completed, task.ID).Error; err != nil || completed.Status != "SUCCEEDED" {
				t.Fatalf("task = %+v, err=%v", completed, err)
			}
			var names []string
			var steps []models.AgentWorkTaskStep
			if err := db.Where("task_id = ? AND step_type = ?", task.ID, "tool").Order("sequence").Find(&steps).Error; err != nil {
				t.Fatal(err)
			}
			for _, step := range steps {
				names = append(names, step.ToolName)
				if step.Status != "SUCCEEDED" || step.IdempotencyKey == "" {
					t.Fatalf("invalid persisted tool step: %+v", step)
				}
			}
			if len(names) != 3 || names[0] != "retail.sales_trend" || names[1] != "retail.product_inventory" || names[2] != test.expectedTool {
				t.Fatalf("dynamic tool path = %v", names)
			}
			var plannerCalls, audits int64
			db.Model(&models.AgentPlannerCall{}).Where("task_id = ?", task.ID).Count(&plannerCalls)
			db.Model(&models.AgentToolCallAudit{}).Where("task_id = ? AND status = ?", task.ID, "SUCCEEDED").Count(&audits)
			if plannerCalls != 4 || audits != 3 {
				t.Fatalf("planner calls=%d tool audits=%d, want 4 and 3", plannerCalls, audits)
			}
		})
	}
}

func openAgentNativeIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("ERP_AGENT_NATIVE_TEST_DSN")
	parsed, err := mysqldsn.ParseDSN(dsn)
	if err != nil || !strings.HasSuffix(parsed.DBName, "_test") {
		t.Skip("set ERP_AGENT_NATIVE_TEST_DSN to an isolated MySQL database ending in _test")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Product{}, &models.Order{}, &models.OrderItem{}, &models.AgentSignal{}, &models.AgentGoal{},
		&models.AgentWorkTask{}, &models.AgentWorkTaskStep{}, &models.AgentToolCallAudit{}, &models.AgentPlannerCall{},
		&models.AgentSchedule{}, &models.AgentScheduleRun{}, &models.AgentReplenishmentDraft{}, &models.AgentActivity{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}
