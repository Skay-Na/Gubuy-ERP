<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ArrowRight, Management, Plus, Switch } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import {
  fetchFinanceWorkspace,
  fetchFinanceLogs,
  createAccount as createAccountApi,
  transferAccounts,
  calibrateAccount as calibrateAccountApi
} from '../api/finance'
import { formatMoney, formatDateTime } from '../utils/format'

// ---------- 资金工作台聚合数据 ----------
const currentMonth = () => {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}
const ws = ref(null)
const wsLoading = ref(false)
const monthMode = ref('current') // current | cumulative，只作用于经营概况卡
const dismissAlertTypes = ref([]) // 用户手动关闭的异常类型

const fetchWorkspace = async () => {
  wsLoading.value = true
  try {
    const month = monthMode.value === 'cumulative' ? 'cumulative' : currentMonth()
    const res = await fetchFinanceWorkspace({ month })
    if (res.data.code === 200) {
      ws.value = res.data.data
    }
  } catch {
    // 静默失败，卡片显示占位
  } finally {
    wsLoading.value = false
  }
}

const visibleAlerts = computed(() =>
  (ws.value?.alerts || []).filter(alert => !dismissAlertTypes.value.includes(alert.type))
)
const dismissAlert = (alert) => {
  dismissAlertTypes.value = [...dismissAlertTypes.value, alert.type]
}

// ---------- 问候与格式化 ----------
const greeting = computed(() => {
  const hour = new Date().getHours()
  if (hour < 6) return '夜深了'
  if (hour < 12) return '早上好'
  if (hour < 18) return '下午好'
  return '晚上好'
})
const todayLabel = computed(() => {
  const now = new Date()
  const weekdays = ['日', '一', '二', '三', '四', '五', '六']
  return `${now.getFullYear()} 年 ${now.getMonth() + 1} 月 ${now.getDate()} 日 星期${weekdays[now.getDay()]}`
})
const money = value => `¥${formatMoney(value)}`
const formatTime = value => formatDateTime(value)

// ---------- 待我处理 ----------
const todos = computed(() => {
  const raw = ws.value?.todos || {}
  return [
    { key: 'subsidy_to_allocate', label: '国补到账待核销', count: raw.subsidy_to_allocate?.count || 0, amount: raw.subsidy_to_allocate?.amount || 0, to: '/admin/subsidy' },
    { key: 'subsidy_to_submit', label: '国补资料待提交', count: raw.subsidy_to_submit?.count || 0, amount: raw.subsidy_to_submit?.amount || 0, to: '/admin/subsidy' },
    { key: 'balance_due', label: '尾款待收', count: raw.balance_due?.count || 0, amount: raw.balance_due?.amount || 0, to: '/admin/orders' }
  ]
})

// ---------- 钱在哪 ----------
const funds = computed(() => ws.value?.funds || { accounts: [], frozen: { count: 0, amount: 0 }, available: 0, total: 0 })

// ---------- 经营概况 ----------
const monthData = computed(() => ws.value?.month || {})

// ---------- 最近流水 ----------
const recentFlows = computed(() => ws.value?.recent_flows || [])

// ---------- 流水明细抽屉（点击账户/流水时复用财务流水接口） ----------
const flowsDrawer = reactive({ visible: false, title: '', loading: false, rows: [], total: 0 })
const openFlows = async (title, params) => {
  flowsDrawer.visible = true
  flowsDrawer.title = title
  flowsDrawer.loading = true
  try {
    const res = await fetchFinanceLogs({ page_size: 20, ...params })
    if (res.data.code === 200) {
      flowsDrawer.rows = res.data.data?.logs || res.data.data || []
      flowsDrawer.total = res.data.total ?? flowsDrawer.rows.length
    }
  } catch {
    ElMessage.error('流水查询失败，请稍后重试')
  } finally {
    flowsDrawer.loading = false
  }
}
const openAccountFlows = (account) => openFlows(`${account.name} · 最近流水`, { account_id: account.id })
const openFlowDetail = (flow) => openFlows(flow.related_no ? `流水明细 · ${flow.related_no}` : `流水明细 · ${flow.category}`, { keyword: flow.related_no || flow.category })

// ---------- 账户操作（既有功能区保留） ----------
const accountDialogVisible = ref(false)
const transferDialogVisible = ref(false)
const calibrateDialogVisible = ref(false)
const submitting = ref(false)
const accountFormRef = ref()
const transferFormRef = ref()
const accounts = computed(() => funds.value.accounts || [])
const accountForm = reactive({ name: '', balance: 0 })
const transferForm = reactive({ from_account_id: null, to_account_id: null, amount: 0, remark: '' })
const calibrateForm = reactive({ accountId: null, accountName: '', currentBalance: 0, realBalance: 0, remark: '' })
const calibrateDiff = computed(() => calibrateForm.realBalance - calibrateForm.currentBalance)

async function refreshAll() { await fetchWorkspace() }
function openCalibrate(account) { Object.assign(calibrateForm, { accountId: account.id, accountName: account.name, currentBalance: Number(account.balance || 0), realBalance: Number(account.balance || 0), remark: '' }); calibrateDialogVisible.value = true }
async function createAccount() {
  if (!accountFormRef.value || !await accountFormRef.value.validate().catch(() => false)) return
  submitting.value = true
  try { const response = await createAccountApi(accountForm); if (response.data.code !== 200) return ElMessage.error(response.data.msg || '创建失败'); ElMessage.success('账户已创建'); accountDialogVisible.value = false; Object.assign(accountForm, { name: '', balance: 0 }); await refreshAll() } catch (error) { ElMessage.error(error.response?.data?.msg || '创建失败') } finally { submitting.value = false }
}
async function transferFunds() {
  if (!transferFormRef.value || !await transferFormRef.value.validate().catch(() => false)) return
  if (transferForm.from_account_id === transferForm.to_account_id) return ElMessage.warning('转出和转入账户不能相同')
  submitting.value = true
  try { const response = await transferAccounts(transferForm); if (response.data.code !== 200) return ElMessage.error(response.data.msg || '划拨失败'); ElMessage.success('资金划拨成功'); transferDialogVisible.value = false; Object.assign(transferForm, { from_account_id: null, to_account_id: null, amount: 0, remark: '' }); await refreshAll() } catch (error) { ElMessage.error(error.response?.data?.msg || '划拨失败') } finally { submitting.value = false }
}
async function calibrateAccount() {
  if (!calibrateDiff.value) return
  submitting.value = true
  try { const response = await calibrateAccountApi(calibrateForm.accountId, { real_balance: calibrateForm.realBalance, remark: calibrateForm.remark }); if (response.data.code !== 200) return ElMessage.error(response.data.msg || '校准失败'); ElMessage.success('账户余额已校准，并保留审计记录'); calibrateDialogVisible.value = false; await refreshAll() } catch (error) { ElMessage.error(error.response?.data?.msg || '校准失败') } finally { submitting.value = false }
}
onMounted(refreshAll)
</script>

<template>
  <main class="finance-page">
    <header class="finance-header">
      <div>
        <div class="page-kicker">FINANCE WORKSPACE</div>
        <h1>{{ greeting }}，老板</h1>
        <p>{{ todayLabel }} · 本月已记账 <b class="hl">{{ ws?.month?.booked_count ?? '—' }}</b> 笔</p>
      </div>
      <div class="header-actions">
        <el-button type="primary" :icon="Plus" @click="$router.push('/admin/bookkeeping')">记一笔</el-button>
        <el-button @click="$router.push('/admin/subsidy')">核销国补</el-button>
        <el-button @click="$router.push('/admin/orders')">记尾款</el-button>
        <el-button link type="primary" @click="$router.push('/admin/bookkeeping')">前往记账</el-button>
        <el-dropdown trigger="click">
          <el-button :icon="Management">账户操作</el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item :icon="Switch" @click="transferDialogVisible = true">资金划拨</el-dropdown-item>
              <el-dropdown-item :icon="Plus" @click="accountDialogVisible = true">新增资金账户</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </header>

    <!-- 异常横幅 -->
    <el-alert
      v-for="alert in visibleAlerts"
      :key="alert.type"
      :title="alert.message"
      type="warning"
      show-icon
      closable
      class="ws-alert"
      @close="dismissAlert(alert)"
    />

    <!-- 待我处理 -->
    <el-card shadow="never" class="ws-card" v-loading="wsLoading">
      <div class="ws-card-title">待我处理</div>
      <div class="todo-rows">
        <div v-for="todo in todos" :key="todo.key" class="todo-row" @click="$router.push(todo.to)">
          <span class="todo-label">{{ todo.label }}</span>
          <span class="todo-count">{{ todo.count }} 笔</span>
          <b class="todo-amount">{{ money(todo.amount) }}</b>
          <span class="todo-go">去处理 <el-icon><ArrowRight /></el-icon></span>
        </div>
      </div>
    </el-card>

    <!-- 钱在哪 -->
    <el-card shadow="never" class="ws-card" v-loading="wsLoading">
      <div class="funds-grid">
        <div class="funds-main">
          <div class="ws-card-title">钱在哪</div>
          <div class="funds-available">{{ money(funds.available) }}</div>
          <div class="funds-sub">可用资金</div>
          <el-tag v-if="funds.frozen?.count > 0" type="warning" effect="plain" class="funds-frozen" size="small">
            公户冻结 {{ money(funds.frozen.amount) }} · {{ funds.frozen.count }} 笔
          </el-tag>
          <div class="funds-total">4 账户账面合计 <b>{{ money(funds.total) }}</b></div>
        </div>
        <div class="funds-list">
          <div v-for="account in accounts" :key="account.id" class="funds-account" @click="openAccountFlows(account)">
            <span class="funds-account-name">{{ account.name }}</span>
            <b>{{ money(account.balance) }}</b>
            <el-icon class="funds-account-go"><ArrowRight /></el-icon>
          </div>
        </div>
      </div>
    </el-card>

    <!-- 本月经营概况 -->
    <el-card shadow="never" class="ws-card" v-loading="wsLoading">
      <div class="month-head">
        <div class="ws-card-title">经营概况</div>
        <el-radio-group v-model="monthMode" size="small" @change="fetchWorkspace">
          <el-radio-button value="current">本月</el-radio-button>
          <el-radio-button value="cumulative">累计</el-radio-button>
        </el-radio-group>
      </div>
      <div class="profit-grid">
        <div class="profit-cell">
          <span class="profit-label">净利润<small>毛利扣费用</small></span>
          <b :class="Number(monthData.net_profit || 0) >= 0 ? 'pos' : 'neg'">{{ money(monthData.net_profit) }}</b>
        </div>
        <div class="profit-cell">
          <span class="profit-label">真实净利润<small>扣完费用工资</small></span>
          <b :class="Number(monthData.real_profit || 0) >= 0 ? 'pos' : 'neg'">{{ money(monthData.real_profit) }}</b>
        </div>
      </div>
      <div class="bridge-rows">
        <div class="bridge-row"><span>国补未回款</span><b>{{ money(monthData.bridge?.subsidy_unreceived) }}</b></div>
        <div class="bridge-row"><span>经营开销（工资 {{ money(monthData.payroll_total) }}）</span><b>{{ money(monthData.bridge?.operating_expense) }}</b></div>
        <div class="bridge-row"><span>净现金流</span><b>{{ money(monthData.net_cash_flow) }}</b></div>
        <div class="bridge-row muted"><span>净利润-净现金流差异</span><b>{{ money(monthData.bridge?.profit_cash_gap) }}</b></div>
      </div>
      <div v-if="monthData.uncosted_order_item_count > 0" class="ws-warn">* 本期有 {{ monthData.uncosted_order_item_count }} 个商品未录成本，毛利可能偏高</div>
    </el-card>

    <!-- 最近流水 -->
    <el-card shadow="never" class="ws-card" v-loading="wsLoading" body-class="!p-0">
      <div class="ws-card-title recent-title">最近流水</div>
      <el-table :data="recentFlows" size="small" style="width: 100%" empty-text="暂无资金流水" @row-click="openFlowDetail">
        <el-table-column label="时间" width="110">
          <template #default="{ row }"><span class="muted-cell">{{ formatTime(row.created_at) }}</span></template>
        </el-table-column>
        <el-table-column prop="account_name" label="账户" width="100" class-name="col-account" />
        <el-table-column prop="category" label="分类" min-width="100" show-overflow-tooltip class-name="col-category" />
        <el-table-column prop="customer_name" label="客户" min-width="100" show-overflow-tooltip />
        <el-table-column label="金额" align="right" width="140">
          <template #default="{ row }">
            <b :class="row.flow_type === 1 ? 'pos' : 'neg'">{{ row.flow_type === 1 ? '+' : '-' }}{{ money(row.amount) }}</b>
          </template>
        </el-table-column>
        <el-table-column prop="related_no" label="单号" min-width="130" show-overflow-tooltip class-name="col-related" />
      </el-table>
    </el-card>

    <!-- 流水明细抽屉 -->
    <el-drawer v-model="flowsDrawer.visible" :title="flowsDrawer.title" size="480px" class="finance-flow-drawer">
      <el-table :data="flowsDrawer.rows" v-loading="flowsDrawer.loading" size="small" style="width: 100%">
        <el-table-column label="时间" width="100">
          <template #default="{ row }"><span class="muted-cell">{{ formatTime(row.created_at) }}</span></template>
        </el-table-column>
        <el-table-column label="账户" width="90">
          <template #default="{ row }">{{ row.account?.name }}</template>
        </el-table-column>
        <el-table-column prop="category" label="分类" min-width="90" show-overflow-tooltip />
        <el-table-column label="金额" align="right" width="120">
          <template #default="{ row }">
            <b :class="row.flow_type === 1 ? 'pos' : 'neg'">{{ row.flow_type === 1 ? '+' : '-' }}{{ money(row.amount) }}</b>
          </template>
        </el-table-column>
        <el-table-column prop="related_no" label="单号" min-width="120" show-overflow-tooltip />
      </el-table>
    </el-drawer>

    <el-dialog v-model="accountDialogVisible" title="新增资金账户" width="440px" destroy-on-close><el-form ref="accountFormRef" :model="accountForm" label-width="92px"><el-form-item label="账户名称" prop="name" :rules="[{ required: true, message: '请输入账户名称' }]"><el-input v-model="accountForm.name" placeholder="例如：支付宝、对公账户、备用金" /></el-form-item><el-form-item label="初始余额"><el-input-number v-model="accountForm.balance" :min="0" :precision="2" class="full-width" /></el-form-item></el-form><template #footer><el-button @click="accountDialogVisible = false">取消</el-button><el-button type="primary" :loading="submitting" @click="createAccount">创建账户</el-button></template></el-dialog>
    <el-dialog v-model="transferDialogVisible" title="账户资金划拨" width="500px" destroy-on-close><el-alert title="划拨只移动账户间资金，不计入收入或支出。" type="info" :closable="false" show-icon class="dialog-alert" /><el-form ref="transferFormRef" :model="transferForm" label-width="92px"><el-form-item label="转出账户" prop="from_account_id" :rules="[{ required: true, message: '请选择转出账户' }]"><el-select v-model="transferForm.from_account_id" class="full-width"><el-option v-for="account in accounts" :key="account.id" :label="`${account.name}（${formatMoney(account.balance || 0)}）`" :value="account.id" /></el-select></el-form-item><el-form-item label="转入账户" prop="to_account_id" :rules="[{ required: true, message: '请选择转入账户' }]"><el-select v-model="transferForm.to_account_id" class="full-width"><el-option v-for="account in accounts" :key="account.id" :label="account.name" :value="account.id" /></el-select></el-form-item><el-form-item label="划拨金额" prop="amount" :rules="[{ required: true, message: '请输入划拨金额' }]"><el-input-number v-model="transferForm.amount" :min="0.01" :precision="2" class="full-width" /></el-form-item><el-form-item label="备注"><el-input v-model="transferForm.remark" placeholder="例如：补充备用金" /></el-form-item></el-form><template #footer><el-button @click="transferDialogVisible = false">取消</el-button><el-button type="primary" :loading="submitting" @click="transferFunds">确认划拨</el-button></template></el-dialog>
    <el-dialog v-model="calibrateDialogVisible" :title="`核对余额：${calibrateForm.accountName}`" width="460px" destroy-on-close><el-alert title="校准会将账面余额对齐为实际余额，并自动保留一条校准审计记录。" type="warning" :closable="false" show-icon class="dialog-alert" /><el-form :model="calibrateForm" label-position="top"><el-form-item label="当前账面余额"><strong class="current-balance">¥{{ formatMoney(calibrateForm.currentBalance) }}</strong></el-form-item><el-form-item label="实际余额"><el-input-number v-model="calibrateForm.realBalance" :min="0" :precision="2" class="full-width" /><small v-if="calibrateDiff" :class="calibrateDiff > 0 ? 'diff-positive' : 'diff-negative'">差异 {{ calibrateDiff > 0 ? '+' : '' }}¥{{ formatMoney(calibrateDiff) }}</small></el-form-item><el-form-item label="备注"><el-input v-model="calibrateForm.remark" placeholder="例如：月末对账" /></el-form-item></el-form><template #footer><el-button @click="calibrateDialogVisible = false">取消</el-button><el-button type="warning" :loading="submitting" :disabled="!calibrateDiff" @click="calibrateAccount">确认校准</el-button></template></el-dialog>
  </main>
</template>

<style scoped>
.finance-page { max-width: 1320px; min-height: 100%; margin: 0 auto; padding: 28px; display: grid; gap: 18px; background: #f8fafc; align-content: start; }
.finance-header { display: flex; align-items: center; justify-content: space-between; gap: 20px; }
.page-kicker { color: #2563eb; font-size: 11px; font-weight: 800; letter-spacing: .14em; }
.finance-header h1 { margin: 4px 0; color: #0f172a; font-size: 28px; letter-spacing: -.04em; }
.finance-header p { margin: 0; color: #64748b; font-size: 13px; }
.finance-header p .hl { color: #2563eb; }
.header-actions { display: flex; align-items: center; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.ws-alert { border-radius: 12px; }
.ws-card { border-radius: 16px; }
.ws-card-title { font-size: 15px; font-weight: 800; color: #0f172a; margin-bottom: 14px; }
.recent-title { padding: 16px 16px 0; }
.full-width { width: 100%; }
.dialog-alert { margin-bottom: 18px; }
.current-balance { color: #334155; font-size: 22px; }
.diff-positive { display: block; margin-top: 7px; color: #059669; }
.diff-negative { display: block; margin-top: 7px; color: #e11d48; }
.pos { color: #059669; }
.neg { color: #e11d48; }
.muted-cell { color: #94a3b8; font-size: 12px; }

.todo-rows { display: grid; gap: 6px; }
.todo-row { display: grid; grid-template-columns: 1fr 70px minmax(110px, auto) auto; align-items: center; gap: 14px; padding: 12px 14px; border: 1px solid #f1f5f9; border-radius: 12px; cursor: pointer; transition: background .15s; }
.todo-row:hover { background: #f0f7ff; border-color: #dbeafe; }
.todo-label { font-weight: 700; color: #334155; }
.todo-count { color: #64748b; font-size: 13px; }
.todo-amount { color: #0f172a; }
.todo-go { display: inline-flex; align-items: center; gap: 4px; color: #2563eb; font-size: 13px; font-weight: 600; }

.funds-grid { display: grid; grid-template-columns: minmax(260px, 1fr) minmax(280px, 1.2fr); gap: 24px; align-items: center; }
.funds-available { font-size: 34px; font-weight: 900; letter-spacing: -.04em; color: #0f172a; }
.funds-sub { color: #64748b; font-size: 13px; margin-top: 2px; }
.funds-frozen { margin-top: 10px; }
.funds-total { margin-top: 12px; color: #64748b; font-size: 13px; }
.funds-total b { color: #334155; }
.funds-list { display: grid; gap: 8px; }
.funds-account { display: flex; align-items: center; gap: 12px; padding: 11px 14px; border: 1px solid #f1f5f9; border-radius: 12px; cursor: pointer; transition: background .15s; }
.funds-account:hover { background: #f0f7ff; border-color: #dbeafe; }
.funds-account-name { flex: 1; font-weight: 700; color: #334155; }
.funds-account-go { color: #cbd5e1; }

.month-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 4px; }
.month-head .ws-card-title { margin-bottom: 0; }
.profit-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin: 14px 0; }
.profit-cell { padding: 16px; border: 1px solid #f1f5f9; border-radius: 12px; background: #f8fafc; }
.profit-label { display: block; color: #64748b; font-size: 12.5px; font-weight: 600; margin-bottom: 6px; }
.profit-label small { margin-left: 6px; color: #94a3b8; font-weight: 400; }
.profit-cell b { font-size: 24px; letter-spacing: -.03em; }
.bridge-rows { display: grid; gap: 4px; }
.bridge-row { display: flex; justify-content: space-between; padding: 7px 14px; border-radius: 8px; color: #475569; font-size: 13px; }
.bridge-row:nth-child(odd) { background: #f8fafc; }
.bridge-row.muted { color: #94a3b8; }
.ws-warn { margin-top: 10px; color: #d97706; font-size: 11.5px; }

@media (max-width: 900px) {
  .funds-grid { grid-template-columns: 1fr; }
  .finance-header { align-items: flex-start; flex-direction: column; gap: 12px; }
  .header-actions { width: 100%; justify-content: flex-start; }
}

/* 窄屏（<768px）移动端适配：老板手机查钱/核销场景 */
@media (max-width: 768px) {
  /* 页面骨架收窄，杜绝横向溢出 */
  .finance-page { padding: 16px 12px; gap: 12px; }
  /* 顶部三个动作按钮保持可见：纵向排布不折叠进菜单 */
  .header-actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .header-actions :deep(.el-button) { margin-left: 0; }
  .header-actions :deep(.el-dropdown) { flex: 1; }
  .header-actions :deep(.el-dropdown .el-button) { width: 100%; }
  /* 指标卡网格 → 单列堆叠 */
  .profit-grid { grid-template-columns: 1fr; gap: 10px; }
  .profit-cell b { font-size: 21px; }
  /* 钱在哪：可用资金大字与账户列表上下堆叠（funds-grid 已单列），字号收缩 */
  .funds-available { font-size: 28px; }
  /* 待我处理：名称+去处理一行，数量金额第二行，"去处理"不换行不挤压金额 */
  .todo-row { grid-template-columns: minmax(0, 1fr) auto auto; grid-template-areas: "label label go" "count amount go"; row-gap: 3px; padding: 11px 12px; }
  .todo-label { grid-area: label; min-width: 0; }
  .todo-count { grid-area: count; }
  .todo-amount { grid-area: amount; justify-self: end; }
  .todo-go { grid-area: go; align-self: center; white-space: nowrap; }
  /* 概况卡桥接行字号微收 */
  .bridge-row { font-size: 12.5px; }
  /* 最近流水：隐藏次要列（账户/分类/单号），保留时间/客户/金额，防横向滚动 */
  :deep(.el-table .col-account),
  :deep(.el-table .col-category),
  :deep(.el-table .col-related) { display: none; }
  :deep(.el-table) { min-width: 0; }
}
</style>

<style>
/* 流水明细抽屉：窄屏占满屏宽（抽屉宽度是行内样式，需全局覆盖） */
@media (max-width: 768px) {
  .finance-flow-drawer { width: 100% !important; }
}
</style>
