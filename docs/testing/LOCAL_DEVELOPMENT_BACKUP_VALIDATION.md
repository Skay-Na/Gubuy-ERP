# 本机开发环境与 SQL 备份验证

验证日期：2026-10-22。

## 开发入口

- `./dev.sh start`（或不带参数的 `./dev.sh`）或 `npm run dev:stack` 启动本机开发环境；`status` 检查进程与 HTTP 健康状态，`stop` 停止 Go/Air 与 Vite，`restart` 重启两者。
- Vite `5173` → 本机 Go/Air `8081` → Docker MySQL `127.0.0.1:3307`。
- 开发脚本检查备份/恢复客户端，固定本机 API 代理，停止重复运行的 Docker app。
- Air 配置已保存；Go 编译失败时停止旧程序，忽略构建产物和测试文件变动。
- `stop` 会退出 Vite、Air 和 Go 子进程，保留 MySQL 容器及数据卷；`foreground` 模式可用 Ctrl+C 停止前后端。
- macOS 上 `start` 会自动打开 Docker Desktop 并等待 Docker 引擎；等待上限由 `DEV_DOCKER_STARTUP_TIMEOUT` 控制。生命周期命令日志位于 `logs/dev-stack/dev.log`。
- 本轮新增的 Docker Desktop 自动启动和生命周期命令尚未端到端执行；下方停止/重启记录是本验证文档既有的开发栈验证结果。
- 本机通过 Homebrew 安装 MariaDB 客户端，数据库继续使用原有 Docker MySQL 8.0；未启动 Homebrew 数据库服务。

## 备份实现

- 支持 `mariadb-dump`/`mysqldump`、`mariadb`/`mysql`，兼容 MariaDB 旧名符号链接。
- 根据实际客户端选择 SSL 参数，避免 MySQL 与 MariaDB 参数混用。
- 导出启用 `--single-transaction --quick --hex-blob --no-tablespaces`，保存 InnoDB 一致性快照和图片等二进制内容。
- 导入检查临时文件写入完成，浏览器超时晚于后端任务期限。
- 页面解析 Blob 内的 JSON 错误，重复错误提示合并；生成期间阻止重复请求。
- 导出和恢复仅允许管理员访问。

## 已完成检查

1. `bash -n dev.sh scripts/dev.sh`、`git diff --check` 通过。
2. Go 后端构建和 `npm run build` 通过。
3. 客户端发现、MariaDB/MySQL SSL 参数和符号链接兼容测试通过。
4. 备份/恢复未登录返回 401、员工返回 403 的路由测试通过。
5. 通过真实开发代理 `http://localhost:5173/api/system/backup` 导出成功：HTTP 200，10,902,421 字节，46 张表，SQL 有完整结束标记。
6. 创建唯一命名且以 `_test` 结尾的临时数据库，启动绑定 `127.0.0.1:18081` 的隔离 API，关闭所有 Agent Worker。通过实际 `/api/system/restore` 上传该 SQL，返回 HTTP 200。
7. 临时库的表清单与开发库一致，46 张表的 `CHECKSUM TABLE ... EXTENDED` 全部匹配。验证期间源开发库的校验和未变化。
8. 隔离 API、临时库及 SQL 文件均已清理；原开发库没有执行恢复覆盖。
9. 实际执行停止/重启：停止后 `5173` 和 `8081` 均无残留监听，MySQL 容器 ID 和启动时间保持不变；重启后前端及 API 代理就绪检查通过。

现有 controllers 测试包会在初始化时连接审计数据库，因此本次 Go 回归测试也使用单独创建并在结束后删除的 `_test` 数据库，显式配置 `ERP_AUDIT_DB=1`、`DB_HOST=127.0.0.1` 和 `DB_PORT=3307`。

验证范围为本机开发环境，未连接或更新软路由。
