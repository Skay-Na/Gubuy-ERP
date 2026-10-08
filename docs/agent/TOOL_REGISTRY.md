# Tool Registry 设计与现状

旧 Agent 工具已收录到 `controllers/ai_agent_tool_registry.go`，每项声明名称、描述、输入/输出契约、管理员权限、风险、副作用、超时、幂等说明和审计要求；执行前会拒绝未注册工具与一部分畸形/过大参数。实际工具包括 `data_catalog`、`data_query`、`customer_orders`、`profit`、`inventory`、`unpaid_orders`、员工查询、`system_help`、`draft_expense` 和 `draft_order`。只读数据查询继续使用数据集/字段白名单；写入仍经草稿与服务端确认。

**限制：** 元数据中的超时和输出 schema 尚未全部成为通用运行时强制约束；注册表仍位于旧 Agent 控制器内，未与新增经营任务处理器合并。不能宣称工具平台已全面完成。

目标注册信息：

| 工具 | 权限 | 风险 | 副作用 | 幂等/审计 |
| --- | --- | --- | --- | --- |
| `inventory.read` | 指定店铺读取 | LOW | 无 | 查询追踪 |
| `sales.read` | 指定店铺读取 | LOW | 无 | 查询追踪、限制时间与行数 |
| `replenishment.suggest` | 店长分析 | LOW | 内部草案 | 按 signal_id 幂等、活动日志 |
| `purchase.submit` | 审批后的指定权限 | HIGH | 内部正式采购订单 | 一次性审批、版本重验、幂等键、审计 |
| `purchase.send` | 单独授权 | HIGH | 对外发送 | 待供应商集成与恢复策略 |
| `payment.execute` | 禁用 | CRITICAL | 资金变动 | 本轮不启用 |

这些是目标契约，不应将未实现的条目暴露给模型调用。模型返回工具名称不会产生权限，服务端必须以认证上下文和当前授权重新验证。
