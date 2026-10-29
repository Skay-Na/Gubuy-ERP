package controllers

// 工程模块 · 控制器公共辅助
//
// 只放"取参数 / 格式化 / 事务外通用判断"这类无业务含义的工具，
// 业务算法一律在 services/engineering_service.go（纯函数、可单测）。

import (
	"erp-backend/services"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// engToday 业务"今天"（自然日零点，本地时区）。
// 逾期、日期上限校验统一用它，避免同一请求里多次 time.Now() 跨零点导致前后不一致。
func engToday() time.Time {
	now := time.Now()
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}

// engPageParams 分页参数：page 默认 1，page_size 默认 20、上限 100（契约 §1.5）
func engPageParams(c *gin.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.Query("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ = strconv.Atoi(c.Query("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return
}

// engBoolQuery 布尔 query 参数：1/true/yes 为真
func engBoolQuery(c *gin.Context, name string) bool {
	switch strings.ToLower(strings.TrimSpace(c.Query(name))) {
	case "1", "true", "yes", "y":
		return true
	}
	return false
}

// engIntQuery 整数 query 参数，未传或非法返回 0
func engIntQuery(c *gin.Context, name string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(c.Query(name)))
	return v
}

// engIntListQuery 逗号分隔的整数列表，如 status=1,2,3
func engIntListQuery(c *gin.Context, name string) []int {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil
	}
	var out []int
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if v, err := strconv.Atoi(part); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// engTrimKeyword 关键词截断（契约上限 50 字，按字符而非字节，避免中文被切坏）
func engTrimKeyword(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) > 50 {
		r = r[:50]
	}
	return string(r)
}

// engLikePattern 构造 LIKE 模式并转义 % _ \ ，防止用户输入 % 把全表捞出来
func engLikePattern(keyword string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return "%" + replacer.Replace(keyword) + "%"
}

// engIDParam 解析路径参数中的正整数 ID；非法时已写好 400 响应，调用方直接 return
func engIDParam(c *gin.Context, name string) (uint, bool) {
	v, err := strconv.ParseUint(strings.TrimSpace(c.Param(name)), 10, 64)
	if err != nil || v == 0 {
		FailBadRequest(c, "ID 参数不正确")
		return 0, false
	}
	return uint(v), true
}

// engRuneLen 按字符计长度（中文按 1 个字，不按字节）
func engRuneLen(s string) int { return len([]rune(s)) }

// engDatePtr 日期字段输出：nil 出 null，否则 YYYY-MM-DD
func engDatePtr(t *time.Time) *string {
	return services.FormatBusinessDate(t)
}

// engDateTime 时间戳字段输出：nil 出 null，否则 YYYY-MM-DD HH:mm。
// 结清时间用这个而不是 engDatePtr —— 原样吐 time.Time 会被序列化成
// 2026-10-29T22:26:25.520+08:00，前端「关键日期」区块里和其他日期格式不一致。
func engDateTime(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	s := t.Format("2006-01-02 15:04")
	return &s
}

// engConflictError 业务冲突（平账不平、并发抢改、幂等命中）。
// 用独立类型而不是靠字符串匹配，避免以后改文案就丢了 409 语义。
type engConflictError struct{ msg string }

func (e engConflictError) Error() string { return e.msg }

// engConflict 构造 409 类业务冲突
func engConflict(msg string) error { return engConflictError{msg} }

// engIsConflictErr 判断是否为冲突类错误（409）；其余一律 400
func engIsConflictErr(err error) bool {
	if err == nil {
		return false
	}
	var ce engConflictError
	if errors.As(err, &ce) {
		return true
	}
	// 兜底：历史文案（避免遗漏点导致语义漂移）
	msg := err.Error()
	return strings.Contains(msg, "不一致") || strings.Contains(msg, "相差") ||
		strings.Contains(msg, "已变化") || strings.Contains(msg, "已登记") || strings.Contains(msg, "已记录")
}
