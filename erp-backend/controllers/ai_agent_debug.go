package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetAIAgentMessageTrace exposes persisted diagnostics only to the session owner.
// @Summary 查询 Agent 消息执行追踪
// @Tags AI
// @Produce json
// @Param message_id path int true "消息编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID get_agent_message_trace
// @Router /api/ai/agent/messages/{message_id}/trace [get]
func GetAIAgentMessageTrace(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.ParseUint(c.Param("message_id"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "消息编号无效"})
		return
	}
	var record struct {
		models.AIAgentMessage
		Debug bool
	}
	err = config.DB.Table("ai_agent_messages").
		Select("ai_agent_messages.*, ai_agent_sessions.debug AS debug").
		Joins("JOIN ai_agent_sessions ON ai_agent_sessions.id = ai_agent_messages.session_id").
		Where("ai_agent_messages.id = ? AND ai_agent_sessions.owner = ?", id, c.GetString("username")).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "消息不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "追踪读取失败"})
		return
	}
	message := record.AIAgentMessage
	data := gin.H{"message": message, "debug": record.Debug}
	for name, raw := range map[string]string{
		"trace": message.Trace, "task": message.Task, "evidence": message.Evidence,
	} {
		if raw == "" {
			data[name] = nil
			continue
		}
		if !json.Valid([]byte(raw)) {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "追踪记录格式错误"})
			return
		}
		data[name] = json.RawMessage(raw)
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": data})
}

// recordAgentDebugMemory keeps the test exchange without changing persistent memory.
func recordAgentDebugMemory(c *gin.Context, session models.AIAgentSession, input agentSendInput) bool {
	trace := &agentRunTrace{
		Profile: agentProfileFor(input.Thinking, input.Message),
		Calls:   []agentCallTrace{}, Tools: []agentToolTrace{}, Stop: "debug_memory_skipped",
	}
	message := models.AIAgentMessage{
		SessionID: session.ID, Role: "assistant", Status: "complete",
		Content: "调试会话已记录这条消息，未修改长期记忆。",
		Sources: "[]", Evidence: "[]", Trace: agentTraceJSON(trace),
	}
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		user := models.AIAgentMessage{SessionID: session.ID, Role: "user", Content: input.Message}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return tx.Create(&message).Error
	})
	if err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "调试消息保存失败"})
		return true
	}
	c.JSON(200, gin.H{"code": 200, "data": gin.H{
		"message": message, "memory_updates": []models.AIAgentMemory{},
	}})
	return true
}
