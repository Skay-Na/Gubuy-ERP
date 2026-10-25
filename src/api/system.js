// 系统管理相关 API（AI 供应商配置/备份恢复/重置/历史成本修复/操作历史）
import request from '../utils/request'

// ---------- AI 供应商配置 ----------

// 查询 AI 供应商列表
export const fetchAIProviders = () => request.get('/api/system/ai/providers')

// 保存 AI 供应商配置
export const saveAIProvider = (providerName, data) => request.put(`/api/system/ai/providers/${providerName}`, data)

// 删除 AI 供应商配置
export const deleteAIProvider = providerName => request.delete(`/api/system/ai/providers/${providerName}`)

// 激活 AI 供应商
export const activateAIProvider = provider => request.put('/api/system/ai/active', { provider })

// ---------- 备份与恢复 ----------

// 查询备份记录
export const fetchBackupLogs = params => request.get('/api/system/backup', { params })

// 恢复备份（formData 上传，需放宽超时）
export const restoreBackup = (formData, config = {}) => request.post('/api/system/restore', formData, config)

// 系统数据重置
export const resetSystem = data => request.post('/api/system/reset', data)

// ---------- 历史成本修复 ----------

// 查询历史成本修复候选清单
export const fetchHistoricalCostCandidates = params =>
  request.get('/api/system/historical-cost-repair/candidates', { params })

// 执行历史成本修复
export const repairHistoricalCost = data => request.post('/api/system/historical-cost-repair', data)

// ---------- 操作历史 ----------

// 查询操作历史
export const fetchOperations = params => request.get('/api/operations', { params })

// 撤销操作
export const undoOperation = operationId => request.post(`/api/operations/${operationId}/undo`)
