package controllers

import (
	"errors"
	"math"
	"strings"
	"time"
)

// agentToolSpec makes the legacy chat tools explicit. These are all scoped to
// an authenticated administrator by the HTTP route; model output never grants
// a new permission. Draft tools write only to Agent tables.
type agentToolSpec struct {
	Name               string
	Description        string
	InputSchema        string
	OutputSchema       string
	RequiredPermission string
	RiskLevel          string
	SideEffects        bool
	Timeout            time.Duration
	Idempotency        string
	Audit              bool
}

var agentToolRegistry = map[string]agentToolSpec{
	"finish": {
		Name: "finish", Description: "返回有证据的自然语言答复",
		InputSchema:  `{"type":"object","required":["tool","answer"]}`,
		OutputSchema: `{"type":"string"}`, RequiredPermission: "admin",
		RiskLevel: "LOW", Timeout: 5 * time.Second, Audit: true,
	},
	"data_catalog": {
		Name: "data_catalog", Description: "查看允许查询的数据目录",
		InputSchema:  `{"type":"object","required":["tool"]}`,
		OutputSchema: `{"type":"string"}`, RequiredPermission: "admin",
		RiskLevel: "LOW", Timeout: 5 * time.Second, Audit: true,
	},
	"data_query": {
		Name: "data_query", Description: "受限业务数据查询",
		InputSchema:  `{"type":"object","required":["tool","data"]}`,
		OutputSchema: `{"type":"string"}`, RequiredPermission: "admin",
		RiskLevel: "LOW", Timeout: 8 * time.Second, Audit: true,
	},
	"customer_orders": {
		Name: "customer_orders", Description: "查询客户有效订单",
		InputSchema:  `{"type":"object","required":["tool","customer"]}`,
		OutputSchema: `{"type":"string"}`, RequiredPermission: "admin",
		RiskLevel: "LOW", Timeout: 8 * time.Second, Audit: true,
	},
	"profit": {
		Name: "profit", Description: "读取现有利润口径",
		InputSchema:  `{"type":"object","required":["tool","period"]}`,
		OutputSchema: `{"type":"string"}`, RequiredPermission: "admin",
		RiskLevel: "LOW", Timeout: 8 * time.Second, Audit: true,
	},
	"inventory": {
		Name: "inventory", Description: "查询指定商品库存",
		InputSchema:  `{"type":"object","required":["tool","product"]}`,
		OutputSchema: `{"type":"string"}`, RequiredPermission: "admin",
		RiskLevel: "LOW", Timeout: 8 * time.Second, Audit: true,
	},
	"unpaid_orders": {
		Name: "unpaid_orders", Description: "查询未结清订单",
		InputSchema:  `{"type":"object","required":["tool"]}`,
		OutputSchema: `{"type":"string"}`, RequiredPermission: "admin",
		RiskLevel: "LOW", Timeout: 8 * time.Second, Audit: true,
	},
	"employee_commission": {
		Name: "employee_commission", Description: "查询员工提成",
		InputSchema:  `{"type":"object","required":["tool"]}`,
		OutputSchema: `{"type":"string"}`, RequiredPermission: "admin",
		RiskLevel: "LOW", Timeout: 8 * time.Second, Audit: true,
	},
	"employee_profile": {
		Name: "employee_profile", Description: "查询员工资料",
		InputSchema:  `{"type":"object","required":["tool","employee"]}`,
		OutputSchema: `{"type":"string"}`, RequiredPermission: "admin",
		RiskLevel: "LOW", Timeout: 8 * time.Second, Audit: true,
	},
	"employee_orders": {
		Name: "employee_orders", Description: "查询员工订单",
		InputSchema:  `{"type":"object","required":["tool","employee"]}`,
		OutputSchema: `{"type":"string"}`, RequiredPermission: "admin",
		RiskLevel: "LOW", Timeout: 8 * time.Second, Audit: true,
	},
	"system_help": {
		Name: "system_help", Description: "返回系统操作指引",
		InputSchema:  `{"type":"object","required":["tool","category"]}`,
		OutputSchema: `{"type":"string"}`, RequiredPermission: "admin",
		RiskLevel: "LOW", Timeout: 5 * time.Second, Audit: true,
	},
	"draft_expense": {
		Name: "draft_expense", Description: "生成待人工确认的支出草稿",
		InputSchema:  `{"type":"object","required":["tool","amount","category"]}`,
		OutputSchema: `{"type":"object","required":["action_id"]}`, RequiredPermission: "admin",
		RiskLevel: "LOW", SideEffects: true, Timeout: 8 * time.Second,
		Idempotency: "尚未提供请求幂等键；不得自动重试", Audit: true,
	},
	"draft_order": {
		Name: "draft_order", Description: "生成待人工核对的订单草稿",
		InputSchema:  `{"type":"object","required":["tool","order"]}`,
		OutputSchema: `{"type":"object","required":["action_id"]}`, RequiredPermission: "admin",
		RiskLevel: "LOW", SideEffects: true, Timeout: 8 * time.Second,
		Idempotency: "尚未提供请求幂等键；不得自动重试", Audit: true,
	},
}

func validateAgentToolPlan(plan agentPlan) error {
	if _, ok := agentToolRegistry[plan.Tool]; !ok {
		return errors.New("未注册的 Agent 工具")
	}
	if len([]rune(plan.Product)) > 200 || len([]rune(plan.Customer)) > 200 ||
		len([]rune(plan.Employee)) > 200 || len([]rune(plan.Category)) > 200 {
		return errors.New("Agent 工具参数过长")
	}
	if math.IsNaN(plan.Amount) || math.IsInf(plan.Amount, 0) || plan.Amount > 1e9 {
		return errors.New("Agent 金额参数无效")
	}
	if plan.Tool != "draft_order" {
		return nil
	}
	if len(plan.Order.Items) > 12 || len([]rune(plan.Order.CustomerName)) > 100 {
		return errors.New("订单草稿参数超出限制")
	}
	for _, item := range plan.Order.Items {
		if strings.TrimSpace(item.Name) == "" || len([]rune(item.Name)) > 255 ||
			item.Quantity <= 0 || item.Quantity > 1000 ||
			math.IsNaN(item.UnitPrice) || math.IsInf(item.UnitPrice, 0) ||
			item.UnitPrice < 0 || item.UnitPrice > 1e9 {
			return errors.New("订单草稿商品参数无效")
		}
	}
	return nil
}
