package controllers

import (
	"erp-backend/models"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestGenerateOrderNo(t *testing.T) {
	now := time.Date(2026, 6, 25, 14, 37, 59, 0, time.Local)

	if got := GenerateOrderNo(now, 1); got != "202606250001" {
		t.Fatalf("GenerateOrderNo() = %q, want %q", got, "202606250001")
	}

	if got := GenerateOrderNo(now, 42); got != "202606250042" {
		t.Fatalf("GenerateOrderNo() = %q, want %q", got, "202606250042")
	}
}

func TestOrderCostUsesCalibratedAverageCost(t *testing.T) {
	product := models.Product{AvgCost: 5300, LatestCost: 6350}

	if got := orderCostSnapshot(product); got != 5300 {
		t.Fatalf("orderCostSnapshot() = %.2f, want calibrated average cost 5300.00", got)
	}
}

func TestOrderCostFallsBackToLatestCostWhenAverageCostIsUnset(t *testing.T) {
	product := models.Product{AvgCost: 0, LatestCost: 6350}

	if got := orderCostSnapshot(product); got != 6350 {
		t.Fatalf("orderCostSnapshot() = %.2f, want latest cost fallback 6350.00", got)
	}
}

func TestApplyOrderFiltersKeywordMatchesSubsidyOrders(t *testing.T) {
	sql := dryRunOrderFilterSQL(t, "国补")

	for _, want := range []string{
		"order_no LIKE ?",
		"subsidy_amount > 0",
		"subsidy_status != 0",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("filter SQL missing %q:\n%s", want, sql)
		}
	}
}

func TestApplyOrderFiltersKeywordMatchesTradeInAmount(t *testing.T) {
	sql := dryRunOrderFilterSQL(t, "trade in")

	for _, want := range []string{
		"trade_in_type LIKE ?",
		"trade_in_amount > 0",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("filter SQL missing %q:\n%s", want, sql)
		}
	}
}

func TestApplyOrderFiltersOrdinaryKeywordOnlyUsesFuzzySearch(t *testing.T) {
	sql := dryRunOrderFilterSQL(t, "张三")

	if !strings.Contains(sql, "order_no LIKE ?") {
		t.Fatalf("filter SQL missing fuzzy search:\n%s", sql)
	}
	for _, unwanted := range []string{"subsidy_amount > 0", "subsidy_status != 0", "trade_in_amount > 0"} {
		if strings.Contains(sql, unwanted) {
			t.Fatalf("ordinary keyword added %q:\n%s", unwanted, sql)
		}
	}
}

func TestApplyOrderFiltersPaymentAndFulfillmentStatus(t *testing.T) {
	sql := dryRunOrderFilterSQLWithStatus(t, "1", "pending_install", "")

	for _, want := range []string{
		"payment_status = ?",
		"delivery_method != ?",
		"is_delivered = ? AND is_installed = ?",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("filter SQL missing %q:\n%s", want, sql)
		}
	}
}

func TestApplyOrderFiltersPickupFulfillmentStatus(t *testing.T) {
	sql := dryRunOrderFilterSQLWithStatus(t, "", "pickup", "")

	if !strings.Contains(sql, "delivery_method = ?") {
		t.Fatalf("filter SQL missing pickup delivery_method condition:\n%s", sql)
	}
}

func TestApplyOrderFiltersSubsidyAndOrderStatus(t *testing.T) {
	sql := dryRunOrderFilterSQLWithParams(t, "", "2", "", "", "1")

	for _, want := range []string{
		"subsidy_status = ?",
		"order_status = ?",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("filter SQL missing %q:\n%s", want, sql)
		}
	}
}

func TestValidateReferralFeeUpdate(t *testing.T) {
	tests := []struct {
		name    string
		order   models.Order
		fee     float64
		wantErr error
	}{
		{name: "pending fee can be updated", order: models.Order{}, fee: 88.50},
		{name: "paid fee cannot be updated", order: models.Order{IsReferralFeePaid: true}, fee: 88.50, wantErr: errReferralFeeAlreadyPaid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReferralFeeUpdate(&tt.order, tt.fee)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("validateReferralFeeUpdate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestPaymentMethodName(t *testing.T) {
	tests := []struct {
		method int
		want   string
	}{
		{1, "支付宝"},
		{2, "微信"},
		{3, "公户"},
		{5, "现金"},
		{4, ""},
		{0, ""},
		{99, ""},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("method=%d", tt.method), func(t *testing.T) {
			if got := paymentMethodName(tt.method); got != tt.want {
				t.Fatalf("paymentMethodName(%d) = %q, want %q", tt.method, got, tt.want)
			}
		})
	}
}

func TestOrderDeliveryToWarehouse(t *testing.T) {
	tests := []struct {
		name           string
		deliveryMethod int
		want           int
	}{
		{name: "门店自提 → 门店", deliveryMethod: 1, want: 2},
		{name: "主仓发货 → 主仓", deliveryMethod: 2, want: 1},
		{name: "云仓代发 → 云仓", deliveryMethod: 3, want: 3},
		{name: "未知值原样返回", deliveryMethod: 99, want: 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := orderDeliveryToWarehouse(tt.deliveryMethod); got != tt.want {
				t.Fatalf("orderDeliveryToWarehouse(%d) = %d, want %d", tt.deliveryMethod, got, tt.want)
			}
		})
	}
}

func dryRunOrderFilterSQL(t *testing.T, keyword string) string {
	return dryRunOrderFilterSQLWithParams(t, keyword, "", "", "", "")
}

func dryRunOrderFilterSQLWithStatus(t *testing.T, paymentStatus, fulfillmentStatus, orderStatus string) string {
	return dryRunOrderFilterSQLWithParams(t, "", "", paymentStatus, fulfillmentStatus, orderStatus)
}

func dryRunOrderFilterSQLWithParams(t *testing.T, keyword, subsidyStatus, paymentStatus, fulfillmentStatus, orderStatus string) string {
	t.Helper()

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}

	query := applyOrderFilters(db.Model(&models.Order{}), keyword, subsidyStatus, paymentStatus, fulfillmentStatus, orderStatus, "", "", "")
	tx := query.Find(&[]models.Order{})
	if tx.Error != nil {
		t.Fatalf("build dry-run query: %v", tx.Error)
	}
	return tx.Statement.SQL.String()
}
