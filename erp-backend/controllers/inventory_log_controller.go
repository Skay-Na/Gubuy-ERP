package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WriteInventoryLog 在事务中写入一条库存流水记录（公用辅助函数）
// tx: 当前事务（保证原子性，流水与主操作同事务回滚）
// productID: 商品ID
// warehouseType: 1-主仓, 2-门店, 3-云仓, 4-样机
// logType: purchase / sale / transfer / stocktake / cancel
// beforeQty: 操作前库存
// changeQty: 变动量（正=增加，负=减少）
// relatedNo: 关联单号（如订单号）
// remark: 说明
func WriteInventoryLog(tx *gorm.DB, productID uint, warehouseType int, logType string, beforeQty int, changeQty int, relatedNo string, remark string) error {
	log := models.InventoryLog{
		ProductID:     productID,
		WarehouseType: warehouseType,
		LogType:       logType,
		ChangeQty:     changeQty,
		BeforeQty:     beforeQty,
		AfterQty:      beforeQty + changeQty,
		RelatedNo:     relatedNo,
		Remark:        remark,
	}
	return tx.Create(&log).Error
}

// GetInventoryLogs 查询指定商品的库存流水记录
// GET /api/products/:id/logs?warehouse_type=&page=&page_size=
func GetInventoryLogs(c *gin.Context) {
	productID := c.Param("id")
	warehouseType := c.Query("warehouse_type")
	page := c.DefaultQuery("page", "1")
	pageSize := c.DefaultQuery("page_size", "50")
	_ = page
	_ = pageSize

	query := config.DB.
		Preload("Product").
		Where("product_id = ?", productID).
		Order("created_at desc")

	if warehouseType != "" {
		query = query.Where("warehouse_type = ?", warehouseType)
	}

	var total int64
	query.Model(&models.InventoryLog{}).Count(&total)

	var offset int
	var limit int
	// Simply use default for now
	offset = 0
	limit = 50

	var logs []models.InventoryLog
	if err := query.Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  200,
		"total": total,
		"data":  logs,
	})
}

// GetGlobalInventoryLogs 查询全局库存流水记录（不限商品）
// GET /api/inventory/logs?warehouse_type=&log_type=&product_id=&page=&page_size=
func GetGlobalInventoryLogs(c *gin.Context) {
	warehouseType := c.Query("warehouse_type")
	logType := c.Query("log_type")
	productID := c.Query("product_id")
	page := c.DefaultQuery("page", "1")
	pageSize := c.DefaultQuery("page_size", "50")
	_ = page
	_ = pageSize

	query := config.DB.
		Preload("Product").
		Order("created_at desc")

	if warehouseType != "" {
		query = query.Where("warehouse_type = ?", warehouseType)
	}
	if logType != "" {
		query = query.Where("log_type = ?", logType)
	}
	if productID != "" {
		query = query.Where("product_id = ?", productID)
	}

	var total int64
	query.Model(&models.InventoryLog{}).Count(&total)

	var offset int
	var limit int
	offset = 0
	limit = 100 // Global logs usually need more visibility

	var logs []models.InventoryLog
	if err := query.Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  200,
		"total": total,
		"data":  logs,
	})
}
