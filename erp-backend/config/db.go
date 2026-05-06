package config

import (
	"fmt"
	"log"

	"erp-backend/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

// InitDB 初始化数据库连接
func InitDB() {
	// 请根据实际情况修改 DSN (Data Source Name)
	dsn := "root:@tcp(127.0.0.1:3306)/erp_db?charset=utf8mb4&parseTime=True&loc=Local"

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	fmt.Println("Database connection successful")

	// 自动迁移模型
	err = db.AutoMigrate(
		&models.Account{},
		&models.Employee{},
		&models.MonthlyAttendance{},
		&models.Product{},
		&models.Order{},
		&models.OrderItem{},
		&models.Expense{},
		&models.InboundRecord{},
		&models.StocktakeRecord{},
		&models.TransferRecord{},
		&models.InventoryLog{},
		&models.FinancialLog{},
		&models.AdminUser{},
		&models.DailyAttendance{},
	)
	if err != nil {
		log.Fatalf("Failed to auto migrate database: %v", err)
	}

	fmt.Println("Database migration completed")
	DB = db

	// 初始化基础账户数据 (Seed)
	seedAccounts(db)
}

// seedAccounts 初始化三个基础资金账户（幂等，重复执行不会重复插入）
func seedAccounts(db *gorm.DB) {
	defaultAccounts := []models.Account{
		{Name: "支付宝"},
		{Name: "微信"},
		{Name: "公户"},
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
