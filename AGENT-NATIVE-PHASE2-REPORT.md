# Agent-Native Retail OS 第二阶段报告

日期：2026-10-08

## 摘要

本轮完成 JWT 默认密钥移除、AI 店长 API 管理员边界复核、内部补货草案的人工审核后端和工作台交互，并更新 Swagger。普通聊天回合现在写入与监测任务相同的 `AgentWorkTask` 表，聊天消息可携任务 ID 跳转工作台；Agent 草稿确认也会推进关联任务状态。完整目标**未完成**：旧聊天 Agent 与 Monitor 的工具执行仍分离，没有多步骤后台执行/恢复，周期任务没有数据库配置或暂停/恢复能力。

## 完成功能和主要文件

- `erp-backend/middlewares/jwt_middleware.go`：删除内置密钥回退，增加显式密钥配置与最低长度检查。
- `erp-backend/main.go`：在数据库启动前校验 JWT 配置。
- `.env.example`、`docker-compose.yml`、`README.md`：不再提供可启动的公开默认值，并说明密钥更换导致会话失效。
- `erp-backend/agentnative/draft_review.go`：事务化审核、状态幂等、库存快照冲突检测和活动审计；持久化聊天任务。
- `erp-backend/services/retail_read.go`：商品库存搜索与补货上下文读取共享服务；旧 Agent 与 Monitor 复用该服务。
- `erp-backend/models/agent_native.go`：补充审核人、备注、时间字段；由现有 AutoMigrate 兼容添加。
- `erp-backend/controllers/agent_native_controller.go`、`routers/router.go`、`docs/*swagger*`：管理员草案审核接口和 Swagger 契约。
- `src/views/AIManager.vue`：查看草案状态、调整建议数量、确认内部计划或拒绝并留备注。

## 服务与 Agent 工具情况

旧 Agent 工具规划通过 `agentToolRegistry` 校验并由 `agentExecuteTool` 分发；商品库存检索和 Monitor 补货上下文现在复用 `RetailReadService`。一般销售分析、库存流水、入库事务等仍有 Controller 直连路径。Monitor 是独立的 AgentNative 服务；其和聊天 Registry 未统一。工具注册仍没有覆盖监测信号、持久任务、经营目标及补货草案读写的统一契约。

聊天消息和会话运行记录在保存事务中关联 `AgentWorkTask`，消息携任务 ID，工作台展示聊天任务的结构化目标和答复。Agent 草稿等待确认时任务进入 `WAITING_APPROVAL`，确认后状态更新且记录活动。该任务在整个同步 LLM 回合结束后创建，不能展示实时步骤进度，也没有后台恢复执行能力；Monitor Worker 与旧 Agent 仍未共享工具执行器。因此“聊天消息联动持久任务”已实现基础版，“聊天触发补货分析全闭环”和“跨模块动态工具组合”未验收。

## 生命周期、调度、信号与草案

第一阶段已有规则扫描、每日低库存信号去重、持久任务租约、过期租约回收及 TaskID 唯一的补货草案。`AGENT_MONITOR_ENABLED=1` 时当前进程运行 24 小时 ticker；任务重启恢复来自持久任务，而调度配置本身仍非持久化。用户无法在聊天创建或暂停周期计划，也没有通用多步骤审批等待流程。

本轮新增的草案审核只对内部建议生效。管理员确认/拒绝会持久化状态和审核主体；审批时如果库存快照变化，草案会变为 `stale`。没有新增正式采购单、销售订单、付款或库存变更。

## 安全和兼容性

生产启动必须显式配置至少 32 字节的 `JWT_SECRET`。上线前要安全生成并配置私有值；若过去使用内置公开密钥，切换后所有已有 JWT 都需重新登录。禁止在日志或文档中记录密钥。AI 会话、Provider 设置、经营工作台和新审核端点限定管理员；员工 `emp_` 前缀被拒绝。单店数据模式未变化，没有宣称已实现多租户。

## 测试结果和限制

通过：`go vet ./...`、临时 MySQL 隔离环境内 `go test ./...`、`npm run build`、`node --test src/**/*.test.mjs`（31 项）、Swagger 生成及 diff 空白检查。Playwright 验证了隔离环境管理员登录、规则扫描展示草案、调整数量与备注并确认，操作历史显示审核人。测试容器已停止并自动移除。

三项指定验收场景仍未全部完成：场景一聊天触发任务闭环、场景二 Agent 信号分析链路、场景三长期任务创建/暂停/恢复。当前验证覆盖场景二中的规则扫描和人工审核子流程，以及第一阶段租约恢复用例；不应把部分路径描述成完整场景通过。

## 风险和生产部署

- 需先将 `JWT_SECRET` 注入部署环境；Compose 变量现在不设默认值，缺失时会报错。
- 业务服务抽取和统一工具执行入口未完成，AgentNative 与旧聊天工具仍有不同调用路径。
- 定期调度配置、暂停/取消 API、工作流步骤和通用审批队列未实现。
- 库存冲突的草案只被标记过期，缺少一键重新计算流程。
- 采购订单、供应商、预算、交付周期和支付都不属于现有草案能力。

## 能力对照

| 用户需求 | 是否实现 | 对应代码 | 验证结果 |
|---|---|---|---|
| Agent 分析库存 | 部分：旧 Agent 有受限库存查询，尚无补货专用多工具闭环 | `erp-backend/controllers/ai_agent_engine.go`、`ai_agent_tool_registry.go` | 工具单测可运行；场景未验收 |
| Agent 自动发现问题 | 已有规则监测；没有 LLM 信号分析融合 | `erp-backend/agentnative/monitor.go` | 第一阶段隔离 MySQL 记录在既有文档；本轮隔离重跑待完成 |
| Agent 创建长期任务 | 未实现自然语言创建/暂停的持久周期配置 | `erp-backend/agentnative/scheduler.go` | 未验证 |
| Agent 创建补货草案 | 规则 Worker 可以创建；聊天 Agent 未统一调用 | `erp-backend/agentnative/monitor.go` | 既有幂等实现；本轮集成重跑待完成 |
| 管理员审核草案 | 已实现内部确认、拒绝、调整数量、冲突检测和审计 | `erp-backend/agentnative/draft_review.go`、`src/views/AIManager.vue` | 代码/权限契约已测；数据库和浏览器尚待验证 |
| 任务中断恢复 | 监测任务租约恢复已实现；聊天回合是同步处理，没有多步骤恢复 | `erp-backend/agentnative/monitor.go`、`agentnative/draft_review.go` | MySQL 隔离测试验证租约和聊天任务幂等键；聊天执行恢复未验收 |
| Agent 跨模块调用 | 旧 Agent 有限工具规划；商品库存查询共享服务已接通，仍未统一 Registry/全业务服务层 | `erp-backend/controllers/ai_agent_tool_registry.go`、`erp-backend/services/retail_read.go` | 共享读取集成测试通过；通用跨模块闭环未完成 |

## 下一阶段建议

先在隔离 MySQL 和浏览器中完成草案审核验证；随后抽取共享商品/库存/销售服务，让 Monitor 与聊天工具共用。再定义统一工具执行器和持久步骤任务，把聊天入口、监测入口、审批等待和工作台任务详情关联起来。最后建设数据库持久化周期配置与多 Worker 安全认领，并按三个验收场景执行故障恢复验证。
