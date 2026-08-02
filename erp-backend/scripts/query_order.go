package main

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"erp-backend/models"
    "encoding/json"
)

func main() {
	dsn := "root:@tcp(127.0.0.1:3306)/erp_db?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Println("failed to connect database:", err)
		return
	}

	var order models.Order
	result := db.Unscoped().Where("order_no = ?", "ORD20260606154355729639").First(&order)
	if result.Error != nil {
		fmt.Println("Error:", result.Error)
		return
	}

    b, _ := json.MarshalIndent(order, "", "  ")
	fmt.Println(string(b))
}
