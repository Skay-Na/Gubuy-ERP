package config

import (
	"erp-backend/models"
	"erp-backend/utils"
	"fmt"
)

// AutoInitPinyin 自动为历史数据补充拼音索引
func AutoInitPinyin() {
	var count int64
	// 检查是否有未生成拼音的员工（简化判断，如果有个别没生成就全量扫一次空值）
	DB.Model(&models.Employee{}).Where("pinyin = ? OR pinyin IS NULL", "").Count(&count)
	if count > 0 {
		var employees []models.Employee
		DB.Where("pinyin = ? OR pinyin IS NULL", "").Find(&employees)
		for _, emp := range employees {
			if emp.Name != "" {
				pStr, pInit := utils.GetPinyin(emp.Name)
				DB.Model(&emp).Updates(map[string]interface{}{
					"pinyin":         pStr,
					"pinyin_initial": pInit,
				})
			}
		}
		fmt.Printf("Auto initialized pinyin for %d employees.\n", count)
	}

	var orderCount int64
	DB.Model(&models.Order{}).Where("(customer_pinyin = ? OR customer_pinyin IS NULL) AND customer_name != ?", "", "").Count(&orderCount)
	if orderCount > 0 {
		var orders []models.Order
		DB.Where("(customer_pinyin = ? OR customer_pinyin IS NULL) AND customer_name != ?", "", "").Find(&orders)
		for _, order := range orders {
			if order.CustomerName != "" {
				pStr, pInit := utils.GetPinyin(order.CustomerName)
				DB.Model(&order).Updates(map[string]interface{}{
					"customer_pinyin":         pStr,
					"customer_pinyin_initial": pInit,
				})
			}
		}
		fmt.Printf("Auto initialized pinyin for %d orders.\n", orderCount)
	}
}
