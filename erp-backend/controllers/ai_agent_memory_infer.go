package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"gorm.io/gorm"
)

type agentMemoryCandidate struct {
	Kind       string  `json:"kind"`
	Content    string  `json:"content"`
	Evidence   string  `json:"evidence"`
	Confidence float64 `json:"confidence"`
	ReplaceID  uint    `json:"replace_id"`
}

const agentMemoryInferencePrompt = `AGENT_MEMORY_INFER_V1
你是长期记忆归纳器，只返回 JSON {"candidates":[{"kind":"profile/preference/context","content":"简短稳定的归纳","evidence":"用户原话的连续片段","confidence":0.0,"replace_id":0}]}，最多两条。
从用户本人明确表达的稳定身份、职责、回答偏好、持续工作习惯中提炼可在未来会话使用的信息。对回答太长、希望更简短或详细的反馈可以归纳为表达偏好。也可以从用户重复纠正同一偏好推断，但单次纠正当前查询不代表长期偏好。没有可靠内容时返回空数组。
evidence 必须逐字摘自用户发言；不能引用助手、工具结果或业务数据库。content 不得包含近期金额、销量、日期、订单状态、临时需求、猜测的身份或系统权限。不能将本轮查询结果当记忆。不要记密码、密钥、验证码、联系人隐私或敏感数据。
已有记忆若表达同一偏好，返回空数组。只有新表达与已有自动归纳记忆冲突时填写 replace_id；不得覆盖手动添加或明确要求记住的内容。用户当前的明确说法优先。`

var agentMemoryDynamicPattern = regexp.MustCompile(`(?i)(\d{4}[-年/]\d{1,2}|[¥￥]|\d+(?:\.\d+)?\s*(?:元|块|件|单)|上月|本月|昨天|今天|上周|订单号|手机号)`)

func agentMemoryUserLines(recent []models.AIAgentMessage, question string) []string {
	lines := []string{}
	for index, item := range recent {
		if item.Role == "user" && index+1 < len(recent) {
			answer := recent[index+1]
			if answer.Role == "assistant" && answer.Status == "complete" && answer.ActionID == nil {
				lines = append(lines, strings.TrimSpace(item.Content))
			}
		}
	}
	lines = append(lines, strings.TrimSpace(question))
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	return lines
}

func agentMemoryShouldReflect(recent []models.AIAgentMessage, question string) bool {
	for _, cue := range []string{"我喜欢", "我偏好", "我更", "我希望", "我是", "我负责", "以后", "今后", "每次", "总是", "习惯", "默认", "不要", "通常", "一向", "我关心", "我关注", "我主要", "我只看", "我倾向", "太长", "太啰嗦", "简短点", "详细点", "先说结论", "直接给结论"} {
		if strings.Contains(question, cue) {
			return true
		}
	}
	if strings.Contains(question, "我说的是") || strings.Contains(question, "我指的是") || strings.Contains(question, "我问的是") {
		corrections := 0
		for _, item := range recent {
			if item.Role == "user" && (strings.Contains(item.Content, "我说的是") || strings.Contains(item.Content, "我指的是") || strings.Contains(item.Content, "我问的是")) {
				corrections++
			}
		}
		return corrections > 0
	}
	return false
}

func agentMemoryIsStandaloneStatement(question string) bool {
	question = strings.TrimSpace(question)
	for _, word := range []string{"？", "?", "多少", "查询", "查一下", "分析", "预测", "对比", "帮我", "查看", "看看", "怎么", "如何", "吗", "呢"} {
		if strings.Contains(question, word) {
			return false
		}
	}
	for _, prefix := range []string{"以后回答", "今后回答", "每次回答", "我喜欢", "我偏好", "我更喜欢", "我负责", "我是", "回答太长", "太长了", "太啰嗦", "简短点", "详细点", "先说结论", "直接给结论"} {
		if strings.HasPrefix(question, prefix) {
			return true
		}
	}
	return false
}

func agentMemoryEvidenceIsUserQuote(evidence string, lines []string) bool {
	evidence = strings.TrimSpace(evidence)
	if len([]rune(evidence)) < 3 || len([]rune(evidence)) > 240 {
		return false
	}
	count := 0
	for _, line := range lines {
		if strings.Contains(line, evidence) {
			count++
		}
	}
	if count == 0 {
		return false
	}
	if (strings.Contains(evidence, "我说的是") || strings.Contains(evidence, "我问的是") || strings.Contains(evidence, "我指的是")) && count < 2 {
		return false
	}
	latest := lines[len(lines)-1]
	if strings.Contains(latest, evidence) {
		return true
	}
	return count >= 2
}

func agentInferMemories(
	ctx context.Context,
	runtime aiProviderRuntime,
	owner string,
	question string,
	recent []models.AIAgentMessage,
) []models.AIAgentMemory {
	if owner == "" {
		return []models.AIAgentMemory{}
	}
	var state models.AIAgentMemoryState
	if err := config.DB.Where("owner = ?", owner).Find(&state).Error; err != nil {
		return []models.AIAgentMemory{}
	}
	if !state.ForgotBefore.IsZero() {
		eligible := []models.AIAgentMessage{}
		for _, message := range recent {
			if message.CreatedAt.After(state.ForgotBefore) {
				eligible = append(eligible, message)
			}
		}
		recent = eligible
	}
	if !agentMemoryShouldReflect(recent, question) {
		var turns int64
		sessions := config.DB.Model(&models.AIAgentSession{}).Select("id").Where("owner = ?", owner)
		err := config.DB.Model(&models.AIAgentMessage{}).Where("session_id IN (?) AND role = ? AND status = ? AND action_id IS NULL", sessions, "assistant", "complete").Count(&turns).Error
		if err != nil || turns == 0 || turns%4 != 0 {
			return []models.AIAgentMemory{}
		}
	}
	memories := []models.AIAgentMemory{}
	if err := config.DB.Where("owner = ?", owner).Order("updated_at DESC, id DESC").Limit(30).Find(&memories).Error; err != nil {
		return []models.AIAgentMemory{}
	}
	type existingMemory struct {
		ID      uint   `json:"id"`
		Kind    string `json:"kind"`
		Content string `json:"content"`
		Source  string `json:"source"`
	}
	existing := make([]existingMemory, 0, len(memories))
	for _, memory := range memories {
		existing = append(existing, existingMemory{ID: memory.ID, Kind: memory.Kind, Content: memory.Content, Source: memory.Source})
	}
	lines := agentMemoryUserLines(recent, question)
	input, _ := json.Marshal(struct {
		UserWords []string         `json:"user_words"`
		Existing  []existingMemory `json:"existing"`
	}{UserWords: lines, Existing: existing})
	callCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	profile := agentProfileFor("quick", "")
	profile.Tokens = 4096
	callCtx = context.WithValue(callCtx, agentTraceKey{}, &agentRunTrace{Profile: profile})
	content, err := callAIChat(callCtx, runtime, agentMemoryInferencePrompt, string(input), true)
	if err != nil {
		return []models.AIAgentMemory{}
	}
	var result struct {
		Candidates []agentMemoryCandidate `json:"candidates"`
	}
	if json.Unmarshal([]byte(cleanAIJSON(content)), &result) != nil {
		return []models.AIAgentMemory{}
	}
	saved := []models.AIAgentMemory{}
	for index, candidate := range result.Candidates {
		if index >= 2 {
			break
		}
		if !agentMemoryKind(candidate.Kind) || candidate.Confidence < 0.85 || !agentMemoryEvidenceIsUserQuote(candidate.Evidence, lines) {
			continue
		}
		if agentMemoryDynamicPattern.MatchString(candidate.Content) || agentMemoryDynamicPattern.MatchString(candidate.Evidence) {
			continue
		}
		if _, err := agentMemoryContent(candidate.Content); err != nil {
			continue
		}
		if _, err := agentMemoryContent(candidate.Evidence); err != nil {
			continue
		}
		memory, changed, err := saveInferredAgentMemory(owner, candidate)
		if err == nil && changed {
			saved = append(saved, memory)
		}
	}
	return saved
}

func saveInferredAgentMemory(owner string, candidate agentMemoryCandidate) (models.AIAgentMemory, bool, error) {
	memory := models.AIAgentMemory{}
	changed := false
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("owner = ? AND kind = ? AND content = ?", owner, candidate.Kind, candidate.Content).Limit(1).Find(&memory).Error; err != nil {
			return err
		}
		if memory.ID != 0 {
			return nil
		}
		if candidate.ReplaceID != 0 {
			if err := tx.Where("id = ? AND owner = ? AND source = ?", candidate.ReplaceID, owner, "inferred").First(&memory).Error; err != nil {
				return err
			}
			if err := tx.Model(&memory).Updates(map[string]any{"kind": candidate.Kind, "content": candidate.Content, "evidence": candidate.Evidence}).Error; err != nil {
				return err
			}
			memory.Kind = candidate.Kind
			memory.Content = candidate.Content
			memory.Evidence = candidate.Evidence
			changed = true
			return nil
		}
		var count int64
		if err := tx.Model(&models.AIAgentMemory{}).Where("owner = ?", owner).Count(&count).Error; err != nil {
			return err
		}
		if count >= agentMemoryLimit {
			return errors.New("记忆达到上限")
		}
		memory = models.AIAgentMemory{
			Owner: owner, Kind: candidate.Kind, Content: candidate.Content,
			Source: "inferred", Evidence: candidate.Evidence,
		}
		if err := tx.Create(&memory).Error; err != nil {
			return err
		}
		changed = true
		return nil
	})
	return memory, changed, err
}
