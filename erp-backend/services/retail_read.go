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
	ProductID uint
	Available int
	Sales     int
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
	err := s.DB.WithContext(ctx).
		Select("id", "name", "store_stock", "main_stock", "cloud_stock").
		Where("name LIKE ?", "%"+keyword+"%").Order("id DESC").Limit(limit).Find(&products).Error
	if err != nil {
		return nil, fmt.Errorf("search product inventory: %w", err)
	}
	return products, nil
}

// GetReplenishmentContext uses valid orders (order_status=1) and the same
// sellable-stock sum used by the monitor: main + store + cloud, excluding samples.
func (s RetailReadService) GetReplenishmentContext(ctx context.Context, start, end time.Time) ([]ReplenishmentContext, error) {
	if s.DB == nil {
		return nil, errors.New("database unavailable")
	}
	if start.IsZero() || end.Before(start) || end.Sub(start) > 366*24*time.Hour {
		return nil, errors.New("invalid replenishment date range")
	}
	type productStock struct {
		ID         uint
		MainStock  int
		StoreStock int
		CloudStock int
	}
	products := []productStock{}
	var lastID uint
	for {
		page := []productStock{}
		if err := s.DB.WithContext(ctx).Table("products").
			Select("id, main_stock, store_stock, cloud_stock").
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
		ProductID uint
		Quantity  int
	}
	sales := []salesRow{}
	if err := s.DB.WithContext(ctx).Table("order_items AS oi").
		Select("oi.product_id, COALESCE(SUM(oi.quantity), 0) AS quantity").
		Joins("JOIN orders AS o ON o.id = oi.order_id").
		Where("o.order_status = ? AND o.deleted_at IS NULL AND o.created_at >= ? AND o.created_at <= ?", 1, start, end).
		Group("oi.product_id").Scan(&sales).Error; err != nil {
		return nil, fmt.Errorf("read effective product sales: %w", err)
	}
	byProduct := make(map[uint]int, len(sales))
	for _, row := range sales {
		byProduct[row.ProductID] = row.Quantity
	}
	observations := make([]ReplenishmentContext, 0, len(products))
	for _, product := range products {
		observations = append(observations, ReplenishmentContext{
			ProductID: product.ID,
			Available: product.MainStock + product.StoreStock + product.CloudStock,
			Sales:     byProduct[product.ID],
		})
	}
	return observations, nil
}
