package controllers

// 商品每台固定提成 · 功能级端到端验收用例（qa-verifier 编写）。
// 复用审计测试基建：真实 MySQL（ERP_AUDIT_DB=1 → erp_audit_test 隔离库）+ gin 裸路由。
// 覆盖：双固定金额商品混算手算对照、赠品不发固定提成、更新归零后回归旧公式、DTO 校验。

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
)

var commE2ERouter *gin.Engine

func commE2ESetup(t *testing.T) {
	t.Helper()
	setupAudit(t) // 复用审计基建：重置隔离库 + 补齐种子账户
	if commE2ERouter == nil {
		commE2ERouter = gin.New()
		commE2ERouter.POST("/api/orders", CreateOrder)
		commE2ERouter.POST("/api/products", CreateProduct)
		commE2ERouter.PUT("/api/products/:id", UpdateProduct)
	}
}

func commE2EPost(t *testing.T, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	jsonValue, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", path, bytes.NewReader(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	commE2ERouter.ServeHTTP(w, req)
	return w
}

func commE2EPut(t *testing.T, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	jsonValue, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", path, bytes.NewReader(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	commE2ERouter.ServeHTTP(w, req)
	return w
}

func commE2EEmployee(t *testing.T, name string, rate float64) uint {
	t.Helper()
	empNo := "QA-" + name
	// setupAudit 不清理 employees 表，先清掉同名残留
	config.DB.Unscoped().Where("emp_no = ?", empNo).Delete(&models.Employee{})
	emp := models.Employee{
		Name:           name,
		EmpNo:          empNo,
		CommissionRate: rate,
	}
	if err := config.DB.Create(&emp).Error; err != nil {
		t.Fatalf("创建员工失败: %v", err)
	}
	return emp.ID
}

func commE2EProduct(t *testing.T, name string, price float64, fixed float64) uint {
	t.Helper()
	categoryID := ensureCategory(t)
	p := models.Product{
		Name:                  name,
		CategoryID:            categoryID,
		LatestCost:            10,
		AvgCost:               10,
		MainStock:             100,
		StoreStock:            100,
		MarginRate:            0.1,
		CommissionFixedAmount: fixed,
	}
	if err := config.DB.Create(&p).Error; err != nil {
		t.Fatalf("创建商品 %s 失败: %v", name, err)
	}
	return p.ID
}

func commE2EOrder(t *testing.T, employeeID uint, items []gin.H, discount float64) *models.Order {
	t.Helper()
	body := gin.H{
		"items":              items,
		"payment_method":     1,
		"delivery_method":    2,
		"discount_amount":    discount,
		"employee_id":        employeeID,
		"bypass_price_check": true,
		"customer_name":      "验收客户",
	}
	w := commE2EPost(t, "/api/orders", body)
	if w.Code != http.StatusOK {
		t.Fatalf("开单失败: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		OrderNo string `json:"order_no"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.OrderNo == "" {
		t.Fatalf("开单未返回订单号: %s", w.Body.String())
	}
	return readOrder(t, resp.OrderNo)
}

func commE2EAssertCommission(t *testing.T, order *models.Order, want float64) {
	t.Helper()
	if math.Abs(order.CommissionAmount-want) > 1e-9 {
		t.Fatalf("订单 %s 提成 = %.2f, 手算期望 = %.2f (total=%.2f actualPay=%.2f)",
			order.OrderNo, order.CommissionAmount, want, order.TotalAmount, order.ActualPayAmount)
	}
}

// 场景 b：双固定金额商品 + 默认商品混算（折扣单）。
// 商品 A(默认, ¥400) + B(固定¥20/台, ¥350) + C(固定¥50/台, ¥250)，抹零 ¥100：total=1000, actualPay=900。
// 手算：默认部分 900×400/1000×0.10=36.00；固定 B=20×1 + C=50×1=70.00 → 合计 106.00。
func TestE2ECommissionDualFixedWithDiscount(t *testing.T) {
	commE2ESetup(t)
	empID := commE2EEmployee(t, "双固定验收员工", 0.10)
	pa := commE2EProduct(t, "验收商品A-跟随默认", 400, 0)
	pb := commE2EProduct(t, "验收商品B-固定20", 350, 20)
	pc := commE2EProduct(t, "验收商品C-固定50", 250, 50)

	order := commE2EOrder(t, empID, []gin.H{
		{"product_id": pa, "quantity": 1, "unit_price": 400},
		{"product_id": pb, "quantity": 1, "unit_price": 350},
		{"product_id": pc, "quantity": 1, "unit_price": 250},
	}, 100)

	if math.Abs(order.TotalAmount-1000) > 0.01 || math.Abs(order.ActualPayAmount-900) > 0.01 {
		t.Fatalf("订单金额异常: total=%.2f actualPay=%.2f", order.TotalAmount, order.ActualPayAmount)
	}
	commE2EAssertCommission(t, order, 106.00)
}

// 场景 c：赠品行不发固定提成。
// A(¥400, 默认) + B(固定¥20/台, ¥350) + C 赠品1台：total=750, actualPay=750。
// 手算：默认 750×400/750×0.10=40.00；B=20.00；C 赠品跳过 → 合计 60.00。
func TestE2ECommissionGiftItemSkipsFixed(t *testing.T) {
	commE2ESetup(t)
	empID := commE2EEmployee(t, "赠品验收员工", 0.10)
	pa := commE2EProduct(t, "赠品验收商品A", 400, 0)
	pb := commE2EProduct(t, "赠品验收商品B", 350, 20)
	pc := commE2EProduct(t, "赠品验收商品C", 250, 50)

	order := commE2EOrder(t, empID, []gin.H{
		{"product_id": pa, "quantity": 1, "unit_price": 400},
		{"product_id": pb, "quantity": 1, "unit_price": 350},
		{"product_id": pc, "quantity": 1, "unit_price": 250, "is_gift": true},
	}, 0)

	commE2EAssertCommission(t, order, 60.00)
}

// 场景 d：商品固定提成 → 归零更新后，再开单回到旧公式。
// A(¥400, 默认) + B(¥350, 固定¥20)，抹零 ¥100：total=750, actualPay=650。
// 更新后全默认：旧公式 commissionBase=650×750/750=650 → 650×0.10=65.00。
// 若 B 仍按固定计提会得 65+20=85.00，与 65.00 可明确区分。
func TestE2ECommissionFixedUpdateRevertsToLegacy(t *testing.T) {
	commE2ESetup(t)
	empID := commE2EEmployee(t, "改模验收员工", 0.10)
	pa := commE2EProduct(t, "改模验收商品A", 400, 0)
	pb := commE2EProduct(t, "改模验收商品B", 350, 20)

	// 通过商品更新接口把 B 的固定提成清零
	w := commE2EPut(t, "/api/products/"+uintToString(pb), gin.H{
		"name": "改模验收商品B", "commission_fixed_amount": 0,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("更新商品失败: %d %s", w.Code, w.Body.String())
	}
	var updated models.Product
	if err := config.DB.First(&updated, pb).Error; err != nil {
		t.Fatalf("查询商品失败: %v", err)
	}
	if updated.CommissionFixedAmount != 0 {
		t.Fatalf("商品 B 更新后固定提成未归零: fixed=%.2f", updated.CommissionFixedAmount)
	}

	order := commE2EOrder(t, empID, []gin.H{
		{"product_id": pa, "quantity": 1, "unit_price": 400},
		{"product_id": pb, "quantity": 1, "unit_price": 350},
	}, 100)

	commE2EAssertCommission(t, order, 65.00)
}

// 场景 e：DTO 校验（创建与更新共用 normalizeProductCommission）。
func TestE2EProductCommissionDTOValidation(t *testing.T) {
	commE2ESetup(t)

	// fixed=-1 → 400
	w := commE2EPost(t, "/api/products", gin.H{
		"name": "校验商品-固定负数", "commission_fixed_amount": -1,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("fixed=-1 期望 400，实际 %d %s", w.Code, w.Body.String())
	}

	// no_commission=true + fixed>0：静默归零
	w = commE2EPost(t, "/api/products", gin.H{
		"name": "校验商品-无提成互斥", "no_commission": true, "category_id": ensureCategory(t),
		"commission_fixed_amount": 50,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("no_commission 互斥用例期望 200，实际 %d %s", w.Code, w.Body.String())
	}
	var created models.Product
	if err := config.DB.Where("name = ?", "校验商品-无提成互斥").First(&created).Error; err != nil {
		t.Fatalf("查询商品失败: %v", err)
	}
	if created.CommissionFixedAmount != 0 {
		t.Fatalf("no_commission=true 未强制归零: fixed=%.2f", created.CommissionFixedAmount)
	}

	// 更新接口同样校验：fixed=-1 → 400
	pa := commE2EProduct(t, "校验商品-更新接口", 100, 0)
	w = commE2EPut(t, "/api/products/"+uintToString(pa), gin.H{
		"name": "校验商品-更新接口", "commission_fixed_amount": -1,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("更新接口校验期望 400，实际 %d %s", w.Code, w.Body.String())
	}
}

func uintToString(v uint) string {
	return json.Number(fmtUint(v)).String()
}

func fmtUint(v uint) string {
	b, _ := json.Marshal(v)
	return string(b)
}
