// 看板明细钻取 API（只读查询）
import request from '../utils/request'

// 利润计算过程汇总（销售额/货品成本/五项费用/毛利/经营开销/净利润）
export const fetchProfitBreakdown = params => request.get('/api/finance/profit-breakdown', { params })

// 按类型钻取明细列表（分页，响应带后端全量合计 total_amount）
export const fetchBreakdownDetail = (type, params) => request.get(`/api/finance/breakdown/${type}`, { params })
