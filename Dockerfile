# --- Stage 1: Build Frontend ---
FROM node:20-alpine AS frontend-builder
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
RUN npm run build

# --- Stage 2: Build Backend ---
FROM golang:1.26-alpine AS backend-builder
WORKDIR /app
# 复制 go.mod 和 go.sum 并下载依赖
COPY erp-backend/go.mod erp-backend/go.sum ./
RUN go mod download
# 复制后端源代码
COPY erp-backend/ .
# 编译可执行文件
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# --- Stage 3: Final Runtime ---
FROM alpine:latest
WORKDIR /root/

# 安装必要的库和 mysql 客户端 (用于备份)
RUN apk --no-cache add ca-certificates mariadb-client

# 从前端构建阶段复制编译后的文件
COPY --from=frontend-builder /app/dist ./dist

# 从后端构建阶段复制可执行文件
COPY --from=backend-builder /app/main .

# 暴露端口 (Gin 默认 8080)
EXPOSE 8080

# 运行应用
CMD ["./main"]
