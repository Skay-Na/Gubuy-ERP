package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"erp-backend/models"

	mysqldsn "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestRetailReadService_ValidatesQueries(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  error
	}{
		{name: "missing database", limit: 10, want: errors.New("database unavailable")},
		{name: "limit too small", limit: 0, want: errors.New("product search limit must be between 1 and 100")},
		{name: "limit too large", limit: 101, want: errors.New("product search limit must be between 1 and 100")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := RetailReadService{}
			if tt.name != "missing database" {
				service.DB = &gorm.DB{}
			}
			_, err := service.SearchProductInventory(context.Background(), "item", tt.limit)
			if err == nil || err.Error() != tt.want.Error() {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRetailReadService_IsolatedMySQLReplenishmentContext(t *testing.T) {
	dsn := os.Getenv("ERP_RETAIL_READ_TEST_DSN")
	if dsn == "" {
		dsn = os.Getenv("ERP_AGENT_NATIVE_TEST_DSN")
	}
	if dsn == "" {
		t.Skip("set ERP_AGENT_NATIVE_TEST_DSN to an isolated test database")
	}
	parsed, err := mysqldsn.ParseDSN(dsn)
	if err != nil || !strings.HasSuffix(parsed.DBName, "_test") {
		t.Fatal("ERP_AGENT_NATIVE_TEST_DSN must point to a database ending in _test")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Product{}, &models.Order{}, &models.OrderItem{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	product := models.Product{Name: fmt.Sprintf("read-test-%d", now.UnixNano()), MainStock: 2, StoreStock: 3, CloudStock: 4, SampleStock: 99}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	order := models.Order{OrderNo: fmt.Sprintf("read-test-%d", now.UnixNano()), OrderStatus: 1, TotalAmount: 210, ActualPayAmount: 210, CreatedAt: now.Add(-time.Hour)}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	item := models.OrderItem{OrderID: order.ID, ProductID: product.ID, UnitPrice: 30, Quantity: 7}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	previousOrder := models.Order{OrderNo: fmt.Sprintf("read-prev-test-%d", now.UnixNano()), OrderStatus: 1, CreatedAt: now.Add(-30 * time.Hour)}
	if err := db.Create(&previousOrder).Error; err != nil {
		t.Fatal(err)
	}
	previousItem := models.OrderItem{OrderID: previousOrder.ID, ProductID: product.ID, UnitPrice: 30, Quantity: 10}
	if err := db.Create(&previousItem).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Delete(&previousItem).Error; err != nil {
			t.Error(err)
		}
		if err := db.Delete(&previousOrder).Error; err != nil {
			t.Error(err)
		}
		if err := db.Delete(&item).Error; err != nil {
			t.Error(err)
		}
		if err := db.Delete(&order).Error; err != nil {
			t.Error(err)
		}
		if err := db.Delete(&product).Error; err != nil {
			t.Error(err)
		}
	}()

	service := RetailReadService{DB: db}
	rows, err := service.GetReplenishmentContext(context.Background(), now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, row := range rows {
		if row.ProductID == product.ID {
			found = true
			if row.Available != 7 || row.Sales != 7 {
				t.Fatalf("replenishment context = %+v, want available=7 sales=7 (main stock is retired)", row)
			}
		}
	}
	if !found {
		t.Fatalf("product %d not found in replenishment context", product.ID)
	}
	trends, err := service.GetProductSalesTrend(context.Background(), now.Add(-24*time.Hour), now, 10)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, trend := range trends {
		if trend.ProductID == product.ID {
			found = true
			if trend.CurrentUnits != 7 || trend.PreviousUnits != 10 || trend.DeltaUnits != -3 || trend.ChangePercent == nil || *trend.ChangePercent != -30 {
				t.Fatalf("sales trend = %+v, want 7 current, 10 previous, -3 units, -30%%", trend)
			}
		}
	}
	if !found {
		t.Fatalf("product %d not found in sales trend", product.ID)
	}
	products, err := service.SearchProductInventory(context.Background(), product.Name, 10)
	if err != nil || len(products) != 1 || products[0].ID != product.ID {
		t.Fatalf("search product inventory = %+v, err=%v", products, err)
	}
}
