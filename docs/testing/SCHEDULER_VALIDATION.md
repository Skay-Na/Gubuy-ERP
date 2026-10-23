# 持久化 Scheduler 验收记录

日期：2026-10-11（Asia/Shanghai）

## 自动验证

```sh
ERP_AGENT_NATIVE_TEST_DSN='root:agentnative-test-only@tcp(127.0.0.1:33306)/erp_agent_native_test?charset=utf8mb4&parseTime=True&loc=Local' \
go test -count=1 ./agentnative -run '^Test(NextDailyRunResolvesDSTAndRejectsInvalidInput|PersistentSchedulerIsDurableAndMultiWorkerSafe)$' -v
```

通过。覆盖 IANA 时区校验、春季跳时、秋季回拨、计划时间修改、暂停期间不派发、恢复、取消、语义重复计划复用、服务对象重建后读取计划、owner-scoped 运行历史，以及两个独立 Scheduler 实例同时派发一个到期周期只生成一个 Run/`AgentWorkTask`。计划任务权限不含草案创建、订单写入、资金操作或计划管理权限。

单独进程崩溃窗口也通过 `TestWorkerProcessCrashRecoversPlannerAndIdempotentDraft` 验证：Scheduler 子进程提交唯一 Run 与 Runtime 任务后被父进程强制终止；新 Worker 从待执行任务恢复并完成同一 Run，数据库仅保留一个计划运行实体。

## 浏览器端到端

在隔离 MySQL 的真实页面上执行：

- 自然语言创建 **“每天帮我检查缺货商品”**。Planner 调用注册的 `schedule.create`，数据库保存唯一 `daily_inventory_check`，IANA 时区 `Asia/Shanghai`，`daily 09:00`。
- 自然语言执行 **“暂停我的每日库存检查”** 和 **“恢复之前的每日库存检查”**。Scheduler 记录状态切换；恢复后 UI 从 API 显示 `active`、09:00 和服务端计算的 `next_run_at`。
- 回归验证 Planner 操作完成后页面立即刷新计划列表，无需浏览器重载。
- 另一条隔离计划在真实分钟轮询窗口触发一次，数据库建立 `AgentScheduleRun` 并链接 Agent Runtime 任务；AIManager 历史显示本次 `PARTIAL`，因为当时 Provider 不可用，但确定性规则步骤已成功。结果明确展示 LLM 语义分析未完成。
- `schedule.history` 已注册为 owner-scoped 自然语言只读工具；本次浏览器通过 AIManager 历史按钮验证，没有单独通过聊天查询历史。

Browser 使用真实前后端和本地 Mock Provider（自然语言计划管理），不是实际外部模型。定时触发使用可控隔离库中的短窗口验证，没有等待 24 小时。

## 安全与限制

只实现并执行真实任务 `daily_inventory_check`。该计划执行库存只读规则，不会获得补货草案创建、采购、支付或改价权限。调度开关 `AGENT_PERSISTENT_SCHEDULER_ENABLED` 默认关闭；不会在升级时启用自动行为。未执行正式部署或生产迁移。
# 历史验证记录（功能已停用）

以下内容记录周期库存检查计划停用前的验证；它不代表当前系统仍提供计划创建、派发或执行能力。
