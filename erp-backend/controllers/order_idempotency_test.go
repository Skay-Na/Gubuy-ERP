package controllers

// 开单幂等专项回归（对应 commit 7f7c5ca）：
// 1. 同一 client_request_id 二次提交返回相同订单且 duplicated:true；
// 2. 不产生第二单、不重复扣库存、不重复写资金流水；
// 3. 并发同键提交最终只落一单（唯一索引兜底）；
// 4. 不同键各自开单互不影响；超长键(>64)被参数校验拒绝。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
)

func idemOrderBody(productID uint, key string) gin.H {
	body := gin.H{
		"items": []gin.H{
			{"product_id": productID, "quantity": 2, "unit_price": 1000.00},
		},
		"payment_method":  5, // 现金
		"delivery_method": 2,
		"customer_name":   "幂等测试客户",
		"customer_phone":  "13611111111",
	}
	if key != "" {
		body["client_request_id"] = key
	}
	return body
}

func parseOrderNo(t *testing.T, body []byte) string {
	t.Helper()
	var resp struct {
		OrderNo string `json:"order_no"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.OrderNo
}

func countFinancialLogs(t *testing.T, orderNo string) int64 {
	t.Helper()
	var n int64
	if err := config.DB.Model(&models.FinancialLog{}).Where("related_no = ?", orderNo).Count(&n).Error; err != nil {
		t.Fatalf("统计资金流水失败: %v", err)
	}
	return n
}

func idemProductStock(t *testing.T, productID uint) int {
	t.Helper()
	var p models.Product
	if err := config.DB.Select("main_stock").First(&p, productID).Error; err != nil {
		t.Fatalf("查询商品库存失败: %v", err)
	}
	return p.MainStock
}

// TestCreateOrderIdempotentSameKey 顺序重复提交：第二次必须返回原订单
// 且 duplicated:true，库存/账户/流水零重复。
func TestCreateOrderIdempotentSameKey(t *testing.T) {
	setupAudit(t)
	productID := ensureProduct(t, "幂等测试商品A", 1000.00, 600, 100)
	key := fmt.Sprintf("qa-idem-%d", time.Now().UnixNano())

	w1 := postJSON("/api/orders", idemOrderBody(productID, key))
	if w1.Code != http.StatusOK {
		t.Fatalf("首次开单失败: %d %s", w1.Code, w1.Body.String())
	}
	firstNo := parseOrderNo(t, w1.Body.Bytes())
	if firstNo == "" {
		t.Fatalf("首次开单未返回订单号: %s", w1.Body.String())
	}
	var firstResp struct {
		Duplicated bool `json:"duplicated"`
	}
	_ = json.Unmarshal(w1.Body.Bytes(), &firstResp)
	if firstResp.Duplicated {
		t.Fatal("首次开单不应标记 duplicated")
	}

	stockAfterFirst := idemProductStock(t, productID)
	balanceAfterFirst := accountBalance(t, "现金")
	logsAfterFirst := countFinancialLogs(t, firstNo)

	// 同键重复提交（双击场景）
	w2 := postJSON("/api/orders", idemOrderBody(productID, key))
	if w2.Code != http.StatusOK {
		t.Fatalf("同键重复提交应返回 200 与已有订单，实际 %d %s", w2.Code, w2.Body.String())
	}
	var secondResp struct {
		OrderNo    string `json:"order_no"`
		Duplicated bool   `json:"duplicated"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &secondResp)
	if !secondResp.Duplicated {
		t.Fatalf("同键重复提交应返回 duplicated:true，实际响应: %s", w2.Body.String())
	}
	if secondResp.OrderNo != firstNo {
		t.Fatalf("同键重复提交应返回原订单号 %s，实际 %s", firstNo, secondResp.OrderNo)
	}

	// 不产生第二单
	var orderCount int64
	if err := config.DB.Model(&models.Order{}).Where("client_request_id = ?", key).Count(&orderCount).Error; err != nil {
		t.Fatalf("按幂等键统计订单失败: %v", err)
	}
	if orderCount != 1 {
		t.Fatalf("同幂等键应只存在 1 笔订单，实际 %d 笔", orderCount)
	}
	// 库存只扣一次、账户只收一次款、流水只写一次
	if got := idemProductStock(t, productID); got != stockAfterFirst {
		t.Fatalf("重复提交不应再次扣库存: 首次后 %d，重复后 %d", stockAfterFirst, got)
	}
	if got := accountBalance(t, "现金"); got != balanceAfterFirst {
		t.Fatalf("重复提交不应再次收款: 首次后 %.2f，重复后 %.2f", balanceAfterFirst, got)
	}
	if got := countFinancialLogs(t, firstNo); got != logsAfterFirst {
		t.Fatalf("重复提交不应重复写资金流水: 首次后 %d 条，重复后 %d 条", logsAfterFirst, got)
	}
}

// TestCreateOrderDistinctKeysIndependent 不同幂等键各自开单互不影响；
// 不带键开单（历史客户端路径）也保持可用。
func TestCreateOrderDistinctKeysIndependent(t *testing.T) {
	setupAudit(t)
	productID := ensureProduct(t, "幂等测试商品B", 1000.00, 600, 100)

	responses := make([]*httptest.ResponseRecorder, 3)
	responses[0] = postJSON("/api/orders", idemOrderBody(productID, "qa-idem-key-one"))
	responses[1] = postJSON("/api/orders", idemOrderBody(productID, "qa-idem-key-two"))
	responses[2] = postJSON("/api/orders", idemOrderBody(productID, "")) // 无键
	for i, w := range responses {
		if w.Code != http.StatusOK {
			t.Fatalf("第 %d 次开单应成功，实际 %d %s", i+1, w.Code, w.Body.String())
		}
	}
	no1 := parseOrderNo(t, responses[0].Body.Bytes())
	no2 := parseOrderNo(t, responses[1].Body.Bytes())
	no3 := parseOrderNo(t, responses[2].Body.Bytes())
	if no1 == "" || no2 == "" || no3 == "" {
		t.Fatalf("三笔订单均应返回订单号: %s / %s / %s", no1, no2, no3)
	}
	if no1 == no2 || no1 == no3 || no2 == no3 {
		t.Fatalf("不同键（含无键）不应互相命中: %s / %s / %s", no1, no2, no3)
	}
	var total int64
	if err := config.DB.Model(&models.Order{}).Where("customer_phone = ?", "13611111111").Count(&total).Error; err != nil {
		t.Fatalf("统计订单失败: %v", err)
	}
	if total != 3 {
		t.Fatalf("三笔请求应产生 3 笔订单，实际 %d", total)
	}
}

// TestCreateOrderConcurrentSameKeySingleOrder 并发同键：多个请求同时提交
// 同一幂等键，最终库里有且仅有一笔订单，库存/收款只发生一次。
// 预检可能同时通过，唯一索引兜底保证落库唯一。
func TestCreateOrderConcurrentSameKeySingleOrder(t *testing.T) {
	setupAudit(t)
	productID := ensureProduct(t, "幂等测试商品C", 1000.00, 600, 100)
	key := fmt.Sprintf("qa-idem-conc-%d", time.Now().UnixNano())

	const workers = 4
	results := make([]*httptest.ResponseRecorder, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx] = postJSON("/api/orders", idemOrderBody(productID, key))
		}(i)
	}
	close(start)
	wg.Wait()

	for i, w := range results {
		if w.Code != http.StatusOK {
			t.Errorf("并发请求 %d 应返回 200（预检命中或唯一索引兜底），实际 %d %s", i, w.Code, w.Body.String())
		}
	}

	var orderCount int64
	if err := config.DB.Model(&models.Order{}).Where("client_request_id = ?", key).Count(&orderCount).Error; err != nil {
		t.Fatalf("按幂等键统计订单失败: %v", err)
	}
	if orderCount != 1 {
		t.Fatalf("并发同键应只落 1 笔订单，实际 %d 笔", orderCount)
	}
	// 库存只扣一次（2000 元 2 件）：初始 100 → 应为 98
	if got := idemProductStock(t, productID); got != 98 {
		t.Fatalf("并发同键库存应只扣一次(100→98)，实际 %d", got)
	}
	if got := accountBalance(t, "现金"); got != 2000 {
		t.Fatalf("并发同键应只收款一次 2000，实际 %.2f", got)
	}
	// 收款流水只写一笔：对最终订单号统计，金额合计应等于一次收款
	winnerNo := ""
	for _, w := range results {
		if no := parseOrderNo(t, w.Body.Bytes()); no != "" {
			winnerNo = no
			break
		}
	}
	if got := countFinancialLogs(t, winnerNo); got != 1 {
		t.Fatalf("并发同键应只写一笔收款流水，实际 %d 笔", got)
	}
}

// TestCreateOrderRejectsOversizedIdempotencyKey 超过 64 字符的幂等键
// 应被 binding max=64 校验拒绝，避免落库截断。
func TestCreateOrderRejectsOversizedIdempotencyKey(t *testing.T) {
	setupAudit(t)
	productID := ensureProduct(t, "幂等测试商品D", 1000.00, 600, 100)

	longKey := make([]byte, 65)
	for i := range longKey {
		longKey[i] = 'k'
	}
	w := postJSON("/api/orders", idemOrderBody(productID, string(longKey)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("超过 64 字符的幂等键应返回 400，实际 %d %s", w.Code, w.Body.String())
	}
}
