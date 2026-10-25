# ERP Agent 服务（pi 核心）

基于 [earendil-works/pi](https://github.com/earendil-works/pi)（@earendil-works/pi-agent-core + pi-ai）重建的 ERP Agent 执行层，替代旧的 Go `agentnative` JSON 计划协议。

## 与旧版的核心差异

| | 旧版（Go agentnative） | 新版（pi 核心） |
|---|---|---|
| LLM 交互 | 自定义 JSON 决策协议，每轮只回一个动作 | 原生 tool-calling，完整消息转录逐轮回传 |
| 流式 | 无（阻塞最长 270s） | SSE 流式（逐字输出 + 工具进度） |
| 上下文 | 截断的 evidence 摘要（≤24KB） | 完整对话史 |
| 工具护栏 | 服务端注册表 + 权限 | pi AgentTool schema 校验 + beforeToolCall 钩子 |

## 架构

```
Vue 前端 ──/api/ai/agent/*──▶ 本服务（8082，Fastify + SSE）
                               ├─ pi Agent loop（@earendil-works/pi-agent-core）
                               ├─ pi-ai openai-completions 适配器（DeepSeek/千问/MiMo/MiniMax…）
                               ├─ 读工具 ──透传 JWT──▶ Go 后端 /api/ai/agent-tools/*（8081）
                               └─ 草稿/记忆/会话 ──▶ 共享 MySQL（复用 ai_agent_* 表）
```

- **记账写入不直连**：支出草稿确认时本服务先原子占用草稿，再透传用户 JWT 调 Go `/api/bookkeeping/entries`，失败自动回滚草稿。
- **订单草稿**：前端确认时直接走原有 `/api/orders`（携带 `agent_action_id`）。

## 启动

开发栈已集成（scripts/dev.ps1）：`scripts\dev.cmd start` 会同时启动 Go API、Vite 和本服务（日志在 `logs/dev-stack/agent.*.log`）。

单独启动：

```bash
cd agent-service
cp .env.example .env   # 按需修改
npm install
npm run dev            # 或 npm start
```

环境变量见 `.env.example`。`JWT_SECRET`、`AI_CONFIG_SECRET` 必须与 Go 后端一致（AI 密钥解密共用同一套 AES-256-GCM 口径）。

## 工具清单

| 工具 | 数据来源 |
|---|---|
| query_product_inventory / sales_summary / sales_trend / inventory_sales | Go `services/retail_read.go`（经 /api/ai/agent-tools/*） |
| query_unpaid_orders | Go 直查（未结清有效订单） |
| create_order_draft / create_expense_draft | 写 ai_agent_actions（pending，24h 过期，一次性） |
| save_long_term_memory | 写 ai_agent_memories |

## 冒烟测试

不调真实 LLM，用 pi faux provider 编排工具调用，验证 loop→工具→Go 全链路：

```bash
npx tsx scripts/smoke-pi-loop.ts   # 需 Go 后端已在 8081 运行
```

## 已知事项

- **AI 密钥加密口径**：与 Go 后端完全一致（`sha256("erpwonchon-ai-provider-key-v1:" + secret)`，AES-256-GCM，nonce 前置）。若历史密钥是以旧的加密密钥保存的，两套服务都会解不开——在系统设置重新保存一次 API Key 即可。
- 附件支持图片（作为视觉输入）、纯文本与 PDF（抽文本走上下文，单附件上限 20000 字符，超出置 `truncated` 标记）。
- 旧 Go agent 的定时任务子系统（agentnative/scheduler，`AGENT_RUNTIME_ENABLED` 门控）未被替换，保持原样；如需迁移到本服务再做。

## 与旧 agentnative 的关系

- **路由切换**：旧 Go `/api/ai/agent/*` 会话/消息/记忆/草稿接口已从 `routers/router.go` 下线，由本服务（:8082）按相同路径承接；Go 侧仅保留 `/api/ai/agent-tools/*` 5 条只读数据工具（供本服务透传 JWT 调用）。
- **定时任务定性**：旧 `agentnative` 定时任务子系统（库存告警、补货扫描等持久调度）**默认关闭、不随 pi 迁移**——仍由 `AGENT_RUNTIME_ENABLED` / `AGENT_PERSISTENT_SCHEDULER_ENABLED` 环境变量门控，生产保持 0。旧"AI 店长"工作台路由（`/api/ai/manager/*`）已改为通配 404 占位。后续如有真实的定时扫描需求，再评估在本服务内以轻量调度重新实现；在那之前不做任何迁移。
- **数据兼容**：`ai_agent_sessions` / `ai_agent_messages` / `ai_agent_memories` / `ai_agent_actions` 表结构与旧版一致，历史数据无需清洗；`agent_transcripts` 为本服务新增的完整转录存档。
- 本服务复用 `ai_agent_*` 表，历史会话记录可直接继续使用；完整 pi 转录存 `agent_transcripts` 表。
