package controllers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"erp-backend/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type bookkeepingServiceError struct {
	status  int
	message string
	err     error
}

func (e *bookkeepingServiceError) Error() string { return e.message }
func (e *bookkeepingServiceError) Unwrap() error { return e.err }

func bookkeepingFailure(status int, message string, err error) error {
	return &bookkeepingServiceError{status: status, message: message, err: err}
}

func bookkeepingHTTPError(err error) (int, string) {
	var serviceErr *bookkeepingServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.status, serviceErr.message
	}
	return http.StatusInternalServerError, "记账失败"
}

// postBookkeepingEntry is the single deterministic write path used by the
// bookkeeping page and Agent expense confirmation. The caller owns the
// transaction and any one-use draft or AI-import lock.
func postBookkeepingEntry(
	tx *gorm.DB,
	req CreateBookkeepingEntryRequest,
	occurredAt time.Time,
) (models.FinancialLog, *models.Order, error) {
	linkedOrder, err := resolveBookkeepingOrder(tx, &req)
	if err != nil {
		return models.FinancialLog{}, nil, bookkeepingFailure(http.StatusBadRequest, err.Error(), err)
	}
	if linkedOrder != nil {
		field, fieldErr := bookkeepingOrderField(req, *linkedOrder)
		if fieldErr != nil {
			return models.FinancialLog{}, nil, bookkeepingFailure(http.StatusBadRequest, fieldErr.Error(), fieldErr)
		}
		if field != "" {
			if err := tx.Model(linkedOrder).
				UpdateColumn(field, gorm.Expr(field+" + ?", req.Amount)).Error; err != nil {
				return models.FinancialLog{}, nil, bookkeepingFailure(http.StatusInternalServerError, "订单费用同步失败", err)
			}
		}
	}

	var account models.Account
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, req.AccountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.FinancialLog{}, nil, bookkeepingFailure(http.StatusNotFound, "资金账户不存在", err)
		}
		return models.FinancialLog{}, nil, bookkeepingFailure(http.StatusInternalServerError, "资金账户读取失败", err)
	}
	balanceExpression := gorm.Expr("balance + ?", req.Amount)
	if req.EntryType == bookkeepingExpense {
		balanceExpression = gorm.Expr("balance - ?", req.Amount)
	}
	if err := tx.Model(&account).UpdateColumn("balance", balanceExpression).Error; err != nil {
		return models.FinancialLog{}, nil, bookkeepingFailure(http.StatusInternalServerError, "账户余额更新失败", err)
	}
	if err := tx.First(&account, req.AccountID).Error; err != nil {
		return models.FinancialLog{}, nil, bookkeepingFailure(http.StatusInternalServerError, "账户余额读取失败", err)
	}

	entry := models.FinancialLog{
		AccountID:    req.AccountID,
		FlowType:     req.EntryType,
		Category:     req.Category,
		Amount:       req.Amount,
		BalanceAfter: account.Balance,
		Counterparty: req.Counterparty,
		RelatedNo:    req.RelatedNo,
		Remark:       req.Remark,
		SourceType:   "manual",
		CreatedAt:    occurredAt,
	}
	if err := tx.Create(&entry).Error; err != nil {
		return models.FinancialLog{}, nil, bookkeepingFailure(http.StatusInternalServerError, "记账流水保存失败", err)
	}
	if req.EntryType == bookkeepingExpense && linkedOrder != nil && strings.Contains(req.Category, "退款") {
		if err := postExitOrderRefundAlert(tx, *linkedOrder, req.Category, req.Amount, &entry.ID, time.Now()); err != nil {
			return models.FinancialLog{}, nil, bookkeepingFailure(http.StatusInternalServerError, "离职后订单退款告警写入失败", err)
		}
	}
	return entry, linkedOrder, nil
}

func postExitOrderRefundAlert(tx *gorm.DB, order models.Order, category string, amount float64, financialLogID *uint, occurredAt time.Time) error {
	if !strings.Contains(category, "退款") {
		return nil
	}
	return PostExitOrderChangeAlerts(tx, order, "order_refund", amount, financialLogID, occurredAt)
}
