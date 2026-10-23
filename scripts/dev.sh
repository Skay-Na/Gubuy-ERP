#!/usr/bin/env bash
set -Eeuo pipefail

# Start the local development stack:
#   - MySQL in Docker (the named volume is kept when this script exits)
#   - Go API on the host with Air live reload
#   - Vue/Vite dev server on the host
#
# Lifecycle commands:
#   ./dev.sh [start|status|stop|restart]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKEND_DIR="$REPO_ROOT/erp-backend"
STATE_DIR="$REPO_ROOT/logs/dev-stack"
PID_FILE="$STATE_DIR/runner.pid"
LOG_FILE="$STATE_DIR/dev.log"
LOCK_DIR="$STATE_DIR/control.lock"

AIR_PID=""
VITE_PID=""

log() {
  printf '\n\033[1;34m==> %s\033[0m\n' "$*"
}

fail() {
  printf '\n\033[1;31m错误：%s\033[0m\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
用法：./dev.sh [start|status|stop|restart|foreground]

  start       在后台启动 MySQL、Go/Air 和 Vite（默认）
  status      查看开发服务和 MySQL 状态
  stop        停止 Go/Air 和 Vite，保留 MySQL 容器及数据卷
  restart     重启 Go/Air 和 Vite
  foreground  在当前终端运行；按 Ctrl+C 停止前后端

macOS 上 start 会在需要时打开 Docker Desktop 并等待引擎就绪。
日志：logs/dev-stack/dev.log
EOF
}

load_project_env() {
  [[ -f "$REPO_ROOT/.env" ]] || return 0
  set -a
  # shellcheck disable=SC1091
  . "$REPO_ROOT/.env"
  set +a
}

configure_dev_ports() {
  DEV_API_PORT="${DEV_API_PORT:-8081}"
  DEV_VITE_PORT="${DEV_VITE_PORT:-5173}"
  MYSQL_HOST_PORT="${MYSQL_HOST_PORT:-3307}"
}

ensure_docker_daemon() {
  local timeout="${DEV_DOCKER_STARTUP_TIMEOUT:-180}"
  local elapsed=0
  local context

  if docker info >/dev/null 2>&1; then
    log "Docker 引擎已就绪"
    return 0
  fi

  case "$(uname -s)" in
    Darwin)
      if ! command -v open >/dev/null 2>&1 || ! open -Ra Docker >/dev/null 2>&1; then
        fail "Docker CLI 已安装，但未找到 Docker Desktop。请安装并启动 Docker Desktop 后重试。"
      fi
      log "Docker 引擎未启动，正在打开 Docker Desktop"
      if ! open -a Docker >/dev/null 2>&1; then
        fail "无法打开 Docker Desktop。请手动启动 Docker Desktop 后重试。"
      fi
      ;;
    *)
      fail "Docker CLI 已安装，但 Docker 引擎不可用。请先启动 Docker Desktop 或 Docker 服务，再重试。"
      ;;
  esac

  while (( elapsed < timeout )); do
    sleep 2
    elapsed=$((elapsed + 2))
    if docker info >/dev/null 2>&1; then
      log "Docker 引擎已就绪（等待 ${elapsed} 秒）"
      return 0
    fi
    if (( elapsed % 10 == 0 )); then
      log "等待 Docker Desktop 启动（${elapsed}/${timeout} 秒）"
    fi
  done

  context="$(docker context show 2>/dev/null || printf '未知')"
  fail "等待 Docker Desktop ${timeout} 秒后仍无法连接（当前 context：${context}）。请检查 Docker Desktop 状态，或设置 DEV_DOCKER_STARTUP_TIMEOUT 后重试。"
}

runner_matches_pid() {
  local pid="$1"
  local command_line
  local process_state

  [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null || return 1
  process_state="$(ps -p "$pid" -o stat= 2>/dev/null || true)"
  [[ "$process_state" == Z* ]] && return 1
  command_line="$(ps -p "$pid" -o command= 2>/dev/null || true)"
  case "$command_line" in
    *"$SCRIPT_DIR/dev.sh"*"_run"*|*"$SCRIPT_DIR/dev.sh"*"foreground"*) return 0 ;;
    *) return 1 ;;
  esac
}

read_runner_pid() {
  [[ -f "$PID_FILE" ]] || return 1
  cat "$PID_FILE"
}

remove_runner_pid_if_owned() {
  local pid_file_pid=""
  [[ -f "$PID_FILE" ]] && pid_file_pid="$(cat "$PID_FILE" 2>/dev/null || true)"
  if [[ "$pid_file_pid" == "$$" ]]; then
    rm -f "$PID_FILE"
  fi
}

acquire_control_lock() {
  local attempts=0
  local owner=""
  mkdir -p "$STATE_DIR"
  while ! mkdir "$LOCK_DIR" 2>/dev/null; do
    owner=""
    [[ -f "$LOCK_DIR/owner" ]] && owner="$(cat "$LOCK_DIR/owner" 2>/dev/null || true)"
    if [[ -n "$owner" ]] && ! kill -0 "$owner" 2>/dev/null; then
      rm -rf "$LOCK_DIR"
      continue
    fi
    if (( attempts >= 50 )); then
      fail "另一个 dev.sh 生命周期命令仍在执行；如确认没有命令运行，请移除 $LOCK_DIR 后重试。"
    fi
    sleep 0.1
    attempts=$((attempts + 1))
  done
  printf '%s\n' "$$" > "$LOCK_DIR/owner"
}

release_control_lock() {
  local owner=""
  [[ -f "$LOCK_DIR/owner" ]] && owner="$(cat "$LOCK_DIR/owner" 2>/dev/null || true)"
  [[ "$owner" == "$$" ]] && rm -rf "$LOCK_DIR"
}

process_children() {
  ps -axo pid=,ppid= 2>/dev/null | awk -v parent="$1" '$2 == parent { print $1 }'
}

terminate_process_tree() {
  local pid="$1"
  local children child attempt

  runner_matches_pid "$pid" || {
    kill -0 "$pid" 2>/dev/null || return 0
    local process_state
    process_state="$(ps -p "$pid" -o stat= 2>/dev/null || true)"
    [[ "$process_state" == Z* ]] && return 0
  }
  children="$(process_children "$pid" || true)"
  for child in $children; do
    terminate_process_tree "$child"
  done

  kill -TERM "$pid" 2>/dev/null || true
  for ((attempt = 0; attempt < 50; attempt++)); do
    kill -0 "$pid" 2>/dev/null || return 0
    local process_state
    process_state="$(ps -p "$pid" -o stat= 2>/dev/null || true)"
    [[ "$process_state" == Z* ]] && return 0
    sleep 0.1
  done
  kill -KILL "$pid" 2>/dev/null || true
}

service_responds() {
  local url="$1"
  command -v curl >/dev/null 2>&1 && \
    curl --noproxy '*' --connect-timeout 1 --max-time 2 --silent --fail --output /dev/null "$url" 2>/dev/null
}

status_stack() {
  local runner_pid=""
  local api_url vite_url proxy_url
  local status=0

  load_project_env
  configure_dev_ports
  api_url="http://127.0.0.1:${DEV_API_PORT}/api/admin/status"
  vite_url="http://localhost:${DEV_VITE_PORT}/"
  proxy_url="http://localhost:${DEV_VITE_PORT}/api/admin/status"

  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    printf 'Docker 引擎：就绪（context：%s）\n' "$(docker context show 2>/dev/null || printf '未知')"
  else
    printf 'Docker 引擎：未就绪（macOS 上运行 start 会自动打开 Docker Desktop）\n'
    status=1
  fi

  if runner_pid="$(read_runner_pid 2>/dev/null)" && runner_matches_pid "$runner_pid"; then
    printf '开发进程：运行中（PID %s）\n' "$runner_pid"
  else
    printf '开发进程：未运行'
    if [[ -n "$runner_pid" ]]; then
      printf '（清理过期 PID 记录）'
      rm -f "$PID_FILE"
    fi
    printf '\n'
    status=1
  fi

  if service_responds "$api_url"; then
    printf 'Go API：就绪 %s\n' "$api_url"
  else
    printf 'Go API：未就绪 %s\n' "$api_url"
    status=1
  fi
  if service_responds "$vite_url"; then
    printf 'Vite：就绪 %s\n' "$vite_url"
  else
    printf 'Vite：未就绪 %s\n' "$vite_url"
    status=1
  fi
  if service_responds "$proxy_url"; then
    printf 'API 代理：就绪 %s\n' "$proxy_url"
  else
    printf 'API 代理：未就绪 %s\n' "$proxy_url"
    status=1
  fi

  if command -v docker >/dev/null 2>&1 && docker compose ps --status running --services 2>/dev/null | grep -Fxq db; then
    printf 'MySQL：运行中（127.0.0.1:%s）\n' "$MYSQL_HOST_PORT"
  else
    printf 'MySQL：未运行（预期端口 127.0.0.1:%s）\n' "$MYSQL_HOST_PORT"
    status=1
  fi
  printf '日志：%s\n' "$LOG_FILE"
  return "$status"
}

stop_stack() {
  local runner_pid=""
  local attempt
  acquire_control_lock
  if ! runner_pid="$(read_runner_pid 2>/dev/null)"; then
    printf '开发服务未运行；MySQL 容器及数据卷未更改。\n'
    release_control_lock
    return 0
  fi
  if ! runner_matches_pid "$runner_pid"; then
    rm -f "$PID_FILE"
    printf '已清理过期 PID 记录；未找到受管理的开发服务。\n'
    release_control_lock
    return 0
  fi

  log "停止开发服务（PID ${runner_pid}）"
  terminate_process_tree "$runner_pid"
  for ((attempt = 0; attempt < 50; attempt++)); do
    runner_matches_pid "$runner_pid" || break
    sleep 0.1
  done
  if runner_matches_pid "$runner_pid"; then
    release_control_lock
    fail "开发进程未能停止，请查看日志：$LOG_FILE"
  fi
  rm -f "$PID_FILE"
  printf 'Go/Air 与 Vite 已停止；MySQL 容器及数据卷保留。\n'
  release_control_lock
}

wait_for_managed_start() {
  local runner_pid="$1"
  local elapsed=0
  local timeout="${DEV_TOTAL_STARTUP_TIMEOUT:-600}"

  while (( elapsed <= timeout )); do
    if service_responds "http://127.0.0.1:${DEV_API_PORT}/api/admin/status" && \
      service_responds "http://localhost:${DEV_VITE_PORT}/api/admin/status"; then
      printf '开发环境已就绪：\n  管理员端：http://localhost:%s/admin\n  员工端：http://localhost:%s/terminal\n' \
        "$DEV_VITE_PORT" "$DEV_VITE_PORT"
      printf '日志：%s\n' "$LOG_FILE"
      return 0
    fi
    if ! runner_matches_pid "$runner_pid"; then
      printf '开发服务启动失败，最近日志如下：\n' >&2
      tail -n 60 "$LOG_FILE" >&2 2>/dev/null || true
      return 1
    fi
    if (( elapsed > 0 && elapsed % 10 == 0 )); then
      printf '开发环境仍在启动（%s 秒），进度日志：%s\n' "$elapsed" "$LOG_FILE"
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done

  printf '开发服务在 %s 秒内未就绪，最近日志如下：\n' "$timeout" >&2
  tail -n 60 "$LOG_FILE" >&2 2>/dev/null || true
  stop_stack || true
  return 1
}

start_stack() {
  local runner_pid=""
  local attempts=0

  load_project_env
  configure_dev_ports
  acquire_control_lock
  if runner_pid="$(read_runner_pid 2>/dev/null)" && runner_matches_pid "$runner_pid"; then
    release_control_lock
    printf '开发进程已在运行（PID %s），检查服务状态……\n' "$runner_pid"
    wait_for_managed_start "$runner_pid"
    return $?
  fi
  rm -f "$PID_FILE"
  mkdir -p "$STATE_DIR"
  touch "$LOG_FILE"
  log "后台启动开发环境"
  nohup bash "$SCRIPT_DIR/dev.sh" _run >> "$LOG_FILE" 2>&1 < /dev/null &
  runner_pid=$!
  while (( attempts < 50 )); do
    if [[ -f "$PID_FILE" ]] && [[ "$(cat "$PID_FILE" 2>/dev/null || true)" == "$runner_pid" ]]; then
      break
    fi
    if ! kill -0 "$runner_pid" 2>/dev/null; then
      break
    fi
    sleep 0.1
    attempts=$((attempts + 1))
  done
  release_control_lock

  if ! runner_matches_pid "$runner_pid"; then
    printf '开发环境未能启动，最近日志如下：\n' >&2
    tail -n 60 "$LOG_FILE" >&2 2>/dev/null || true
    return 1
  fi
  wait_for_managed_start "$runner_pid"
}

run_stack() {
  if [[ "${1:-}" == _run || "${1:-}" == foreground ]]; then
    mkdir -p "$STATE_DIR"
    if [[ "${1:-}" == foreground ]]; then
      local existing_pid=""
      acquire_control_lock
      if existing_pid="$(read_runner_pid 2>/dev/null)" && runner_matches_pid "$existing_pid"; then
        release_control_lock
        fail "开发服务已由 PID ${existing_pid} 管理；请先运行 ./dev.sh stop。"
      fi
      rm -f "$PID_FILE"
    fi
    printf '%s\n' "$$" > "$PID_FILE.$$.tmp"
    mv "$PID_FILE.$$.tmp" "$PID_FILE"
    if [[ "${1:-}" == foreground ]]; then
      release_control_lock
    fi
  fi

cleanup() {
  local exit_code=$?
  trap - EXIT INT TERM

  if [[ -n "$VITE_PID" ]]; then
    terminate_process_tree "$VITE_PID"
    wait "$VITE_PID" 2>/dev/null || true
  fi
  if [[ -n "$AIR_PID" ]]; then
    terminate_process_tree "$AIR_PID"
    wait "$AIR_PID" 2>/dev/null || true
  fi
  remove_runner_pid_if_owned

  exit "$exit_code"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

command -v docker >/dev/null 2>&1 || fail "未找到 Docker CLI"
command -v go >/dev/null 2>&1 || fail "未找到 Go，请安装 Go 1.26 或确保 GOTOOLCHAIN=auto 可用"
command -v npm >/dev/null 2>&1 || fail "未找到 npm"
command -v curl >/dev/null 2>&1 || fail "未找到 curl，无法检查本地服务是否启动成功"
command -v lsof >/dev/null 2>&1 || fail "未找到 lsof，无法检查开发端口是否被占用"

[[ -f "$REPO_ROOT/.env" ]] || fail \
  "未找到项目根目录 .env。请先复制 .env.example 为 .env 并设置至少 32 位的 JWT_SECRET。"
[[ -x "$REPO_ROOT/node_modules/.bin/vite" ]] || fail \
  "前端依赖尚未安装，请先在项目根目录运行 npm install。"

# The project .env contains the same KEY=value entries consumed by Compose.
# Export them so the host-run Go process receives JWT and database settings too.
set -a
# shellcheck disable=SC1091
. "$REPO_ROOT/.env"
set +a

# Homebrew's versioned/client-only formulae may be keg-only.
if command -v brew >/dev/null 2>&1; then
  brew_prefix="$(brew --prefix)"
  for client_dir in "$brew_prefix"/opt/mariadb*/bin "$brew_prefix"/opt/mysql*/bin; do
    if [[ -d "$client_dir" ]]; then
      export PATH="$PATH:$client_dir"
    fi
  done
fi

BACKUP_CLIENT="$(command -v mariadb-dump || command -v mysqldump || true)"
RESTORE_CLIENT="$(command -v mariadb || command -v mysql || true)"
[[ -n "$BACKUP_CLIENT" && -n "$RESTORE_CLIENT" ]] || fail \
  "缺少 SQL 备份/恢复客户端。macOS 可运行 brew install mariadb；Linux 请安装 mariadb-client 或 mysql-client，再重启开发环境。"

DEV_API_PORT="${DEV_API_PORT:-8081}"
DEV_VITE_PORT="${DEV_VITE_PORT:-5173}"
MYSQL_HOST_PORT="${MYSQL_HOST_PORT:-3307}"

[[ -n "${JWT_SECRET:-}" && ${#JWT_SECRET} -ge 32 ]] || fail \
  ".env 中的 JWT_SECRET 不能为空且至少需要 32 字节。"

[[ "$DEV_API_PORT" != "$DEV_VITE_PORT" ]] || fail \
  "DEV_API_PORT 和 DEV_VITE_PORT 不能使用同一个端口。"

require_free_port() {
  local port="$1"
  local service="$2"
  local listeners

  [[ "$port" =~ ^[0-9]{1,5}$ ]] && ((10#$port >= 1 && 10#$port <= 65535)) || fail \
    "${service}端口配置无效：${port}。端口范围必须是 1–65535。"

  if listeners="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null)"; then
    printf '\n端口 %s（%s）已被占用：\n%s\n' "$port" "$service" "$listeners" >&2
    fail "请停止占用该端口的进程，或在 .env 中修改 DEV_API_PORT / DEV_VITE_PORT 后重试。"
  fi
}

# Check before Docker starts so a port conflict cannot leave a partial stack running.
require_free_port "$DEV_API_PORT" "Go API"
require_free_port "$DEV_VITE_PORT" "Vite"

ensure_docker_daemon

export DB_HOST="127.0.0.1"
DB_PORT="$MYSQL_HOST_PORT"
export DB_PORT
export DB_USER="${DB_USER:-root}"
export DB_PASS="${DB_PASS:-${MYSQL_ROOT_PASSWORD:-rootpassword}}"
export DB_NAME="${DB_NAME:-erp_db}"
export BACKEND_LISTEN_ADDR=":$DEV_API_PORT"
export VITE_API_PROXY_TARGET="http://127.0.0.1:$DEV_API_PORT"
export VITE_API_BASE_URL=""

cd "$REPO_ROOT"
if ! docker compose config --quiet; then
  fail "Docker Compose 配置校验失败，请检查 .env。"
fi

if docker compose ps --status running --services 2>/dev/null | grep -Fxq app; then
  log "停止本地打包版 app，避免与开发 API 同时执行后台任务"
  docker compose stop app
fi

DB_PORT="${DB_PORT:-3307}"
log "启动并等待本地 MySQL（端口 ${DB_PORT}）"
docker compose up -d --wait db
log "本机开发数据源：127.0.0.1:${DB_PORT}/${DB_NAME}"
log "数据库客户端已就绪：${BACKUP_CLIENT} / ${RESTORE_CLIENT}"

AIR_BIN="$(command -v air || true)"
if [[ -z "$AIR_BIN" ]]; then
  GOPATH="$(go env GOPATH)"
  AIR_BIN="$GOPATH/bin/air"
  if [[ ! -x "$AIR_BIN" ]]; then
    log "首次启动需要安装 Go 热重载工具 Air"
    (cd "$REPO_ROOT" && go install github.com/air-verse/air@v1.67.4)
  fi
fi
[[ -x "$AIR_BIN" ]] || fail "Air 安装失败，请确认 $(go env GOPATH)/bin 在 PATH 中。"

if [[ ! -f "$BACKEND_DIR/docs/docs.go" ]]; then
  log "首次启动生成 Swagger 文档"
  (cd "$BACKEND_DIR" && go generate .)
fi

log "启动 Go API 热重载（http://127.0.0.1:${DEV_API_PORT}）"
(
  cd "$BACKEND_DIR"
  exec "$AIR_BIN" -c .air.toml
) &
AIR_PID=$!

log "启动 Vue/Vite（http://localhost:${DEV_VITE_PORT}）"
(
  cd "$REPO_ROOT"
  exec "$REPO_ROOT/node_modules/.bin/vite" --port "$DEV_VITE_PORT" --strictPort
) &
VITE_PID=$!

wait_for_http() {
  local service="$1"
  local url="$2"
  local pid="$3"
  local elapsed=0
  local timeout="${DEV_STARTUP_TIMEOUT:-180}"
  local status

  log "等待 ${service} 就绪"
  while ! curl --noproxy '*' --connect-timeout 1 --max-time 2 --silent --fail --output /dev/null "$url" 2>/dev/null; do
    if ! kill -0 "$pid" 2>/dev/null; then
      if wait "$pid"; then
        fail "${service} 已退出。"
      else
        status=$?
        fail "${service} 异常退出（状态码 ${status}）。"
      fi
    fi
    if (( elapsed >= timeout )); then
      fail "${service} 在 ${timeout} 秒内未就绪（${url}）。可设置 DEV_STARTUP_TIMEOUT 延长等待时间。"
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
}

wait_for_http "Go API" "http://127.0.0.1:${DEV_API_PORT}/api/admin/status" "$AIR_PID"
wait_for_http "Vite" "http://localhost:${DEV_VITE_PORT}/" "$VITE_PID"
if ! curl --noproxy '*' --connect-timeout 2 --max-time 5 --silent --fail --output /dev/null \
  "http://localhost:${DEV_VITE_PORT}/api/admin/status"; then
  fail "Vite 已启动，但 API 代理请求失败，请检查 vite.config.js 中的代理配置。"
fi

open_dev_pages() {
  local admin_url="http://localhost:${DEV_VITE_PORT}/admin"
  local staff_url="http://localhost:${DEV_VITE_PORT}/terminal"

  case "$(uname -s)" in
    Darwin)
      if open "$admin_url" "$staff_url"; then
        log "已在默认浏览器打开管理员端和员工端"
      else
        printf '警告：浏览器未能打开，请手动访问：\n  管理员端：%s\n  员工端：%s\n' \
          "$admin_url" "$staff_url" >&2
      fi
      ;;
    Linux)
      if command -v xdg-open >/dev/null 2>&1; then
        xdg-open "$admin_url" >/dev/null 2>&1 &
        xdg-open "$staff_url" >/dev/null 2>&1 &
        log "正在默认浏览器打开管理员端和员工端"
      else
        printf '警告：未找到 xdg-open，请手动访问：\n  管理员端：%s\n  员工端：%s\n' \
          "$admin_url" "$staff_url" >&2
      fi
      ;;
    *)
      printf '提示：请在浏览器中打开：\n  管理员端：%s\n  员工端：%s\n' \
        "$admin_url" "$staff_url"
      ;;
  esac
}

log "开发环境已启动；修改 Vue 会即时热更新，修改 Go 会自动重编译并重启。"
if [[ "${DEV_OPEN_BROWSER:-1}" == 1 ]]; then
  open_dev_pages
fi
if [[ "${1:-}" == foreground ]]; then
  printf '%s\n' '按 Ctrl+C 停止前后端开发进程。MySQL 容器和数据卷会保留。'
else
  printf '%s\n' '使用 ./dev.sh stop 停止 Go/Air 与 Vite；MySQL 容器和数据卷会保留。'
fi

# macOS ships Bash 3.2, so poll both children instead of relying on wait -n.
while true; do
  if ! kill -0 "$AIR_PID" 2>/dev/null; then
    if wait "$AIR_PID"; then
      fail "Air 已退出。"
    else
      status=$?
      fail "Air 异常退出（状态码 ${status}）。"
    fi
  fi
  if ! kill -0 "$VITE_PID" 2>/dev/null; then
    if wait "$VITE_PID"; then
      fail "Vite 已退出。"
    else
      status=$?
      fail "Vite 异常退出（状态码 ${status}）。"
    fi
  fi
  sleep 1
done
}

COMMAND="${1:-start}"
case "$COMMAND" in
  start)
    start_stack
    ;;
  status)
    status_stack
    ;;
  stop)
    stop_stack
    ;;
  restart)
    stop_stack
    start_stack
    ;;
  foreground)
    run_stack foreground
    ;;
  _run)
    run_stack _run
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    fail "未知命令：$COMMAND"
    ;;
esac
