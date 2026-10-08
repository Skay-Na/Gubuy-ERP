<script setup>
import { computed, onMounted, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { Refresh, Right } from '@element-plus/icons-vue'
import { useDashboard } from '../../composables/useDashboard'
import { formatCurrency } from '../../utils/dashboard'
import DashboardTrend from '../../components/dashboard/DashboardTrend.vue'
import DashboardDetails from '../../components/dashboard/DashboardDetails.vue'

const router = useRouter()
const { summary, todos, productInsights, totalBalance, loading, error, updatedAt, load } = useDashboard()
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
const pendingItems = computed(() => todos.value.items.filter(item => item.count > 0 || item.amount > 0))
const updateText = computed(() => updatedAt.value ? `更新于 ${updatedAt.value.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}` : '等待数据')
const orderCount = computed(() => Number(summary.value.order_count || 0))
const revenue = computed(() => Number(summary.value.today_revenue || 0))
const netProfit = computed(() => Number(summary.value.net_profit || 0))
const kpis = computed(() => [
  { label: '销售额', value: formatCurrency(revenue.value), note: `${orderCount.value} 单 · 客单 ${formatCurrency(orderCount.value ? revenue.value / orderCount.value : 0)}`, route: '/admin/orders', tone: 'primary' },
  { label: '经营净利润', value: formatCurrency(netProfit.value), note: `毛利 ${formatCurrency(summary.value.gross_profit)} · 费用 ${formatCurrency(summary.value.operating_expense)}`, route: '/admin/finance', tone: 'profit' },
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
onMounted(refresh)
</script>

<template>
  <div class="dashboard">
    <header class="page-header">
      <div class="page-heading">
        <span class="eyebrow">BUSINESS OVERVIEW</span>
        <h1>经营看板</h1>
        <p>先看经营结果，再处理待办。{{ updateText }}</p>
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

    <section class="kpi-grid" :aria-label="`${periodLabel}经营指标`">
      <button v-for="card in kpis" :key="card.label" class="kpi" :class="card.tone" @click="navigate(card.route)">
        <span class="kpi-label">{{ card.label }} <el-icon><Right /></el-icon></span>
        <strong>{{ card.value }}</strong>
        <span class="kpi-note">{{ card.note }}</span>
      </button>
    </section>

    <div class="main-grid">
      <section class="panel tasks" aria-labelledby="tasks-title">
        <div class="panel-heading">
          <div><span class="eyebrow">ACTION CENTER</span><h2 id="tasks-title">待处理事项</h2></div>
          <span class="task-total">{{ todos.totalCount }} 项</span>
        </div>
        <div v-if="pendingItems.length" class="task-list">
          <button v-for="item in pendingItems" :key="item.key" class="task-row" @click="navigate(item.route)">
            <span class="priority" :class="item.level" />
            <span class="task-title">{{ item.title }}<small v-if="item.amount > 0">{{ formatCurrency(item.amount) }}</small></span>
            <span class="task-count">{{ item.count }} 项</span>
            <el-icon><Right /></el-icon>
          </button>
        </div>
        <div v-else class="empty-tasks"><span class="checkmark">✓</span><strong>当前事项已处理完</strong><small>有新事项时会显示在这里</small></div>
      </section>
      <DashboardTrend :trend="summary.sales_trend || []" :period="period" />
    </div>

    <DashboardDetails :summary="summary" :insights="productInsights" :total-balance="totalBalance" @navigate="navigate" />
  </div>
</template>

<style scoped>
.dashboard{padding:0 0 30px;color:#263449}.page-header{display:flex;justify-content:space-between;align-items:end;gap:24px;margin:3px 0 22px}.eyebrow{font-size:10px;font-weight:800;letter-spacing:.14em;color:#94a3b8}.page-heading h1{margin:5px 0 3px;color:#172033;font-size:28px;font-weight:760;letter-spacing:-.04em}.page-heading p{margin:0;color:#8390a2;font-size:12px}.controls{display:flex;align-items:center;gap:10px}.periods{display:flex;gap:3px;padding:4px;background:#eef2f7;border-radius:11px}.periods button{border:0;border-radius:8px;padding:8px 12px;background:transparent;color:#66758a;font-size:12px;cursor:pointer;white-space:nowrap}.periods button.active{background:#fff;color:#1d4ed8;box-shadow:0 1px 4px #1e293b14;font-weight:700}.refresh{display:flex;align-items:center;gap:5px;height:37px;padding:0 12px;background:#fff;border:1px solid #e1e7ef;border-radius:10px;color:#57667a;font-size:12px;cursor:pointer}.refresh:disabled{opacity:.6;cursor:wait}.spinning{animation:spin 1s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}.date-range{display:flex;align-items:center;gap:12px;margin:-6px 0 18px}.desktop-date{max-width:290px}.mobile-dates{display:none}.date-help{font-size:12px;color:#8793a5}.error-banner{display:flex;justify-content:space-between;align-items:center;padding:10px 14px;margin-bottom:16px;border:1px solid #f5c7c7;background:#fff4f3;border-radius:10px;color:#bd4141;font-size:13px}.error-banner button{border:0;background:none;color:#b02f2f;font-weight:700;cursor:pointer}.kpi-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:14px;margin-bottom:16px}.kpi{min-width:0;min-height:142px;display:flex;flex-direction:column;align-items:flex-start;text-align:left;padding:19px 20px;background:#fff;border:1px solid #e8edf3;border-radius:17px;cursor:pointer;transition:transform .15s,border-color .15s}.kpi:hover{transform:translateY(-2px);border-color:#bdcdeb}.kpi.primary{background:#2359cf;border-color:#2359cf;color:#fff}.kpi-label{display:flex;justify-content:space-between;align-items:center;width:100%;font-size:12px;color:#748298}.kpi.primary .kpi-label,.kpi.primary .kpi-note{color:#dbe7ff}.kpi-label .el-icon{opacity:.65}.kpi strong{display:block;margin:19px 0 5px;color:#1a273b;font-size:25px;line-height:1.1;letter-spacing:-.04em;white-space:nowrap}.kpi.primary strong{color:#fff}.kpi-note{color:#96a1af;font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:100%}.kpi.profit strong{color:#138f74}.kpi.balance strong{color:#d16447}.kpi.subsidy strong{color:#ad771c}.main-grid{display:grid;grid-template-columns:minmax(320px,.86fr) minmax(0,1.14fr);gap:16px;margin-bottom:16px}.panel{min-width:0;background:#fff;border:1px solid #e8edf3;border-radius:18px;padding:22px}.panel-heading{display:flex;justify-content:space-between;align-items:flex-end;gap:12px}h2{margin:5px 0 0;color:#172033;font-size:18px;font-weight:750}.task-total{padding:5px 9px;border-radius:8px;background:#edf3ff;color:#315fbd;font-size:12px;font-weight:750}.task-list{margin-top:16px}.task-row{display:flex;align-items:center;gap:11px;width:100%;min-height:49px;padding:8px 0;background:none;border:0;border-bottom:1px solid #f0f3f7;text-align:left;cursor:pointer}.task-row:last-child{border-bottom:0}.priority{width:7px;height:7px;flex:none;border-radius:50%;background:#b8c4d1}.priority.high{background:#e26755}.priority.medium{background:#e5aa3f}.task-title{flex:1;min-width:0;color:#263449;font-size:13px;font-weight:630;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.task-title small{display:block;margin-top:2px;color:#9ba7b5;font-size:11px;font-weight:400}.task-count{color:#75849a;font-size:12px;white-space:nowrap}.task-row .el-icon{font-size:12px;color:#adbac9}.empty-tasks{min-height:264px;display:flex;flex-direction:column;justify-content:center;align-items:center;gap:7px;color:#52657c}.empty-tasks small{color:#a0adba}.checkmark{display:grid;place-items:center;width:36px;height:36px;border-radius:50%;background:#e6f6ef;color:#159b71;font-size:20px}
@media(max-width:1100px){.page-header{align-items:flex-start;flex-direction:column}.kpi-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.main-grid{grid-template-columns:1fr}}
@media(max-width:767px){.dashboard{padding:3px 0 24px}.page-header{gap:15px;margin:0 0 16px}.page-heading h1{font-size:24px}.controls{width:100%}.periods{flex:1;min-width:0;overflow-x:auto}.periods button{padding:8px 10px;flex:1}.refresh{flex:none;padding:0 10px}.refresh span{display:none}.desktop-date{display:none}.mobile-dates{display:flex;align-items:center;gap:7px;width:100%}.mobile-dates input{min-width:0;flex:1;box-sizing:border-box;padding:8px 5px;border:1px solid #dce4ed;border-radius:8px;background:#fff;color:#435268;font-size:12px}.date-range{flex-wrap:wrap}.kpi-grid{gap:10px;margin-bottom:12px}.kpi{min-height:121px;padding:14px;border-radius:14px}.kpi strong{font-size:19px;margin:16px 0 6px}.kpi-note{font-size:10px}.main-grid{gap:12px;margin-bottom:12px}.panel{padding:18px}.task-row{min-height:48px}}
</style>
