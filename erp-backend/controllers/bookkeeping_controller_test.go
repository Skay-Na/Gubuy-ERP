package controllers

import (
	"erp-backend/models"
	"errors"
	"testing"
	"time"
)

func TestBookkeepingOrderField(t *testing.T) {
	tests := []struct {
		name    string
		request CreateBookkeepingEntryRequest
		order   models.Order
		field   string
		wantErr error
	}{
		{
			name:    "links channel reward",
			request: CreateBookkeepingEntryRequest{EntryType: bookkeepingExpense, Category: "渠道奖励", Amount: 300},
			order:   models.Order{OrderType: 1, ActualPayAmount: 5000},
			field:   "referral_fee",
		},
		{
			name:    "links installation fee",
			request: CreateBookkeepingEntryRequest{EntryType: bookkeepingExpense, Category: "安装费", Amount: 200},
			order:   models.Order{OrderType: 1, ActualPayAmount: 5000},
			field:   "installation_fee",
		},
		{
			name:    "links delivery fee",
			request: CreateBookkeepingEntryRequest{EntryType: bookkeepingExpense, Category: "运输配送", Amount: 150},
			order:   models.Order{OrderType: 1, ActualPayAmount: 5000},
			field:   "delivery_fee",
		},
		{
			name:    "links employee commission within limit",
			request: CreateBookkeepingEntryRequest{EntryType: bookkeepingExpense, Category: "员工提成", Amount: 400},
			order:   models.Order{OrderType: 1, ActualPayAmount: 5000, CommissionAmount: 300},
			field:   "commission_amount",
		},
		{
			name:    "ignores ordinary category",
			request: CreateBookkeepingEntryRequest{EntryType: bookkeepingExpense, Category: "房租", Amount: 1000},
			order:   models.Order{OrderType: 1, ActualPayAmount: 5000},
		},
		{
			name:    "rejects order cost as income",
			request: CreateBookkeepingEntryRequest{EntryType: bookkeepingIncome, Category: "安装费", Amount: 200},
			order:   models.Order{OrderType: 1, ActualPayAmount: 5000},
			wantErr: errOrderCostRequiresExpense,
		},
		{
			name:    "rejects chamber order employee commission",
			request: CreateBookkeepingEntryRequest{EntryType: bookkeepingExpense, Category: "员工提成", Amount: 100},
			order:   models.Order{OrderType: 2, ActualPayAmount: 5000},
			wantErr: errOrderCommissionForbidden,
		},
		{
			name:    "rejects employee commission above cap",
			request: CreateBookkeepingEntryRequest{EntryType: bookkeepingExpense, Category: "员工提成", Amount: 600},
			order:   models.Order{OrderType: 1, ActualPayAmount: 3000, CommissionAmount: 400},
			wantErr: errOrderCommissionTooHigh,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			field, err := bookkeepingOrderField(testCase.request, testCase.order)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("bookkeepingOrderField() error = %v, want %v", err, testCase.wantErr)
			}
			if field != testCase.field {
				t.Fatalf("bookkeepingOrderField() field = %q, want %q", field, testCase.field)
			}
		})
	}
}

func TestValidateBookkeepingEntry(t *testing.T) {
	tests := []struct {
		name    string
		request CreateBookkeepingEntryRequest
		wantErr error
	}{
		{
			name: "accepts income and trims text fields",
			request: CreateBookkeepingEntryRequest{
				EntryType:    bookkeepingIncome,
				Category:     "  其他收入  ",
				Amount:       1200,
				AccountID:    1,
				Counterparty: "  张三  ",
			},
		},
		{
			name: "accepts expense",
			request: CreateBookkeepingEntryRequest{
				EntryType: bookkeepingExpense,
				Category:  "房租",
				Amount:    3500,
				AccountID: 2,
			},
		},
		{
			name: "rejects unsupported type",
			request: CreateBookkeepingEntryRequest{
				EntryType: 3,
				Category:  "调拨",
				Amount:    100,
				AccountID: 1,
			},
			wantErr: errInvalidBookkeepingType,
		},
		{
			name: "rejects non-positive amount",
			request: CreateBookkeepingEntryRequest{
				EntryType: bookkeepingExpense,
				Category:  "耗材",
				Amount:    0,
				AccountID: 1,
			},
			wantErr: errInvalidBookkeepingAmount,
		},
		{
			name: "rejects blank category",
			request: CreateBookkeepingEntryRequest{
				EntryType: bookkeepingExpense,
				Category:  "   ",
				Amount:    10,
				AccountID: 1,
			},
			wantErr: errMissingBookkeepingFields,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := testCase.request
			err := validateBookkeepingEntry(&request)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("validateBookkeepingEntry() error = %v, want %v", err, testCase.wantErr)
			}
			if err == nil && request.Category == "" {
				t.Fatal("validateBookkeepingEntry() kept an empty category")
			}
			if err == nil && testCase.name == "accepts income and trims text fields" {
				if request.Category != "其他收入" || request.Counterparty != "张三" {
					t.Fatalf("validateBookkeepingEntry() did not trim fields: %#v", request)
				}
			}
		})
	}
}

func TestParseBookkeepingOccurredAt(t *testing.T) {
	fallback := time.Date(2026, time.October, 3, 10, 0, 0, 0, time.Local)
	tests := []struct {
		name      string
		value     string
		want      time.Time
		wantError bool
	}{
		{name: "uses fallback for blank value", value: "", want: fallback},
		{
			name:  "parses local date time",
			value: "2026-10-03 14:35:00",
			want:  time.Date(2026, time.October, 3, 14, 35, 0, 0, time.Local),
		},
		{name: "rejects invalid value", value: "2026/10/03", wantError: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := parseBookkeepingOccurredAt(testCase.value, fallback)
			if (err != nil) != testCase.wantError {
				t.Fatalf("parseBookkeepingOccurredAt() error = %v, wantError %v", err, testCase.wantError)
			}
			if !testCase.wantError && !got.Equal(testCase.want) {
				t.Fatalf("parseBookkeepingOccurredAt() = %v, want %v", got, testCase.want)
			}
		})
	}
}
