package middlewares

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type operationRoute struct {
	title  string
	tables []string
}

type rowChange struct {
	Table   string          `json:"table"`
	ID      uint            `json:"id"`
	Columns []string        `json:"columns,omitempty"`
	Before  json.RawMessage `json:"before,omitempty"`
	After   json.RawMessage `json:"after,omitempty"`
}

type tableCapture struct {
	ids        map[uint]struct{}
	relations  map[string]map[uint]struct{}
	strings    map[string]map[string]struct{}
	columns    []string
	includeAll bool
	includeNew bool
	maxID      uint
}

type operationCapture struct {
	tables map[string]*tableCapture
}

var operationHistoryMu sync.Mutex

// LockOperationHistory serializes a manual undo with request snapshots.
func LockOperationHistory() func() {
	operationHistoryMu.Lock()
	return operationHistoryMu.Unlock
}

var inventoryStocktakeOperationTables = []string{"products", "inventory_logs", "stocktake_records", "historical_cost_repairs"}

var operationRoutes = map[string]operationRoute{
	"POST /api/categories":                                   {title: "新增商品分类", tables: []string{"categories"}},
	"PUT /api/categories/:id":                                {title: "修改商品分类", tables: []string{"categories"}},
	"DELETE /api/categories/:id":                             {title: "删除商品分类", tables: []string{"categories"}},
	"POST /api/products":                                     {title: "新增商品", tables: []string{"products"}},
	"POST /api/products/recalculate-costs":                   {title: "重新核算商品成本", tables: []string{"products"}},
	"PUT /api/products/:id":                                  {title: "修改商品", tables: []string{"products", "historical_cost_repairs"}},
	"PUT /api/products/:id/transfer":                         {title: "商品库存调拨", tables: []string{"products", "inventory_logs", "stocktake_records", "pending_stocktakes"}},
	"POST /api/products/:id/stocktake":                       {title: "商品库存盘点", tables: inventoryStocktakeOperationTables},
	"POST /api/orders":                                       {title: "创建销售订单", tables: []string{"orders", "order_items", "products", "accounts", "inventory_logs", "financial_logs", "expenses", "ai_imports", "ai_agent_actions", "agent_work_tasks", "employee_handover_tasks", "employee_offboarding_alerts"}},
	"POST /api/ai/orders":                                    {title: "创建 AI 销售订单", tables: []string{"orders", "order_items", "products", "accounts", "inventory_logs", "financial_logs", "expenses", "ai_imports", "ai_agent_actions", "agent_work_tasks", "employee_handover_tasks", "employee_offboarding_alerts"}},
	"PUT /api/orders/:id/subsidy":                            {title: "修改订单国补状态", tables: []string{"orders", "order_items", "products", "accounts", "inventory_logs", "financial_logs", "expenses"}},
	"PUT /api/orders/:id/balance":                            {title: "收取订单尾款", tables: []string{"orders", "order_items", "products", "accounts", "inventory_logs", "financial_logs", "expenses"}},
	"PUT /api/orders/:id/cancel":                             {title: "取消销售订单", tables: []string{"orders", "order_items", "products", "accounts", "inventory_logs", "historical_cost_repairs", "financial_logs", "expenses", "employee_offboarding_alerts"}},
	"PUT /api/orders/:id/reversal-cancel":                    {title: "冲销取消销售订单", tables: []string{"orders", "order_items", "products", "accounts", "inventory_logs", "historical_cost_repairs", "financial_logs", "expenses", "employee_offboarding_alerts"}},
	"PUT /api/orders/:id/install":                            {title: "更新订单安装状态", tables: []string{"orders", "order_items", "products", "accounts", "inventory_logs", "financial_logs", "expenses", "employee_offboarding_alerts"}},
	"PUT /api/orders/:id/deliver":                            {title: "更新订单配送状态", tables: []string{"orders", "order_items", "products", "accounts", "inventory_logs", "financial_logs", "expenses", "employee_offboarding_alerts"}},
	"PUT /api/orders/:id/commission":                         {title: "修改订单提成", tables: []string{"orders", "order_items", "products", "accounts", "inventory_logs", "financial_logs", "expenses"}},
	"PUT /api/orders/:id/referrer":                           {title: "修改订单推荐人/渠道", tables: []string{"orders"}},
	"PUT /api/order-items/:item_id/deliver":                  {title: "更新商品配送状态", tables: []string{"orders", "order_items", "employees", "accounts", "financial_logs", "expenses", "employee_offboarding_alerts"}},
	"PUT /api/order-items/:item_id/install":                  {title: "更新商品安装状态", tables: []string{"orders", "order_items", "employees", "accounts", "financial_logs", "expenses", "employee_offboarding_alerts"}},
	"PUT /api/order-items/:item_id/delivery-fee":             {title: "修改商品配送费用", tables: []string{"orders", "order_items", "employees", "accounts", "financial_logs", "expenses"}},
	"PUT /api/order-items/:item_id/installation-fee":         {title: "修改商品安装费用", tables: []string{"orders", "order_items", "employees", "accounts", "financial_logs", "expenses"}},
	"POST /api/inbounds":                                     {title: "采购入库", tables: []string{"products", "inbound_records", "inventory_logs", "historical_cost_repairs"}},
	"POST /api/inbounds/cancel-by-log/:log_id":               {title: "撤销采购入库", tables: []string{"products", "inbound_records", "accounts", "financial_logs", "inventory_logs", "historical_cost_repairs", "expenses"}},
	"POST /api/inbounds/update-price/:log_id":                {title: "修改入库成本", tables: []string{"products", "inbound_records", "inventory_logs", "expenses"}},
	"POST /api/pending-inbounds/:id/approve":                 {title: "审核通过入库申请", tables: []string{"pending_inbounds", "products", "inbound_records", "inventory_logs", "historical_cost_repairs"}},
	"POST /api/pending-inbounds/:id/reject":                  {title: "驳回入库申请", tables: []string{"pending_inbounds"}},
	"POST /api/pending-stocktakes/:id/approve":               {title: "审核通过盘点申请", tables: []string{"pending_stocktakes", "stocktake_batch_items", "products", "stocktake_records", "accounts", "financial_logs", "inventory_logs", "historical_cost_repairs", "expenses"}},
	"POST /api/pending-stocktakes/:id/reject":                {title: "驳回盘点申请", tables: []string{"pending_stocktakes"}},
	"POST /api/stocktake-batches":                            {title: "创建盘点任务", tables: []string{"stocktake_batches", "stocktake_batch_items"}},
	"PUT /api/stocktake-batches/:id/items":                   {title: "更新盘点数量", tables: []string{"stocktake_batches", "stocktake_batch_items", "pending_stocktakes"}},
	"POST /api/stocktake-batches/:id/submit":                 {title: "提交盘点结果", tables: []string{"stocktake_batches", "stocktake_batch_items", "pending_stocktakes", "products", "stocktake_records", "accounts", "financial_logs", "inventory_logs", "historical_cost_repairs", "expenses"}},
	"POST /api/stocktake-batches/:id/items/:item_id/refresh": {title: "刷新盘点商品库存", tables: []string{"stocktake_batches", "stocktake_batch_items", "products"}},
	"POST /api/stocktake-batches/:id/print":                  {title: "标记盘点单已打印", tables: []string{"stocktake_batches"}},
	"POST /api/admin/stocktake-batches/:id/terminate":        {title: "终止盘点任务", tables: []string{"stocktake_batches", "stocktake_batch_items", "pending_stocktakes"}},
	"DELETE /api/admin/stocktake-batches/:id":                {title: "删除盘点任务", tables: []string{"stocktake_batches", "stocktake_batch_items", "pending_stocktakes"}},
	"POST /api/expenses":                                     {title: "新增支出记录", tables: []string{"expenses", "accounts", "financial_logs", "orders", "order_items", "employees", "employee_offboarding_alerts"}},
	"PUT /api/expenses/:id":                                  {title: "修改支出记录", tables: []string{"expenses", "accounts", "financial_logs", "orders", "order_items", "employees", "employee_offboarding_alerts"}},
	"POST /api/bookkeeping/entries":                          {title: "新增记账记录", tables: []string{"expenses", "accounts", "financial_logs", "orders", "order_items", "employees", "ai_imports", "employee_offboarding_alerts"}},
	"POST /api/ai/agent/actions/:id/confirm":                 {title: "确认 Agent 记账草稿", tables: []string{"ai_agent_actions", "agent_work_tasks", "expenses", "accounts", "financial_logs", "orders", "order_items", "employees", "employee_offboarding_alerts"}},
	"POST /api/accounts":                                     {title: "新增资金账户", tables: []string{"accounts"}},
	"POST /api/accounts/transfer":                            {title: "账户资金划拨", tables: []string{"accounts", "transfer_records", "financial_logs"}},
	"PUT /api/accounts/:id/calibrate":                        {title: "校准账户余额", tables: []string{"accounts", "financial_logs"}},
	"PUT /api/accounts/:id/set-default-expense":              {title: "设置默认支出账户", tables: []string{"accounts"}},
	"POST /api/employees":                                    {title: "新增员工", tables: []string{"employees"}},
	"PUT /api/employees/:id":                                 {title: "修改员工资料", tables: []string{"employees"}},
	"DELETE /api/employees/:id":                              {title: "删除员工", tables: []string{"employees"}},
	"POST /api/employees/attendance":                         {title: "更新员工考勤", tables: []string{"daily_attendances", "monthly_attendances"}},
	"POST /api/employees/check-in":                           {title: "员工签到", tables: []string{"daily_attendances"}},
	"POST /api/employees/check-out":                          {title: "员工签退", tables: []string{"daily_attendances"}},
	"POST /api/subsidy/settle":                               {title: "登记国补回款", tables: []string{"subsidy_settlements", "accounts", "financial_logs"}},
	"POST /api/subsidy/allocate":                             {title: "核销国补回款", tables: []string{"subsidy_receipts", "orders"}},
	"POST /api/system/historical-cost-repair":                {title: "修复历史成本", tables: []string{"products", "orders", "order_items", "inventory_logs", "historical_cost_repairs"}},
	"POST /api/employees/:id/offboardings":                   {title: "发起员工离职交接", tables: []string{"employees", "employee_offboardings", "employee_handover_tasks", "employee_offboarding_alerts"}},
	"PUT /api/employee-offboardings/:id/tasks/:task_id":      {title: "更新离职交接任务", tables: []string{"employee_handover_tasks"}},
	"POST /api/employee-offboardings/:id/settlement":         {title: "确认离职薪资结算", tables: []string{"employees", "employee_offboardings"}},
	"POST /api/employee-offboarding-alerts/:id/resolve":      {title: "处理离职后订单告警", tables: []string{"employee_offboarding_alerts"}},
}

// OperationHistoryMiddleware records reversible changes for core business writes.
func OperationHistoryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		route, tracked := operationRoutes[c.Request.Method+" "+c.FullPath()]
		if !tracked || config.DB == nil {
			c.Next()
			return
		}

		operationHistoryMu.Lock()
		defer operationHistoryMu.Unlock()

		body := readRequestBody(c)
		capture := buildOperationCapture(c, route.tables, body)
		before, err := captureRows(config.DB, capture, false)
		if err != nil {
			log.Printf("operation history snapshot before request failed: %v", err)
			c.Next()
			return
		}

		c.Next()

		after, err := captureRows(config.DB, capture, true)
		if err != nil {
			log.Printf("operation history snapshot after request failed: %v", err)
			return
		}
		changes := diffSnapshots(before, after, capture)
		if len(changes) == 0 {
			return
		}

		encoded, err := json.Marshal(changes)
		if err != nil {
			log.Printf("operation history encoding failed: %v", err)
			return
		}
		entry := models.OperationHistory{
			Username: c.GetString("username"),
			Title:    route.title,
			Method:   c.Request.Method,
			Path:     c.FullPath(),
			Changes:  string(encoded),
		}
		if err := config.DB.Create(&entry).Error; err != nil {
			log.Printf("operation history save failed: %v", err)
		}
	}
}

func readRequestBody(c *gin.Context) map[string]interface{} {
	if c.Request.Body == nil || !strings.Contains(c.GetHeader("Content-Type"), "application/json") {
		return map[string]interface{}{}
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("operation history request body read failed: %v", err)
		return map[string]interface{}{}
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if len(body) == 0 {
		return map[string]interface{}{}
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return map[string]interface{}{}
	}
	return parsed
}

func buildOperationCapture(c *gin.Context, tables []string, body map[string]interface{}) *operationCapture {
	capture := &operationCapture{tables: map[string]*tableCapture{}}
	for _, table := range tables {
		capture.addTable(table)
	}

	for _, param := range c.Params {
		id, err := strconv.ParseUint(param.Value, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		if table := pathParamTable(c.FullPath(), param.Key); table != "" {
			capture.addID(table, uint(id))
		}
	}

	for _, field := range []string{"product_id", "product_ids"} {
		capture.addIDs("products", collectBodyIDs(body, field)...)
	}
	for _, field := range []string{"order_id", "order_ids"} {
		capture.addIDs("orders", collectBodyIDs(body, field)...)
	}
	if _, ok := capture.tables["orders"]; ok {
		for _, field := range []string{"related_no", "order_no"} {
			capture.addRelatedStrings("orders", "order_no", collectBodyStrings(body, field)...)
		}
	}
	for _, field := range []string{"account_id", "account_ids", "from_account_id", "to_account_id"} {
		capture.addIDs("accounts", collectBodyIDs(body, field)...)
	}
	for _, field := range []string{"employee_id", "installer_id", "delivery_person_id", "operator_id", "submitter_employee_id"} {
		capture.addIDs("employees", collectBodyIDs(body, field)...)
		capture.addRelatedIDs("daily_attendances", "employee_id", collectBodyIDs(body, field)...)
		capture.addRelatedIDs("monthly_attendances", "employee_id", collectBodyIDs(body, field)...)
	}
	for _, field := range []string{"batch_id", "batch_ids"} {
		capture.addIDs("stocktake_batches", collectBodyIDs(body, field)...)
	}
	for _, field := range []string{"pending_inbound_id", "pending_inbound_ids"} {
		capture.addIDs("pending_inbounds", collectBodyIDs(body, field)...)
	}
	for _, field := range []string{"pending_stocktake_id", "pending_stocktake_ids"} {
		capture.addIDs("pending_stocktakes", collectBodyIDs(body, field)...)
	}
	for _, field := range []string{"category_id", "category_ids"} {
		capture.addIDs("categories", collectBodyIDs(body, field)...)
	}
	for _, field := range []string{"expense_id", "expense_ids"} {
		capture.addIDs("expenses", collectBodyIDs(body, field)...)
	}
	for _, field := range []string{"inventory_log_id", "inventory_log_ids"} {
		capture.addIDs("inventory_logs", collectBodyIDs(body, field)...)
	}
	for _, field := range []string{"log_id", "log_ids"} {
		capture.addIDs("inventory_logs", collectBodyIDs(body, field)...)
	}
	for _, field := range []string{"item_id", "item_ids"} {
		capture.addIDs("stocktake_batch_items", collectBodyIDs(body, field)...)
	}
	for _, field := range []string{"ai_import_id", "ai_import_ids"} {
		capture.addIDs("ai_imports", collectBodyIDs(body, field)...)
	}
	for _, field := range []string{"agent_action_id", "agent_action_ids"} {
		capture.addIDs("ai_agent_actions", collectBodyIDs(body, field)...)
	}
	if _, ok := capture.tables["ai_imports"]; ok {
		capture.setColumns("ai_imports", []string{"id", "status", "target_id", "updated_at"})
		capture.excludeNew("ai_imports")
	}
	if _, ok := capture.tables["ai_agent_actions"]; ok {
		capture.setColumns("ai_agent_actions", []string{"id", "status", "target_id", "updated_at"})
		capture.excludeNew("ai_agent_actions")
		var links []struct {
			TaskID *uint `gorm:"column:task_id"`
		}
		if err := config.DB.Table("ai_agent_messages").Select("task_id").
			Where("action_id IN ? AND task_id IS NOT NULL", capture.ids("ai_agent_actions")).Scan(&links).Error; err != nil {
			log.Printf("operation history linked task lookup failed: %v", err)
		}
		for _, link := range links {
			if link.TaskID != nil {
				capture.addID("agent_work_tasks", *link.TaskID)
			}
		}
	}
	if _, ok := capture.tables["agent_work_tasks"]; ok {
		capture.setColumns("agent_work_tasks", []string{"id", "status", "step", "updated_at"})
		capture.excludeNew("agent_work_tasks")
	}

	if _, ok := capture.tables["accounts"]; ok {
		capture.includeAll("accounts")
	}
	if _, ok := capture.tables["employees"]; ok && len(capture.tables["employees"].ids) == 0 {
		capture.includeAll("employees")
	}
	if _, ok := capture.tables["categories"]; ok && len(capture.tables["categories"].ids) == 0 {
		capture.includeAll("categories")
	}
	if c.FullPath() == "/api/products/recalculate-costs" {
		capture.includeAll("products")
	}

	for _, orderNo := range capture.stringValues("orders", "order_no") {
		orders := selectRowsByStrings(config.DB, "orders", "order_no", []string{orderNo})
		for _, row := range orders {
			capture.addID("orders", rowID(row))
		}
	}
	orderIDs := capture.ids("orders")
	itemIDs := capture.ids("order_items")
	if len(orderIDs) > 0 || len(itemIDs) > 0 {
		rows := selectRowsByIDs(config.DB, "order_items", "order_id", orderIDs, itemIDs)
		for _, row := range rows {
			capture.addID("order_items", rowID(row))
			capture.addID("products", rowUint(row, "product_id"))
			capture.addID("orders", rowUint(row, "order_id"))
		}
	}
	orderIDs = capture.ids("orders")
	if len(orderIDs) > 0 {
		orders := selectRowsByIDs(config.DB, "orders", "", nil, orderIDs)
		for _, row := range orders {
			orderNo, _ := row["order_no"].(string)
			capture.addRelatedStrings("financial_logs", "related_no", orderNo)
			capture.addRelatedStrings("inventory_logs", "related_no", orderNo)
			capture.addRelatedStrings("expenses", "related_no", orderNo)
		}
		if _, tracked := capture.tables["employee_offboarding_alerts"]; tracked {
			capture.addRelatedIDs("employee_offboarding_alerts", "order_id", orderIDs...)
		}
	}

	batchIDs := capture.ids("stocktake_batches")
	batchItemIDs := capture.ids("stocktake_batch_items")
	if len(batchIDs) > 0 || len(batchItemIDs) > 0 {
		rows := selectRowsByIDs(config.DB, "stocktake_batch_items", "batch_id", batchIDs, batchItemIDs)
		for _, row := range rows {
			capture.addID("stocktake_batch_items", rowID(row))
			capture.addID("products", rowUint(row, "product_id"))
			capture.addID("stocktake_batches", rowUint(row, "batch_id"))
			capture.addID("pending_stocktakes", rowUint(row, "pending_stocktake_id"))
		}
	}

	for _, table := range []string{"pending_inbounds", "pending_stocktakes", "expenses", "inventory_logs", "inbound_records"} {
		ids := capture.ids(table)
		if len(ids) == 0 {
			continue
		}
		rows := selectRowsByIDs(config.DB, table, "", nil, ids)
		for _, row := range rows {
			switch table {
			case "pending_inbounds", "pending_stocktakes":
				capture.addID("products", rowUint(row, "product_id"))
			case "expenses":
				capture.addID("accounts", rowUint(row, "account_id"))
			case "inventory_logs":
				capture.addID("products", rowUint(row, "product_id"))
				if inboundID := parsePositiveID(row["related_no"]); inboundID > 0 {
					capture.addID("inbound_records", inboundID)
				}
			case "inbound_records":
				capture.addID("products", rowUint(row, "product_id"))
				capture.addID("accounts", rowUint(row, "account_id"))
			}
		}
	}

	if len(capture.ids("pending_stocktakes")) > 0 {
		pendingRows := selectRowsByIDs(config.DB, "pending_stocktakes", "", nil, capture.ids("pending_stocktakes"))
		for _, row := range pendingRows {
			capture.addID("stocktake_batch_items", rowUint(row, "stocktake_batch_item_id"))
		}
	}
	if len(capture.ids("stocktake_batch_items")) > 0 {
		items := selectRowsByIDs(config.DB, "stocktake_batch_items", "", nil, capture.ids("stocktake_batch_items"))
		for _, row := range items {
			capture.addID("products", rowUint(row, "product_id"))
			capture.addID("stocktake_batches", rowUint(row, "batch_id"))
			capture.addID("pending_stocktakes", rowUint(row, "pending_stocktake_id"))
		}
	}
	if c.FullPath() == "/api/system/historical-cost-repair" {
		capture.addRelatedIDs("order_items", "product_id", capture.ids("products")...)
		capture.addRelatedIDs("inventory_logs", "product_id", capture.ids("products")...)
	}
	if c.FullPath() == "/api/products/:id/stocktake" {
		productIDs := capture.ids("products")
		for _, table := range []string{"inventory_logs", "stocktake_records", "historical_cost_repairs"} {
			capture.addRelatedIDs(table, "product_id", productIDs...)
			capture.excludeNew(table)
		}
	}

	if len(orderIDs) > 0 {
		capture.addRelatedIDs("order_items", "order_id", orderIDs...)
	}
	if len(batchIDs) > 0 {
		capture.addRelatedIDs("stocktake_batch_items", "batch_id", batchIDs...)
	}
	return capture
}

func pathParamTable(path, key string) string {
	switch key {
	case "id":
		switch {
		case strings.HasPrefix(path, "/api/products/"):
			return "products"
		case strings.HasPrefix(path, "/api/orders/"):
			return "orders"
		case strings.HasPrefix(path, "/api/categories/"):
			return "categories"
		case strings.HasPrefix(path, "/api/expenses/"):
			return "expenses"
		case strings.HasPrefix(path, "/api/employees/"):
			return "employees"
		case strings.HasPrefix(path, "/api/employee-offboardings/"):
			return "employee_offboardings"
		case strings.HasPrefix(path, "/api/employee-offboarding-alerts/"):
			return "employee_offboarding_alerts"
		case strings.HasPrefix(path, "/api/ai/agent/actions/"):
			return "ai_agent_actions"
		case strings.HasPrefix(path, "/api/accounts/"):
			return "accounts"
		case strings.HasPrefix(path, "/api/stocktake-batches/") || strings.HasPrefix(path, "/api/admin/stocktake-batches/"):
			return "stocktake_batches"
		case strings.HasPrefix(path, "/api/pending-inbounds/"):
			return "pending_inbounds"
		case strings.HasPrefix(path, "/api/pending-stocktakes/"):
			return "pending_stocktakes"
		}
	case "item_id":
		if strings.HasPrefix(path, "/api/order-items/") {
			return "order_items"
		}
		if strings.HasPrefix(path, "/api/stocktake-batches/") {
			return "stocktake_batch_items"
		}
	case "task_id":
		if strings.HasPrefix(path, "/api/employee-offboardings/") {
			return "employee_handover_tasks"
		}
	case "log_id":
		return "inventory_logs"
	}
	return ""
}

func (capture *operationCapture) addTable(table string) {
	if _, exists := capture.tables[table]; exists {
		return
	}
	capture.tables[table] = &tableCapture{
		ids:        map[uint]struct{}{},
		relations:  map[string]map[uint]struct{}{},
		strings:    map[string]map[string]struct{}{},
		includeNew: true,
	}
}

func (capture *operationCapture) addID(table string, id uint) {
	if id == 0 {
		return
	}
	capture.addTable(table)
	capture.tables[table].ids[id] = struct{}{}
}

func (capture *operationCapture) addIDs(table string, ids ...uint) {
	for _, id := range ids {
		capture.addID(table, id)
	}
}

func (capture *operationCapture) addRelatedIDs(table, column string, ids ...uint) {
	if len(ids) == 0 {
		return
	}
	capture.addTable(table)
	if capture.tables[table].relations[column] == nil {
		capture.tables[table].relations[column] = map[uint]struct{}{}
	}
	for _, id := range ids {
		if id > 0 {
			capture.tables[table].relations[column][id] = struct{}{}
		}
	}
}

func (capture *operationCapture) addRelatedStrings(table, column string, values ...string) {
	if len(values) == 0 {
		return
	}
	capture.addTable(table)
	if capture.tables[table].strings[column] == nil {
		capture.tables[table].strings[column] = map[string]struct{}{}
	}
	for _, value := range values {
		if value != "" {
			capture.tables[table].strings[column][value] = struct{}{}
		}
	}
}

func (capture *operationCapture) setColumns(table string, columns []string) {
	capture.addTable(table)
	capture.tables[table].columns = append([]string(nil), columns...)
}

func (capture *operationCapture) excludeNew(table string) {
	capture.addTable(table)
	capture.tables[table].includeNew = false
}

func (capture *operationCapture) includeAll(table string) {
	capture.addTable(table)
	capture.tables[table].includeAll = true
}

func (capture *operationCapture) ids(table string) []uint {
	ids := []uint{}
	if spec := capture.tables[table]; spec != nil {
		for id := range spec.ids {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (capture *operationCapture) stringValues(table, column string) []string {
	values := []string{}
	if spec := capture.tables[table]; spec != nil {
		for value := range spec.strings[column] {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values
}

func collectBodyStrings(value interface{}, target string) []string {
	values := map[string]struct{}{}
	var visit func(interface{})
	visit = func(current interface{}) {
		switch typed := current.(type) {
		case map[string]interface{}:
			for key, child := range typed {
				if key == target {
					if text, ok := child.(string); ok && strings.TrimSpace(text) != "" {
						values[strings.TrimSpace(text)] = struct{}{}
					}
					continue
				}
				visit(child)
			}
		case []interface{}:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	result := make([]string, 0, len(values))
	for item := range values {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func collectBodyIDs(value interface{}, target string) []uint {
	ids := map[uint]struct{}{}
	var visit func(interface{})
	visit = func(current interface{}) {
		switch typed := current.(type) {
		case map[string]interface{}:
			for key, child := range typed {
				if key == target {
					addJSONIDs(ids, child)
					continue
				}
				visit(child)
			}
		case []interface{}:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	result := make([]uint, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func addJSONIDs(ids map[uint]struct{}, value interface{}) {
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			if id := parsePositiveID(item); id > 0 {
				ids[id] = struct{}{}
			}
		}
	default:
		if id := parsePositiveID(value); id > 0 {
			ids[id] = struct{}{}
		}
	}
}

func parsePositiveID(value interface{}) uint {
	parsed, err := strconv.ParseUint(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
	if err != nil || parsed == 0 {
		return 0
	}
	return uint(parsed)
}

func selectRowsByIDs(db *gorm.DB, table, relationColumn string, relationIDs, rowIDs []uint) []map[string]interface{} {
	query := db.Table(table)
	conditions := false
	if len(rowIDs) > 0 {
		query = query.Where("id IN ?", rowIDs)
		conditions = true
	}
	if len(relationIDs) > 0 && relationColumn != "" {
		if conditions {
			query = query.Or(relationColumn+" IN ?", relationIDs)
		} else {
			query = query.Where(relationColumn+" IN ?", relationIDs)
		}
	}
	if !conditions && (len(relationIDs) == 0 || relationColumn == "") {
		return []map[string]interface{}{}
	}
	rows := []map[string]interface{}{}
	if err := query.Find(&rows).Error; err != nil {
		log.Printf("operation history relationship lookup failed for %s: %v", table, err)
		return []map[string]interface{}{}
	}
	return normalizeRows(rows)
}

func selectRowsByStrings(db *gorm.DB, table, column string, values []string) []map[string]interface{} {
	if len(values) == 0 {
		return []map[string]interface{}{}
	}
	rows := []map[string]interface{}{}
	if err := db.Table(table).Where(column+" IN ?", values).Find(&rows).Error; err != nil {
		log.Printf("operation history relationship lookup failed for %s: %v", table, err)
		return []map[string]interface{}{}
	}
	return normalizeRows(rows)
}

func captureRows(db *gorm.DB, capture *operationCapture, includeNew bool) (map[string]map[uint]json.RawMessage, error) {
	snapshot := map[string]map[uint]json.RawMessage{}
	tables := make([]string, 0, len(capture.tables))
	for table := range capture.tables {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		spec := capture.tables[table]
		if !includeNew {
			if err := db.Table(table).Select("COALESCE(MAX(id), 0)").Scan(&spec.maxID).Error; err != nil {
				return nil, fmt.Errorf("get %s high-water mark: %w", table, err)
			}
		}
		query := db.Table(table)
		if len(spec.columns) > 0 {
			query = query.Select(spec.columns)
		}
		hasCondition := spec.includeAll
		if !spec.includeAll {
			if len(spec.ids) > 0 {
				ids := mapKeys(spec.ids)
				query = query.Where("id IN ?", ids)
				hasCondition = true
			}
			for column, values := range spec.relations {
				ids := mapKeys(values)
				if len(ids) == 0 {
					continue
				}
				if hasCondition {
					query = query.Or(column+" IN ?", ids)
				} else {
					query = query.Where(column+" IN ?", ids)
					hasCondition = true
				}
			}
			for column, values := range spec.strings {
				items := make([]string, 0, len(values))
				for value := range values {
					items = append(items, value)
				}
				if len(items) == 0 {
					continue
				}
				if hasCondition {
					query = query.Or(column+" IN ?", items)
				} else {
					query = query.Where(column+" IN ?", items)
					hasCondition = true
				}
			}
			if includeNew && spec.includeNew {
				if hasCondition {
					query = query.Or("id > ?", spec.maxID)
				} else {
					query = query.Where("id > ?", spec.maxID)
					hasCondition = true
				}
			}
		}
		if !hasCondition {
			snapshot[table] = map[uint]json.RawMessage{}
			continue
		}
		rows := []map[string]interface{}{}
		if err := query.Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("read %s rows: %w", table, err)
		}
		snapshot[table] = map[uint]json.RawMessage{}
		for _, row := range normalizeRows(rows) {
			id := rowID(row)
			if id == 0 {
				continue
			}
			encoded, err := json.Marshal(row)
			if err != nil {
				return nil, fmt.Errorf("encode %s row %d: %w", table, id, err)
			}
			snapshot[table][id] = encoded
		}
	}
	return snapshot, nil
}

func diffSnapshots(before, after map[string]map[uint]json.RawMessage, capture *operationCapture) []rowChange {
	changes := []rowChange{}
	tables := make([]string, 0, len(before))
	for table := range before {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		ids := map[uint]struct{}{}
		for id := range before[table] {
			ids[id] = struct{}{}
		}
		for id := range after[table] {
			ids[id] = struct{}{}
		}
		orderedIDs := make([]uint, 0, len(ids))
		for id := range ids {
			orderedIDs = append(orderedIDs, id)
		}
		sort.Slice(orderedIDs, func(i, j int) bool { return orderedIDs[i] < orderedIDs[j] })
		for _, id := range orderedIDs {
			previous, hadBefore := before[table][id]
			current, hasAfter := after[table][id]
			if hadBefore && hasAfter && bytes.Equal(canonicalJSON(previous), canonicalJSON(current)) {
				continue
			}
			change := rowChange{Table: table, ID: id}
			if spec := capture.tables[table]; spec != nil && len(spec.columns) > 0 {
				change.Columns = append([]string(nil), spec.columns...)
			}
			if hadBefore {
				change.Before = previous
			}
			if hasAfter {
				change.After = current
			}
			changes = append(changes, change)
		}
	}
	return changes
}

func canonicalJSON(raw json.RawMessage) []byte {
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return encoded
}

func normalizeRows(rows []map[string]interface{}) []map[string]interface{} {
	for _, row := range rows {
		for key, value := range row {
			if bytesValue, ok := value.([]byte); ok {
				row[key] = string(bytesValue)
			}
		}
	}
	return rows
}

func rowID(row map[string]interface{}) uint {
	return rowUint(row, "id")
}

func rowUint(row map[string]interface{}, key string) uint {
	return parsePositiveID(row[key])
}

func mapKeys(values map[uint]struct{}) []uint {
	keys := make([]uint, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
