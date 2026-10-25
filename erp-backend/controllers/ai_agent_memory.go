package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const agentMemoryLimit = 100

type agentMemoryContextKey struct{}

type agentMemoryInput struct {
	Kind    string `json:"kind"`
	Content string `json:"content"`
}

func agentMemoryKind(kind string) bool {
	return kind == "profile" || kind == "preference" || kind == "context"
}

func agentMemoryContent(content string) (string, error) {
	content = strings.TrimSpace(content)
	if utf8.RuneCountInString(content) < 2 || utf8.RuneCountInString(content) > 240 {
		return "", errors.New("记忆内容需要 2 到 240 个字")
	}
	for _, r := range content {
		if unicode.IsControl(r) && r != '\n' {
			return "", errors.New("记忆内容不能包含控制字符")
		}
	}
	lower := strings.ToLower(content)
	for _, word := range []string{"密码", "密钥", "验证码", "secret", "token", "api key", "apikey", "private key"} {
		if strings.Contains(lower, word) {
			return "", errors.New("请勿将密码、密钥或验证码存为记忆")
		}
	}
	return content, nil
}

func saveAgentMemory(owner, kind, content, source string) (models.AIAgentMemory, error) {
	var memory models.AIAgentMemory
	if owner == "" || !agentMemoryKind(kind) {
		return memory, errors.New("记忆类型无效")
	}
	content, err := agentMemoryContent(content)
	if err != nil {
		return memory, err
	}
	err = config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("owner = ? AND kind = ? AND content = ?", owner, kind, content).Limit(1).Find(&memory).Error; err != nil {
			return err
		}
		if memory.ID != 0 {
			return nil
		}
		var count int64
		if err := tx.Model(&models.AIAgentMemory{}).Where("owner = ?", owner).Count(&count).Error; err != nil {
			return err
		}
		if count >= agentMemoryLimit {
			return errors.New("记忆已达上限，请先删除不再需要的内容")
		}
		memory = models.AIAgentMemory{Owner: owner, Kind: kind, Content: content, Source: source}
		return tx.Create(&memory).Error
	})
	return memory, err
}

// ListAIAgentMemories lists the current user's persistent memories.
// @Summary 列出 Agent 长期记忆
// @Tags AI
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID list_agent_memories
func ListAIAgentMemories(c *gin.Context) {
	memories := []models.AIAgentMemory{}
	if err := config.DB.Where("owner = ?", c.GetString("username")).Order("updated_at DESC, id DESC").Limit(agentMemoryLimit).Find(&memories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记忆读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": memories})
}

// CreateAIAgentMemory creates a user-controlled memory.
// @Summary 新增长期记忆
// @Tags AI
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID create_agent_memory
func CreateAIAgentMemory(c *gin.Context) {
	var input agentMemoryInput
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "记忆内容无效"})
		return
	}
	memory, err := saveAgentMemory(c.GetString("username"), input.Kind, input.Content, "manual")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": memory})
}

// UpdateAIAgentMemory updates one of the current user's memories.
// @Summary 更新长期记忆
// @Tags AI
// @Accept json
// @Produce json
// @Param memory_id path int true "记忆编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID update_agent_memory
func UpdateAIAgentMemory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("memory_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "记忆编号无效"})
		return
	}
	var input agentMemoryInput
	if c.ShouldBindJSON(&input) != nil || !agentMemoryKind(input.Kind) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "记忆类型无效"})
		return
	}
	content, err := agentMemoryContent(input.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	result := config.DB.Model(&models.AIAgentMemory{}).Where("id = ? AND owner = ?", id, c.GetString("username")).Updates(map[string]any{"kind": input.Kind, "content": content, "source": "manual", "evidence": ""})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记忆更新失败"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "记忆不存在"})
		return
	}
	var memory models.AIAgentMemory
	if err := config.DB.Where("id = ? AND owner = ?", id, c.GetString("username")).First(&memory).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记忆读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": memory})
}

// DeleteAIAgentMemory deletes one of the current user's memories.
// @Summary 删除长期记忆
// @Tags AI
// @Produce json
// @Param memory_id path int true "记忆编号"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID delete_agent_memory
func DeleteAIAgentMemory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("memory_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "记忆编号无效"})
		return
	}
	owner := c.GetString("username")
	deleted := false
	err = config.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND owner = ?", id, owner).Delete(&models.AIAgentMemory{})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		deleted = true
		state := models.AIAgentMemoryState{Owner: owner, ForgotBefore: time.Now()}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "owner"}}, DoUpdates: clause.AssignmentColumns([]string{"forgot_before"})}).Create(&state).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记忆删除失败"})
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "记忆不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"deleted": true}})
}

func agentMemoryText(ctx context.Context) string {
	value, _ := ctx.Value(agentMemoryContextKey{}).(string)
	return value
}

// Memory is retrieved locally without embeddings. Its content stays a quoted,
// lower-priority background note and is never evidence of a business value.
func agentRecallMemory(owner, question string) (string, error) {
	if owner == "" {
		return "", nil
	}
	memories := []models.AIAgentMemory{}
	if err := config.DB.Where("owner = ?", owner).Order("updated_at DESC, id DESC").Limit(agentMemoryLimit).Find(&memories).Error; err != nil {
		return "", err
	}
	type candidate struct {
		memory models.AIAgentMemory
		score  int
	}
	selected := []candidate{}
	for _, memory := range memories {
		score := agentMemoryOverlap(question, memory.Content)
		if memory.Kind == "profile" || memory.Kind == "preference" {
			score += 3
		}
		if memory.Kind == "context" && score == 0 {
			continue
		}
		selected = append(selected, candidate{memory: memory, score: score})
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].score > selected[j].score })
	lines := []string{}
	length := 0
	for _, item := range selected {
		line := fmt.Sprintf("[%s #%d] %s", item.memory.Kind, item.memory.ID, item.memory.Content)
		if length+utf8.RuneCountInString(line) > 1200 || len(lines) >= 8 {
			break
		}
		lines = append(lines, line)
		length += utf8.RuneCountInString(line)
	}
	return strings.Join(lines, "\n"), nil
}

func agentMemoryOverlap(question, content string) int {
	question = strings.ToLower(question)
	content = strings.ToLower(content)
	letters := []rune(question)
	score := 0
	seen := map[string]bool{}
	for i := 0; i+1 < len(letters); i++ {
		pair := string(letters[i : i+2])
		if seen[pair] || !unicode.IsLetter(letters[i]) || !unicode.IsLetter(letters[i+1]) {
			continue
		}
		seen[pair] = true
		if strings.Contains(content, pair) {
			score++
		}
	}
	return score
}

func agentExplicitMemory(input string) (string, bool) {
	text := strings.TrimSpace(input)
	for _, prefix := range []string{"请记住", "帮我记住", "记住"} {
		if strings.HasPrefix(text, prefix) {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(text, prefix)), "：:，,。 "), true
		}
	}
	return "", false
}

func handleAgentExplicitMemory(c *gin.Context, session models.AIAgentSession, input agentSendInput) bool {
	content, explicit := agentExplicitMemory(input.Message)
	if !explicit {
		return false
	}
	if session.Debug {
		return recordAgentDebugMemory(c, session, input)
	}
	kind := "context"
	if strings.HasPrefix(content, "我") || strings.Contains(content, "我的偏好") {
		kind = "profile"
	}
	if strings.Contains(content, "回答") || strings.Contains(content, "默认") || strings.Contains(content, "喜欢") {
		kind = "preference"
	}
	memory, err := saveAgentMemory(session.Owner, kind, content, "explicit_chat")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return true
	}
	message := models.AIAgentMessage{SessionID: session.ID, Role: "assistant", Content: "已记住。你可以在「长期记忆」中查看、修改或删除。", Status: "complete", Sources: "[]", Evidence: "[]"}
	err = config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&models.AIAgentMessage{SessionID: session.ID, Role: "user", Content: strings.TrimSpace(input.Message)}).Error; err != nil {
			return err
		}
		return tx.Create(&message).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记忆已保存，但会话记录失败"})
		return true
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"message": message, "memory": memory}})
	return true
}
