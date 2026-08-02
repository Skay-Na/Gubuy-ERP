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

## Docker 部署

### 首次部署

```bash
cp .env.example .env
docker compose up -d --build
```

关键点：

- `docker compose` 会自动读取当前目录的 `.env`；如果没有 `.env`，也会回退到 `docker-compose.yml` 里的默认值，因此不会卡死已有部署。
- 数据库卷显式命名为 `gubuy-erp-mysql-data`，避免因为 Compose 项目名变化导致新旧卷错位。
- 备份恢复大小限制通过 `BACKUP_UPLOAD_MAX_MB` 控制，默认 `200MB`。
- 建议务必修改 `.env` 中的 `JWT_SECRET` 与数据库密码。

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
