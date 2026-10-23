package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"erp-backend/models"

	"gorm.io/gorm"
)

// RetailReadService exposes validated read capabilities shared by HTTP and
// Agent workflows. It preserves the ERP's existing available-stock and
// effective-order definitions.
type RetailReadService struct {
	DB *gorm.DB
}

type ReplenishmentContext struct {
	ProductID   uint   `json:"product_id"`
	ProductName string `json:"product_name"`
	Available   int    `json:"available"`
	Sales       int    `json:"sales"`
}

type ProductSalesSummary struct {
	ProductID   uint   `json:"product_id"`
	ProductName string `json:"product_name"`
	Units       int    `json:"units"`
	OrderCount  int    `json:"order_count"`
}

type ProductSalesTrend struct {
	ProductID     uint     `json:"product_id"`
	ProductName   string   `json:"product_name"`
	CurrentUnits  int      `json:"current_units"`
	PreviousUnits int      `json:"previous_units"`
	DeltaUnits    int      `json:"delta_units"`
	ChangePercent *float64 `json:"change_percent,omitempty"`
}

type ProductInventorySalesSummary struct {
	ProductID   uint   `json:"product_id"`
	ProductName string `json:"product_name"`
	Available   int    `json:"available"`
	Units       int    `json:"units"`
	OrderCount  int    `json:"order_count"`
}

func (s RetailReadService) SearchProductInventory(ctx context.Context, keyword string, limit int) ([]models.Product, error) {
	if s.DB == nil {
		return nil, errors.New("database unavailable")
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("product search limit must be between 1 and 100")
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" || len([]rune(keyword)) > 200 {
		return nil, errors.New("product search keyword must contain 1 to 200 characters")
	}
	var products []models.Product
	pattern := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(keyword)
	err := s.DB.WithContext(ctx).
		Select("id", "name", "store_stock", "cloud_stock").
		Where("name LIKE ?", "%"+pattern+"%").Order("id DESC").Limit(limit).Find(&products).Error
	if err != nil {
		return nil, fmt.Errorf("search product inventory: %w", err)
	}
	return products, nil
}

// ListProductInventory returns a bounded current sellable-stock snapshot.
func (s RetailReadService) ListProductInventory(ctx context.Context, limit int) ([]models.Product, error) {
	if s.DB == nil {
		return nil, errors.New("database unavailable")
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("product inventory limit must be between 1 and 100")
	}
	products := []models.Product{}
	err := s.DB.WithContext(ctx).Model(&models.Product{}).
		Select("id", "name", "store_stock", "cloud_stock").
		Order("id DESC").Limit(limit).Find(&products).Error
	if err != nil {
		return nil, fmt.Errorf("list product inventory: %w", err)
	}
	return products, nil
}

// GetReplenishmentContext uses valid orders (order_status=1) and the same
// sellable-stock sum: store + cloud, excluding samples and the retired main warehouse.
func (s RetailReadService) GetReplenishmentContext(ctx context.Context, start, end time.Time) ([]ReplenishmentContext, error) {
	if s.DB == nil {
		return nil, errors.New("database unavailable")
	}
	if start.IsZero() || end.Before(start) || end.Sub(start) > 366*24*time.Hour {
		return nil, errors.New("invalid replenishment date range")
	}
	type productStock struct {
		ID         uint
		Name       string
		StoreStock int
		CloudStock int
	}
	products := []productStock{}
	var lastID uint
	for {
		page := []productStock{}
		if err := s.DB.WithContext(ctx).Table("products").
			Select("id, name, store_stock, cloud_stock").
			Where("id > ? AND deleted_at IS NULL", lastID).Order("id").Limit(500).Scan(&page).Error; err != nil {
			return nil, fmt.Errorf("read product stock: %w", err)
		}
		if len(page) == 0 {
			break
		}
		products = append(products, page...)
		lastID = page[len(page)-1].ID
	}
	type salesRow struct {
		ProductID   uint
		ProductName string
		Quantity    int
	}
	sales := []salesRow{}
	if err := s.DB.WithContext(ctx).Table("order_items AS oi").
		Select("oi.product_id, p.name AS product_name, COALESCE(SUM(oi.quantity), 0) AS quantity").
		Joins("JOIN orders AS o ON o.id = oi.order_id").
		Joins("JOIN products AS p ON p.id = oi.product_id AND p.deleted_at IS NULL").
		Where("o.order_status = ? AND o.deleted_at IS NULL AND o.created_at >= ? AND o.created_at <= ?", 1, start, end).
		Group("oi.product_id").Scan(&sales).Error; err != nil {
		return nil, fmt.Errorf("read effective product sales: %w", err)
	}
	byProduct := make(map[uint]int, len(sales))
	nameByProduct := make(map[uint]string, len(sales))
	for _, row := range sales {
		byProduct[row.ProductID] = row.Quantity
		nameByProduct[row.ProductID] = row.ProductName
	}
	observations := make([]ReplenishmentContext, 0, len(products))
	for _, product := range products {
		observations = append(observations, ReplenishmentContext{
			ProductID: product.ID, ProductName: product.Name,
			Available: product.StoreStock + product.CloudStock,
			Sales:     byProduct[product.ID],
		})
	}
	for index := range observations {
		if observations[index].ProductName == "" {
			observations[index].ProductName = nameByProduct[observations[index].ProductID]
		}
	}
	return observations, nil
}

// GetProductSalesSummary returns bounded unit and order counts for valid orders.
// Order discounts are not allocated to products, so the service does not
// invent per-product net revenue.
func (s RetailReadService) GetProductSalesSummary(ctx context.Context, start, end time.Time, limit int) ([]ProductSalesSummary, error) {
	if s.DB == nil {
		return nil, errors.New("database unavailable")
	}
	if start.IsZero() || !end.After(start) || end.Sub(start) > 366*24*time.Hour {
		return nil, errors.New("invalid sales date range")
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("sales summary limit must be between 1 and 100")
	}
	rows := []ProductSalesSummary{}
	err := s.DB.WithContext(ctx).Table("order_items AS oi").
		Select("oi.product_id, p.name AS product_name, COALESCE(SUM(oi.quantity), 0) AS units, COUNT(DISTINCT o.id) AS order_count").
		Joins("JOIN orders AS o ON o.id = oi.order_id").
		Joins("JOIN products AS p ON p.id = oi.product_id AND p.deleted_at IS NULL").
		Where("o.order_status = ? AND o.deleted_at IS NULL AND o.created_at >= ? AND o.created_at < ?", 1, start, end).
		Group("oi.product_id, p.name").Order("units ASC, oi.product_id ASC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("read product sales summary: %w", err)
	}
	return rows, nil
}

// GetProductSalesTrend compares the requested range with the immediately
// preceding range of equal duration. The end boundary is exclusive.
func (s RetailReadService) GetProductSalesTrend(ctx context.Context, start, end time.Time, limit int) ([]ProductSalesTrend, error) {
	if s.DB == nil {
		return nil, errors.New("database unavailable")
	}
	if start.IsZero() || !end.After(start) || end.Sub(start) > 366*24*time.Hour {
		return nil, errors.New("invalid sales trend date range")
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("sales trend limit must be between 1 and 100")
	}
	previousStart := start.Add(-end.Sub(start))
	type aggregate struct {
		ProductID     uint
		ProductName   string
		CurrentUnits  int
		PreviousUnits int
	}
	rows := []aggregate{}
	err := s.DB.WithContext(ctx).Table("order_items AS oi").
		Select("oi.product_id, p.name AS product_name, SUM(CASE WHEN o.created_at >= ? AND o.created_at < ? THEN oi.quantity ELSE 0 END) AS current_units, SUM(CASE WHEN o.created_at >= ? AND o.created_at < ? THEN oi.quantity ELSE 0 END) AS previous_units", start, end, previousStart, start).
		Joins("JOIN orders AS o ON o.id = oi.order_id").
		Joins("JOIN products AS p ON p.id = oi.product_id AND p.deleted_at IS NULL").
		Where("o.order_status = ? AND o.deleted_at IS NULL AND o.created_at >= ? AND o.created_at < ?", 1, previousStart, end).
		Group("oi.product_id, p.name").
		Order("(current_units - previous_units) ASC, oi.product_id ASC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("read product sales trend: %w", err)
	}
	result := make([]ProductSalesTrend, 0, len(rows))
	for _, row := range rows {
		item := ProductSalesTrend{
			ProductID: row.ProductID, ProductName: row.ProductName,
			CurrentUnits: row.CurrentUnits, PreviousUnits: row.PreviousUnits,
			DeltaUnits: row.CurrentUnits - row.PreviousUnits,
		}
		if row.PreviousUnits > 0 {
			percent := float64(item.DeltaUnits) * 100 / float64(row.PreviousUnits)
			item.ChangePercent = &percent
		}
		result = append(result, item)
	}
	return result, nil
}

// GetInventorySalesSummary joins current sellable stock to valid-order sales
// for a bounded time range and result count.
func (s RetailReadService) GetInventorySalesSummary(ctx context.Context, start, end time.Time, limit int) ([]ProductInventorySalesSummary, error) {
	if s.DB == nil {
		return nil, errors.New("database unavailable")
	}
	if start.IsZero() || !end.After(start) || end.Sub(start) > 366*24*time.Hour {
		return nil, errors.New("invalid inventory/sales date range")
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("inventory/sales limit must be between 1 and 100")
	}
	rows := []ProductInventorySalesSummary{}
	err := s.DB.WithContext(ctx).Table("products AS p").
		Select("p.id AS product_id, p.name AS product_name, (p.store_stock + p.cloud_stock) AS available, COALESCE(SUM(CASE WHEN o.id IS NOT NULL THEN oi.quantity ELSE 0 END), 0) AS units, COUNT(DISTINCT o.id) AS order_count").
		Joins("LEFT JOIN order_items AS oi ON oi.product_id = p.id").
		Joins("LEFT JOIN orders AS o ON o.id = oi.order_id AND o.order_status = ? AND o.deleted_at IS NULL AND o.created_at >= ? AND o.created_at < ?", 1, start, end).
		Where("p.deleted_at IS NULL").Group("p.id, p.name, p.store_stock, p.cloud_stock").
		Order("units ASC, p.id ASC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("read inventory and sales summary: %w", err)
	}
	return rows, nil
}
