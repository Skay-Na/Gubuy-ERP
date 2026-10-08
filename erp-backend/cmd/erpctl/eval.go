package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

type evalCase struct {
	Name           string         `json:"name"`
	DatasetVersion string         `json:"dataset_version,omitempty"`
	AsOf           string         `json:"as_of,omitempty"`
	Question       string         `json:"question"`
	Turns          []string       `json:"turns,omitempty"`
	Contains       []string       `json:"contains"`
	NotContains    []string       `json:"not_contains"`
	Assertions     evalAssertions `json:"assertions,omitempty"`
}
type evalResult struct {
	Name             string            `json:"name"`
	DatasetVersion   string            `json:"dataset_version,omitempty"`
	AsOf             string            `json:"as_of,omitempty"`
	Question         string            `json:"question"`
	Passed           bool              `json:"passed"`
	Failures         []string          `json:"failures"`
	Diagnostic       diagnostic        `json:"diagnostic"`
	Assertions       []assertionResult `json:"assertions"`
	TurnResults      []evalTurnResult  `json:"turn_results,omitempty"`
	DurationMS       int64             `json:"duration_ms"`
	PromptTokens     int               `json:"prompt_tokens"`
	CompletionTokens int               `json:"completion_tokens"`
	ToolCalls        int               `json:"tool_calls"`
}

type evalTurnResult struct {
	Question string          `json:"question"`
	Message  message         `json:"message"`
	Trace    json.RawMessage `json:"trace,omitempty"`
	Error    string          `json:"error,omitempty"`
}

func (c client) evaluate(ctx context.Context, o options) (any, error) {
	raw, err := os.ReadFile(o.cases)
	if err != nil {
		return nil, fmt.Errorf("read evaluation cases: %w", err)
	}
	cases := []evalCase{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cases); err != nil {
		return nil, fmt.Errorf("decode evaluation cases: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("evaluation cases must be one JSON array")
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("evaluation cases are empty")
	}
	seen := map[string]bool{}
	for _, item := range cases {
		if strings.TrimSpace(item.Name) == "" || (strings.TrimSpace(item.Question) == "" && len(item.Turns) == 0) || (strings.TrimSpace(item.Question) != "" && len(item.Turns) > 0) {
			return nil, fmt.Errorf("each evaluation case requires either question or turns")
		}
		if seen[item.Name] {
			return nil, fmt.Errorf("duplicate case name %q", item.Name)
		}
		seen[item.Name] = true
		if item.AsOf != "" {
			asOf, err := time.Parse("2006-01-02", item.AsOf)
			if err != nil || asOf.Format("2006-01-02") != item.AsOf {
				return nil, fmt.Errorf("case %q has invalid as_of (expected YYYY-MM-DD)", item.Name)
			}
			if !o.debug {
				return nil, fmt.Errorf("case %q uses as_of but evaluation is not in debug mode", item.Name)
			}
		}
		turns := item.Turns
		if len(turns) == 0 {
			turns = []string{item.Question}
		}
		for _, turn := range turns {
			if len([]rune(turn)) > 500 || strings.TrimSpace(turn) == "" {
				return nil, fmt.Errorf("case %q has an empty or overlong turn", item.Name)
			}
		}
		if err := item.Assertions.validate(); err != nil {
			return nil, fmt.Errorf("case %q: %w", item.Name, err)
		}
	}
	if c.token == "" {
		return nil, fmt.Errorf("run erpctl auth login or set ERP_TOKEN")
	}
	results := []evalResult{}
	passed := 0
	for _, item := range cases {
		if err := ctx.Err(); err != nil {
			return map[string]any{"results": results}, fmt.Errorf("evaluation cancelled: %w", err)
		}
		turns := item.Turns
		if len(turns) == 0 {
			turns = []string{item.Question}
		}
		result := evalResult{Name: item.Name, DatasetVersion: item.DatasetVersion, AsOf: item.AsOf, Question: strings.Join(turns, " → "), Failures: []string{}, TurnResults: []evalTurnResult{}}
		caseOptions := o
		caseOptions.asOf = item.AsOf
		var d diagnostic
		for _, question := range turns {
			turnStarted := time.Now()
			var askErr error
			d, askErr = c.ask(ctx, question, caseOptions)
			turnResult := evalTurnResult{Question: question, Message: d.Message, Trace: d.Trace}
			result.DurationMS += time.Since(turnStarted).Milliseconds()
			var runTrace struct {
				Milliseconds int64 `json:"milliseconds"`
				Calls        []struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
				} `json:"calls"`
				Tools []json.RawMessage `json:"tools"`
			}
			if json.Unmarshal(d.Trace, &runTrace) == nil {
				if runTrace.Milliseconds > 0 {
					result.DurationMS += runTrace.Milliseconds - time.Since(turnStarted).Milliseconds()
				}
				result.ToolCalls += len(runTrace.Tools)
				for _, call := range runTrace.Calls {
					result.PromptTokens += call.PromptTokens
					result.CompletionTokens += call.CompletionTokens
				}
			}
			if askErr != nil {
				turnResult.Error = askErr.Error()
				result.Failures = append(result.Failures, askErr.Error())
			}
			result.TurnResults = append(result.TurnResults, turnResult)
			if d.Message.Status != "" && d.Message.Status != "complete" {
				result.Failures = append(result.Failures, "answer status is not complete")
			}
			if o.session == 0 && d.Message.SessionID != 0 {
				caseOptions.session = d.Message.SessionID
			}
			if askErr != nil {
				break
			}
		}
		result.Diagnostic = d
		if d.Message.Status != "complete" {
			found := false
			for _, failure := range result.Failures {
				if failure == "answer status is not complete" {
					found = true
					break
				}
			}
			if !found {
				result.Failures = append(result.Failures, "answer status is not complete")
			}
		}
		for _, text := range item.Contains {
			if !strings.Contains(d.Message.Content, text) {
				result.Failures = append(result.Failures, "missing expected text: "+text)
			}
		}
		for _, text := range item.NotContains {
			if strings.Contains(d.Message.Content, text) {
				result.Failures = append(result.Failures, "unexpected text: "+text)
			}
		}
		result.Assertions = checkAssertions(d, item.Assertions)
		for _, assertion := range result.Assertions {
			if !assertion.Passed {
				result.Failures = append(result.Failures, "assertion failed: "+assertion.Check+" "+assertion.Detail)
			}
		}
		result.Passed = len(result.Failures) == 0
		if result.Passed {
			passed++
		}
		results = append(results, result)
	}
	durations := make([]int64, 0, len(results))
	var promptTokens, completionTokens, toolCalls int
	for _, result := range results {
		durations = append(durations, result.DurationMS)
		promptTokens += result.PromptTokens
		completionTokens += result.CompletionTokens
		toolCalls += result.ToolCalls
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	percentile := func(p float64) int64 {
		if len(durations) == 0 {
			return 0
		}
		index := int(float64(len(durations)-1)*p + 0.5)
		return durations[index]
	}
	report := map[string]any{"generated_at": time.Now().Format(time.RFC3339), "debug": o.debug, "cases_file": o.cases, "total": len(cases), "passed": passed, "failed": len(cases) - passed, "duration_p50_ms": percentile(.50), "duration_p95_ms": percentile(.95), "prompt_tokens": promptTokens, "completion_tokens": completionTokens, "tool_calls": toolCalls, "results": results}
	if o.compare != "" {
		baseline, readErr := os.ReadFile(o.compare)
		if readErr != nil {
			return report, fmt.Errorf("read comparison report: %w", readErr)
		}
		var previous struct {
			Total   int `json:"total"`
			Passed  int `json:"passed"`
			Failed  int `json:"failed"`
			Results []struct {
				Name   string `json:"name"`
				Passed bool   `json:"passed"`
			} `json:"results"`
		}
		if err := json.Unmarshal(baseline, &previous); err != nil || previous.Total == 0 {
			return report, fmt.Errorf("comparison report is invalid or empty")
		}
		baselineByName := make(map[string]bool, len(previous.Results))
		for _, item := range previous.Results {
			baselineByName[item.Name] = item.Passed
		}
		if len(baselineByName) != len(results) {
			return report, fmt.Errorf("comparison report uses a different evaluation case set")
		}
		regressions, improvements := []string{}, []string{}
		for _, item := range results {
			previousPassed, exists := baselineByName[item.Name]
			if !exists {
				return report, fmt.Errorf("comparison report is missing case %q", item.Name)
			}
			if previousPassed && !item.Passed {
				regressions = append(regressions, item.Name)
			}
			if !previousPassed && item.Passed {
				improvements = append(improvements, item.Name)
			}
		}
		report["comparison"] = map[string]any{"report": o.compare, "baseline_total": previous.Total, "baseline_passed": previous.Passed, "baseline_failed": previous.Failed, "pass_delta": passed - previous.Passed, "regressions": regressions, "improvements": improvements}
	}
	if o.report != "" {
		encoded, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			return report, marshalErr
		}
		if writeErr := os.WriteFile(o.report, append(encoded, '\n'), 0600); writeErr != nil {
			return report, fmt.Errorf("write evaluation report: %w", writeErr)
		}
		if chmodErr := os.Chmod(o.report, 0600); chmodErr != nil {
			return report, fmt.Errorf("restrict evaluation report permissions: %w", chmodErr)
		}
	}
	if passed != len(cases) {
		return report, errCheck
	}
	return report, nil
}
