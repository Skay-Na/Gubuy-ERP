package controllers

import (
	"math"
	"testing"
)

func TestAgentToolRegistryRejectsInvalidCalls(t *testing.T) {
	tests := []struct {
		name string
		plan agentPlan
	}{
		{name: "unregistered", plan: agentPlan{Tool: "delete_all"}},
		{name: "nonfinite amount", plan: agentPlan{Tool: "draft_expense", Amount: math.NaN(), Category: "其他"}},
		{name: "too many order items", plan: func() agentPlan {
			plan := agentPlan{Tool: "draft_order"}
			plan.Order.Items = make([]struct {
				Name      string  `json:"name"`
				Quantity  int     `json:"quantity"`
				UnitPrice float64 `json:"unit_price"`
			}, 13)
			return plan
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateAgentToolPlan(tt.plan); err == nil {
				t.Fatalf("expected invalid tool call to be rejected: %+v", tt.plan)
			}
		})
	}
}

func TestAgentToolRegistryDeclaresSafetyMetadata(t *testing.T) {
	for name, spec := range agentToolRegistry {
		if spec.Name != name || spec.RequiredPermission == "" || spec.RiskLevel == "" ||
			spec.InputSchema == "" || spec.OutputSchema == "" || spec.Timeout <= 0 || !spec.Audit {
			t.Fatalf("tool %q lacks safety metadata: %+v", name, spec)
		}
	}
}
