package controllers

import (
	"bytes"
	"encoding/json"
	"erp-backend/middlewares"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestPendingInboundValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.POST("/api/pending-inbounds", CreatePendingInbound)

	body := map[string]interface{}{
		"warehouse_type": 1,
		"quantity":       10,
	}
	jsonValue, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/pending-inbounds", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPendingInboundApproveValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.POST("/api/pending-inbounds/:id/approve", ApprovePendingInbound)

	body := map[string]interface{}{
		"account_id": 1,
	}
	jsonValue, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/pending-inbounds/1/approve", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPendingInboundRejectsUnsupportedWarehouseBeforeDatabaseAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/pending-inbounds", CreatePendingInbound)

	body := map[string]interface{}{
		"product_id":     1,
		"warehouse_type": 4,
		"quantity":       1,
	}
	jsonValue, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/pending-inbounds", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsupported warehouse status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHistoricalCostRepairRequiresExplicitConfirmationBeforeDatabaseAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/system/historical-cost-repair", RepairHistoricalCost)

	body := map[string]interface{}{
		"log_id":    1,
		"unit_cost": 100,
	}
	jsonValue, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/system/historical-cost-repair", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminAuthRejectsEmployeeTokenBeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/admin-only", middlewares.AdminAuthRequired(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	token, err := middlewares.GenerateToken("emp_123")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", "/admin-only", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("employee admin access status = %d, want %d", w.Code, http.StatusForbidden)
	}
}
