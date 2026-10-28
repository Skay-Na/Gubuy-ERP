package controllers

// 单商品单独设置提成 · 功能级端到端验收用例（qa-verifier 编写）。
// 复用审计测试基建：真实 MySQL（ERP_AUDIT_DB=1 → erp_audit_test 隔离库）+ gin 裸路由。
// 覆盖：混合模式开单手算对照、赠品不发固定提成、mode 更新后回归旧公式、DTO 校验。

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

func commE2EProduct(t *testing.T, name string, price float64, mode int, rate, fixed float64) uint {
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
		CommissionMode:        mode,
		CommissionRate:        rate,
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
		"items":             items,
		"payment_method":    1,
		"delivery_method":   2,
		"discount_amount":   discount,
		"employee_id":       employeeID,
		"bypass_price_check": true,
		"customer_name":     "验收客户",
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

// 场景 b：混合模式 + 折扣单。
// 商品 A(mode=0, ¥400) + B(mode=1 rate=0.20, ¥350) + C(mode=2 fixed=¥50, ¥250)，
// 抹零 ¥100：total=1000, actualPay=900, K=0.9。
// 手算：默认部分 900×400/1000×0.10=36.00；B=350×0.9×0.20=63.00；C=50×1=50.00 → 合计 149.00。
func TestE2ECommissionMixedModesWithDiscount(t *testing.T) {
	commE2ESetup(t)
	empID := commE2EEmployee(t, "混合模式验收员工", 0.10)
	pa := commE2EProduct(t, "验收商品A-跟随默认", 400, 0, 0, 0)
	pb := commE2EProduct(t, "验收商品B-按比例", 350, 1, 0.20, 0)
	pc := commE2EProduct(t, "验收商品C-固定每台", 250, 2, 0, 50)

	order := commE2EOrder(t, empID, []gin.H{
		{"product_id": pa, "quantity": 1, "unit_price": 400},
		{"product_id": pb, "quantity": 1, "unit_price": 350},
		{"product_id": pc, "quantity": 1, "unit_price": 250},
	}, 100)

	if math.Abs(order.TotalAmount-1000) > 0.01 || math.Abs(order.ActualPayAmount-900) > 0.01 {
		t.Fatalf("订单金额异常: total=%.2f actualPay=%.2f", order.TotalAmount, order.ActualPayAmount)
	}
	commE2EAssertCommission(t, order, 149.00)
}

// 场景 c：赠品行不发固定提成。
// A(¥400) + B(¥350) + C 赠品1台：total=750, actualPay=750, K=1。
// 手算：默认 750×400/750×0.10=40.00；B=350×1×0.20=70.00；C 赠品跳过 → 合计 110.00。
func TestE2ECommissionGiftItemSkipsFixed(t *testing.T) {
	commE2ESetup(t)
	empID := commE2EEmployee(t, "赠品验收员工", 0.10)
	pa := commE2EProduct(t, "赠品验收商品A", 400, 0, 0, 0)
	pb := commE2EProduct(t, "赠品验收商品B", 350, 1, 0.20, 0)
	pc := commE2EProduct(t, "赠品验收商品C", 250, 2, 0, 50)

	order := commE2EOrder(t, empID, []gin.H{
		{"product_id": pa, "quantity": 1, "unit_price": 400},
		{"product_id": pb, "quantity": 1, "unit_price": 350},
		{"product_id": pc, "quantity": 1, "unit_price": 250, "is_gift": true},
	}, 0)

	commE2EAssertCommission(t, order, 110.00)
}

// 场景 d：商品 mode=1 → mode=0 更新后，再开单回到旧公式。
// A(¥400) + B(¥350)，抹零 ¥100：total=750, actualPay=650。
// 更新后全 mode=0：旧公式 commissionBase=650×750/750=650 → 650×0.10=65.00。
// 若 B 仍按 mode=1 计提会得 400+350×(650/750)×0.2=125.67，与 65.00 可明确区分。
func TestE2ECommissionModeUpdateRevertsToLegacy(t *testing.T) {
	commE2ESetup(t)
	empID := commE2EEmployee(t, "改模验收员工", 0.10)
	pa := commE2EProduct(t, "改模验收商品A", 400, 0, 0, 0)
	pb := commE2EProduct(t, "改模验收商品B", 350, 1, 0.20, 0)

	// 通过商品更新接口把 B 从 mode=1 改回 mode=0
	w := commE2EPut(t, "/api/products/"+uintToString(pb), gin.H{
		"name":            "改模验收商品B",
		"commission_mode": 0,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("更新商品失败: %d %s", w.Code, w.Body.String())
	}
	var updated models.Product
	if err := config.DB.First(&updated, pb).Error; err != nil {
		t.Fatalf("查询商品失败: %v", err)
	}
	if updated.CommissionMode != 0 || updated.CommissionRate != 0 || updated.CommissionFixedAmount != 0 {
		t.Fatalf("商品 B 更新后提成字段未归零: mode=%d rate=%.4f fixed=%.2f",
			updated.CommissionMode, updated.CommissionRate, updated.CommissionFixedAmount)
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
	cases := []struct {
		name string
		body gin.H
		want string
	}{
		{"mode=1 rate=0", gin.H{"name": "校验商品-比例0", "commission_mode": 1, "commission_rate": 0}, "请设置提成比例（大于0且不超过1）"},
		{"mode=1 rate=1.5", gin.H{"name": "校验商品-比例1.5", "commission_mode": 1, "commission_rate": 1.5}, "请设置提成比例（大于0且不超过1）"},
		{"mode=2 fixed=0", gin.H{"name": "校验商品-固定0", "commission_mode": 2, "commission_fixed_amount": 0}, "请设置每台固定提成金额"},
		{"mode=2 fixed=-1", gin.H{"name": "校验商品-固定负数", "commission_mode": 2, "commission_fixed_amount": -1}, "请设置每台固定提成金额"},
		{"mode=3", gin.H{"name": "校验商品-模式3", "commission_mode": 3}, "提成模式非法"},
	}
	for _, tc := range cases {
		w := commE2EPost(t, "/api/products", tc.body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: 期望 400，实际 %d %s", tc.name, w.Code, w.Body.String())
		}
		var resp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Msg != tc.want {
			t.Fatalf("%s: 期望文案 %q，实际 %q", tc.name, tc.want, resp.Msg)
		}
	}

	// no_commission=true 与单独提成互斥：静默归零
	w := commE2EPost(t, "/api/products", gin.H{
		"name": "校验商品-无提成互斥", "no_commission": true, "category_id": ensureCategory(t),
		"commission_mode": 2, "commission_fixed_amount": 50,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("no_commission 互斥用例期望 200，实际 %d %s", w.Code, w.Body.String())
	}
	var created models.Product
	if err := config.DB.Where("name = ?", "校验商品-无提成互斥").First(&created).Error; err != nil {
		t.Fatalf("查询商品失败: %v", err)
	}
	if created.CommissionMode != 0 || created.CommissionRate != 0 || created.CommissionFixedAmount != 0 {
		t.Fatalf("no_commission=true 未强制归零: mode=%d rate=%.4f fixed=%.2f",
			created.CommissionMode, created.CommissionRate, created.CommissionFixedAmount)
	}

	// 更新接口同样校验
	pa := commE2EProduct(t, "校验商品-更新接口", 100, 0, 0, 0)
	w = commE2EPut(t, "/api/products/"+uintToString(pa), gin.H{
		"name": "校验商品-更新接口", "commission_mode": 1, "commission_rate": 0,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("更新接口校验期望 400，实际 %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Msg string `json:"msg"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Msg != "请设置提成比例（大于0且不超过1）" {
		t.Fatalf("更新接口校验文案异常: %q", resp.Msg)
	}
}

func uintToString(v uint) string {
	return json.Number(fmtUint(v)).String()
}

func fmtUint(v uint) string {
	b, _ := json.Marshal(v)
	return string(b)
}
