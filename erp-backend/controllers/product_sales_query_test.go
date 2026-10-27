package controllers

import (
	"strings"
	"testing"

	"erp-backend/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// dryRunProductDB 打开一个 DryRun 数据库连接（不实际连接 MySQL），用于校验 GORM 生成的 SQL。
func dryRunProductDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	return db
}

// 回归：order_by=sales_qty 全量拉取（不带 page 参数）时若漏设 Model，
// GORM 会按目标结构体 productWithSales 推断表名 product_with_sales（不存在）导致 500。
// 见 ProductGrid.vue fetchProducts 的调用方式：只传 keyword/category_id/order_by，不传 page。
func TestProductSalesQueryUsesProductsTable(t *testing.T) {
	db := dryRunProductDB(t)

	tx := productSalesQuery(db).Find(&[]productWithSales{})
	if tx.Error != nil {
		t.Fatalf("build dry-run query: %v", tx.Error)
	}
	sql := tx.Statement.SQL.String()

	if !strings.Contains(sql, "FROM `products`") {
		t.Fatalf("expected FROM `products` in SQL, got: %s", sql)
	}
	if strings.Contains(sql, "product_with_sales") {
		t.Fatalf("unexpected product_with_sales table in SQL: %s", sql)
	}
	if !strings.Contains(sql, "sales_agg") {
		t.Fatalf("sales aggregate subquery missing in SQL: %s", sql)
	}
	if !strings.Contains(sql, "order_status = 1") {
		t.Fatalf("cancelled orders must be excluded (order_status = 1) in SQL: %s", sql)
	}
}

// 回归：分页路径下 Count 不得受销量 JOIN 影响（total 不重复计数）。
func TestProductSalesPagedCountHasNoJoin(t *testing.T) {
	db := dryRunProductDB(t)

	var total int64
	tx := db.Model(&models.Product{}).Count(&total)
	if tx.Error != nil {
		t.Fatalf("build dry-run count: %v", tx.Error)
	}
	sql := tx.Statement.SQL.String()

	if !strings.Contains(sql, "count(*)") && !strings.Contains(sql, "COUNT(*)") {
		t.Fatalf("expected COUNT(*) in SQL, got: %s", sql)
	}
	if strings.Contains(sql, "sales_agg") {
		t.Fatalf("count query must not join sales aggregate, got: %s", sql)
	}
}
