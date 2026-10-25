package controllers

// 国补状态机补充矩阵用例（对应 commit e26a53d 的补充验证）：
// order_subsidy_test.go 已覆盖 0→1/0→2/1→2 放行、2→1 回退、1→1 重复、1→3 已回款。
// 本文件补齐矩阵剩余角：任意状态→3 一律拒绝（含 0→3、2→3）、
// 2→2 重复、0→0 重置，以及接口层的 1→1 重复提交与 0→2 直达放行。

import (
	"net/http"
	"strings"
	"testing"

	"erp-backend/config"

	"github.com/gin-gonic/gin"
)

// TestSubsidyTransitionMatrixAnyToPaidRejected 纯逻辑：任何当前状态请求
// 写入 3（已回款）都必须拒绝，且错误信息指向国补核销流程。
func TestSubsidyTransitionMatrixAnyToPaidRejected(t *testing.T) {
	for _, current := range []int{0, 1, 2, 3} {
		err := validateSubsidyTransition(current, 3)
		if err == nil {
			t.Fatalf("当前 %d → 3 应一律拒绝（已回款只能由核销流程写入）", current)
		}
		if !strings.Contains(err.Error(), "核销") {
			t.Fatalf("当前 %d → 3 的错误信息应指向核销流程，实际: %v", current, err)
		}
	}
}

// TestSubsidyTransitionMatrixNoRepeatOrReset 纯逻辑：禁止重复设置（2→2）
// 与重置为无国补（0→0）。
func TestSubsidyTransitionMatrixNoRepeatOrReset(t *testing.T) {
	cases := []struct {
		name    string
		current int
		target  int
	}{
		{"已提交重复设置", 2, 2},
		{"重置为无国补", 0, 0},
		{"已回款重复设置", 3, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSubsidyTransition(tc.current, tc.target)
			if err == nil {
				t.Fatalf("当前 %d → %d 应拒绝（回退/重复/重置均不允许）", tc.current, tc.target)
			}
		})
	}
}

// TestSubsidyDuplicateForwardRejectedApi 接口层：国补订单创建后为 1（待提交），
// 再次提交 target_status=1 属重复设置，应 400 且状态保持 1。
func TestSubsidyDuplicateForwardRejectedApi(t *testing.T) {
	setupSubsidyTest(t)
	orderNo := createSubsidyOrder(t, "国补重复提交客户")
	order := readOrder(t, orderNo)

	w := putJSON("/api/orders/"+itoa(order.ID)+"/subsidy", gin.H{"target_status": 1})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("重复设置同状态应返回 400，实际 %d %s", w.Code, w.Body.String())
	}
	if got := readOrder(t, orderNo).SubsidyStatus; got != 1 {
		t.Fatalf("重复设置被拒后状态应保持 1，实际 %d", got)
	}
}

// TestSubsidyFromZeroDirectToSubmittedApi 接口层：0（无国补）→ 2（已提交）
// 属允许的单向跃迁（业务上存在无资料直接已提交的场景），应放行。
func TestSubsidyFromZeroDirectToSubmittedApi(t *testing.T) {
	setupSubsidyTest(t)
	orderNo := createSubsidyOrder(t, "国补零状态直达客户")
	order := readOrder(t, orderNo)

	// 手工把状态归零，模拟"无资料直接已提交"的起点
	if err := config.DB.Model(order).Update("subsidy_status", 0).Error; err != nil {
		t.Fatalf("重置国补状态失败: %v", err)
	}

	w := putJSON("/api/orders/"+itoa(order.ID)+"/subsidy", gin.H{"target_status": 2})
	if w.Code != http.StatusOK {
		t.Fatalf("0→2 应放行，实际 %d %s", w.Code, w.Body.String())
	}
	if got := readOrder(t, orderNo).SubsidyStatus; got != 2 {
		t.Fatalf("0→2 放行后状态应为 2，实际 %d", got)
	}
}

// TestSubsidySubmittedToPaidRejectedApi 接口层：已提交(2)后请求 3（已回款）
// 必须被拒绝且状态保持 2。
func TestSubsidySubmittedToPaidRejectedApi(t *testing.T) {
	setupSubsidyTest(t)
	orderNo := createSubsidyOrder(t, "国补已提交回款客户")
	order := readOrder(t, orderNo)

	if w := putJSON("/api/orders/"+itoa(order.ID)+"/subsidy", gin.H{"target_status": 2}); w.Code != http.StatusOK {
		t.Fatalf("前置：提交资料应成功: %d %s", w.Code, w.Body.String())
	}

	w := putJSON("/api/orders/"+itoa(order.ID)+"/subsidy", gin.H{"target_status": 3})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("2→3 应返回 400（已回款只能由核销流程写入），实际 %d %s", w.Code, w.Body.String())
	}
	if got := readOrder(t, orderNo).SubsidyStatus; got != 2 {
		t.Fatalf("拒绝写入已回款后状态应保持 2，实际 %d", got)
	}
}
