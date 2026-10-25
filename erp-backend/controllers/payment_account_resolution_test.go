package controllers

// 账户解析专项回归（对应 commit 7a010c5）：
// 1. 支付方式 1/2/3/5 必须按规范账户名解析（支付宝/微信/公户/现金），
//    现金方式编号=5 而现金账户 id=4，历史上存在"编号当 ID 回退"的错位假账风险；
// 2. 账户名查不到时必须返回明确错误，绝不允许回退到 First(id=编号)；
// 3. CreateOrder 与 PayOrderBalance 走同一解析函数 resolvePaymentAccount。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
)

// rebuildAccountsWithDecoy 清空账户表后按显式 ID 重建四个种子账户，
// 并额外插入一个"陷阱账户"（id=5，名称不是现金）：
// 若任何代码路径仍按"支付方式编号当账户主键"查询，方法 5 会错误命中陷阱账户。
func rebuildAccountsWithDecoy(t *testing.T) {
	t.Helper()
	if err := config.DB.Exec("DELETE FROM accounts").Error; err != nil {
		t.Fatalf("清空账户表失败: %v", err)
	}
	seed := []struct {
		id   uint
		name string
	}{
		{1, "支付宝"},
		{2, "微信"},
		{3, "公户"},
		{4, "现金"},
		{5, "陷阱账户-编号错位防护"},
	}
	for _, s := range seed {
		if err := config.DB.Exec("INSERT INTO accounts (id, name, balance, created_at, updated_at) VALUES (?, ?, 0, NOW(), NOW())", s.id, s.name).Error; err != nil {
			t.Fatalf("重建账户 %s(id=%d) 失败: %v", s.name, s.id, err)
		}
	}
}

// TestResolvePaymentAccountPerMethod 正常路径：方式 1/2/3/5 各解析到规范账户，
// 现金(方式 5)必须命中 id=4 的"现金"账户而不是 id=5 的陷阱账户。
func TestResolvePaymentAccountPerMethod(t *testing.T) {
	setupAudit(t)
	rebuildAccountsWithDecoy(t)

	cases := []struct {
		method   int
		wantName string
		wantID   uint
	}{
		{method: 1, wantName: "支付宝", wantID: 1},
		{method: 2, wantName: "微信", wantID: 2},
		{method: 3, wantName: "公户", wantID: 3},
		{method: 5, wantName: "现金", wantID: 4}, // 关键：方式编号 5 ≠ 账户 id 5
	}
	for _, tc := range cases {
		t.Run(tc.wantName, func(t *testing.T) {
			acc, err := resolvePaymentAccount(config.DB, tc.method, nil)
			if err != nil {
				t.Fatalf("方式 %d 解析账户失败: %v", tc.method, err)
			}
			if acc.Name != tc.wantName {
				t.Fatalf("方式 %d 应解析到账户「%s」，实际「%s」", tc.method, tc.wantName, acc.Name)
			}
			if acc.ID != tc.wantID {
				t.Fatalf("方式 %d 应解析到账户 id=%d，实际 id=%d（疑似编号当ID回退）", tc.method, tc.wantID, acc.ID)
			}
		})
	}

	// 不支持的编号（如 4）必须明确报错，而不是命中任何账户
	if _, err := resolvePaymentAccount(config.DB, 4, nil); err == nil {
		t.Fatal("支付方式编号 4 未配置，应返回错误而不是回退查询")
	} else if want := "不支持的支付方式编号 4"; !strings.Contains(err.Error(), want) {
		t.Fatalf("错误信息应包含 %q，实际: %v", want, err)
	}
}

// TestLookupPaymentAccountNoIDFallback 错误路径：账户表里只有与规范名无关的
// "陷阱账户"（id 与方式编号一一对应），任何方式都必须报"未找到"，
// 证明"查不到名称就 First(id=编号)"的回退分支确已删除。
func TestLookupPaymentAccountNoIDFallback(t *testing.T) {
	setupAudit(t)
	if err := config.DB.Exec("DELETE FROM accounts").Error; err != nil {
		t.Fatalf("清空账户表失败: %v", err)
	}
	for id := 1; id <= 5; id++ {
		name := fmt.Sprintf("陷阱账户%d号", id)
		if err := config.DB.Exec("INSERT INTO accounts (id, name, balance, created_at, updated_at) VALUES (?, ?, 0, NOW(), NOW())", id, name).Error; err != nil {
			t.Fatalf("插入陷阱账户 id=%d 失败: %v", id, err)
		}
	}

	for _, method := range []int{1, 2, 3, 5} {
		acc, err := lookupPaymentAccount(config.DB, method, nil, false)
		if err == nil {
			t.Fatalf("方式 %d：规范账户不存在时应报错，实际命中了账户「%s」(id=%d)，编号当ID回退未删除", method, acc.Name, acc.ID)
		}
		if !strings.Contains(err.Error(), "未找到") && !strings.Contains(err.Error(), "不支持的支付方式编号") {
			t.Fatalf("方式 %d 错误信息应明确指出账户未找到，实际: %v", method, err)
		}
	}

	// 显式 accountID 查不到时也必须明确报错
	bogusID := uint(9999)
	if _, err := lookupPaymentAccount(config.DB, 1, &bogusID, false); err == nil {
		t.Fatal("不存在的账户 ID 应报错")
	}
}

// TestCreateOrderCashMethodHitsCashAccount 集成：开单用现金(方式 5)收款，
// 钱必须进"现金"(id=4)账户，陷阱账户(id=5)余额必须保持 0。
func TestCreateOrderCashMethodHitsCashAccount(t *testing.T) {
	setupAudit(t)
	rebuildAccountsWithDecoy(t)
	productID := ensureProduct(t, "账户解析测试商品-现金", 1000.00, 600, 100)

	body := gin.H{
		"items": []gin.H{
			{"product_id": productID, "quantity": 2, "unit_price": 1000.00},
		},
		"payment_method":  5, // 现金
		"delivery_method": 2,
		"customer_name":   "现金收款客户",
		"customer_phone":  "13500000001",
	}
	w := postJSON("/api/orders", body)
	if w.Code != http.StatusOK {
		t.Fatalf("现金开单失败: %d %s", w.Code, w.Body.String())
	}

	if got := accountBalance(t, "现金"); got != 2000 {
		t.Fatalf("现金收款应进入「现金」账户 2000，实际 %.2f", got)
	}
	var decoy models.Account
	if err := config.DB.Where("id = ?", 5).First(&decoy).Error; err != nil {
		t.Fatalf("查询陷阱账户失败: %v", err)
	}
	if decoy.Balance != 0 {
		t.Fatalf("陷阱账户(id=5)不应收款，实际余额 %.2f —— 编号当ID回退假账", decoy.Balance)
	}
}

// TestPayOrderBalanceUsesUnifiedResolution 集成：结清尾款同样走统一解析。
// 尾款用现金(5)支付必须进「现金」账户；非法编号 4/99 必须被 oneof 校验拒绝。
func TestPayOrderBalanceUsesUnifiedResolution(t *testing.T) {
	setupAudit(t)
	rebuildAccountsWithDecoy(t)
	productID := ensureProduct(t, "账户解析测试商品-尾款", 1000.00, 600, 100)
	setAccountBalance(t, "支付宝", 5000)

	body := gin.H{
		"items": []gin.H{
			{"product_id": productID, "quantity": 1, "unit_price": 1000.00},
		},
		"payment_method":  1,
		"delivery_method": 2,
		"deposit_amount":  300.00,
		"customer_name":   "尾款现金客户",
		"customer_phone":  "13500000002",
	}
	w := postJSON("/api/orders", body)
	if w.Code != http.StatusOK {
		t.Fatalf("定金下单失败: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		OrderNo string `json:"order_no"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	order := readOrder(t, resp.OrderNo)

	// 非法编号：4（未配置）与 99（超范围）都应被 oneof=1 2 3 5 拒绝
	for _, bad := range []int{4, 99} {
		wb := putJSON(fmt.Sprintf("/api/orders/%d/balance", order.ID), gin.H{"final_payment_method": bad})
		if wb.Code != http.StatusBadRequest {
			t.Fatalf("final_payment_method=%d 应返回 400，实际 %d %s", bad, wb.Code, wb.Body.String())
		}
		if got := accountBalance(t, "现金"); got != 0 {
			t.Fatalf("非法编号被拒后现金账户不应有变动，实际 %.2f", got)
		}
	}

	// 合法编号 5：全款 1000 应进入「现金」(id=4)，陷阱账户(id=5)保持 0
	w2 := putJSON(fmt.Sprintf("/api/orders/%d/balance", order.ID), gin.H{"final_payment_method": 5})
	if w2.Code != http.StatusOK {
		t.Fatalf("现金结清尾款失败: %d %s", w2.Code, w2.Body.String())
	}
	if got := accountBalance(t, "现金"); got != 1000 {
		t.Fatalf("尾款现金应进入「现金」账户 1000，实际 %.2f", got)
	}
	var decoy models.Account
	if err := config.DB.Where("id = ?", 5).First(&decoy).Error; err != nil {
		t.Fatalf("查询陷阱账户失败: %v", err)
	}
	if decoy.Balance != 0 {
		t.Fatalf("尾款收款命中陷阱账户(id=5)，余额 %.2f —— 编号当ID回退假账", decoy.Balance)
	}
}
