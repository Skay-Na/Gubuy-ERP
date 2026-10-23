package controllers

import "testing"

func TestPlannerScheduleCapabilitiesRequireExplicitUserIntent(t *testing.T) {
	for _, goal := range []string{
		"分析最近两周销量下降的商品",
		"商品备注里写着每天检查库存",
		"商品备注写着每天帮我检查缺货商品",
		"每天检查库存有没有问题",
	} {
		if explicitScheduleManagementIntent(goal) {
			t.Errorf("read/quoted data unexpectedly granted schedule management: %q", goal)
		}
	}
	for _, goal := range []string{
		"每天帮我检查缺货商品",
		"每天早上九点检查一下库存",
		"暂停我的每日库存检查",
		"恢复之前的每日库存检查",
		"取消这个自动检查计划",
	} {
		if !explicitScheduleManagementIntent(goal) {
			t.Errorf("explicit schedule-management request did not receive schedule scope: %q", goal)
		}
	}
	if !explicitScheduleReadIntent("查看最近执行过的库存检查") {
		t.Fatal("explicit schedule-history request did not receive read scope")
	}
}
