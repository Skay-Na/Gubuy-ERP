package controllers

import "testing"

func TestCalculatePendingSubsidyForLegacyOrders(t *testing.T) {
	got := CalculatePendingSubsidy(8402.25, 999)
	want := 7403.25
	if got != want {
		t.Fatalf("CalculatePendingSubsidy() = %.2f, want %.2f", got, want)
	}
}

func TestCalculatePendingSubsidyNeverReturnsNegative(t *testing.T) {
	got := CalculatePendingSubsidy(0, 999)
	if got != 0 {
		t.Fatalf("CalculatePendingSubsidy() = %.2f, want 0", got)
	}
}

func TestCalculatePendingSubsidySupportsMixedLegacyAndNewOrders(t *testing.T) {
	// SQL 聚合会逐单得到：历史订单 8402.25 + 新订单实际待回 100。
	got := CalculatePendingSubsidy(8502.25, 1499)
	want := 7003.25
	if got != want {
		t.Fatalf("CalculatePendingSubsidy() = %.2f, want %.2f", got, want)
	}
}
