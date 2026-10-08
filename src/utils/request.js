import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'

// 支持读取环境变量或回退到默认后端地址
const baseURL = import.meta.env.VITE_API_BASE_URL || ''

const service = axios.create({
  baseURL,
  timeout: 10000
})

const isTerminalRoute = () => router.currentRoute.value.path === '/terminal'
const isPinVerifyRequest = config => config?.url?.includes('/api/employees/verify-pin')

const clearAuth = () => {
  localStorage.removeItem('erp_token')
  localStorage.removeItem('erp_username')
}

const clearPosAuth = () => {
  localStorage.removeItem('pos_token')
  localStorage.removeItem('pos_current_employee')
  window.dispatchEvent(new CustomEvent('pos-auth-expired'))
}

// 请求拦截器
service.interceptors.request.use(
  config => {
    const token = isTerminalRoute()
      ? localStorage.getItem('pos_token')
      : localStorage.getItem('erp_token')
    if (token) {
      // 注入 JWT Token
      config.headers['Authorization'] = `Bearer ${token}`
    }
    return config
  },
  error => {
    console.error('Request error:', error?.message || '请求配置失败')
    return Promise.reject(error)
  }
)

// 响应拦截器
service.interceptors.response.use(
  response => {
    const payload = response.data
    if (payload && payload.code === 401) {
      if (isPinVerifyRequest(response.config)) {
        return Promise.reject(new Error(payload.msg || 'Unauthorized'))
      }
      if (isTerminalRoute()) {
        ElMessage.error(payload.msg || '员工登录已过期，请重新输入 PIN')
        clearPosAuth()
        router.replace('/terminal')
        return Promise.reject(new Error(payload.msg || 'Unauthorized'))
      }
      ElMessage.error(payload.msg || '未授权，请重新登录')
      clearAuth()
      router.push('/')
      return Promise.reject(new Error(payload.msg || 'Unauthorized'))
    }

    if (payload && typeof payload === 'object' && !Array.isArray(payload)) {
      response.code = payload.code
      response.msg = payload.msg
      response.result = payload.data
    }

    return response
  },
  error => {
    console.error('Response error:', error?.response?.status || error?.message || '请求失败')
    if (error.response && error.response.status === 401) {
      if (isPinVerifyRequest(error.config)) {
        return Promise.reject(error)
      }
      if (isTerminalRoute()) {
        ElMessage.error(error.response?.data?.msg || '员工登录已过期，请重新输入 PIN')
        clearPosAuth()
        router.replace('/terminal')
        return Promise.reject(error)
      }
      ElMessage.error('登录失效或无权限，请重新登录')
      clearAuth()
      router.push('/')
    } else {
      ElMessage.error(error.response?.data?.msg || error.message || '网络请求错误')
    }
    return Promise.reject(error)
  }
)

export default service
