//go:build integration

package controllers

import (
	"fmt"
	"testing"

	"erp-backend/config"
	"erp-backend/models"
)

func TestEmployeeBatchStocktakeWithoutApproval(t *testing.T) {
	for _, warehouse := range []int{2, 3} {
		t.Run(fmt.Sprintf("warehouse_%d", warehouse), func(t *testing.T) {
			product, employee := inventoryFixture(t)
			batch := models.StocktakeBatch{BatchNo: employee.EmpNo, WarehouseType: warehouse, OperatorID: &employee.ID, OperatorName: employee.Name, Status: "in_progress", StocktakeType: 2}
			if err := config.DB.Create(&batch).Error; err != nil {
				t.Fatal(err)
			}
			actual := 4
			item := models.StocktakeBatchItem{BatchID: batch.ID, ProductID: product.ID, ProductName: product.Name, ExpectedStock: 10, ActualStock: &actual, Remark: "批量盘点差六台", Status: "uncounted"}
			if err := config.DB.Create(&item).Error; err != nil {
				t.Fatal(err)
			}
			result := submitStocktakeBatchItem(batch, item.ID)
			if result.Status != "applied" {
				t.Fatalf("batch result: %+v", result)
			}
			if err := config.DB.First(&item, item.ID).Error; err != nil {
				t.Fatal(err)
			}
			if item.Status != "applied" || item.PendingStocktakeID != nil {
				t.Fatalf("batch item still requires review: %+v", item)
			}
			assertInventoryApplied(t, product, employee, warehouse, -6)
		})
	}
}
