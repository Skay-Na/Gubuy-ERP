import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const apiProxyTarget = process.env.VITE_API_PROXY_TARGET || 'http://127.0.0.1:8081'
const agentProxyTarget = process.env.VITE_AGENT_PROXY_TARGET || 'http://127.0.0.1:8082'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
  server: {
    host: true, // 允许局域网访问
    port: 5173,
    proxy: {
      // pi 核心 Agent 服务（正则排除 /api/ai/agent-tools，其仍归 Go 后端）
      '^/api/ai/agent(?!-tools)': {
        target: agentProxyTarget,
        changeOrigin: true
      },
      '/api': {
        target: apiProxyTarget,
        changeOrigin: true
      }
    }
  }
})
