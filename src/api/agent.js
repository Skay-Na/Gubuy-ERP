// AI 助手与 AI 导入相关 API（不含 pi-agent 运行时接口，那部分仍由 useAIAgent 直接调用）
import request from '../utils/request'

// ---------- AI 记忆 ----------

// 查询 AI 记忆列表
export const fetchAgentMemories = () => request.get('/api/ai/agent/memories')

// 新建 AI 记忆
export const createAgentMemory = data => request.post('/api/ai/agent/memories', data)

// 更新 AI 记忆
export const updateAgentMemory = (memoryId, data) => request.put(`/api/ai/agent/memories/${memoryId}`, data)

// 删除 AI 记忆
export const deleteAgentMemory = memoryId => request.delete(`/api/ai/agent/memories/${memoryId}`)

// ---------- AI 单据导入 ----------

// 查询 AI 导入单列表（kind: order/saleorder 等）
export const fetchAIImports = params => request.get('/api/ai/imports', { params })

// 查询 AI 导入单详情
export const fetchAIImport = importId => request.get(`/api/ai/imports/${importId}`)

// 查询 AI 导入单原始图片（blob）
export const fetchAIImportImage = importId => request.get(`/api/ai/imports/${importId}/image`, { responseType: 'blob' })

// 提交 AI 导入（识别耗时较长，默认放宽超时）
export const createAIImport = (data, config = {}) => request.post('/api/ai/imports', data, { timeout: 120000, ...config })

// 删除 AI 导入单
export const deleteAIImport = importId => request.delete(`/api/ai/imports/${importId}`)
