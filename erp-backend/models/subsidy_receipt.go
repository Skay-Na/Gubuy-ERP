package models

import "time"

// SubsidyReceipt 国补回款核销记录：把回款池（subsidy_settlements）资金逐单分配到订单。
// 核销本身不写 FinancialLog：入账时 CreateSubsidySettlement 已记账，
// 本表只是"池 → 订单"的分配台账，避免双记。
// "已回款"语义 = orders.subsidy_received_amt >= subsidy_pending_amount，
// subsidy_status=3 只能由核销动作写入。
type SubsidyReceipt struct {
	ID           uint    `gorm:"primaryKey;comment:主键ID" json:"id"`
	SettlementID uint    `gorm:"not null;uniqueIndex:uk_settlement_order,priority:1;comment:关联回款结算单ID" json:"settlement_id"`
	OrderID      uint    `gorm:"not null;uniqueIndex:uk_settlement_order,priority:2;index:idx_order;comment:核销订单ID" json:"order_id"`
	OrderNo      string  `gorm:"type:varchar(50);not null;default:'';comment:订单号快照" json:"order_no"`
	AllocatedAmt float64 `gorm:"type:decimal(10,2);not null;comment:本次核销金额" json:"allocated_amt"`
	// RemainingBefore 核销前订单剩余待回款快照（subsidy_pending_amount - subsidy_received_amt）
	RemainingBefore float64   `gorm:"type:decimal(10,2);not null;default:0;comment:核销前订单剩余待回款快照" json:"remaining_before"`
	CreatedBy       string    `gorm:"type:varchar(100);not null;default:'';comment:操作人" json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
}
