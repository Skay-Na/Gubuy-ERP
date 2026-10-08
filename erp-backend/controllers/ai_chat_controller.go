package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
)

type aiChatInput struct {
	Message string `json:"message" binding:"required"`
	Path    string `json:"path"`
}

type aiChatIntent struct {
	Action   string `json:"action"`
	Customer string `json:"customer"`
	Period   string `json:"period"`
	Path     string `json:"path"`
	Answer   string `json:"answer"`
}

var aiChatPages = map[string]string{
	"/admin/dashboard": "经营看板", "/admin/pos": "收银开单", "/admin/products": "商品管理",
	"/admin/products/inbound": "采购入库", "/admin/products/inventory-logs": "库存流水",
	"/admin/orders": "订单管理", "/admin/bookkeeping": "记账", "/admin/ai-import": "拍照导入订单或支出",
	"/admin/finance": "财务报表", "/admin/subsidy": "国补管理", "/admin/employees": "员工管理",
	"/admin/settings": "系统设置及 AI 服务商密钥",
}

// ChatAI answers usage questions and executes only the allowlisted read-only data queries below.
// @Summary 与系统 AI 助手对话
// @Tags AI
// @Accept json
// @Produce json
// @Param input body aiChatInput true "问题与当前页面"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID post_api_ai_chat
// @Router /api/ai/chat [post]
func ChatAI(c *gin.Context) {
	var input aiChatInput
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Message) == "" || len([]rune(strings.TrimSpace(input.Message))) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请输入不超过 500 字的问题"})
		return
	}
	runtime, err := activeAIProvider()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": err.Error()})
		return
	}
	pageList, _ := json.Marshal(aiChatPages)
	system := "你是 ERP 系统助手。仅返回 JSON 对象，字段 action、customer、period、path、answer。" +
		"action 只能是 customer_orders、profit、help。问某客户买了什么选 customer_orders，customer 填客户姓名；问利润、销售额、订单数选 profit；其余选 help。" +
		"period 只能为 last_30_days、this_month、last_month、today，默认 last_30_days。" +
		"help 的 answer 用简洁中文回答用户如何操作，path 必须从页面路径列表中选择或为空。不要声称已经执行了创建、修改、删除。" +
		"客户数据和利润数据你无法直接访问，只能选择 action；不要编造数字。页面路径列表：" + string(pageList)
	intentText, err := callAIChat(c.Request.Context(), runtime, system, "当前页面："+input.Path+"\n用户问题："+input.Message, true)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "AI 服务暂时不可用：" + err.Error()})
		return
	}
	var intent aiChatIntent
	if err := json.Unmarshal([]byte(cleanAIJSON(intentText)), &intent); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "AI 未返回可用的答复，请重试"})
		return
	}
	answer := strings.TrimSpace(intent.Answer)
	path := ""
	switch intent.Action {
	case "customer_orders":
		answer, err = answerCustomerOrders(strings.TrimSpace(intent.Customer))
		path = "/admin/orders"
	case "profit":
		answer, err = answerProfit(intent.Period)
		path = "/admin/finance"
	default:
		if _, ok := aiChatPages[intent.Path]; ok {
			path = intent.Path
		}
		if answer == "" {
			answer = "可以问我系统怎么操作、某位客户买了什么，或近 30 天的利润。"
		}
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "系统数据查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"answer": answer, "path": path, "path_label": aiChatPages[path]}})
}

func cleanAIJSON(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimSpace(strings.TrimPrefix(content, "```json"))
		content = strings.TrimSpace(strings.TrimPrefix(content, "```JSON"))
		content = strings.TrimSpace(strings.TrimPrefix(content, "```"))
		content = strings.TrimSpace(strings.TrimSuffix(content, "```"))
	}
	return content
}

var errAIEmptyContent = errors.New("模型未返回内容")
var errAITruncated = errors.New("模型输出达到预算上限，回答未完成")

func callAIChat(ctx context.Context, runtime aiProviderRuntime, system, user string, jsonMode bool) (content string, callErr error) {
	started := time.Now()
	trace := agentTraceFrom(ctx)
	call := agentCallTrace{Model: runtime.Model}
	defer func() {
		if trace != nil {
			call.Milliseconds = time.Since(started).Milliseconds()
			if callErr != nil {
				call.Error = callErr.Error()
			}
			trace.Calls = append(trace.Calls, call)
		}
	}()
	payload := gin.H{"model": runtime.Model, "messages": []gin.H{{"role": "system", "content": system}, {"role": "user", "content": user}}}
	if jsonMode && runtime.JSONMode {
		payload["response_format"] = gin.H{"type": "json_object"}
		payload["max_tokens"] = 4096
	}
	if runtime.Provider == "qwen" {
		payload["enable_thinking"] = false
	}
	timeout := 60 * time.Second
	if trace != nil {
		payload["max_tokens"] = trace.Profile.Tokens
		timeout = trace.Profile.Timeout
		if runtime.Provider == "deepseek" {
			payload["thinking"] = gin.H{"type": "enabled"}
			payload["reasoning_effort"] = trace.Profile.Effort
		}
	}
	body, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, runtime.URL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+runtime.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: timeout}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("模型服务返回 %d", response.StatusCode)
	}
	var result struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			Details          struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return "", err
	}
	call.PromptTokens = result.Usage.PromptTokens
	call.CompletionTokens = result.Usage.CompletionTokens
	call.ReasoningTokens = result.Usage.Details.ReasoningTokens
	if len(result.Choices) > 0 {
		call.Finish = result.Choices[0].FinishReason
		if call.Finish == "length" {
			return "", errAITruncated
		}
	}
	if len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return "", errAIEmptyContent
	}
	return result.Choices[0].Message.Content, nil
}

func answerCustomerOrders(customer string) (string, error) {
	if customer == "" || len([]rune(customer)) > 80 {
		return "请告诉我客户姓名，例如“张三买过什么”。", nil
	}
	var orders []models.Order
	pattern := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(customer) + "%"
	err := config.DB.Preload("OrderItems.Product").Where("customer_name LIKE ? AND order_status = 1", pattern).Order("created_at DESC").Limit(20).Find(&orders).Error
	if err != nil {
		return "", err
	}
	if len(orders) == 0 {
		return fmt.Sprintf("没有找到客户“%s”的有效订单。", customer), nil
	}
	var lines []string
	for _, order := range orders {
		items := make([]string, 0, len(order.OrderItems))
		for _, item := range order.OrderItems {
			name := "商品 #" + fmt.Sprint(item.ProductID)
			if item.Product != nil {
				name = item.Product.Name
			}
			items = append(items, fmt.Sprintf("%s × %d", name, item.Quantity))
		}
		lines = append(lines, fmt.Sprintf("订单 %s · %s · %s · %s · ¥%.2f", order.OrderNo, order.CreatedAt.Format("2006-01-02"), order.CustomerName, strings.Join(items, "、"), order.TotalAmount-order.DiscountAmount))
	}
	return fmt.Sprintf("找到最近 %d 笔有效订单（最多显示 20 笔）：\n%s", len(orders), strings.Join(lines, "\n")), nil
}

func answerProfit(period string) (string, error) {
	now := time.Now()
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	start, end, label := today.AddDate(0, 0, -29), now, "近 30 天"
	switch period {
	case "today":
		start, end, label = today, now, "今天"
	case "this_month":
		start, end, label = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc), now, "本月截至目前"
	case "last_month":
		end = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		start, label = end.AddDate(0, -1, 0), "上月"
	}
	where := "order_status = 1 AND created_at >= ? AND created_at < ?"
	var revenue, cost, operating float64
	var count int64
	var fees struct{ DeliveryFee, InstallationFee, ReferralFee, CommissionAmount, TradeInAmount float64 }
	if err := config.DB.Model(&models.Order{}).Where(where, start, end).
		Select("COALESCE(SUM(total_amount - discount_amount), 0)").Scan(&revenue).Error; err != nil {
		return "", err
	}
	if err := config.DB.Model(&models.Order{}).Where(where, start, end).Count(&count).Error; err != nil {
		return "", err
	}
	if err := config.DB.Table("order_items").Joins("JOIN orders ON order_items.order_id = orders.id").
		Where("orders.order_status = 1 AND orders.deleted_at IS NULL AND orders.created_at >= ? AND orders.created_at < ?", start, end).
		Select("COALESCE(SUM(order_items.unit_cost * order_items.quantity), 0)").Scan(&cost).Error; err != nil {
		return "", err
	}
	if err := config.DB.Model(&models.Order{}).Where(where, start, end).
		Select("COALESCE(SUM(delivery_fee), 0) AS delivery_fee, COALESCE(SUM(installation_fee), 0) AS installation_fee, COALESCE(SUM(referral_fee), 0) AS referral_fee, COALESCE(SUM(commission_amount), 0) AS commission_amount, COALESCE(SUM(trade_in_amount), 0) AS trade_in_amount").Scan(&fees).Error; err != nil {
		return "", err
	}
	if err := config.DB.Model(&models.Expense{}).Where("expense_type NOT IN ? AND created_at >= ? AND created_at < ?", []string{"采购货款", "以旧换新"}, start, end).
		Select("COALESCE(SUM(amount), 0)").Scan(&operating).Error; err != nil {
		return "", err
	}
	gross := CalculateGrossProfit(revenue, cost, fees.DeliveryFee, fees.InstallationFee, fees.ReferralFee, fees.CommissionAmount, fees.TradeInAmount)
	return fmt.Sprintf("%s（%s 至 %s）：\n营业额 ¥%.2f，订单 %d 笔。\n销售毛利 ¥%.2f，经营费用 ¥%.2f，经营净利润 ¥%.2f。\n口径与财务总览一致：仅统计有效订单，净利润为毛利减经营费用。", label, start.Format("2006-01-02"), end.Format("2006-01-02"), revenue, count, gross, operating, CalculateNetProfit(gross, operating)), nil
}
