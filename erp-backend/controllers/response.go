package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 统一响应 helper（T2 响应契约地基）。
// 规范见 docs/api-conventions.md：所有新代码必须使用 OK/Fail 系列 helper，
// 存量 300+ 处 gin.H 随后续服务层改造渐进替换，不做一次性全量改写。

// OK 成功响应：HTTP 200 + 业务码 200，data 可为 nil（omitempty 时省略字段）
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{Code: ErrCodeOK, Msg: "success", Data: data})
}

// OKWithMsg 带自定义提示语的成功响应（与前端 ElMessage.success(res.data.msg) 配合）
func OKWithMsg(c *gin.Context, msg string, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{Code: ErrCodeOK, Msg: msg, Data: data})
}

// Fail 失败响应：httpStatus 为 HTTP 状态码，code 为业务码，两者通常保持一致。
// 例如：Fail(c, http.StatusBadRequest, ErrCodeBadRequest, "参数错误")
func Fail(c *gin.Context, httpStatus int, code int, msg string) {
	c.JSON(httpStatus, APIResponse{Code: code, Msg: msg})
}

// FailBadRequest 参数校验失败（HTTP 400 / code 400）
func FailBadRequest(c *gin.Context, msg string) {
	Fail(c, http.StatusBadRequest, ErrCodeBadRequest, msg)
}

// FailUnauthorized 未授权（HTTP 401 / code 401）
func FailUnauthorized(c *gin.Context, msg string) {
	Fail(c, http.StatusUnauthorized, ErrUnauthorized, msg)
}

// FailForbidden 无权限（HTTP 403 / code 403）
func FailForbidden(c *gin.Context, msg string) {
	Fail(c, http.StatusForbidden, ErrForbidden, msg)
}

// FailNotFound 资源不存在（HTTP 404 / code 404）
func FailNotFound(c *gin.Context, msg string) {
	Fail(c, http.StatusNotFound, ErrNotFound, msg)
}

// FailDuplicated 幂等命中/重复请求（HTTP 409 / code 409）
func FailDuplicated(c *gin.Context, msg string) {
	Fail(c, http.StatusConflict, ErrDuplicated, msg)
}

// FailLimitExceeded 超出上限/限流（HTTP 429 / code 429）
func FailLimitExceeded(c *gin.Context, msg string) {
	Fail(c, http.StatusTooManyRequests, ErrLimitExceeded, msg)
}

// FailInternal 服务端内部错误（HTTP 500 / code 500）
func FailInternal(c *gin.Context, msg string) {
	Fail(c, http.StatusInternalServerError, ErrInternal, msg)
}
