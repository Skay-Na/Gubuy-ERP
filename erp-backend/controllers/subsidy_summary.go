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
// the same order fallback and recorded settlement rules.
func readSubsidyOverview(ctx context.Context) (subsidyOverview, error) {
	var overview subsidyOverview
	err := config.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var generated struct {
			Count  int64
			Amount float64
		}
		if err := tx.Model(&models.Order{}).
			Where("order_status = 1 AND subsidy_amount > 0").
			Select("COUNT(*) AS count, COALESCE(SUM(CASE WHEN subsidy_pending_amount > 0 THEN subsidy_pending_amount ELSE subsidy_amount END), 0) AS amount").
			Scan(&generated).Error; err != nil {
			return err
		}
		var received struct {
			Count  int64
			Amount float64
		}
		if err := tx.Model(&models.SubsidySettlement{}).
			Select("COUNT(*) AS count, COALESCE(SUM(amount), 0) AS amount").
			Scan(&received).Error; err != nil {
			return err
		}
		overview.TotalGenerated = agentRound(generated.Amount)
		overview.TotalReceived = agentRound(received.Amount)
		overview.PendingBalance = agentRound(CalculatePendingSubsidy(
			overview.TotalGenerated,
			overview.TotalReceived,
		))
		overview.OrderCount = generated.Count
		overview.ReceiptCount = received.Count
		return nil
	})
	return overview, err
}
