//go:generate go run github.com/swaggo/swag/cmd/swag init -g main.go -o docs --parseDependency --parseInternal

package main

import (
	"erp-backend/agentnative"
	"erp-backend/config"
	"erp-backend/middlewares"
	"erp-backend/models"
	"erp-backend/routers"
	"fmt"
	"log"
	"os"

	_ "erp-backend/docs"
)

// @title           ERP Backend API
// @version         1.0
// @description     This is the API server for Gubuy-ERP.
// @BasePath        /
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
// @description Use the value: Bearer {token}
func main() {
	if err := middlewares.LoadJWTSecretFromEnvironment(); err != nil {
		log.Fatal("invalid JWT configuration: JWT_SECRET must be set to a secret of at least 32 bytes")
	}
	// 1. 初始化数据库
	config.InitDB()
	if os.Getenv("AGENT_RUNTIME_ENABLED") == "1" {
		agentnative.StartRuntimeWorker(config.DB)
	} else if os.Getenv("AGENT_MONITOR_ENABLED") == "1" {
		agentnative.StartDailyMonitor(config.DB)
	}

	// 自动初始化拼音索引
	config.AutoInitPinyin()

	// 演示数据只能在明确指定的隔离环境中启用。
	if os.Getenv("ERP_SEED_DEMO_DATA") == "1" {
		seedData()
	}

	// 3. 初始化路由
	r := routers.SetupRouter()

	// 设置上传文件大小限制 (100MB)
	r.MaxMultipartMemory = 100 << 20

	// 4. 启动服务
	fmt.Println("Backend service starting on http://localhost:8080")
	r.Run(":8080")
}

func strPtr(s string) *string {
	return &s
}

// seedData 插入 Mock 数据
func seedData() {
	var count int64
	if err := config.DB.Model(&models.Product{}).Count(&count).Error; err != nil {
		log.Printf("demo product count failed: %v", err)
		return
	}
	if count == 0 {
		category := models.Category{Name: "演示商品"}
		if err := config.DB.Where("name = ?", category.Name).FirstOrCreate(&category).Error; err != nil {
			log.Printf("demo category creation failed: %v", err)
			return
		}
		products := []models.Product{
			{
				Name:        "海尔冰箱 BCD-500",
				CategoryID:  category.ID,
				Remark:      strPtr("HR-RF-500"),
				LatestCost:  3200.00,
				MarginRate:  0.15,
				MainStock:   10,
				StoreStock:  0,
				SampleStock: 0,
			},
			{
				Name:        "美的洗衣机 MD-100",
				CategoryID:  category.ID,
				Remark:      strPtr("MD-WM-100"),
				LatestCost:  1800.00,
				MarginRate:  0.12,
				MainStock:   5,
				StoreStock:  0,
				SampleStock: 0,
			},
			{
				Name:        "小米电视 65寸",
				CategoryID:  category.ID,
				Remark:      strPtr("XM-TV-65"),
				LatestCost:  2500.00,
				MarginRate:  0.10,
				MainStock:   8,
				StoreStock:  0,
				SampleStock: 0,
			},
		}
		if err := config.DB.Create(&products).Error; err != nil {
			log.Printf("demo product creation failed: %v", err)
			return
		}
		fmt.Println("Inserted 3 mock products into database.")
	}

	// 初始化管理员账户 (已改为通过前端首次运行初始化)
	/*
		var adminCount int64
		config.DB.Model(&models.AdminUser{}).Count(&adminCount)
		if adminCount == 0 {
			admin := models.AdminUser{
				Username: "admin",
				Password: "admin123", // 默认初始密码
			}
			config.DB.Create(&admin)
			fmt.Println("Created default admin user: admin / admin123")
		}
	*/
}
