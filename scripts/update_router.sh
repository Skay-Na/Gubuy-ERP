#!/bin/bash

# ==========================================
# 软路由 ERP 系统一键更新脚本
# ==========================================

ROUTER_IP="192.168.5.22"
ROUTER_USER="root"
ROUTER_PASS="888888"
REMOTE_DIR="/mnt/opt/docker/install"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LOCAL_TAR_FILE="$REPO_ROOT/erpwonchon-app.tar"
LOCAL_BACKUP_DIR="/Users/nanjiakai/Desktop"

echo "=================================================="
echo "开始执行一键更新部署流程"
echo "=================================================="

echo "=> [1/3] 正在备份软路由中的旧镜像到桌面..."
expect -c "
set timeout -1
spawn scp -o StrictHostKeyChecking=no $ROUTER_USER@$ROUTER_IP:$REMOTE_DIR/*.tar $LOCAL_BACKUP_DIR/
expect {
    \"password:\" {
        send \"$ROUTER_PASS\r\"
        exp_continue
    }
    eof
}
"
echo "备份完成！"
echo ""

echo "=> [2/3] 正在上传最新镜像到软路由..."
expect -c "
set timeout -1
spawn scp -o StrictHostKeyChecking=no $LOCAL_TAR_FILE $ROUTER_USER@$ROUTER_IP:$REMOTE_DIR/
expect {
    \"password:\" {
        send \"$ROUTER_PASS\r\"
        exp_continue
    }
    eof
}
"
echo "上传完成！"
echo ""

echo "=> [3/3] 正在远程载入镜像并重启 Docker 容器..."
expect -c "
set timeout -1
spawn ssh -o StrictHostKeyChecking=no $ROUTER_USER@$ROUTER_IP \"cd $REMOTE_DIR && docker load -i erpwonchon-app.tar && if command -v docker-compose >/dev/null 2>&1; then docker-compose up -d; else docker compose up -d; fi\"
expect {
    \"password:\" {
        send \"$ROUTER_PASS\r\"
        exp_continue
    }
    eof
}
"
echo "容器更新及重启完成！"
echo ""

echo "=================================================="
echo "🎉 所有更新操作执行完毕！您可以开始测试服务了。"
echo "=================================================="
