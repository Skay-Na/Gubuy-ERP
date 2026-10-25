#!/bin/sh
# Gubuy-ERP 单容器双进程入口：
#   1) Go API        :8080（主进程，退出即容器退出）
#   2) agent-service :8082（pi 核心 Agent 服务，崩溃后自动重启；
#      重启循环 shell 自身退出同样终止容器，避免半活状态）
set -u

cd "$(dirname "$0")"

shutdown() {
  [ -n "${GO_PID:-}" ] && kill "$GO_PID" 2>/dev/null
  [ -n "${AGENT_LOOP_PID:-}" ] && kill "$AGENT_LOOP_PID" 2>/dev/null
  exit 0
}
trap shutdown INT TERM

# 1. Go API
./main &
GO_PID=$!

# 2. agent-service（循环重启：进程崩溃 3 秒后拉起）
(
  while :; do
    node agent-service/dist/server.js
    echo "[entrypoint] agent-service exited (code=$?), restarting in 3s..." >&2
    sleep 3
  done
) &
AGENT_LOOP_PID=$!

# 轮询双进程存活：任一退出（且无法由重启循环恢复的）都终结容器
while kill -0 "$GO_PID" 2>/dev/null && kill -0 "$AGENT_LOOP_PID" 2>/dev/null; do
  sleep 2
done

echo "[entrypoint] critical process exited (go=$GO_PID agent_loop=$AGENT_LOOP_PID), shutting down container" >&2
shutdown
