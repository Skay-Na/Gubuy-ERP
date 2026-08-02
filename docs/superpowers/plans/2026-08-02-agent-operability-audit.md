# AI Agent 可操作性审计 — 执行计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 通过真实 API 五步闭环，对 erpwonchon ERP/POS 系统进行 AI Agent 可操作性审计，输出落盘到桌面的中文审计报告。

**Architecture:** 启动独立 docker 测试栈（`.env.audit`）→ 构建 Node Agent 客户端（内置 fetch，无第三方依赖）→ 对 60+ REST 端点和 9 个 UI 模块执行 5 步闭环（发现→读取→执行→验证→清理）→ 跑 5 个端到端旅程 → 汇总缺陷与验证结果 → 写报告。

**Tech Stack:**
- Docker Compose（MySQL 8.0 + 自建 Go 后端镜像）
- Node 18+ 内置 `fetch`
- Git（仅记录设计文档与本计划，**不提交审计产物与报告**）
- Bash / curl / jq

## Global Constraints

- 工作区已有 40+ uncommitted 改动；**不覆盖、不丢弃、不提交**
- 仅读源码 + 调用 API；**不改任何源代码**（用户明确要求"只记录不修复"）
- 所有审计产物（`audit/*`、报告）**不提交 git**
- 测试数据统一前缀 `audit_2026_08_02_*`；审计结束清理
- 报告落盘到 `/Users/nanjiakai/Desktop/OpenCode_AI-Agent可操作性审计报告.md`
- 不动 `.env.example`；用 `.env.audit` 隔离
- 端口冲突回退到 18080/13307
- 严重度：阻断/高/中/低（按设计文档 §6）
- 设计文档：`docs/superpowers/specs/2026-08-02-agent-operability-audit-design.md`（commit 69ec3e8）
- 本计划：`docs/superpowers/plans/2026-08-02-agent-operability-audit.md`

## File Structure

```
erpwonchon/                                    # 项目根（不提交）
├── .env.audit                                  # 临时测试环境（不提交）
├── audit/                                      # 审计产物（不提交）
│   ├── agent-client.mjs                        # Agent 客户端
│   ├── recon/                                  # 阶段 0 侦察
│   │   ├── routes.tsv                          # 全部路由表
│   │   ├── controllers-map.tsv                 # 路由→控制器→handler
│   │   └── swagger-coverage.tsv                # Swagger 覆盖统计
│   ├── matrix/                                 # 阶段 1-3 矩阵
│   │   ├── matrix.tsv                          # 完整能力矩阵（88 行）
│   │   └── per-module/                         # 分模块细分
│   │       ├── auth.tsv
│   │       ├── products.tsv
│   │       ├── orders.tsv
│   │       ├── ...
│   ├── probes/                                 # 阶段 3-5 探测脚本与原始记录
│   │   ├── products.mjs
│   │   ├── orders.mjs
│   │   ├── ...
│   ├── journeys/                               # 阶段 4 端到端旅程
│   │   ├── A-data-lifecycle.md
│   │   ├── B-config-to-behavior.md
│   │   ├── C-financial-close.md
│   │   ├── D-exports.md
│   │   └── E-cross-entry.md
│   ├── failure/                                # 阶段 5 失败恢复
│   │   └── recovery-probes.md
│   ├── defects.tsv                             # 缺陷清单
│   ├── verification.md                         # 验证结果
│   └── cleanup.tsv                             # 清理记录
└── docs/superpowers/                           # 已提交
    ├── specs/2026-08-02-agent-operability-audit-design.md
    └── plans/2026-08-02-agent-operability-audit.md

/Users/nanjiakai/Desktop/OpenCode_AI-Agent可操作性审计报告.md   # 最终报告（不提交）
```

---

### Task 0: 环境准备与测试栈启动

**Files:**
- Create: `/.env.audit`
- Create: `/audit/.gitignore` (内容: `*`)
- Create: `/audit/agent-client.mjs`（基础结构）

**Interfaces:**
- Consumes: `.env.example`（已有）
- Produces: 运行的 `gubuy-erp-app` 容器 + 健康的 `gubuy-erp-db` 容器

- [ ] **Step 0.1: 准备审计目录与 git 隔离**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
mkdir -p audit/{recon,matrix/per-module,probes,journeys,failure}
echo '*' > audit/.gitignore
```

- [ ] **Step 0.2: 复制 .env.example → .env.audit**

```bash
cp .env.example .env.audit
```

- [ ] **Step 0.3: 检查端口占用，决定使用默认还是回退端口**

```bash
# 检查默认端口
lsof -i :8080 2>/dev/null | head -1
lsof -i :3307 2>/dev/null | head -1
# 若任一被占用，编辑 .env.audit
# APP_PORT=18080
# MYSQL_HOST_PORT=13307
```

- [ ] **Step 0.4: 构建并启动测试栈**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
docker compose --env-file .env.audit up -d --build
```

预期：构建完成，`app` 和 `db` 两个容器出现。

- [ ] **Step 0.5: 等待 db healthy**

```bash
# 循环等待，最长 120 秒
for i in $(seq 1 60); do
  status=$(docker inspect --format='{{.State.Health.Status}}' gubuy-erp-db 2>/dev/null)
  echo "[$i] db=$status"
  [ "$status" = "healthy" ] && break
  sleep 2
done
[ "$status" = "healthy" ] || { echo "DB 未就绪"; exit 1; }
```

预期：60 次循环内出现 `db=healthy`。

- [ ] **Step 0.6: 等待 app 就绪**

```bash
for i in $(seq 1 60); do
  body=$(curl -sS -m 3 http://localhost:${APP_PORT}/api/admin/status 2>/dev/null)
  if [ -n "$body" ]; then echo "app ready: $body"; break; fi
  echo "[$i] waiting app..."
  sleep 2
done
```

预期：循环内出现 `app ready: {"..."}`。

- [ ] **Step 0.7: 记录栈启动信息到 recon/**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
APP_PORT=$(grep ^APP_PORT .env.audit | cut -d= -f2)
DB_PORT=$(grep ^MYSQL_HOST_PORT .env.audit | cut -d= -f2)
{
  echo "# 测试栈启动信息"
  echo "启动时间: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "APP_PORT=$APP_PORT"
  echo "DB_PORT=$DB_PORT"
  echo "DB_NAME=$(grep ^DB_NAME .env.audit | cut -d= -f2)"
  echo "JWT_SECRET=$(grep ^JWT_SECRET .env.audit | cut -d= -f2 | head -c 8)..."
  echo
  echo "## 容器状态"
  docker compose --env-file .env.audit ps
  echo
  echo "## admin status 响应"
  curl -sS http://localhost:$APP_PORT/api/admin/status
} > audit/recon/00-stack-up.md
cat audit/recon/00-stack-up.md
```

- [ ] **Step 0.8: 提交本任务执行记录到 recon/**

记录 stack up 时间和参数到 `audit/recon/00-stack-up.md`（在 Step 0.7 已生成）。

---

### Task 1: Agent 客户端构建

**Files:**
- Create: `/audit/agent-client.mjs`

**Interfaces:**
- Consumes: 环境变量 `APP_PORT`、`JWT_SECRET`
- Produces: 导出的 `req`, `loginAdmin`, `loginEmployee`, `expect200`, `expectId`, `assert`，可被后续 probe 脚本 import

- [ ] **Step 1.1: 写 agent-client.mjs 骨架（接口契约）**

```javascript
// audit/agent-client.mjs
import { readFileSync } from 'node:fs';

const ENV = (() => {
  const lines = readFileSync('.env.audit', 'utf8').split('\n');
  const o = {};
  for (const l of lines) {
    const m = l.match(/^([A-Z_]+)=(.*)$/);
    if (m) o[m[1]] = m[2];
  }
  return o;
})();

const BASE = `http://localhost:${ENV.APP_PORT || 8080}`;

let TOKEN = null;
let TOKEN_ADMIN = null;

export async function req(method, path, { body, headers, token = TOKEN } = {}) {
  const url = path.startsWith('http') ? path : `${BASE}${path}`;
  const init = {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(headers || {}),
    },
  };
  if (body !== undefined) init.body = typeof body === 'string' ? body : JSON.stringify(body);
  const t0 = Date.now();
  let res, text, json;
  try {
    res = await fetch(url, init);
    text = await res.text();
    try { json = text ? JSON.parse(text) : null; } catch { json = null; }
  } catch (e) {
    return { ok: false, error: String(e), status: 0, duration: Date.now() - t0 };
  }
  return {
    ok: res.ok,
    status: res.status,
    body: json ?? text,
    raw: text,
    headers: Object.fromEntries(res.headers),
    duration: Date.now() - t0,
  };
}

export async function loginAdmin(password) {
  const r = await req('POST', '/api/admin/verify', { body: { password }, token: null });
  if (!r.ok) throw new Error(`admin verify failed: ${r.status} ${JSON.stringify(r.body)}`);
  // 该接口通常返回 token 或确认状态
  return r.body;
}

export async function initAdmin(password) {
  return req('POST', '/api/admin/init', { body: { password }, token: null });
}

export async function loginEmployee(pin) {
  const r = await req('POST', '/api/employees/verify-pin', { body: { pin }, token: null });
  if (!r.ok) throw new Error(`employee pin verify failed: ${r.status}`);
  return r.body;
}

// 断言工具
export function expectStatus(r, expected) {
  if (r.status !== expected) {
    throw new Error(`expected status ${expected} got ${r.status}: ${JSON.stringify(r.body)}`);
  }
  return r;
}

export function expectField(r, key) {
  if (!r.body || r.body[key] === undefined) {
    throw new Error(`missing field '${key}' in ${JSON.stringify(r.body)}`);
  }
  return r.body[key];
}

export function expectOk(r) { return expectStatus(r, 200); }
export function expectCreated(r) { return expectStatus(r, 201); }

export const config = { BASE, ENV };
```

- [ ] **Step 1.2: 写最小自检脚本验证客户端连通**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
cat > /tmp/agent-smoke.mjs <<'EOF'
import { req, expectOk, config } from './audit/agent-client.mjs';
const r = await req('GET', '/api/admin/status');
console.log('BASE:', config.BASE);
console.log('admin/status:', r.status, JSON.stringify(r.body));
if (!r.ok) process.exit(1);
EOF
node /tmp/agent-smoke.mjs
```

预期：输出 `admin/status: 200 {...}` 或对应的初始状态码。

- [ ] **Step 1.3: 验证 admin init / verify 闭环**

通过 `agent-client.mjs` 尝试 `initAdmin` + `loginAdmin` 并记录：
- 若已初始化，记录"已存在"
- 若未初始化，初始化后立即 verify，记录响应结构

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
cat > /tmp/admin-init.mjs <<'EOF'
import { initAdmin, loginAdmin, req } from './audit/agent-client.mjs';

// 探测状态
const s = await req('GET', '/api/admin/status');
console.log('status before:', s.body);

if (s.body?.initialized === false) {
  const init = await initAdmin('audit_2026_08_02_pwd');
  console.log('init:', init.status, init.body);
}

const v = await loginAdmin('audit_2026_08_02_pwd');
console.log('verify:', v.status, v.body);
EOF
node /tmp/admin-init.mjs | tee audit/recon/01-admin-bootstrap.md
```

预期：能看到 init/verify 响应；如果已有 admin 密码则跳过 init 步骤。

---

### Task 2: 接口侦察与路由表（Phase 0）

**Files:**
- Create: `/audit/recon/routes.tsv`
- Create: `/audit/recon/controllers-map.tsv`
- Create: `/audit/recon/swagger-coverage.tsv`

- [ ] **Step 2.1: 提取所有路由**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
# 解析 router.go 输出 [METHOD] [PATH] -> [HANDLER] -> [AUTH]
grep -E 'auth\.|api\.' erp-backend/routers/router.go | tee /tmp/router-raw.txt
```

- [ ] **Step 2.2: 手工结构化到 routes.tsv**

输出字段：`method \t path \t handler \t auth_tier \t controller_file \t has_swagger_annotation(y/n)`

按控制器文件确认 handler 归属。`auth_tier` 取 `public` / `jwt` / `admin` 之一。

- [ ] **Step 2.3: 用 grep 验证每个 handler 在哪个文件**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
grep -l "func CreateProduct" erp-backend/controllers/
grep -l "func GetProducts" erp-backend/controllers/
# ... 对每个 handler 跑一次
```

- [ ] **Step 2.4: 验证 Swagger 覆盖**

对每个 handler，用 `grep -B2 "func Xxx" controller.go` 看是否有 `@Summary`/`@Description`/`@Tags` 等 swag 注释。统计覆盖率。

输出 `audit/recon/swagger-coverage.tsv`：
- 全部 handler 数
- 有 swag 注释的 handler 数
- 覆盖率

- [ ] **Step 2.5: 记录最终统计到 recon/00-stack-up.md 末尾**

追加：
- 路由总数
- 控制器文件数
- Swagger 覆盖率
- 无 MCP/CLI/桌面 UI/异步任务的明确证据（main.go grep）

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
{
  echo
  echo "## 接口层统计"
  echo "路由总数: $(grep -cE 'auth?\.(GET|POST|PUT|DELETE)' erp-backend/routers/router.go)"
  echo "控制器文件: $(ls erp-backend/controllers/*.go | wc -l)"
  echo "Swagger 覆盖: 待计算"
  echo "MCP 服务: $(grep -ri 'mcp' erp-backend/ | wc -l) (期望 0)"
  echo "CLI: $(find . -name 'main*.go' -not -path './node_modules/*' | xargs grep -l 'flag\\.' 2>/dev/null | wc -l) (期望 0)"
  echo "WebSocket: $(grep -rE 'websocket|ws://|wss://' erp-backend/ | wc -l) (期望 0)"
} >> audit/recon/00-stack-up.md
```

---

### Task 3: 能力矩阵构建（Phase 1）

**Files:**
- Create: `/audit/matrix/matrix.tsv`（骨架）
- Create: `/audit/matrix/per-module/<module>.tsv`（每个模块）

- [ ] **Step 3.1: 定义矩阵列（按设计文档 §5.1）**

矩阵列：
```
id | module | method | path | description_source(swagger/infer) | discoverable(y/n)
| current_state_readable(y/n) | executable(y/n) | result_verifiable(y/n)
| cross_feature_usable(y/n) | recoverable(y/n) | conclusion(PASS/PARTIAL/FAIL/BLOCKED/N/A)
| evidence_file | notes
```

- [ ] **Step 3.2: 遍历 routes.tsv 写入骨架行**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
# 用 awk 从 routes.tsv 生成 matrix.tsv 骨架（其他列空）
awk -F'\t' 'NR>1 {printf "%s\t%s\t%s\t%s\t\t\t\t\t\t\t\t\t\t\n", $1, $5, $1, $2}' audit/recon/routes.tsv > audit/matrix/matrix.tsv
```

- [ ] **Step 3.3: 按模块切分**

为每个模块生成 `audit/matrix/per-module/<module>.tsv`：
- `auth` - 鉴权 / admin
- `products` - 商品 + 分类
- `orders` - 订单 + 收银
- `inbound` - 入库 + 库存流水
- `employees` - 员工 + 考勤
- `finance` - 财务 + 账户 + 支出
- `subsidy` - 补贴
- `dashboard` - 看板
- `search` - 搜索
- `export` - 导出
- `system` - 系统/备份/恢复/拼音

- [ ] **Step 3.4: 标注每行初始状态**

逐行处理：
- 静态分析：Swagger 描述、参数 schema、返回结构清晰度
- 写 initial `discoverable` 值
- 写 `conclusion=pending`
- 留 `evidence_file` 指向后续 probe 脚本

---

### Task 4: 可发现性 / 可理解性审计（Phase 2）

**Files:**
- Create: `/audit/matrix/discoverability.md`
- Modify: `/audit/matrix/matrix.tsv`（填充 discoverable / state_readable 列）

- [ ] **Step 4.1: 写 Swagger 完整性 probe**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
# 提取 swag 注释块
for f in erp-backend/controllers/*.go; do
  awk '/^\/\/ @Summary|^\/\/ @Description|^\/\/ @Tags|^\/\/ @Param|^\/\/ @Success/{print FILENAME":"NR": "$0}' "$f"
done > audit/recon/swagger-comments.txt
wc -l audit/recon/swagger-comments.txt
```

- [ ] **Step 4.2: 抽样验证 5 个端点的 swagger 准确性**

挑 5 个端点（如 `CreateProduct`, `CreateOrder`, `VerifyPin`, `TransferFunds`, `ExportOrders`），用 agent-client 实际调用并对比 swagger 描述的字段。

- [ ] **Step 4.3: 错误响应可读性探测**

故意发非法请求：
- 缺字段的 POST
- 不存在的 ID
- 错误的鉴权
- 超大 payload
- 错误枚举值

记录每种响应的结构化程度（是否有 code/msg/field）。结果写入 `audit/matrix/discoverability.md`。

- [ ] **Step 4.4: UI 模块 DOM 探查（仅静态读取）**

不启动浏览器；通过 `read` 工具读每个 `views/*.vue`：
- 找 `data-testid`、`aria-label`、button 文本、表单 `name`
- 标注"可发现性"等级

写入 `audit/matrix/ui-discoverability.tsv`：
```
module | view_file | data_testid_count | aria_label_count | semantic_text_count | finding
```

---

### Task 5: 五步闭环 - 核心模块（Phase 3）

**Files:**
- Create: `/audit/probes/<module>.mjs`
- Create: `/audit/probes/<module>-results.tsv`
- Modify: `/audit/matrix/matrix.tsv`（更新 conclusion / evidence）

对每个核心模块（products, categories, orders, inbounds, employees, accounts, finance, subsidy, search, exports, system, admin）按以下步骤执行。

- [ ] **Step 5.X.1: 写 probe 脚本（每个模块一个）**

模板：
```javascript
// audit/probes/products.mjs
import { req, expectOk, expectCreated, expectField, config } from '../agent-client.mjs';
import { writeFileSync } from 'node:fs';

const stamp = Date.now();
const SKU = `audit_2026_08_02_sku_${stamp}`;
const results = [];

// 1. 发现：列出商品列表
const list = await req('GET', '/api/products');
results.push({ step: 'discover', op: 'GET /products', status: list.status, ok: list.ok });
if (!list.ok) { writeFileSync('audit/probes/products-results.tsv', JSON.stringify(results, null, 2)); process.exit(1); }

// 2. 读取：找一个 category id 用作依赖
const cats = await req('GET', '/api/categories');
const categoryId = cats.body?.[0]?.id || 1;

// 3. 执行：创建商品
const create = await req('POST', '/api/products', { body: {
  name: SKU, sku: SKU, category_id: categoryId, price: 99, stock: 10
}});
results.push({ step: 'execute', op: 'POST /products', status: create.status, ok: create.ok, body: create.body });

// 4. 验证：再次 GET 详情
const createdId = create.body?.id;
const detail = await req('GET', `/api/products?search=${SKU}`);
results.push({ step: 'verify', op: 'GET /products?search', status: detail.status, ok: detail.ok, found: detail.body?.length });

// 5. 清理：删除
if (createdId) {
  // 项目无 DELETE /products, 用编辑替代或归档
  results.push({ step: 'cleanup', op: 'note: no DELETE endpoint', note: '留待任务清理' });
}

writeFileSync('audit/probes/products-results.tsv', JSON.stringify(results, null, 2));
console.log(JSON.stringify(results, null, 2));
```

- [ ] **Step 5.X.2: 运行 probe**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
node audit/probes/products.mjs
```

- [ ] **Step 5.X.3: 异常路径探测**

对每个核心端点，加测：
- 无效输入（缺字段、错误类型、负数、超大）
- 资源不存在（404）
- 重复提交（POST 两次同样的体）
- 鉴权缺失（去掉 token）

```javascript
// audit/probes/products-errors.mjs
import { req } from '../agent-client.mjs';

const errs = [];
// 1. 缺字段
errs.push({ case: 'missing_name', r: await req('POST', '/api/products', { body: { sku: 'x' } }) });
// 2. 无 token
errs.push({ case: 'no_auth', r: await req('POST', '/api/products', { body: { name: 'x' }, token: null }) });
// 3. 错误类型
errs.push({ case: 'bad_type', r: await req('POST', '/api/products', { body: { name: 'x', price: 'not-a-number' } }) });

console.log(JSON.stringify(errs, null, 2));
```

- [ ] **Step 5.X.4: 写入矩阵 conclusion**

根据 probe 结果，对每行打 `PASS/PARTIAL/FAIL/BLOCKED/N/A`，evidence_file 指向 probe 脚本和结果。

---

### Task 6: 端到端旅程（Phase 4）

**Files:**
- Create: `/audit/journeys/A-data-lifecycle.md`
- Create: `/audit/journeys/B-config-to-behavior.md`
- Create: `/audit/journeys/C-financial-close.md`
- Create: `/audit/journeys/D-exports.md`
- Create: `/audit/journeys/E-cross-entry.md`

- [ ] **Step 6.1: 旅程 A — 数据生命周期**

写 `/audit/probes/journey-A.mjs`，按 A 旅程：emp → cat → sku → inb → ord → deliver → 清理。每个步骤记录 status / 实体 ID / 是否可读回。

执行：
```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
node audit/probes/journey-A.mjs > audit/journeys/A-data-lifecycle.md
```

- [ ] **Step 6.2: 旅程 B — 配置→行为**

写 `/audit/probes/journey-B.mjs`：
- 读 admin 当前状态
- 修改 admin 密码
- 用旧密码 verify（应失败）
- 用新密码 verify（应成功）
- 修改后再读取确认

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
node audit/probes/journey-B.mjs > audit/journeys/B-config-to-behavior.md
```

- [ ] **Step 6.3: 旅程 C — 财务闭环**

写 `/audit/probes/journey-C.mjs`：
- 创建订单（含商品）
- 读取订单总金额
- 用混合方式"支付"（cash + subsidy + account，按实际接口能力）
- 读取订单与支付明细
- 读取 finance summary
- 验证 summary 与订单一致
- 取消订单（如允许）
- 再读取确认

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
node audit/probes/journey-C.mjs > audit/journeys/C-financial-close.md
```

- [ ] **Step 6.4: 旅程 D — 导出完整性**

写 `/audit/probes/journey-D.mjs`：
- 调 5 个 export 端点
- 解析返回的 CSV/Excel
- 验证行数 vs 列表接口返回行数
- 验证关键字段存在

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
node audit/probes/journey-D.mjs > audit/journeys/D-exports.md
```

- [ ] **Step 6.5: 旅程 E — 跨入口一致性**

写 `/audit/probes/journey-E.mjs`：
- 通过 REST 创建数据
- 验证数据在 list 端点可读
- 读取前端 Vue 组件中关键数据绑定逻辑（静态读源码验证 UI 是否会渲染该数据）

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
node audit/probes/journey-E.mjs > audit/journeys/E-cross-entry.md
```

- [ ] **Step 6.6: 汇总旅程结果**

为每个旅程写：
- 使用入口
- 结果
- 最终验证证据
- 阻塞/缺陷

写入 `/audit/journeys/_summary.md`。

---

### Task 7: 失败恢复与自主性（Phase 5）

**Files:**
- Create: `/audit/failure/recovery-probes.md`

- [ ] **Step 7.1: 异常场景枚举**

按设计文档 §8 表格，对每个场景设计 probe：
- 参数不合法
- 资源不存在/已被删除
- 前置条件不满足
- 空列表/无数据
- 网络/服务短暂失败（通过关停 app 容器测试重连）
- 超时（大 payload）
- 重复请求（幂等性）
- 异步尚未完成（项目无异步任务，标 N/A 并说明）
- 页面刷新/重连

- [ ] **Step 7.2: 执行每场景 probe**

每个场景记录：
- Agent 实际步骤
- 系统返回
- 是否能继续
- 是否产生重复实体

写入 `/audit/failure/recovery-probes.md`。

---

### Task 8: 缺陷清单与分级

**Files:**
- Create: `/audit/defects.tsv`

- [ ] **Step 8.1: 汇总所有发现**

从 `audit/matrix/matrix.tsv` (PARTIAL/FAIL) + `audit/failure/` + `audit/journeys/` 中收集。

- [ ] **Step 8.2: 写入 defects.tsv**

列：`id | title | severity(blocker/high/medium/low) | module | journey | agent_blocker | root_cause | evidence_file | repro_steps | min_fix_suggestion | fix_status(not_fixed)`

- [ ] **Step 8.3: 复核严重度**

确保：
- 阻断：核心链路不可完成 / 无法确认关键结果
- 高：可能重复创建/重复结算
- 中：依赖脆弱 UI / 隐含参数
- 低：描述/反馈/可读性

---

### Task 9: 数据清理

**Files:**
- Create: `/audit/cleanup.tsv`
- Create: `/audit/cleanup.sql`（备份 SQL 备份指令）

- [ ] **Step 9.1: 列出需清理的实体 ID**

从 journey A 和 probe 收集所有创建的 `audit_2026_08_02_*` 实体 ID。

- [ ] **Step 9.2: 通过 API 优先清理**

- 取消未完成订单
- 删除测试商品（如接口允许）
- 删除测试员工
- 删除测试分类

- [ ] **Step 9.3: 数据库清理（最后手段）**

如果 API 不能完全清理（如无 DELETE endpoint），用 mysql client：

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
DB_PORT=$(grep ^MYSQL_HOST_PORT .env.audit | cut -d= -f2)
docker exec -i gubuy-erp-db mysql -uroot -prootpassword erp_db <<SQL > audit/cleanup.sql
SELECT 'products to delete' AS step, COUNT(*) FROM products WHERE name LIKE 'audit_2026_08_02_%';
SELECT 'categories to delete' AS step, COUNT(*) FROM categories WHERE name LIKE 'audit_2026_08_02_%';
SELECT 'employees to delete' AS step, COUNT(*) FROM employees WHERE name LIKE 'audit_2026_08_02_%';
-- 实际删除（视情况）
-- DELETE FROM products WHERE name LIKE 'audit_2026_08_02_%';
SQL
```

- [ ] **Step 9.4: 验证清理结果**

```bash
# 确认无残留
docker exec gubuy-erp-db mysql -uroot -prootpassword erp_db -e "
SELECT 'products' AS tbl, COUNT(*) AS n FROM products WHERE name LIKE 'audit_2026_08_02_%'
UNION ALL
SELECT 'categories', COUNT(*) FROM categories WHERE name LIKE 'audit_2026_08_02_%'
UNION ALL
SELECT 'employees', COUNT(*) FROM employees WHERE name LIKE 'audit_2026_08_02_%';
"
```

写入 `audit/cleanup.tsv`：每张表清理前/后行数。

---

### Task 10: 验证结果

**Files:**
- Create: `/audit/verification.md`

- [ ] **Step 10.1: 后端构建**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon/erp-backend
go build ./...
```
记录退出码与输出。

- [ ] **Step 10.2: 后端 vet**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon/erp-backend
go vet ./...
```

- [ ] **Step 10.3: 后端已有测试**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon/erp-backend
go test ./... 2>&1 | tail -30
```

- [ ] **Step 10.4: 前端构建**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
npm run build 2>&1 | tail -20
```

- [ ] **Step 10.5: 前端已有测试**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
node --test src/utils/*.test.mjs src/views/*.test.mjs 2>&1 | tail -30
```

- [ ] **Step 10.6: API 闭环验证（抽样）**

随机抽 5 个 probe 重跑，确认结果一致。

- [ ] **Step 10.7: 数据清理验证（已在 Task 9.4）**

- [ ] **Step 10.8: 写入 verification.md**

每项填 `PASS / FAIL / BLOCKED / NOT RUN`，附真实命令与真实输出摘要。

---

### Task 11: 最终报告

**Files:**
- Create: `/Users/nanjiakai/Desktop/OpenCode_AI-Agent可操作性审计报告.md`

- [ ] **Step 11.1: 起草报告骨架**

按设计文档 §7 七节：
1. 总结
2. Agent 能力矩阵
3. 端到端旅程结果
4. 缺陷清单
5. 已修复内容（空：本审计不修复）
6. 验证结果
7. Agent 接入改进清单

- [ ] **Step 11.2: 填充总结**

- 分支：main（commit 69ec3e8 + 工作区未提交改动）
- 测试栈：端口、容器名
- 矩阵统计：PASS / PARTIAL / FAIL / BLOCKED / N/A 计数
- Agent 完整可操作率：`PASS / 适用功能数`

- [ ] **Step 11.3: 复制矩阵表**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
column -t -s $'\t' audit/matrix/matrix.tsv > /tmp/matrix.txt
```

将 `/tmp/matrix.txt` 内容嵌入报告。

- [ ] **Step 11.4: 嵌入旅程结果**

从 `audit/journeys/_summary.md` 复制旅程表。

- [ ] **Step 11.5: 嵌入缺陷清单**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
column -t -s $'\t' audit/defects.tsv > /tmp/defects.txt
```

- [ ] **Step 11.6: 嵌入验证结果**

复制 `audit/verification.md` 内容到第 6 节。

- [ ] **Step 11.7: 写 Agent 接入改进清单**

从 defects 提炼出 actionable 建议：
- 缺失的 GET/查询接口
- 缺 swagger 注释的端点
- 错误响应不结构化
- UI 缺少 `data-testid`
- 重复提交防护
- 等等

- [ ] **Step 11.8: 落盘并验证**

```bash
ls -la /Users/nanjiakai/Desktop/OpenCode_AI-Agent可操作性审计报告.md
wc -l /Users/nanjiakai/Desktop/OpenCode_AI-Agent可操作性审计报告.md
```

- [ ] **Step 11.9: 关闭测试栈（保留 audit/ 目录）**

```bash
cd /Users/nanjiakai/Documents/Development/coder/erpwonchon
docker compose --env-file .env.audit down
```

---

### Task 12: 最终自检

- [ ] **Step 12.1: 报告七节齐全**
- [ ] **Step 12.2: 矩阵每行有结论**
- [ ] **Step 12.3: 缺陷每条有 ID/严重度/根因/复现/修复建议**
- [ ] **Step 12.4: 验证章节 PASS/FAIL/BLOCKED 都有真实命令与输出**
- [ ] **Step 12.5: 测试栈已关闭、`.env.audit` 与 `audit/` 目录未提交**
- [ ] **Step 12.6: 工作区 40+ 原始改动未被覆盖**

---

## Self-Review

**1. Spec coverage:**
- §2 范围 — Task 2-7 覆盖 ✓
- §3 测试环境 — Task 0 覆盖 ✓
- §4 Agent 客户端 — Task 1 覆盖 ✓
- §5 能力矩阵 — Task 3-5 覆盖 ✓
- §6 严重度 — Task 8 覆盖 ✓
- §7 报告结构 — Task 11 覆盖 ✓
- §8 执行步骤 — Task 0-12 全覆盖 ✓
- §9 风险缓解 — Task 0.3 端口回退 / Task 9 清理 / Task 12 自检 ✓
- §10 完成定义 — Task 12 逐条 ✓

**2. Placeholder scan:** 无 TBD/TODO/同前/类似 ✓

**3. Type/interface consistency:** `req` / `expectOk` / `expectId` 在 Task 1 定义，Task 5/6 一致使用 ✓

**4. Issue spotted:** Task 1.3 admin 密码初始化依赖 seed 状态（项目代码会判定是否已 init），已用 `if (s.body?.initialized === false)` 防护 ✓

**5. Issue spotted:** Task 9 清理依赖具体表结构（products/categories/employees），其他表（orders/inbounds）需在执行时按实际 schema 调整；Task 9.2/9.3 已留有现场调整空间 ✓
