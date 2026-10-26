# --- Stage 1: Build Frontend ---
# 构建阶段固定跑原生架构（BUILDPLATFORM），前端产物是静态文件与架构无关，
# 避免 node 在 QEMU 模拟下慢数倍
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend-builder
WORKDIR /app
COPY package*.json ./
# 使用 cache mount 缓存 npm 依赖
RUN --mount=type=cache,target=/root/.npm \
    npm install
COPY . .
RUN npm run build

# --- Stage 2: Build Backend ---
# 构建阶段跑原生架构，GOOS/GOARCH 交叉编译出目标架构二进制（软路由 = linux/arm64）
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend-builder
# 国内环境用 goproxy.cn 拉模块（proxy.golang.org 直连不稳定）
ENV GOPROXY=https://goproxy.cn,direct
# buildx/build --platform 时由构建器自动注入，用于交叉编译到目标架构
# （软路由是 linux/arm64，必须设 GOARCH，否则编出 amd64 二进制无法运行）
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
# 复制 go.mod 和 go.sum
COPY erp-backend/go.mod erp-backend/go.sum ./
# 使用 cache mount 缓存 Go 模块
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
# 复制后端源代码
COPY erp-backend/ .
# Swagger 源码由注释生成，docs/ 在 Git 中忽略。
RUN --mount=type=cache,target=/go/pkg/mod \
    go generate .
# 使用 cache mount 缓存编译产物，提升二次构建速度
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o main .

# --- Stage 3: Build agent-service（pi 核心 Agent 服务，TS → dist） ---
# 构建阶段同样跑原生架构，产物是纯 JS，与目标架构无关
FROM --platform=$BUILDPLATFORM node:20-alpine AS agent-builder
WORKDIR /agent-service
COPY agent-service/package*.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci
COPY agent-service/ ./
RUN npm run build

# --- Stage 4: agent-service 生产运行时依赖（node_modules 仅含 dependencies） ---
FROM --platform=$BUILDPLATFORM node:20-alpine AS agent-deps
WORKDIR /agent-service
COPY agent-service/package*.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --omit=dev

# --- Stage 5: Final Runtime ---
# 运行镜像需要 node 运行时承载 agent-service，因此基镜像从 alpine 换为 node:20-alpine
FROM node:20-alpine

# 安装必要库、mysql 客户端及时间数据，并创建非特权用户
# apk 源换国内镜像（dl-cdn.alpinelinux.org 直连偶发 I/O 错误）
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk --no-cache add ca-certificates mariadb-client tzdata && \
    (command -v mariadb-dump >/dev/null 2>&1 || command -v mysqldump >/dev/null 2>&1) && \
    (command -v mariadb >/dev/null 2>&1 || command -v mysql >/dev/null 2>&1) && \
    adduser -D -g '' appuser

WORKDIR /home/appuser
ENV TZ=Asia/Shanghai

# 从前端构建阶段复制编译后的文件
COPY --from=frontend-builder --chown=appuser:appuser /app/dist ./dist

# 从后端构建阶段复制可执行文件
COPY --from=backend-builder --chown=appuser:appuser /app/main .

# agent-service 编译产物 + 生产依赖（运行 node agent-service/dist/server.js，:8082）
COPY --from=agent-builder --chown=appuser:appuser /agent-service/dist ./agent-service/dist
COPY --from=agent-deps --chown=appuser:appuser /agent-service/node_modules ./agent-service/node_modules

# 双进程入口脚本（Go + agent-service，任一关键进程退出容器即退出）
COPY --chown=appuser:appuser docker-entrypoint.sh ./docker-entrypoint.sh
RUN chmod +x docker-entrypoint.sh

# 切换为非特权用户
USER appuser

# 暴露端口 (Go API 8080 / agent-service 8082)
EXPOSE 8080 8082

# 启动双进程
CMD ["./docker-entrypoint.sh"]
