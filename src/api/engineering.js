// 工程模块（engineering）接口封装
// 契约基线：docs/tech/2026-10-29-engineering-module-api-contract.md（API-ENG-001 §4）
// 共 16 个函数，与 16 个后端接口一一对应；函数名不得随意改动（前后端并行开发的对接点）。
//
// 注意：工程费用走 /api/admin/engineering/expenses，
// 与日常经营开销的 /api/expenses 是两套账，不可混用。
import request from '../utils/request'

const BASE = '/api/admin/engineering'

// ---------- 项目 ----------

// 汇总卡（6 张卡的数字口径全部由后端算，前端不重算）
export const fetchEngineeringSummary = params => request.get(`${BASE}/summary`, { params })

// 项目列表
export const fetchEngineeringProjects = params => request.get(`${BASE}/projects`, { params })

// 建档
export const createEngineeringProject = data => request.post(`${BASE}/projects`, data)

// 项目详情（一次返回 items / terms / receipts / shipments / expenses 与派生字段）
export const getEngineeringProject = id => request.get(`${BASE}/projects/${id}`)

// 更新主档（不含清单/节点/回款/出库/费用）
export const updateEngineeringProject = (id, data) => request.put(`${BASE}/projects/${id}`, data)

// 删除（仅限跟进中且无任何业务数据的项目）
export const deleteEngineeringProject = id => request.delete(`${BASE}/projects/${id}`)

// 状态流转（action: sign / accept / revert / void）
export const updateEngineeringStatus = (id, data) => request.put(`${BASE}/projects/${id}/status`, data)

// 清单整表保存（整表提交，不逐格保存）
export const saveEngineeringItems = (id, data) => request.put(`${BASE}/projects/${id}/items`, data)

// 付款节点整表保存（合计必须等于合同总额）
export const saveEngineeringTerms = (id, data) => request.put(`${BASE}/projects/${id}/payment-terms`, data)

// 登记回款
export const createEngineeringReceipt = (id, data) => request.post(`${BASE}/projects/${id}/receipts`, data)

// 冲销回款（生成负数记录，不删原记录）
export const reverseEngineeringReceipt = (id, rid, data) =>
  request.post(`${BASE}/projects/${id}/receipts/${rid}/reverse`, data)

// 工程出库（扣库存 + 写 inventory_logs(log_type=engineering)）
export const createEngineeringShipment = (id, data) => request.post(`${BASE}/projects/${id}/shipments`, data)

// ---------- 工程费用（独立成账） ----------

// 费用台账（跨项目，含 project_id 为空的公共费用）
export const fetchEngineeringExpenses = params => request.get(`${BASE}/expenses`, { params })

// 新增费用（project_id 传 null 即公共费用）
export const createEngineeringExpense = data => request.post(`${BASE}/expenses`, data)

// 编辑费用
export const updateEngineeringExpense = (expenseId, data) => request.put(`${BASE}/expenses/${expenseId}`, data)

// 删除费用（软删除，操作历史留 before 快照）
export const deleteEngineeringExpense = expenseId => request.delete(`${BASE}/expenses/${expenseId}`)
