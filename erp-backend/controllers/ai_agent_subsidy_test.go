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
		// 历史单：只有 subsidy_amount、pending=0。统一口径下不回退，不计入应回款。
		{OrderNo: fmt.Sprintf("AGENT-SUB-OLD-%d", time.Now().UnixNano()), OrderStatus: 1, TotalAmount: 1000, SubsidyAmount: 100},
		// 新口径单：以 subsidy_pending_amount 为准。
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
	// 回款池入账 70：未核销前不冲减订单口径的待回款余额。
	if err := config.DB.Create(&models.SubsidySettlement{Amount: 70, AccountID: account.ID}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestAgentSubsidyPendingUsesExistingSummaryBasis(t *testing.T) {
	setupAudit(t)
	seedAgentSubsidyBalance(t)
	// 统一口径：应回款 = SUM(pending) = 120；已回款 = SUM(按单核销) = 0；待回款 = 120。
	overview, err := readSubsidyOverview(context.Background())
	if err != nil || overview.TotalGenerated != 120 || overview.TotalReceived != 0 || overview.PendingBalance != 120 {
		t.Fatalf("overview: %+v, %v", overview, err)
	}
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = httptest.NewRequest("GET", "/api/subsidy/summary", nil)
	GetSubsidySummary(ctx)
	if writer.Code != 200 || !strings.Contains(writer.Body.String(), `"pending_balance":120`) {
		t.Fatalf("page summary: %d %s", writer.Code, writer.Body.String())
	}
	task := agentTask{Goal: "国补还有多少没有回款", Topic: "subsidy", Operation: "lookup", Metrics: []string{"未回款金额"}, Scope: "all"}
	script := &agentMockScript{Task: func(string) agentTask { return task }}
	traceCtx, _ := agentTestContext()
	out := runAgentTask(traceCtx, agentMockRuntime(t, script), models.AIAgentSession{}, agentSendInput{Message: "国补还有多少没有回款"}, nil)
	if out.Status != "complete" || out.Task.Metrics[0] != "subsidy_pending" || !strings.Contains(out.Answer, "尚未回款 ¥120.00") || script.PlanCalls != 0 {
		t.Fatalf("agent subsidy answer: %+v", out)
	}
}
