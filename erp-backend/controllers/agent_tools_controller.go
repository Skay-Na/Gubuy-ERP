package controllers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/services"

	"github.com/gin-gonic/gin"
)

// 新版 Agent（pi 核心 agent-service）的服务端数据工具。
// 鉴权沿用 AdminAuthRequired：agent-service 透传管理员的 JWT 调用这些接口。

func agentToolLimit(c *gin.Context, def int) int {
	raw := c.Query("limit")
	if raw == "" {
		return def
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		return def
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func agentToolDate(c *gin.Context, key string, def time.Time) (time.Time, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return def, true
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return def, false
	}
	return parsed, true
}

func agentToolData(c *gin.Context, summary string, items any) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"summary": summary, "items": items}})
}

// AgentToolProductInventory 按关键词查询可售库存。
// @Summary Agent 工具：商品库存
// @Tags AI
// @Produce json
// @Param keyword query string true "商品关键词"
// @Param limit query int false "条数上限"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID agent_tool_product_inventory
// @Router /api/ai/agent-tools/product-inventory [get]
func AgentToolProductInventory(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		c.JSON(400, gin.H{"code": 400, "msg": "请提供商品关键词"})
		return
	}
	items, err := (services.RetailReadService{DB: config.DB}).SearchProductInventory(c.Request.Context(), keyword, agentToolLimit(c, 20))
	if err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	agentToolData(c, "读取 "+strconv.Itoa(len(items))+" 个商品的当前可售库存", items)
}

// AgentToolSalesSummary 统计时间范围内有效订单销量。
// @Summary Agent 工具：销量汇总
// @Tags AI
// @Produce json
// @Param start query string true "开始日期 YYYY-MM-DD"
// @Param end query string true "结束日期 YYYY-MM-DD（含当天）"
// @Param limit query int false "条数上限"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID agent_tool_sales_summary
// @Router /api/ai/agent-tools/sales-summary [get]
func AgentToolSalesSummary(c *gin.Context) {
	now := time.Now()
	start, ok1 := agentToolDate(c, "start", now.AddDate(0, 0, -30))
	end, ok2 := agentToolDate(c, "end", now)
	if !ok1 || !ok2 {
		c.JSON(400, gin.H{"code": 400, "msg": "日期格式应为 YYYY-MM-DD"})
		return
	}
	items, err := (services.RetailReadService{DB: config.DB}).GetProductSalesSummary(c.Request.Context(), start, end, agentToolLimit(c, 30))
	if err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	agentToolData(c, "读取 "+strconv.Itoa(len(items))+" 个商品在所选期间的有效订单销量", items)
}

// AgentToolSalesTrend 对比相邻等长期间销量变化。
// @Summary Agent 工具：销量趋势
// @Tags AI
// @Produce json
// @Param start query string true "开始日期 YYYY-MM-DD"
// @Param end query string true "结束日期 YYYY-MM-DD（排他）"
// @Param limit query int false "条数上限"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID agent_tool_sales_trend
// @Router /api/ai/agent-tools/sales-trend [get]
func AgentToolSalesTrend(c *gin.Context) {
	now := time.Now()
	start, ok1 := agentToolDate(c, "start", now.AddDate(0, 0, -14))
	end, ok2 := agentToolDate(c, "end", now)
	if !ok1 || !ok2 {
		c.JSON(400, gin.H{"code": 400, "msg": "日期格式应为 YYYY-MM-DD"})
		return
	}
	items, err := (services.RetailReadService{DB: config.DB}).GetProductSalesTrend(c.Request.Context(), start, end, agentToolLimit(c, 30))
	if err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	agentToolData(c, "对比 "+strconv.Itoa(len(items))+" 个商品的相邻等长期间销量变化；下降不证明原因", items)
}

// AgentToolInventorySales 合并库存与销量。
// @Summary Agent 工具：库存与销量组合
// @Tags AI
// @Produce json
// @Param start query string true "开始日期 YYYY-MM-DD"
// @Param end query string true "结束日期 YYYY-MM-DD（含当天）"
// @Param limit query int false "条数上限"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID agent_tool_inventory_sales
// @Router /api/ai/agent-tools/inventory-sales [get]
func AgentToolInventorySales(c *gin.Context) {
	now := time.Now()
	start, ok1 := agentToolDate(c, "start", now.AddDate(0, 0, -30))
	end, ok2 := agentToolDate(c, "end", now)
	if !ok1 || !ok2 {
		c.JSON(400, gin.H{"code": 400, "msg": "日期格式应为 YYYY-MM-DD"})
		return
	}
	items, err := (services.RetailReadService{DB: config.DB}).GetInventorySalesSummary(c.Request.Context(), start, end, agentToolLimit(c, 100))
	if err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	agentToolData(c, "读取 "+strconv.Itoa(len(items))+" 个商品的当前可售库存与所选期间销量", items)
}

// AgentToolUnpaidOrders 查询未结清有效订单。
// @Summary Agent 工具：未结清订单
// @Tags AI
// @Produce json
// @Param customer query string false "客户姓名关键词"
// @Param limit query int false "条数上限"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID agent_tool_unpaid_orders
// @Router /api/ai/agent-tools/unpaid-orders [get]
func AgentToolUnpaidOrders(c *gin.Context) {
	items := agentUnpaidOrderRows(strings.TrimSpace(c.Query("customer")), agentToolLimit(c, 20))
	agentToolData(c, "读取 "+strconv.Itoa(len(items))+" 张未结清有效订单", items)
}

type agentUnpaidOrderRow struct {
	OrderNo      string  `json:"order_no"`
	CustomerName string  `json:"customer_name"`
	Outstanding  float64 `json:"outstanding"`
	CreatedAt    string  `json:"created_at"`
}

func agentUnpaidOrderRows(customer string, limit int) []agentUnpaidOrderRow {
	type orderRow struct {
		OrderNo         string    `json:"order_no"`
		CustomerName    string    `json:"customer_name"`
		ActualPayAmount float64   `json:"actual_pay_amount"`
		DepositAmount   float64   `json:"deposit_amount"`
		CreatedAt       time.Time `json:"created_at"`
	}
	query := config.DB.Table("orders").
		Select("order_no, customer_name, actual_pay_amount, deposit_amount, created_at").
		Where("order_status = 1 AND payment_status = 1 AND actual_pay_amount > deposit_amount")
	if customer != "" {
		query = query.Where("customer_name LIKE ?", "%"+customer+"%")
	}
	var rows []orderRow
	if err := query.Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return []agentUnpaidOrderRow{}
	}
	items := make([]agentUnpaidOrderRow, 0, len(rows))
	for _, row := range rows {
		items = append(items, agentUnpaidOrderRow{
			OrderNo:      row.OrderNo,
			CustomerName: row.CustomerName,
			Outstanding:  row.ActualPayAmount - row.DepositAmount,
			CreatedAt:    row.CreatedAt.Format("2006-01-02"),
		})
	}
	return items
}
