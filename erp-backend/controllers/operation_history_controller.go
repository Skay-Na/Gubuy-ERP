package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/middlewares"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type operationHistoryItem struct {
	ID        uint       `json:"id"`
	Username  string     `json:"username"`
	Title     string     `json:"title"`
	Detail    string     `json:"detail"`
	Method    string     `json:"method"`
	CreatedAt time.Time  `json:"created_at"`
	UndoneAt  *time.Time `json:"undone_at,omitempty"`
	UndoneBy  string     `json:"undone_by,omitempty"`
	CanUndo   bool       `json:"can_undo"`
}

// GetOperationHistory 获取操作历史
// @Summary 获取操作历史
// @Description 获取操作历史
// @Tags Operations
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_operations
// @Router /api/operations [get]
// GetOperationHistory lists recent tracked business operations for administrators.
func GetOperationHistory(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limit < 1 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}

	var records []models.OperationHistory
	if err := config.DB.Order("id desc").Limit(limit).Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "历史操作加载失败"})
		return
	}
	employeesByID, employeesByNumber := operationHistoryEmployees(records)

	items := make([]operationHistoryItem, 0, len(records))
	for _, record := range records {
		items = append(items, operationHistoryItem{
			ID:        record.ID,
			Username:  record.Username,
			Title:     record.Title,
			Detail:    describeOperationHistory(record, employeesByID, employeesByNumber),
			Method:    record.Method,
			CreatedAt: record.CreatedAt,
			UndoneAt:  record.UndoneAt,
			UndoneBy:  record.UndoneBy,
			CanUndo:   record.UndoneAt == nil,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

func operationHistoryEmployees(records []models.OperationHistory) (map[uint]string, map[string]string) {
	employeeIDs := map[uint]struct{}{}
	employeeNumbers := map[string]struct{}{}
	for _, record := range records {
		if strings.HasPrefix(record.Username, "emp_") {
			employeeNumbers[strings.TrimPrefix(record.Username, "emp_")] = struct{}{}
		}
		for _, change := range decodeOperationHistoryChanges(record.Changes) {
			for _, raw := range []json.RawMessage{change.Before, change.After} {
				row := decodeOperationHistoryRow(raw)
				for _, key := range []string{"employee_id", "assignee_employee_id", "successor_employee_id"} {
					if id := operationRowUint(row, key); id > 0 {
						employeeIDs[id] = struct{}{}
					}
				}
			}
		}
	}

	byID := map[uint]string{}
	byNumber := map[string]string{}
	if len(employeeIDs) == 0 && len(employeeNumbers) == 0 {
		return byID, byNumber
	}

	var employees []models.Employee
	query := config.DB.Unscoped().Model(&models.Employee{}).Select("id", "name", "emp_no")
	switch {
	case len(employeeIDs) > 0 && len(employeeNumbers) > 0:
		query = query.Where("id IN ? OR emp_no IN ?", operationSortedUintKeys(employeeIDs), operationSortedStringKeys(employeeNumbers))
	case len(employeeIDs) > 0:
		query = query.Where("id IN ?", operationSortedUintKeys(employeeIDs))
	default:
		query = query.Where("emp_no IN ?", operationSortedStringKeys(employeeNumbers))
	}
	if err := query.Find(&employees).Error; err != nil {
		return byID, byNumber
	}
	for _, employee := range employees {
		if employee.Name == "" {
			continue
		}
		byID[employee.ID] = employee.Name
		byNumber[employee.EmpNo] = employee.Name
	}
	return byID, byNumber
}

func operationSortedUintKeys(values map[uint]struct{}) []uint {
	result := make([]uint, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func operationSortedStringKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func decodeOperationHistoryChanges(raw string) []operationRowChange {
	changes := []operationRowChange{}
	if err := json.Unmarshal([]byte(raw), &changes); err != nil {
		return changes
	}
	return changes
}

func decodeOperationHistoryRow(raw json.RawMessage) map[string]interface{} {
	row := map[string]interface{}{}
	if len(raw) == 0 || string(raw) == "null" {
		return row
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return map[string]interface{}{}
	}
	return row
}

func describeOperationHistory(record models.OperationHistory, employeesByID map[uint]string, employeesByNumber map[string]string) string {
	changes := decodeOperationHistoryChanges(record.Changes)
	actor := operationHistoryActor(record.Username, employeesByNumber)

	if record.Title == "员工签到" || record.Title == "员工签退" {
		if detail := describeAttendanceMark(record, changes, actor, employeesByID); detail != "" {
			return detail
		}
	}

	details := describeOperationChanges(record.Title, changes, employeesByID)
	if details == "" {
		details = record.Title + "已完成，相关业务数据已更新"
	}
	return fmt.Sprintf("%s于%s：%s", actor, operationHistoryTime(record.CreatedAt), details)
}

func operationHistoryActor(username string, employeesByNumber map[string]string) string {
	if strings.HasPrefix(username, "emp_") {
		employeeNumber := strings.TrimPrefix(username, "emp_")
		if name := employeesByNumber[employeeNumber]; name != "" {
			return "员工" + name
		}
		if employeeNumber != "" {
			return "工号 " + employeeNumber
		}
	}
	if strings.TrimSpace(username) == "" {
		return "系统"
	}
	return username
}

func operationHistoryTime(value time.Time) string {
	if value.IsZero() {
		return "时间未知"
	}
	return value.Local().Format("2006年01月02日 15:04")
}

func describeAttendanceMark(record models.OperationHistory, changes []operationRowChange, actor string, employeesByID map[uint]string) string {
	for _, change := range changes {
		if change.Table != "daily_attendances" {
			continue
		}
		row := operationChangeRow(change)
		person := operationEmployeeName(operationRowUint(row, "employee_id"), employeesByID, actor)
		date := operationDateLabel(operationRowString(row, "date"))
		verb, field := "上班签到", "check_in"
		if record.Title == "员工签退" {
			verb, field = "下班签退", "check_out"
		}
		at := operationClockLabel(row[field])
		if at == "" {
			at = operationHistoryTime(record.CreatedAt)
		} else {
			at = date + " " + at
		}
		return fmt.Sprintf("%s于%s完成%s", person, at, verb)
	}
	return ""
}

func operationChangeRow(change operationRowChange) map[string]interface{} {
	if len(change.After) > 0 {
		return decodeOperationHistoryRow(change.After)
	}
	return decodeOperationHistoryRow(change.Before)
}

func operationEmployeeName(id uint, employeesByID map[uint]string, fallback string) string {
	if name := employeesByID[id]; name != "" {
		return "员工" + name
	}
	if fallback != "" {
		return fallback
	}
	return "员工"
}

func describeOperationChanges(title string, changes []operationRowChange, employeesByID map[uint]string) string {
	products := operationProductNames(changes)
	orderedChanges := append([]operationRowChange{}, changes...)
	sort.SliceStable(orderedChanges, func(i, j int) bool {
		return operationDetailTablePriority(title, orderedChanges[i].Table) < operationDetailTablePriority(title, orderedChanges[j].Table)
	})
	parts := []string{}
	for _, change := range orderedChanges {
		row := operationChangeRow(change)
		before := decodeOperationHistoryRow(change.Before)
		switch change.Table {
		case "orders":
			if text := describeOrderChange(before, row); text != "" {
				parts = append(parts, text)
			}
		case "order_items":
			if title == "创建销售订单" || title == "创建 AI 销售订单" {
				if text := describeOrderItem(row, products); text != "" {
					parts = append(parts, text)
				}
			} else if text := describeOrderItemChange(before, row, products); text != "" {
				parts = append(parts, text)
			}
		case "products":
			if text := describeProductChange(before, row); text != "" {
				parts = append(parts, text)
			}
		case "categories":
			if text := describeNamedChange("分类", before, row, "name"); text != "" {
				parts = append(parts, text)
			}
		case "daily_attendances":
			if title == "更新员工考勤" {
				if text := describeDailyAttendance(before, row, employeesByID); text != "" {
					parts = append(parts, text)
				}
			}
		case "monthly_attendances":
			if text := describeMonthlyAttendance(before, row, employeesByID); text != "" {
				parts = append(parts, text)
			}
		case "inbound_records":
			if text := describeInboundRecord(before, row, products); text != "" {
				parts = append(parts, text)
			}
		case "inventory_logs":
			if text := describeInventoryLog(row, products); text != "" {
				parts = append(parts, text)
			}
		case "stocktake_records":
			if text := describeStocktakeRecord(row, products); text != "" {
				parts = append(parts, text)
			}
		case "stocktake_batch_items":
			if text := describeStocktakeBatchItem(row); text != "" {
				parts = append(parts, text)
			}
		case "expenses":
			if text := describeExpense(before, row); text != "" {
				parts = append(parts, text)
			}
		case "financial_logs":
			if text := describeFinancialLog(row); text != "" {
				parts = append(parts, text)
			}
		case "accounts":
			if text := describeAccountChange(before, row); text != "" {
				parts = append(parts, text)
			}
		case "transfer_records":
			if text := describeTransferRecord(row); text != "" {
				parts = append(parts, text)
			}
		case "employees":
			if text := describeEmployeeChange(before, row); text != "" {
				parts = append(parts, text)
			}
		case "employee_offboardings":
			if text := describeEmployeeOffboarding(before, row); text != "" {
				parts = append(parts, text)
			}
		case "employee_handover_tasks":
			if text := describeEmployeeHandoverTask(before, row, employeesByID); text != "" {
				parts = append(parts, text)
			}
		case "employee_offboarding_alerts":
			if text := describeEmployeeOffboardingAlert(before, row); text != "" {
				parts = append(parts, text)
			}
		case "pending_inbounds":
			if text := describePendingInbound(row, products); text != "" {
				parts = append(parts, text)
			}
		case "pending_stocktakes":
			if text := describePendingStocktake(row, products); text != "" {
				parts = append(parts, text)
			}
		case "subsidy_settlements":
			if amount := operationRowNumber(row, "amount"); amount != 0 {
				parts = append(parts, "登记国补回款 "+operationCurrency(amount))
			}
		case "historical_cost_repairs":
			if text := describeHistoricalCostRepair(row, products); text != "" {
				parts = append(parts, text)
			}
		case "stocktake_batches":
			if text := describeStocktakeBatch(row); text != "" {
				parts = append(parts, text)
			}
		}
	}

	parts = uniqueOperationDetails(parts)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) > 3 {
		return strings.Join(parts[:3], "；") + fmt.Sprintf("；另有 %d 项相关变更", len(parts)-3)
	}
	return strings.Join(parts, "；")
}

func operationDetailTablePriority(title, table string) int {
	switch {
	case strings.Contains(title, "订单") || strings.Contains(title, "配送") || strings.Contains(title, "安装"):
		switch table {
		case "orders":
			return 0
		case "order_items":
			return 1
		case "products":
			return 2
		case "expenses", "financial_logs", "accounts":
			return 3
		}
	case strings.Contains(title, "入库"):
		switch table {
		case "inbound_records", "pending_inbounds":
			return 0
		case "products":
			return 1
		case "inventory_logs", "historical_cost_repairs":
			return 2
		}
	case strings.Contains(title, "盘点") || strings.Contains(title, "调拨"):
		switch table {
		case "stocktake_records", "stocktake_batch_items", "pending_stocktakes", "transfer_records":
			return 0
		case "products":
			return 1
		case "inventory_logs", "historical_cost_repairs":
			return 2
		}
	case strings.Contains(title, "支出") || strings.Contains(title, "记账") || strings.Contains(title, "账户"):
		switch table {
		case "expenses", "transfer_records", "financial_logs":
			return 0
		case "accounts":
			return 1
		}
	case strings.Contains(title, "员工") || strings.Contains(title, "考勤"):
		switch table {
		case "employees", "employee_offboardings", "daily_attendances", "monthly_attendances":
			return 0
		case "employee_handover_tasks":
			return 1
		case "employee_offboarding_alerts":
			return 2
		}
	}
	return 10
}

func operationProductNames(changes []operationRowChange) map[uint]string {
	products := map[uint]string{}
	for _, change := range changes {
		if change.Table != "products" && change.Table != "stocktake_batch_items" {
			continue
		}
		row := operationChangeRow(change)
		id := operationRowUint(row, "id")
		name := operationRowString(row, "name")
		if name == "" {
			name = operationRowString(row, "product_name")
			id = operationRowUint(row, "product_id")
		}
		if id > 0 && name != "" {
			products[id] = name
		}
	}
	return products
}

func describeOrderChange(before, after map[string]interface{}) string {
	row := after
	if len(row) == 0 {
		row = before
	}
	parts := []string{}
	if orderNo := operationRowString(row, "order_no"); orderNo != "" {
		parts = append(parts, "订单 "+orderNo)
	}
	if customer := operationRowString(row, "customer_name"); customer != "" {
		parts = append(parts, "客户 "+customer)
	}
	if len(before) == 0 {
		if amount, ok := row["actual_pay_amount"]; ok {
			parts = append(parts, "客户应付 "+operationCurrency(operationNumber(amount)))
		}
		if amount := operationRowNumber(row, "deposit_amount"); amount > 0 {
			parts = append(parts, "已收定金 "+operationCurrency(amount))
		}
		return strings.Join(parts, "，")
	}

	for _, field := range []struct {
		key   string
		label string
	}{
		{key: "order_status", label: "订单状态"},
		{key: "payment_status", label: "付款状态"},
		{key: "actual_pay_amount", label: "客户应付"},
		{key: "deposit_amount", label: "已收定金"},
		{key: "subsidy_status", label: "国补资料状态"},
		{key: "is_delivered", label: "配送状态"},
		{key: "is_installed", label: "安装状态"},
		{key: "delivery_fee", label: "配送费用"},
		{key: "installation_fee", label: "安装费用"},
		{key: "commission_amount", label: "员工提成"},
		{key: "referrer_name", label: "推荐人/渠道"},
		{key: "referral_fee", label: "渠道奖励"},
	} {
		if oldValue, newValue, changed := operationChangedValue(before, after, field.key); changed {
			parts = append(parts, field.label+" "+operationFieldValue(field.key, oldValue)+"→"+operationFieldValue(field.key, newValue))
		}
	}
	return strings.Join(parts, "，")
}

func describeOrderItem(row map[string]interface{}, products map[uint]string) string {
	productID := operationRowUint(row, "product_id")
	name := products[productID]
	if name == "" {
		name = "商品"
	}
	quantity := operationRowNumber(row, "quantity")
	if quantity <= 0 {
		return ""
	}
	if operationRowBool(row, "is_gift") {
		return fmt.Sprintf("赠送%s ×%s", name, operationDisplayNumber(quantity))
	}
	return fmt.Sprintf("商品%s ×%s", name, operationDisplayNumber(quantity))
}

func describeOrderItemChange(before, after map[string]interface{}, products map[uint]string) string {
	row := after
	if len(row) == 0 {
		row = before
	}
	name := products[operationRowUint(row, "product_id")]
	if name == "" {
		name = "商品"
	}
	parts := []string{}
	for _, field := range []struct {
		key   string
		label string
	}{
		{key: "is_delivered", label: "配送状态"},
		{key: "is_installed", label: "安装状态"},
		{key: "delivery_fee", label: "配送费用"},
		{key: "installation_fee", label: "安装费用"},
	} {
		if oldValue, newValue, changed := operationChangedValue(before, after, field.key); changed {
			parts = append(parts, field.label+" "+operationFieldValue(field.key, oldValue)+"→"+operationFieldValue(field.key, newValue))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "商品「" + name + "」：" + strings.Join(parts, "，")
}

func describeProductChange(before, after map[string]interface{}) string {
	row := after
	if len(row) == 0 {
		row = before
	}
	name := operationRowString(row, "name")
	if name == "" {
		return ""
	}
	if len(before) == 0 {
		return "新增商品「" + name + "」"
	}
	if len(after) == 0 {
		return "删除商品「" + name + "」"
	}
	parts := []string{}
	for _, field := range []struct {
		key   string
		label string
	}{
		{key: "store_stock", label: "门店库存"},
		{key: "cloud_stock", label: "云仓库存"},
		{key: "sample_stock", label: "样机数量"},
		{key: "avg_cost", label: "平均成本"},
		{key: "latest_cost", label: "最新进价"},
		{key: "special_price", label: "特价"},
		{key: "margin_rate", label: "固定毛利率"},
		{key: "no_commission", label: "不计提成"},
		{key: "commission_fixed_amount", label: "每台固定提成"},
	} {
		if oldValue, newValue, changed := operationChangedValue(before, after, field.key); changed {
			parts = append(parts, field.label+" "+operationFieldValue(field.key, oldValue)+"→"+operationFieldValue(field.key, newValue))
		}
	}
	if _, _, changed := operationChangedValue(before, after, "category_id"); changed {
		parts = append(parts, "商品分类已调整")
	}
	if len(parts) == 0 {
		if oldName, newName, changed := operationChangedValue(before, after, "name"); changed {
			parts = append(parts, "名称「"+operationRowString(map[string]interface{}{"name": oldName}, "name")+"」改为「"+operationRowString(map[string]interface{}{"name": newName}, "name")+"」")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "商品「" + name + "」：" + strings.Join(parts, "，")
}

func describeNamedChange(label string, before, after map[string]interface{}, key string) string {
	oldValue, newValue, changed := operationChangedValue(before, after, key)
	if !changed {
		return ""
	}
	if len(before) == 0 {
		return "新增" + label + "「" + fmt.Sprint(newValue) + "」"
	}
	if len(after) == 0 {
		return "删除" + label + "「" + fmt.Sprint(oldValue) + "」"
	}
	return label + "「" + fmt.Sprint(oldValue) + "」改为「" + fmt.Sprint(newValue) + "」"
}

func describeDailyAttendance(before, after map[string]interface{}, employeesByID map[uint]string) string {
	row := after
	if len(row) == 0 {
		row = before
	}
	person := operationEmployeeName(operationRowUint(row, "employee_id"), employeesByID, "")
	date := operationDateLabel(operationRowString(row, "date"))
	parts := []string{}
	for _, field := range []struct {
		key   string
		label string
	}{
		{key: "check_in", label: "签到"},
		{key: "check_out", label: "签退"},
	} {
		if oldValue, newValue, changed := operationChangedValue(before, after, field.key); changed {
			parts = append(parts, field.label+"时间 "+operationFieldValue(field.key, oldValue)+"→"+operationFieldValue(field.key, newValue))
		}
	}
	if oldValue, newValue, changed := operationChangedValue(before, after, "remark"); changed {
		parts = append(parts, "备注「"+operationCleanText(fmt.Sprint(oldValue))+"」改为「"+operationCleanText(fmt.Sprint(newValue))+"」")
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s，%s考勤：%s", person, date, strings.Join(parts, "，"))
}

func describeMonthlyAttendance(before, after map[string]interface{}, employeesByID map[uint]string) string {
	row := after
	if len(row) == 0 {
		row = before
	}
	person := operationEmployeeName(operationRowUint(row, "employee_id"), employeesByID, "")
	month := operationRowString(row, "month")
	parts := []string{}
	for _, field := range []struct {
		key   string
		label string
	}{
		{key: "leave_days", label: "请假"},
		{key: "actual_days", label: "出勤"},
		{key: "late_count", label: "迟到次数"},
	} {
		if oldValue, newValue, changed := operationChangedValue(before, after, field.key); changed {
			unit := " 天"
			if field.key == "late_count" {
				unit = " 次"
			}
			parts = append(parts, field.label+" "+operationDisplayNumber(operationNumber(oldValue))+"→"+operationDisplayNumber(operationNumber(newValue))+unit)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s，%s考勤统计：%s", person, month, strings.Join(parts, "，"))
}

func describeInboundRecord(before, after map[string]interface{}, products map[uint]string) string {
	row := after
	if len(row) == 0 {
		row = before
	}
	productID := operationRowUint(row, "product_id")
	name := operationProductName(productID, products)
	quantity := operationRowNumber(row, "quantity")
	if quantity <= 0 {
		return ""
	}
	detail := fmt.Sprintf("%s入库 %s 件，入库成本 %s", name, operationDisplayNumber(quantity), operationCurrency(operationRowNumber(row, "total_cost")))
	if oldCost, newCost, changed := operationChangedValue(before, after, "unit_cost"); changed {
		detail += "，单件成本 " + operationCurrency(operationNumber(oldCost)) + "→" + operationCurrency(operationNumber(newCost))
	}
	return detail
}

func describeInventoryLog(row map[string]interface{}, products map[uint]string) string {
	change := operationRowNumber(row, "change_qty")
	if change == 0 {
		return ""
	}
	name := operationProductName(operationRowUint(row, "product_id"), products)
	verb := "增加"
	if change < 0 {
		verb = "减少"
	}
	detail := fmt.Sprintf("%s%s %s 件，库存 %s→%s", name, verb, operationDisplayNumber(absOperationNumber(change)), operationDisplayNumber(operationRowNumber(row, "before_qty")), operationDisplayNumber(operationRowNumber(row, "after_qty")))
	if related := operationRowString(row, "related_no"); related != "" {
		detail += "，关联单号 " + related
	}
	return detail
}

func describeStocktakeRecord(row map[string]interface{}, products map[uint]string) string {
	before := operationRowNumber(row, "before_stock")
	after := operationRowNumber(row, "after_stock")
	difference := operationRowNumber(row, "difference")
	if before == after && difference == 0 {
		return ""
	}
	name := operationProductName(operationRowUint(row, "product_id"), products)
	return fmt.Sprintf("%s库存盘点：%s→%s（差异 %s 件）", name, operationDisplayNumber(before), operationDisplayNumber(after), signedOperationNumber(difference))
}

func describeStocktakeBatchItem(row map[string]interface{}) string {
	name := operationRowString(row, "product_name")
	if name == "" {
		return ""
	}
	actual := operationRowNumber(row, "actual_stock")
	difference := operationRowNumber(row, "difference")
	if operationRowString(row, "status") == "uncounted" {
		return "盘点任务已包含商品「" + name + "」"
	}
	return fmt.Sprintf("商品「%s」实盘 %s 件（差异 %s 件）", name, operationDisplayNumber(actual), signedOperationNumber(difference))
}

func describeExpense(before, after map[string]interface{}) string {
	row := after
	if len(row) == 0 {
		row = before
	}
	typeName := operationRowString(row, "expense_type")
	if typeName == "" {
		typeName = "支出"
	}
	amount := operationRowNumber(row, "amount")
	text := typeName + " " + operationCurrency(amount)
	if len(before) > 0 {
		if oldAmount, newAmount, changed := operationChangedValue(before, after, "amount"); changed {
			text = typeName + "金额 " + operationCurrency(operationNumber(oldAmount)) + "→" + operationCurrency(operationNumber(newAmount))
		}
	}
	if remark := operationCleanText(operationRowString(row, "remark")); remark != "" {
		text += "，备注：" + remark
	}
	if related := operationRowString(row, "related_no"); related != "" {
		text += "，关联 " + related
	}
	return text
}

func describeFinancialLog(row map[string]interface{}) string {
	category := operationRowString(row, "category")
	amount := operationRowNumber(row, "amount")
	if category == "" && amount == 0 {
		return ""
	}
	flow := "收入"
	if operationRowNumber(row, "flow_type") == 2 {
		flow = "支出"
	}
	detail := fmt.Sprintf("%s%s %s", flow, category, operationCurrency(amount))
	if related := operationRowString(row, "related_no"); related != "" {
		detail += "，关联单号 " + related
	}
	if remark := operationCleanText(operationRowString(row, "remark")); remark != "" {
		detail += "，备注：" + remark
	}
	return detail
}

func describeAccountChange(before, after map[string]interface{}) string {
	row := after
	if len(row) == 0 {
		row = before
	}
	name := operationRowString(row, "name")
	if name == "" {
		return ""
	}
	oldBalance, newBalance, changed := operationChangedValue(before, after, "balance")
	if !changed {
		if _, newDefault, changed := operationChangedValue(before, after, "is_default_expense"); changed {
			state := "否"
			if operationBoolean(newDefault) {
				state = "是"
			}
			return fmt.Sprintf("账户「%s」已更新为默认支出账户：%s", name, state)
		}
		return ""
	}
	return fmt.Sprintf("账户「%s」余额 %s→%s", name, operationCurrency(operationNumber(oldBalance)), operationCurrency(operationNumber(newBalance)))
}

func describeTransferRecord(row map[string]interface{}) string {
	amount := operationRowNumber(row, "amount")
	if amount == 0 {
		return ""
	}
	detail := "账户资金划拨 " + operationCurrency(amount)
	if remark := operationCleanText(operationRowString(row, "remark")); remark != "" {
		detail += "，备注：" + remark
	}
	return detail
}

func describeEmployeeChange(before, after map[string]interface{}) string {
	row := after
	if len(row) == 0 {
		row = before
	}
	name := operationRowString(row, "name")
	if name == "" {
		name = operationRowString(row, "emp_no")
	}
	if name == "" {
		return ""
	}
	parts := []string{}
	for _, field := range []struct {
		key   string
		label string
	}{
		{key: "name", label: "姓名"},
		{key: "emp_no", label: "工号"},
		{key: "role", label: "岗位"},
		{key: "base_salary", label: "底薪"},
		{key: "commission_rate", label: "提成比例"},
		{key: "is_outsourced", label: "是否安装/送货师傅"},
		{key: "outsourced_type", label: "外协类型"},
	} {
		if oldValue, newValue, changed := operationChangedValue(before, after, field.key); changed {
			parts = append(parts, field.label+" "+operationFieldValue(field.key, oldValue)+"→"+operationFieldValue(field.key, newValue))
		}
	}
	if len(before) == 0 {
		return "新增员工「" + name + "」"
	}
	if len(after) == 0 {
		return "删除员工「" + name + "」"
	}
	if len(parts) == 0 {
		return "员工「" + name + "」资料已更新"
	}
	return "员工「" + name + "」：" + strings.Join(parts, "，")
}

func describeEmployeeOffboarding(before, after map[string]interface{}) string {
	row := after
	if len(row) == 0 {
		row = before
	}
	name := operationRowString(row, "employee_name")
	if name == "" {
		name = "员工"
	}
	if len(before) == 0 {
		detail := "员工「" + name + "」发起离职交接"
		if lastDay := operationDateLabel(operationRowString(row, "last_working_date")); lastDay != "日期未知" {
			detail += "，最后工作日 " + lastDay
		}
		if month := operationRowString(row, "settlement_month"); month != "" {
			detail += "，结算月份 " + month
		}
		if amount := operationRowNumber(row, "final_payable_snapshot"); amount > 0 {
			detail += "，应结算金额快照 " + operationCurrency(amount)
		}
		return detail
	}
	if len(after) == 0 {
		return "删除员工「" + name + "」的离职交接记录"
	}
	parts := []string{}
	if oldValue, newValue, changed := operationChangedValue(before, after, "status"); changed {
		parts = append(parts, "交接状态 "+operationStatusLabel(fmt.Sprint(oldValue))+"→"+operationStatusLabel(fmt.Sprint(newValue)))
	}
	if oldValue, newValue, changed := operationChangedValue(before, after, "final_payable_snapshot"); changed {
		parts = append(parts, "应结算金额快照 "+operationCurrency(operationNumber(oldValue))+"→"+operationCurrency(operationNumber(newValue)))
	}
	if _, newValue, changed := operationChangedValue(before, after, "settlement_confirmed_at"); changed {
		if newValue == nil {
			parts = append(parts, "结算确认已清除")
		} else {
			parts = append(parts, "离职薪资结算已确认")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "员工「" + name + "」：" + strings.Join(parts, "，")
}

func describeEmployeeHandoverTask(before, after map[string]interface{}, employeesByID map[uint]string) string {
	row := after
	if len(row) == 0 {
		row = before
	}
	title := operationCleanText(operationRowString(row, "title"))
	if title == "" {
		title = "员工离职交接任务"
	}
	if len(before) == 0 {
		return "新增交接任务「" + title + "」"
	}
	if len(after) == 0 {
		return "删除交接任务「" + title + "」"
	}
	parts := []string{}
	if oldValue, newValue, changed := operationChangedValue(before, after, "status"); changed {
		parts = append(parts, "状态 "+operationStatusLabel(fmt.Sprint(oldValue))+"→"+operationStatusLabel(fmt.Sprint(newValue)))
	}
	if oldValue, newValue, changed := operationChangedValue(before, after, "notes"); changed {
		parts = append(parts, "备注「"+operationCleanText(fmt.Sprint(oldValue))+"」改为「"+operationCleanText(fmt.Sprint(newValue))+"」")
	}
	if oldID, newID, changed := operationChangedValue(before, after, "assignee_employee_id"); changed {
		oldName := operationEmployeeName(uint(operationNumber(oldID)), employeesByID, "未指定")
		newName := operationEmployeeName(uint(operationNumber(newID)), employeesByID, "未指定")
		parts = append(parts, "负责人 "+oldName+"→"+newName)
	}
	if len(parts) == 0 {
		return ""
	}
	return "交接任务「" + title + "」：" + strings.Join(parts, "，")
}

func describeEmployeeOffboardingAlert(before, after map[string]interface{}) string {
	row := after
	if len(row) == 0 {
		row = before
	}
	orderNo := operationRowString(row, "order_no")
	name := operationRowString(row, "employee_name")
	if name == "" {
		name = "员工"
	}
	detail := "员工「" + name + "」离职后订单提醒"
	if orderNo != "" {
		detail += "，订单 " + orderNo
	}
	if amount := operationRowNumber(row, "refund_amount"); amount > 0 {
		detail += "，涉及退款金额 " + operationCurrency(amount)
	}
	if amount := operationRowNumber(row, "commission_amount"); amount > 0 {
		detail += "，涉及提成金额 " + operationCurrency(amount)
	}
	if status := operationRowString(row, "status"); status != "" {
		detail += "，状态：" + operationStatusLabel(status)
	}
	if message := operationCleanText(operationRowString(row, "message")); message != "" {
		detail += "，" + message
	}
	return detail
}

func describePendingInbound(row map[string]interface{}, products map[uint]string) string {
	quantity := operationRowNumber(row, "quantity")
	if quantity == 0 {
		return ""
	}
	name := operationProductName(operationRowUint(row, "product_id"), products)
	status := operationRowString(row, "status")
	detail := fmt.Sprintf("%s入库申请 %s 件", name, operationDisplayNumber(quantity))
	if submitter := operationCleanText(operationRowString(row, "submitter_name")); submitter != "" {
		detail += "，提交人：" + submitter
	}
	if status != "" {
		detail += "，状态：" + operationStatusLabel(status)
	}
	if remark := operationCleanText(operationRowString(row, "remark")); remark != "" {
		detail += "，备注：" + remark
	}
	return detail
}

func describePendingStocktake(row map[string]interface{}, products map[uint]string) string {
	name := operationProductName(operationRowUint(row, "product_id"), products)
	difference := operationRowNumber(row, "difference")
	status := operationRowString(row, "status")
	statusDetail := ""
	if status != "" {
		statusDetail = "，状态：" + operationStatusLabel(status)
	}
	if difference == 0 {
		return name + "盘点记录已更新" + statusDetail
	}
	return fmt.Sprintf("%s盘点差异 %s 件，实盘 %s 件%s", name, signedOperationNumber(difference), operationDisplayNumber(operationRowNumber(row, "actual_stock")), statusDetail)
}

func describeHistoricalCostRepair(row map[string]interface{}, products map[uint]string) string {
	cost := operationRowNumber(row, "unit_cost")
	if cost == 0 {
		return ""
	}
	name := operationProductName(operationRowUint(row, "product_id"), products)
	detail := name + "补录成本 " + operationCurrency(cost)
	if orderNo := operationRowString(row, "order_no"); orderNo != "" {
		detail += "，订单 " + orderNo
	}
	return detail
}

func describeStocktakeBatch(row map[string]interface{}) string {
	batchNo := operationRowString(row, "batch_no")
	if batchNo == "" {
		return ""
	}
	label := "盘点任务 " + batchNo
	if scope := operationCleanText(operationRowString(row, "scope_label")); scope != "" {
		label += "，范围：" + scope
	}
	if status := operationRowString(row, "status"); status != "" {
		label += "，状态：" + operationStatusLabel(status)
	}
	return label
}

func operationChangedValue(before, after map[string]interface{}, key string) (interface{}, interface{}, bool) {
	oldValue, hadBefore := before[key]
	newValue, hadAfter := after[key]
	if !hadBefore && !hadAfter {
		return nil, nil, false
	}
	if operationFieldValue(key, oldValue) == operationFieldValue(key, newValue) {
		return oldValue, newValue, false
	}
	return oldValue, newValue, true
}

func operationFieldValue(key string, value interface{}) string {
	if value == nil {
		return "无"
	}
	switch key {
	case "actual_pay_amount", "deposit_amount", "delivery_fee", "installation_fee", "commission_amount", "referral_fee", "avg_cost", "latest_cost", "special_price", "base_salary", "unit_cost", "total_cost", "commission_fixed_amount":
		return operationCurrency(operationNumber(value))
	case "margin_rate":
		return fmt.Sprintf("%.2f%%", operationNumber(value)*100)
	case "check_in", "check_out":
		if result := operationClockLabel(value); result != "" {
			return result
		}
		return "未记录"
	case "order_status":
		if operationNumber(value) == 2 {
			return "已取消"
		}
		return "正常"
	case "payment_status":
		if operationNumber(value) == 2 {
			return "已付全款"
		}
		return "仅付定金"
	case "referrer_name":
		if text := operationCleanText(fmt.Sprint(value)); text != "" {
			return text
		}
		return "未填写"
	case "is_delivered":
		if operationBoolean(value) {
			return "已配送"
		}
		return "待配送"
	case "is_installed":
		if operationBoolean(value) {
			return "已安装"
		}
		return "待安装"
	case "subsidy_status":
		return map[int64]string{0: "无国补", 1: "待提交", 2: "已提交", 3: "已回款"}[int64(operationNumber(value))]
	case "no_commission", "is_outsourced", "is_default_expense":
		if operationBoolean(value) {
			return "是"
		}
		return "否"
	case "outsourced_type":
		// 存的是英文枚举，直接展示会给用户看 "install"，这里翻成业务口径。
		switch strings.TrimSpace(fmt.Sprint(value)) {
		case "install":
			return "安装/送货师傅"
		case "boss":
			return "老板虚拟账号"
		}
		return "-"
	case "commission_rate":
		return fmt.Sprintf("%.2f%%", operationNumber(value)*100)
	case "category_id":
		if operationNumber(value) == 0 {
			return "未分类"
		}
		return "已调整"
	default:
		if text, ok := value.(string); ok {
			return operationCleanText(text)
		}
		if value == nil {
			return "无"
		}
		return fmt.Sprint(value)
	}
}

func operationRowString(row map[string]interface{}, key string) string {
	value, ok := row[key]
	if !ok || value == nil {
		return ""
	}
	return operationCleanText(fmt.Sprint(value))
}

func operationRowNumber(row map[string]interface{}, key string) float64 {
	return operationNumber(row[key])
}

func operationNumber(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case uint:
		return float64(typed)
	case json.Number:
		result, _ := typed.Float64()
		return result
	case string:
		result, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return result
	default:
		return 0
	}
}

func operationRowUint(row map[string]interface{}, key string) uint {
	value := operationNumber(row[key])
	if value <= 0 {
		return 0
	}
	return uint(value)
}

func operationRowBool(row map[string]interface{}, key string) bool {
	return operationBoolean(row[key])
}

func operationBoolean(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	case int:
		return typed != 0
	case string:
		return typed == "true" || typed == "1"
	default:
		return false
	}
}

func operationCurrency(value float64) string {
	return fmt.Sprintf("¥%.2f", value)
}

func operationDisplayNumber(value float64) string {
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func signedOperationNumber(value float64) string {
	if value > 0 {
		return "+" + operationDisplayNumber(value)
	}
	return operationDisplayNumber(value)
}

func absOperationNumber(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func operationProductName(id uint, products map[uint]string) string {
	if name := products[id]; name != "" {
		return "商品「" + name + "」"
	}
	return "商品"
}

func operationClockLabel(value interface{}) string {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" || text == "0001-01-01T00:00:00Z" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.Local().Format("15:04")
		}
	}
	return ""
}

func operationDateLabel(value string) string {
	if parsed, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
		return parsed.Format("2006年01月02日")
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.Local().Format("2006年01月02日")
	}
	return "日期未知"
}

func operationCleanText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) > 60 {
		return string([]rune(value)[:57]) + "…"
	}
	return value
}

func operationStatusLabel(value string) string {
	switch value {
	case "pending":
		return "待处理"
	case "in_progress":
		return "进行中"
	case "approved", "completed", "done":
		return "已完成"
	case "rejected":
		return "已驳回"
	case "terminated":
		return "已终止"
	case "open":
		return "待复核"
	case "resolved":
		return "已处理"
	default:
		return value
	}
}

func uniqueOperationDetails(parts []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

// UndoOperation 撤销操作
// @Summary 撤销操作
// @Description 撤销操作
// @Tags Operations
// @Param id path int true "id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_operations_id_undo
// @Router /api/operations/{id}/undo [post]
// UndoOperation restores the captured rows if no later change has touched them.
func UndoOperation(c *gin.Context) {
	release := middlewares.LockOperationHistory()
	defer release()

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "操作记录编号无效"})
		return
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "撤销事务创建失败"})
		return
	}

	var record models.OperationHistory
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, id).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "历史操作不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "历史操作读取失败"})
		return
	}
	if record.UndoneAt != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "该操作已经撤销"})
		return
	}

	changes := []operationRowChange{}
	if err := json.Unmarshal([]byte(record.Changes), &changes); err != nil || len(changes) == 0 {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "该操作没有可撤销的数据"})
		return
	}
	for _, change := range changes {
		if !undoableOperationTables[change.Table] || change.ID == 0 {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "操作记录数据不完整，无法安全撤销"})
			return
		}
		current, exists, err := readOperationRow(tx, change.Table, change.ID, change.Columns, true)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "撤销校验失败"})
			return
		}
		if len(change.After) == 0 {
			if exists {
				tx.Rollback()
				c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "相关数据已被后续操作更改，系统已阻止撤销"})
				return
			}
			continue
		}
		if !exists || !bytes.Equal(operationCanonicalJSON(current), operationCanonicalJSON(change.After)) {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "相关数据已被后续操作更改，系统已阻止撤销"})
			return
		}
	}

	if err := applyOperationUndo(tx, changes); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "撤销操作失败"})
		return
	}

	undoneAt := time.Now()
	if err := tx.Model(&record).Updates(map[string]interface{}{
		"undone_at": undoneAt,
		"undone_by": c.GetString("username"),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "撤销状态保存失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "撤销事务提交失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "撤销成功"})
}

type operationRowChange struct {
	Table   string          `json:"table"`
	ID      uint            `json:"id"`
	Columns []string        `json:"columns,omitempty"`
	Before  json.RawMessage `json:"before,omitempty"`
	After   json.RawMessage `json:"after,omitempty"`
}

var undoableOperationTables = map[string]bool{
	"accounts": true, "ai_agent_actions": true, "ai_imports": true, "agent_work_tasks": true,
	"categories": true, "daily_attendances": true, "employees": true,
	"employee_handover_tasks": true, "employee_offboarding_alerts": true, "employee_offboardings": true,
	"expenses": true, "financial_logs": true, "historical_cost_repairs": true,
	"inbound_records": true, "inventory_logs": true, "monthly_attendances": true,
	"order_items": true, "orders": true, "pending_inbounds": true, "pending_stocktakes": true,
	"products": true, "stocktake_batch_items": true, "stocktake_batches": true,
	"stocktake_records": true, "subsidy_settlements": true, "subsidy_receipts": true, "payroll_confirmations": true, "transfer_records": true,
}

func readOperationRow(tx *gorm.DB, table string, id uint, columns []string, lock bool) (map[string]interface{}, bool, error) {
	query := tx.Table(table).Where("id = ?", id)
	if len(columns) > 0 {
		query = query.Select(columns)
	}
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	rows := []map[string]interface{}{}
	if err := query.Limit(1).Find(&rows).Error; err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	return normalizeOperationRow(rows[0]), true, nil
}

func applyOperationUndo(tx *gorm.DB, changes []operationRowChange) error {
	ordered := append([]operationRowChange(nil), changes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := operationTableOrder(ordered[i].Table)
		right := operationTableOrder(ordered[j].Table)
		if left == right {
			return ordered[i].ID < ordered[j].ID
		}
		return left < right
	})

	for _, change := range ordered {
		if len(change.Before) == 0 || len(change.After) == 0 {
			continue
		}
		row, err := operationRestoreMap(tx, change.Table, change.Before)
		if err != nil {
			return fmt.Errorf("decode previous %s row %d: %w", change.Table, change.ID, err)
		}
		if err := tx.Table(change.Table).Where("id = ?", change.ID).Updates(row).Error; err != nil {
			return fmt.Errorf("restore %s row %d: %w", change.Table, change.ID, err)
		}
	}

	for _, change := range ordered {
		if len(change.Before) == 0 || len(change.After) != 0 {
			continue
		}
		row, err := operationRestoreMap(tx, change.Table, change.Before)
		if err != nil {
			return fmt.Errorf("decode deleted %s row %d: %w", change.Table, change.ID, err)
		}
		if err := tx.Table(change.Table).Create(row).Error; err != nil {
			return fmt.Errorf("recreate %s row %d: %w", change.Table, change.ID, err)
		}
	}

	for index := len(ordered) - 1; index >= 0; index-- {
		change := ordered[index]
		if len(change.Before) != 0 || len(change.After) == 0 {
			continue
		}
		if err := tx.Exec("DELETE FROM `"+change.Table+"` WHERE id = ?", change.ID).Error; err != nil {
			return fmt.Errorf("remove new %s row %d: %w", change.Table, change.ID, err)
		}
	}
	return nil
}

func operationTableOrder(table string) int {
	order := map[string]int{
		"accounts": 1, "categories": 2, "employees": 3, "employee_offboardings": 4,
		"employee_handover_tasks": 5, "employee_offboarding_alerts": 6, "products": 7,
		"orders": 8, "stocktake_batches": 9, "pending_inbounds": 10,
		"pending_stocktakes": 11, "inbound_records": 12, "expenses": 13,
		"subsidy_settlements": 14, "transfer_records": 15, "stocktake_batch_items": 16,
		"order_items": 17, "stocktake_records": 18, "historical_cost_repairs": 19,
		"daily_attendances": 20, "monthly_attendances": 21, "inventory_logs": 22,
		"financial_logs": 23, "ai_imports": 24, "ai_agent_actions": 25, "agent_work_tasks": 26,
		"subsidy_receipts": 27, "payroll_confirmations": 28,
	}
	return order[table]
}

func operationRestoreMap(tx *gorm.DB, table string, raw json.RawMessage) (map[string]interface{}, error) {
	row := map[string]interface{}{}
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, err
	}
	columns, err := tx.Migrator().ColumnTypes(table)
	if err != nil {
		return nil, err
	}
	for _, column := range columns {
		value, ok := row[column.Name()]
		text, isString := value.(string)
		if !ok || !isString {
			continue
		}
		typeName := strings.ToUpper(column.DatabaseTypeName())
		if strings.Contains(typeName, "DATE") || strings.Contains(typeName, "TIME") {
			parsed, parseErr := parseOperationTime(text, typeName)
			if parseErr != nil {
				return nil, fmt.Errorf("parse %s.%s: %w", table, column.Name(), parseErr)
			}
			row[column.Name()] = parsed
		}
	}
	return row, nil
}

func parseOperationTime(value, typeName string) (time.Time, error) {
	if strings.Contains(typeName, "DATE") && !strings.Contains(typeName, "TIME") {
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed, nil
		}
		return time.ParseInLocation("2006-01-02", value, time.Local)
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}

func normalizeOperationRow(row map[string]interface{}) map[string]interface{} {
	for key, value := range row {
		if bytesValue, ok := value.([]byte); ok {
			row[key] = string(bytesValue)
		}
	}
	return row
}

func operationCanonicalJSON(value interface{}) []byte {
	var normalized interface{}
	switch typed := value.(type) {
	case json.RawMessage:
		if err := json.Unmarshal(typed, &normalized); err != nil {
			return typed
		}
	case []byte:
		if err := json.Unmarshal(typed, &normalized); err != nil {
			return typed
		}
	default:
		normalized = value
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return []byte(fmt.Sprint(normalized))
	}
	return encoded
}
