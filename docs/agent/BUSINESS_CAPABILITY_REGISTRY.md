# Agent 业务能力注册现状

## 现有旧 Agent 注册表

`erp-backend/controllers/ai_agent_tool_registry.go` 声明了旧聊天 Agent 工具的输入/输出概要、权限、风险、副作用、超时、幂等及审计元数据。`validateAgentToolPlan` 校验注册名称及部分参数；`agentExecuteTool` 是旧工具执行入口。聊天 API 要求管理员身份。已有能力包括受限数据查询、商品库存查询、利润/订单/员工信息读取、系统帮助以及待确认支出/订单草稿。

## AgentNative 能力

规则监测和任务处理由 `agentnative.MonitorService` 提供。管理工作台 Controller 当前直接查询 AgentNative 模型；`ReviewDraft` 是新增的内部草案审核服务。AgentNative 服务尚未以同一份可枚举的 Tool Registry 暴露给聊天 Agent；现有的 `agentToolRegistry` 元数据也尚未单一化到通用工具包。

## 本阶段边界

| 能力 | 当前情况 |
|---|---|
| 商品与库存只读查询 | `inventory` 工具已使用共享服务的商品库存查询；尚无完整商品状态、库存流水服务 |
| 销售区间与趋势分析 | 旧 Agent 通过既有查询规划和数据目录读取；补货上下文销量与 Monitor 已共用 `RetailReadService`，没有统一 `sales.get_product_sales` Tool |
| 监测信号查询 | 工作台 API 支持，聊天工具注册表未暴露 |
| 创建/获取补货草案 | Monitor Worker 可创建；聊天 Agent 未通过统一 Tool Registry 调用 |
| 任务/目标读取 | 工作台 API 支持，旧 Agent Registry 未暴露 |
| 审核内部补货建议 | 管理员 HTTP API 调用共享 MonitorService；需重新校验库存快照 |

所有写能力都必须由服务端校验参数和权限。模型输出不是授权凭证。注册表元数据不能替代真正的统一执行入口；统一入口改造仍是后续工作。
