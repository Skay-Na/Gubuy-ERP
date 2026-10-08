package controllers

import "testing"

func TestCalculateGrossProfit(t *testing.T) {
	tests := []struct {
		name             string
		revenue          float64
		productCost      float64
		deliveryFee      float64
		installationFee  float64
		referralFee      float64
		commissionAmount float64
		tradeInAmount    float64
		want             float64
	}{
		{
			name:             "deducts discount-adjusted direct costs and trade-in",
			revenue:          9800,
			productCost:      6000,
			deliveryFee:      300,
			installationFee:  200,
			referralFee:      100,
			commissionAmount: 200,
			tradeInAmount:    500,
			want:             2500,
		},
		{
			name:        "allows a loss",
			revenue:     1000,
			productCost: 1200,
			want:        -200,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := CalculateGrossProfit(
				testCase.revenue,
				testCase.productCost,
				testCase.deliveryFee,
				testCase.installationFee,
				testCase.referralFee,
				testCase.commissionAmount,
				testCase.tradeInAmount,
			)
			if got != testCase.want {
				t.Fatalf("CalculateGrossProfit() = %.2f, want %.2f", got, testCase.want)
			}
		})
	}
}

func TestCalculateNetProfit(t *testing.T) {
	if got := CalculateNetProfit(2500, 800); got != 1700 {
		t.Fatalf("CalculateNetProfit() = %.2f, want 1700.00", got)
	}
}

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
