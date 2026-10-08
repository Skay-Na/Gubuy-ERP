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
# 保留近期 BuildKit 缓存以加速日常构建；成功构建后自动回收过期缓存。
# 设为 0 可跳过清理；设为 all 可在本次完成后回收所有未使用构建缓存。
PRUNE_DOCKER_CACHE="${PRUNE_DOCKER_CACHE:-1}"
BUILD_CACHE_MAX_AGE="${BUILD_CACHE_MAX_AGE:-168h}"

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

cleanup_docker_cache() {
  case "$PRUNE_DOCKER_CACHE" in
    0|false|FALSE|no|NO)
      log "跳过 Docker 缓存清理（PRUNE_DOCKER_CACHE=${PRUNE_DOCKER_CACHE}）"
      return
      ;;
    1|true|TRUE|yes|YES)
      log "9/9 回收 ${BUILD_CACHE_MAX_AGE} 前的未使用 BuildKit 缓存及悬空镜像"
      docker builder prune --force --filter "until=${BUILD_CACHE_MAX_AGE}"
      docker image prune --force --filter "until=${BUILD_CACHE_MAX_AGE}"
      ;;
    all)
      log "9/9 回收全部未使用 BuildKit 缓存及悬空镜像"
      docker builder prune --force
      docker image prune --force
      ;;
    *)
      fail "PRUNE_DOCKER_CACHE 仅支持 0、1 或 all（当前：${PRUNE_DOCKER_CACHE}）"
      ;;
  esac
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

log "1/9 前端构建校验: npm run build"
npm run build

log "2/9 后端应用编译校验: go build ."
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

log "3/9 构建 Docker 镜像: docker compose build ${APP_SERVICE}"
if ! docker compose build "$APP_SERVICE"; then
  warn "常规构建失败，尝试 --pull=false（适合网络差但本地已有基础镜像的情况）..."
  docker compose build --pull=false "$APP_SERVICE"
fi

log "4/9 更新本地容器: docker compose up -d ${APP_SERVICE}"
docker compose up -d "$APP_SERVICE"

log "5/9 查看 Compose 状态"
docker compose ps

log "6/9 查看应用日志尾部"
docker logs --tail 80 "$APP_CONTAINER"

log "7/9 等待应用就绪并探测 HTTP 首页: ${APP_URL}"
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

log "8/9 导出镜像: docker save ${APP_IMAGE} -o ${TAR_PATH}"
docker save "$APP_IMAGE" -o "$TAR_PATH"

log "校验 tar 包"
du -h "$TAR_PATH"
tar -tf "$TAR_PATH" | head

cleanup_docker_cache

log "完成：容器已更新，镜像包已导出到 ${REPO_ROOT}/${TAR_PATH}"
