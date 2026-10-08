package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type agentDataRequest struct {
	Dataset     string         `json:"dataset"`
	Operation   string         `json:"operation"`
	Fields      []string       `json:"fields"`
	Filters     map[string]any `json:"filters"`
	Metric      string         `json:"metric"`
	Aggregation string         `json:"aggregation"`
	GroupBy     string         `json:"group_by"`
	TimeField   string         `json:"time_field"`
	StartDate   string         `json:"start_date"`
	EndDate     string         `json:"end_date"`
	Grain       string         `json:"grain"`
	Horizon     int            `json:"horizon"`
	Limit       int            `json:"limit"`
	Offset      int            `json:"offset"`
}

type agentDataset struct {
	Label   string
	Model   any
	Note    string
	Hide    []string
	Metrics map[string]string
}

// Only business records are exposed. Credentials, AI conversations, raw images and
// employee PINs are deliberately absent from the model-visible catalog.
var agentDatasets = map[string]agentDataset{
	"accounts":           {Label: "资金账户", Model: &models.Account{}},
	"transfers":          {Label: "账户划拨", Model: &models.TransferRecord{}},
	"categories":         {Label: "商品分类", Model: &models.Category{}},
	"products":           {Label: "商品与当前库存", Model: &models.Product{}},
	"employees":          {Label: "员工档案", Model: &models.Employee{}, Hide: []string{"pin_code"}},
	"daily_attendance":   {Label: "每日考勤", Model: &models.DailyAttendance{}},
	"monthly_attendance": {Label: "月度考勤", Model: &models.MonthlyAttendance{}},
	"orders":             {Label: "销售订单", Model: &models.Order{}, Metrics: map[string]string{"sales_revenue": "(`total_amount` - `discount_amount`)"}, Note: "订单统计通常筛选 order_status=1；毛利等业务指标需按现有报表口径计算"},
	"order_items":        {Label: "订单商品明细", Model: &models.OrderItem{}, Note: "订单明细含取消订单，计算有效销售时需结合 orders.order_status"},
	"sales_items": {
		Label: "有效订单商品明细", Model: &models.OrderItem{},
		Note: "自动排除取消订单。line_revenue 为单价乘数量，line_cost 为成本快照乘数量，line_margin 为商品毛利，未扣订单级费用或抹零。",
		Metrics: map[string]string{
			"line_revenue": "(`unit_price` * `quantity`)",
			"line_cost":    "(`unit_cost` * `quantity`)",
			"line_margin":  "((`unit_price` - `unit_cost`) * `quantity`)",
		},
	},
	"expenses":                {Label: "日常支出", Model: &models.Expense{}},
	"financial_logs":          {Label: "财务流水", Model: &models.FinancialLog{}, Note: "现金流统计需排除 reversal_pair_id 非空的冲销记录；利润不等于现金流"},
	"inbound_records":         {Label: "采购入库", Model: &models.InboundRecord{}},
	"pending_inbounds":        {Label: "待确认入库", Model: &models.PendingInbound{}},
	"stocktake_records":       {Label: "库存盘点", Model: &models.StocktakeRecord{}},
	"inventory_logs":          {Label: "库存流水", Model: &models.InventoryLog{}},
	"historical_cost_repairs": {Label: "历史成本修复", Model: &models.HistoricalCostRepair{}},
	"subsidy_settlements":     {Label: "国补回款", Model: &models.SubsidySettlement{}},
	"ai_imports":              {Label: "拍照导入状态", Model: &models.AIImport{}, Hide: []string{"owner", "image_hash", "image_type", "image", "extracted"}},
}

func agentDatasetFields(spec agentDataset) (string, map[string]*schema.Field, error) {
	statement := &gorm.Statement{DB: config.DB}
	if err := statement.Parse(spec.Model); err != nil {
		return "", nil, err
	}
	hidden := make(map[string]bool, len(spec.Hide)+1)
	hidden["deleted_at"] = true
	for _, column := range spec.Hide {
		hidden[column] = true
	}
	fields := make(map[string]*schema.Field)
	for _, field := range statement.Schema.Fields {
		if field.DBName != "" && !hidden[field.DBName] {
			fields[field.DBName] = field
		}
	}
	return statement.Schema.Table, fields, nil
}

func agentDataCatalogSummary() string {
	names := make([]string, 0, len(agentDatasets))
	for name := range agentDatasets {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ") + "。字段不确定时调用 data_catalog。"
}

func agentDataCatalog() string {
	names := make([]string, 0, len(agentDatasets))
	for name := range agentDatasets {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		spec := agentDatasets[name]
		_, fields, err := agentDatasetFields(spec)
		if err != nil {
			continue
		}
		columns := make([]string, 0, len(fields))
		for column := range fields {
			columns = append(columns, column)
		}
		sort.Strings(columns)
		metrics := make([]string, 0, len(spec.Metrics))
		for metric := range spec.Metrics {
			metrics = append(metrics, metric)
		}
		sort.Strings(metrics)
		lines = append(lines, fmt.Sprintf("%s（%s）：字段 %s；计算指标 %s。%s",
			name, spec.Label, strings.Join(columns, ","), strings.Join(metrics, ","), spec.Note))
	}
	return "可查询业务数据集及字段：\n" + strings.Join(lines, "\n")
}

func agentDataQuery(ctx context.Context, req agentDataRequest) (string, error) {
	spec, ok := agentDatasets[req.Dataset]
	if !ok {
		return "未知数据集。" + agentDataCatalogSummary(), nil
	}
	_, fields, err := agentDatasetFields(spec)
	if err != nil {
		return "", err
	}
	queryContext, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	query := config.DB.WithContext(queryContext).Model(spec.Model)
	if req.Dataset == "sales_items" {
		query = query.Where("order_id IN (SELECT id FROM orders WHERE order_status = 1 AND deleted_at IS NULL)")
	}
	if req.Dataset == "orders" && req.Operation != "rows" {
		if _, specified := req.Filters["order_status"]; !specified {
			query = query.Where("order_status = ?", 1)
		}
	}
	query, err = agentApplyDataFilters(query, req, fields)
	if err != nil {
		return "查询条件无效：" + err.Error(), nil
	}
	var result any
	switch req.Operation {
	case "rows":
		result, err = agentDataRows(query, req, fields)
	case "aggregate":
		result, err = agentDataAggregate(query, req, spec, fields)
	case "trend":
		result, err = agentDataTrend(query, req, spec, fields, agentNow(ctx))
	case "forecast":
		if !agentForecastDataset(req.Dataset) {
			return "该数据集不是持续发生的业务流水，无法按月预测；请选择订单、财务流水、支出、入库、库存流水或国补回款。", nil
		}
		result, err = agentDataForecast(query, req, spec, fields, agentNow(ctx))
	default:
		return "operation 请使用 rows、aggregate、trend 或 forecast。", nil
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "查询超时，请缩小时间范围或增加筛选条件。", nil
		}
		var invalid *agentDataInputError
		if errors.As(err, &invalid) {
			return "查询参数无效：" + err.Error(), nil
		}
		return "", err
	}
	body, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	parameters, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	defaultFilter := ""
	if req.Dataset == "orders" && req.Operation != "rows" {
		if _, specified := req.Filters["order_status"]; !specified {
			defaultFilter = "默认只含有效订单（order_status=1）。"
		}
	}
	return fmt.Sprintf("数据集 %s（%s），操作 %s。%s%s\n查询参数：%s\n结果：%s",
		req.Dataset, spec.Label, req.Operation, spec.Note, defaultFilter, parameters, body), nil
}

func agentForecastDataset(dataset string) bool {
	switch dataset {
	case "orders", "sales_items", "financial_logs", "expenses", "inbound_records", "inventory_logs", "subsidy_settlements":
		return true
	default:
		return false
	}
}

type agentDataInputError struct{ message string }

func (err *agentDataInputError) Error() string { return err.message }

func agentField(fields map[string]*schema.Field, name string) (string, error) {
	if _, ok := fields[name]; !ok {
		return "", &agentDataInputError{message: fmt.Sprintf("字段 %q 不在目录中", name)}
	}
	return "`" + name + "`", nil
}

func agentApplyDataFilters(query *gorm.DB, req agentDataRequest, fields map[string]*schema.Field) (*gorm.DB, error) {
	if len(req.Filters) > 8 {
		return nil, &agentDataInputError{message: "最多使用 8 个精确筛选条件"}
	}
	for name, value := range req.Filters {
		column, err := agentField(fields, name)
		if err != nil {
			return nil, err
		}
		switch typed := value.(type) {
		case nil:
			query = query.Where(column + " IS NULL")
		case string:
			if len(typed) > 120 {
				return nil, &agentDataInputError{message: "筛选值过长"}
			}
			query = query.Where(column+" = ?", typed)
		case map[string]any:
			if len(typed) != 1 {
				return nil, &agentDataInputError{message: "高级筛选只接受一个操作符"}
			}
			for operation, operand := range typed {
				switch operation {
				case "contains":
					value, ok := operand.(string)
					if !ok || len(value) > 120 {
						return nil, &agentDataInputError{message: "contains 需要不超过 120 字的文本"}
					}
					pattern := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(value) + "%"
					query = query.Where(column+" LIKE ?", pattern)
				case "gte", "lte":
					if !agentScalarFilter(operand) {
						return nil, &agentDataInputError{message: "范围筛选需要标量"}
					}
					operator := ">="
					if operation == "lte" {
						operator = "<="
					}
					query = query.Where(column+" "+operator+" ?", operand)
				default:
					return nil, &agentDataInputError{message: "筛选操作符请使用 contains、gte、lte"}
				}
			}
		case float64, bool:
			query = query.Where(column+" = ?", typed)
		case []any:
			if len(typed) == 0 || len(typed) > 20 {
				return nil, &agentDataInputError{message: "IN 筛选需要 1 到 20 个值"}
			}
			query = query.Where(column+" IN ?", typed)
		default:
			return nil, &agentDataInputError{message: "筛选值必须是标量或数组"}
		}
	}
	if req.StartDate == "" && req.EndDate == "" {
		return query, nil
	}
	timeField := req.TimeField
	if timeField == "" {
		timeField = "created_at"
	}
	column, err := agentField(fields, timeField)
	if err != nil {
		return nil, err
	}
	if req.StartDate != "" {
		start, err := time.ParseInLocation("2006-01-02", req.StartDate, time.Local)
		if err != nil || start.Format("2006-01-02") != req.StartDate {
			return nil, &agentDataInputError{message: "start_date 应为 YYYY-MM-DD"}
		}
		query = query.Where(column+" >= ?", start)
	}
	if req.EndDate != "" {
		end, err := time.ParseInLocation("2006-01-02", req.EndDate, time.Local)
		if err != nil || end.Format("2006-01-02") != req.EndDate {
			return nil, &agentDataInputError{message: "end_date 应为 YYYY-MM-DD"}
		}
		query = query.Where(column+" < ?", end.AddDate(0, 0, 1))
	}
	return query, nil
}

func agentScalarFilter(value any) bool {
	switch typed := value.(type) {
	case string:
		return len(typed) <= 120
	case float64, bool:
		return true
	default:
		return false
	}
}

func agentDataRows(query *gorm.DB, req agentDataRequest, fields map[string]*schema.Field) (any, error) {
	if len(req.Fields) == 0 || len(req.Fields) > 20 {
		return nil, &agentDataInputError{message: "rows 需要 1 到 20 个 fields"}
	}
	columns := make([]string, 0, len(req.Fields))
	for _, name := range req.Fields {
		column, err := agentField(fields, name)
		if err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	limit := req.Limit
	if limit <= 0 || limit > 30 {
		limit = 30
	}
	if req.Offset < 0 || req.Offset > 10000 {
		return nil, &agentDataInputError{message: "offset 应在 0 到 10000 之间"}
	}
	rows := make([]map[string]any, 0)
	if err := query.Select(strings.Join(columns, ",")).Order("`id` DESC").Offset(req.Offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return map[string]any{"rows": rows, "returned": len(rows), "limit": limit, "offset": req.Offset, "possibly_truncated": len(rows) == limit}, nil
}

func agentAggregateExpression(req agentDataRequest, spec agentDataset, fields map[string]*schema.Field) (string, error) {
	switch req.Aggregation {
	case "count":
		return "COUNT(*)", nil
	case "sum", "avg":
		if expression, ok := spec.Metrics[req.Metric]; ok {
			return strings.ToUpper(req.Aggregation) + "(" + expression + ")", nil
		}
		column, err := agentField(fields, req.Metric)
		if err != nil {
			return "", err
		}
		kind := fields[req.Metric].FieldType.Kind()
		if kind == reflect.Pointer {
			kind = fields[req.Metric].FieldType.Elem().Kind()
		}
		if kind != reflect.Int && kind != reflect.Int64 && kind != reflect.Uint && kind != reflect.Float64 {
			return "", &agentDataInputError{message: "sum/avg 只能用于数值字段"}
		}
		return strings.ToUpper(req.Aggregation) + "(" + column + ")", nil
	default:
		return "", &agentDataInputError{message: "aggregation 请使用 count、sum 或 avg"}
	}
}

func agentDataAggregate(query *gorm.DB, req agentDataRequest, spec agentDataset, fields map[string]*schema.Field) (any, error) {
	expression, err := agentAggregateExpression(req, spec, fields)
	if err != nil {
		return nil, err
	}
	if req.GroupBy == "" {
		var row struct {
			Value float64 `json:"value"`
		}
		err := query.Select("COALESCE(" + expression + ",0) AS value").Scan(&row).Error
		return row, err
	}
	column, err := agentField(fields, req.GroupBy)
	if err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	rows := make([]struct {
		GroupValue string  `json:"group_value"`
		Value      float64 `json:"value"`
	}, 0)
	err = query.Select(column + " AS group_value, COALESCE(" + expression + ",0) AS value").
		Group(column).Order("value DESC").Limit(limit).Scan(&rows).Error
	return map[string]any{"groups": rows, "returned": len(rows), "limit": limit, "possibly_truncated": len(rows) == limit}, err
}

type agentTrendPoint struct {
	Bucket string  `json:"bucket"`
	Value  float64 `json:"value"`
}

func agentTrendDiagnostics(points []agentTrendPoint, grain string, now time.Time) map[string]any {
	analysis := map[string]any{"method": "最近一个完整周期与前一周期比较；异常以此前最多6个有记录周期的均值和标准差判断，阈值为2倍标准差。缺失周期不补零。"}
	current := now.Format("2006-01")
	if grain == "day" {
		current = now.Format("2006-01-02")
	}
	complete := make([]agentTrendPoint, 0, len(points))
	for _, point := range points {
		if point.Bucket < current {
			complete = append(complete, point)
		}
	}
	analysis["complete_periods"] = len(complete)
	if len(complete) == 0 {
		return analysis
	}
	latest := complete[len(complete)-1]
	analysis["latest_complete_bucket"] = latest.Bucket
	analysis["latest_value"] = latest.Value
	if len(complete) >= 2 {
		previous := complete[len(complete)-2].Value
		analysis["previous_value"] = previous
		if previous != 0 {
			analysis["change_pct"] = math.Round((latest.Value-previous)/math.Abs(previous)*10000) / 100
		}
	}
	if len(complete) < 4 {
		return analysis
	}
	previous := complete[:len(complete)-1]
	if len(previous) > 6 {
		previous = previous[len(previous)-6:]
	}
	var mean, variance float64
	for _, point := range previous {
		mean += point.Value / float64(len(previous))
	}
	for _, point := range previous {
		variance += math.Pow(point.Value-mean, 2) / float64(len(previous))
	}
	deviation := math.Sqrt(variance)
	analysis["baseline_mean"] = math.Round(mean*100) / 100
	analysis["baseline_stddev"] = math.Round(deviation*100) / 100
	if deviation > 0 {
		analysis["anomaly_score"] = math.Round((latest.Value-mean)/deviation*100) / 100
		analysis["anomaly"] = math.Abs(latest.Value-mean) >= 2*deviation
	} else {
		analysis["anomaly"] = latest.Value != mean
	}
	return analysis
}

func agentDataTrend(query *gorm.DB, req agentDataRequest, spec agentDataset, fields map[string]*schema.Field, asOf time.Time) (any, error) {
	expression, err := agentAggregateExpression(req, spec, fields)
	if err != nil {
		return nil, err
	}
	timeField := req.TimeField
	if timeField == "" {
		timeField = "created_at"
	}
	column, err := agentField(fields, timeField)
	if err != nil {
		return nil, err
	}
	format := "%Y-%m"
	grain := "month"
	if req.Grain == "day" {
		format = "%Y-%m-%d"
		grain = "day"
	} else if req.Grain != "" && req.Grain != "month" {
		return nil, &agentDataInputError{message: "grain 请使用 day 或 month"}
	}
	bucket := "DATE_FORMAT(" + column + ", '" + format + "')"
	rows := make([]agentTrendPoint, 0)
	if req.StartDate == "" && req.EndDate == "" {
		start := asOf.AddDate(-1, 0, 0)
		query = query.Where(column+" >= ?", start)
	}
	err = query.Select(bucket + " AS bucket, COALESCE(" + expression + ",0) AS value").
		Group(bucket).Order("bucket ASC").Limit(366).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"grain": grain, "points": rows, "diagnostics": agentTrendDiagnostics(rows, grain, asOf),
		"possibly_truncated": len(rows) == 366,
	}, nil
}

func agentDataForecast(query *gorm.DB, req agentDataRequest, spec agentDataset, fields map[string]*schema.Field, asOf time.Time) (any, error) {
	if req.Grain != "" && req.Grain != "month" {
		return nil, &agentDataInputError{message: "预测目前按月计算"}
	}
	if req.GroupBy != "" || req.StartDate != "" || req.EndDate != "" {
		return nil, &agentDataInputError{message: "预测使用最近 12 个完整月，请勿传 group_by 或日期范围"}
	}
	expression, err := agentAggregateExpression(req, spec, fields)
	if err != nil {
		return nil, err
	}
	timeField := req.TimeField
	if timeField == "" {
		timeField = "created_at"
	}
	column, err := agentField(fields, timeField)
	if err != nil {
		return nil, err
	}
	end := time.Date(asOf.Year(), asOf.Month(), 1, 0, 0, 0, 0, time.Local)
	start := end.AddDate(0, -12, 0)
	rows := make([]agentTrendPoint, 0)
	bucket := "DATE_FORMAT(" + column + ", '%Y-%m')"
	err = query.Where(column+" >= ? AND "+column+" < ?", start, end).
		Select(bucket + " AS bucket, COALESCE(" + expression + ",0) AS value").
		Group(bucket).Order("bucket ASC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) < 3 {
		return map[string]any{"forecast_available": false, "reason": "最近 12 个完整月中只有少于 3 个有记录的月份", "observed_months": len(rows)}, nil
	}
	byMonth := make(map[string]float64, len(rows))
	for _, row := range rows {
		byMonth[row.Bucket] = row.Value
	}
	recentObserved := 0
	for index := 9; index < 12; index++ {
		if _, exists := byMonth[start.AddDate(0, index, 0).Format("2006-01")]; exists {
			recentObserved++
		}
	}
	if recentObserved < 2 {
		return map[string]any{
			"forecast_available": false,
			"reason":             "最近 3 个完整月中少于 2 个月有记录，近期数据不足以支持预测",
			"observed_months":    len(rows),
		}, nil
	}
	series := make([]float64, 12)
	for index := range series {
		series[index] = byMonth[start.AddDate(0, index, 0).Format("2006-01")]
	}
	var recent, previous float64
	for _, value := range series[9:] {
		recent += value / 3
	}
	for _, value := range series[6:9] {
		previous += value / 3
	}
	change := recent - previous
	horizon := req.Horizon
	if horizon < 1 || horizon > 3 {
		horizon = 1
	}
	predictions := make([]map[string]any, 0, horizon)
	for index := 1; index <= horizon; index++ {
		value := recent + change*float64(index)/3
		if value < 0 {
			value = 0
		}
		predictions = append(predictions, map[string]any{
			"month":    end.AddDate(0, index-1, 0).Format("2006-01"),
			"baseline": float64(moneyCents(value)) / 100,
		})
	}
	return map[string]any{
		"forecast_available": true,
		"confidence":         "低：仅使用历史基线，未验证季节性和外部因素",
		"method":             "最近3个完整月均值，加上此前3个月与最近3个月均值差的三分之一作为每月趋势；负值截为0。仅为基线估计，不含季节性、促销或政策变化。",
		"sample_start":       start.Format("2006-01"), "sample_end": end.AddDate(0, -1, 0).Format("2006-01"),
		"observed_months": len(rows), "recent_3_month_mean": recent, "previous_3_month_mean": previous,
		"forecast": predictions,
	}, nil
}
