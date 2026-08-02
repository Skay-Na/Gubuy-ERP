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

func TestNormalizeInboundSettlementRequiresAccountForCashPurchase(t *testing.T) {
	_, err := normalizeInboundSettlement(SettlementTypeCashPurchase, nil)
	if err == nil {
		t.Fatal("normalizeInboundSettlement() error = nil, want missing account error")
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
