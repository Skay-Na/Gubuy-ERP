# 运行与恢复

- `AGENT_MONITOR_ENABLED` 默认为 `0`。开启后运行规则扫描与持久任务处理；不需要 LLM 密钥。管理员可手动扫描。
- 新增表由项目启动时的 GORM AutoMigrate 添加：`agent_signals`、`agent_goals`、`agent_work_tasks`、`agent_replenishment_drafts`、`agent_activities`。部署前备份数据库与 Provider 配置，在隔离 MySQL 验证迁移。不要清空 MySQL 卷。
- 重启后 `CREATED` 和租约过期的 `RUNNING` 任务会在下一次调度/手动扫描中重新处理。草案 `task_id` 唯一，信号指纹唯一。失败任务最多尝试 3 次；达到上限需人工排查，当前没有管理端重试/取消 API。
- 监测错误写入应用日志；任务和活动可从工作台查询。没有统一 outbox、告警通知、集中指标或成本预算统计。未来应补齐。
- 不要在生产数据库运行测试。集成测试只读取 `ERP_AGENT_NATIVE_TEST_DSN` 且要求库名以 `_test` 结尾；现有控制器测试使用 `ERP_AUDIT_DB=1` 并会清理该测试库业务表。
- 回滚应用版本时保留新增表，旧版本不会读取它们。删除这些表是破坏性操作，本轮不提供自动回滚删除。
