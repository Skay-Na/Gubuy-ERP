// 入库与盘点相关 API（含入库审批、库存日志、盘点批次）
import request from '../utils/request'

// 创建入库单
export const createInbound = data => request.post('/api/inbounds', data)

// 按库存日志撤销入库
export const cancelInboundByLog = logId => request.post(`/api/inbounds/cancel-by-log/${logId}`)

// 修改入库价格
export const updateInboundPrice = (logId, data) => request.post(`/api/inbounds/update-price/${logId}`, data)

// 查询库存变动日志
export const fetchInventoryLogs = params => request.get('/api/inventory/logs', { params })

// 查询待审批入库申请（status=pending）
export const fetchPendingInbounds = () => request.get('/api/pending-inbounds?status=pending')

// 审批通过入库申请
export const approvePendingInbound = (id, data) => request.post(`/api/pending-inbounds/${id}/approve`, data)

// 驳回入库申请
export const rejectPendingInbound = (id, data) => request.post(`/api/pending-inbounds/${id}/reject`, data)

// 查询当前员工的入库申请
export const fetchMyPendingInbounds = params => request.get('/api/pending-inbounds/mine', { params })

// 查询待审批盘点申请
export const fetchPendingStocktakes = params => request.get('/api/pending-stocktakes', { params })

// 审批通过盘点申请
export const approvePendingStocktake = id => request.post(`/api/pending-stocktakes/${id}/approve`)

// 驳回盘点申请
export const rejectPendingStocktake = (id, data) => request.post(`/api/pending-stocktakes/${id}/reject`, data)

// 查询当前员工的盘点申请
export const fetchMyPendingStocktakes = params => request.get('/api/pending-stocktakes/mine', { params })

// ---------- 盘点批次 ----------

// 查询盘点批次列表
export const fetchStocktakeBatches = () => request.get('/api/stocktake-batches')

// 创建盘点批次
export const createStocktakeBatch = data => request.post('/api/stocktake-batches', data)

// 查询盘点批次详情
export const fetchStocktakeBatch = batchId => request.get(`/api/stocktake-batches/${batchId}`)

// 保存盘点批次明细
export const updateStocktakeBatchItems = (batchId, data) => request.put(`/api/stocktake-batches/${batchId}/items`, data)

// 提交盘点批次
export const submitStocktakeBatch = batchId => request.post(`/api/stocktake-batches/${batchId}/submit`)

// 刷新盘点批次单项库存快照
export const refreshStocktakeBatchItem = (batchId, itemId) =>
  request.post(`/api/stocktake-batches/${batchId}/items/${itemId}/refresh`)

// 盘点批次打印（需二次确认 confirmed: true）
export const printStocktakeBatch = (batchId, data) => request.post(`/api/stocktake-batches/${batchId}/print`, data)

// ---------- 管理端盘点批次 ----------

// 查询全部盘点批次（管理端）
export const fetchAdminStocktakeBatches = () => request.get('/api/admin/stocktake-batches')

// 终止盘点批次
export const terminateStocktakeBatch = batchId => request.post(`/api/admin/stocktake-batches/${batchId}/terminate`)

// 删除盘点批次
export const deleteStocktakeBatch = batchId => request.delete(`/api/admin/stocktake-batches/${batchId}`)
