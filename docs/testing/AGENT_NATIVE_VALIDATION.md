# Agent Native 验证记录（2026-10-08）

## 已执行

- `npm run build`：通过。
- `node --test src/**/*.test.mjs`：31/31 通过。
- `go generate .`：Swagger 生成完成；工具报告一个 Go runtime 常量解析警告，未阻止生成。
- 隔离的临时 MySQL 8 容器：`ERP_AGENT_NATIVE_TEST_DSN=.../erp_agent_native_test`、`ERP_AUDIT_DB=1 DB_NAME=erp_audit_test` 下运行 `go test ./...`：通过。集成测试覆盖规则阈值、信号去重、草案幂等、过期租约恢复、库存恢复关闭信号，以及不创建业务订单/不改库存。
- `go vet ./...`、`git diff --check`：通过。
- 新管理 API 未授权/员工 JWT 拒绝测试：由路由测试覆盖。
- 既有 `controllers` 回归测试在隔离审计库通过；记账与 Agent 确认路径重构由该套件覆盖。
- Playwright CLI 在临时 MySQL 容器中的 `erp_ui_test` 上完成浏览器流程：首次初始化管理员并登录后进入 `/admin/ai-manager`；插入隔离测试商品和有效订单后，手动扫描显示 1 个信号、1 个任务和 1 个待核对草案（建议 4 件）；重复扫描仍为 1 个；目标成功保存；提高测试库存后再次扫描，待关注信号和待核对草案均为 0，活动记录显示关闭。仅用于隔离测试。

## 未验证

- 没有对真实生产数据库、真实供应商或真实采购订单操作。
- 没有采购审批、正式采购、收货关联的完整端到端测试，因为相关业务能力尚未实现。
- 未开展真实多租户隔离、并发高负载、长时间调度与故障注入验证。现有库是单店模型。
