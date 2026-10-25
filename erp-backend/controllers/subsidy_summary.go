package controllers

import (
	"context"

	"erp-backend/config"
	"erp-backend/models"
	"gorm.io/gorm"
)

type subsidyOverview struct {
	TotalGenerated float64 `json:"total_generated"`
	TotalReceived  float64 `json:"total_received"`
	PendingBalance float64 `json:"pending_balance"`
	OrderCount     int64   `json:"-"`
	ReceiptCount   int64   `json:"-"`
}

// readSubsidyOverview is shared by the国补 page and Agent so their balances use
// the same per-order received rules.
func readSubsidyOverview(ctx context.Context) (subsidyOverview, error) {
	var overview subsidyOverview
	err := config.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 统一口径：pending 可用额 = subsidy_pending_amount - subsidy_received_amt（按单）。
		// 不再对历史单回退 subsidy_amount，也不再直接用回款池入账额冲抵
		// （入账后未核销的部分是"池余额"，不是订单层面的待回款）。
		var generated struct {
			Count  int64
			Amount float64
		}
		if err := tx.Model(&models.Order{}).
			Where("order_status = 1 AND subsidy_pending_amount > 0").
			Select("COUNT(*) AS count, COALESCE(SUM(subsidy_pending_amount), 0) AS amount").
			Scan(&generated).Error; err != nil {
			return err
		}
		var allocated struct {
			Count  int64
			Amount float64
		}
		if err := tx.Model(&models.Order{}).
			Where("order_status = 1 AND subsidy_received_amt > 0").
			Select("COUNT(*) AS count, COALESCE(SUM(subsidy_received_amt), 0) AS amount").
			Scan(&allocated).Error; err != nil {
			return err
		}
		overview.TotalGenerated = agentRound(generated.Amount)
		overview.TotalReceived = agentRound(allocated.Amount)
		overview.PendingBalance = agentRound(CalculatePendingSubsidy(
			overview.TotalGenerated,
			overview.TotalReceived,
		))
		overview.OrderCount = generated.Count
		overview.ReceiptCount = allocated.Count
		return nil
	})
	return overview, err
}
