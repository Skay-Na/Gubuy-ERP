// 认证与初始化相关 API（Portal 登录、管理员密码）
import request from '../utils/request'

// 获取管理员初始化状态（是否已存在管理员账号）
export const fetchAdminStatus = () => request.get('/api/admin/status')

// 校验管理员账号密码，签发 Token
export const verifyAdmin = data => request.post('/api/admin/verify', data)

// 首次初始化管理员账号
export const initAdmin = data => request.post('/api/admin/init', data)

// 修改管理员密码
export const updateAdminPassword = data => request.put('/api/admin/password', data)
