package agentnative

import "testing"

func TestAssessLowStock(t *testing.T) {
	tests := []struct {
		name string
		in   StockObservation
		want LowStockAssessment
	}{
		{
			name: "insufficient sales",
			in:   StockObservation{Available: 0, Sales30Days: 2},
			want: LowStockAssessment{},
		},
		{
			name: "stock above threshold",
			in:   StockObservation{Available: 4, Sales30Days: 10},
			want: LowStockAssessment{Threshold: 3},
		},
		{
			name: "suggest limited fourteen day replenishment",
			in:   StockObservation{Available: 1, Sales30Days: 10},
			want: LowStockAssessment{Threshold: 3, Quantity: 4, Eligible: true},
		},
		{
			name: "cap large suggestions",
			in:   StockObservation{Available: 0, Sales30Days: 1000},
			want: LowStockAssessment{Threshold: 234, Quantity: 100, Eligible: true},
		},
		{
			name: "invalid stock",
			in:   StockObservation{Available: -1, Sales30Days: 10},
			want: LowStockAssessment{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AssessLowStock(tt.in); got != tt.want {
				t.Fatalf("AssessLowStock(%+v) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}
