# 第二阶段验证记录

日期：2026-10-08

## 已通过

- `go vet ./...`
- `go test ./middlewares ./agentnative ./routers`
- `npm run build`
- `node --test src/**/*.test.mjs`：31 项通过
- `go generate .`：Swagger 文档生成；工具对 Go 1.26 runtime 常量解析输出警告，但生成完成
- `git diff --check`
- `go test ./...`：在临时 MySQL 8 容器内全量通过。审计测试库为 `erp_audit_test`，AgentNative 集成库为 `erp_agent_native_test`，UI 测试库为 `erp_ui_test`；容器使用临时存储，验证后停止并自动移除。

路由契约测试确认新增草案审核接口已列入 Swagger；管理员接口员工 JWT 拒绝测试包含新审核路由。AgentNative MySQL 集成测试验证审核状态、修改数量、重复审核保护、聊天任务无信号关联/幂等键、过期租约恢复，以及库存恢复时保留已审核草案状态。RetailReadService 隔离 MySQL 测试验证有效订单销售量和可售库存口径，并确认 Agent 库存查询读取共享服务结果。

Playwright 在隔离 UI 数据库验证了管理员初始化/登录、AI 店长页面加载、规则扫描创建信号/任务/草案，以及修改数量和备注后确认草案。页面显示审核主体与活动日志；未产生采购订单或库存写入。

缺少 `JWT_SECRET` 时启动进程在数据库连接前以明确配置错误退出。

## 尚未通过或未运行

- 聊天回合与同一任务表的持久关联有数据库集成测试；浏览器中的聊天消息到任务 ID 跳转尚未通过真实 Provider 的端到端流程验证。
- 三个核心业务验收场景仍未全部完成：聊天触发补货分析全闭环、自然语言长期任务创建/暂停/重启，以及 Prompt 越权和 LLM 不可用降级。
- 未连接真实生产库、Provider 或采购系统。
