package controllers

import "testing"

func TestNormalizeInboundSettlementDefaultsToSaleSettlement(t *testing.T) {
	settlement, err := normalizeInboundSettlement("", nil)
	if err != nil {
		t.Fatalf("normalizeInboundSettlement() error = %v", err)
	}
	if settlement.Type != SettlementTypeSaleSettlement {
		t.Fatalf("settlement.Type = %q, want %q", settlement.Type, SettlementTypeSaleSettlement)
	}
}

func TestNormalizeInboundSettlementNeverCreatesCashPayment(t *testing.T) {
	accountID := uint(7)
	settlement, err := normalizeInboundSettlement(SettlementTypeCashPurchase, &accountID)
	if err != nil {
		t.Fatalf("normalizeInboundSettlement() error = %v", err)
	}
	if settlement.Type != SettlementTypeSaleSettlement {
		t.Fatalf("settlement.Type = %q, want %q", settlement.Type, SettlementTypeSaleSettlement)
	}
	if settlement.AccountID != nil {
		t.Fatalf("settlement.AccountID = %v, want nil", settlement.AccountID)
	}
}

func TestNormalizeInboundSettlementAllowsSaleSettlementWithoutAccountOrSupplier(t *testing.T) {
	settlement, err := normalizeInboundSettlement(SettlementTypeSaleSettlement, nil)
	if err != nil {
		t.Fatalf("normalizeInboundSettlement() error = %v", err)
	}
	if settlement.Type != SettlementTypeSaleSettlement {
		t.Fatalf("settlement.Type = %q, want %q", settlement.Type, SettlementTypeSaleSettlement)
	}
}
