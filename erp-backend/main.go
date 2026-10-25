//go:generate go run github.com/swaggo/swag/cmd/swag init -g main.go -o docs --parseDependency --parseInternal

package main

import (
	"erp-backend/agentnative"
	"erp-backend/config"
	"erp-backend/controllers"
	"erp-backend/middlewares"
	"erp-backend/models"
	"erp-backend/routers"
	"fmt"
	"log"
	"os"

	_ "erp-backend/docs"
	"github.com/gin-gonic/gin"
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
	// 生产环境关闭 gin 调试模式（不打印路由调试信息与告警横幅）
	if os.Getenv("APP_ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	if err := middlewares.LoadJWTSecretFromEnvironment(); err != nil {
		log.Fatal("invalid JWT configuration: JWT_SECRET must be set to a secret of at least 32 bytes")
	}
	// 1. 初始化数据库
	config.InitDB()
	runtimeEnabled := os.Getenv("AGENT_RUNTIME_ENABLED") == "1"
	legacyTaskWorkerEnabled := os.Getenv("AGENT_PERSISTENT_SCHEDULER_ENABLED") == "1"
	var planner agentnative.AgentPlanner
	if os.Getenv("AGENT_LLM_PLANNER_ENABLED") == "1" {
		planner = controllers.NewRuntimePlanner()
	}
	if legacyTaskWorkerEnabled || runtimeEnabled {
		// Durable user-requested tasks remain available, but inventory alert and
		// replenishment scans are retired and must not run at startup.
		agentnative.StartAgentTaskWorker(config.DB, planner)
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
	listenAddr := os.Getenv("BACKEND_LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":8080"
	}
	fmt.Printf("Backend service starting on %s\n", listenAddr)
	if err := r.Run(listenAddr); err != nil {
		log.Fatalf("backend server stopped: %v", err)
	}
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
				CloudStock:  10,
				StoreStock:  0,
				SampleStock: 0,
			},
			{
				Name:        "美的洗衣机 MD-100",
				CategoryID:  category.ID,
				Remark:      strPtr("MD-WM-100"),
				LatestCost:  1800.00,
				MarginRate:  0.12,
				CloudStock:  5,
				StoreStock:  0,
				SampleStock: 0,
			},
			{
				Name:        "小米电视 65寸",
				CategoryID:  category.ID,
				Remark:      strPtr("XM-TV-65"),
				LatestCost:  2500.00,
				MarginRate:  0.10,
				CloudStock:  8,
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
