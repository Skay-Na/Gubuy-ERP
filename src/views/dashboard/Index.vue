<script setup>
import { computed, onMounted, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { Refresh, Right, TrendCharts, Coin, Wallet, Money } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { resolveOffboardingAlert } from '../../api/employees'
import { useDashboard } from '../../composables/useDashboard'
import { formatCurrency } from '../../utils/dashboard'
import DashboardImportantReminders from '../../components/dashboard/DashboardImportantReminders.vue'
import DashboardDetails from '../../components/dashboard/DashboardDetails.vue'
import DashboardTrendChart from '../../components/dashboard/DashboardTrendChart.vue'
import DashboardDrillDownDrawer from '../../components/dashboard/DashboardDrillDownDrawer.vue'
import { buildPeriodParams } from '../../components/dashboard/drillColumns'
import { barDateRange } from '../../utils/trendAggregate'

const router = useRouter()
const { summary, prevSummary, totalBalance, loading, error, updatedAt, employeeOffboardingAlerts, employeeOffboardingAlertsError, employeeOffboardingAlertsLoading, load, loadEmployeeOffboardingAlerts } = useDashboard()
const period = shallowRef('today')
const customDates = shallowRef([])
const periods = [
  { value: 'today', label: '今日' },
  { value: 'month', label: '本月' },
  { value: 'last_month', label: '上月' },
  { value: 'year', label: '本年' },
  { value: 'custom', label: '自定义' }
]
const periodLabel = computed(() => periods.find(item => item.value === period.value)?.label || '本期')
const updateText = computed(() => updatedAt.value ? `更新于 ${updatedAt.value.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}` : '等待数据')
const orderCount = computed(() => Number(summary.value.order_count || 0))
const revenue = computed(() => Number(summary.value.today_revenue || 0))
const netProfit = computed(() => Number(summary.value.net_profit || 0))
const costIncomplete = computed(() => Boolean(summary.value.cost_incomplete))

// 环比：与上一周期（昨日/上月/去年/等长前移）对比，上一周期数据缺失时不展示
const deltaBaseLabel = computed(() => ({ today: '较昨日', month: '较上月', last_month: '较上上月', year: '较去年', custom: '较上一周期' }[period.value] || '较上一周期'))
const compactMoney = value => `¥${Math.abs(Number(value || 0)).toLocaleString('zh-CN', { maximumFractionDigits: 0 })}`
function buildDelta(current, previous) {
  if (previous === null || previous === undefined) return null
  const diff = Number(current || 0) - Number(previous || 0)
  if (Math.abs(diff) < 0.005) return { cls: 'flat', text: `${deltaBaseLabel.value}持平` }
  const pct = Number(previous) > 0 ? `（${diff > 0 ? '+' : '-'}${Math.abs(diff / Number(previous) * 100).toFixed(1)}%）` : ''
  return {
    cls: diff > 0 ? 'up' : 'down',
    text: `${deltaBaseLabel.value} ${diff > 0 ? '+' : '-'}${compactMoney(diff)} ${pct}`.trim()
  }
}
const revenueDelta = computed(() => prevSummary.value ? buildDelta(revenue.value, prevSummary.value.today_revenue) : null)
const profitDelta = computed(() => (prevSummary.value && !costIncomplete.value) ? buildDelta(netProfit.value, prevSummary.value.net_profit) : null)

const kpis = computed(() => [
  { label: '销售额', value: formatCurrency(revenue.value), delta: revenueDelta.value, note: `${orderCount.value} 单 · 客单 ${formatCurrency(orderCount.value ? revenue.value / orderCount.value : 0)}`, drill: { type: 'revenue' }, fallbackRoute: '/admin/orders', tone: 'primary' },
  { label: '经营净利润', value: costIncomplete.value ? '待核算' : formatCurrency(netProfit.value), delta: profitDelta.value, note: costIncomplete.value ? `${summary.value.uncosted_order_item_count || 0} 条订单明细成本待核，毛利暂不完整` : '点击卡片查看利润计算过程', drill: { type: 'net_profit' }, fallbackRoute: '/admin/finance', tone: 'profit' },
  { label: '待收尾款', value: formatCurrency(summary.value.pending_balance), note: `${summary.value.pending_balance_count || 0} 单未结清 · 点击查看欠款明细`, drill: { type: 'pending_balance' }, fallbackRoute: '/admin/orders?tab=pending_balance', tone: 'balance' },
  { label: '国补待回款', value: formatCurrency(summary.value.pending_subsidy), note: '点击查看核销与回款明细', drill: { type: 'pending_subsidy' }, fallbackRoute: '/admin/subsidy', tone: 'subsidy' }
])

/** KPI 卡图标（按 tone 分配，纯视觉） */
const KPI_ICONS = { primary: TrendCharts, profit: Coin, balance: Wallet, subsidy: Money }

function selectPeriod(value) {
  if (period.value === value) return
  period.value = value
  if (value !== 'custom') {
    customDates.value = []
    refresh()
  }
}

function updateMobileDate(index, value) {
  const dates = [...customDates.value]
  dates[index] = value
  customDates.value = dates
  if (dates[0] && dates[1] && dates[0] <= dates[1]) refresh()
}

function refresh() { load(period.value, customDates.value) }
function navigate(path) { router.push(path) }

// —— 明细钻取抽屉 ——————————————————————————————————————————
// Index 持有抽屉状态；页内所有钻取入口（KPI 卡含利润计算过程/员工行/账户行/趋势柱）统一走 openDrill
const drawerState = shallowRef({ visible: false, type: '', params: {}, label: '', fallbackRoute: '' })

function openDrill(type, params = {}, fallbackRoute = '', label = '') {
  drawerState.value = { visible: true, type, params, label, fallbackRoute }
}
function closeDrill() {
  drawerState.value = { ...drawerState.value, visible: false }
}

/** 带当前看板期间的钻取参数（extra 为类型专属参数） */
function baseDrillParams(extra = {}) {
  return buildPeriodParams(period.value, customDates.value, extra)
}

function onKpiClick(card) {
  if (card.drill) openDrill(card.drill.type, baseDrillParams(card.drill.params), card.fallbackRoute)
  else navigate(card.fallbackRoute)
}

/** 员工行/账户行钻取（Details 发射归属参数，这里合并当前期间） */
function onDetailsDrill({ type, params, label }) {
  openDrill(type, baseDrillParams(params), '', label || '')
}

/** 当前统计期间的日期边界（供趋势柱区间 clamp），无法确定时返回 null */
function currentPeriodBounds() {
  const now = new Date()
  const iso = date => `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
  switch (period.value) {
    case 'today': return { start: iso(now), end: iso(now) }
    case 'month': return { start: iso(new Date(now.getFullYear(), now.getMonth(), 1)), end: iso(now) }
    case 'last_month': return { start: iso(new Date(now.getFullYear(), now.getMonth() - 1, 1)), end: iso(new Date(now.getFullYear(), now.getMonth(), 0)) }
    case 'year': return { start: `${now.getFullYear()}-01-01`, end: `${now.getFullYear()}-12-31` }
    case 'custom': return (customDates.value[0] && customDates.value[1]) ? { start: customDates.value[0], end: customDates.value[1] } : null
    default: return { start: iso(now), end: iso(now) }
  }
}

/** 趋势柱点击：按聚合粒度换算柱覆盖区间（clamp 到当前期间边界），钻取该区间订单明细 */
function onTrendBarClick(item, granularityValue) {
  const range = barDateRange(item, granularityValue, currentPeriodBounds())
  if (!range) return
  openDrill('trend_orders', { period: 'custom', start_date: range.start_date, end_date: range.end_date }, '/admin/orders')
}
function openHistoricalCostRepair() {
  router.push({ path: '/admin/settings', query: { open: 'historical-cost-repair' } })
}
function openEmployeePayroll(item) {
  // 定稿工资变动待办：跳转员工管理的工资条页签，按员工+薪资月定位核对
  router.push({ path: '/admin/employees', query: { tab: 'payroll', employee_id: item.employee_id, payroll_month: item.month } })
}
function alertLabel(alert) {
  return ({
    pending_fulfillment: '待履约',
    order_cancelled: '离职后取消',
    order_reversed: '离职后红冲',
    order_refund: '离职后退款'
  })[alert.alert_type] || '订单变更'
}
function viewAlertOrder(alert) {
  router.push({ name: 'OrderManage', query: { order_no: alert.order_no } })
}
async function resolveAlert(alert) {
  try {
    const { value } = await ElMessageBox.prompt('请填写复核结论。若需调整离职结算，请另行登记结算调整。', `复核 ${alert.order_no}`, {
      confirmButtonText: '保存复核', cancelButtonText: '取消', inputType: 'textarea',
      inputValidator: value => Boolean(String(value || '').trim()) || '请填写复核说明'
    })
    const response = await resolveOffboardingAlert(alert.id, { resolution: value })
    if (response.data.code !== 200) throw new Error(response.data.msg || '告警处理失败')
    ElMessage.success('复核记录已保存')
    await loadEmployeeOffboardingAlerts()
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error(error.response?.data?.msg || error.message || '告警处理失败')
  }
}
onMounted(refresh)
</script>

<template>
  <div class="dashboard">
    <header class="page-header">
      <div class="page-heading">
        <span class="eyebrow">BUSINESS OVERVIEW</span>
        <h1>经营看板</h1>
        <p>经营数据概览。{{ updateText }}</p>
      </div>
      <div class="controls">
        <div class="periods" aria-label="统计时间">
          <button v-for="item in periods" :key="item.value" :class="{ active: period === item.value }" :aria-pressed="period === item.value" @click="selectPeriod(item.value)">{{ item.label }}</button>
        </div>
        <button class="refresh" :disabled="loading" aria-label="刷新看板" @click="refresh"><el-icon :class="{ spinning: loading }"><Refresh /></el-icon><span>刷新</span></button>
      </div>
    </header>

    <div v-if="period === 'custom'" class="date-range">
      <el-date-picker v-model="customDates" class="desktop-date" type="daterange" value-format="YYYY-MM-DD" range-separator="至" start-placeholder="开始日期" end-placeholder="结束日期" :clearable="false" @change="refresh" />
      <div class="mobile-dates">
        <input type="date" aria-label="开始日期" :value="customDates[0] || ''" @change="updateMobileDate(0, $event.target.value)" />
        <span>至</span>
        <input type="date" aria-label="结束日期" :value="customDates[1] || ''" @change="updateMobileDate(1, $event.target.value)" />
      </div>
      <span v-if="!customDates[0] || !customDates[1]" class="date-help">请选择完整日期范围</span>
    </div>

    <div v-if="error" class="error-banner" role="alert">{{ error }} <button @click="refresh">重试</button></div>

    <section v-if="employeeOffboardingAlertsLoading || employeeOffboardingAlertsError || employeeOffboardingAlerts.length" class="offboarding-alert-panel" aria-label="员工离职后订单提醒">
      <div class="offboarding-alert-heading"><div><span class="alert-eyebrow">HR / ORDER FOLLOW-UP</span><h2>员工离职交接提醒</h2></div><span class="alert-count">{{ employeeOffboardingAlerts.length }} 条待处理</span></div>
      <div v-if="employeeOffboardingAlertsError" class="alert-error">{{ employeeOffboardingAlertsError }} <button @click="loadEmployeeOffboardingAlerts">重试</button></div>
      <div v-else-if="employeeOffboardingAlertsLoading && !employeeOffboardingAlerts.length" class="alert-loading">正在加载离职订单提醒…</div>
      <div v-else class="offboarding-alert-list">
        <article v-for="alert in employeeOffboardingAlerts" :key="alert.id" class="offboarding-alert-row">
          <div class="offboarding-alert-main"><el-tag type="danger" effect="light" size="small">{{ alertLabel(alert) }}</el-tag><strong>{{ alert.employee_name }}</strong><span>订单 {{ alert.order_no }}</span><small>{{ alert.message }}</small></div>
          <div class="offboarding-alert-money"><span>涉及提成 <strong>{{ formatCurrency(alert.commission_amount) }}</strong></span><span v-if="alert.refund_amount">退款 <strong>{{ formatCurrency(alert.refund_amount) }}</strong></span></div>
          <div class="offboarding-alert-actions"><button @click="viewAlertOrder(alert)">查看订单</button><button v-if="alert.alert_type !== 'pending_fulfillment'" class="resolve-alert" @click="resolveAlert(alert)">记录复核</button></div>
        </article>
      </div>
    </section>

    <section class="kpi-grid" :aria-label="`${periodLabel}经营指标`">
      <button v-for="card in kpis" :key="card.label" class="kpi" :class="card.tone" @click="onKpiClick(card)">
        <span class="kpi-head">
          <span class="kpi-badge"><el-icon><component :is="KPI_ICONS[card.tone]" /></el-icon></span>
          <span class="kpi-label">{{ card.label }} <el-icon><Right /></el-icon></span>
        </span>
        <strong>{{ card.value }}</strong>
        <span v-if="card.delta" class="kpi-delta" :class="card.delta.cls">{{ card.delta.text }}</span>
        <span class="kpi-note">{{ card.note }}</span>
      </button>
    </section>

    <DashboardTrendChart :trend="summary.sales_trend || []" :title="`${periodLabel}销售趋势`" class="trend-section" @bar-click="onTrendBarClick" />

    <DashboardImportantReminders
      :zero-cost-sale-count="summary.pending_zero_cost_sale_count || 0"
      :payroll-mismatches="summary.pending_payroll_mismatches || []"
      class="important-reminders"
      @open-cost-repair="openHistoricalCostRepair"
      @view-payroll="openEmployeePayroll"
    />

    <DashboardDetails :summary="summary" :total-balance="totalBalance" @navigate="navigate" @drill="onDetailsDrill" />

    <DashboardDrillDownDrawer
      :visible="drawerState.visible"
      :type="drawerState.type"
      :params="drawerState.params"
      :label="drawerState.label"
      :fallback-route="drawerState.fallbackRoute"
      @close="closeDrill"
    />
  </div>
</template>

<style scoped>
.dashboard{padding:0 0 30px;color:#263449}.page-header{display:flex;justify-content:space-between;align-items:end;gap:24px;margin:3px 0 22px}.eyebrow{font-size:10px;font-weight:800;letter-spacing:.14em;color:#8898aa}.page-heading h1{margin:5px 0 3px;color:#172033;font-size:28px;font-weight:760;letter-spacing:-.04em}.page-heading p{margin:0;color:#8390a2;font-size:12px}.controls{display:flex;align-items:center;gap:10px}.periods{display:flex;gap:3px;padding:4px;background:#eef2f7;border-radius:10px}.periods button{border:0;border-radius:8px;padding:8px 12px;background:transparent;color:#66758a;font-size:12px;cursor:pointer;white-space:nowrap}.periods button.active{background:#fff;color:#635bff;box-shadow:0 1px 3px rgba(10,37,64,.1);font-weight:700}.refresh{display:flex;align-items:center;gap:5px;height:37px;padding:0 12px;background:#fff;border:1px solid #e3e8ee;border-radius:10px;color:#425466;font-size:12px;cursor:pointer;box-shadow:0 1px 2px rgba(10,37,64,.04)}.refresh:disabled{opacity:.6;cursor:wait}.spinning{animation:spin 1s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}.date-range{display:flex;align-items:center;gap:12px;margin:-6px 0 18px}.desktop-date{max-width:290px}.mobile-dates{display:none}.date-help{font-size:12px;color:#8793a5}.error-banner{display:flex;justify-content:space-between;align-items:center;padding:10px 14px;margin-bottom:16px;border:1px solid #f5c7c7;background:#fff4f3;border-radius:10px;color:#bd4141;font-size:13px}.error-banner button{border:0;background:none;color:#b02f2f;font-weight:700;cursor:pointer}.kpi-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:14px;margin-bottom:16px}.kpi{min-width:0;min-height:150px;display:flex;flex-direction:column;align-items:flex-start;gap:10px;text-align:left;padding:18px 20px;background:#fff;border:1px solid #e3e8ee;border-radius:12px;cursor:pointer;transition:transform .15s,border-color .15s,box-shadow .15s;box-shadow:0 1px 3px rgba(10,37,64,.06)}.kpi:hover{transform:translateY(-2px);border-color:#cfd7df;box-shadow:0 4px 12px rgba(10,37,64,.09)}.kpi-head{display:flex;align-items:center;gap:10px;width:100%}.kpi-badge{display:inline-flex;align-items:center;justify-content:center;flex:none;width:36px;height:36px;border-radius:10px;font-size:18px}.kpi .kpi-badge{background:#e8e9ff;color:#635bff}.kpi-label{flex:1;display:flex;justify-content:space-between;align-items:center;min-width:0;font-size:12px;font-weight:600;color:#64748b}.kpi-label .el-icon{opacity:.55}.kpi strong{display:block;flex:none;margin:0;color:#172033;font-size:26px;line-height:1.1;letter-spacing:-.04em;white-space:nowrap;font-variant-numeric:tabular-nums}.kpi-note{margin-top:auto;width:100%;color:#8a94a6;font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
@media(max-width:1100px){.page-header{align-items:flex-start;flex-direction:column}.kpi-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:767px){.dashboard{padding:3px 0 24px}.page-header{gap:15px;margin:0 0 16px}.page-heading h1{font-size:24px}.controls{width:100%}.periods{flex:1;min-width:0;overflow-x:auto}.periods button{padding:8px 10px;flex:1}.refresh{flex:none;padding:0 10px}.refresh span{display:none}.desktop-date{display:none}.mobile-dates{display:flex;align-items:center;gap:7px;width:100%}.mobile-dates input{min-width:0;flex:1;box-sizing:border-box;padding:8px 5px;border:1px solid #dce4ed;border-radius:8px;background:#fff;color:#435268;font-size:12px}.date-range{flex-wrap:wrap}.kpi-grid{gap:10px;margin-bottom:12px}.kpi{min-height:126px;padding:14px;border-radius:12px;gap:8px}.kpi-head{gap:8px}.kpi-badge{width:32px;height:32px;border-radius:9px;font-size:16px}.kpi strong{font-size:19px}.kpi-note{font-size:10px}.kpi-delta{height:18px;padding:0 7px;font-size:9px}}
/* 环比胶囊：固定高度，间距交给 .kpi 的 flex gap，不再用 margin 叠加 */
.kpi-delta{display:inline-flex;align-items:center;flex:none;height:21px;padding:0 9px;border-radius:999px;font-size:11px;font-weight:700;line-height:1.3}
.kpi-delta.up{background:#e8f7ee;color:#15803d}
.kpi-delta.down{background:#fdeaea;color:#dc2626}
.kpi-delta.flat{background:#eef2f7;color:#66758a}
/* 国补待回款卡：与其他卡同为白卡，仅数字用品牌紫作重点色（不整卡变色） */
.kpi.subsidy{background:#fff;border-color:#e3e8ee}
.kpi.subsidy:hover{border-color:#cfd7df}
.kpi.subsidy strong{color:#635bff}
</style>

<style scoped>
.offboarding-alert-panel{margin-bottom:16px;padding:18px 20px;border:1px solid #f4c7c3;border-radius:12px;background:#fff;box-shadow:0 1px 3px rgba(10,37,64,.06)}.offboarding-alert-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:10px}.alert-eyebrow{font-size:9px;font-weight:800;letter-spacing:.13em;color:#bf6657}.offboarding-alert-heading h2{margin:4px 0 0;color:#8f3026;font-size:16px}.alert-count{padding:6px 9px;border-radius:999px;background:#fde5e1;color:#a83b31;font-size:11px;font-weight:700}.offboarding-alert-list{display:grid;gap:8px}.offboarding-alert-row{display:grid;grid-template-columns:minmax(0,1fr) auto auto;align-items:center;gap:16px;padding:11px 12px;border:1px solid #f2dfdc;border-radius:10px;background:#fbfcfe}.offboarding-alert-main{display:flex;align-items:center;gap:9px;min-width:0;flex-wrap:wrap}.offboarding-alert-main strong{color:#27354a;font-size:12px}.offboarding-alert-main>span{color:#607088;font-size:11px}.offboarding-alert-main small{flex-basis:100%;padding-left:2px;color:#8390a2;font-size:11px;white-space:normal}.offboarding-alert-money{display:flex;flex-direction:column;gap:3px;color:#8290a2;font-size:10px;white-space:nowrap}.offboarding-alert-money strong{color:#b44237}.offboarding-alert-actions{display:flex;gap:5px}.offboarding-alert-actions button{padding:7px 9px;border:1px solid #e3e9f0;border-radius:8px;background:white;color:#53657b;font-size:10px;font-weight:700;cursor:pointer}.offboarding-alert-actions .resolve-alert{border-color:#edc5bf;background:#fff6f4;color:#a63d32}.alert-error,.alert-loading{padding:9px 0;color:#a63d32;font-size:12px}.alert-error button{margin-left:8px;border:0;background:none;color:#a63d32;font-weight:700;cursor:pointer}
@media(max-width:767px){.offboarding-alert-panel{padding:14px}.offboarding-alert-row{grid-template-columns:1fr;gap:9px}.offboarding-alert-actions button{flex:1}.offboarding-alert-money{flex-direction:row;gap:12px}}
</style>
