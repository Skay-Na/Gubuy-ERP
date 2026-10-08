package controllers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"
)

func TestAgentDataCatalogExcludesSecrets(t *testing.T) {
	setupAudit(t)
	catalog := agentDataCatalog()
	for _, expected := range []string{"orders", "financial_logs", "inventory_logs", "daily_attendance", "subsidy_settlements"} {
		if !strings.Contains(catalog, expected) {
			t.Fatalf("catalog missing %s", expected)
		}
	}
	for _, forbidden := range []string{"pin_code", "password", "api_key", "image_hash", "extracted"} {
		if strings.Contains(catalog, forbidden) {
			t.Fatalf("catalog exposed %s", forbidden)
		}
	}
	response, err := agentDataQuery(context.Background(), agentDataRequest{
		Dataset: "employees", Operation: "rows", Fields: []string{"id", "pin_code"},
	})
	if err != nil || !strings.Contains(response, "字段") {
		t.Fatalf("hidden field query = %q, err = %v", response, err)
	}
	response, err = agentDataQuery(context.Background(), agentDataRequest{
		Dataset: "orders", Operation: "aggregate", Aggregation: "sum", Metric: "total_amount; DROP TABLE orders",
	})
	if err != nil || !strings.Contains(response, "查询参数无效") {
		t.Fatalf("invalid column query = %q, err = %v", response, err)
	}
}

func TestAgentTrendDiagnosticsUsesCompletePeriods(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.Local)
	points := []agentTrendPoint{
		{Bucket: "2026-06", Value: 10},
		{Bucket: "2026-07", Value: 10},
		{Bucket: "2026-08", Value: 10},
		{Bucket: "2026-09", Value: 30},
		{Bucket: "2026-10", Value: 1000},
	}
	result := agentTrendDiagnostics(points, "month", now)
	if result["latest_complete_bucket"] != "2026-09" || result["change_pct"] != float64(200) || result["anomaly"] != true {
		t.Fatalf("trend diagnostics = %+v", result)
	}
}

func TestAgentDataQueryAggregatesAndForecastsOrders(t *testing.T) {
	setupAudit(t)
	now := time.Now()
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	productID := ensureProduct(t, "Agent数据分析商品", 100, 60, 10)
	products, err := agentDataQuery(context.Background(), agentDataRequest{
		Dataset: "products", Operation: "rows", Fields: []string{"id", "name"},
		Filters: map[string]any{"name": map[string]any{"contains": "数据分析"}}, Limit: 1,
	})
	if err != nil || !strings.Contains(products, "Agent数据分析商品") {
		t.Fatalf("filtered products = %q, err = %v", products, err)
	}
	for index, amount := range []float64{100, 200, 300} {
		order := models.Order{
			OrderNo:     "AGENT-DATA-" + thisMonth.AddDate(0, index-3, 0).Format("200601"),
			OrderStatus: 1, TotalAmount: amount,
			CreatedAt:  thisMonth.AddDate(0, index-3, 3),
			OrderItems: []models.OrderItem{{ProductID: productID, UnitPrice: amount, UnitCost: amount / 2, Quantity: 1}},
		}
		if err := config.DB.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
	}
	canceled := models.Order{
		OrderNo:     "AGENT-DATA-CANCELED-" + thisMonth.Format("200601"),
		OrderStatus: 2, TotalAmount: 900, CreatedAt: thisMonth.AddDate(0, -1, 3),
		OrderItems: []models.OrderItem{{ProductID: productID, UnitPrice: 900, UnitCost: 450, Quantity: 1}},
	}
	if err := config.DB.Create(&canceled).Error; err != nil {
		t.Fatal(err)
	}
	start := thisMonth.AddDate(0, -3, 0).Format("2006-01-02")
	end := thisMonth.AddDate(0, 0, -1).Format("2006-01-02")
	answer, err := agentDataQuery(context.Background(), agentDataRequest{
		Dataset: "orders", Operation: "aggregate", Aggregation: "sum", Metric: "total_amount",
		StartDate: start, EndDate: end,
	})
	if err != nil || !strings.Contains(answer, `"value":600`) {
		t.Fatalf("aggregate = %q, err = %v", answer, err)
	}
	itemMargin, err := agentDataQuery(context.Background(), agentDataRequest{
		Dataset: "sales_items", Operation: "aggregate", Aggregation: "sum", Metric: "line_margin", GroupBy: "product_id",
	})
	if err != nil || !strings.Contains(itemMargin, `"value":300`) || strings.Contains(itemMargin, `"value":750`) {
		t.Fatalf("valid item margin = %q, err = %v", itemMargin, err)
	}
	answer, err = agentDataQuery(context.Background(), agentDataRequest{
		Dataset: "orders", Operation: "trend", Aggregation: "sum", Metric: "total_amount",
		StartDate: start, EndDate: end, Grain: "month",
	})
	if err != nil || !strings.Contains(answer, `"value":300`) || strings.Contains(answer, `"value":1200`) {
		t.Fatalf("trend = %q, err = %v", answer, err)
	}
	referenceContext := context.WithValue(context.Background(), agentReferenceDateKey{}, thisMonth)
	answer, err = agentDataQuery(referenceContext, agentDataRequest{
		Dataset: "orders", Operation: "forecast", Aggregation: "sum", Metric: "total_amount",
	})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ForecastAvailable bool `json:"forecast_available"`
		ObservedMonths    int  `json:"observed_months"`
		Forecast          []struct {
			Month string `json:"month"`
		} `json:"forecast"`
	}
	parts := strings.SplitN(answer, "结果：", 2)
	if len(parts) != 2 || json.Unmarshal([]byte(parts[1]), &result) != nil || !result.ForecastAvailable || result.ObservedMonths != 3 || len(result.Forecast) == 0 || result.Forecast[0].Month != thisMonth.Format("2006-01") {
		t.Fatalf("forecast = %q", answer)
	}
}
