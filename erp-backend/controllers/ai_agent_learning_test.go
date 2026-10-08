package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
)

func learningTestRouter() *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("username", "agent-learning-test"); c.Next() })
	router.POST("/knowledge", CreateAIAgentKnowledge)
	router.PUT("/knowledge/:knowledge_id", UpdateAIAgentKnowledge)
	router.GET("/knowledge", ListAIAgentKnowledge)
	router.POST("/predictions/evaluate", EvaluateAIAgentPredictions)
	router.GET("/predictions", ListAIAgentPredictions)
	return router
}

func TestAIAgentKnowledgeRequiresApprovalAndExpires(t *testing.T) {
	setupAudit(t)
	if err := config.DB.Where("submitted_by = ?", "agent-learning-test").Delete(&models.AIAgentKnowledge{}).Error; err != nil {
		t.Fatal(err)
	}
	router := learningTestRouter()
	create := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/knowledge", strings.NewReader(`{"topic":"sales","category":"metric_alias","content":"销售额指有效订单的折后金额。"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(create, request)
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Data models.AIAgentKnowledge `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.Status != "pending" {
		t.Fatalf("new knowledge status=%q", created.Data.Status)
	}
	if text, ids, err := approvedAIAgentKnowledge("sales"); err != nil || text != "" || len(ids) != 0 {
		t.Fatalf("pending knowledge became active: text=%q ids=%v err=%v", text, ids, err)
	}
	approve := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/knowledge/"+strconv.FormatUint(uint64(created.Data.ID), 10), strings.NewReader(`{"status":"approved"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(approve, request)
	if approve.Code != http.StatusOK {
		t.Fatalf("approve status=%d body=%s", approve.Code, approve.Body.String())
	}
	text, ids, err := approvedAIAgentKnowledge("sales")
	if err != nil || !strings.Contains(text, "折后金额") || len(ids) != 1 || ids[0] != created.Data.ID {
		t.Fatalf("approved knowledge=%q ids=%v err=%v", text, ids, err)
	}
	expired := time.Now().Add(-time.Minute)
	if err := config.DB.Model(&models.AIAgentKnowledge{}).Where("id = ?", created.Data.ID).Update("expires_at", expired).Error; err != nil {
		t.Fatal(err)
	}
	if text, ids, err := approvedAIAgentKnowledge("sales"); err != nil || text != "" || len(ids) != 0 {
		t.Fatalf("expired knowledge became active: text=%q ids=%v err=%v", text, ids, err)
	}
}

func TestAgentRunCarriesPromptAndBuildVersions(t *testing.T) {
	if got := agentPromptVersion(); len(got) != 34 || !strings.HasPrefix(got, "p-") {
		t.Fatalf("prompt version=%q", got)
	}
	if got := agentToolVersion(); got == "" {
		t.Fatal("tool version is empty")
	}
}

func TestPredictionSnapshotBacktestUsesSavedScopeAndBaseline(t *testing.T) {
	setupAudit(t)
	if err := config.DB.Where("owner = ?", "agent-learning-test").Delete(&models.AIAgentPredictionSnapshot{}).Error; err != nil {
		t.Fatal(err)
	}
	month := time.Now().AddDate(0, -1, 0)
	monthStart := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.Local)
	productID := ensureProduct(t, "Agent回测固定商品", 100, 50, 5)
	order := models.Order{OrderNo: "AGENT-BACKTEST-" + strconv.FormatInt(time.Now().UnixNano(), 10), OrderStatus: 1, TotalAmount: 123, CreatedAt: monthStart.AddDate(0, 0, 3), OrderItems: []models.OrderItem{{ProductID: productID, UnitPrice: 123, UnitCost: 50, Quantity: 1}}}
	if err := config.DB.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	evidence := []agentEvidence{{Plan: agentPlan{Tool: "data_query", Data: agentDataRequest{Dataset: "orders", Operation: "forecast", Metric: "total_amount", Aggregation: "sum", TimeField: "created_at", Filters: map[string]any{"order_status": float64(1)}}}, Result: "预测\n结果：{\"forecast_available\":true,\"recent_3_month_mean\":80,\"forecast\":[{\"month\":\"" + monthStart.Format("2006-01") + "\",\"baseline\":100}]}"}}
	message := models.AIAgentMessage{RunID: "run-backtest", ID: 991}
	if err := persistAgentPredictionSnapshots("agent-learning-test", message, evidence, time.Now().AddDate(0, -2, 0)); err != nil {
		t.Fatal(err)
	}
	var snapshot models.AIAgentPredictionSnapshot
	if err := config.DB.Where("assistant_message_id = ?", message.ID).First(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	if snapshot.ForecastValue != 100 || snapshot.NaiveValue != 80 || !strings.Contains(snapshot.FiltersJSON, "order_status") {
		t.Fatalf("snapshot did not preserve forecast inputs: %+v", snapshot)
	}
	router := learningTestRouter()
	backtest := httptest.NewRecorder()
	router.ServeHTTP(backtest, httptest.NewRequest(http.MethodPost, "/predictions/evaluate", bytes.NewReader(nil)))
	if backtest.Code != http.StatusOK {
		t.Fatalf("backtest status=%d body=%s", backtest.Code, backtest.Body.String())
	}
	if err := config.DB.First(&snapshot, snapshot.ID).Error; err != nil {
		t.Fatal(err)
	}
	if snapshot.ActualValue == nil || *snapshot.ActualValue != 123 || snapshot.EvaluatedAt == nil {
		t.Fatalf("actual result not persisted: %+v", snapshot)
	}
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/predictions", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("prediction list status=%d body=%s", list.Code, list.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	data := response["data"].(map[string]any)
	metrics := data["metrics"].(map[string]any)
	if metrics["mae_forecast"].(float64) != 23 || metrics["mae_naive_3_month"].(float64) != 43 {
		t.Fatalf("unexpected backtest metrics: %s", fmt.Sprint(metrics))
	}
}
