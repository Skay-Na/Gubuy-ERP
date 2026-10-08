package controllers

import (
	"strings"
	"testing"
	"testing/fstest"

	"erp-backend/models"
)

func TestAgentManualRegistryLoadsCurrentModules(t *testing.T) {
	manuals, err := registeredAgentManuals()
	if err != nil {
		t.Fatal(err)
	}
	if len(manuals) < 9 {
		t.Fatalf("registered manuals: %d", len(manuals))
	}
	index, err := agentManualIndex()
	if err != nil || !strings.Contains(index, "subsidy（国补回款）") {
		t.Fatalf("manual index: %s, %v", index, err)
	}
	text, err := agentManualForTask(agentTask{Topic: "inventory"}, "现在库存怎么样")
	if err != nil || !strings.Contains(text, "不含样机") || strings.Contains(text, "工资与提成") {
		t.Fatalf("inventory manual selection: %s, %v", text, err)
	}
	text, err = agentManualForTask(agentTask{Topic: "general"}, "国补回款与订单销售对比")
	if err != nil || !strings.Contains(text, "国补回款") || !strings.Contains(text, "销售与订单") {
		t.Fatalf("multi-module selection: %s, %v", text, err)
	}
}

func TestAgentManualNewModuleRequiresOnlyManifestForDiscovery(t *testing.T) {
	files := fstest.MapFS{
		"agent_manuals/returns.json": &fstest.MapFile{Data: []byte(`{
			"id":"returns","title":"退货","summary":"退货登记查询",
			"keywords":["退货"],"datasets":["orders"],
			"rules":["退货单与原订单分别核对"],"limits":["登记不等于已退款"]}`)},
	}
	manuals, err := loadAgentManuals(files)
	if err != nil || len(manuals) != 1 || manuals[0].ID != "returns" {
		t.Fatalf("new manual registration: %+v, %v", manuals, err)
	}
}

func TestAgentManualRejectsUnregisteredDataset(t *testing.T) {
	files := fstest.MapFS{
		"agent_manuals/unsafe.json": &fstest.MapFile{Data: []byte(`{
			"id":"unsafe","title":"错误示例","summary":"敏感记录",
			"datasets":["ai_provider_configs"],"rules":["查询密钥"]}`)},
	}
	if _, err := loadAgentManuals(files); err == nil || !strings.Contains(err.Error(), "未开放的数据集") {
		t.Fatalf("unsafe manual accepted: %v", err)
	}
}

func TestAgentManualIsLoadedBeforePlanning(t *testing.T) {
	script := &agentMockScript{
		Task: func(string) agentTask {
			return agentTask{Goal: "看采购入库", Topic: "inbound", Operation: "lookup", Metrics: []string{"采购入库情况"}, Scope: "all"}
		},
		Plans: []agentPlan{{Tool: "finish"}},
	}
	ctx, _ := agentTestContext()
	runAgentTask(ctx, agentMockRuntime(t, script), models.AIAgentSession{}, agentSendInput{Message: "采购入库怎么样"}, nil)
	if len(script.Systems) < 2 || !strings.Contains(script.Systems[0], "已注册业务模块") {
		t.Fatalf("task prompt missing manual index: %+v", script.Systems)
	}
	if !strings.Contains(script.Systems[1], "待确认申请不能直接算作现有库存") ||
		strings.Contains(script.Systems[1], "员工工资与提成") {
		t.Fatalf("planner did not load the matching manual: %s", script.Systems[1])
	}
}
