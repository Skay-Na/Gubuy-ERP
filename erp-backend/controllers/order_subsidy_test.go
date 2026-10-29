package controllers

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

// subsidyRouteOnce 保证 subsidy 路由只注册一次（auditRouter 为包级共享路由器）。
var subsidyRouteOnce sync.Once

// setupSubsidyTest 复用审计测试基建（ERP_AUDIT_DB 测试库 + resetAuditDB），
// 并在共享路由器上注册国补状态接口。
func setupSubsidyTest(t *testing.T) {
	t.Helper()
	setupAudit(t)
	subsidyRouteOnce.Do(func() {
		auditRouter.PUT("/api/orders/:id/subsidy", UpdateSubsidyStatus)
	})
}

// createSubsidyOrder 下一个国补订单（公户收款），返回订单号。
func createSubsidyOrder(t *testing.T, customer string) string {
	t.Helper()
	productID := ensureProduct(t, "国补状态机测试商品", 10000.00, 8000, 100)

	body := gin.H{
		"items": []gin.H{
			{"product_id": productID, "quantity": 1, "unit_price": 10000.00},
		},
		"payment_method":   3,
		"delivery_method":  2,
		"is_subsidy":       true,
		"customer_name":    customer,
		"customer_phone":   "13600000000",
		"customer_address": "国补测试地址",
	}
	w := postJSON("/api/orders", body)
	if w.Code != http.StatusOK {
		t.Fatalf("创建国补订单失败: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		OrderNo string `json:"order_no"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.OrderNo == "" {
		t.Fatalf("下单未返回订单号: %s", w.Body.String())
	}
	return resp.OrderNo
}

// TestSubsidyTransitionRules 纯逻辑校验状态机规则：
// 允许 0→1、0→2、1→2 单向前进；禁止回退、禁止重复设置、禁止写入 3（已回款）。
func TestSubsidyTransitionRules(t *testing.T) {
	cases := []struct {
		name      string
		current   int
		target    int
		wantErr   bool
		errSubstr string
	}{
		{name: "无国补到待提交", current: 0, target: 1, wantErr: false},
		{name: "无国补直接已提交", current: 0, target: 2, wantErr: false},
		{name: "待提交到已提交", current: 1, target: 2, wantErr: false},
		{name: "回退到待提交", current: 2, target: 1, wantErr: true, errSubstr: "回退"},
		{name: "重复设置同状态", current: 1, target: 1, wantErr: true, errSubstr: "回退"},
		{name: "已回款禁止接口写入", current: 1, target: 3, wantErr: true, errSubstr: "核销"},
		{name: "已回款后禁止再改", current: 3, target: 2, wantErr: true, errSubstr: "回退"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSubsidyTransition(tc.current, tc.target)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("当前 %d → 目标 %d 应拒绝，实际通过", tc.current, tc.target)
				}
				if !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("错误信息 %q 应包含 %q", err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("当前 %d → 目标 %d 应允许，实际报错: %v", tc.current, tc.target, err)
			}
		})
	}
}

// TestSubsidyStatusForwardTransition 正向跃迁：新建国补订单为 1（待提交资料），
// 提交资料后应变为 2（已提交资料）。
func TestSubsidyStatusForwardTransition(t *testing.T) {
	setupSubsidyTest(t)
	orderNo := createSubsidyOrder(t, "国补正向客户")

	order := readOrder(t, orderNo)
	if order.SubsidyStatus != 1 {
		t.Fatalf("新建国补订单状态应为 1，实际 %d", order.SubsidyStatus)
	}

	w := putJSON("/api/orders/"+itoa(order.ID)+"/subsidy", gin.H{"target_status": 2})
	if w.Code != http.StatusOK {
		t.Fatalf("提交资料应成功: %d %s", w.Code, w.Body.String())
	}
	if got := readOrder(t, orderNo).SubsidyStatus; got != 2 {
		t.Fatalf("提交资料后状态应为 2，实际 %d", got)
	}
}

// TestSubsidyStatusRejectsBackwardTransition 回退禁止：状态 2 时请求改回 1，
// 应返回 400 且状态保持 2 不变。
func TestSubsidyStatusRejectsBackwardTransition(t *testing.T) {
	setupSubsidyTest(t)
	orderNo := createSubsidyOrder(t, "国补回退客户")

	order := readOrder(t, orderNo)
	if w := putJSON("/api/orders/"+itoa(order.ID)+"/subsidy", gin.H{"target_status": 2}); w.Code != http.StatusOK {
		t.Fatalf("前置：提交资料应成功: %d %s", w.Code, w.Body.String())
	}

	w := putJSON("/api/orders/"+itoa(order.ID)+"/subsidy", gin.H{"target_status": 1})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("回退请求应返回 400，实际 %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp.Msg, "回退") {
		t.Fatalf("错误信息应说明不允许回退，实际: %s", resp.Msg)
	}
	if got := readOrder(t, orderNo).SubsidyStatus; got != 2 {
		t.Fatalf("回退被拒绝后状态应保持 2，实际 %d", got)
	}
}

// TestSubsidyStatusRejectsPaidTarget 已回款（3）只能由核销流程写入：
// 即使用于绑定校验的接口直接收到 3 也必须拒绝且状态不变。
func TestSubsidyStatusRejectsPaidTarget(t *testing.T) {
	setupSubsidyTest(t)
	orderNo := createSubsidyOrder(t, "国补回款客户")

	order := readOrder(t, orderNo)
	w := putJSON("/api/orders/"+itoa(order.ID)+"/subsidy", gin.H{"target_status": 3})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("target_status=3 应返回 400，实际 %d %s", w.Code, w.Body.String())
	}
	if got := readOrder(t, orderNo).SubsidyStatus; got != 1 {
		t.Fatalf("拒绝写入已回款后状态应保持 1，实际 %d", got)
	}
}
