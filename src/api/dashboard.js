// 仪表盘相关 API（复用资金汇总与离职提醒，集中导出避免视图直接散调）
export { fetchFinanceSummary } from './finance'
export { fetchOffboardingAlerts, resolveOffboardingAlert } from './employees'
