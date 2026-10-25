<script setup>
import { computed, onMounted, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { Refresh, Right } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { resolveOffboardingAlert } from '../../api/employees'
import { useDashboard } from '../../composables/useDashboard'
import { formatCurrency } from '../../utils/dashboard'
import DashboardImportantReminders from '../../components/dashboard/DashboardImportantReminders.vue'
import DashboardDetails from '../../components/dashboard/DashboardDetails.vue'

const router = useRouter()
const { summary, totalBalance, loading, error, updatedAt, employeeOffboardingAlerts, employeeOffboardingAlertsError, employeeOffboardingAlertsLoading, load, loadEmployeeOffboardingAlerts } = useDashboard()
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
const kpis = computed(() => [
  { label: '销售额', value: formatCurrency(revenue.value), note: `${orderCount.value} 单 · 客单 ${formatCurrency(orderCount.value ? revenue.value / orderCount.value : 0)}`, route: '/admin/orders', tone: 'primary' },
  { label: '经营净利润', value: costIncomplete.value ? '待核算' : formatCurrency(netProfit.value), note: costIncomplete.value ? `${summary.value.uncosted_order_item_count || 0} 条订单明细成本待核，毛利暂不完整` : `毛利 ${formatCurrency(summary.value.gross_profit)} · 费用 ${formatCurrency(summary.value.operating_expense)}`, route: '/admin/finance', tone: 'profit' },
  { label: '待收尾款', value: formatCurrency(summary.value.pending_balance), note: `${summary.value.pending_balance_count || 0} 单未结清 · 实时存量`, route: '/admin/orders', tone: 'balance' },
  { label: '国补待回款', value: formatCurrency(summary.value.pending_subsidy), note: '实时存量', route: '/admin/subsidy', tone: 'subsidy' }
])

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
function openHistoricalCostRepair() {
  router.push({ path: '/admin/settings', query: { open: 'historical-cost-repair' } })
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
      <button v-for="card in kpis" :key="card.label" class="kpi" :class="card.tone" @click="navigate(card.route)">
        <span class="kpi-label">{{ card.label }} <el-icon><Right /></el-icon></span>
        <strong>{{ card.value }}</strong>
        <span class="kpi-note">{{ card.note }}</span>
      </button>
    </section>

    <DashboardImportantReminders
      :zero-cost-sale-count="summary.pending_zero_cost_sale_count || 0"
      class="important-reminders"
      @open-cost-repair="openHistoricalCostRepair"
    />

    <DashboardDetails :summary="summary" :total-balance="totalBalance" @navigate="navigate" />
  </div>
</template>

<style scoped>
.dashboard{padding:0 0 30px;color:#263449}.page-header{display:flex;justify-content:space-between;align-items:end;gap:24px;margin:3px 0 22px}.eyebrow{font-size:10px;font-weight:800;letter-spacing:.14em;color:#94a3b8}.page-heading h1{margin:5px 0 3px;color:#172033;font-size:28px;font-weight:760;letter-spacing:-.04em}.page-heading p{margin:0;color:#8390a2;font-size:12px}.controls{display:flex;align-items:center;gap:10px}.periods{display:flex;gap:3px;padding:4px;background:#eef2f7;border-radius:11px}.periods button{border:0;border-radius:8px;padding:8px 12px;background:transparent;color:#66758a;font-size:12px;cursor:pointer;white-space:nowrap}.periods button.active{background:#fff;color:#1d4ed8;box-shadow:0 1px 4px #1e293b14;font-weight:700}.refresh{display:flex;align-items:center;gap:5px;height:37px;padding:0 12px;background:#fff;border:1px solid #e1e7ef;border-radius:10px;color:#57667a;font-size:12px;cursor:pointer}.refresh:disabled{opacity:.6;cursor:wait}.spinning{animation:spin 1s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}.date-range{display:flex;align-items:center;gap:12px;margin:-6px 0 18px}.desktop-date{max-width:290px}.mobile-dates{display:none}.date-help{font-size:12px;color:#8793a5}.error-banner{display:flex;justify-content:space-between;align-items:center;padding:10px 14px;margin-bottom:16px;border:1px solid #f5c7c7;background:#fff4f3;border-radius:10px;color:#bd4141;font-size:13px}.error-banner button{border:0;background:none;color:#b02f2f;font-weight:700;cursor:pointer}.kpi-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:14px;margin-bottom:16px}.kpi{min-width:0;min-height:142px;display:flex;flex-direction:column;align-items:flex-start;text-align:left;padding:19px 20px;background:#fff;border:1px solid #e8edf3;border-radius:17px;cursor:pointer;transition:transform .15s,border-color .15s}.kpi:hover{transform:translateY(-2px);border-color:#bdcdeb}.kpi.primary{background:#2359cf;border-color:#2359cf;color:#fff}.kpi-label{display:flex;justify-content:space-between;align-items:center;width:100%;font-size:12px;color:#748298}.kpi.primary .kpi-label,.kpi.primary .kpi-note{color:#dbe7ff}.kpi-label .el-icon{opacity:.65}.kpi strong{display:block;margin:19px 0 5px;color:#1a273b;font-size:25px;line-height:1.1;letter-spacing:-.04em;white-space:nowrap}.kpi.primary strong{color:#fff}.kpi-note{color:#96a1af;font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:100%}.kpi.profit strong{color:#138f74}.kpi.balance strong{color:#d16447}.kpi.subsidy strong{color:#ad771c}.panel{min-width:0;background:#fff;border:1px solid #e8edf3;border-radius:18px;padding:22px}.panel-heading{display:flex;justify-content:space-between;align-items:flex-end;gap:12px}h2{margin:5px 0 0;color:#172033;font-size:18px;font-weight:750}
@media(max-width:1100px){.page-header{align-items:flex-start;flex-direction:column}.kpi-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:767px){.dashboard{padding:3px 0 24px}.page-header{gap:15px;margin:0 0 16px}.page-heading h1{font-size:24px}.controls{width:100%}.periods{flex:1;min-width:0;overflow-x:auto}.periods button{padding:8px 10px;flex:1}.refresh{flex:none;padding:0 10px}.refresh span{display:none}.desktop-date{display:none}.mobile-dates{display:flex;align-items:center;gap:7px;width:100%}.mobile-dates input{min-width:0;flex:1;box-sizing:border-box;padding:8px 5px;border:1px solid #dce4ed;border-radius:8px;background:#fff;color:#435268;font-size:12px}.date-range{flex-wrap:wrap}.kpi-grid{gap:10px;margin-bottom:12px}.kpi{min-height:121px;padding:14px;border-radius:14px}.kpi strong{font-size:19px;margin:16px 0 6px}.kpi-note{font-size:10px}}
</style>

<style scoped>
.offboarding-alert-panel{margin-bottom:16px;padding:18px 20px;border:1px solid #f0c5c1;border-radius:16px;background:linear-gradient(135deg,#fff8f6,#fff 62%);box-shadow:0 5px 18px #b94b3510}.offboarding-alert-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:10px}.alert-eyebrow{font-size:9px;font-weight:800;letter-spacing:.13em;color:#bf6657}.offboarding-alert-heading h2{margin:4px 0 0;color:#8f3026;font-size:16px}.alert-count{padding:6px 9px;border-radius:999px;background:#fde5e1;color:#a83b31;font-size:11px;font-weight:700}.offboarding-alert-list{display:grid;gap:8px}.offboarding-alert-row{display:grid;grid-template-columns:minmax(0,1fr) auto auto;align-items:center;gap:16px;padding:11px 12px;border:1px solid #f2dfdc;border-radius:11px;background:#fff}.offboarding-alert-main{display:flex;align-items:center;gap:9px;min-width:0;flex-wrap:wrap}.offboarding-alert-main strong{color:#27354a;font-size:12px}.offboarding-alert-main>span{color:#607088;font-size:11px}.offboarding-alert-main small{flex-basis:100%;padding-left:2px;color:#8390a2;font-size:11px;white-space:normal}.offboarding-alert-money{display:flex;flex-direction:column;gap:3px;color:#8290a2;font-size:10px;white-space:nowrap}.offboarding-alert-money strong{color:#b44237}.offboarding-alert-actions{display:flex;gap:5px}.offboarding-alert-actions button{padding:7px 9px;border:1px solid #e3e9f0;border-radius:8px;background:white;color:#53657b;font-size:10px;font-weight:700;cursor:pointer}.offboarding-alert-actions .resolve-alert{border-color:#edc5bf;background:#fff6f4;color:#a63d32}.alert-error,.alert-loading{padding:9px 0;color:#a63d32;font-size:12px}.alert-error button{margin-left:8px;border:0;background:none;color:#a63d32;font-weight:700;cursor:pointer}
@media(max-width:767px){.offboarding-alert-panel{padding:14px}.offboarding-alert-row{grid-template-columns:1fr;gap:9px}.offboarding-alert-actions button{flex:1}.offboarding-alert-money{flex-direction:row;gap:12px}}
</style>
