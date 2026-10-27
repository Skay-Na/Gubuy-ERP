// 资金相关 API（账户/划拨/校准/流水汇总/记账）
import request from '../utils/request'

// 查询资金账户列表
export const fetchAccounts = () => request.get('/api/accounts')

// 新建资金账户
export const createAccount = data => request.post('/api/accounts', data)

// 账户间资金划拨
export const transferAccounts = data => request.post('/api/accounts/transfer', data)

// 校准账户余额（保留审计记录）
export const calibrateAccount = (accountId, data) => request.put(`/api/accounts/${accountId}/calibrate`, data)

// 资金汇总（按 period 过滤）
export const fetchFinanceSummary = params => request.get('/api/finance/summary', { params })

// 资金流水列表
export const fetchFinanceLogs = params => request.get('/api/finance/logs', { params })

// 资金工作台数据（按月）
export const fetchFinanceWorkspace = params => request.get('/api/finance/workspace', { params })

// 销售分析聚合（员工/品类/商品/支付方式，按月或累计）
export const fetchFinanceSalesAnalysis = params => request.get('/api/finance/sales-analysis', { params })

// 新增记账凭证
export const createBookkeepingEntry = data => request.post('/api/bookkeeping/entries', data)

// 记账订单搜索（q 关键字）
export const searchBookkeepingOrders = params => request.get('/api/bookkeeping/orders', { params })
