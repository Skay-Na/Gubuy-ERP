# Runtime 恢复、租约与取消

## 任务认领和 fencing

Worker 用条件更新认领 `CREATED` 任务或租约过期的 `RUNNING` 任务，同时递增 `LeaseVersion` 并设置 `LeaseUntil`。Planner 决策、步骤结果、最终状态等写入事务重新读取任务并检查状态、版本和未过期租约；过期 Worker 无法提交覆盖新 Worker 的状态。

每个工具步骤有唯一 `(task_id, sequence)` 和幂等键。成功工具输出持久化后可恢复上下文。内部补货草案在业务事务内按 task_id 唯一创建；若进程在副作用提交后、步骤状态保存前终止，恢复时可依据业务记录重建成功结果，而不再创建草案。

Scheduler 的周期唯一键为 `(schedule_id, planned_run_at)`；Run 与 AgentWorkTask 在同一数据库事务里创建。该保证针对本地数据库去重和恢复，不是外部系统 exactly-once 保证。

## 取消

取消入口持久化任务状态、聊天状态和取消活动。Worker 周期检查数据库取消状态；可取消操作通过子 Context 传播到 Planner HTTP 请求和支持 Context 的工具。取消后不再开始新步骤。数据库事务一旦进入无法安全中止的提交窗口，可完成或回滚当前事务后停止。

## 崩溃验收

`TestWorkerProcessCrashRecoversPlannerAndIdempotentDraft` 启动独立 Go 子进程，并由父进程实际杀死：

1. Planner 决策落库、工具步骤开始前。
2. 内部草案事务成功、工具步骤成功状态提交前。

父测试等待租约到期并重新处理任务，检查计划决策/步骤恢复和草案唯一性。这是真实子进程终止测试；它不模拟断电、磁盘损坏或 MySQL 故障。

## 运维排查

检查 `agent_work_tasks` 的 status/step/attempts/lease_version/error_code、对应 steps、`agent_tool_call_audits`、`agent_planner_calls`、`agent_activities` 和 `agent_schedule_runs`。过期租约重领记录恢复活动。敏感输入只保存摘要，定位 Provider 问题时结合错误分类和调用耗时。
