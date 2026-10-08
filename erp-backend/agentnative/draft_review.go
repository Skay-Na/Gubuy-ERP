package agentnative

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"erp-backend/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrDraftNotFound      = errors.New("replenishment draft not found")
	ErrDraftNotReviewable = errors.New("replenishment draft is no longer reviewable")
	ErrDraftStale         = errors.New("replenishment draft is stale because stock changed")
	ErrInvalidDraftReview = errors.New("invalid replenishment draft review")
)

type ChatTaskInput struct {
	RunID      string
	Actor      string
	InputJSON  string
	OutputJSON string
	Status     string
	Step       string
}

// PersistChatTask stores a completed chat turn in the same durable task table
// used by monitoring. The caller can include it in the chat-message transaction.
func PersistChatTask(tx *gorm.DB, input ChatTaskInput) (models.AgentWorkTask, error) {
	if tx == nil || strings.TrimSpace(input.RunID) == "" || strings.TrimSpace(input.Actor) == "" ||
		input.Status == "" || input.Step == "" || len(input.RunID) > 36 {
		return models.AgentWorkTask{}, errors.New("invalid chat task")
	}
	idempotencyKey := "chat:" + input.RunID
	task := models.AgentWorkTask{
		TenantID: SingleTenant, Kind: "chat_request", Status: input.Status,
		Step: input.Step, Attempts: 1, CreatedBy: input.Actor,
		IdempotencyKey: &idempotencyKey, InputJSON: input.InputJSON, OutputJSON: input.OutputJSON,
	}
	if err := tx.Create(&task).Error; err != nil {
		return models.AgentWorkTask{}, fmt.Errorf("persist chat task: %w", err)
	}
	return task, nil
}

type DraftReviewInput struct {
	DraftID  uint
	Decision string
	Quantity int
	Note     string
	Actor    string
	Now      time.Time
}

// ReviewDraft applies an internal human decision to a suggested replenishment.
// It never creates a purchase order or changes inventory.
func (s MonitorService) ReviewDraft(ctx context.Context, input DraftReviewInput) (models.AgentReplenishmentDraft, error) {
	if s.DB == nil {
		return models.AgentReplenishmentDraft{}, errors.New("database unavailable")
	}
	if input.DraftID == 0 || (input.Decision != "approve" && input.Decision != "reject") ||
		input.Quantity < 0 || input.Quantity > 100000 || len([]rune(input.Note)) > 500 || strings.TrimSpace(input.Actor) == "" {
		return models.AgentReplenishmentDraft{}, ErrInvalidDraftReview
	}
	if input.Now.IsZero() {
		input.Now = time.Now()
	}
	var draft models.AgentReplenishmentDraft
	stale := false
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ?", input.DraftID, SingleTenant).First(&draft).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrDraftNotFound
			}
			return fmt.Errorf("load draft: %w", err)
		}
		if draft.Status != "suggested" {
			return ErrDraftNotReviewable
		}
		var product models.Product
		if err := tx.Select("id", "main_stock", "store_stock", "cloud_stock").First(&product, draft.ProductID).Error; err != nil {
			return fmt.Errorf("load draft product: %w", err)
		}
		currentStock := product.MainStock + product.StoreStock + product.CloudStock
		status := "approved"
		detail := "管理员确认了内部补货建议；未创建采购订单且未修改库存"
		if input.Decision == "reject" {
			status = "rejected"
			detail = "管理员拒绝了内部补货建议；未创建采购订单且未修改库存"
		} else if currentStock != draft.StockSnapshot {
			status = "stale"
			stale = true
			detail = fmt.Sprintf("审核时库存已从快照 %d 变为 %d，草案已标记过期，请重新计算", draft.StockSnapshot, currentStock)
		}
		updates := map[string]any{
			"status": status, "reviewed_by": input.Actor,
			"review_note": strings.TrimSpace(input.Note), "reviewed_at": input.Now,
		}
		if status == "approved" && input.Quantity > 0 {
			updates["quantity"] = input.Quantity
		}
		if err := tx.Model(&draft).Updates(updates).Error; err != nil {
			return fmt.Errorf("save draft review: %w", err)
		}
		if err := tx.Create(&models.AgentActivity{
			TenantID: SingleTenant, TaskID: draft.TaskID, Event: status, Detail: detail,
		}).Error; err != nil {
			return fmt.Errorf("record draft review: %w", err)
		}
		if err := tx.First(&draft, draft.ID).Error; err != nil {
			return fmt.Errorf("reload reviewed draft: %w", err)
		}
		return nil
	})
	if err != nil {
		return models.AgentReplenishmentDraft{}, err
	}
	if stale {
		return draft, ErrDraftStale
	}
	return draft, nil
}
