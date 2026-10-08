#!/bin/bash
set -euo pipefail

# ==========================================
# 王冲店软路由 ERP 系统一键更新脚本
# 在店内 Wi-Fi（192.168.5.0/24）优先走局域网；其他网络走 Tailscale。
# ==========================================

ROUTER_LAN_IP="${ROUTER_LAN_IP:-192.168.5.22}"
ROUTER_TAILSCALE_IP="${ROUTER_TAILSCALE_IP:-100.83.204.35}"
ROUTER_USER="${ROUTER_USER:-root}"
REMOTE_DIR="/mnt/opt/docker/install"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LOCAL_TAR_FILE="${LOCAL_TAR_FILE:-$REPO_ROOT/erpwonchon-app.tar}"
LOCAL_BACKUP_DIR="${LOCAL_BACKUP_DIR:-/Users/nanjiakai/Desktop}"
SSH_CONNECT_TIMEOUT="${SSH_CONNECT_TIMEOUT:-3}"
EXPECT_TIMEOUT="${EXPECT_TIMEOUT:-900}"
# 默认保留旧镜像，方便手动回滚；设为 1 才清理悬空镜像。
REMOTE_PRUNE_DOCKER_IMAGES="${REMOTE_PRUNE_DOCKER_IMAGES:-0}"

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    printf '错误：缺少必需命令 %s\n' "$1" >&2
    exit 1
  }
}

is_on_store_lan() {
  local ip
  while IFS= read -r ip; do
    [[ "$ip" =~ ^192\.168\.5\.[0-9]{1,3}$ ]] && return 0
  done < <(ifconfig 2>/dev/null | awk '/inet / { print $2 }')
  return 1
}

ssh_port_open() {
  nc -z -G "$SSH_CONNECT_TIMEOUT" "$1" 22 >/dev/null 2>&1
}

select_router_host() {
  if is_on_store_lan; then
    printf '检测到店内 Wi-Fi 网段，尝试局域网地址 %s…\n' "$ROUTER_LAN_IP" >&2
    if ssh_port_open "$ROUTER_LAN_IP"; then
      printf 'Using store LAN address: %s\n' "$ROUTER_LAN_IP" >&2
      printf '%s\n' "$ROUTER_LAN_IP"
      return 0
    fi
    printf '局域网 SSH 不可达，回退到 Tailscale 地址…\n' >&2
  else
    printf '当前不在 192.168.5.0/24 店内网段，使用 Tailscale…\n' >&2
  fi

  if ssh_port_open "$ROUTER_TAILSCALE_IP"; then
    printf 'Using Tailscale address: %s\n' "$ROUTER_TAILSCALE_IP" >&2
    printf '%s\n' "$ROUTER_TAILSCALE_IP"
    return 0
  fi

  printf '错误：局域网地址 %s 与 Tailscale 地址 %s 的 SSH 均不可达。\n' \
    "$ROUTER_LAN_IP" "$ROUTER_TAILSCALE_IP" >&2
  return 1
}

if [[ "${1:-}" == "--select-host" ]]; then
  require_command ifconfig
  require_command awk
  require_command nc
  select_router_host
  exit 0
fi

require_command expect
require_command scp
require_command ssh
require_command nc
require_command ifconfig
require_command awk
require_command tar

case "$REMOTE_PRUNE_DOCKER_IMAGES" in
  0|1) ;;
  *)
    printf '错误：REMOTE_PRUNE_DOCKER_IMAGES 仅支持 0 或 1（当前：%s）\n' \
      "$REMOTE_PRUNE_DOCKER_IMAGES" >&2
    exit 1
    ;;
esac

[[ -f "$LOCAL_TAR_FILE" ]] || {
  printf '错误：未找到待部署镜像 %s。请先在项目根目录执行 Docker 构建和导出。\n' "$LOCAL_TAR_FILE" >&2
  exit 1
}
tar -tf "$LOCAL_TAR_FILE" manifest.json >/dev/null 2>&1 || {
  printf '错误：镜像包无效或不完整：%s\n' "$LOCAL_TAR_FILE" >&2
  exit 1
}

ROUTER_IP="$(select_router_host)"

if [[ -z "${ROUTER_PASS:-}" ]]; then
  read -r -s -p "请输入软路由 SSH 密码：" ROUTER_PASS </dev/tty
  printf '\n'
fi

mkdir -p "$LOCAL_BACKUP_DIR"

run_with_password() {
  ROUTER_PASS="$ROUTER_PASS" EXPECT_TIMEOUT="$EXPECT_TIMEOUT" expect -f - "$@" <<'EXPECT_SCRIPT'
set timeout $env(EXPECT_TIMEOUT)
set password_prompts 0
spawn {*}$argv
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

printf '%s\n' '=================================================='
printf '%s\n' '开始执行一键更新部署流程'
printf '目标软路由：%s（%s）\n' "$ROUTER_IP" \
  "$([[ "$ROUTER_IP" == "$ROUTER_LAN_IP" ]] && printf '店内局域网' || printf 'Tailscale')"
printf '%s\n' '=================================================='

printf '%s\n' '=> [1/4] 正在备份软路由中的旧镜像到桌面…'
run_with_password scp -o "ConnectTimeout=$SSH_CONNECT_TIMEOUT" -o StrictHostKeyChecking=accept-new \
  "$ROUTER_USER@$ROUTER_IP:$REMOTE_DIR/*.tar" "$LOCAL_BACKUP_DIR/" || {
    printf '错误：备份失败，更新已停止。\n' >&2
    exit 1
  }
printf '%s\n\n' '备份完成！'

printf '%s\n' '=> [2/4] 正在上传最新镜像到软路由…'
run_with_password scp -o "ConnectTimeout=$SSH_CONNECT_TIMEOUT" -o StrictHostKeyChecking=accept-new \
  "$LOCAL_TAR_FILE" "$ROUTER_USER@$ROUTER_IP:$REMOTE_DIR/" || {
    printf '错误：上传失败，容器未更新。\n' >&2
    exit 1
  }
printf '%s\n\n' '上传完成！'

# 远端目录可能名为 install，Compose 默认 app 镜像便是 install-app。
# 将导入镜像标记为现有容器使用的名称，再强制重建 app 并核对镜像 ID。
remote_update='
set -eu
cd /mnt/opt/docker/install
target_image=$(docker inspect --format "{{.Config.Image}}" gubuy-erp-app)
case "$target_image" in
  ""|sha256:*|*@sha256:*) echo "无法识别现有 app 镜像名：$target_image" >&2; exit 1 ;;
esac
docker load -i erpwonchon-app.tar
new_image_id=$(docker image inspect --format "{{.Id}}" erpwonchon-app:latest)
docker image tag erpwonchon-app:latest "$target_image"
# 这里生成的只是服务端加密用的随机值，不是用户填写的服务商 API Key。
# 值只保存在软路由上，不进入 Docker 镜像或本地导出的 tar 包。
secret_file=.erp-ai-config-secret
override_file=.erp-ai-config-secret.compose.yml
current_env=$(docker inspect --format "{{range .Config.Env}}{{println .}}{{end}}" gubuy-erp-app)
current_ai_secret=$(printf "%s\n" "$current_env" | sed -n "s/^AI_CONFIG_SECRET=//p" | tail -n 1)
current_jwt_secret=$(printf "%s\n" "$current_env" | sed -n "s/^JWT_SECRET=//p" | tail -n 1)
use_override=0
if [ -s "$secret_file" ] || { { [ "${#current_ai_secret}" -lt 24 ] ||
  [ "$current_ai_secret" = please_change_this_secret ] ||
  [ "$current_ai_secret" = erp_wonchon_default_secret_key_2026_please_change ]; } &&
  { [ -n "$current_ai_secret" ] || [ "${#current_jwt_secret}" -lt 24 ] ||
    [ "$current_jwt_secret" = please_change_this_secret ] ||
    [ "$current_jwt_secret" = erp_wonchon_default_secret_key_2026_please_change ]; }; }; then
  if [ ! -s "$secret_file" ]; then
    umask 077
    LC_ALL=C tr -dc "A-Za-z0-9" < /dev/urandom | head -c 48 > "$secret_file"
  fi
  ai_secret=$(cat "$secret_file")
  if [ "${#ai_secret}" -lt 24 ]; then
    echo "AI 加密密钥文件无效：$secret_file" >&2
    exit 1
  fi
  chmod 600 "$secret_file"
  printf "services:\n  app:\n    environment:\n      AI_CONFIG_SECRET: %s\n" "$ai_secret" > "$override_file"
  chmod 600 "$override_file"
  use_override=1
fi
set -- -f docker-compose.yml
if [ "$use_override" = 1 ]; then set -- "$@" -f "$override_file"; fi
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
if [ "$(docker inspect --format "{{.State.Running}}" gubuy-erp-app)" != true ]; then
  echo "容器未处于运行状态" >&2
  exit 1
fi
if [ "$use_override" = 1 ] && ! docker inspect --format "{{range .Config.Env}}{{println .}}{{end}}" gubuy-erp-app | grep -Fqx "AI_CONFIG_SECRET=$ai_secret"; then
  echo "容器未收到 AI_CONFIG_SECRET" >&2
  exit 1
fi
echo "镜像校验通过：$new_image_id"
if [ "$use_override" = 1 ]; then echo "AI 加密密钥已持久保存并注入容器"; fi
if [ "__PRUNE__" = 1 ]; then docker image prune --force --filter dangling=true; fi
'
remote_update="${remote_update//__PRUNE__/$REMOTE_PRUNE_DOCKER_IMAGES}"

printf '%s\n' '=> [3/4] 正在远端导入镜像、重建 app 容器并核对镜像 ID…'
run_with_password ssh -o "ConnectTimeout=$SSH_CONNECT_TIMEOUT" -o StrictHostKeyChecking=accept-new \
  "$ROUTER_USER@$ROUTER_IP" "$remote_update" || {
    printf '错误：远端容器更新失败，请查看上方 SSH/Docker 错误。\n' >&2
    exit 1
  }
printf '%s\n\n' '容器更新及重启完成！'

printf '%s\n' '=> [4/4] 正在确认 ERP 容器状态…'
run_with_password ssh -o "ConnectTimeout=$SSH_CONNECT_TIMEOUT" -o StrictHostKeyChecking=accept-new \
  "$ROUTER_USER@$ROUTER_IP" "docker ps --filter name=gubuy-erp-app --format '{{.Names}} {{.Image}} {{.Status}}'" || {
    printf '错误：无法读取容器状态。\n' >&2
    exit 1
  }

printf '%s\n' '=================================================='
printf '%s\n' '🎉 所有更新操作执行完毕！请确认上方 ERP 容器状态为 Up 后再开始使用。'
printf '%s\n' '=================================================='
