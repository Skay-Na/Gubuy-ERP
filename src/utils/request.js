import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'

// 支持读取环境变量或回退到默认后端地址
const baseURL = import.meta.env.VITE_API_BASE_URL || ''

const service = axios.create({
  baseURL,
  timeout: 10000
})

const isTerminalRoute = () => router.currentRoute.value.path.startsWith('/terminal')
const isPinVerifyRequest = config => config?.url?.includes('/api/employees/verify-pin')

// 清除管理端登录态（退出登录、Token 失效时共用）
export const clearAuth = () => {
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

// 统一 401 处理入口：业务 code===401 与 HTTP 401 双轨收敛到这里
// 返回 true 表示已按当前路由完成提示与登出跳转，调用方需 reject；
// 返回 false 表示 PIN 校验请求，由调用方按原逻辑自行 reject（不打扰、不跳转）
const handleUnauthorized = (config, terminalMessage, adminMessage, isBlobRequest = false) => {
  if (isPinVerifyRequest(config)) {
    return false
  }
  if (isTerminalRoute()) {
    if (!isBlobRequest) ElMessage.error(terminalMessage)
    clearPosAuth()
    router.replace('/terminal')
    return true
  }
  if (!isBlobRequest) ElMessage.error(adminMessage)
  clearAuth()
  router.push('/')
  return true
}

// 响应拦截器
service.interceptors.response.use(
  response => {
    const payload = response.data
    if (payload && payload.code === 401) {
      handleUnauthorized(
        response.config,
        payload.msg || '员工登录已过期，请重新输入 PIN',
        payload.msg || '未授权，请重新登录'
      )
      return Promise.reject(new Error(payload.msg || 'Unauthorized'))
    }

    if (payload && typeof payload === 'object' && !Array.isArray(payload)) {
      response.code = payload.code
      response.msg = payload.msg
      response.result = payload.data
      // 幂等命中：后端返回 duplicated:true 表示请求已提交过，统一给出友好提示
      if (payload.duplicated) {
        ElMessage.info(payload.msg || '该请求已提交过，未重复处理')
      }
    }

    return response
  },
  error => {
    console.error('Response error:', error?.response?.status || error?.message || '请求失败')
    const isBlobRequest = error.config?.responseType === 'blob'
    if (error.response && error.response.status === 401) {
      handleUnauthorized(
        error.config,
        error.response?.data?.msg || '员工登录已过期，请重新输入 PIN',
        '登录失效或无权限，请重新登录',
        isBlobRequest
      )
      return Promise.reject(error)
    }
    if (!isBlobRequest) ElMessage.error(error.response?.data?.msg || error.message || '网络请求错误')
    return Promise.reject(error)
  }
)

export default service
