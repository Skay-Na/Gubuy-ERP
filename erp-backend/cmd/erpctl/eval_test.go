package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluateReportsFailuresAndReusesSession(t *testing.T) {
	t.Setenv("ERP_TOKEN", "token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{"message": map[string]any{
			"id": 8, "session_id": 4, "status": "complete", "content": "销售额900",
		}, "debug": true}
		if r.URL.Path == "/api/ai/agent/sessions/4" {
			data = map[string]any{"session": map[string]any{"id": 4, "debug": true}}
		}
		isMessage := r.URL.Path == "/api/ai/agent/sessions/4/messages"
		isTrace := r.URL.Path == "/api/ai/agent/messages/8/trace"
		isSession := r.URL.Path == "/api/ai/agent/sessions/4"
		if !isMessage && !isTrace && !isSession {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": data}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "cases.json")
	body := `[
        {"name":"pass","question":"销售额","contains":["900"]},
        {"name":"fail","question":"销售额","contains":["1900"]}
    ]`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	args := []string{"agent", "eval", "--cases", path, "--url", server.URL, "--session", "4", "--json"}
	if code := run(context.Background(), args, &out, &stderr); code != 3 {
		t.Fatalf("code=%d stderr=%s", code, &stderr)
	}
	var report struct {
		Passed, Failed int
		Results        []evalResult
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Passed != 1 || report.Failed != 1 || len(report.Results[1].Failures) != 1 {
		t.Fatalf("report=%+v", report)
	}
}

func TestEvaluateRejectsBadAssertionsBeforeRequest(t *testing.T) {
	tests := []struct{ name, body string }{
		{name: "missing value", body: `[
  {
    "name": "bad",
    "question": "test",
    "assertions": {
      "values": [
        {
          "metric": "sales_revenue"
        }
      ]
    }
  }
]`},
		{name: "negative tolerance", body: `[
  {
    "name": "bad",
    "question": "test",
    "assertions": {
      "values": [
        {
          "metric": "sales_revenue",
          "value": 0,
          "tolerance": -1
        }
      ]
    }
  }
]`},
		{name: "partial period", body: `[
  {
    "name": "bad",
    "question": "test",
    "assertions": {
      "values": [
        {
          "metric": "sales_revenue",
          "value": 0,
          "start": "2026-08-01"
        }
      ]
    }
  }
]`},
		{name: "typo", body: `[
  {
    "name": "bad",
    "question": "test",
    "assertions": {
      "metricz": [
        "sales_revenue"
      ]
    }
  }
]`},
		{name: "trailing document", body: `[{"name":"bad","question":"test"}] {}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cases.json")
			if err := os.WriteFile(path, []byte(tt.body), 0600); err != nil {
				t.Fatal(err)
			}
			// A nil HTTP client ensures validation completes before any HTTP operation.
			if _, err := (client{token: "token"}).evaluate(context.Background(), options{cases: path}); err == nil {
				t.Fatal("invalid assertions accepted")
			}
		})
	}
}

func TestEvaluateAsOfWritesRestrictedComparisonReport(t *testing.T) {
	t.Setenv("ERP_TOKEN", "token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/messages") {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["reference_date"] != "2026-10-01" {
				t.Errorf("reference_date=%q", body["reference_date"])
			}
		}
		data := map[string]any{"message": map[string]any{"id": 8, "session_id": 4, "status": "complete", "content": "答复"}, "debug": true}
		if r.URL.Path == "/api/ai/agent/sessions/4" {
			data = map[string]any{"session": map[string]any{"id": 4, "debug": true}}
		}
		if r.URL.Path == "/api/ai/agent/messages/8/trace" {
			data["trace"] = map[string]any{"milliseconds": 12, "calls": []any{map[string]any{"prompt_tokens": 2, "completion_tokens": 3}}, "tools": []any{map[string]any{}}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": data})
	}))
	defer server.Close()
	dir := t.TempDir()
	cases := filepath.Join(dir, "cases.json")
	report := filepath.Join(dir, "report.json")
	baseline := filepath.Join(dir, "baseline.json")
	if err := os.WriteFile(cases, []byte(`[{"name":"fixed","dataset_version":"v1","as_of":"2026-10-01","question":"测试"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baseline, []byte(`{"total":1,"passed":0,"failed":1,"results":[{"name":"fixed","passed":false}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	args := []string{"agent", "eval", "--cases", cases, "--report", report, "--compare", baseline, "--url", server.URL, "--session", "4", "--json"}
	if code := run(context.Background(), args, &out, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	info, err := os.Stat(report)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("report permissions=%o", info.Mode().Perm())
	}
	var decoded map[string]any
	body, err := os.ReadFile(report)
	if err != nil || json.Unmarshal(body, &decoded) != nil {
		t.Fatalf("report decode error=%v", err)
	}
	if decoded["passed"].(float64) != 1 || decoded["prompt_tokens"].(float64) != 2 || decoded["tool_calls"].(float64) != 1 {
		t.Fatalf("report summary=%v", decoded)
	}
	comparison := decoded["comparison"].(map[string]any)
	if comparison["pass_delta"].(float64) != 1 || len(comparison["improvements"].([]any)) != 1 {
		t.Fatalf("comparison=%v", comparison)
	}
}
