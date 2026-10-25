package controllers

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"erp-backend/agentnative"
	"erp-backend/config"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type agentScheduleRequest struct {
	TaskType  string `json:"task_type"`
	Timezone  string `json:"timezone"`
	TimeOfDay string `json:"time_of_day"`
}

func scheduleService() agentnative.ScheduleService {
	return agentnative.ScheduleService{DB: config.DB}
}

func scheduleID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	return uint(id), err == nil && id > 0
}

func schedulerEnabled() bool {
	return os.Getenv("AGENT_PERSISTENT_SCHEDULER_ENABLED") == "1"
}

// ListAgentSchedules returns only schedules owned by the authenticated administrator.
// @Summary 列出长期 Agent 任务
// @Tags AI
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_agent_schedules
func ListAgentSchedules(c *gin.Context) {
	items, err := scheduleService().List(c.Request.Context(), c.GetString("username"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "长期任务读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

// CreateAgentSchedule creates an explicitly requested daily inventory check.
// @Summary 创建每日库存检查计划
// @Tags AI
// @Accept json
// @Produce json
// @Param body body agentScheduleRequest true "执行时间与时区"
// @Success 201 {object} APIResponse
// @Security ApiKeyAuth
// @ID create_agent_schedule
func CreateAgentSchedule(c *gin.Context) {
	if !schedulerEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "持久化 Scheduler 尚未启用"})
		return
	}
	var request agentScheduleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "长期任务参数无效"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	schedule, err := scheduleService().Create(ctx, agentnative.ScheduleInput{
		OwnerID: c.GetString("username"), TaskType: request.TaskType,
		Timezone: request.Timezone, TimeOfDay: request.TimeOfDay,
	})
	if err != nil {
		if errors.Is(err, agentnative.ErrInvalidSchedule) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "仅支持有效时区和每日库存检查计划"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "长期任务创建失败"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": 201, "data": schedule})
}

// UpdateAgentSchedule changes the time and timezone of an owned schedule.
// @Summary 修改长期任务执行时间
// @Tags AI
// @Accept json
// @Produce json
// @Param id path int true "计划编号"
// @Param body body agentScheduleRequest true "执行时间与时区"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID update_agent_schedule
func UpdateAgentSchedule(c *gin.Context) {
	id, ok := scheduleID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "计划编号无效"})
		return
	}
	var request agentScheduleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "执行时间参数无效"})
		return
	}
	schedule, err := scheduleService().Update(c.Request.Context(), c.GetString("username"), id, request.Timezone, request.TimeOfDay)
	writeScheduleResult(c, schedule, err)
}

// PauseAgentSchedule pauses one owned recurring task.
// @Summary 暂停长期任务
// @Tags AI
// @Produce json
// @Param id path int true "计划编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID pause_agent_schedule
func PauseAgentSchedule(c *gin.Context) { setAgentScheduleState(c, "pause") }

// ResumeAgentSchedule resumes one owned recurring task.
// @Summary 恢复长期任务
// @Tags AI
// @Produce json
// @Param id path int true "计划编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID resume_agent_schedule
func ResumeAgentSchedule(c *gin.Context) { setAgentScheduleState(c, "resume") }

// CancelAgentSchedule cancels one owned recurring task.
// @Summary 取消长期任务
// @Tags AI
// @Produce json
// @Param id path int true "计划编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID cancel_agent_schedule
func CancelAgentSchedule(c *gin.Context) { setAgentScheduleState(c, "cancel") }

func setAgentScheduleState(c *gin.Context, action string) {
	id, ok := scheduleID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "计划编号无效"})
		return
	}
	if action == "resume" && !schedulerEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "持久化 Scheduler 尚未启用"})
		return
	}
	schedule, err := scheduleService().SetState(c.Request.Context(), c.GetString("username"), id, action)
	writeScheduleResult(c, schedule, err)
}

// ListAgentScheduleRuns returns recent execution history for one owned schedule.
// @Summary 查询长期任务执行历史
// @Tags AI
// @Produce json
// @Param id path int true "计划编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_agent_schedule_runs
func ListAgentScheduleRuns(c *gin.Context) {
	id, ok := scheduleID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "计划编号无效"})
		return
	}
	items, err := scheduleService().History(c.Request.Context(), c.GetString("username"), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "长期任务不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "执行历史读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

func writeScheduleResult(c *gin.Context, schedule any, err error) {
	switch {
	case errors.Is(err, agentnative.ErrScheduleNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "长期任务不存在"})
	case errors.Is(err, agentnative.ErrInvalidSchedule):
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "计划时间或时区无效"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "长期任务操作失败"})
	default:
		c.JSON(http.StatusOK, gin.H{"code": 200, "data": schedule})
	}
}
