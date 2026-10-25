# API 响应契约规范

> 状态：**渐进推广中**（T2 地基已落地，存量代码随后续服务层改造逐步替换，不做一次性全量改写）

## 统一响应形状

所有业务接口返回如下 JSON 包络：

```json
{
  "code": 200,
  "msg": "success",
  "data": { }
}
```

| 字段   | 类型   | 说明                                                                 |
| ------ | ------ | -------------------------------------------------------------------- |
| `code` | int    | 业务码，成功固定 `200`；失败与 HTTP 状态码保持同值                    |
| `msg`  | string | 提示语，成功时可自定义（前端会直接 `ElMessage.success(res.data.msg)`） |
| `data` | any    | 业务数据；无数据时省略（`omitempty`）                                  |

特殊形状（已存在的兼容约定，新增接口不得模仿）：

- 列表类接口可在顶层附加 `total` 字段（分页总数）。
- `GET /api/admin/status` 返回顶层 `initialized` 字段（历史遗留）。

## 业务码表

常量定义：`erp-backend/controllers/errcode.go`

| 常量               | 值   | 含义                                             |
| ------------------ | ---- | ------------------------------------------------ |
| `ErrCodeOK`        | 200  | 成功                                             |
| `ErrCodeBadRequest`| 400  | 参数错误 / 非法状态流转                          |
| `ErrUnauthorized`  | 401  | 未登录、登录态失效、凭证错误                     |
| `ErrForbidden`     | 403  | 已登录但无权限（如员工访问管理端资金接口）       |
| `ErrNotFound`      | 404  | 资源不存在                                       |
| `ErrDuplicated`    | 409  | 重复请求 / 幂等命中（前端有 duplicated 友好提示） |
| `ErrLimitExceeded` | 429  | 超出业务上限（如退款超过累计实付）或触发限流     |
| `ErrInternal`      | 500  | 服务端内部错误                                   |

## Helper 用法（新代码必须使用）

定义：`erp-backend/controllers/response.go`，响应形状由 `swagger_types.go` 的 `APIResponse` 承载。

```go
// 成功（默认 msg: "success"）
OK(c, gin.H{"token": token})

// 成功 + 自定义提示
OKWithMsg(c, "密码修改成功", nil)

// 失败：通用
Fail(c, http.StatusBadRequest, ErrCodeBadRequest, "参数错误")

// 失败：常用快捷方式
FailBadRequest(c, "参数错误")
FailUnauthorized(c, "旧密码错误")
FailForbidden(c, "无权限执行该操作")
FailNotFound(c, "未找到管理员账户")
FailDuplicated(c, "该请求已提交过")
FailLimitExceeded(c, "退款金额超过累计实付")
FailInternal(c, "数据库查询失败")
```

## 迁移策略

1. **本批次已应用**：`auth_controller.go`（登录/改密码）+ 本批次改动的文件。
2. 其余 300+ 处 `gin.H` **保持不动**，后续每次重构某个服务/控制器时顺带替换。
3. 替换时必须保持响应形状零漂移：`code`/`msg`/`data` 逐字段对齐，列表接口保留顶层 `total`。
4. 禁止在新代码中新增裸 `gin.H{"code": ...}` 写法（评审拦截项）。
