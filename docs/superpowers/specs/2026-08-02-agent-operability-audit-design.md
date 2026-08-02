# erpwonchon AI Agent 可操作性审计 — 设计文档

> 状态：草案 v1
> 日期：2026-08-02
> 作者：OpenCode
> 任务来源：`/Users/nanjiakai/Desktop/OpenCode_AI-Agent可操作性审计任务.md`

---

## 1. 目标

审计 erpwonchon 门店 ERP/POS 系统**是否可被 AI Agent 完整、可靠、可验证地操作**。
不是测试"人能不能点页面"，而是验证具备 API/工具调用能力的 Agent，能否在
不依赖人工猜测、屏幕坐标、隐含上下文的前提下，完成发现→理解→执行→验证→恢复的全闭环。

最终交付：中文审计报告，落盘到 `/Users/nanjiakai/Desktop/OpenCode_AI-Agent可操作性审计报告.md`。

## 2. 范围

### 2.1 包含

- 后端 60+ REST 端点（`/api/*`），覆盖三个鉴权层级（无鉴权 / JWT / Admin）
- 前端 9 个管理页面 + Portal + Staff Terminal 的"可发现性 + 稳定自动化入口"标注
- Swagger 文档一致性核对（`erp-backend/docs/swagger.{json,yaml,go}`）
- 5 个端到端 Agent 旅程：数据生命周期 / 配置→行为 / 财务闭环 / 导出 / 跨入口一致性
- 异常路径：参数非法、资源不存在、前置缺失、空数据、超时、重复提交、刷新重连
- 测试栈内所有数据完整清理

### 2.2 不包含（按任务定义）

- 渗透测试 / 漏洞扫描 / 权限绕过
- 真实支付、真实通知、真实外部副作用验证
- 改动任何源代码（含修复缺陷）
- 提交到 git 的工作区改动（除本设计文档外）

### 2.3 项目内不存在的层级（矩阵中标注 N/A）

- MCP / Tool 入口（项目无 MCP 服务）
- CLI（项目无独立命令行工具）
- 桌面 UI（无 Electron/Tauri 客户端）
- 异步任务 / WebSocket（routes 与 main.go 未发现）

## 3. 测试环境

### 3.1 栈启动

```bash
# 1. 拷贝临时 .env
cp .env.example .env.audit

# 2. 启动测试栈（APP=8080, DB=3307 默认）
docker compose --env-file .env.audit up -d --build

# 3. 等待 db healthy + app 启动
docker compose --env-file .env.audit ps
# 期望：db=healthy, app=running
```

如果默认端口被占用，回退方案：

```bash
sed -i '' 's/^APP_PORT=.*/APP_PORT=18080/' .env.audit
sed -i '' 's/^MYSQL_HOST_PORT=.*/MYSQL_HOST_PORT=13307/' .env.audit
```

### 3.2 端口与凭据

| 项 | 默认值 | 来源 |
|---|---|---|
| APP | `http://localhost:8080` | `.env.example` APP_PORT |
| DB host port | 3307 | `.env.example` MYSQL_HOST_PORT |
| DB user/pass | root / rootpassword | `.env.example` |
| DB name | erp_db | `.env.example` |
| JWT secret | `please_change_this_secret` | `.env.example` |
| Admin bootstrap | 通过 `/api/admin/init` 初始化 | `auth_controller.InitAdmin` |
| Staff PIN | seed 数据中固定 | `main.go seedData` |

### 3.3 测试数据命名约定

所有审计写入用统一前缀，便于清理：

- 员工：`audit_2026_08_02_emp_<seq>`
- 商品：`audit_2026_08_02_sku_<seq>`
- 分类：`audit_2026_08_02_cat_<seq>`
- 订单备注：`audit-2026-08-02-<seq>` 前缀
- 日志/流水无需标记，清理时按时间窗口 `>= 2026-08-02 audit start`

## 4. Agent 客户端

新建 Node 脚本 `audit/agent-client.mjs`（仅供审计期间使用，**不提交 git**）：

- 封装 `loginAdmin()` / `loginEmployee(pin)`：取 JWT
- 通用 `req(method, path, body)`：
  - 自动注入 `Authorization: Bearer ${token}`
  - 自动捕获 `status`, `body`, `headers`, `duration`
  - 失败时打印 request/response 原始内容
- 助手：`expect200(r)` / `expectField(r, key)` / `expectId(r)` / `cleanup(prefix)`
- 失败/异常分类：
  - `expected`（如 400/404/409）：断言级别
  - `unexpected`（5xx / 解析失败）：记录为缺陷

不引入任何第三方 HTTP 客户端（用 Node 18+ 内置 `fetch`）。

## 5. Agent 能力矩阵

### 5.1 矩阵构建方法

1. **枚举来源**：`routers/router.go` 的 60+ 路由 + `src/views/*` 9 个页面 + `src/router/index.js` 的路由表
2. **每行 = 1 个端点 / 1 个 UI 功能 / 1 个跨入口操作**
3. **每列**：
   - `Agent 可发现` — Swagger 描述、参数 schema、错误码文档
   - `可读取当前状态` — GET 接口或 UI 可读状态
   - `可执行` — POST/PUT/DELETE 接口或 UI 动作
   - `可验证结果` — 操作后可通过 GET 重新读取确认
   - `跨功能可用` — 关联数据/统计/导出是否一致
   - `可恢复` — 异常后能否判断 + 重试
   - `结论` — PASS / PARTIAL / FAIL / BLOCKED / N/A

### 5.2 端点分类与预期矩阵行数

| 模块 | 端点数 | UI 页面 | 矩阵行数估算 |
|---|---|---|---|
| 鉴权/Admin | 6 | 1 (Portal/Settings) | ~8 |
| 商品 | 6 | 1 | ~9 |
| 分类 | 4 | 1 | ~5 |
| 订单/收银 | 12 | 2 (Cashier/OrderManage) | ~16 |
| 入库/库存 | 6 | 2 (Inbound/InventoryLog) | ~10 |
| 员工 | 9 | 1 | ~11 |
| 财务/账户/补贴 | 11 | 2 (Finance/SubsidyManage) | ~14 |
| 看板/统计 | 1 | 1 | ~3 |
| 搜索 | 1 | - | ~1 |
| 导出 | 5 | - | ~5 |
| 备份/恢复 | 2 | 1 | ~3 |
| 系统拼音/杂项 | ~3 | - | ~3 |
| **合计** | **~66** | **~10** | **~88** |

### 5.3 端到端旅程

| 旅程 | 入口 | 期望 |
|---|---|---|
| A 数据生命周期 | REST | emp → cat → sku → inb → ord → del → emp/del；可发现→可读取→可执行→可验证→可清理 |
| B 配置→行为 | REST | 修改 admin pass → 重登 → 验证生效；改 JWT secret 后失效符合预期 |
| C 财务闭环 | REST | 下单 → 混合支付（现金+补贴+余额）→ 读取金额/状态 → 退款 → 核对财务汇总与流水 |
| D 导出完整性 | REST | 各 `*export` 端点下载并解析 → 验证行数 / 字段 / 一致性 |
| E 跨入口一致性 | REST + UI DOM | REST 写入 → 读取 UI 关键状态文本 → 验证可见 |

## 6. 缺陷分级与处理

按任务定义严重度：

- **阻断**：核心业务链路不可完成 / 无法确认关键结果
- **高**：可执行但可能重复创建/重复结算/状态错误
- **中**：依赖脆弱 UI / 隐含参数 / 不完整错误信息
- **低**：描述/反馈/可读性问题

**本审计只记录不修复**（用户确认）。
报告"已修复内容"章节为空，列出"Agent 接入改进清单"作为后续建议。

## 7. 报告结构

主报告路径：`/Users/nanjiakai/Desktop/OpenCode_AI-Agent可操作性审计报告.md`

1. 总结（分支/工作区/入口/命令/Pass 占比）
2. Agent 能力矩阵（每模块 1 张子表）
3. 端到端旅程结果
4. 缺陷清单
5. 已修复内容（本审计为空）
6. 验证结果（构建/类型检查/单测/API 闭环/UI smoke/数据清理）
7. Agent 接入改进清单

## 8. 执行步骤（高层）

1. 启动测试栈 → 健康检查
2. 构建 Agent 客户端
3. 阶段 0：接口侦察（已完成于设计阶段）
4. 阶段 1：能力矩阵构建
5. 阶段 2：可发现性 / 可理解性审计
6. 阶段 3：五步闭环核心功能
7. 阶段 4：端到端旅程
8. 阶段 5：失败恢复 / 自主性
9. 缺陷清单汇总
10. 数据清理
11. 报告撰写 → 落盘桌面
12. 关闭测试栈（保留 `.env.audit` 与 `audit/agent-client.mjs` 不提交）

## 9. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 端口冲突 | 回退到 18080/13307 |
| 现有 40+ uncommitted 改动影响 API 行为 | 审计基于"当前工作区"快照，矩阵在文档中标注"基线 = main + uncommitted" |
| 任务规模大 / 时间长 | 严格按矩阵行数预算，每模块结束立即更新报告 |
| 清理失败 | 在主报告"清理结果"章节显式列出未清理项 ID + 原因 |
| Docker 镜像未构建 | `docker compose up -d --build` 强制 rebuild |

## 10. 完成定义

- [ ] 主报告落盘到桌面且七节齐全
- [ ] 矩阵 88 行均有结论
- [ ] 5 个旅程均跑完五步闭环
- [ ] 缺陷清单每条有 ID/严重度/根因/复现/修复建议
- [ ] 测试数据清理记录存在（成功或显式记录残留）
- [ ] 验证章节所有 PASS/FAIL/BLOCKED 都有真实命令与真实输出

---

> 后续：用户审阅本设计 → 批准 → 调用 `writing-plans` 生成执行计划。
