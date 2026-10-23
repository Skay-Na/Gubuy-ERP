package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"erp-backend/agentnative"
	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CreateAIAgentPlannerTaskRequest struct {
	Message string `json:"message"`
}

// CreateAIAgentPlannerTask queues a durable, read-only retail analysis from chat.
// @Summary 提交通用 Agent Planner 分析任务
// @Tags AI
// @Accept json
// @Produce json
// @Param id path int true "会话编号"
// @Param body body CreateAIAgentPlannerTaskRequest true "经营分析目标"
// @Success 202 {object} APIResponse
// @Security ApiKeyAuth
// @ID create_agent_planner_task
// @Router /api/ai/agent/sessions/{id}/planner-tasks [post]
func CreateAIAgentPlannerTask(c *gin.Context) {
	session, ok := agentSession(c)
	if !ok {
		return
	}
	if os.Getenv("AGENT_RUNTIME_ENABLED") != "1" || os.Getenv("AGENT_LLM_PLANNER_ENABLED") != "1" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "通用 Planner 尚未启用，请联系管理员检查 Agent Runtime 配置"})
		return
	}
	var request CreateAIAgentPlannerTaskRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "任务参数无效"})
		return
	}
	goal := strings.TrimSpace(request.Message)
	if goal == "" || len([]rune(goal)) > 1200 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "分析目标必须为 1 至 1200 个字符"})
		return
	}
	runID, err := newAgentRunID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "任务编号生成失败"})
		return
	}
	inputJSON, err := json.Marshal(map[string]string{"goal": goal})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "任务内容编码失败"})
		return
	}
	capabilities := []string{"retail.read", "agent.task.read"}
	capabilitiesJSON, err := json.Marshal(capabilities)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "任务权限范围生成失败"})
		return
	}
	var task models.AgentWorkTask
	var assistant models.AIAgentMessage
	userMessage := models.AIAgentMessage{SessionID: session.ID, Role: "user", Content: goal, RunID: runID}
	err = config.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		key := "planner-chat:" + runID
		task = models.AgentWorkTask{
			TenantID: agentnative.SingleTenant, Kind: "agent_analysis", Status: "CREATED", Step: "queued",
			CreatedBy: session.Owner, IdempotencyKey: &key, InputJSON: string(inputJSON),
			CapabilitiesJSON: string(capabilitiesJSON), ChatSessionID: &session.ID,
		}
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		taskID := task.ID
		userMessage.TaskID = &taskID
		if err := tx.Create(&userMessage).Error; err != nil {
			return err
		}
		assistant = models.AIAgentMessage{
			SessionID: session.ID, Role: "assistant", Content: "已提交通用经营分析任务，正在等待 Agent Runtime 执行。",
			Sources: "[]", Evidence: "[]", Status: "running", TaskID: &taskID, RunID: runID,
		}
		if err := tx.Create(&assistant).Error; err != nil {
			return err
		}
		return tx.Model(&task).Update("chat_message_id", assistant.ID).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "Planner 任务入队失败"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"code": 202, "data": gin.H{"task": task, "message": assistant, "user_message": userMessage}})
}

func nativeMonitor() agentnative.MonitorService {
	return agentnative.MonitorService{DB: config.DB}
}

// ListAgentNativeSignals lists persisted operating signals.
// @Summary 列出经营信号
// @Tags AI
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_agent_native_signals
// @Router /api/ai/manager/signals [get]
func ListAgentNativeSignals(c *gin.Context) {
	items := []models.AgentSignal{}
	if err := config.DB.Where("tenant_id = ?", agentnative.SingleTenant).
		Order("id DESC").Limit(100).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "经营信号读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

// ListAgentNativeTasks lists durable operating tasks.
// @Summary 列出经营任务
// @Tags AI
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_agent_native_tasks
// @Router /api/ai/manager/tasks [get]
func ListAgentNativeTasks(c *gin.Context) {
	items := []models.AgentWorkTask{}
	if err := config.DB.Where("tenant_id = ?", agentnative.SingleTenant).
		Order("id DESC").Limit(100).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "任务读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

// ListAgentNativeTaskSteps returns the persisted tool execution trace for a task.
// @Summary 列出 Agent 任务执行步骤
// @Tags AI
// @Produce json
// @Param id path int true "任务编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_agent_native_task_steps
// @Router /api/ai/manager/tasks/{id}/steps [get]
func ListAgentNativeTaskSteps(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "任务编号无效"})
		return
	}
	items := []models.AgentWorkTaskStep{}
	if err := config.DB.Joins("JOIN agent_work_tasks ON agent_work_tasks.id = agent_work_task_steps.task_id").
		Where("agent_work_task_steps.task_id = ? AND agent_work_tasks.tenant_id = ?", id, agentnative.SingleTenant).
		Order("agent_work_task_steps.sequence").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "步骤记录读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

// CancelAgentNativeTask cancels a queued or running task; workers observe the durable status between tools.
// @Summary 取消 Agent 任务
// @Tags AI
// @Produce json
// @Param id path int true "任务编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID cancel_agent_native_task
// @Router /api/ai/manager/tasks/{id}/cancel [post]
func CancelAgentNativeTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "任务编号无效"})
		return
	}
	now := time.Now()
	err = config.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var task models.AgentWorkTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"id = ? AND tenant_id = ? AND status IN ?", id, agentnative.SingleTenant, []string{"CREATED", "PLANNING", "RUNNING"},
		).First(&task).Error; err != nil {
			return err
		}
		if err := tx.Model(&task).Updates(map[string]any{"status": "CANCELLED", "step": "cancelled", "lease_until": nil}).Error; err != nil {
			return err
		}
		if task.ChatMessageID != nil {
			if err := tx.Model(&models.AIAgentMessage{}).Where("id = ?", *task.ChatMessageID).
				Updates(map[string]any{"status": "cancelled", "content": "已取消该 Agent 任务。"}).Error; err != nil {
				return err
			}
		}
		if task.Kind == "schedule_inventory_check" {
			if err := tx.Model(&models.AgentScheduleRun{}).Where("linked_task_id = ? AND status IN ?", task.ID, []string{"QUEUED", "RUNNING"}).
				Updates(map[string]any{"status": "CANCELLED", "finished_at": now, "error": "用户取消", "lease_until": nil}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&models.AgentActivity{
			TenantID: agentnative.SingleTenant, TaskID: uint(id), Event: "task_cancelled",
			Detail: "管理员请求取消任务；可取消的模型和工具调用已收到取消信号",
		}).Error
	})
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "任务取消失败"})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "任务不存在或已结束"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "任务已取消"})
}

// ListAgentNativeDrafts lists internal replenishment suggestions.
// @Summary 列出补货草案
// @Tags AI
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_agent_native_drafts
// @Router /api/ai/manager/drafts [get]
func ListAgentNativeDrafts(c *gin.Context) {
	items := []models.AgentReplenishmentDraft{}
	if err := config.DB.Where("tenant_id = ?", agentnative.SingleTenant).
		Order("id DESC").Limit(100).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "补货草案读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

type ReviewAgentDraftRequest struct {
	Decision string `json:"decision"`
	Quantity int    `json:"quantity"`
	Note     string `json:"note"`
}

// ReviewAgentNativeDraft records an administrator's internal replenishment decision.
// It does not create purchase orders or mutate stock.
// @Summary 审核内部补货草案
// @Tags AI
// @Accept json
// @Produce json
// @Param id path int true "草案编号"
// @Param body body ReviewAgentDraftRequest true "审核决定"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID review_agent_native_draft
// @Router /api/ai/manager/drafts/{id}/review [post]
func ReviewAgentNativeDraft(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "草案编号无效"})
		return
	}
	var req ReviewAgentDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "审核参数无效"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	draft, err := nativeMonitor().ReviewDraft(ctx, agentnative.DraftReviewInput{
		DraftID: uint(id), Decision: req.Decision, Quantity: req.Quantity,
		Note: req.Note, Actor: c.GetString("username"),
	})
	if err != nil {
		switch {
		case errors.Is(err, agentnative.ErrDraftNotFound):
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "补货草案不存在"})
		case errors.Is(err, agentnative.ErrDraftNotReviewable):
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "草案已处理，不能重复审核"})
		case errors.Is(err, agentnative.ErrDraftStale):
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "审核时库存已变化，草案已标记过期，请重新扫描"})
		case errors.Is(err, agentnative.ErrInvalidDraftReview):
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "审核参数无效"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "草案审核暂时不可用"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": draft})
}

// ListAgentNativeActivities lists task activity records.
// @Summary 列出经营任务活动
// @Tags AI
// @Produce json
// @Param task_id query int false "任务编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_agent_native_activities
// @Router /api/ai/manager/activities [get]
func ListAgentNativeActivities(c *gin.Context) {
	items := []models.AgentActivity{}
	query := config.DB.Where("tenant_id = ?", agentnative.SingleTenant)
	if raw := c.Query("task_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "任务编号无效"})
			return
		}
		query = query.Where("task_id = ?", id)
	}
	if err := query.Order("id DESC").Limit(100).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "活动读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

// ListAgentNativeGoals lists operating goals.
// @Summary 列出经营目标
// @Tags AI
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_agent_native_goals
// @Router /api/ai/manager/goals [get]
func ListAgentNativeGoals(c *gin.Context) {
	items := []models.AgentGoal{}
	if err := config.DB.Where("tenant_id = ?", agentnative.SingleTenant).
		Order("id DESC").Limit(100).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "目标读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

type CreateAgentGoalRequest struct {
	GoalType     string  `json:"goal_type"`
	TargetMetric string  `json:"target_metric"`
	TargetValue  float64 `json:"target_value"`
	Deadline     string  `json:"deadline"`
	Constraints  string  `json:"constraints"`
	Priority     int     `json:"priority"`
}

// CreateAgentNativeGoal creates a structured objective, not authorization.
// @Summary 创建经营目标
// @Tags AI
// @Accept json
// @Produce json
// @Param body body CreateAgentGoalRequest true "经营目标"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID create_agent_native_goal
// @Router /api/ai/manager/goals [post]
func CreateAgentNativeGoal(c *gin.Context) {
	var req CreateAgentGoalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "目标参数无效"})
		return
	}
	deadline, err := time.ParseInLocation("2006-01-02", req.Deadline, time.Local)
	validType := req.GoalType == "reduce_stockout"
	validMetric := req.TargetMetric == "stockout_signals"
	if err != nil || !deadline.After(time.Now()) || !validType || !validMetric ||
		req.TargetValue < 0 || req.TargetValue > 100000 || len([]rune(req.Constraints)) > 500 ||
		req.Priority < 1 || req.Priority > 3 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "目标类型、指标、数值、日期或优先级无效"})
		return
	}
	goal := models.AgentGoal{
		TenantID: agentnative.SingleTenant,
		GoalType: req.GoalType, TargetMetric: req.TargetMetric,
		TargetValue: req.TargetValue, Deadline: deadline,
		Constraints: strings.TrimSpace(req.Constraints),
		Priority:    req.Priority, Status: "active", CreatedBy: c.GetString("username"),
	}
	if err := config.DB.Create(&goal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "目标保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": goal})
}

// RunAgentNativeMonitor is an explicit rule scan. It never orders or pays.
// @Summary 执行经营规则扫描
// @Tags AI
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID run_agent_native_monitor
// @Router /api/ai/manager/scan [post]
func RunAgentNativeMonitor(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	service := nativeMonitor()
	signals, err := service.Scan(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "经营监测失败"})
		return
	}
	tasks, err := service.ProcessPending(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "信号已保存，草案处理失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"new_signals": signals, "processed_tasks": tasks}})
}
