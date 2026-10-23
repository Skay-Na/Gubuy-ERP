#!/usr/bin/env bash
set -Eeuo pipefail

# 一键构建并更新软路由；整个流程不会启动 Mac 本机的 app 容器。
# 在仓库根目录执行：./scripts/build-deploy-router.sh

APP_IMAGE="erpwonchon-app:latest"
RUN_TESTS="${RUN_TESTS:-0}"
AUDIT_DB_NAME="${AUDIT_DB_NAME:-erp_audit_test}"
ROUTER_LAN_IP="${ROUTER_LAN_IP:-192.168.5.22}"
ROUTER_TAILSCALE_IP="${ROUTER_TAILSCALE_IP:-100.83.204.35}"
ROUTER_USER="${ROUTER_USER:-root}"
REMOTE_DIR="/mnt/opt/docker/install"
REMOTE_TAR_NAME="erpwonchon-app.tar"
LOCAL_BACKUP_DIR="${LOCAL_BACKUP_DIR:-/Users/nanjiakai/Desktop}"
SSH_CONNECT_TIMEOUT="${SSH_CONNECT_TIMEOUT:-3}"
EXPECT_TIMEOUT="${EXPECT_TIMEOUT:-900}"
ROUTER_KEYCHAIN_SERVICE="${ROUTER_KEYCHAIN_SERVICE:-erpwonchon-router-ssh}"
REMOTE_PRUNE_DOCKER_IMAGES="${REMOTE_PRUNE_DOCKER_IMAGES:-0}"
# 保留近期 BuildKit 缓存；设为 0 跳过清理，设为 all 清理全部未使用缓存。
PRUNE_DOCKER_CACHE="${PRUNE_DOCKER_CACHE:-1}"
BUILD_CACHE_MAX_AGE="${BUILD_CACHE_MAX_AGE:-168h}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

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
  exit "$exit_code"
}
trap on_error ERR

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "未找到必需命令：$1"
}

resolve_path() {
  local path="$1"
  [[ "$path" = /* ]] || path="$REPO_ROOT/$path"

  local parent
  parent="$(cd "$(dirname "$path")" 2>/dev/null && pwd)" || \
    fail "镜像包所在目录不存在：$(dirname "$path")"
  printf '%s/%s\n' "$parent" "$(basename "$path")"
}

ssh_port_open() {
  nc -z -G "$SSH_CONNECT_TIMEOUT" "$1" 22 >/dev/null 2>&1
}

select_router_host() {
  printf '尝试软路由局域网地址 %s…\n' "$ROUTER_LAN_IP" >&2
  if ssh_port_open "$ROUTER_LAN_IP"; then
    printf '使用局域网地址：%s\n' "$ROUTER_LAN_IP" >&2
    printf '%s\n' "$ROUTER_LAN_IP"
    return 0
  fi

  printf '局域网 SSH 不可达，尝试 Tailscale 地址 %s…\n' "$ROUTER_TAILSCALE_IP" >&2
  if ssh_port_open "$ROUTER_TAILSCALE_IP"; then
    printf '使用 Tailscale 地址：%s\n' "$ROUTER_TAILSCALE_IP" >&2
    printf '%s\n' "$ROUTER_TAILSCALE_IP"
    return 0
  fi

  printf '错误：局域网地址 %s 与 Tailscale 地址 %s 的 SSH 均不可达。\n' \
    "$ROUTER_LAN_IP" "$ROUTER_TAILSCALE_IP" >&2
  return 1
}

run_with_password() {
  ROUTER_PASS="$ROUTER_PASS" EXPECT_TIMEOUT="$EXPECT_TIMEOUT" expect -f - "$@" <<'EXPECT_SCRIPT'
set timeout $env(EXPECT_TIMEOUT)
set password_prompts 0
# Do not print argv: the SSH command contains the full remote deployment script.
spawn -noecho {*}$argv
expect {
    -re "(?i)password:" {
        incr password_prompts
        if {$password_prompts > 3} {
            puts stderr "密码验证失败，请检查 ROUTER_PASS"
            exit 1
        }
        send -- "$env(ROUTER_PASS)\r"
        exp_continue
    }
    timeout {
        puts stderr "SSH/SCP 操作超时"
        exit 124
    }
    eof
}
set result [wait]
set exit_code [lindex $result 3]
if {![string is integer -strict $exit_code]} { exit 1 }
exit $exit_code
EXPECT_SCRIPT
}

deploy_to_router() {
  mkdir -p "$LOCAL_BACKUP_DIR"

  log "5/6 备份旧镜像并上传新镜像包"
  printf '备份目录：%s\n' "$LOCAL_BACKUP_DIR"
  run_with_password scp -o "ConnectTimeout=$SSH_CONNECT_TIMEOUT" -o StrictHostKeyChecking=accept-new \
    "$ROUTER_USER@$ROUTER_IP:$REMOTE_DIR/*.tar" "$LOCAL_BACKUP_DIR/" || \
    fail "软路由旧镜像备份失败，更新已停止。"

  printf '上传位置：%s/%s\n' "$REMOTE_DIR" "$REMOTE_TAR_NAME"
  run_with_password scp -o "ConnectTimeout=$SSH_CONNECT_TIMEOUT" -o StrictHostKeyChecking=accept-new \
    "$TAR_PATH" "$ROUTER_USER@$ROUTER_IP:$REMOTE_DIR/$REMOTE_TAR_NAME" || \
    fail "镜像上传失败，软路由容器未更新。"

  local remote_update
  remote_update='
set -eu
cd /mnt/opt/docker/install
if ! target_image=$(docker inspect --format "{{.Config.Image}}" gubuy-erp-app 2>/dev/null); then
  echo "未找到软路由现有 ERP app 容器，无法继承其镜像名称" >&2
  exit 1
fi
case "${target_image:-}" in
  ""|sha256:*|*@sha256:*) echo "无法识别现有 app 镜像名：${target_image:-<空值>}" >&2; exit 1 ;;
esac
docker load -i erpwonchon-app.tar
new_image_id=$(docker image inspect --format "{{.Id}}" erpwonchon-app:latest)
docker image tag erpwonchon-app:latest "$target_image"
# 服务端密钥只保存在软路由上，不进入镜像包。
ai_secret_file=.erp-ai-config-secret
jwt_secret_file=.erp-jwt-secret
override_file=.erp-runtime-secrets.compose.yml
current_env=$(docker inspect --format "{{range .Config.Env}}{{println .}}{{end}}" gubuy-erp-app)
current_ai_secret=$(printf "%s\n" "$current_env" | sed -n "s/^AI_CONFIG_SECRET=//p" | tail -n 1)
current_jwt_secret=$(printf "%s\n" "$current_env" | sed -n "s/^JWT_SECRET=//p" | tail -n 1)
umask 077
if [ -s "$ai_secret_file" ]; then
  ai_secret=$(cat "$ai_secret_file")
else
  ai_secret=$current_ai_secret
  if [ "${#ai_secret}" -lt 24 ] ||
    [ "$ai_secret" = please_change_this_secret ] ||
    [ "$ai_secret" = erp_wonchon_default_secret_key_2026_please_change ]; then
    ai_secret=$(LC_ALL=C tr -dc "A-Za-z0-9" < /dev/urandom | head -c 48)
  fi
  printf "%s" "$ai_secret" > "$ai_secret_file"
fi
if [ "${#ai_secret}" -lt 24 ]; then
  echo "AI 加密密钥文件无效：$ai_secret_file" >&2
  exit 1
fi
chmod 600 "$ai_secret_file"

if [ -s "$jwt_secret_file" ]; then
  jwt_secret=$(cat "$jwt_secret_file")
else
  jwt_secret=$current_jwt_secret
  if [ "${#jwt_secret}" -lt 32 ] ||
    [ "$jwt_secret" = please_change_this_secret ] ||
    [ "$jwt_secret" = erp_wonchon_default_secret_key_2026_please_change ]; then
    jwt_secret=$(LC_ALL=C tr -dc "A-Za-z0-9" < /dev/urandom | head -c 64)
  fi
  printf "%s" "$jwt_secret" > "$jwt_secret_file"
fi
if [ "${#jwt_secret}" -lt 32 ] ||
  [ "$jwt_secret" = please_change_this_secret ] ||
  [ "$jwt_secret" = erp_wonchon_default_secret_key_2026_please_change ]; then
  echo "JWT 密钥文件无效：$jwt_secret_file（需至少 32 字节）" >&2
  exit 1
fi
chmod 600 "$jwt_secret_file"

printf "services:\n  app:\n    environment:\n      AI_CONFIG_SECRET: %s\n      JWT_SECRET: %s\n" \
  "$ai_secret" "$jwt_secret" > "$override_file"
chmod 600 "$override_file"
set -- -f docker-compose.yml
set -- "$@" -f "$override_file"
if [ -f .erp-app-dns.compose.yml ]; then set -- "$@" -f .erp-app-dns.compose.yml; fi
if command -v docker-compose >/dev/null 2>&1; then
  docker-compose "$@" up -d --no-build --no-deps --force-recreate app
else
  docker compose "$@" up -d --no-build --no-deps --force-recreate app
fi
running_image_id=$(docker inspect --format "{{.Image}}" gubuy-erp-app)
if [ "$running_image_id" != "$new_image_id" ]; then
  echo "容器仍使用旧镜像：$running_image_id；期望：$new_image_id" >&2
  exit 1
fi
status=unknown
for attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
  status=$(docker inspect --format "{{.State.Status}}" gubuy-erp-app)
  [ "$status" = running ] && break
  sleep 2
done
if [ "$status" != running ]; then
  echo "容器未进入 running 状态（当前：$status），最近日志如下：" >&2
  docker logs --tail 80 gubuy-erp-app >&2 || true
  exit 1
fi
container_env=$(docker inspect --format "{{range .Config.Env}}{{println .}}{{end}}" gubuy-erp-app)
if ! printf "%s\n" "$container_env" | grep -Fqx "AI_CONFIG_SECRET=$ai_secret"; then
  echo "容器未收到 AI_CONFIG_SECRET" >&2
  exit 1
fi
if ! printf "%s\n" "$container_env" | grep -Fqx "JWT_SECRET=$jwt_secret"; then
  echo "容器未收到 JWT_SECRET" >&2
  exit 1
fi
echo "镜像校验通过：$new_image_id"
echo "AI_CONFIG_SECRET 与 JWT_SECRET 已持久保存并注入容器"
if [ "__REMOTE_PRUNE__" = 1 ]; then docker image prune --force --filter dangling=true; fi
'
  remote_update="${remote_update//__REMOTE_PRUNE__/$REMOTE_PRUNE_DOCKER_IMAGES}"

  log "6/6 更新软路由 app 并校验运行状态"
  run_with_password ssh -o "ConnectTimeout=$SSH_CONNECT_TIMEOUT" -o StrictHostKeyChecking=accept-new \
    "$ROUTER_USER@$ROUTER_IP" "$remote_update" || \
    fail "软路由容器更新失败，请查看上方 SSH/Docker 错误。"

  run_with_password ssh -o "ConnectTimeout=$SSH_CONNECT_TIMEOUT" -o StrictHostKeyChecking=accept-new \
    "$ROUTER_USER@$ROUTER_IP" \
    "docker ps --filter name=gubuy-erp-app --format '{{.Names}} {{.Image}} {{.Status}}'" || \
    fail "容器更新完成，但无法读取软路由容器状态。"
}

cleanup_docker_cache() {
  case "$PRUNE_DOCKER_CACHE" in
    0|false|FALSE|no|NO)
      log "跳过 Docker 缓存清理（PRUNE_DOCKER_CACHE=${PRUNE_DOCKER_CACHE}）"
      ;;
    1|true|TRUE|yes|YES)
      log "回收 ${BUILD_CACHE_MAX_AGE} 前的未使用 BuildKit 缓存及悬空镜像"
      docker builder prune --force --filter "until=${BUILD_CACHE_MAX_AGE}"
      docker image prune --force --filter "until=${BUILD_CACHE_MAX_AGE}"
      ;;
    all)
      log "回收全部未使用 BuildKit 缓存及悬空镜像"
      docker builder prune --force
      docker image prune --force
      ;;
    *)
      fail "PRUNE_DOCKER_CACHE 仅支持 0、1 或 all（当前：${PRUNE_DOCKER_CACHE}）"
      ;;
  esac
}

# 统一两个旧脚本使用过的产物路径变量，并确保构建和部署指向同一个 tar。
requested_tar_path="${TAR_PATH:-${LOCAL_TAR_FILE:-erpwonchon-app.tar}}"
requested_local_tar_file="${LOCAL_TAR_FILE:-$requested_tar_path}"
TAR_PATH="$(resolve_path "$requested_tar_path")"
LOCAL_TAR_FILE="$(resolve_path "$requested_local_tar_file")"
[[ "$TAR_PATH" == "$LOCAL_TAR_FILE" ]] || fail \
  "TAR_PATH 与 LOCAL_TAR_FILE 指向不同文件。请让它们保持一致，或只设置其中一个。"
export TAR_PATH LOCAL_TAR_FILE

cd "$REPO_ROOT"

for command_name in npm go docker expect scp ssh nc tar; do
  require_command "$command_name"
done

case "$PRUNE_DOCKER_CACHE" in
  0|1|false|FALSE|no|NO|true|TRUE|yes|YES|all) ;;
  *) fail "PRUNE_DOCKER_CACHE 仅支持 0、1 或 all（当前：${PRUNE_DOCKER_CACHE}）" ;;
esac

printf '\n%s\n' '=================================================='
printf '%s\n' '开始一键构建并更新软路由'
printf '镜像包：%s\n' "$TAR_PATH"
printf '%s\n' '=================================================='

log "1/6 部署前检查：确认软路由 SSH 可达"
ROUTER_IP="$(select_router_host)"
if [[ "$ROUTER_IP" == "$ROUTER_LAN_IP" ]]; then ROUTER_CONNECTION_TYPE="局域网"; else ROUTER_CONNECTION_TYPE="Tailscale"; fi
printf '部署目标：%s（%s）\n' "$ROUTER_IP" "$ROUTER_CONNECTION_TYPE"

case "$REMOTE_PRUNE_DOCKER_IMAGES" in
  0|1) ;;
  *) fail "REMOTE_PRUNE_DOCKER_IMAGES 仅支持 0 或 1（当前：${REMOTE_PRUNE_DOCKER_IMAGES}）" ;;
esac

if [[ -z "${ROUTER_PASS:-}" ]]; then
  require_command security
  if ! ROUTER_PASS="$(security find-generic-password \
    -a "$ROUTER_USER" -s "$ROUTER_KEYCHAIN_SERVICE" -w 2>/dev/null)"; then
    fail "macOS 钥匙串中未找到软路由 SSH 密码（账户：${ROUTER_USER}，服务：${ROUTER_KEYCHAIN_SERVICE}）。"
  fi
fi
[[ -n "$ROUTER_PASS" ]] || fail "软路由 SSH 密码为空。"

printf '校验 Docker Compose 配置\n'
if ! docker compose config --quiet; then
  fail "Docker Compose 配置校验失败。请先修复上方错误；首次部署需在项目根目录 .env 中设置 JWT_SECRET（私有随机值，至少 32 字节，可用 openssl rand -hex 32 生成）。更换密钥会使现有登录会话失效。"
fi

log "2/6 构建校验：前端与后端"
printf '前端：npm run build\n'
npm run build

printf '后端：go build .\n'
(
  cd erp-backend
  # 不使用 ./...：erp-backend/scripts 下是多个独立维护工具，不能作为一个 Go package 一起构建。
  go build .
)

if [[ "$RUN_TESTS" == "1" ]]; then
  [[ -n "$AUDIT_DB_NAME" && "$AUDIT_DB_NAME" != "erp_db" ]] || \
    fail "RUN_TESTS=1 时必须指定非生产审计库（默认：erp_audit_test），禁止对 erp_db 跑测试"
  log "可选审计测试：DB_NAME=${AUDIT_DB_NAME} go test ./controllers"
  (
    cd erp-backend
    DB_NAME="$AUDIT_DB_NAME" go test ./controllers
  )
else
  log "跳过后端测试（默认不触碰数据库；如需执行：RUN_TESTS=1 ./scripts/build-deploy-router.sh）"
fi

log "3/6 构建 Docker 镜像：app"
if ! docker compose build app; then
  warn "常规构建失败，尝试 --pull=false（适合网络较差但本地已有基础镜像的情况）..."
  docker compose build --pull=false app
fi

log "4/6 导出镜像包并清理本地构建缓存"
docker save "$APP_IMAGE" -o "$TAR_PATH"
tar -tf "$TAR_PATH" manifest.json >/dev/null 2>&1 || \
  fail "镜像包无效或不完整：${TAR_PATH}"
printf '镜像包：%s（' "$TAR_PATH"
du -h "$TAR_PATH" | awk '{print $1 "）"}'

cleanup_docker_cache

[[ -f "$TAR_PATH" ]] || fail "未生成镜像包：${TAR_PATH}"
tar -tf "$TAR_PATH" manifest.json >/dev/null 2>&1 || fail "镜像包无效或不完整：${TAR_PATH}"

deploy_to_router

printf '\n%s\n' '=================================================='
printf '%s\n' '🎉 新镜像已构建并完成软路由部署；Mac 本机 app 容器未启动。'
printf '镜像包：%s\n' "$TAR_PATH"
printf '%s\n' '=================================================='
