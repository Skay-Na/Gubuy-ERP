# Agent-Native Retail OS：Phase 3–5 实施与验收报告

日期：2026-10-11（Asia/Shanghai）

## 结论

已在现有 Vue 3 + Gin/GORM + MySQL 单体中完成通用 Planner、持久化日程、统一 Runtime 恢复与主要集成验收。聊天分析、主动信号分析、计划任务都通过 `AgentWorkTask` 和统一 `RuntimeExecutor` 执行业务工具；库存规则扫描仍是确定性逻辑。所有新自动运行开关默认关闭。

核心能力通过隔离 MySQL、可控 Mock Provider、实际子进程强杀/恢复和浏览器流程验证。**真实外部 LLM、线上数据库迁移、生产部署和进行中任务的浏览器取消未验证。**没有连接或修改生产数据库，没有调用真实采购、支付或供应商服务。

## 1. 原架构与新架构

| 范围 | 原状态（见 `AGENT-RUNTIME-UNIFICATION-REPORT.md`） | 本次结果 |
|---|---|---|
| Agent 分析 | 固定工作流；无通用 Planner；聊天与后台路径有重复 | 新增结构化 `AgentPlanner` 决策循环；聊天、监测分析和周期任务可进入持久任务执行器 |
| 工具执行 | 4 项工具，权限检查不含逐工具参数 Schema | 固定服务端注册表，严格校验输入、能力范围及权限；所有模型工具调用走 `RuntimeExecutor` |
| 周期调度 | 进程内 24 小时 ticker，无用户级计划或历史 | MySQL 计划/运行记录，分钟派发、唯一计划周期、暂停/恢复/取消和历史 |
| 恢复 | 有租约，但无 fencing；无实际进程崩溃验收 | 租约版本 fencing、持久步骤/工具结果恢复、草案幂等核对，并由父进程实际杀死 Worker 验证 |
| 模型安全与可观测性 | 无通用调用预算/Planner 调用记录 | 限额、Provider/Model/延迟/Token 数据（若 Provider 提供）、错误分类和 Planner 调用持久记录 |

统一路径：

```text
聊天 / Monitor 信号 / Persistent Scheduler
                 ↓
         AgentWorkTask（MySQL）
                 ↓
    AgentPlanner → 校验 JSON 决策
                 ↓
 Tool Registry → RuntimeExecutor → RetailReadService / 内部草案服务
                 ↓
  步骤、证据、工具审计、任务结果持久化
```

主动监测仍先运行确定性低库存规则；发现新信号时可创建一次带去重键的动态分析任务。原有确定性补货流程继续作为独立规则路径，通过同一 RuntimeExecutor 执行允许的读取及内部草案工具。

## 2. 三阶段实际完成情况

### 阶段一：通用 Planner 与动态工具执行

- `AgentPlanner.Decide` 每次只返回一个 JSON 动作：`CALL_TOOL`、`FINISH`、`REQUEST_CLARIFICATION`、`REQUEST_APPROVAL` 或 `FAIL`。
- 模型只能从服务端注册表选择工具；服务端执行严格 JSON 解码、输入校验、工具权限检查及任务能力检查，然后交由 `RuntimeExecutor`。
- 工具成功结果作为持久化证据提供给下一次 Planner 调用。任务重启后从已成功步骤重建证据；已成功调用不会仅因重启而从头执行。
- Provider 适配器复用既有 AI 设置、密钥解密和 `callAIChat`；没有新增密钥或 Provider 配置系统。结构化 JSON 层适配未使用原生工具调用的现有路径。
- Planner 决策仅保留短理由摘要、动作、模型/Provider/用量元数据；不记录隐藏推理或密钥。
- 基于两组不同销售/库存事实通过 Mock Provider 验证下一步工具分支不同，见 `docs/testing/PLANNER_VALIDATION.md`。

### 阶段二：持久 Scheduler

- 新增 `AgentSchedule` 和 `AgentScheduleRun`。目前只开放确实有执行器的 `daily_inventory_check`。
- 使用 IANA 时区和每日时间，默认 `Asia/Shanghai 09:00`；数据库保存 UTC `next_run_at`。定义了夏令时跳时/回拨、有限错过窗口策略和更新时间后重算规则。
- 数据事务创建唯一的 `(schedule_id, planned_run_at)` 运行记录和关联 `AgentWorkTask`。多 Worker 对同一周期并发派发仅产生一条 Run/Task；服务重建后定义和历史仍可读取。
- 聊天支持创建、列表、历史、修改、暂停、恢复和取消；管理 API 与 Planner 工具均限定当前 owner。缺少时间使用明确默认时间；周期任务只有只读库存能力，不继承补货草案或订单、支付、改价权限。
- AIManager 新增长期任务列表、真实状态、下次/最近运行时间、暂停/恢复/取消、历史和关联任务入口。浏览器验证自然语言创建/暂停/恢复、自动刷新及一次实际分钟窗口触发。

### 阶段三：可靠性、安全及集成验证

- Runtime 租约递增 `LeaseVersion`，更新步骤/任务时校验当前租约，避免过期 Worker 覆盖新 Worker 状态。
- 取消状态持久化并记录活动；Worker 监视状态并向 Planner/工具传递 Context 取消。取消后不再开始后续步骤；同步数据库事务按数据库自身事务边界完成或回滚。
- 对同一任务草案使用唯一键和恢复核对处理“草案事务已提交、步骤状态尚未成功提交”的崩溃窗口。
- 独立子进程测试真实杀死 Worker，覆盖 Planner 决策保存后、只读 SQL 执行期间、草案提交后、Scheduler Run/Task 提交后四个窗口，再验证租约恢复和唯一性。
- 测试预算超限后的部分完成与证据保留、Context 取消、恶意工具输出、权限提升/非法工具/参数拒绝、owner 隔离和并发调度。
- 浏览器使用真实 Vue、Gin 后端、隔离 MySQL 与本地 OpenAI-compatible Mock Provider；完成销售/库存分析、工作台步骤、周期计划、库存信号与内部草案审核流程。

## 3. 关键文件与业务工具

- Planner 和持久执行循环：`erp-backend/agentnative/planner.go`、`planner_persistence.go`、`planner_tools.go`。
- Runtime 权限、执行、任务租约及恢复：`erp-backend/agentnative/tool_runtime.go`、`monitor.go`。
- Provider 适配：`erp-backend/controllers/agent_runtime_planner.go`。
- 销售/库存确定性读取：`erp-backend/services/retail_read.go`。
- Scheduler：`erp-backend/agentnative/schedule.go`、`controllers/agent_schedule_controller.go`。
- 数据模型：`erp-backend/models/agent_native.go`。
- 任务/计划 API：`erp-backend/controllers/agent_native_controller.go`、`routers/router.go`。
- 聊天和工作台：`src/components/AIChatAssistant.vue`、`src/components/agent/AgentSchedulePanel.vue`、`src/views/AIManager.vue`。
- 强杀恢复及集成测试：`erp-backend/agentnative/crash_recovery_integration_test.go`、`planner_integration_test.go`、`planner_reliability_integration_test.go`、`schedule_integration_test.go`。

已注册工具覆盖商品库存、补货上下文、销售汇总、时间范围销售趋势、销量/库存综合查询、监测信号、内部补货草案读取和创建、任务状态，以及周期计划 create/list/history/update/pause/resume/cancel。工具定义携带描述、输入/输出 Schema、所需权限、风险、超时、副作用、幂等与审计元数据。内部草案不是采购单，仍须人工审核。

本轮没有注册库存流水工具；销量按有效订单确定性聚合，库存是可售门店/云仓库存（不含样机）。系统仍为单店模型，`tenant_id=single` 不代表多租户隔离。

## 4. Provider、限制与成本

服务端默认每任务最多 8 次 Planner 调用、12 次工具调用、4 分钟、16,000 Token 预算、1 次无效/可重试输出重试、3,000 字符最终答案；工具返回最多 6 KiB、Planner 上下文最多 24 KiB、单次响应最多 8 KiB，工具调用受超时和行数限制。所有请求受 Context 超时/取消。

Provider 有 Token 用量时记录 Prompt/Completion 用量；没有时记录估算 Token 并标记 `token_usage_known=false`。调用记录含 Provider、Model、延迟、错误分类及任务关系。没有可验证价格配置，费用为未知，不显示伪精确成本。

Provider 不可用时，复杂语义分析会明确失败；每日库存计划可完成确定性规则扫描并以 `PARTIAL` 说明 LLM 不可用。不会用固定模板冒充模型推理。真实外部 Provider、供应商限流及真实 Token 计费路径本轮未验证。

## 5. 数据库兼容与迁移

新增 Planner 调用表、Scheduler/运行表及 AgentWorkTask 补充字段由现有 GORM AutoMigrate 以新增表/列/索引方式管理。周期执行唯一键为 `(schedule_id, planned_run_at)`，Planner 调用唯一键为 `(task_id, call_number)`；保留旧任务/步骤数据，不要求清空或重置数据库。

迁移只在临时 Docker MySQL 的隔离测试库执行过，未在生产库执行。回滚应用版本时保留新表和字段/数据，不直接删表；保持新 Feature Flag 关闭即可停用新自动行为。正式部署前应先备份，审查 GORM 实际 DDL、唯一索引与现存历史数据，再安排数据库变更窗口。

**审计时发现一段工作树中的旧数据迁移会把所有商品 `main_stock` 改成 0 并启用云仓履约。**本次已将它置于 `ERP_MIGRATE_UNIFIED_CLOUD_INVENTORY=1` 的显式开关之后，并在 `.env.example` 默认设为 `0`。它与 Agent Planner/Scheduler 无关；不要在未审核数据影响、备份及回滚方案前启用。此次未运行该迁移。

## 6. 真实测试命令与结果

所有数据库测试使用本机临时容器 `codex-agentnative-test-mysql`（端口 33306）及名字以 `_test` 结尾的隔离库。浏览器使用独立 `erp_browser_test`。没有连接 Compose 默认端口或生产数据库。

| 命令/流程 | 结果 |
|---|---|
| `ERP_AGENT_NATIVE_TEST_DSN=... ERP_RETAIL_READ_TEST_DSN=... ERP_AUDIT_DB=1 DB_NAME=erp_audit_delivery_test ... go test -count=1 ./...`（工作目录 `erp-backend`；三个 DSN/库名均为隔离 `_test` 库） | 通过；`agentnative`、`controllers`、`services`、`routers`、`middlewares` 及其他有测试包均通过 |
| `go vet ./...`（`erp-backend`） | 通过 |
| `npm run build` | 通过；有 Browserslist 数据过期提示和约 1.135 MB 路由 chunk 提示 |
| `node --test src/**/*.test.mjs` | 31/31 通过 |
| `go generate .`（`erp-backend`） | 成功；`swag`/Go 1.26 的 `runtime.mprof.go` 常量求值提示未阻止生成 |
| `git diff --check` | 最终文档和代码检查通过 |
| `TestPlannerDynamicallyBranchesOnIsolatedMySQLEvidence`（Mock Provider + 隔离 MySQL） | 通过：库存紧张与充足商品使用不同后续工具，并持久化决策/结果 |
| `TestPersistentSchedulerIsDurableAndMultiWorkerSafe` | 通过：两个服务实例竞争同一周期只生成一条计划运行及任务；覆盖时区、DST、owner 历史、修改、暂停和恢复 |
| `TestWorkerProcessCrashRecoversPlannerAndIdempotentDraft` | 通过：独立 Worker 被父进程实际 Kill 后恢复；草案保持唯一，计划 Run/任务不重复 |
| Planner 预算、取消传播、恶意工具输出/权限提升测试 | 通过：保留部分证据，不再开新步骤；取消传入 Planner Context；攻击性工具文本不能授予调度权限 |

命令日志、数据口径和各项具体断言见 `docs/testing/PLANNER_VALIDATION.md`、`SCHEDULER_VALIDATION.md` 和 `RUNTIME_RELIABILITY_VALIDATION.md`。Mock Provider 是可控协议测试，不是外部模型测试。

## 7. 浏览器验收事实

1. 管理员在聊天输入“分析最近两周销量下降但库存较多的商品”。隔离库商品近 14 天销量 0、前 14 天 10、可售库存 50。持久步骤依次查询销售趋势与库存/销量工具，聊天和工作台显示相同数据及分析结果；没有把相关性写成确定原因。
2. 聊天创建“每天帮我检查缺货商品”，数据库记录 owner、`daily_inventory_check`、`Asia/Shanghai`、09:00、`active`。自然语言暂停/恢复后，AIManager 计划状态无需页面重载即更新。
3. 计划在实际一分钟轮询窗口触发，产生唯一 Run 和关联 Runtime 任务。该次 Provider 不可用，所以工作台如实显示规则检查后 `PARTIAL`，没有报告模型分析成功。
4. 库存为 1、近 30 天销量 10 的隔离商品触发低库存信号；Runtime 创建建议数量 4 的内部草案。管理员在工作台审核确认后状态为 `approved`，库存仍为 1；没有创建真实采购单或付款。
5. 工作台通过后端 API 显示任务、步骤、周期和执行历史。AIManager 中的历史按钮已在浏览器验证；用聊天自然语言查询历史由 owner-scoped 工具和 intent 单测覆盖，但该具体句式未做浏览器 E2E。

## 8. 未完成、未验证与已知风险

- 未对真实外部模型进行调用验证；模型厂商 token 计量、限流及复杂输出故障尚未在真实供应商环境验收。
- 未在浏览器中取消一个正在进行的 Planner 任务；Worker 的持久取消检测、Context 取消和不再启动后续步骤由 MySQL 集成测试验证。
- 未实现库存流水 Planner 工具。
- 未测试主机断电、磁盘损坏、MySQL 自身故障，亦不承诺外部系统 exactly-once。
- Scheduler 的运行恢复和唯一派发在 MySQL 集成/子进程测试通过；部署拓扑、数据库高可用和真实跨主机网络分区尚未验证。
- 浏览器 E2E 由本地 Mock Provider 驱动；数据来自隔离测试商品，不能代表真实线上运营数据或生产模型质量。
- 全量销售分析数据规模、索引性能和生产负载上限未做压测。前端构建的 chunk 体积提示仍在。
- 继续沿用单店/单租户模型；不能以 `tenant_id=single` 作为隔离边界。
- 工作树包含其他已有文件修改和未跟踪的评估/Playwright 产物，本轮未清理或覆盖它们。

## 9. 部署开关与安全启用顺序

所有新行为默认关闭；示例配置参见 `.env.example` 和 `docker-compose.yml`：

- `AGENT_RUNTIME_ENABLED=1`：启动持久 Agent Worker（聊天任务及可用 Planner）。
- `AGENT_LLM_PLANNER_ENABLED=1`：复用现有已配置 Provider。需要 Runtime 或 Scheduler Worker 启动。
- `AGENT_PERSISTENT_SCHEDULER_ENABLED=1`：启动统一 Agent Worker 和持久 Scheduler，并替代旧 24 小时进程内库存扫描 ticker；不要与旧扫描重复启用。
- `AGENT_MONITOR_ENABLED=1`：仅旧兼容模式；当 Runtime/Scheduler 新路径启用时由 `main.go` 优先选择新路径。
- `ERP_MIGRATE_UNIFIED_CLOUD_INVENTORY=0`：保持旧库存数据重写关闭。

建议先在备份后的隔离/预发布库运行迁移、测试 Provider 和核对管理员权限；确认后由运维人员单独安排 Feature Flag 变更。单纯部署代码不会启用新 Planner 或周期扫描。此报告不代表已批准或执行生产迁移/部署。

## 10. 当前 UI 可试用的自然语言指令

以下能力由本地浏览器 Mock Provider E2E 及服务/集成测试支持；所有数据请求都使用已授权工具：

1. “分析最近两周销量下降但库存较多的商品。”——浏览器 E2E 已验证动态销售/库存工具组合。
2. “每天帮我检查缺货商品。”——浏览器 E2E 已验证计划持久化和默认时间展示。
3. “暂停我的每日库存检查。”——浏览器 E2E 已验证计划状态及工作台实时刷新。
4. “恢复之前的每日库存检查。”——浏览器 E2E 已验证恢复与下次运行时间更新。
5. “查看最近执行过的库存检查。”——计划历史工具、owner 权限及意图处理已有测试；聊天中此句的完整浏览器 E2E 尚未执行，可在当前配置的 Planner 下进一步验收。

周期检查只做库存读取/规则扫描；不代表允许自动采购、改价或付款。

## 11. 下一阶段建议

1. 在独立预发布环境接入获批的 Provider，验证真实结构化输出、限流/超时、用量和供应商策略。
2. 对数据库变更及 `ERP_MIGRATE_UNIFIED_CLOUD_INVENTORY` 旧迁移做数据负责人审查；给 AutoMigrate 新表/索引准备明确 DDL、备份和回滚说明。
3. 补充浏览器中进行中任务的取消/失败重试验收，以及聊天历史查询 E2E。
4. 基于目标生产数据规模审查销售聚合查询计划和索引，处理当前前端 chunk warning 后再评估性能。
5. 在扩展任务类型前先实现并验收对应确定性服务、权限范围与恢复测试；保持默认关闭的逐步启用方式。
