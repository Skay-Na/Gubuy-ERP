#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
test_dir="$(mktemp -d)"
trap 'rm -r "$test_dir"' EXIT
mkdir -p "$test_dir/bin" "$test_dir/backup"
printf '[]\n' > "$test_dir/manifest.json"
tar -cf "$test_dir/image.tar" -C "$test_dir" manifest.json

cat > "$test_dir/bin/ifconfig" <<'MOCK'
#!/bin/sh
printf 'inet 192.168.5.100 netmask 0xffffff00\n'
MOCK
cat > "$test_dir/bin/nc" <<'MOCK'
#!/bin/sh
exit 0
MOCK
cat > "$test_dir/bin/scp" <<'MOCK'
#!/bin/sh
if [ "${SCP_FAIL:-0}" = 1 ]; then exit 7; fi
exit 0
MOCK
cat > "$test_dir/bin/ssh" <<'MOCK'
#!/bin/sh
printf '%s\n' "$*" >> "$SSH_LOG"
if [ "${SSH_FAIL:-0}" = 1 ]; then exit 9; fi
exit 0
MOCK
chmod +x "$test_dir/bin/ifconfig" "$test_dir/bin/nc" "$test_dir/bin/scp" "$test_dir/bin/ssh"

export PATH="$test_dir/bin:$PATH"
export ROUTER_PASS=test-password
export LOCAL_BACKUP_DIR="$test_dir/backup"
export LOCAL_TAR_FILE="$test_dir/image.tar"
export SSH_LOG="$test_dir/ssh.log"

bash "$SCRIPT_DIR/update_router.sh" > "$test_dir/success.log" 2>&1
rg -q '所有更新操作执行完毕' "$test_dir/success.log"
rg -q -- '--no-build --no-deps --force-recreate app' "$SSH_LOG"
rg -q 'docker image tag erpwonchon-app:latest' "$SSH_LOG"

if SCP_FAIL=1 bash "$SCRIPT_DIR/update_router.sh" > "$test_dir/scp-fail.log" 2>&1; then
  printf 'FAIL: SCP failure was reported as success\n' >&2
  exit 1
fi
rg -q '备份失败' "$test_dir/scp-fail.log"

if SSH_FAIL=1 bash "$SCRIPT_DIR/update_router.sh" > "$test_dir/ssh-fail.log" 2>&1; then
  printf 'FAIL: SSH failure was reported as success\n' >&2
  exit 1
fi
rg -q '远端容器更新失败' "$test_dir/ssh-fail.log"

printf 'PASS: successful path and SSH/SCP failures are handled correctly\n'
