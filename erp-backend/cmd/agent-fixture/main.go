// Command agent-fixture seeds the versioned Agent evaluation data into an isolated test database.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"gorm.io/gorm"
)

const (
	fixtureProductName  = "AgentEvalFixtureV1"
	fixtureCategoryName = "AgentEvalFixtureV1"
	fixtureOrderPrefix  = "AGENT-EVAL-V1-"
)

type evaluationFixture struct {
	DatasetVersion string                        `json:"dataset_version"`
	ReferenceDate  string                        `json:"reference_date"`
	Timezone       string                        `json:"timezone"`
	Expected       map[string]map[string]float64 `json:"expected"`
	SeedHelper     string                        `json:"seed_helper"`
	Fixture        []struct {
		Dataset        string  `json:"dataset"`
		OrderMonth     string  `json:"order_month"`
		OrderStatus    int     `json:"order_status"`
		TotalAmount    float64 `json:"total_amount"`
		DiscountAmount float64 `json:"discount_amount"`
		Items          []struct {
			UnitPrice float64 `json:"unit_price"`
			UnitCost  float64 `json:"unit_cost"`
			Quantity  int     `json:"quantity"`
		} `json:"items"`
	} `json:"fixture"`
}

func validateDatabaseTarget(database, confirmation string) error {
	if database == "" || database != confirmation {
		return errors.New("set --database-confirmation to exactly match DB_NAME")
	}
	if !strings.HasPrefix(database, "erp_agent_") || !strings.HasSuffix(database, "_test") {
		return errors.New("refusing to seed: DB_NAME must start with erp_agent_ and end with _test")
	}
	return nil
}

func loadFixture(path string) (evaluationFixture, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return evaluationFixture{}, fmt.Errorf("read fixture: %w", err)
	}
	var fixture evaluationFixture
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		return fixture, fmt.Errorf("decode fixture: %w", err)
	}
	if fixture.DatasetVersion != "agent-core-v1" || fixture.ReferenceDate != "2026-10-01" || fixture.Timezone != "Asia/Shanghai" || len(fixture.Fixture) == 0 {
		return fixture, errors.New("fixture metadata is invalid or unsupported")
	}
	for _, order := range fixture.Fixture {
		month, err := time.Parse("2006-01", order.OrderMonth)
		if err != nil || month.Format("2006-01") != order.OrderMonth || order.Dataset != "orders" || order.OrderStatus != 1 || order.TotalAmount <= 0 || order.DiscountAmount < 0 || len(order.Items) == 0 {
			return fixture, fmt.Errorf("invalid order fixture for month %q", order.OrderMonth)
		}
		for _, item := range order.Items {
			if item.UnitPrice <= 0 || item.UnitCost < 0 || item.Quantity <= 0 {
				return fixture, fmt.Errorf("invalid order item fixture for month %q", order.OrderMonth)
			}
		}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fixture, errors.New("fixture must contain one JSON object")
	}
	return fixture, nil
}

func seedFixture(db *gorm.DB, fixture evaluationFixture) error {
	return db.Transaction(func(tx *gorm.DB) error {
		orderNumbers := make([]string, 0, len(fixture.Fixture))
		for _, order := range fixture.Fixture {
			orderNumbers = append(orderNumbers, fixtureOrderPrefix+strings.ReplaceAll(order.OrderMonth, "-", ""))
		}
		if err := tx.Exec("DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE order_no IN ?)", orderNumbers).Error; err != nil {
			return fmt.Errorf("remove prior fixture items: %w", err)
		}
		if err := tx.Unscoped().Where("order_no IN ?", orderNumbers).Delete(&models.Order{}).Error; err != nil {
			return fmt.Errorf("remove prior fixture orders: %w", err)
		}
		if err := tx.Unscoped().Where("name = ?", fixtureProductName).Delete(&models.Product{}).Error; err != nil {
			return fmt.Errorf("remove prior fixture product: %w", err)
		}
		if err := tx.Unscoped().Where("name = ?", fixtureCategoryName).Delete(&models.Category{}).Error; err != nil {
			return fmt.Errorf("remove prior fixture category: %w", err)
		}
		category := models.Category{Name: fixtureCategoryName}
		if err := tx.Create(&category).Error; err != nil {
			return fmt.Errorf("create fixture category: %w", err)
		}
		product := models.Product{Name: fixtureProductName, CategoryID: category.ID, LatestCost: 100, AvgCost: 100, MarginRate: 0.1}
		if err := tx.Create(&product).Error; err != nil {
			return fmt.Errorf("create fixture product: %w", err)
		}
		for _, input := range fixture.Fixture {
			orderDate, err := time.ParseInLocation("2006-01", input.OrderMonth, time.Local)
			if err != nil {
				return fmt.Errorf("parse order month: %w", err)
			}
			items := make([]models.OrderItem, 0, len(input.Items))
			for _, item := range input.Items {
				items = append(items, models.OrderItem{ProductID: product.ID, UnitPrice: item.UnitPrice, UnitCost: item.UnitCost, Quantity: item.Quantity})
			}
			order := models.Order{
				OrderNo:      fixtureOrderPrefix + strings.ReplaceAll(input.OrderMonth, "-", ""),
				CustomerName: "评估样本客户", OrderStatus: input.OrderStatus, OrderType: 1,
				TotalAmount: input.TotalAmount, ActualPayAmount: input.TotalAmount - input.DiscountAmount,
				DiscountAmount: input.DiscountAmount, CreatedAt: orderDate.AddDate(0, 0, 9), OrderItems: items,
			}
			if err := tx.Create(&order).Error; err != nil {
				return fmt.Errorf("create %s fixture order: %w", input.OrderMonth, err)
			}
		}
		return nil
	})
}

func verifyFixture(db *gorm.DB, fixture evaluationFixture) error {
	for _, input := range fixture.Fixture {
		orderNumber := fixtureOrderPrefix + strings.ReplaceAll(input.OrderMonth, "-", "")
		var actual struct {
			SalesRevenue float64 `gorm:"column:sales_revenue"`
			OrderCount   int64   `gorm:"column:order_count"`
			Units        int64   `gorm:"column:units"`
		}
		if err := db.Table("orders AS orders").
			Select("COALESCE(SUM(orders.total_amount - orders.discount_amount),0) AS sales_revenue, COUNT(DISTINCT orders.id) AS order_count, COALESCE(SUM(order_items.quantity),0) AS units").
			Joins("LEFT JOIN order_items ON order_items.order_id = orders.id").
			Where("orders.order_no = ? AND orders.order_status = ? AND orders.deleted_at IS NULL", orderNumber, 1).
			Scan(&actual).Error; err != nil {
			return fmt.Errorf("verify %s: %w", input.OrderMonth, err)
		}
		expected := fixture.Expected[input.OrderMonth]
		if actual.SalesRevenue != expected["sales_revenue"] || float64(actual.OrderCount) != expected["order_count"] || float64(actual.Units) != expected["units"] {
			return fmt.Errorf("fixture %s mismatch: got revenue %.2f, orders %d, units %d", input.OrderMonth, actual.SalesRevenue, actual.OrderCount, actual.Units)
		}
	}
	return nil
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("agent-fixture", flag.ContinueOnError)
	flags.SetOutput(stderr)
	apply := flags.Bool("apply", false, "replace only the versioned Agent evaluation fixture")
	confirmation := flags.String("database-confirmation", "", "must exactly match DB_NAME; only erp_agent_*_test databases are accepted")
	fixturePath := flags.String("fixture", "../docs/agent-evals/fixture-v1.json", "fixture JSON path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if !*apply || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "refusing to write without --apply and no positional arguments")
		return 2
	}
	database := os.Getenv("DB_NAME")
	if err := validateDatabaseTarget(database, *confirmation); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	fixture, err := loadFixture(*fixturePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := os.Setenv("ERP_AUDIT_DB", "1"); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	config.InitDB()
	if err := seedFixture(config.DB, fixture); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := verifyFixture(config.DB, fixture); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "seeded %s into %s (%d orders)\n", fixture.DatasetVersion, database, len(fixture.Fixture))
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
