// 项目详情页状态（T12）
//
// 设计取舍：所有写操作（签约/验收/回退/作废/清单/节点/回款/出库/费用）都走这里，
// 组件只负责发事件，不各自调用 API —— 这样「一次写操作后要刷新哪些数据」只有一处逻辑，
// 避免出现"回款登记了但汇总卡没变"这类前后不一致。
import { computed, ref } from 'vue'
import {
  createEngineeringReceipt,
  createEngineeringShipment,
  getEngineeringProject,
  reverseEngineeringReceipt,
  saveEngineeringItems,
  saveEngineeringTerms,
  updateEngineeringProject,
  updateEngineeringStatus
} from '../api/engineering'

export function useEngineeringProject() {
  const project = ref(null)
  const loading = ref(false)
  const errorMessage = ref('')
  const actionLoading = ref(false)

  const load = async id => {
    loading.value = true
    errorMessage.value = ''
    try {
      const res = await getEngineeringProject(id)
      // 拦截器返回原始 response：业务数据在 res.result
      project.value = res?.result || null
      return project.value
    } catch (e) {
      project.value = null
      errorMessage.value = e?.response?.data?.msg || '项目加载失败'
      return null
    } finally {
      loading.value = false
    }
  }

  const items = computed(() => project.value?.items || [])
  const terms = computed(() => project.value?.terms || [])
  const receipts = computed(() => project.value?.receipts || [])
  const shipments = computed(() => project.value?.shipments || [])
  const expenses = computed(() => project.value?.expenses || [])

  const status = computed(() => Number(project.value?.status) || 0)
  const isVoided = computed(() => status.value === 9)
  const isSettled = computed(() => status.value === 5)
  // 金额锁定：验收后不可再改清单/节点
  const amountLocked = computed(() => [4, 5, 9].includes(status.value))

  const paidTermCount = computed(() => terms.value.filter(t => Number(t.pending_amount) <= 0).length)
  const allShipped = computed(() => {
    if (!project.value) return false
    const ordered = Number(project.value.ordered_qty) || 0
    return ordered > 0 && Number(project.value.shipped_qty) >= ordered
  })

  // 节点合计与合同额是否平：签约按钮的可用性依据
  const termsBalanced = computed(() => {
    if (!project.value) return false
    const total = terms.value.reduce((sum, t) => sum + (Number(t.planned_amount) || 0), 0)
    return Math.abs(total - Number(project.value.contract_total)) < 0.01
  })

  const runAction = async (fn, after) => {
    actionLoading.value = true
    try {
      const res = await fn()
      if (after) await after(res)
      return res
    } finally {
      actionLoading.value = false
    }
  }

  // ---------- 状态流转 ----------

  const sign = (payload = {}) =>
    runAction(
      () => updateEngineeringStatus(project.value.id, { action: 'sign', ...payload }),
      () => load(project.value.id)
    )

  const accept = (payload = {}) =>
    runAction(
      () => updateEngineeringStatus(project.value.id, { action: 'accept', ...payload }),
      () => load(project.value.id)
    )

  const revert = reason =>
    runAction(
      () => updateEngineeringStatus(project.value.id, { action: 'revert', reason }),
      () => load(project.value.id)
    )

  const voidProject = reason =>
    runAction(
      () => updateEngineeringStatus(project.value.id, { action: 'void', reason }),
      () => load(project.value.id)
    )

  // ---------- 主档 ----------

  const updateMain = payload =>
    runAction(
      () => updateEngineeringProject(project.value.id, payload),
      () => load(project.value.id)
    )

  // ---------- 清单 / 节点 ----------

  const saveItems = payload =>
    runAction(
      () => saveEngineeringItems(project.value.id, payload),
      () => load(project.value.id)
    )

  const saveTerms = payload =>
    runAction(
      () => saveEngineeringTerms(project.value.id, payload),
      () => load(project.value.id)
    )

  // ---------- 回款 ----------

  const addReceipt = payload =>
    runAction(
      () => createEngineeringReceipt(project.value.id, payload),
      () => load(project.value.id)
    )

  const reverseReceipt = (receiptId, payload) =>
    runAction(
      () => reverseEngineeringReceipt(project.value.id, receiptId, payload),
      () => load(project.value.id)
    )

  // ---------- 出库 ----------

  const addShipment = payload =>
    runAction(
      () => createEngineeringShipment(project.value.id, payload),
      () => load(project.value.id)
    )

  // 费用增删改在费用面板内直接调 /expenses 接口，成功后只需重拉详情：
  const refresh = () => load(project.value.id)

  return {
    project,
    loading,
    errorMessage,
    actionLoading,
    items,
    terms,
    receipts,
    shipments,
    expenses,
    status,
    isVoided,
    isSettled,
    amountLocked,
    paidTermCount,
    allShipped,
    termsBalanced,
    load,
    refresh,
    sign,
    accept,
    revert,
    voidProject,
    updateMain,
    saveItems,
    saveTerms,
    addReceipt,
    reverseReceipt,
    addShipment
  }
}
