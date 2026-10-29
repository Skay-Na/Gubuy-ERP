<template>
  <!--
    工程项目详情页（SPEC-ENG-001 §5，任务卡 T12）
    容器职责：页头 + Tab 切换 + 弹层编排；各 Tab 内容由 T13/T14 组件承担。
  -->
  <div class="min-h-screen bg-slate-50/60">
    <div class="mx-auto w-full max-w-[1440px] px-4 py-5 md:px-8 md:py-7">
      <!-- 返回 -->
      <button type="button" class="eng-back" @click="goBack">← 工程列表</button>

      <!-- 加载骨架 -->
      <div v-if="loading && !project" class="mt-4 space-y-3">
        <div class="eng-skeleton eng-skeleton--head" />
        <div class="eng-skeleton eng-skeleton--body" />
      </div>

      <!-- 加载失败 -->
      <div v-else-if="!project" class="mt-4 rounded-2xl border border-slate-100 bg-white p-10 text-center shadow-sm">
        <p class="text-sm font-semibold text-slate-700">{{ errorMessage || '项目不存在或已被删除' }}</p>
        <el-button class="mt-4" size="small" @click="goBack">返回工程列表</el-button>
      </div>

      <template v-else>
        <!-- ===== 页头 ===== -->
        <header class="eng-head">
          <div class="eng-head-main">
            <div class="eng-head-title-row">
              <h1 class="eng-h1">{{ project.name }}</h1>
              <span class="eng-no-chip">{{ project.project_no }}</span>
              <EngineeringStatusTag :status="project.status" />
            </div>
            <p class="eng-head-meta">
              {{ project.client_name }}
              <span class="eng-sep">·</span>{{ project.client_contact }}
              <span class="eng-sep">·</span>{{ project.client_phone }}
              <span class="eng-sep">·</span>{{ project.project_type_label }}
              <span class="eng-sep">·</span>{{ project.owner_name || '未设负责人' }}
            </p>
          </div>

          <div class="eng-head-right">
            <div class="eng-money-triple">
              <div class="eng-money-block">
                <p class="eng-money-label">合同总额</p>
                <p class="eng-money-num">{{ formatCardMoney(project.contract_total) }}</p>
              </div>
              <div class="eng-money-block">
                <p class="eng-money-label">已回款</p>
                <p class="eng-money-num" style="color: #15803d">{{ formatCardMoney(project.received_amount) }}</p>
              </div>
              <div class="eng-money-block">
                <p class="eng-money-label">待回款</p>
                <p
                  class="eng-money-num"
                  :style="
                    Number(project.pending_amount) > 0 && Number(project.overdue_amount) > 0 ? { color: '#dc2626' } : null
                  "
                >
                  {{ formatCardMoney(project.pending_amount) }}
                </p>
              </div>
            </div>

            <div class="eng-head-actions">
              <el-tooltip v-if="status === 1" :disabled="canSign" :content="signBlockReason" placement="bottom">
                <span>
                  <el-button
                    type="primary"
                    size="small"
                    :disabled="!canSign"
                    :loading="actionLoading"
                    @click="confirmSign"
                  >
                    确认签约
                  </el-button>
                </span>
              </el-tooltip>
              <el-button
                v-if="[1, 2, 3, 4].includes(status)"
                size="small"
                :type="status === 1 ? 'default' : 'primary'"
                @click="openReceipt(null)"
              >
                登记回款
              </el-button>
              <el-button v-if="[2, 3].includes(status)" size="small" @click="openShipment">新增出库</el-button>
              <el-button v-if="status === 3 && allShipped" size="small" type="primary" @click="acceptVisible = true">
                确认验收
              </el-button>

              <el-dropdown v-if="[1, 2, 3, 4].includes(status)" trigger="click" @command="onMoreCommand">
                <el-button size="small">更多</el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item command="edit" :disabled="status !== 1">编辑主档</el-dropdown-item>
                    <el-dropdown-item command="revert" :disabled="![2, 3].includes(status)">状态回退</el-dropdown-item>
                    <el-dropdown-item command="void" divided>作废项目</el-dropdown-item>
                    <el-dropdown-item command="delete" :disabled="!canDelete">删除项目</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </div>
        </header>

        <!-- 作废警示 -->
        <div v-if="isVoided" class="eng-banner eng-banner--void">
          项目已于 {{ formatEngDate(project.voided_at) }} 作废{{
            project.void_reason ? `：${project.void_reason}` : ''
          }}，不计入任何统计
        </div>

        <!-- ===== Tab（移动端横向滑动，右侧渐隐提示还能往右） ===== -->
        <div class="eng-tabs-wrap">
        <nav class="eng-tabs">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            type="button"
            class="eng-tab"
            :class="activeTab === tab.key ? 'eng-tab--active' : ''"
            @click="activeTab = tab.key"
          >
            {{ tab.label }}
            <span v-if="tab.count !== null" class="eng-tab-count">{{ tab.count }}</span>
            <span v-if="tab.dot" class="eng-tab-dot" />
          </button>
        </nav>
        </div>

        <!-- ===== 内容 ===== -->
        <div class="mt-3">
          <EngineeringOverviewPanel
            v-if="activeTab === 'overview'"
            :project="project"
            @goto-tab="k => (activeTab = k)"
            @accept="acceptVisible = true"
          />

          <EngineeringItemTable
            v-else-if="activeTab === 'items'"
            ref="itemTableRef"
            :items="items"
            :status="status"
            :saving="actionLoading"
            @save="onSaveItems"
          />

          <EngineeringPaymentPanel
            v-else-if="activeTab === 'payment'"
            ref="paymentPanelRef"
            :terms="terms"
            :receipts="receipts"
            :contract-total="Number(project.contract_total) || 0"
            :sign-date="project.sign_date || ''"
            :status="status"
            :saving="actionLoading"
            @save-terms="onSaveTerms"
            @add-receipt="openReceipt"
            @reverse-receipt="openReverse"
          />

          <EngineeringShipmentPanel
            v-else-if="activeTab === 'shipment'"
            :shipments="shipments"
            :project="project"
            :can-ship="[2, 3].includes(status)"
            @add="openShipment"
          />

          <EngineeringExpensePanel
            v-else-if="activeTab === 'expense'"
            :project-id="project.id"
            :expenses="expenses"
            :expense-summary="project.expense_summary"
            :status="status"
            @changed="refresh"
          />
        </div>
      </template>
    </div>

    <!-- ===== 弹层 ===== -->
    <EngineeringReceiptDialog
      v-model="receiptVisible"
      :terms="terms"
      :term="receiptTerm"
      :saving="actionLoading"
      @confirm="onConfirmReceipt"
    />

    <EngineeringShipmentDialog
      ref="shipmentDialogRef"
      v-model="shipmentVisible"
      :items="items"
      :saving="actionLoading"
      @confirm="onConfirmShipment"
    />

    <EngineeringVoidDialog v-model="voidVisible" :project="project" mode="void" @saved="afterVoid" />
    <EngineeringVoidDialog v-model="revertVisible" :project="project" mode="revert" @saved="refresh" />

    <EngineeringProjectDrawer v-model="drawerVisible" :project="project" :employees="employees" @saved="onDrawerSaved" />

    <!-- 冲销回款 -->
    <el-dialog
      v-model="reverseVisible"
      title="冲销回款"
      :width="isMobile ? '92%' : '460px'"
      :close-on-click-modal="false"
    >
      <div v-if="reverseTarget" class="eng-reverse">
        <div class="eng-reverse-card">
          <p class="eng-reverse-line">
            <span>原记录</span>
            <b>{{ formatEngDate(reverseTarget.payment_date) }} · {{ reverseTarget.term_name }}</b>
          </p>
          <p class="eng-reverse-line">
            <span>冲销金额</span>
            <b style="color: #dc2626">-{{ formatEngMoney(reverseTarget.amount) }}</b>
          </p>
        </div>
        <el-form label-position="top">
          <el-form-item label="冲销原因" required>
            <el-input
              v-model="reverseReason"
              type="textarea"
              :rows="3"
              maxlength="200"
              show-word-limit
              placeholder="如：银行退回，账号信息有误"
            />
            <p v-if="reverseReasonTooShort" class="eng-err">原因至少 5 个字</p>
          </el-form-item>
        </el-form>
        <p class="eng-reverse-tip">冲销不删除原记录，会生成一条负数回款，操作历史可追溯</p>
      </div>
      <template #footer>
        <div class="flex gap-2">
          <el-button class="flex-1 md:flex-none" @click="reverseVisible = false">取消</el-button>
          <el-button
            class="flex-1 md:flex-none"
            type="danger"
            :loading="actionLoading"
            :disabled="reverseReasonTooShort"
            @click="confirmReverse"
          >
            确认冲销
          </el-button>
        </div>
      </template>
    </el-dialog>

    <!-- 确认验收 -->
    <el-dialog v-model="acceptVisible" title="确认验收" :width="isMobile ? '92%' : '440px'" :close-on-click-modal="false">
      <el-form label-position="top">
        <el-form-item label="验收日期">
          <el-date-picker
            v-model="acceptForm.date"
            type="date"
            value-format="YYYY-MM-DD"
            :disabled-date="date => date.getTime() > Date.now()"
            class="w-full"
          />
        </el-form-item>
        <el-form-item label="验收备注">
          <el-input
            v-model="acceptForm.remark"
            type="textarea"
            :rows="2"
            maxlength="200"
            show-word-limit
            placeholder="选填"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="flex gap-2">
          <el-button class="flex-1 md:flex-none" @click="acceptVisible = false">取消</el-button>
          <el-button class="flex-1 md:flex-none" type="primary" :loading="actionLoading" @click="confirmAccept">
            确认验收
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteEngineeringProject } from '../../api/engineering'
import { fetchSimpleEmployees } from '../../api/employees'
import { useEngineeringProject } from '../../composables/useEngineeringProject'
import { formatCardMoney, formatEngDate, formatEngMoney } from '../../utils/engineeringFormat'
import EngineeringStatusTag from '../../components/engineering/EngineeringStatusTag.vue'
import EngineeringOverviewPanel from '../../components/engineering/EngineeringOverviewPanel.vue'
import EngineeringItemTable from '../../components/engineering/EngineeringItemTable.vue'
import EngineeringPaymentPanel from '../../components/engineering/EngineeringPaymentPanel.vue'
import EngineeringShipmentPanel from '../../components/engineering/EngineeringShipmentPanel.vue'
import EngineeringExpensePanel from '../../components/engineering/EngineeringExpensePanel.vue'
import EngineeringReceiptDialog from '../../components/engineering/EngineeringReceiptDialog.vue'
import EngineeringShipmentDialog from '../../components/engineering/EngineeringShipmentDialog.vue'
import EngineeringVoidDialog from '../../components/engineering/EngineeringVoidDialog.vue'
import EngineeringProjectDrawer from '../../components/engineering/EngineeringProjectDrawer.vue'

const route = useRoute()
const router = useRouter()

const {
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
  allShipped,
  termsBalanced,
  load,
  refresh,
  sign,
  accept,
  saveItems,
  saveTerms,
  addReceipt,
  reverseReceipt,
  addShipment
} = useEngineeringProject()

const projectId = computed(() => Number(route.params.id) || 0)

const isMobile = ref(window.innerWidth < 768)
window.addEventListener('resize', () => {
  isMobile.value = window.innerWidth < 768
})

const activeTab = ref('overview')
const itemTableRef = ref(null)
const paymentPanelRef = ref(null)
const shipmentDialogRef = ref(null)

// Tab 与 URL query 同步：既方便把"某个项目的费用 Tab"直接发给同事，
// 也让刷新后停在原分区（刷新不该跳回概览）
const VALID_TABS = ['overview', 'items', 'payment', 'shipment', 'expense']
if (VALID_TABS.includes(route.query.tab)) activeTab.value = route.query.tab
watch(activeTab, tab => {
  if (route.query.tab === tab) return
  const query = { ...route.query }
  if (tab === 'overview') delete query.tab
  else query.tab = tab
  router.replace({ query })
})

const paidTermCount = computed(() => terms.value.filter(t => Number(t.pending_amount) <= 0).length)

const tabs = computed(() => [
  { key: 'overview', label: '概览', count: null, dot: false },
  { key: 'items', label: '设备清单', count: items.value.length, dot: false },
  { key: 'payment', label: '回款', count: `${paidTermCount.value}/${terms.value.length}`, dot: false },
  { key: 'shipment', label: '出库', count: shipments.value.length, dot: false },
  { key: 'expense', label: '费用', count: expenses.value.length, dot: expenses.value.length > 0 }
])

// 签约前置条件：有清单 + 节点合计等于合同额（§5.8）
const canSign = computed(() => {
  if (!project.value) return false
  if (!items.value.length) return false
  return termsBalanced.value
})
const signBlockReason = computed(() => {
  if (!items.value.length) return '设备清单为空，先添加商品再签约'
  const total = terms.value.reduce((s, t) => s + (Number(t.planned_amount) || 0), 0)
  const diff = Math.abs(total - Number(project.value?.contract_total || 0))
  return `付款节点合计与合同总额相差 ${formatEngMoney(diff)}，请先调平`
})

const canDelete = computed(() => status.value === 1 && receipts.value.length === 0 && shipments.value.length === 0)

const goBack = () => {
  // 优先浏览器返回，保留列表页筛选与页码（§2.2）
  if (window.history.length > 1) router.back()
  else router.push('/admin/engineering')
}

// ---------- 签约 ----------
const confirmSign = async () => {
  const termsTotal = terms.value.reduce((s, t) => s + (Number(t.planned_amount) || 0), 0)
  try {
    await ElMessageBox.confirm(
      `合同总额 ${formatEngMoney(project.value.contract_total)}，付款节点合计 ${formatEngMoney(
        termsTotal
      )}，签约后金额锁定。`,
      '确认签约',
      { confirmButtonText: '确认签约', cancelButtonText: '取消', type: 'warning' }
    )
  } catch (e) {
    return
  }
  try {
    // 签约日期优先用主档里已填的，未填则默认今天（后端已签约状态要求签约日期非空）
    const d = new Date()
    const pad = n => String(n).padStart(2, '0')
    const today = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
    await sign({ sign_date: project.value.sign_date || today })
    ElMessage.success('已签约，金额已锁定')
  } catch (e) {
    /* 拦截器已提示 */
  }
}

// ---------- 验收 ----------
const acceptVisible = ref(false)
const acceptForm = reactive({ date: '', remark: '' })

watch(acceptVisible, open => {
  if (!open) return
  const d = new Date()
  const pad = n => String(n).padStart(2, '0')
  acceptForm.date = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  acceptForm.remark = ''
})

const confirmAccept = async () => {
  try {
    const res = await accept({ accept_date: acceptForm.date, reason: acceptForm.remark })
    acceptVisible.value = false
    // 款项在验收前已收齐时，后端会在同一事务里直接置「已结清」，提示要说清楚结果
    ElMessage.success(res?.result?.settled ? '已验收，款项已收齐，项目自动结清' : '已确认验收')
  } catch (e) {
    /* 拦截器已提示 */
  }
}

// ---------- 回款 ----------
const receiptVisible = ref(false)
const receiptTerm = ref(null)

const openReceipt = term => {
  receiptTerm.value = term && Number(term.pending_amount) > 0 ? term : null
  receiptVisible.value = true
}

const onConfirmReceipt = async payload => {
  try {
    const res = await addReceipt(payload)
    receiptVisible.value = false
    const data = res?.result || {}
    if (data.settled) {
      ElMessage.success('回款已登记，项目已结清')
    } else {
      ElMessage.success(`回款已登记，项目剩余待回 ${formatEngMoney(data.project_pending)}`)
    }
  } catch (e) {
    /* 拦截器已提示 */
  }
}

// ---------- 冲销 ----------
const reverseVisible = ref(false)
const reverseTarget = ref(null)
const reverseReason = ref('')
const reverseReasonTooShort = computed(
  () => reverseReason.value.trim().length > 0 && reverseReason.value.trim().length < 5
)

const openReverse = receipt => {
  reverseTarget.value = receipt
  reverseReason.value = ''
  reverseVisible.value = true
}

const confirmReverse = async () => {
  const reason = reverseReason.value.trim()
  if (reason.length < 5) return
  try {
    await reverseReceipt(reverseTarget.value.id, { reason })
    reverseVisible.value = false
    ElMessage.success(`已冲销 ${formatEngMoney(reverseTarget.value.amount)}`)
  } catch (e) {
    /* 拦截器已提示 */
  }
}

// ---------- 出库 ----------
const shipmentVisible = ref(false)

const openShipment = () => {
  shipmentVisible.value = true
}

const onConfirmShipment = async payload => {
  try {
    const res = await addShipment(payload)
    shipmentVisible.value = false
    ElMessage.success(`已出库 ${res?.result?.total_qty || 0} 台，库存已更新`)
  } catch (e) {
    const msg = e?.response?.data?.msg || ''
    // 库存不足：切成"确认负库存"态再提交一次（§8）
    if (msg.includes('库存不足')) shipmentDialogRef.value?.showStockWarning(msg)
  }
}

// ---------- 清单 / 节点保存 ----------
const onSaveItems = async payload => {
  try {
    await saveItems(payload)
    itemTableRef.value?.finishSave()
    ElMessage.success('清单已保存')
  } catch (e) {
    /* 平账/校验失败保留编辑态，让用户改 */
  }
}

const onSaveTerms = async payload => {
  try {
    await saveTerms(payload)
    paymentPanelRef.value?.finishSave()
    ElMessage.success('付款节点已保存')
  } catch (e) {
    /* 保留编辑态 */
  }
}

// ---------- 更多菜单 ----------
const voidVisible = ref(false)
const revertVisible = ref(false)
const drawerVisible = ref(false)
const employees = ref([])

const onMoreCommand = async command => {
  if (command === 'edit') {
    drawerVisible.value = true
    return
  }
  if (command === 'revert') {
    revertVisible.value = true
    return
  }
  if (command === 'void') {
    voidVisible.value = true
    return
  }
  if (command === 'delete') {
    try {
      await ElMessageBox.confirm('删除后不可恢复，确定删除该工程？', '删除工程', {
        type: 'warning',
        confirmButtonText: '确认删除',
        cancelButtonText: '取消',
        confirmButtonClass: 'el-button--danger'
      })
    } catch (e) {
      return
    }
    try {
      await deleteEngineeringProject(project.value.id)
      ElMessage.success('项目已删除')
      router.replace('/admin/engineering')
    } catch (e) {
      /* 拦截器已提示 */
    }
  }
}

const afterVoid = () => {
  voidVisible.value = false
  refresh()
}

const onDrawerSaved = () => {
  drawerVisible.value = false
  refresh()
}

onMounted(async () => {
  await load(projectId.value)
  try {
    const res = await fetchSimpleEmployees()
    employees.value = res?.result || []
  } catch (e) {
    employees.value = []
  }
})
</script>

<style scoped>
.eng-back {
  font-size: 13px;
  color: #64748b;
  background: none;
  border: none;
  padding: 0;
  cursor: pointer;
}
.eng-back:hover {
  color: #635bff;
}
.eng-head {
  margin-top: 10px;
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 16px 18px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
}
.eng-head-main {
  min-width: 0;
  flex: 1 1 380px;
}
.eng-head-title-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.eng-h1 {
  font-size: 22px;
  font-weight: 700;
  color: #0f172a;
  line-height: 30px;
}
/* 全局 style.css 在 ≤767px 给 h1 加了 nowrap，长工程名会顶出屏幕，这里允许折两行 */
@media (max-width: 767px) {
  .eng-h1 {
    white-space: normal;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    font-size: 19px;
    line-height: 26px;
  }
}
.eng-no-chip {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  color: #475569;
  background: #f1f5f9;
  padding: 2px 8px;
  border-radius: 6px;
}
.eng-head-meta {
  margin-top: 6px;
  font-size: 12px;
  color: #94a3b8;
  line-height: 18px;
}
.eng-sep {
  margin: 0 5px;
  color: #cbd5e1;
}
.eng-head-right {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 10px;
}
.eng-money-triple {
  display: flex;
  gap: 26px;
}
.eng-money-label {
  font-size: 11px;
  color: #94a3b8;
  line-height: 16px;
}
.eng-money-num {
  font-size: 18px;
  font-weight: 900;
  color: #0f172a;
  line-height: 26px;
  font-variant-numeric: tabular-nums;
}
.eng-head-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.eng-banner {
  margin-top: 12px;
  border-radius: 12px;
  padding: 10px 14px;
  font-size: 12px;
}
.eng-banner--void {
  background: #f8fafc;
  color: #64748b;
  border: 1px dashed #e2e8f0;
}
.eng-tabs-wrap {
  position: relative;
}
/* 只在移动端给右侧渐隐，提示 Tab 还可以右滑（桌面 5 个 Tab 放得下） */
@media (max-width: 767px) {
  .eng-tabs-wrap::after {
    content: '';
    position: absolute;
    top: 0;
    right: 0;
    bottom: 1px;
    width: 28px;
    pointer-events: none;
    background: linear-gradient(90deg, rgba(248, 250, 252, 0), #f8fafc 70%);
  }
}
.eng-tabs {
  display: flex;
  gap: 4px;
  margin-top: 14px;
  border-bottom: 1px solid #f1f5f9;
  overflow-x: auto;
  scrollbar-width: none;
}
.eng-tabs::-webkit-scrollbar {
  display: none;
}
.eng-tab {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 9px 12px;
  font-size: 13px;
  color: #64748b;
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  cursor: pointer;
  white-space: nowrap;
}
/* 移动端把 Tab 左右内边距收到 8px，5 个 Tab（含计数）刚好铺满 343px，不必横滑 */
@media (max-width: 767px) {
  .eng-tabs {
    gap: 2px;
  }
  .eng-tab {
    padding: 9px 7px;
    font-size: 13px;
    gap: 4px;
  }
}
.eng-tab--active {
  color: #635bff;
  font-weight: 600;
  border-bottom-color: #635bff;
}
.eng-tab-count {
  font-size: 11px;
  color: #94a3b8;
  font-variant-numeric: tabular-nums;
}
.eng-tab--active .eng-tab-count {
  color: #635bff;
}
.eng-tab-dot {
  width: 4px;
  height: 4px;
  border-radius: 999px;
  background: #f59e0b;
}
.eng-skeleton {
  border-radius: 16px;
  background: #fff;
  border: 1px solid #f1f5f9;
}
.eng-skeleton--head {
  height: 120px;
}
.eng-skeleton--body {
  height: 220px;
}
.eng-reverse-card {
  border-radius: 10px;
  background: #f8fafc;
  padding: 10px 12px;
  margin-bottom: 12px;
}
.eng-reverse-line {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: #64748b;
  line-height: 22px;
}
.eng-reverse-line b {
  color: #0f172a;
}
.eng-reverse-tip {
  font-size: 11px;
  color: #94a3b8;
}
.eng-err {
  margin-top: 2px;
  font-size: 11px;
  color: #dc2626;
}
@media (max-width: 767px) {
  .eng-head-right {
    width: 100%;
    align-items: stretch;
  }
  .eng-money-triple {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 8px;
  }
  .eng-money-num {
    font-size: 15px;
  }
  .eng-head-actions :deep(.el-button) {
    flex: 1;
  }
}
</style>
