//go:build integration

package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEmployeeInboundWithoutApprovalForEveryWarehouse(t *testing.T) {
	for _, warehouse := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("warehouse_%d", warehouse), func(t *testing.T) {
			product, employee := inventoryFixture(t)
			response := callEmployeeInventory(t, employee, CreatePendingInbound, "/api/pending-inbounds", gin.H{"product_id": product.ID, "warehouse_type": warehouse, "quantity": 6, "submitter_name": "伪造操作人", "remark": "实物到货六台"})
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			assertInventoryApplied(t, product, employee, warehouse, 6)
		})
	}
}
