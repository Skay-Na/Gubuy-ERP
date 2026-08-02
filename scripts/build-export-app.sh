#!/usr/bin/env bash
set -Eeuo pipefail

# Build, deploy, verify, and export the ERP Wonchon Docker image.
# Run from anywhere:
#   /Users/nanjiakai/Documents/Development/coder/erpwonchon/scripts/build-export-app.sh
# Or from repo root:
#   ./scripts/build-export-app.sh

APP_SERVICE="${APP_SERVICE:-app}"
APP_CONTAINER="${APP_CONTAINER:-gubuy-erp-app}"
APP_IMAGE="${APP_IMAGE:-erpwonchon-app:latest}"
APP_URL="${APP_URL:-http://localhost:8080/}"
TAR_PATH="${TAR_PATH:-erpwonchon-app.tar}"
RUN_TESTS="${RUN_TESTS:-0}"
AUDIT_DB_NAME="${AUDIT_DB_NAME:-erp_audit_test}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

log() {
  printf '\n\033[1;34m==> %s\033[0m\n' "$*"
}

warn() {
  printf '\n\033[1;33mWARN: %s\033[0m\n' "$*" >&2
}

fail() {
  printf '\n\033[1;31mERROR: %s\033[0m\n' "$*" >&2
  exit 1
}

on_error() {
  local exit_code=$?
  warn "脚本在第 ${BASH_LINENO[0]} 行失败，退出码 ${exit_code}。"
  warn "如果失败点是 docker compose build app 且提示 Docker Hub/registry 超时，通常是网络问题；网络恢复后重跑本脚本即可。"
  exit "$exit_code"
}
trap on_error ERR

command -v npm >/dev/null || fail "未找到 npm"
command -v go >/dev/null || fail "未找到 go"
command -v docker >/dev/null || fail "未找到 docker"

log "1/8 前端构建校验: npm run build"
npm run build

log "2/8 后端应用编译校验: go build ."
(
  cd erp-backend
  # 不使用 ./...：erp-backend/scripts 下是多个独立的维护工具，不能作为一个 Go package 一起构建。
  go build .
)

if [ "$RUN_TESTS" = "1" ]; then
  [ -n "$AUDIT_DB_NAME" ] && [ "$AUDIT_DB_NAME" != "erp_db" ] || \
    fail "RUN_TESTS=1 时必须指定非生产审计库（默认：erp_audit_test），禁止对 erp_db 跑测试"
  log "可选审计测试: DB_NAME=${AUDIT_DB_NAME} go test ./controllers"
  (
    cd erp-backend
    DB_NAME="$AUDIT_DB_NAME" go test ./controllers
  )
else
  log "跳过后端测试（默认不触碰数据库；如需执行：RUN_TESTS=1 ./scripts/build-export-app.sh）"
fi

log "3/8 构建 Docker 镜像: docker compose build ${APP_SERVICE}"
if ! docker compose build "$APP_SERVICE"; then
  warn "常规构建失败，尝试 --pull=false（适合网络差但本地已有基础镜像的情况）..."
  docker compose build --pull=false "$APP_SERVICE"
fi

log "4/8 更新本地容器: docker compose up -d ${APP_SERVICE}"
docker compose up -d "$APP_SERVICE"

log "5/8 查看 Compose 状态"
docker compose ps

log "6/8 查看应用日志尾部"
docker logs --tail 80 "$APP_CONTAINER"

log "7/8 等待应用就绪并探测 HTTP 首页: ${APP_URL}"
ready=false
for attempt in {1..10}; do
  if curl -fsSI --max-time 10 "$APP_URL"; then
    ready=true
    break
  fi
  warn "应用尚未就绪（第 ${attempt}/10 次），2 秒后重试..."
  sleep 2
done
[ "$ready" = true ] || fail "应用启动后仍未通过 HTTP 健康检查"

log "8/8 导出镜像: docker save ${APP_IMAGE} -o ${TAR_PATH}"
docker save "$APP_IMAGE" -o "$TAR_PATH"

log "校验 tar 包"
du -h "$TAR_PATH"
tar -tf "$TAR_PATH" | head

log "完成：容器已更新，镜像包已导出到 ${REPO_ROOT}/${TAR_PATH}"
