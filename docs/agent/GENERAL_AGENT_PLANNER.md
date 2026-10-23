# 通用 Agent Planner

## 目标与边界

用户提交的只读分析通过 `AgentWorkTask` 汇入 `RuntimeExecutor`。主动库存监测与周期库存检查已停用。`AgentPlanner` 每次只返回一个受限 JSON 决策；Runtime 校验决策并把工具调用交给 `RuntimeExecutor.Execute`。Planner 不能访问 SQL、数据库句柄或未注册工具，也不能修改服务端权限。

任务输入只保存目标和服务端分配的能力范围。Planner 请求包含当前时间、允许的工具目录和已持久化的工具证据。模型输出的 `reason` 仅保存为简短决策摘要，不保存隐藏推理。

## 决策协议

- `CALL_TOOL`: 选择一项目录内工具；服务端再校验名称、参数 Schema 和权限。
- `FINISH`: 以已保存证据生成答案。
- `REQUEST_CLARIFICATION`: 将任务置为 `WAITING_CLARIFICATION` 并更新聊天回复。
- `REQUEST_APPROVAL`: 将任务置为 `WAITING_APPROVAL`；审批不自动执行高风险操作。
- `FAIL`: 记录失败状态和原因。

工具执行成功后结果写入 `AgentWorkTaskStep`，随后 Planner 接收新的证据并可调整下一步。进程恢复时 Runtime 从成功步骤重建证据；已完成步骤不会重新执行。

## Provider 与限制

`controllers.NewRuntimePlanner` 复用现有活动 Provider、密钥解密和 `callAIChat` HTTP 适配器。没有新增密钥存储。适配器限制 JSON 输出大小、拒绝未知字段和尾随 JSON，并沿用 Context 取消和请求超时。

默认每任务最多 8 次 Planner 调用、12 次工具调用、4 分钟、16,000 Token 估算/用量预算，单次结果最多 6 KiB、上下文最多 24 KiB、答案最多 3,000 个 Unicode 字符。费用价格未配置，因此费用记录为未知；不会伪造金额。

Provider 未配置或不可用时，复杂聊天分析明确失败。Planner 不会创建低库存提醒、补货建议或周期库存检查计划。

## 开关

- `AGENT_RUNTIME_ENABLED=1`: 启动持久 Runtime Worker。
- `AGENT_LLM_PLANNER_ENABLED=1`: 将现有 Provider 注入统一 Planner。
- 两个开关默认关闭。旧环境变量 `AGENT_PERSISTENT_SCHEDULER_ENABLED` 作为兼容项只启动 Worker，不运行计划任务。

## 验证

`go test ./agentnative` 的隔离 MySQL Mock Provider 集成测试验证了 Planner 根据不同库存证据选择不同的下一项工具，并持久化每个决策和结果。它不代表真实供应商模型调用验证。真实 Provider 未配置时，线上模型行为未验证。
