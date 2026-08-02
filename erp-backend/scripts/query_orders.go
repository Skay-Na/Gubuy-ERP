package main

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"erp-backend/models"
)

func main() {
	dsn := "root:@tcp(127.0.0.1:3306)/erp_db?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Println("failed to connect database:", err)
		return
	}

	var orders []models.Order
	db.Find(&orders)
	fmt.Printf("Total orders in DB: %d\n", len(orders))
    for i, o := range orders {
        if i < 5 {
            fmt.Println(o.OrderNo)
        }
    }
}
