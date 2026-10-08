package main

import (
	"encoding/json"
	"testing"
)

func reportEvidence(t *testing.T, rows string) json.RawMessage {
	t.Helper()
	result := `{"rows":` + rows + `}`
	raw, err := json.Marshal([]map[string]any{{"query": map[string]string{"tool": "business_report"}, "result": result}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCheckAssertions(t *testing.T) {
	t.Parallel()
	value := 900.0
	period := evalPeriod{Start: "2026-08-01", End: "2026-08-31"}
	assertion := evalAssertions{
		Topic: "sales", Metrics: []string{"sales_revenue"}, Periods: []evalPeriod{period},
		Values: []metricExpectation{{
			Metric: "sales_revenue", Start: period.Start, End: period.End, Value: &value, Tolerance: 0.01,
		}},
	}
	task := json.RawMessage(`{
  "topic": "sales",
  "metrics": [
    "sales_revenue"
  ],
  "periods": [
    {
      "start": "2026-08-01",
      "end": "2026-08-31"
    }
  ]
}`)
	tests := []struct {
		name, rows string
		task       json.RawMessage
		want       bool
	}{
		{name: "match", rows: `[
  {
    "period": {
      "start": "2026-08-01",
      "end": "2026-08-31"
    },
    "values": {
      "sales_revenue": 900
    }
  }
]`, task: task, want: true},
		{name: "within tolerance", rows: `[
  {
    "period": {
      "start": "2026-08-01",
      "end": "2026-08-31"
    },
    "values": {
      "sales_revenue": 900.005
    }
  }
]`, task: task, want: true},
		{name: "wrong amount", rows: `[
  {
    "period": {
      "start": "2026-08-01",
      "end": "2026-08-31"
    },
    "values": {
      "sales_revenue": 950
    }
  }
]`, task: task},
		{name: "wrong period", rows: `[
  {
    "period": {
      "start": "2026-09-01",
      "end": "2026-09-30"
    },
    "values": {
      "sales_revenue": 900
    }
  }
]`, task: task},
		{name: "missing metric", rows: `[
  {
    "period": {
      "start": "2026-08-01",
      "end": "2026-08-31"
    },
    "values": {}
  }
]`, task: task},
		{name: "null metric", rows: `[
  {
    "period": {
      "start": "2026-08-01",
      "end": "2026-08-31"
    },
    "values": {
      "sales_revenue": null
    }
  }
]`, task: task},
		{name: "incomplete data", rows: `[
  {
    "period": {
      "start": "2026-08-01",
      "end": "2026-08-31"
    },
    "values": {
      "sales_revenue": 900
    },
    "missing": true
  }
]`, task: task},
		{name: "duplicate reports", rows: `[
  {
    "period": {
      "start": "2026-08-01",
      "end": "2026-08-31"
    },
    "values": {
      "sales_revenue": 900
    }
  },
  {
    "period": {
      "start": "2026-08-01",
      "end": "2026-08-31"
    },
    "values": {
      "sales_revenue": 901
    }
  }
]`, task: task},
		{name: "missing task", rows: `[]`},
		{name: "malformed report", rows: `broken`, task: task},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := diagnostic{Task: tt.task, Evidence: reportEvidence(t, tt.rows)}
			results := checkAssertions(d, assertion)
			passed := true
			for _, result := range results {
				passed = passed && result.Passed
			}
			if passed != tt.want {
				t.Fatalf("results=%+v", results)
			}
		})
	}
}

func TestZeroMetricDoesNotAcceptMissingData(t *testing.T) {
	value := 0.0
	a := evalAssertions{Values: []metricExpectation{{Metric: "subsidy_pending", Value: &value}}}
	for _, tt := range []struct {
		name, rows string
		want       bool
	}{
		{name: "zero", rows: `[{"values":{"subsidy_pending":0}}]`, want: true},
		{name: "missing", rows: `[{"values":{}}]`},
		{name: "null", rows: `[{"values":{"subsidy_pending":null}}]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			results := checkAssertions(diagnostic{Evidence: reportEvidence(t, tt.rows)}, a)
			if len(results) != 1 || results[0].Passed != tt.want {
				t.Fatalf("results=%+v", results)
			}
		})
	}
}
