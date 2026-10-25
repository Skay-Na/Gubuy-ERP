// 员工相关 API（档案/考勤/PIN/提成/工资确认/离职）
import request from '../utils/request'

// 查询员工列表（include_departed=true 含离职员工）
export const fetchEmployees = params => request.get('/api/employees', { params })

// 新建员工
export const createEmployee = data => request.post('/api/employees', data)

// 更新员工
export const updateEmployee = (employeeId, data) => request.put(`/api/employees/${employeeId}`, data)

// 删除员工
export const deleteEmployee = employeeId => request.delete(`/api/employees/${employeeId}`)

// 查询精简员工列表（收银/终端下拉用）
export const fetchSimpleEmployees = () => request.get('/api/employees/simple')

// 查询考勤记录（month 过滤）
export const fetchAttendanceLogs = params => request.get('/api/employees/attendance-logs', { params })

// 管理端补录考勤
export const recordAttendance = data => request.post('/api/employees/attendance', data)

// 员工签到
export const checkIn = data => request.post('/api/employees/check-in', data)

// 员工签退
export const checkOut = data => request.post('/api/employees/check-out', data)

// 员工 PIN 校验（POS 登录）
export const verifyPin = data => request.post('/api/employees/verify-pin', data)

// 员工提成报表（按月）
export const fetchCommissionReport = month => request.get(`/api/employees/commission-report?month=${month}`)

// ---------- 工资确认 ----------

// 查询工资确认状态
export const fetchPayrollConfirmations = params => request.get('/api/payroll/confirmations', { params })

// 确认工资单
export const confirmPayroll = data => request.post('/api/payroll/confirm', data)

// ---------- 离职与提醒 ----------

// 查询员工离职单列表
export const fetchOffboardings = employeeId => request.get(`/api/employees/${employeeId}/offboardings`)

// 创建离职单
export const createOffboarding = (employeeId, data) => request.post(`/api/employees/${employeeId}/offboardings`, data)

// 更新离职任务（done/notes）
export const updateOffboardingTask = (caseId, taskId, data) =>
  request.put(`/api/employee-offboardings/${caseId}/tasks/${taskId}`, data)

// 离职结算
export const settleOffboarding = caseId => request.post(`/api/employee-offboardings/${caseId}/settlement`)

// 查询离职提醒列表（仪表盘）
export const fetchOffboardingAlerts = () => request.get('/api/employee-offboarding-alerts')

// 处理离职提醒
export const resolveOffboardingAlert = (alertId, data) =>
  request.post(`/api/employee-offboarding-alerts/${alertId}/resolve`, data)
