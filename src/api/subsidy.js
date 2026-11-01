// 国补相关 API
import request from '../utils/request'

// 国补汇总
export const fetchSubsidySummary = () => request.get('/api/subsidy/summary')

// 国补结算单列表
export const fetchSubsidySettlements = () => request.get('/api/subsidy/settlements')

// 国补回执列表
export const fetchSubsidyReceipts = () => request.get('/api/subsidy/receipts')

// 国补资金划拨（打款）
export const allocateSubsidy = data => request.post('/api/subsidy/allocate', data)

// 国补结算
export const settleSubsidy = data => request.post('/api/subsidy/settle', data)

// 国补批量到账：勾选多笔订单一次性入账并核销
export const batchReceiveSubsidy = data => request.post('/api/subsidy/batch-receive', data)
