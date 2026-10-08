package controllers

import (
	"strings"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"
)

func TestAgentRecordedPayoutUsesFinancialOutflows(t *testing.T) {
	setupAudit(t)
	var account models.Account
	if err := config.DB.First(&account).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -1, 0)
	entries := []models.FinancialLog{
		{AccountID: account.ID, FlowType: 2, Category: "员工提成", Amount: 120, CreatedAt: start.AddDate(0, 0, 2)},
		{AccountID: account.ID, FlowType: 2, Category: "工资", Amount: 1500, CreatedAt: start.AddDate(0, 0, 3)},
		{AccountID: account.ID, FlowType: 2, Category: "渠道提成", Amount: 90, CreatedAt: start.AddDate(0, 0, 4)},
		{AccountID: account.ID, FlowType: 2, Category: "员工提成", Amount: 80, CreatedAt: start.AddDate(0, -1, 2)},
	}
	for _, entry := range entries {
		if err := config.DB.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{name: "commission paid", message: "上个月我总共给员工发了多少提成", want: "¥120.00"},
		{name: "salary paid", message: "上个月发了多少工资", want: "¥1500.00"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			answer, handled, err := agentRecordedPayout(test.message)
			if err != nil || !handled || !strings.Contains(answer, test.want) {
				t.Fatalf("payout = %q, handled=%v, err=%v", answer, handled, err)
			}
		})
	}
	if _, handled, err := agentRecordedPayout("上个月应发工资是多少"); err != nil || handled {
		t.Fatalf("accrued salary should not be treated as paid: handled=%v, err=%v", handled, err)
	}
}
