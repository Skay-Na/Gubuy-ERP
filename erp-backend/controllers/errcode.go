package controllers

// 统一业务错误码常量（T2 响应契约地基）。
// 约定：HTTP 状态码与业务码保持同值（除 200 成功外），前端拦截器按 code===401 做登录态收敛。
// 新代码必须引用这些常量，禁止在控制器里散落魔法数字；存量 gin.H 随服务层改造渐进替换。
const (
	// ErrCodeOK 成功
	ErrCodeOK = 200
	// ErrCodeBadRequest 参数错误
	ErrCodeBadRequest = 400
	// ErrUnauthorized 未登录 / 登录态失效 / 凭证错误
	ErrUnauthorized = 401
	// ErrForbidden 已登录但无权限（如员工访问管理端资金接口）
	ErrForbidden = 403
	// ErrNotFound 资源不存在
	ErrNotFound = 404
	// ErrDuplicated 重复请求 / 幂等命中（配合 duplicated 友好提示）
	ErrDuplicated = 409
	// ErrLimitExceeded 超出业务上限（如退款超过累计实付）或触发限流
	ErrLimitExceeded = 429
	// ErrInternal 服务端内部错误
	ErrInternal = 500
)
