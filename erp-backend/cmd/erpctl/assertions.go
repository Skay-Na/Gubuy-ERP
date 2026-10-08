package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type evalPeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}
type metricExpectation struct {
	Metric    string   `json:"metric"`
	Start     string   `json:"start,omitempty"`
	End       string   `json:"end,omitempty"`
	Value     *float64 `json:"value"`
	Tolerance float64  `json:"tolerance,omitempty"`
}
type evalAssertions struct {
	Topic   string              `json:"topic,omitempty"`
	Metrics []string            `json:"metrics,omitempty"`
	Periods []evalPeriod        `json:"periods,omitempty"`
	Values  []metricExpectation `json:"values,omitempty"`
}
type assertionResult struct {
	Check    string `json:"check"`
	Passed   bool   `json:"passed"`
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
	Detail   string `json:"detail,omitempty"`
}

func validatePeriod(start, end string) error {
	first, err := time.Parse("2006-01-02", start)
	if err != nil {
		return fmt.Errorf("invalid start date %q", start)
	}
	last, err := time.Parse("2006-01-02", end)
	if err != nil || last.Before(first) {
		return fmt.Errorf("invalid end date %q", end)
	}
	return nil
}

func (a evalAssertions) validate() error {
	for _, period := range a.Periods {
		if err := validatePeriod(period.Start, period.End); err != nil {
			return err
		}
	}
	for _, metric := range a.Metrics {
		if strings.TrimSpace(metric) == "" {
			return errors.New("expected metric must not be empty")
		}
	}
	for _, expectation := range a.Values {
		if strings.TrimSpace(expectation.Metric) == "" || expectation.Value == nil {
			return errors.New("numeric assertion requires metric and value")
		}
		if expectation.Tolerance < 0 || math.IsNaN(expectation.Tolerance) || math.IsInf(expectation.Tolerance, 0) {
			return errors.New("tolerance must be a finite nonnegative number")
		}
		if math.IsNaN(*expectation.Value) || math.IsInf(*expectation.Value, 0) {
			return errors.New("expected value must be finite")
		}
		if expectation.Start != "" || expectation.End != "" {
			if err := validatePeriod(expectation.Start, expectation.End); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkAssertions(d diagnostic, a evalAssertions) []assertionResult {
	results := []assertionResult{}
	var task struct {
		Topic   string       `json:"topic"`
		Metrics []string     `json:"metrics"`
		Periods []evalPeriod `json:"periods"`
	}
	needsTask := a.Topic != "" || len(a.Metrics) > 0 || len(a.Periods) > 0
	if needsTask {
		if err := json.Unmarshal(d.Task, &task); err != nil || string(d.Task) == "null" {
			results = append(results, assertionResult{
				Check: "task", Expected: "structured task", Detail: "task missing or invalid",
			})
		}
	}
	if a.Topic != "" {
		results = append(results, assertionResult{
			Check: "topic", Expected: a.Topic, Actual: task.Topic, Passed: task.Topic == a.Topic,
		})
	}
	for _, metric := range a.Metrics {
		found := false
		for _, actual := range task.Metrics {
			if actual == metric {
				found = true
			}
		}
		results = append(results, assertionResult{Check: "metric", Expected: metric, Actual: task.Metrics, Passed: found})
	}
	for _, period := range a.Periods {
		found := false
		for _, actual := range task.Periods {
			if actual == period {
				found = true
			}
		}
		results = append(results, assertionResult{Check: "period", Expected: period, Actual: task.Periods, Passed: found})
	}
	if len(a.Values) == 0 {
		return results
	}
	rows, err := reportRows(d.Evidence)
	for _, expectation := range a.Values {
		check := assertionResult{Check: "value", Expected: expectation}
		if err != nil {
			check.Detail = err.Error()
			results = append(results, check)
			continue
		}
		matches := []reportRow{}
		for _, row := range rows {
			if expectation.Start != "" && (row.Period.Start != expectation.Start || row.Period.End != expectation.End) {
				continue
			}
			matches = append(matches, row)
		}
		if len(matches) != 1 {
			check.Detail = fmt.Sprintf("expected one report row, found %d; specify an exact period", len(matches))
			results = append(results, check)
			continue
		}
		row := matches[0]
		value, exists := row.Values[expectation.Metric]
		if !exists || value == nil || row.Missing {
			check.Detail = "metric missing, null or report incomplete"
			results = append(results, check)
			continue
		}
		check.Actual = *value
		check.Passed = math.Abs(*value-*expectation.Value) <= expectation.Tolerance+1e-9
		if !check.Passed {
			check.Detail = "value differs beyond absolute tolerance"
		}
		results = append(results, check)
	}
	return results
}

type reportRow struct {
	Period  evalPeriod          `json:"period"`
	Values  map[string]*float64 `json:"values"`
	Missing bool                `json:"missing"`
}

func reportRows(raw json.RawMessage) ([]reportRow, error) {
	evidence := []struct {
		Query struct {
			Tool string `json:"tool"`
		} `json:"query"`
		Result string `json:"result"`
	}{}
	if err := json.Unmarshal(raw, &evidence); err != nil {
		return nil, errors.New("evidence missing or invalid")
	}
	rows := []reportRow{}
	for _, item := range evidence {
		if item.Query.Tool != "business_report" {
			continue
		}
		var report struct {
			Rows []reportRow `json:"rows"`
		}
		if err := json.Unmarshal([]byte(item.Result), &report); err != nil {
			return nil, errors.New("business report is invalid")
		}
		rows = append(rows, report.Rows...)
	}
	return rows, nil
}
