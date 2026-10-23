package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"erp-backend/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

// InitDB 初始化数据库连接
func InitDB() {
	dbUser := getEnv("DB_USER", "root")
	dbPass := getEnv("DB_PASS", "")
	dbHost := getEnv("DB_HOST", "127.0.0.1")
	dbPort := getEnv("DB_PORT", "3306")
	dbName := getEnv("DB_NAME", "erp_db")
	if os.Getenv("ERP_AUDIT_DB") == "1" {
		dbName = getEnv("DB_NAME", "erp_audit_test")
		if !strings.HasSuffix(dbName, "_test") {
			log.Fatalf("refusing database %q for audit tests; DB_NAME must end in _test", dbName)
		}
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		dbUser, dbPass, dbHost, dbPort, dbName)

	var db *gorm.DB
	var err error
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("Failed to connect to database (attempt %d/%d): %v", i+1, maxRetries, err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatalf("Failed to connect to database after %d attempts: %v", maxRetries, err)
	}

	fmt.Println("Database connection successful")

	// 自动迁移模型
	err = db.AutoMigrate(
		&models.Account{},
		&models.Employee{},
		&models.EmployeeOffboarding{},
		&models.EmployeeHandoverTask{},
		&models.EmployeeOffboardingAlert{},
		&models.MonthlyAttendance{},
		&models.OrderDailyCounter{},
		&models.Product{},
		&models.Order{},
		&models.OrderItem{},
		&models.Expense{},
		&models.InboundRecord{},
		&models.PendingInbound{},
		&models.PendingStocktake{},
		&models.StocktakeBatch{},
		&models.StocktakeBatchItem{},
		&models.StocktakeRecord{},
		&models.TransferRecord{},
		&models.InventoryLog{},
		&models.HistoricalCostRepair{},
		&models.FinancialLog{},
		&models.AIImport{},
		&models.AIProviderConfig{},
		&models.AIAgentSession{},
		&models.AIAgentMessage{},
		&models.AIAgentAttachment{},
		&models.AIAgentAction{},
		&models.AIAgentMemory{},
		&models.AIAgentMemoryState{},
		&models.AIAgentRun{},
		&models.AIAgentFeedback{},
		&models.AIAgentCase{},
		&models.AIAgentPredictionSnapshot{},
		&models.AIAgentKnowledge{},
		&models.AgentSignal{},
		&models.AgentGoal{},
		&models.AgentWorkTask{},
		&models.AgentWorkTaskStep{},
		&models.AgentToolCallAudit{},
		&models.AgentPlannerCall{},
		&models.AgentSchedule{},
		&models.AgentScheduleRun{},
		&models.AgentReplenishmentDraft{},
		&models.AgentActivity{},
		&models.AdminUser{},
		&models.DailyAttendance{},
		&models.SubsidySettlement{},
		&models.OperationHistory{},
	)
	if err != nil {
		log.Fatalf("Failed to auto migrate database: %v", err)
	}
	// Retire per-product cloud-fulfillment settings for existing products.
	if err := db.Unscoped().Model(&models.Product{}).Where("support_cloud = ?", false).UpdateColumn("support_cloud", true).Error; err != nil {
		log.Fatalf("Failed to enable cloud fulfillment for all products: %v", err)
	}
	// This legacy data migration rewrites every existing product's main stock.
	// Keep it opt-in so deploying Agent Runtime changes cannot silently mutate
	// business inventory. Run it only after a separate data review and backup.
	if os.Getenv("ERP_MIGRATE_UNIFIED_CLOUD_INVENTORY") == "1" {
		if err := migrateUnifiedInventory(db); err != nil {
			log.Fatalf("Failed to migrate unified inventory: %v", err)
		}
	}

	fmt.Println("Database migration completed")
	DB = db

	// 初始化基础账户数据 (Seed)
	seedAccounts(db)
}

// migrateUnifiedInventory removes the old, manually adjusted main warehouse balance
// and enables cloud fulfillment for existing products. Historical inventory logs
// and orders remain available for audit and refunds.
func migrateUnifiedInventory(db *gorm.DB) error {
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS app_migrations (
		version VARCHAR(100) PRIMARY KEY,
		applied_at DATETIME NOT NULL
	)`).Error; err != nil {
		return err
	}

	const version = "20261011_unified_cloud_inventory"
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("app_migrations").Where("version = ?", version).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}

		if err := tx.Unscoped().Model(&models.Product{}).Where("id > ?", 0).Updates(map[string]interface{}{
			"main_stock":    0,
			"support_cloud": true,
		}).Error; err != nil {
			return err
		}
		return tx.Table("app_migrations").Create(map[string]interface{}{
			"version":    version,
			"applied_at": time.Now(),
		}).Error
	})
}

// seedAccounts 初始化基础资金账户（幂等，重复执行不会重复插入）
func seedAccounts(db *gorm.DB) {
	defaultAccounts := []models.Account{
		{Name: "支付宝"},
		{Name: "微信"},
		{Name: "公户"},
		{Name: "现金"},
	}

	for _, acc := range defaultAccounts {
		var existing models.Account
		result := db.Where("name = ?", acc.Name).First(&existing)
		if result.Error != nil {
			// 不存在，则创建
			if err := db.Create(&acc).Error; err != nil {
				log.Printf("Warning: Failed to seed account '%s': %v", acc.Name, err)
			} else {
				fmt.Printf("Seeded account: %s\n", acc.Name)
			}
		}
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
