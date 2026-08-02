package controllers

import "testing"

func TestCountsTowardEmployeePerformance(t *testing.T) {
	tests := []struct {
		name         string
		referrerName string
		want         bool
	}{
		{name: "normal employee order", referrerName: "", want: true},
		{name: "third party referral", referrerName: "张三", want: true},
		{name: "boss customer", referrerName: "老板客户", want: false},
		{name: "boss keyword variant", referrerName: "老板带单", want: false},
		{name: "boss customer suffix", referrerName: "老板客户-老张", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountsTowardEmployeePerformance(tt.referrerName); got != tt.want {
				t.Fatalf("CountsTowardEmployeePerformance(%q) = %v, want %v", tt.referrerName, got, tt.want)
			}
		})
	}
}
