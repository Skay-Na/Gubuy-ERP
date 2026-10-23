# 持久化 Scheduler（已停用）

低库存检查计划已从产品中移除。应用不再启动计划派发器，Agent 页面没有计划管理入口，相关 HTTP 路由和 Planner 工具也已停用。

为保留备份兼容性，旧的 `agent_schedules`、`agent_schedule_runs` 表及其历史行仍保留。`AGENT_PERSISTENT_SCHEDULER_ENABLED=1` 只作为旧部署配置兼容项启动持久任务 Worker；不会派发或执行库存检查计划。历史调度测试文档只记录停用前的验证结果，不表示当前仍支持该功能。
