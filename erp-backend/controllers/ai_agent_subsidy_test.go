package controllers

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
)

func seedAgentSubsidyBalance(t *testing.T) {
	t.Helper()
	orders := []models.Order{
		{OrderNo: fmt.Sprintf("AGENT-SUB-OLD-%d", time.Now().UnixNano()), OrderStatus: 1, TotalAmount: 1000, SubsidyAmount: 100},
		{OrderNo: fmt.Sprintf("AGENT-SUB-NEW-%d", time.Now().UnixNano()), OrderStatus: 1, TotalAmount: 1000, SubsidyAmount: 80, SubsidyPendingAmount: 120},
		{OrderNo: fmt.Sprintf("AGENT-SUB-CANCEL-%d", time.Now().UnixNano()), OrderStatus: 2, TotalAmount: 1000, SubsidyAmount: 999},
	}
	if err := config.DB.Create(&orders).Error; err != nil {
		t.Fatal(err)
	}
	account := models.Account{Name: fmt.Sprintf("AGENT-SUB-ACCOUNT-%d", time.Now().UnixNano())}
	if err := config.DB.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	if err := config.DB.Create(&models.SubsidySettlement{Amount: 70, AccountID: account.ID}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestAgentSubsidyPendingUsesExistingSummaryBasis(t *testing.T) {
	setupAudit(t)
	seedAgentSubsidyBalance(t)
	overview, err := readSubsidyOverview(context.Background())
	if err != nil || overview.TotalGenerated != 220 || overview.TotalReceived != 70 || overview.PendingBalance != 150 {
		t.Fatalf("overview: %+v, %v", overview, err)
	}
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = httptest.NewRequest("GET", "/api/subsidy/summary", nil)
	GetSubsidySummary(ctx)
	if writer.Code != 200 || !strings.Contains(writer.Body.String(), `"pending_balance":150`) {
		t.Fatalf("page summary: %d %s", writer.Code, writer.Body.String())
	}
	task := agentTask{Goal: "国补还有多少没有回款", Topic: "subsidy", Operation: "lookup", Metrics: []string{"未回款金额"}, Scope: "all"}
	script := &agentMockScript{Task: func(string) agentTask { return task }}
	traceCtx, _ := agentTestContext()
	out := runAgentTask(traceCtx, agentMockRuntime(t, script), models.AIAgentSession{}, agentSendInput{Message: "国补还有多少没有回款"}, nil)
	if out.Status != "complete" || out.Task.Metrics[0] != "subsidy_pending" || !strings.Contains(out.Answer, "尚未回款 ¥150.00") || script.PlanCalls != 0 {
		t.Fatalf("agent subsidy answer: %+v", out)
	}
}
