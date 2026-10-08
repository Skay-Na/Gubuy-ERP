package controllers

import "testing"

func TestNormalizeAIImportFields(t *testing.T) {
	tests := []struct {
		name             string
		fields           aiImportFields
		rawModelOutput   string
		wantSubsidy      bool
		wantPaymentLabel string
	}{
		{
			name:             "payment channel explicitly contains cloud quickpass",
			fields:           aiImportFields{PaymentChannel: "银联云闪付"},
			wantSubsidy:      true,
			wantPaymentLabel: "银联云闪付",
		},
		{
			name:             "raw output fallback contains cloud quickpass",
			fields:           aiImportFields{},
			rawModelOutput:   `{"remark":"客户使用云闪付"}`,
			wantSubsidy:      true,
			wantPaymentLabel: "云闪付",
		},
		{
			name:             "ordinary payment stays non subsidy",
			fields:           aiImportFields{PaymentChannel: "现金"},
			wantSubsidy:      false,
			wantPaymentLabel: "现金",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalizeAIImportFields(&tt.fields, tt.rawModelOutput)
			if tt.fields.IsSubsidy != tt.wantSubsidy {
				t.Fatalf("IsSubsidy = %v, want %v", tt.fields.IsSubsidy, tt.wantSubsidy)
			}
			if tt.fields.PaymentChannel != tt.wantPaymentLabel {
				t.Fatalf("PaymentChannel = %q, want %q", tt.fields.PaymentChannel, tt.wantPaymentLabel)
			}
		})
	}
}
