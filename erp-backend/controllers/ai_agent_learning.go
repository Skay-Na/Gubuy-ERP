package controllers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func newAgentRunID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func makeAgentRun(owner string, session models.AIAgentSession, runID string, user, assistant models.AIAgentMessage, task agentTask, trace *agentRunTrace, duration int64) models.AIAgentRun {
	model, promptTokens, completionTokens := "", 0, 0
	for _, call := range trace.Calls {
		if model == "" {
			model = call.Model
		}
		promptTokens += call.PromptTokens
		completionTokens += call.CompletionTokens
	}
	failureCode := ""
	if assistant.Status != "complete" {
		switch {
		case strings.HasPrefix(trace.Stop, "task_error"):
			failureCode = "task_error"
		case strings.HasPrefix(trace.Stop, "manual_error"):
			failureCode = "manual_error"
		case strings.HasPrefix(trace.Stop, "report_error"):
			failureCode = "report_error"
		case strings.HasPrefix(trace.Stop, "payout_error"):
			failureCode = "payout_error"
		case strings.HasPrefix(trace.Stop, "planner_error"):
			failureCode = "planner_error"
		case strings.HasPrefix(trace.Stop, "repeated_plan"):
			failureCode = "repeated_plan"
		case strings.HasPrefix(trace.Stop, "incomplete"):
			failureCode = "incomplete"
		case strings.HasPrefix(trace.Stop, "synthesis_error"):
			failureCode = "synthesis_error"
		default:
			failureCode = "agent_incomplete"
		}
	}
	for _, call := range trace.Tools {
		if call.Status != "ok" && call.Status != "complete" {
			failureCode = "tool_error"
			break
		}
	}
	return models.AIAgentRun{
		RunID: runID, Owner: owner, SessionID: session.ID,
		UserMessageID: user.ID, AssistantMessageID: assistant.ID,
		Topic: task.Topic, Operation: task.Operation, Status: assistant.Status,
		FailureCode: failureCode, Model: model, PromptVersion: agentPromptVersion(), ToolVersion: agentToolVersion(),
		DurationMilliseconds: duration, PromptTokens: promptTokens,
		CompletionTokens: completionTokens, ToolCalls: len(trace.Tools), EvidenceCount: len(mustEvidence(assistant.Evidence)),
	}
}

func agentPromptVersion() string {
	manuals, _ := registeredAgentManuals()
	body, _ := json.Marshal(struct {
		System  string
		Task    string
		Manuals []agentManual
	}{agentInstructions, agentTaskInstructions, manuals})
	sum := sha256.Sum256(body)
	return "p-" + hex.EncodeToString(sum[:16])
}

func agentToolVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	values := map[string]string{}
	for _, setting := range info.Settings {
		values[setting.Key] = setting.Value
	}
	revision := values["vcs.revision"]
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if revision == "" {
		return "unknown"
	}
	if values["vcs.modified"] == "true" {
		revision += ".dirty"
	}
	return revision
}

func mustEvidence(raw string) []json.RawMessage {
	items := []json.RawMessage{}
	_ = json.Unmarshal([]byte(raw), &items)
	return items
}

func agentCaseFingerprint(owner, question, detector string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(owner) + "\x00" + strings.ToLower(strings.TrimSpace(question)) + "\x00" + detector))
	return hex.EncodeToString(sum[:])
}

func recordAgentCase(tx *gorm.DB, run models.AIAgentRun, owner, question, detector, category, comment, severity string) error {
	if strings.TrimSpace(question) == "" {
		return nil
	}
	fingerprint := agentCaseFingerprint(owner, question, detector)
	var existing models.AIAgentCase
	err := tx.Where("fingerprint = ?", fingerprint).First(&existing).Error
	if err == nil {
		return tx.Model(&existing).Updates(map[string]any{
			"occurrence_count": gorm.Expr("occurrence_count + 1"), "category": category,
			"comment": comment, "updated_at": time.Now(),
		}).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return tx.Create(&models.AIAgentCase{
		Fingerprint: fingerprint, RunID: run.RunID, Owner: owner,
		AssistantMessageID: run.AssistantMessageID, Detector: detector,
		Severity: severity, State: "new", Category: category, Comment: comment,
		OccurrenceCount: 1,
	}).Error
}

func recordAutomaticAgentCase(tx *gorm.DB, run models.AIAgentRun, question string, trace *agentRunTrace) error {
	if run.FailureCode != "" {
		severity := "normal"
		detector := "runtime_failure"
		if run.FailureCode == "tool_error" {
			severity = "high"
			detector = "tool_failure"
		}
		return recordAgentCase(tx, run, run.Owner, question, detector, "runtime", run.FailureCode, severity)
	}
	for _, tool := range trace.Tools {
		if tool.Status != "ok" && tool.Status != "complete" {
			return recordAgentCase(tx, run, run.Owner, question, "tool_failure", "tool", tool.Status, "high")
		}
	}
	return nil
}

func persistAgentRun(run models.AIAgentRun, question string, trace *agentRunTrace) error {
	if err := config.DB.Create(&run).Error; err != nil {
		return err
	}
	return config.DB.Transaction(func(tx *gorm.DB) error {
		return recordAutomaticAgentCase(tx, run, question, trace)
	})
}

func persistAgentPredictionSnapshots(owner string, message models.AIAgentMessage, evidence []agentEvidence, asOf time.Time) error {
	for _, item := range evidence {
		if item.Plan.Tool != "data_query" || item.Plan.Data.Operation != "forecast" {
			continue
		}
		marker := strings.LastIndex(item.Result, "\n结果：")
		if marker < 0 {
			continue
		}
		var result struct {
			Available  bool    `json:"forecast_available"`
			NaiveValue float64 `json:"recent_3_month_mean"`
			Forecast   []struct {
				Month    string  `json:"month"`
				Baseline float64 `json:"baseline"`
			} `json:"forecast"`
		}
		if err := json.Unmarshal([]byte(item.Result[marker+len("\n结果："):]), &result); err != nil || !result.Available {
			continue
		}
		request := item.Plan.Data
		filtersJSON, _ := json.Marshal(request.Filters)
		timeField := request.TimeField
		if timeField == "" {
			timeField = "created_at"
		}
		for _, point := range result.Forecast {
			if _, err := time.Parse("2006-01", point.Month); err != nil {
				continue
			}
			snapshot := models.AIAgentPredictionSnapshot{
				RunID: message.RunID, Owner: owner, AssistantMessageID: message.ID,
				Dataset: request.Dataset, Metric: request.Metric, Aggregation: request.Aggregation,
				TimeField: timeField, TargetMonth: point.Month, AsOf: asOf,
				FiltersJSON: string(filtersJSON), MethodVersion: "monthly-baseline-v1", ForecastValue: point.Baseline, NaiveValue: result.NaiveValue,
			}
			if err := config.DB.Where("assistant_message_id = ? AND target_month = ? AND dataset = ? AND metric = ?", message.ID, point.Month, request.Dataset, request.Metric).FirstOrCreate(&snapshot).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// EvaluateAIAgentPredictions fills actual values for closed forecast months.
// @Summary 回填已结束预测月份的实际值
// @Tags AI
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID evaluate_agent_predictions
func EvaluateAIAgentPredictions(c *gin.Context) {
	var snapshots []models.AIAgentPredictionSnapshot
	monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)
	if err := config.DB.Where("actual_value IS NULL AND target_month < ?", monthStart.Format("2006-01")).Order("id asc").Limit(200).Find(&snapshots).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "预测快照读取失败"})
		return
	}
	updated, failed := 0, 0
	for _, snapshot := range snapshots {
		spec, ok := agentDatasets[snapshot.Dataset]
		if !ok {
			failed++
			continue
		}
		_, fields, err := agentDatasetFields(spec)
		if err != nil {
			failed++
			continue
		}
		start, err := time.ParseInLocation("2006-01", snapshot.TargetMonth, time.Local)
		if err != nil {
			failed++
			continue
		}
		end := start.AddDate(0, 1, 0).AddDate(0, 0, -1)
		filters := map[string]any{}
		if snapshot.FiltersJSON != "" {
			_ = json.Unmarshal([]byte(snapshot.FiltersJSON), &filters)
		}
		req := agentDataRequest{Dataset: snapshot.Dataset, Operation: "aggregate", Metric: snapshot.Metric, Aggregation: snapshot.Aggregation, TimeField: snapshot.TimeField, StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02"), Filters: filters}
		query := config.DB.Model(spec.Model)
		if req.Dataset == "sales_items" {
			query = query.Where("order_id IN (SELECT id FROM orders WHERE order_status = 1 AND deleted_at IS NULL)")
		}
		if req.Dataset == "orders" {
			query = query.Where("order_status = ?", 1)
		}
		query, err = agentApplyDataFilters(query, req, fields)
		if err != nil {
			failed++
			continue
		}
		value, err := agentDataAggregate(query, req, spec, fields)
		if err != nil {
			failed++
			continue
		}
		body, err := json.Marshal(value)
		if err != nil {
			failed++
			continue
		}
		var row struct {
			Value float64 `json:"value"`
		}
		if err := json.Unmarshal(body, &row); err != nil {
			failed++
			continue
		}
		evaluatedAt := time.Now()
		if err := config.DB.Model(&snapshot).Updates(map[string]any{"actual_value": row.Value, "evaluated_at": evaluatedAt}).Error; err != nil {
			failed++
			continue
		}
		updated++
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"evaluated": updated, "failed": failed, "remaining_due": len(snapshots) == 200}})
}

// @Summary 查看 Agent 预测回测结果
// @Tags AI
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_agent_predictions
func ListAIAgentPredictions(c *gin.Context) {
	var snapshots []models.AIAgentPredictionSnapshot
	if err := config.DB.Where("owner = ?", c.GetString("username")).Order("id desc").Limit(100).Find(&snapshots).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "预测记录读取失败"})
		return
	}
	var absoluteError, naiveError, absoluteActual float64
	evaluated := 0
	for _, item := range snapshots {
		if item.ActualValue == nil {
			continue
		}
		errorValue := *item.ActualValue - item.ForecastValue
		absoluteError += math.Abs(errorValue)
		naiveError += math.Abs(*item.ActualValue - item.NaiveValue)
		absoluteActual += math.Abs(*item.ActualValue)
		evaluated++
	}
	metrics := gin.H{"evaluated": evaluated, "mae_forecast": nil, "mae_naive_3_month": nil, "wape_forecast_pct": nil, "wape_naive_3_month_pct": nil}
	if evaluated > 0 {
		metrics["mae_forecast"] = absoluteError / float64(evaluated)
		metrics["mae_naive_3_month"] = naiveError / float64(evaluated)
		if absoluteActual > 0 {
			metrics["wape_forecast_pct"] = absoluteError / absoluteActual * 100
			metrics["wape_naive_3_month_pct"] = naiveError / absoluteActual * 100
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"metrics": metrics, "snapshots": snapshots}})
}

// SubmitAIAgentFeedback records a rating and creates a deduplicated candidate for negative feedback.
// @Summary 提交 Agent 回答反馈
// @Tags AI
// @Accept json
// @Produce json
// @Param message_id path int true "助手消息编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID submit_agent_feedback
func SubmitAIAgentFeedback(c *gin.Context) {
	owner := c.GetString("username")
	id, err := strconv.ParseUint(c.Param("message_id"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "消息编号无效"})
		return
	}
	var input struct {
		Rating   string `json:"rating"`
		Category string `json:"category"`
		Comment  string `json:"comment"`
	}
	if c.ShouldBindJSON(&input) != nil || (input.Rating != "helpful" && input.Rating != "unhelpful") || len([]rune(input.Comment)) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "反馈内容无效"})
		return
	}
	allowed := map[string]bool{"": true, "understanding": true, "calculation": true, "evidence": true, "forecast": true, "failure": true, "other": true}
	if !allowed[input.Category] || (input.Rating == "helpful" && (input.Category != "" || strings.TrimSpace(input.Comment) != "")) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "反馈分类无效"})
		return
	}
	var assistant models.AIAgentMessage
	if err := config.DB.Where("ai_agent_messages.id = ? AND ai_agent_messages.role = ? AND EXISTS (SELECT 1 FROM ai_agent_sessions WHERE ai_agent_sessions.id = ai_agent_messages.session_id AND ai_agent_sessions.owner = ?)", id, "assistant", owner).
		First(&assistant).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "消息不存在"})
		return
	}
	feedback := models.AIAgentFeedback{AssistantMessageID: assistant.ID, Owner: owner, Rating: input.Rating, Category: input.Category, Comment: strings.TrimSpace(input.Comment)}
	err = config.DB.Transaction(func(tx *gorm.DB) error {
		var previous models.AIAgentFeedback
		err := tx.Where("assistant_message_id = ? AND owner = ?", assistant.ID, owner).First(&previous).Error
		if err == gorm.ErrRecordNotFound {
			if err := tx.Create(&feedback).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := tx.Model(&previous).Updates(map[string]any{"rating": feedback.Rating, "category": feedback.Category, "comment": feedback.Comment}).Error; err != nil {
			return err
		}
		if input.Rating != "unhelpful" {
			return nil
		}
		var run models.AIAgentRun
		if err := tx.Where("assistant_message_id = ? AND owner = ?", assistant.ID, owner).First(&run).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}
			run = models.AIAgentRun{RunID: assistant.RunID, Owner: owner, SessionID: assistant.SessionID, AssistantMessageID: assistant.ID}
		}
		var user models.AIAgentMessage
		if err := tx.Where("session_id = ? AND role = ? AND id < ?", assistant.SessionID, "user", assistant.ID).Order("id DESC").First(&user).Error; err != nil {
			return err
		}
		return recordAgentCase(tx, run, owner, user.Content, "user_feedback", input.Category, feedback.Comment, "normal")
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "反馈保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"rating": feedback.Rating}})
}

// ListAIAgentCases returns only the authenticated owner's cases.
type agentCaseView struct {
	models.AIAgentCase
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

var agentPhonePattern = regexp.MustCompile(`1[3-9][0-9]{9}`)
var agentLongIDPattern = regexp.MustCompile(`[0-9]{15,18}[0-9xX]?`)
var agentEmailPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

func redactAgentCaseText(value string, sensitive []string) string {
	value = agentPhonePattern.ReplaceAllString(value, "[手机号]")
	value = agentLongIDPattern.ReplaceAllString(value, "[证件号]")
	value = agentEmailPattern.ReplaceAllString(value, "[邮箱]")
	for _, item := range sensitive {
		if item != "" {
			value = strings.ReplaceAll(value, item, "[已脱敏]")
		}
	}
	return value
}

func agentCaseSensitiveValues() ([]string, error) {
	values := []string{}
	for _, query := range []struct {
		model any
		field string
	}{
		{model: &models.Employee{}, field: "name"}, {model: &models.Employee{}, field: "phone"}, {model: &models.Employee{}, field: "emp_no"},
		{model: &models.Order{}, field: "customer_name"}, {model: &models.Order{}, field: "customer_phone"},
		{model: &models.Order{}, field: "customer_address"}, {model: &models.Order{}, field: "order_no"},
	} {
		var found []string
		if err := config.DB.Model(query.model).Distinct(query.field).Where(query.field+" <> ''").Pluck(query.field, &found).Error; err != nil {
			return nil, err
		}
		for _, value := range found {
			if len([]rune(value)) >= 2 {
				values = append(values, value)
			}
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return values, nil
}

// @Summary 列出 Agent 候选案例
// @Tags AI
// @Produce json
// @Param state query string false "案例状态"
// @Param after_id query int false "案例 ID 游标"
// @Param limit query int false "返回数量"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_agent_cases
func ListAIAgentCases(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	if limit < 1 || limit > 100 {
		limit = 30
	}
	state := c.Query("state")
	afterID, _ := strconv.ParseUint(c.DefaultQuery("after_id", "0"), 10, 64)
	query := config.DB.Table("ai_agent_cases AS cases").
		Select("cases.*, user_message.content AS question, assistant_message.content AS answer").
		Joins("JOIN ai_agent_messages AS assistant_message ON assistant_message.id = cases.assistant_message_id").
		Joins("JOIN ai_agent_messages AS user_message ON user_message.id = (SELECT MAX(id) FROM ai_agent_messages WHERE session_id = assistant_message.session_id AND role = 'user' AND id < assistant_message.id)").
		Where("cases.owner = ?", c.GetString("username"))
	if state != "" {
		query = query.Where("state = ?", state)
	}
	if afterID > 0 {
		query = query.Where("cases.id > ?", afterID)
	}
	var cases []agentCaseView
	if err := query.Order("cases.id ASC").Limit(limit).Find(&cases).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "案例读取失败"})
		return
	}
	sensitive, err := agentCaseSensitiveValues()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "案例脱敏失败，已停止返回案例内容"})
		return
	}
	for i := range cases {
		cases[i].Question = redactAgentCaseText(cases[i].Question, sensitive)
		cases[i].Answer = redactAgentCaseText(cases[i].Answer, sensitive)
		cases[i].Comment = redactAgentCaseText(cases[i].Comment, sensitive)
		cases[i].VerifiedLabel = redactAgentCaseText(cases[i].VerifiedLabel, sensitive)
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": cases})
}

// UpdateAIAgentCase stores a verified label for the authenticated owner's case.
// @Summary 更新 Agent 案例状态和核实结论
// @Tags AI
// @Accept json
// @Produce json
// @Param case_id path int true "案例编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID update_agent_case
func UpdateAIAgentCase(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("case_id"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "案例编号无效"})
		return
	}
	var input struct {
		State         string `json:"state"`
		VerifiedLabel string `json:"verified_label"`
	}
	if c.ShouldBindJSON(&input) != nil || !map[string]bool{"new": true, "triaged": true, "labeled": true, "fixed": true, "rejected": true, "monitored": true}[input.State] || len([]rune(input.VerifiedLabel)) > 3000 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "案例标注无效"})
		return
	}
	var existing models.AIAgentCase
	if err := config.DB.Where("id = ? AND owner = ?", id, c.GetString("username")).First(&existing).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "案例不存在"})
		return
	}
	result := config.DB.Model(&existing).Updates(map[string]any{"state": input.State, "verified_label": input.VerifiedLabel})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "案例更新失败"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "案例不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"state": input.State}})
}
