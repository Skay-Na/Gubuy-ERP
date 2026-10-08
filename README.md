# Wonchon ERP

门店 ERP / POS 单体项目，前端基于 `Vue 3 + Vite`，后端基于 `Gin + GORM + MySQL`。

## 项目结构

- `src/`：前端页面与组件
- `erp-backend/`：Go 后端接口与业务逻辑
- `Dockerfile`：前后端一体化构建镜像
- `docker-compose.yml`：应用与 MySQL 编排
- `.env.example`：部署环境变量模板

## 本地开发

### 前端

```bash
npm install
npm run dev
```

### 后端

后端依赖 MySQL。启动前请准备以下环境变量：

- `DB_USER`
- `DB_PASS`
- `DB_HOST`
- `DB_PORT`
- `DB_NAME`
- `JWT_SECRET`
- `AI_API_KEY`、`AI_MODEL`：启用图片识别时配置。模型需支持图片输入和 JSON 输出。
- `AI_API_URL`：可选的兼容 Chat Completions 接口地址。
- `AI_CONFIG_SECRET`：可选，至少 24 位，用于加密系统设置里保存的服务商 API Key；未配置时使用安全的 `JWT_SECRET`。恢复数据库备份时需保留相同的加密密钥。

## Docker 部署

### 首次部署

```bash
cp .env.example .env
docker compose up -d --build
```

关键点：

- `docker compose` 会自动读取当前目录的 `.env`。首次部署须先设置私有 `JWT_SECRET`（至少 32 字节）；缺失时 Compose 会明确报错，不会使用公开默认密钥。
- 数据库卷显式命名为 `gubuy-erp-mysql-data`，避免因为 Compose 项目名变化导致新旧卷错位。
- 备份恢复大小限制通过 `BACKUP_UPLOAD_MAX_MB` 控制，默认 `200MB`。
- `.env` 必须设置至少 32 字节的随机 `JWT_SECRET`；缺失或过短时后端拒绝启动。更换密钥会使已签发的 JWT 会话失效，需用户重新登录。
- AI 图片导入先保存原图和识别草稿，管理员核对后才创建订单或账目；原图保存在 MySQL 的 `ai_imports` 表中，数据库备份会一并包含图片，需留意备份体积。
- 员工收银台的“拍照录单”可识别手写订单、恢复本人草稿，核对客户与商品后填入原有结算流程；订单仍以当前登录员工归属，草稿提交一次后即失效。员工不能查看其他人的草稿或导入支出。
- 系统设置可分别保存 DeepSeek、MiniMax、小米 MiMo、千问的密钥与图片模型，并选择当前服务商。未启用服务商时沿用 `AI_API_KEY`、`AI_MODEL` 与 `AI_API_URL` 环境变量。密钥在后端加密存储，设置接口不会回传原文。
- 用 `scripts/update_router.sh` 更新旧软路由部署时，如果现有容器没有安全的加密密钥，脚本会在远端 `/mnt/opt/docker/install/.erp-ai-config-secret` 生成并持久保存一个独立密钥，通过 `.erp-ai-config-secret.compose.yml` 注入 app 容器。备份或迁移数据库时须一并保留这两个文件；以后手动重建 app 时使用 `docker compose -f docker-compose.yml -f .erp-ai-config-secret.compose.yml up -d app`。已有安全密钥的容器继续使用原密钥。
- 软路由上的 app 容器若需要独立 DNS，可在同一目录放置 `.erp-app-dns.compose.yml`；更新脚本会自动带上它。手动重建时也须追加 `-f .erp-app-dns.compose.yml`。
- 管理后台右下角的 ERP Agent 支持多轮对话，按权限读取客户订单、员工身份与订单业绩、月度提成、未收尾款、库存和利润，并可按模块提供系统操作指引；每次任务最多执行三个只读查询步骤。员工提成沿用员工管理的月度报表规则。订单、支出由 Agent 生成可编辑草稿，管理员确认后才保存，草稿 24 小时过期且只能提交一次。
- Agent 打开时展示低库存、未收尾款和待处理国补数量；拍照订单及支出截图可从对话框快捷入口进入原有图片导入预览流程。Agent 会话与草稿存于数据库，页面刷新后可恢复。当前 Agent 仅向管理员开放。
- AI 店长工作台位于 `/admin/ai-manager`。设置 `AGENT_RUNTIME_ENABLED=1` 会启动统一持久任务 Worker，扫描和任务执行从数据库恢复；该开关优先于旧 `AGENT_MONITOR_ENABLED`，避免两个扫描 Worker 并行。当前每日扫描仍使用 24 小时间隔，周期配置的持久化管理尚未实现。草案不会提交采购订单、联系供应商或支付货款；阶段状态与迁移限制见 `AGENT-RUNTIME-UNIFICATION-REPORT.md`。
- 演示商品默认不自动写入空数据库。仅在隔离演示环境设置 `ERP_SEED_DEMO_DATA=1` 时启用演示商品初始化。

## R3S / Docker 升级建议

当前你是在 R3S 软路由的 Docker 环境里运行，这种场景升级的关键不是“重建容器”，而是“确保数据卷和配置文件可回滚”。

### 升级前

1. 在系统设置页先导出一份 SQL 备份。
2. 备份当前 `.env` 文件。
3. 记录当前数据库卷名：

```bash
docker volume ls | grep gubuy-erp-mysql-data
```

4. 如需额外保险，可导出卷内原始数据：

```bash
docker run --rm \
  -v gubuy-erp-mysql-data:/from \
  -v "$(pwd)":/to \
  alpine sh -c "cd /from && tar czf /to/mysql-volume-backup.tar.gz ."
```

### 升级步骤

```bash
git pull
cp .env.example .env.example.latest
docker compose up -d --build
```

说明：

- 不要随意删除 `gubuy-erp-mysql-data` 卷。
- 不要在升级时切换 Compose 项目名。
- 如果 R3S 宿主机系统升级导致容器重建，只要 `.env` 和 `gubuy-erp-mysql-data` 仍在，业务数据就能保留。

### 回滚

如果新版本异常：

1. 回退代码版本。
2. 使用原 `.env` 重新执行：

```bash
docker compose up -d --build
```

如果数据库内容也需要回退，再通过系统设置页导入此前导出的 SQL 备份。

## 这次整理的重点

- 统一前端请求入口，避免不同页面鉴权和错误处理不一致。
- 备份/恢复改为走鉴权请求，避免浏览器直连接口导致失败。
- 恢复流程改为唯一临时文件，避免并发导入互相覆盖。
- 交付物改为环境变量驱动，并补齐部署/升级文档。

## 调试 CLI

`erpctl` 提供环境检查、Docker 日志、Agent 提问/执行追踪及批量评测。构建与使用见 [CLI 文档](docs/erpctl.md)，实施范围见 [实施方案](docs/erpctl-plan.md)。

```bash
(cd erp-backend && go build -o ../bin/erpctl ./cmd/erpctl)
./bin/erpctl doctor --json
```
