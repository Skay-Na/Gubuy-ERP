package controllers

import (
	"fmt"
	"strings"
)

const (
	SettlementTypeCashPurchase   = "cash_purchase"
	SettlementTypeSaleSettlement = "sale_settlement"
)

type inboundSettlement struct {
	Type      string
	AccountID *uint
}

func normalizeInboundSettlement(settlementType string, _ *uint) (inboundSettlement, error) {
	settlementType = strings.TrimSpace(settlementType)
	switch settlementType {
	case "", SettlementTypeCashPurchase, SettlementTypeSaleSettlement:
		// Inventory entry records a cost basis only. Procurement is not a real cash
		// payment in this business, so legacy cash_purchase requests are normalized.
		return inboundSettlement{Type: SettlementTypeSaleSettlement}, nil
	default:
		return inboundSettlement{}, fmt.Errorf("不支持的结算方式")
	}
}
