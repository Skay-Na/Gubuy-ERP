# Runtime 可靠性验收记录

日期：2026-10-11（Asia/Shanghai）

## 实际进程强制终止

```sh
ERP_AGENT_NATIVE_TEST_DSN='root:agentnative-test-only@tcp(127.0.0.1:33306)/erp_agent_native_test?charset=utf8mb4&parseTime=True&loc=Local' \
go test -count=1 ./agentnative -run '^TestWorkerProcessCrashRecoversPlannerAndIdempotentDraft$' -v
```

通过。测试启动独立 Go Worker 子进程，由父进程实际 `Kill`，再等待租约到期并启动恢复路径，覆盖：

- Planner 决策已经持久化、工具步骤刚开始前退出。
- 只读库存 SQL 查询在 MySQL 表锁下实际等待期间退出。
- 内部补货草案事务已经提交、工具步骤成功状态尚未提交时退出；恢复只保留一个草案。
- Scheduler 已提交周期 Run 和关联 Runtime 任务、尚未开始 Runtime 消费时退出；重启后只处理同一个 Run/任务。

测试未模拟断电、磁盘损坏、MySQL 本身崩溃或外部系统 exactly-once。

## 预算、取消和并发

```sh
ERP_AGENT_NATIVE_TEST_DSN='root:agentnative-test-only@tcp(127.0.0.1:33306)/erp_agent_native_test?charset=utf8mb4&parseTime=True&loc=Local' \
go test -count=1 ./agentnative -run 'Test(PlannerCallBudgetStopsAndPersistsPartialEvidence|WorkerCancellationPropagatesToPlannerAndStopsFurtherSteps)$' -v
```

通过。调用预算限制后，已有工具证据、Planner 调用、步骤与审计仍保存，任务状态为 `PARTIAL`，不会再发起 Planner 调用。后台 Worker 观察到持久取消状态后取消 Planner Context；工具步骤没有继续启动，最终任务状态仍为 `CANCELLED`。

`TestPersistentSchedulerIsDurableAndMultiWorkerSafe` 以两份独立服务实例并发竞争同一计划，唯一计划周期只有一条 Run 和一个链接 Runtime 任务。任务租约版本、写入 fencing 和 task-scoped 草案唯一键由集成与进程恢复测试覆盖。

`TestMaliciousToolOutputCannotGrantSchedulePermission` 将带指令的商品文本作为工具输出交给 Mock Planner，再验证 `schedule.cancel` 被授权边界拒绝且计划状态不变。

## 浏览器验证与剩余缺口

Playwright 的管理员工作台扫描了隔离测试商品：可售库存 1 件、近 30 天有效销量 10 件、规则阈值 3 件；统一 Runtime 持久化信号、读取补货上下文并创建建议 4 件的内部草案。管理员在 UI 确认内部计划后，工作台显示审核人和 `approved`；隔离库中商品库存仍为 1，未提交采购或资金交易。

自然语言计划创建/暂停/恢复、聊天 Planner 的成功和真实任务步骤都在同一页面完成。**没有在浏览器中取消正在运行的 Planner 任务**；取消 Context 的 Worker 集成测试已通过，API 审计事件实现已做代码检查，但该 UI 操作的 E2E 仍未完成。真实供应商和真实采购/支付始终未调用。
