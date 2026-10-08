# ERP 调试 CLI

实施范围与验收标准见 [实施方案](erpctl-plan.md)。

## 构建

在仓库根目录执行：

```sh
(cd erp-backend && go build -o ../bin/erpctl ./cmd/erpctl)
./bin/erpctl help
```

CLI 独立于后端启动流程，不需要数据库环境变量。后端需更新以提供新的 `GET /api/ai/agent/messages/:message_id/trace` 接口。

## 配置与鉴权

`ERP_URL` 默认 `http://localhost:8080`，可用 `--url` 覆盖。URL 可以包含反向代理路径前缀。推荐先使用管理员密码登录：

```sh
./bin/erpctl auth login
./bin/erpctl auth status --json
./bin/erpctl auth logout
```

macOS/Linux 终端隐藏密码输入；自动化可使用 `auth login --password-stdin` 从 stdin 提供一行密码。没有密码命令参数，不保存密码。登录沿用 `/api/admin/verify`，JWT 按规范化的服务 URL 独立保存，防止不同服务器或路径前缀串用凭据。目录为系统用户配置目录下的 `erpctl`（macOS：`~/Library/Application Support/erpctl`），可通过 `ERP_CONFIG_DIR` 指定专用目录。目录权限 0700、文件权限 0600；拒绝符号链接与过于宽松的权限，采用原子写入。当前是受限本地文件存储，JWT 未加密，请勿将凭据目录放入共享目录或仓库。

`auth status` 同时检查有效期并向服务器验证管理员鉴权，不输出 JWT。过期需要重新登录，程序不自动刷新。`auth logout` 只删除该 URL 对应的本地凭据，不撤销已签发的服务器 token。

`ERP_TOKEN` 仍兼容且优先于保存的登录凭据；环境变量不会被 logout 清除。如希望使用保存的登录，请先 `unset ERP_TOKEN`。使用 zsh 时可隐藏输入已有 token：

```sh
read -rs 'ERP_TOKEN?管理员 JWT: '; export ERP_TOKEN
```

远端日志使用 `ERP_SSH_HOST` 或 `--host` 指定已有 SSH 别名或 `user@host`，需要可用的 SSH 密钥和已验证的主机配置。不会读取部署脚本中的密码或自动尝试登录。

## 环境检查

```sh
./bin/erpctl doctor --json
./bin/erpctl doctor --target router --url http://192.168.5.22:8080 --json
```

通过 `/api/admin/status` 检查服务响应、数据库查询和管理员初始化。设置 token 后检查管理员配置接口及当前 AI 模型/密钥是否配置可用。配置检查不会请求模型供应商，不能证明模型服务在线；不输出密钥。未提供 token 时管理员 AI 检查标记 skipped。

## Agent 调试

```sh
./bin/erpctl agent ask '2026年8月和9月销售情况对比' --json
./bin/erpctl agent ask '我说的是订单数量' --session 12 --thinking deep --json
./bin/erpctl agent trace 34 --json
```

`ask` 默认创建独立调试会话（`--debug=true`），`--session` 复用当前用户已有且模式匹配的会话。调试标记 `debug` 在数据库中保存并由后端强制执行：不读取、不学习、不修改长期记忆，不创建业务草稿；对话、会话上下文和追踪仍然保存。显式“请记住”只记录消息，回复明确说明未修改记忆。普通 UI 会话行为保持不变。

需要普通会话行为时显式使用 `--debug=false`；复用普通会话也必须指定它。模式不允许通过单次提问更改。CLI 发送问题前检查服务端回传的 session.debug；旧后端不支持时停止，不会悄悄降级成普通模式。旧后端可能已创建一个空会话，但不会执行测试问题。

追踪结果增加 `debug` 字段。结果包含 message（含 session_id、id、status、content）以及解析后的 trace、task、evidence。追踪缺失时为 null（例如仅保存记忆的消息），不伪造数据。文本输出先展示回答，随后展示完整 JSON 诊断。

调试调用会保存会话与消息。`--debug=false` 沿用普通 Agent 行为，可能学习持久记忆或生成待确认草稿；CLI 不确认草稿。超时或错误不会自动重试；根据错误中的会话编号先在当前 Agent 界面检查记录。HTTP 及日志命令默认超时 5 分钟，`--timeout 30s` 可调整。

## 批量评测

```sh
./bin/erpctl agent eval --cases docs/agent-evals/core.json --report output/agent-evals/baseline.json --json
./bin/erpctl agent eval --cases docs/agent-evals/core.json --compare output/agent-evals/baseline.json --report output/agent-evals/candidate.json --json
```

文件为 JSON 数组，每项需要唯一的 name，以及 question 或 turns 其中之一，可设置 `dataset_version`、`as_of`、contains、not_contains 文本数组，以及 assertions 结构化断言。`as_of` 用 `YYYY-MM-DD`，仅允许调试模式；它冻结任务理解、趋势和预测的参考时间，绝不冻结或回填业务数据。多轮案例的所有 turn 共用同一调试会话；文本和任务断言检查最后一轮。报告包含生成时间、耗时 P50/P95、token、工具调用数、每轮消息与 trace。`--compare` 对比上一份报告的总通过数；若案例集不同，应先确保比较口径一致。字段拼写、日期、预期值和误差在调用模型前验证。

固定核心集定义在 `docs/agent-evals/core.json`，业务夹具及期望值见 `docs/agent-evals/fixture-v1.json`，Go 集成测试用同口径固定订单验证数值。先在一次性 MySQL 中设置 `DB_NAME=erp_agent_eval_test`，再运行 `cd erp-backend && go run ./cmd/agent-fixture --apply --database-confirmation erp_agent_eval_test` 导入固定数据；导入命令拒绝非 `erp_agent_*_test` 数据库。报告含 trace 和业务证据，文件权限设为 0600，只能存本地受限目录，不能提交或分享。

```json
{
  "name": "固定样本销售额",
  "question": "2026年8月和9月销售情况对比",
  "assertions": {
    "topic": "sales",
    "metrics": ["sales_revenue", "order_count"],
    "periods": [
      {"start": "2026-08-01", "end": "2026-08-31"},
      {"start": "2026-09-01", "end": "2026-09-30"}
    ],
    "values": [
      {"metric": "sales_revenue", "start": "2026-08-01", "end": "2026-08-31", "value": 900, "tolerance": 0.01}
    ]
  }
}
```

上例金额来自集成测试的固定样本；真实环境应填写自己核对的预期值。topic 检查任务主题，metrics/periods 要求任务包含指定指标/期间。values 核对 `business_report` 证据中的结构化数值，tolerance 为绝对允许误差，默认 0；没有预期值会拒绝配置，真实 0 可以通过，缺失/null 不会按 0 处理。跨月必须指定期间；省略期间仅允许报告唯一一行。证据缺失、格式错误、报表标记 missing 或匹配行重复均失败。当前数值断言只支持标准业务报表；它验证查询证据，不替代对自由文本分析质量的人工判断。报告 assertions 数组包含预期值、实测值与逐项结果。默认每个案例创建独立调试会话；同一多轮案例内部自动复用该会话。添加 `--session` 可指定现有调试会话，使用普通会话需 `--debug=false`。评测顺序执行，失败继续收集后续结果。报告包含通过/失败数量、逐项失败原因和诊断信息。预期业务金额需根据固定测试数据填写；示例包含主题/指标/期间和禁止泄漏的文本断言；金额断言需要按测试数据补充。

## 案例

```sh
./bin/erpctl agent cases --case-state new --case-limit 100 --json
```

默认列出当前登录账号的全部候选；`--case-state new` 可筛新案例，`--case-limit` 范围为 1 至 100，`--case-after-id N` 可用于批次游标。案例包含经规则脱敏的问题、回答和反馈说明，属于业务内容；自动优化流程应在受控本地环境读取，并在形成测试样例时复核敏感信息。

每周预测回填入口：`./bin/erpctl agent backtest --json`。它只计算已经结束月份的预测实际值，每次最多处理 200 条；快照保留原预测、三个月均值基线、查询筛选、数据截止时间和方法版本，实际值单独写入。`./bin/erpctl agent predictions --json` 返回当前账号的 MAE/WAPE，并比较预测值与三个月均值基线；样本跨多个目标月份前，汇总误差只用于初步观察。

已审核业务知识由管理员 API `/api/ai/agent/knowledge` 管理：POST 创建 `pending` 条目，PUT 审核为 `approved` 或 `rejected`，GET 可按状态查看。只有未过期的 approved 条目进入 Agent 的规划提示，并在证据中记录知识 ID。知识仅作参考，不能覆盖代码口径。建议每条知识设置复核到期日。

建议在独立测试环境评测：每次提问会产生模型费用并保存测试会话；默认调试模式隔离长期记忆与业务草稿。

## 日志

```sh
./bin/erpctl logs --since 10m --tail 200 --json
./bin/erpctl logs --target router --host erp-router --since 30m --json
```

默认容器为 `gubuy-erp-app`，可用 `--container` 覆盖。远端 logs 不需要 ERP URL。`--since` 接受正数 Go duration，如 30s、10m、2h。tail 限制 1 至 10000 行。日志是原始应用日志，可能包含业务内容，分享前检查。

## 自动化约定

- stdout：完整结果；`--json` 输出单个 JSON 对象。
- stderr：操作错误；JSON 模式不会把错误混入结果。
- 退出码 0：成功；1：请求、网络、文件或执行失败；2：命令参数错误；3：环境检查、回答完整状态或评测未通过。
- HTTP 响应上限 8 MiB，拒绝重定向，避免把鉴权发送到其他站点。
- `doctor` 检查失败及评测不通过仍输出结果；提问已成功但追踪接口不可用时输出已有回答并返回 1。

## 验证

```sh
(cd erp-backend && go test -race ./cmd/erpctl)
(cd erp-backend && go vet ./cmd/erpctl ./controllers ./routers)
# 使用独立 MySQL 测试库，禁止设置为生产库；项目现有 controller 测试会写入、清理测试数据。
(cd erp-backend && DB_NAME=erp_cli_audit_test go test -tags=integration ./controllers -run '^TestGetAIAgentMessageTraceOwnership$')
```
