//go:build integration

package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func inventoryFixture(t *testing.T) (models.Product, models.Employee) {
	t.Helper()
	key := fmt.Sprintf("inventory-%d", time.Now().UnixNano())
	category := models.Category{Name: key}
	if err := config.DB.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	employee := models.Employee{Name: "库存操作员工", EmpNo: key}
	if err := config.DB.Create(&employee).Error; err != nil {
		t.Fatal(err)
	}
	product := models.Product{Name: "回归洗衣机", CategoryID: category.ID, StoreStock: 10, CloudStock: 10, LatestCost: 2000, AvgCost: 2000}
	if err := config.DB.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		config.DB.Where("product_id = ?", product.ID).Delete(&models.PendingInbound{})
		config.DB.Where("product_id = ?", product.ID).Delete(&models.PendingStocktake{})
		config.DB.Where("product_id = ?", product.ID).Delete(&models.StocktakeBatchItem{})
		config.DB.Where("operator_id = ?", employee.ID).Delete(&models.StocktakeBatch{})
		config.DB.Where("product_id = ?", product.ID).Delete(&models.InventoryLog{})
		config.DB.Where("product_id = ?", product.ID).Delete(&models.StocktakeRecord{})
		config.DB.Unscoped().Delete(&product)
		config.DB.Unscoped().Delete(&employee)
		config.DB.Unscoped().Delete(&category)
	})
	return product, employee
}

func callEmployeeInventory(t *testing.T, employee models.Employee, handler gin.HandlerFunc, path string, body gin.H) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("username", "emp_"+employee.EmpNo); c.Next() })
	router.POST("/api/products/:id/stocktake", handler)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	result := httptest.NewRecorder()
	router.ServeHTTP(result, req)
	return result
}

func assertInventoryApplied(t *testing.T, product models.Product, employee models.Employee, warehouse, change int) {
	t.Helper()
	if warehouse == 1 {
		warehouse = 3
	}
	var updated models.Product
	if err := config.DB.First(&updated, product.ID).Error; err != nil {
		t.Fatal(err)
	}
	store, cloud := 10, 10
	if warehouse == 2 {
		store += change
	} else {
		cloud += change
	}
	if updated.StoreStock != store || updated.CloudStock != cloud {
		t.Fatalf("stock store/cloud = %d/%d, want %d/%d", updated.StoreStock, updated.CloudStock, store, cloud)
	}
	var logs []models.InventoryLog
	if err := config.DB.Where("product_id = ?", product.ID).Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("inventory logs = %d, want 1", len(logs))
	}
	log := logs[0]
	if log.OperatorID == nil || *log.OperatorID != employee.ID || log.OperatorName != employee.Name || log.WarehouseType != warehouse || log.BeforeQty != 10 || log.ChangeQty != change || log.AfterQty != 10+change || log.Remark == "" {
		t.Fatalf("invalid audit log: %+v", log)
	}
	var records []models.StocktakeRecord
	if err := config.DB.Where("product_id = ?", product.ID).Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].BeforeStock != 10 || records[0].AfterStock != 10+change {
		t.Fatalf("invalid stock records: %+v", records)
	}
	for _, model := range []any{&models.PendingInbound{}, &models.PendingStocktake{}} {
		var count int64
		if err := config.DB.Model(model).Where("product_id = ?", product.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%T created %d approval requests", model, count)
		}
	}
}

func TestEmployeeStocktakeCanRecordPositiveAdjustmentForEveryWarehouse(t *testing.T) {
	for _, warehouse := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("warehouse_%d", warehouse), func(t *testing.T) {
			product, employee := inventoryFixture(t)
			response := callEmployeeInventory(
				t,
				employee,
				ProductStocktake,
				fmt.Sprintf("/api/products/%d/stocktake", product.ID),
				gin.H{"warehouse_type": warehouse, "actual_stock": 16, "expected_stock": 10, "stocktake_type": 2, "remark": "实物盘点确认库存增加"},
			)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			assertInventoryApplied(t, product, employee, warehouse, 6)
		})
	}
}

func TestEmployeeHighValueOutboundWithoutApproval(t *testing.T) {
	for _, warehouse := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("warehouse_%d", warehouse), func(t *testing.T) {
			product, employee := inventoryFixture(t)
			response := callEmployeeInventory(t, employee, ProductStocktake, fmt.Sprintf("/api/products/%d/stocktake", product.ID), gin.H{"warehouse_type": warehouse, "actual_stock": 9, "expected_stock": 10, "stocktake_type": 2, "remark": "洗衣机出库一台"})
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			assertInventoryApplied(t, product, employee, warehouse, -1)
		})
	}
}

func TestEmployeeStockConflictDoesNotApply(t *testing.T) {
	product, employee := inventoryFixture(t)
	response := callEmployeeInventory(t, employee, ProductStocktake, fmt.Sprintf("/api/products/%d/stocktake", product.ID), gin.H{"warehouse_type": 3, "actual_stock": 9, "expected_stock": 8, "remark": "过期库存快照"})
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var updated models.Product
	if err := config.DB.First(&updated, product.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.CloudStock != 10 {
		t.Fatalf("stock changed despite conflict: %d", updated.CloudStock)
	}
}

func TestEmployeeStocktakeRollsBackWhenAuditFails(t *testing.T) {
	product, employee := inventoryFixture(t)
	callback := "test:reject_inventory_audit"
	if err := config.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "inventory_logs" {
			tx.AddError(errors.New("inventory audit unavailable"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { config.DB.Callback().Create().Remove(callback) })
	response := callEmployeeInventory(t, employee, ProductStocktake, fmt.Sprintf("/api/products/%d/stocktake", product.ID), gin.H{"warehouse_type": 3, "actual_stock": 16, "expected_stock": 10, "stocktake_type": 2, "remark": "盘点差异回滚验证"})
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var updated models.Product
	if err := config.DB.First(&updated, product.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.CloudStock != 10 {
		t.Fatalf("stock escaped rollback: %d", updated.CloudStock)
	}
	var count int64
	if err := config.DB.Model(&models.StocktakeRecord{}).Where("product_id = ?", product.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stock record escaped rollback: %d", count)
	}
}
