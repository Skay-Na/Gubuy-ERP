package main

import (
	"path/filepath"
	"testing"
)

func TestValidateDatabaseTargetRequiresExplicitAgentTestDatabase(t *testing.T) {
	tests := []struct {
		name         string
		database     string
		confirmation string
		wantError    bool
	}{
		{name: "approved target", database: "erp_agent_eval_test", confirmation: "erp_agent_eval_test"},
		{name: "confirmation mismatch", database: "erp_agent_eval_test", confirmation: "erp_agent_other_test", wantError: true},
		{name: "production database", database: "erp_db", confirmation: "erp_db", wantError: true},
		{name: "other test database", database: "erp_cli_audit_test", confirmation: "erp_cli_audit_test", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDatabaseTarget(test.database, test.confirmation)
			if (err != nil) != test.wantError {
				t.Fatalf("validateDatabaseTarget() error=%v, wantError=%v", err, test.wantError)
			}
		})
	}
}

func TestLoadVersionedEvaluationFixture(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "agent-evals", "fixture-v1.json")
	fixture, err := loadFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.DatasetVersion != "agent-core-v1" || len(fixture.Fixture) != 2 || fixture.Expected["2026-09"]["sales_revenue"] != 1900 {
		t.Fatalf("unexpected fixture contents: %+v", fixture)
	}
}
