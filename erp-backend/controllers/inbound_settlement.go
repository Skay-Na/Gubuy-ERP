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

func normalizeInboundSettlement(settlementType string, accountID *uint) (inboundSettlement, error) {
	settlementType = strings.TrimSpace(settlementType)
	if settlementType == "" {
		settlementType = SettlementTypeSaleSettlement
	}

	settlement := inboundSettlement{
		Type:      settlementType,
		AccountID: accountID,
	}

	switch settlement.Type {
	case SettlementTypeCashPurchase:
		if settlement.AccountID == nil || *settlement.AccountID == 0 {
			return inboundSettlement{}, fmt.Errorf("现款采购必须选择付款账户")
		}
	case SettlementTypeSaleSettlement:
	default:
		return inboundSettlement{}, fmt.Errorf("不支持的结算方式")
	}

	return settlement, nil
}
