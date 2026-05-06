package main

import (
	"erp-backend/config"
	"erp-backend/models"
	"erp-backend/routers"
	"fmt"
)

func main() {
	// 1. 初始化数据库
	config.InitDB()

	// 2. 插入测试数据 (如果数据库为空)
	seedData()

	// 3. 初始化路由
	r := routers.SetupRouter()

	// 4. 启动服务
	fmt.Println("Backend service starting on http://localhost:8080")
	r.Run(":8080")
}

// seedData 插入 Mock 数据
func seedData() {
	var count int64
	config.DB.Model(&models.Product{}).Count(&count)
	if count == 0 {
		products := []models.Product{
			{
				Name:       "海尔冰箱 BCD-500",
				SKU:        "HR-RF-500",
				LatestCost: 3200.00,
				MarginRate:  0.15,
				MainStock:   10,
				StoreStock:  0,
				SampleStock: 0,
			},
			{
				Name:       "美的洗衣机 MD-100",
				SKU:        "MD-WM-100",
				LatestCost: 1800.00,
				MarginRate:  0.12,
				MainStock:   5,
				StoreStock:  0,
				SampleStock: 0,
			},
			{
				Name:       "小米电视 65寸",
				SKU:        "XM-TV-65",
				LatestCost: 2500.00,
				MarginRate:  0.10,
				MainStock:   8,
				StoreStock:  0,
				SampleStock: 0,
			},
		}
		config.DB.Create(&products)
		fmt.Println("Inserted 3 mock products into database.")
	}

	// 初始化管理员账户
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
}
