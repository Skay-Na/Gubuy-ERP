package controllers

import (
	"context"
	"testing"
	"time"
)

func TestAgentNowUsesFixedReferenceDate(t *testing.T) {
	date := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	ctx := context.WithValue(context.Background(), agentReferenceDateKey{}, date)
	if got := agentNow(ctx); !got.Equal(date) {
		t.Fatalf("agentNow() = %v, want %v", got, date)
	}
	if got := agentNow(context.Background()); got.IsZero() {
		t.Fatal("agentNow() without reference date returned zero")
	}
}
