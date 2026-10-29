<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ArrowRight, Management, Plus, Switch } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import * as echarts from 'echarts/core'
import { BarChart, HeatmapChart, PieChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, VisualMapComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([BarChart, HeatmapChart, PieChart, GridComponent, TooltipComponent, VisualMapComponent, CanvasRenderer])
import {
  fetchFinanceWorkspace,
  fetchFinanceLogs,
  fetchFinanceSalesAnalysis,
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

// ---------- 销售分析（员工/品类/商品/支付方式） ----------
const sales = ref(null)
const salesLoading = ref(false)
const salesMetric = ref('amount') // 交叉表展示口径：amount 金额 | qty 数量
const salesMonth = ref(currentMonth()) // 销售分析独立月份选择，可看任意月
const salesCumulative = ref(false) // 累计模式
const salesFilterEmp = ref(null) // 员工筛选：employee_id 或 'none'(未分配)
const salesFilterCat = ref(null) // 品类筛选：category_id 或 'none'(未分类)

const fetchSalesAnalysis = async () => {
  salesLoading.value = true
  try {
    const month = salesCumulative.value ? 'cumulative' : salesMonth.value
    const res = await fetchFinanceSalesAnalysis({ month })
    if (res.data.code === 200) {
      sales.value = res.data.data
    }
  } catch {
    // 静默失败
  } finally {
    salesLoading.value = false
  }
}
watch([salesMonth, salesCumulative], fetchSalesAnalysis)

const paymentMethodNames = { 1: '支付宝', 2: '微信', 3: '公户', 4: '混合支付', 5: '现金' }

// ---------- 销售分析筛选（点员工/品类行 → 联动交叉表与热销商品） ----------
const empKeyOf = row => row.employee_id ?? 'none'
const catKeyOf = row => row.category_id ?? 'none'
const empNameByKey = key => (sales.value?.employees || []).find(e => empKeyOf(e) === key)?.employee_name || '未分配'
const catNameByKey = key => (sales.value?.categories || []).find(c => catKeyOf(c) === key)?.category_name || '未分类'
const hasSalesFilter = computed(() => salesFilterEmp.value !== null || salesFilterCat.value !== null)
const clearSalesFilters = () => { salesFilterEmp.value = null; salesFilterCat.value = null }

// 下拉筛选（与图表点击联动，'' = 全部；显式提供"全部"选项保证随时能切回）
const empFilterModel = computed({
  get: () => (salesFilterEmp.value === null ? '' : salesFilterEmp.value),
  set: v => { salesFilterEmp.value = (v === '' || v === undefined || v === null) ? null : v }
})
const catFilterModel = computed({
  get: () => (salesFilterCat.value === null ? '' : salesFilterCat.value),
  set: v => { salesFilterCat.value = (v === '' || v === undefined || v === null) ? null : v }
})
const empOptions = computed(() => (sales.value?.employees || []).map(e => ({ value: empKeyOf(e), label: e.employee_name })))

// 老板客（BOSS001）业绩独立展示
const bossRow = computed(() => sales.value?.boss || null)
const catOptions = computed(() => (sales.value?.categories || []).map(c => ({ value: catKeyOf(c), label: c.category_name })))

// 环比涨幅（累计模式无对比）
const growthOf = (current, prev) => {
  if (prev === null || prev === undefined || !prev) return null
  const pct = ((current - prev) / Math.abs(prev)) * 100
  return { pct, up: pct >= 0 }
}

const salesSummary = computed(() => sales.value?.summary || {})
const salesGrowth = computed(() => ({
  sales: growthOf(salesSummary.value.sales_amount, salesSummary.value.prev_sales_amount),
  orders: growthOf(salesSummary.value.order_count, salesSummary.value.prev_order_count),
  profit: growthOf(salesSummary.value.gross_profit, salesSummary.value.prev_gross_profit)
}))

// ---------- 图表（员工条形图 / 品类占比环图 / 员工×品类热力图，均可点击联动筛选） ----------
const shortMoney = v => (Math.abs(v) >= 10000 ? `${(v / 10000).toFixed(1)}万` : `${Math.round(v)}`)
const empChartRef = ref(null)
const catChartRef = ref(null)
const heatChartRef = ref(null)
let empChart = null
let catChart = null
let heatChart = null

const filteredEmployees = computed(() =>
  salesFilterEmp.value !== null ? (sales.value?.employees || []).filter(e => empKeyOf(e) === salesFilterEmp.value) : (sales.value?.employees || [])
)
const filteredCategories = computed(() =>
  salesFilterCat.value !== null ? (sales.value?.categories || []).filter(c => catKeyOf(c) === salesFilterCat.value) : (sales.value?.categories || [])
)
const hasHeatData = computed(() => (sales.value?.employee_category || []).length > 0)

const employeeChartOption = computed(() => {
  const list = filteredEmployees.value
  return {
    grid: { left: 8, right: 60, top: 10, bottom: 10, containLabel: true },
    tooltip: {
      trigger: 'item',
      confine: true,
      formatter: p => {
        const row = list.find(e => e.employee_name === p.name)
        if (!row) return p.name
        return `<b>${row.employee_name}</b><br/>销售额 ¥${formatMoney(row.sales_amount)}<br/>订单 ${row.order_count} 单 · 提成 ¥${formatMoney(row.commission)}<br/>商品毛利 ¥${formatMoney(row.gross_profit)}<br/><span style="color:#94a3b8">售价−进价，未扣安装与提成 · 点击条形可筛选</span>`
      }
    },
    xAxis: { type: 'value', axisLabel: { color: '#94a3b8', formatter: v => '¥' + shortMoney(v) }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
    yAxis: {
      type: 'category',
      inverse: true,
      data: list.map(e => e.employee_name),
      axisLabel: { color: '#334155', fontWeight: 600 },
      axisTick: { show: false },
      axisLine: { show: false }
    },
    series: [{
      name: '销售额',
      type: 'bar',
      barWidth: 16,
      data: list.map(e => ({
        value: Number(e.sales_amount || 0),
        itemStyle: { color: salesFilterEmp.value !== null && salesFilterEmp.value === empKeyOf(e) ? '#4b44c8' : '#7a70ff', borderRadius: [0, 8, 8, 0] }
      })),
      label: { show: true, position: 'right', color: '#0f172a', fontWeight: 700, formatter: p => '¥' + shortMoney(p.value) }
    }]
  }
})

const categoryChartOption = computed(() => {
  const list = filteredCategories.value
  return {
    tooltip: {
      trigger: 'item',
      confine: true,
      formatter: p => {
        const row = list.find(c => c.category_name === p.name)
        if (!row) return p.name
        return `<b>${p.name}</b><br/>¥${formatMoney(row.sales_amount)} · 占比 ${p.percent}%<br/>${row.qty} 件 · 商品毛利 ¥${formatMoney(row.gross_profit)}<br/><span style="color:#94a3b8">售价−进价，未扣安装与提成 · 点击色块可筛选</span>`
      }
    },
    legend: { bottom: 0, type: 'scroll', textStyle: { color: '#475569', fontSize: 11 }, itemWidth: 10, itemHeight: 10, pageIconSize: 10 },
    series: [{
      name: '品类销售额',
      type: 'pie',
      radius: ['46%', '70%'],
      center: ['50%', '42%'],
      data: list.map(c => ({ name: c.category_name, value: Number(c.sales_amount || 0), itemStyle: { borderColor: '#fff', borderWidth: 2 } })),
      label: { color: '#475569', fontSize: 11, formatter: p => `${p.name} ${p.percent}%` },
      emphasis: { scaleSize: 4 }
    }]
  }
})

const heatChartOption = computed(() => {
  const employees = filteredEmployees.value
  const categories = filteredCategories.value
  const empNames = employees.map(e => e.employee_name)
  const catNames = categories.map(c => c.category_name)
  const cellMap = new Map()
  for (const row of sales.value?.employee_category || []) {
    cellMap.set(`${row.employee_id ?? 'none'}-${row.category_id ?? 'none'}`, row)
  }
  const data = []
  let maxV = 0
  categories.forEach((cat, x) => {
    employees.forEach((emp, y) => {
      const cell = cellMap.get(`${empKeyOf(emp)}-${catKeyOf(cat)}`)
      if (!cell) return
      const v = salesMetric.value === 'amount' ? Number(cell.sales_amount || 0) : Number(cell.qty || 0)
      if (v > maxV) maxV = v
      data.push([x, y, v, Number(cell.sales_amount || 0), cell.qty || 0])
    })
  })
  return {
    grid: { left: 8, right: 16, top: 12, bottom: 40, containLabel: true },
    tooltip: {
      position: 'top',
      confine: true,
      formatter: p => {
        const [x, y, , amt, qty] = p.value
        return `<b>${empNames[y]} × ${catNames[x]}</b><br/>¥${formatMoney(amt)} · ${qty} 件`
      }
    },
    xAxis: {
      type: 'category',
      data: catNames,
      splitArea: { show: true, areaStyle: { color: ['#f8fafc', '#fff'] } },
      axisLabel: { color: '#475569', fontSize: 11, rotate: catNames.length > 6 ? 30 : 0 },
      axisTick: { show: false }
    },
    yAxis: {
      type: 'category',
      data: empNames,
      splitArea: { show: true, areaStyle: { color: ['#f8fafc', '#fff'] } },
      axisLabel: { color: '#334155', fontWeight: 600 },
      axisTick: { show: false }
    },
    visualMap: {
      min: 0,
      max: Math.max(maxV, 1),
      orient: 'horizontal',
      left: 'center',
      bottom: 0,
      itemWidth: 12,
      itemHeight: 80,
      text: ['多', '少'],
      textStyle: { color: '#94a3b8', fontSize: 10 },
      inRange: { color: ['#eef0ff', '#dcdaff', '#9a91ff'] }
    },
    series: [{
      name: salesMetric.value === 'amount' ? '销售额' : '销量',
      type: 'heatmap',
      data,
      label: {
        show: true,
        color: '#1e3a8a',
        fontSize: 10,
        fontWeight: 700,
        formatter: p => (salesMetric.value === 'amount' ? shortMoney(p.value[2]) : `${p.value[2]}件`)
      },
      emphasis: { itemStyle: { shadowBlur: 8, shadowColor: 'rgba(30,58,138,.35)' } }
    }]
  }
})

const renderCharts = () => {
  if (!sales.value) return
  if (empChartRef.value) {
    if (!empChart) {
      empChart = echarts.init(empChartRef.value)
      empChart.on('click', p => {
        const emp = (sales.value?.employees || []).find(e => e.employee_name === p.name)
        if (emp) salesFilterEmp.value = salesFilterEmp.value === empKeyOf(emp) ? null : empKeyOf(emp)
      })
    }
    empChart.setOption(employeeChartOption.value, true)
  }
  if (catChartRef.value) {
    if (!catChart) {
      catChart = echarts.init(catChartRef.value)
      catChart.on('click', p => {
        const cat = (sales.value?.categories || []).find(c => c.category_name === p.name)
        if (cat) salesFilterCat.value = salesFilterCat.value === catKeyOf(cat) ? null : catKeyOf(cat)
      })
    }
    catChart.setOption(categoryChartOption.value, true)
  }
  if (heatChartRef.value) {
    if (!heatChart) heatChart = echarts.init(heatChartRef.value)
    heatChart.setOption(heatChartOption.value, true)
  }
}
watch([sales, salesMetric, salesFilterEmp, salesFilterCat], () => nextTick(renderCharts))
// 页签切换：隐藏容器里初始化的图表尺寸为 0，切到可见后必须 resize
const salesTab = ref('heat')
watch(salesTab, () => nextTick(() => { renderCharts(); handleChartResize() }))
const handleChartResize = () => { empChart?.resize(); catChart?.resize(); heatChart?.resize() }
onMounted(() => window.addEventListener('resize', handleChartResize))
onBeforeUnmount(() => {
  window.removeEventListener('resize', handleChartResize)
  empChart?.dispose(); catChart?.dispose(); heatChart?.dispose()
  empChart = catChart = heatChart = null
})

// 热销商品（受品类筛选联动）
const topProductsView = computed(() => {
  const list = sales.value?.top_products || []
  if (salesFilterCat.value === null) return list
  const name = catNameByKey(salesFilterCat.value)
  return list.filter(p => p.category_name === name)
})

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
    { key: 'subsidy_to_allocate', label: '国补已提交待回款', count: raw.subsidy_to_allocate?.count || 0, amount: raw.subsidy_to_allocate?.amount || 0, to: '/admin/subsidy' },
    { key: 'subsidy_to_submit', label: '国补资料待提交', count: raw.subsidy_to_submit?.count || 0, amount: raw.subsidy_to_submit?.amount || 0, to: '/admin/subsidy' },
    { key: 'balance_due', label: '尾款待收', count: raw.balance_due?.count || 0, amount: raw.balance_due?.amount || 0, to: '/admin/orders' }
  ]
})

// ---------- 钱在哪 ----------
const funds = computed(() => ws.value?.funds || { accounts: [], frozen: { count: 0, amount: 0 }, available: 0, total: 0 })

// ---------- 经营概况 ----------
const monthData = computed(() => ws.value?.month || {})

// ---------- 流水明细抽屉（点击账户时复用财务流水接口） ----------
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

// ---------- 账户操作（既有功能区保留） ----------
const accountDialogVisible = ref(false)
const transferDialogVisible = ref(false)
const calibrateDialogVisible = ref(false)
const submitting = ref(false)
const accountFormRef = ref()
const transferFormRef = ref()
const accounts = computed(() => funds.value.accounts || [])
const accountColors = ['#7a70ff', '#0ea5e9', '#f59e0b', '#10b981', '#f43f5e', '#8b5cf6']
const accountForm = reactive({ name: '', balance: 0 })
const transferForm = reactive({ from_account_id: null, to_account_id: null, amount: 0, remark: '' })
const calibrateForm = reactive({ accountId: null, accountName: '', currentBalance: 0, realBalance: 0, remark: '' })
const calibrateDiff = computed(() => calibrateForm.realBalance - calibrateForm.currentBalance)

async function refreshAll() { await Promise.all([fetchWorkspace(), fetchSalesAnalysis()]) }
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
        <el-button type="primary" :icon="Plus" round @click="$router.push('/admin/bookkeeping')">记一笔</el-button>
        <el-button round @click="$router.push('/admin/subsidy')">核销国补</el-button>
        <el-button round @click="$router.push('/admin/orders')">记尾款</el-button>
        <el-dropdown trigger="click">
          <el-button round :icon="Management">账户操作</el-button>
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

    <!-- 首屏双卡：钱在哪 + 待我处理 -->
    <div class="hero-grid">
      <el-card shadow="never" class="ws-card" v-loading="wsLoading">
        <div class="funds-grid">
          <div class="funds-main">
            <div class="ws-card-title">钱在哪</div>
            <div class="funds-available">{{ money(funds.available) }}</div>
            <div class="funds-sub">可用资金</div>
            <div class="funds-meta">
              <el-tag v-if="funds.frozen?.count > 0" type="warning" effect="plain" size="small">
                公户冻结 {{ money(funds.frozen.amount) }} · {{ funds.frozen.count }} 笔
              </el-tag>
              <span class="funds-total">账面合计 <b>{{ money(funds.total) }}</b></span>
            </div>
          </div>
          <div class="funds-list">
            <div v-for="(account, idx) in accounts" :key="account.id" class="funds-account" @click="openAccountFlows(account)">
              <span class="funds-dot" :style="{ background: accountColors[idx % accountColors.length] }"></span>
              <span class="funds-account-name">{{ account.name }}</span>
              <b>{{ money(account.balance) }}</b>
              <el-icon class="funds-account-go"><ArrowRight /></el-icon>
            </div>
          </div>
        </div>
      </el-card>

      <el-card shadow="never" class="ws-card" v-loading="wsLoading">
        <div class="ws-card-title">待我处理</div>
        <div class="todo-rows">
          <div v-for="todo in todos" :key="todo.key" class="todo-row" @click="$router.push(todo.to)">
            <span class="todo-label">{{ todo.label }}</span>
            <span class="todo-count" :class="{ 'is-zero': !todo.count }">{{ todo.count }} 笔</span>
            <b class="todo-amount" :class="{ 'is-zero': !todo.count }">{{ money(todo.amount) }}</b>
            <span class="todo-go"><el-icon><ArrowRight /></el-icon></span>
          </div>
        </div>
      </el-card>
    </div>

    <!-- 本月经营概况 -->
    <el-card shadow="never" class="ws-card" v-loading="wsLoading">
      <div class="month-head">
        <div class="ws-card-title">经营概况</div>
        <el-radio-group v-model="monthMode" size="small" @change="fetchWorkspace">
          <el-radio-button value="current">本月</el-radio-button>
          <el-radio-button value="cumulative">累计</el-radio-button>
        </el-radio-group>
      </div>
      <div class="kpi-grid">
        <div class="kpi-tile">
          <span class="kpi-label">净利润<small>毛利扣费用</small></span>
          <b :class="Number(monthData.net_profit || 0) >= 0 ? 'pos' : 'neg'">{{ money(monthData.net_profit) }}</b>
        </div>
        <div class="kpi-tile">
          <span class="kpi-label">真实净利润<small>扣完费用工资</small></span>
          <b :class="Number(monthData.real_profit || 0) >= 0 ? 'pos' : 'neg'">{{ money(monthData.real_profit) }}</b>
        </div>
        <div class="kpi-tile">
          <span class="kpi-label">净现金流</span>
          <b>{{ money(monthData.net_cash_flow) }}</b>
        </div>
        <div class="kpi-tile">
          <span class="kpi-label">国补未回款</span>
          <b class="kpi-warn">{{ money(monthData.bridge?.subsidy_unreceived) }}</b>
        </div>
      </div>
      <div class="bridge-rows">
        <div class="bridge-row"><span>经营开销<small>（其中工资 {{ money(monthData.payroll_total) }}）</small></span><b>{{ money(monthData.bridge?.operating_expense) }}</b></div>
        <div class="bridge-row muted"><span>净利润 - 净现金流 差异</span><b>{{ money(monthData.bridge?.profit_cash_gap) }}</b></div>
      </div>
      <div v-if="monthData.uncosted_order_item_count > 0" class="ws-warn">* 本期有 {{ monthData.uncosted_order_item_count }} 个商品未录成本，毛利可能偏高</div>
    </el-card>

    <!-- 销售分析：员工/品类/商品/支付方式 -->
    <el-card shadow="never" class="ws-card" v-loading="salesLoading">
      <div class="month-head sales-head">
        <div class="ws-card-title">销售分析<small class="sales-title-note">商品口径 · 点击员工或品类行可筛选下方明细</small></div>
        <div class="sales-toolbar">
          <el-date-picker
            v-if="!salesCumulative"
            v-model="salesMonth"
            type="month"
            format="YYYY年M月"
            value-format="YYYY-MM"
            :clearable="false"
            size="small"
            class="sales-month-picker"
          />
          <el-button size="small" :type="salesCumulative ? 'primary' : 'default'" :plain="!salesCumulative" @click="salesCumulative = !salesCumulative">累计</el-button>
          <el-divider direction="vertical" />
          <el-radio-group v-model="salesMetric" size="small">
            <el-radio-button value="amount">金额</el-radio-button>
            <el-radio-button value="qty">数量</el-radio-button>
          </el-radio-group>
        </div>
      </div>

      <!-- 核心指标：紧凑一行 -->
      <div class="sales-statline">
        <div class="sales-stat">
          <span class="sales-stat-label">销售额</span>
          <b>{{ money(salesSummary.sales_amount) }}</b>
          <span v-if="salesGrowth.sales" class="sales-growth-inline" :class="salesGrowth.sales.up ? 'pos' : 'neg'">{{ salesGrowth.sales.up ? '↑' : '↓' }}{{ Math.abs(salesGrowth.sales.pct).toFixed(1) }}%</span>
        </div>
        <el-divider direction="vertical" />
        <div class="sales-stat">
          <span class="sales-stat-label">订单</span>
          <b>{{ salesSummary.order_count ?? 0 }}<small> 单</small></b>
          <span v-if="salesGrowth.orders" class="sales-growth-inline" :class="salesGrowth.orders.up ? 'pos' : 'neg'">{{ salesGrowth.orders.up ? '↑' : '↓' }}{{ Math.abs(salesGrowth.orders.pct).toFixed(1) }}%</span>
        </div>
        <el-divider direction="vertical" />
        <div class="sales-stat">
          <span class="sales-stat-label">客单价</span>
          <b>{{ money(salesSummary.avg_ticket) }}</b>
        </div>
        <el-divider direction="vertical" />
        <div class="sales-stat">
          <span class="sales-stat-label" title="销货毛利 = 销售额 − 商品成本 − 安装费 − 提成 − 推荐费 − 以旧换新；与经营看板「毛利」同口径。下表员工/品类/商品行的「商品毛利」是未扣这些费用的售价差价，两者不可直接相加对比。">销货毛利</span>
          <b :class="Number(salesSummary.gross_profit || 0) >= 0 ? 'pos' : 'neg'">{{ money(salesSummary.gross_profit) }}</b>
          <span v-if="salesGrowth.profit" class="sales-growth-inline" :class="salesGrowth.profit.up ? 'pos' : 'neg'">{{ salesGrowth.profit.up ? '↑' : '↓' }}{{ Math.abs(salesGrowth.profit.pct).toFixed(1) }}%</span>
        </div>
      </div>
      <!-- 参考指标：小字一行 -->
      <div class="sales-subline">
        <span>国补 {{ salesSummary.subsidy_order_count ?? 0 }} 笔 / {{ money(salesSummary.subsidy_amount) }}</span>
        <span>以旧换新 {{ money(salesSummary.trade_in_amount) }}</span>
        <span v-if="Number(salesSummary.refund_amount) > 0" class="neg">退款 {{ money(salesSummary.refund_amount) }}</span>
        <span v-if="Number(salesSummary.cancel_count) > 0">取消 {{ salesSummary.cancel_count }} 单</span>
        <span v-for="pm in sales?.payment_methods || []" :key="pm.payment_method">{{ paymentMethodNames[pm.payment_method] || '其他' }} {{ money(pm.amount) }}</span>
      </div>

      <!-- 筛选行：下拉筛选 + 状态 + 数据质量提醒 -->
      <div class="sales-filterbar" :class="{ 'is-active': hasSalesFilter }">
        <el-select v-model="empFilterModel" clearable placeholder="全部员工" size="small" class="sales-filter-select">
          <el-option label="全部员工" value="" />
          <el-option v-for="opt in empOptions" :key="String(opt.value)" :label="opt.label" :value="opt.value" />
        </el-select>
        <el-select v-model="catFilterModel" clearable placeholder="全部品类" size="small" class="sales-filter-select">
          <el-option label="全部品类" value="" />
          <el-option v-for="opt in catOptions" :key="String(opt.value)" :label="opt.label" :value="opt.value" />
        </el-select>
        <el-button v-if="hasSalesFilter" link size="small" type="primary" @click="clearSalesFilters">清除筛选</el-button>
        <span class="sales-filter-tip">点图表条形/色块可联动筛选</span>
        <el-tag v-if="Number(sales?.uncategorized_products) > 0" type="warning" effect="light" size="small" class="sales-uncat-tag">
          {{ sales.uncategorized_products }} 个商品未设分类，按"未分类"统计
          <el-button link type="warning" size="small" @click="$router.push('/admin/products')">去补分类</el-button>
        </el-tag>
      </div>

      <div class="sales-grid">
        <!-- 员工业绩条形图（点击条形 = 筛选该员工；老板客单独一行，不混入员工排名） -->
        <div class="sales-panel">
          <div class="sales-panel-title">员工业绩 · 销售额</div>
          <div v-if="bossRow" class="sales-boss-strip">
            <span class="sales-boss-tag">老板客</span>
            <span class="sales-boss-note">老板本人客户 · 不计入员工排名</span>
            <span class="sales-boss-num"><b>{{ money(bossRow.sales_amount) }}</b></span>
            <span class="sales-boss-num">{{ bossRow.order_count }} 单</span>
            <span class="sales-boss-num">商品毛利 <span :class="bossRow.gross_profit >= 0 ? 'pos' : 'neg'">{{ money(bossRow.gross_profit) }}</span></span>
          </div>
          <div v-if="(sales?.employees || []).length" ref="empChartRef" class="sales-chart sales-chart-bar"></div>
          <div v-else class="sales-empty">本期暂无销售数据</div>
        </div>

        <!-- 品类占比环图（点击色块 = 筛选该品类） -->
        <div class="sales-panel">
          <div class="sales-panel-title">品类销售占比</div>
          <div v-if="(sales?.categories || []).length" ref="catChartRef" class="sales-chart sales-chart-pie"></div>
          <div v-else class="sales-empty">本期暂无销售数据</div>
        </div>
      </div>

      <!-- 明细页签：热力图 / 热销商品 -->
      <el-tabs v-model="salesTab" class="sales-tabs">
        <el-tab-pane label="员工 × 品类热力" name="heat">
          <div v-if="hasHeatData" ref="heatChartRef" class="sales-chart sales-chart-heat"></div>
          <div v-else class="sales-empty">本期暂无交叉数据</div>
        </el-tab-pane>
        <el-tab-pane :label="`热销商品 TOP10（${topProductsView.length}）`" name="top">
          <el-table v-if="topProductsView.length > 0" :data="topProductsView" size="small">
            <el-table-column type="index" label="#" width="48" />
            <el-table-column label="商品" min-width="200">
              <template #default="{ row }">
                <b class="sales-product-name">{{ row.product_name }}</b>
                <el-tag size="small" effect="plain" class="sales-cat-tag">{{ row.category_name }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="qty" label="销量" align="right" width="74" sortable />
            <el-table-column prop="sales_amount" label="销售额" align="right" width="104" sortable>
              <template #default="{ row }"><b>{{ money(row.sales_amount) }}</b></template>
            </el-table-column>
            <el-table-column prop="gross_profit" label="商品毛利" align="right" width="104" sortable class-name="col-mobile-hide" header-align="right">
              <template #header>
                <span title="商品毛利 =（售价 − 进价）× 数量，未扣安装费、提成、推荐费、以旧换新">商品毛利</span>
              </template>
              <template #default="{ row }">
                <span :class="row.gross_profit >= 0 ? 'pos' : 'neg'">{{ money(row.gross_profit) }}</span>
              </template>
            </el-table-column>
          </el-table>
          <div v-else class="sales-empty">本期暂无商品数据</div>
        </el-tab-pane>
      </el-tabs>
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
.finance-page { max-width: 1320px; min-height: 100%; margin: 0 auto; padding: 28px; display: grid; gap: 18px; background: linear-gradient(180deg, #eef0f8 0%, #f6f7fb 320px); align-content: start; }
.finance-header { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 4px 2px; }
.page-kicker { color: #635bff; font-size: 11px; font-weight: 800; letter-spacing: .16em; }
.finance-header h1 { margin: 4px 0; color: #0f172a; font-size: 27px; letter-spacing: -.04em; }
.finance-header p { margin: 0; color: #64748b; font-size: 13px; }
.finance-header p .hl { color: #635bff; }
.header-actions { display: flex; align-items: center; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.ws-alert { border-radius: 12px; }

/* 卡片统一语言：白底大圆角、发丝边框、柔和悬浮投影、标题渐变竖条 */
.ws-card { border-radius: 18px; border: 1px solid #e9edf5; box-shadow: 0 1px 2px rgba(15, 23, 42, .03), 0 14px 36px -22px rgba(15, 23, 42, .16); transition: box-shadow .2s; }
.ws-card:hover { box-shadow: 0 2px 4px rgba(15, 23, 42, .04), 0 18px 44px -22px rgba(15, 23, 42, .2); }
.ws-card-title { display: flex; align-items: center; gap: 8px; font-size: 15px; font-weight: 800; color: #0f172a; margin-bottom: 14px; }
.ws-card-title::before { content: ''; flex: none; width: 4px; height: 15px; border-radius: 2px; background: linear-gradient(180deg, #9a91ff, #635bff); }
.ws-card-title small { font-weight: 400; }
.full-width { width: 100%; }
.dialog-alert { margin-bottom: 18px; }
.current-balance { color: #334155; font-size: 22px; }
.diff-positive { display: block; margin-top: 7px; color: #059669; }
.diff-negative { display: block; margin-top: 7px; color: #e11d48; }
.pos { color: #059669; }
.neg { color: #e11d48; }
.muted-cell { color: #94a3b8; font-size: 12px; }

/* 首屏双卡：钱在哪 + 待我处理 */
.hero-grid { display: grid; grid-template-columns: 1.35fr 1fr; gap: 18px; align-items: stretch; }
.funds-grid { display: grid; grid-template-columns: minmax(240px, 1fr) minmax(280px, 1.15fr); gap: 22px; align-items: center; height: 100%; }
.funds-available { font-size: 36px; font-weight: 900; letter-spacing: -.04em; font-variant-numeric: tabular-nums; background: linear-gradient(120deg, #0f172a 35%, #635bff 115%); -webkit-background-clip: text; background-clip: text; -webkit-text-fill-color: transparent; }
.funds-sub { color: #64748b; font-size: 13px; margin-top: 2px; }
.funds-meta { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin-top: 12px; }
.funds-total { color: #64748b; font-size: 13px; }
.funds-total b { color: #334155; }
.funds-list { display: grid; gap: 8px; }
.funds-account { display: flex; align-items: center; gap: 10px; padding: 11px 14px; border: 1px solid #eef1f7; border-radius: 12px; background: #fafbfe; cursor: pointer; transition: all .15s; }
.funds-account:hover { background: #eef0ff; border-color: #dcdaff; transform: translateY(-1px); }
.funds-dot { flex: none; width: 8px; height: 8px; border-radius: 50%; }
.funds-account-name { flex: 1; font-weight: 700; color: #334155; }
.funds-account b { font-variant-numeric: tabular-nums; color: #0f172a; }
.funds-account-go { color: #cbd5e1; }

.todo-rows { display: grid; gap: 8px; }
.todo-row { display: grid; grid-template-columns: 1fr 64px minmax(96px, auto) 24px; align-items: center; gap: 12px; padding: 13px 14px; border: 1px solid #eef1f7; border-radius: 12px; background: #fafbfe; cursor: pointer; transition: all .15s; }
.todo-row:hover { background: #eef0ff; border-color: #dcdaff; transform: translateY(-1px); }
.todo-label { font-weight: 700; color: #334155; }
.todo-count { color: #64748b; font-size: 13px; }
.todo-amount { color: #0f172a; font-variant-numeric: tabular-nums; }
.todo-amount.is-zero, .todo-count.is-zero { color: #cbd5e1; }
.todo-go { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; background: #eef0ff; color: #635bff; font-size: 12px; }

.month-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 4px; }
.month-head .ws-card-title { margin-bottom: 0; }
.kpi-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin: 14px 0 12px; }
.kpi-tile { padding: 15px 16px; border: 1px solid #eef1f7; border-radius: 14px; background: linear-gradient(180deg, #fafbff, #f4f6fc); }
.kpi-label { display: block; color: #64748b; font-size: 12.5px; font-weight: 600; margin-bottom: 7px; }
.kpi-label small { margin-left: 6px; color: #94a3b8; font-weight: 400; }
.kpi-tile b { font-size: 23px; letter-spacing: -.03em; font-variant-numeric: tabular-nums; }
.kpi-warn { color: #d97706; }
.bridge-rows { display: grid; gap: 4px; }
.bridge-row { display: flex; justify-content: space-between; align-items: baseline; padding: 7px 14px; border-radius: 8px; color: #475569; font-size: 13px; }
.bridge-row:nth-child(odd) { background: #f8fafc; }
.bridge-row small { color: #94a3b8; }
.bridge-row.muted { color: #94a3b8; }
.ws-warn { margin-top: 10px; color: #d97706; font-size: 11.5px; }

/* ---------- 销售分析 ---------- */
.sales-title-note { margin-left: 10px; color: #94a3b8; font-size: 11px; font-weight: 400; }
.sales-head { flex-wrap: wrap; gap: 10px; }
.sales-toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.sales-month-picker { width: 132px !important; }
.sales-toolbar .el-divider--vertical { height: 18px; margin: 0 2px; }
.sales-filterbar { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-bottom: 16px; padding: 10px 14px; border: 1px solid #eef1f7; border-radius: 12px; background: #fafbfe; }
.sales-filterbar.is-active { border-color: #dcdaff; background: #eef0ff; }
.sales-filter-select { width: 132px; }
.sales-filter-tip { color: #94a3b8; font-size: 11.5px; }
.sales-uncat-tag { margin-left: auto; }
.sales-statline { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 18px; padding: 12px 16px; border: 1px solid #eef1f7; border-radius: 14px; background: linear-gradient(180deg, #fafbff, #f4f6fc); }
.sales-stat { display: flex; align-items: baseline; gap: 7px; }
.sales-stat-label { color: #64748b; font-size: 12px; font-weight: 600; }
.sales-stat b { font-size: 20px; letter-spacing: -.02em; color: #0f172a; font-variant-numeric: tabular-nums; }
.sales-stat b small { color: #94a3b8; font-size: 12px; font-weight: 400; }
.sales-growth-inline { font-size: 11.5px; font-weight: 700; padding: 1px 7px; border-radius: 6px; }
.sales-growth-inline.pos { background: #ecfdf5; }
.sales-growth-inline.neg { background: #fff1f2; }
.sales-subline { display: flex; flex-wrap: wrap; gap: 4px 14px; padding: 10px 4px 0; color: #94a3b8; font-size: 11.5px; }
.sales-tabs { margin-top: 12px; }
.sales-tabs :deep(.el-tabs__header) { margin-bottom: 10px; }
.sales-tabs :deep(.el-tabs__item) { font-size: 13px; }
.sales-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 26px; margin-bottom: 18px; }
.sales-panel { min-width: 0; }
.sales-panel-title { font-size: 13px; font-weight: 800; color: #334155; margin-bottom: 6px; }
.sales-product-name { display: block; color: #0f172a; }
.sales-chart { width: 100%; }
.sales-chart-bar { height: 220px; }
.sales-chart-pie { height: 244px; }
.sales-chart-heat { height: 280px; }
.sales-empty { display: flex; align-items: center; justify-content: center; height: 180px; color: #94a3b8; font-size: 13px; }
.sales-boss-strip { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 14px; margin-bottom: 10px; padding: 9px 13px; border: 1px solid #fde68a; border-radius: 12px; background: linear-gradient(120deg, #fffbeb, rgba(254, 243, 199, .65)); }
.sales-boss-tag { padding: 2px 9px; border-radius: 7px; background: linear-gradient(120deg, #f59e0b, #d97706); color: #fff; font-size: 11.5px; font-weight: 800; }
.sales-boss-note { margin-right: auto; color: #b45309; font-size: 11.5px; }
.sales-boss-num { color: #92400e; font-size: 12.5px; }
.sales-boss-num b { font-size: 15px; color: #78350f; font-variant-numeric: tabular-nums; }
.sales-cat-tag { margin-left: 8px; color: #64748b; }

@media (max-width: 900px) {
  .hero-grid { grid-template-columns: 1fr; }
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
  /* 钱在哪：可用资金大字收缩，账户行贴边 */
  .funds-available { font-size: 28px; }
  .funds-account { padding: 10px 12px; }
  /* 待我处理：名称+去处理一行，数量金额第二行，"去处理"不换行不挤压金额 */
  .todo-row { grid-template-columns: minmax(0, 1fr) auto auto; grid-template-areas: "label label go" "count amount go"; row-gap: 3px; padding: 11px 12px; }
  .todo-label { grid-area: label; min-width: 0; }
  .todo-count { grid-area: count; }
  .todo-amount { grid-area: amount; justify-self: end; }
  .todo-go { grid-area: go; align-self: center; white-space: nowrap; }
  /* 经营概况：KPI 磁贴两列 */
  .kpi-grid { grid-template-columns: 1fr 1fr; gap: 10px; }
  .kpi-tile { padding: 12px 13px; }
  .kpi-tile b { font-size: 19px; }
  .bridge-row { font-size: 12.5px; }
  /* 双栏图表单列堆叠 */
  .sales-grid { grid-template-columns: 1fr; gap: 14px; margin-bottom: 12px; }
  .sales-title-note { display: none; }
  /* 核心指标：两行换行排布 */
  .sales-statline { gap: 6px 14px; padding: 10px 12px; }
  .sales-stat b { font-size: 16px; }
  .sales-subline { gap: 3px 10px; }
  /* 工具栏：月份选择器 + 累计 + 口径切换整行铺开 */
  .sales-toolbar { width: 100%; justify-content: flex-start; }
  .sales-month-picker { flex: 1; width: auto !important; }
  .sales-filterbar { padding: 8px 10px; }
  .sales-filter-select { flex: 1; width: auto; }
  .sales-filter-tip { display: none; }
  .sales-uncat-tag { margin-left: 0; }
  /* 图表高度收窄 */
  .sales-chart-bar { height: 200px; }
  .sales-chart-pie { height: 230px; }
  .sales-chart-heat { height: 260px; }
  /* 次要列（毛利/提成/占比）在手机上隐藏，防横向滚动 */
  :deep(.el-table .col-mobile-hide) { display: none; }
  :deep(.el-table) { min-width: 0; }
}
</style>

<style>
/* 流水明细抽屉：窄屏占满屏宽（抽屉宽度是行内样式，需全局覆盖） */
@media (max-width: 768px) {
  .finance-flow-drawer { width: 100% !important; }
}
</style>
