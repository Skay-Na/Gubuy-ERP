<template>
  <div class="dashboard-page">
    <header class="dashboard-header">
      <div>
        <div class="eyebrow">经营工作台</div>
        <h1>看板</h1>
        <p>销售指标按所选期间统计，待办与存量数据实时更新。</p>
      </div>
      <div class="header-actions">
        <div class="period-tabs">
          <button v-for="item in periods" :key="item.value" :class="{ active: period === item.value }" @click="changePeriod(item.value)">
            {{ item.label }}
          </button>
        </div>
        <el-date-picker
          v-if="period === 'custom' && !isMobile"
          v-model="customDateRange"
          type="daterange"
          range-separator="-"
          start-placeholder="开始"
          end-placeholder="结束"
          size="small"
          value-format="YYYY-MM-DD"
          class="!w-full md:!w-60"
          @change="fetchData"
        />
        <div v-if="period === 'custom' && isMobile" class="grid grid-cols-2 gap-2 w-full">
          <input
            :value="customDateRange[0] || ''"
            type="date"
            inputmode="none"
            aria-label="开始日期"
            class="mobile-native-date-input"
            @change="updateCustomDateRange(0, $event.target.value)"
          />
          <input
            :value="customDateRange[1] || ''"
            type="date"
            inputmode="none"
            aria-label="结束日期"
            class="mobile-native-date-input"
            @change="updateCustomDateRange(1, $event.target.value)"
          />
        </div>
        <el-button :icon="Refresh" :loading="loading" round @click="fetchData">刷新</el-button>
      </div>
    </header>

    <section class="hero-grid">
      <article v-for="card in kpiCards" :key="card.key" class="kpi-card" :class="card.tone" @click="card.route && router.push(card.route)">
        <div class="kpi-card-top">
          <span>{{ card.label }}</span>
          <el-icon><component :is="card.icon" /></el-icon>
        </div>
        <strong>{{ card.value }}</strong>
        <p>{{ card.helper }}</p>
      </article>
    </section>

    <section class="workbench-grid">
      <article class="panel todo-panel">
        <div class="panel-head">
          <div>
            <h2>当前要处理</h2>
            <p>按经营风险排序，点右侧按钮直接进入处理页面。</p>
          </div>
          <el-tag :type="todos.totalCount > 0 ? 'danger' : 'success'" effect="light" round>{{ todos.totalCount }} 项</el-tag>
        </div>

        <div class="todo-list">
          <button v-for="item in visibleTodos" :key="item.key" class="todo-item" :class="item.level" @click="router.push(item.route)">
            <div class="todo-priority">{{ levelLabel(item.level) }}</div>
            <div class="todo-main">
              <strong>{{ item.title }}</strong>
              <span v-if="item.amount > 0">{{ formatCurrency(item.amount) }}</span>
              <span v-else>{{ item.count }} 项待处理</span>
            </div>
            <div class="todo-count">{{ item.count }}</div>
            <div class="todo-action">{{ item.action }}</div>
          </button>
          <el-empty v-if="visibleTodos.length === 0" description="当前没有紧急待办" :image-size="72" />
        </div>
      </article>

      <article class="panel trend-panel">
        <div class="panel-head compact">
          <div>
            <h2>销售走势</h2>
            <p>{{ trendHint }}</p>
          </div>
        </div>
        <div class="chart-wrap">
          <v-chart class="chart" :option="trendChartOption" autoresize />
        </div>
      </article>
    </section>

    <section class="mobile-tabs-card">
      <el-tabs v-model="mobileTab" stretch>
        <el-tab-pane label="销售" name="sales">
          <div class="tab-stack">
            <sales-summary-card :summary="summary" />
            <ranking-list title="本期员工业绩" :items="employeeRankingItems" empty-text="暂无员工业绩" />
          </div>
        </el-tab-pane>
        <el-tab-pane label="商品" name="products">
          <div class="tab-stack">
            <product-section title="热销低库存" :items="productInsights.hotLowStock" empty-text="暂无热销缺货" hot-low />
            <product-section title="低库存商品" :items="productInsights.lowStock" empty-text="库存健康" />
          </div>
        </el-tab-pane>
        <el-tab-pane label="资金" name="finance">
          <div class="tab-stack">
            <account-section :accounts="summary.accounts || []" :total="totalBalance" />
          </div>
        </el-tab-pane>
      </el-tabs>
    </section>

    <section class="desktop-detail-grid">
      <article class="panel">
        <div class="panel-head">
          <div>
            <h2>商品经营建议</h2>
            <p>优先关注“卖得动但快没货”的商品。</p>
          </div>
          <el-button text type="primary" @click="router.push('/admin/products')">商品库</el-button>
        </div>
        <div class="insight-columns">
          <product-section title="热销低库存" :items="productInsights.hotLowStock" empty-text="暂无热销缺货" hot-low />
          <product-section title="低库存雷达" :items="productInsights.lowStock" empty-text="库存健康" />
        </div>
      </article>

      <article class="panel">
        <div class="panel-head">
          <div>
            <h2>绩效与收款</h2>
            <p>左侧看谁贡献销售，右侧看钱在哪个账户。</p>
          </div>
        </div>
        <div class="split-list">
          <ranking-list title="本期员工业绩" :items="employeeRankingItems" empty-text="暂无员工业绩" />
          <account-section :accounts="summary.accounts || []" :total="totalBalance" />
        </div>
      </article>
    </section>
  </div>
</template>

<script setup>
import { computed, defineComponent, h, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import request from '../../utils/request'
import { buildDashboardTodos, buildProductInsights, formatCurrency } from '../../utils/dashboard'
import { Money, Wallet, Bell, DataLine, Refresh } from '@element-plus/icons-vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import * as echarts from 'echarts/core'

use([CanvasRenderer, BarChart, LineChart, GridComponent, TooltipComponent, LegendComponent])

const router = useRouter()
const loading = ref(false)
const period = ref('today')
const customDateRange = ref([])
const mobileTab = ref('sales')
const summary = ref({ accounts: [], top_products: [], employee_rankings: [], sales_trend: [] })
const allProducts = ref([])
const allOrders = ref([])
const pendingOrders = ref([])
const pendingInbounds = ref([])
const windowWidth = ref(window.innerWidth)

const periods = [
  { label: '今日', value: 'today' },
  { label: '本月', value: 'month' },
  { label: '上个月', value: 'last_month' },
  { label: '本年', value: 'year' },
  { label: '自定义', value: 'custom' }
]

const handleResize = () => {
  windowWidth.value = window.innerWidth
}

const isMobile = computed(() => windowWidth.value < 768)
const averageOrderValue = computed(() => Number(summary.value.order_count || 0) > 0 ? Number(summary.value.today_revenue || 0) / Number(summary.value.order_count || 1) : 0)
const grossMargin = computed(() => Number(summary.value.today_revenue || 0) > 0 ? (Number(summary.value.gross_profit || 0) / Number(summary.value.today_revenue || 1)) * 100 : 0)
const netCashFlow = computed(() => Number(summary.value.total_income || 0) - Number(summary.value.total_expense || 0))
const totalBalance = computed(() => (summary.value.accounts || []).reduce((sum, account) => sum + Number(account.balance || 0), 0))

const todos = computed(() => buildDashboardTodos({
  summary: summary.value,
  pendingOrders: pendingOrders.value,
  pendingInbounds: pendingInbounds.value,
  products: allProducts.value,
  orders: allOrders.value
}))

const visibleTodos = computed(() => todos.value.items.filter(item => item.count > 0 || item.amount > 0))
const productInsights = computed(() => buildProductInsights({ topProducts: summary.value.top_products || [], products: allProducts.value }))

const kpiCards = computed(() => [
  {
    key: 'revenue',
    label: '销售额',
    value: formatCurrency(summary.value.today_revenue),
    helper: `${summary.value.order_count || 0} 单 · 客单 ${formatCurrency(averageOrderValue.value)}`,
    tone: 'blue',
    icon: DataLine,
    route: '/admin/orders'
  },
  {
    key: 'profit',
    label: '毛利',
    value: formatCurrency(summary.value.gross_profit),
    helper: `毛利率 ${grossMargin.value.toFixed(1)}% · 现金净流量 ${formatCurrency(netCashFlow.value)}`,
    tone: 'green',
    icon: Money,
    route: '/admin/finance'
  },
  {
    key: 'balance',
    label: '待收尾款',
    value: formatCurrency(summary.value.pending_balance),
    helper: `${summary.value.pending_balance_count || 0} 单待结清 · 实时存量`,
    tone: 'red',
    icon: Bell,
    route: '/admin/orders'
  },
  {
    key: 'subsidy',
    label: '国补待回',
    value: formatCurrency(summary.value.pending_subsidy),
    helper: `实时存量 · 账户资产 ${formatCurrency(totalBalance.value)}`,
    tone: 'orange',
    icon: Wallet,
    route: '/admin/subsidy'
  }
])

const trendHint = computed(() => period.value === 'today' ? '今日订单按时间聚合不足时，显示当期趋势。' : '销售额与订单数组合趋势。')

const trendChartOption = computed(() => {
  const data = summary.value.sales_trend || []
  return {
    tooltip: { trigger: 'axis', confine: true },
    legend: { top: 0, right: 0, data: ['销售额', '订单数'] },
    grid: { left: isMobile.value ? 8 : 24, right: 16, top: 42, bottom: 20, containLabel: true },
    xAxis: { type: 'category', data: data.map(item => item.date ? String(item.date).split('T')[0].slice(5) : '-') },
    yAxis: [
      { type: 'value', name: '销售额' },
      { type: 'value', name: '单数' }
    ],
    series: [
      {
        name: '销售额',
        type: 'bar',
        barMaxWidth: 28,
        itemStyle: { color: '#2563eb', borderRadius: [8, 8, 0, 0] },
        data: data.map(item => Number(item.revenue || 0))
      },
      {
        name: '订单数',
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        symbolSize: 8,
        itemStyle: { color: '#f97316' },
        lineStyle: { width: 3 },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(249,115,22,0.20)' },
            { offset: 1, color: 'rgba(249,115,22,0.02)' }
          ])
        },
        data: data.map(item => Number(item.order_count || 0))
      }
    ]
  }
})

const employeeRankingItems = computed(() => (summary.value.employee_rankings || []).slice(0, 6).map((employee, index) => ({
  id: employee.employee_id || `boss-${index}`,
  title: employee.name,
  meta: `${employee.order_count || 0} 单 · 毛利 ${formatCurrency(employee.gross_profit)}`,
  value: formatCurrency(employee.revenue),
  rank: index + 1
})))

const levelLabel = (level) => ({ high: '高', medium: '中', low: '低' }[level] || '低')

const changePeriod = (value) => {
  period.value = value
  if (value !== 'custom') {
    customDateRange.value = []
    fetchData()
  }
}

const updateCustomDateRange = (index, value) => {
  const range = [customDateRange.value?.[0] || '', customDateRange.value?.[1] || '']
  range[index] = value
  customDateRange.value = range.some(Boolean) ? range : []
  if (customDateRange.value[0] && customDateRange.value[1]) fetchData()
}

const fetchData = async () => {
  if (period.value === 'custom' && (!customDateRange.value?.[0] || !customDateRange.value?.[1])) return
  loading.value = true
  try {
    const params = { period: period.value }
    if (period.value === 'custom') {
      params.start_date = customDateRange.value[0]
      params.end_date = customDateRange.value[1]
    }

    const [summaryRes, productsRes, ordersRes, pendingInboundsRes] = await Promise.all([
      request.get('/api/finance/summary', { params }),
      request.get('/api/products', { params: { all: 1 } }),
      request.get('/api/orders'),
      request.get('/api/pending-inbounds?status=pending')
    ])

    if (summaryRes.data.code === 200) summary.value = summaryRes.data.data || {}
    if (productsRes.data.code === 200) allProducts.value = productsRes.data.data || []
    if (ordersRes.data.code === 200) {
      allOrders.value = ordersRes.data.data || []
      pendingOrders.value = allOrders.value.filter(order => order.order_status === 1 && order.payment_status === 1)
    }
    if (pendingInboundsRes.data.code === 200) pendingInbounds.value = pendingInboundsRes.data.data || []
  } catch (error) {
    console.error('Failed to fetch dashboard data:', error)
  } finally {
    loading.value = false
  }
}

const SalesSummaryCard = defineComponent({
  name: 'SalesSummaryCard',
  props: { summary: { type: Object, required: true } },
  setup(props) {
    return () => {
      const revenue = Number(props.summary.today_revenue || 0)
      const orderCount = Number(props.summary.order_count || 0)
      const grossProfit = Number(props.summary.gross_profit || 0)
      const income = Number(props.summary.total_income || 0)
      const expense = Number(props.summary.total_expense || 0)
      const averageOrder = orderCount > 0 ? revenue / orderCount : 0
      const grossMargin = revenue > 0 ? (grossProfit / revenue) * 100 : 0
      return h('div', { class: 'mini-card sales-overview-card' }, [
        h('div', { class: 'sales-overview-head' }, [
          h('div', [h('div', { class: 'mini-card-title' }, '销售概览'), h('small', '本期销售、毛利和客单表现')]),
          h('span', `${orderCount} 单`)
        ]),
        h('div', { class: 'sales-main-metric' }, [
          h('span', '销售额'),
          h('strong', formatCurrency(revenue)),
          h('small', `现金净流量 ${formatCurrency(income - expense)}`)
        ]),
        h('div', { class: 'sales-metric-grid' }, [
          h('div', { class: 'sales-metric-item' }, [h('span', '毛利'), h('b', formatCurrency(grossProfit))]),
          h('div', { class: 'sales-metric-item' }, [h('span', '毛利率'), h('b', `${grossMargin.toFixed(1)}%`)]),
          h('div', { class: 'sales-metric-item' }, [h('span', '客单价'), h('b', formatCurrency(averageOrder))]),
          h('div', { class: 'sales-metric-item' }, [h('span', '支出'), h('b', formatCurrency(expense))])
        ])
      ])
    }
  }
})

const RankingList = defineComponent({
  name: 'RankingList',
  props: { title: String, items: Array, emptyText: String },
  setup(props) {
    return () => h('div', { class: 'mini-card ranking-card' }, [
      h('div', { class: 'mini-card-title' }, props.title),
      props.items?.length
        ? h('div', { class: 'ranking-list' }, props.items.map(item => h('div', { class: 'ranking-row', key: item.id }, [
          h('span', { class: item.rank <= 3 ? 'rank top' : 'rank' }, item.rank),
          h('div', { class: 'ranking-name' }, [h('b', item.title), h('small', item.meta)]),
          h('strong', item.value)
        ])))
        : h('div', { class: 'empty-lite' }, props.emptyText)
    ])
  }
})

const ProductSection = defineComponent({
  name: 'ProductSection',
  props: { title: String, items: Array, emptyText: String, hotLow: Boolean },
  setup(props) {
    return () => h('div', { class: 'mini-card product-card' }, [
      h('div', { class: 'mini-card-title' }, props.title),
      props.items?.length
        ? h('div', { class: 'simple-list' }, props.items.slice(0, 6).map((item, index) => h('button', { class: 'product-row', key: `${item.name || item.id}-${index}`, onClick: () => router.push('/admin/products') }, [
          h('div', { class: 'product-index' }, String(index + 1).padStart(2, '0')),
          h('div', { class: 'simple-main' }, [
            h('b', item.name),
            h('small', props.hotLow ? `销量 ${item.quantity || 0} · 营收 ${formatCurrency(item.revenue)}` : `主仓 ${item.main_stock || 0} · 门店 ${item.store_stock || 0} · 云仓 ${item.cloud_stock || 0}`)
          ]),
          h('strong', { class: Number(item.stockTotal || 0) < 3 ? 'stock-pill danger' : 'stock-pill' }, `余 ${item.stockTotal ?? 0}`)
        ])))
        : h('div', { class: 'empty-lite' }, props.emptyText)
    ])
  }
})

const AccountSection = defineComponent({
  name: 'AccountSection',
  props: { accounts: Array, total: Number },
  setup(props) {
    return () => h('div', { class: 'mini-card account-card' }, [
      h('div', { class: 'account-head' }, [
        h('div', [h('div', { class: 'mini-card-title' }, '收款账户'), h('small', '账户余额分布')]),
        h('strong', formatCurrency(props.total))
      ]),
      h('div', { class: 'account-list' }, (props.accounts || []).map(account => {
        const percent = props.total > 0 ? Math.max(2, Math.round((Number(account.balance || 0) / props.total) * 100)) : 0
        return h('div', { class: 'account-item', key: account.id }, [
          h('div', { class: 'account-row' }, [h('span', account.name), h('b', formatCurrency(account.balance))]),
          h('div', { class: 'account-bar' }, h('i', { style: { width: `${percent}%` } }))
        ])
      }))
    ])
  }
})

onMounted(() => {
  fetchData()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
})
</script>

<style scoped>
.dashboard-page {
  min-height: 100vh;
  padding: clamp(10px, 1.2vw, 18px);
  background: #f8fafc;
  color: #0f172a;
}

.mobile-native-date-input {
  width: 100%;
  min-height: 36px;
  padding: 0 10px;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  background: #fff;
  color: #334155;
  font-size: 16px;
}

.dashboard-header {
  max-width: 1280px;
  margin: 0 auto 14px;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 14px;
}

.eyebrow {
  color: #2563eb;
  font-size: 12px;
  font-weight: 900;
  letter-spacing: 0.16em;
}

.dashboard-header h1 {
  margin: 2px 0;
  font-size: clamp(26px, 2.4vw, 34px);
  font-weight: 950;
  letter-spacing: -0.04em;
}

.dashboard-header p,
.panel-head p {
  margin: 0;
  color: #64748b;
  font-size: 13px;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.period-tabs {
  display: flex;
  padding: 4px;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 999px;
  box-shadow: 0 8px 22px rgba(15, 23, 42, 0.04);
}

.period-tabs button {
  border: 0;
  background: transparent;
  padding: 7px 14px;
  border-radius: 999px;
  color: #64748b;
  font-weight: 800;
  cursor: pointer;
}

.period-tabs button.active {
  background: #2563eb;
  color: #fff;
  box-shadow: 0 8px 18px rgba(37, 99, 235, 0.24);
}

.hero-grid,
.workbench-grid,
.desktop-detail-grid {
  max-width: 1280px;
  margin: 0 auto 14px;
}

.hero-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: clamp(10px, 1vw, 14px);
}

.kpi-card {
  border: 1px solid #e2e8f0;
  background: #fff;
  border-radius: 20px;
  padding: clamp(13px, 1.2vw, 18px);
  cursor: pointer;
  box-shadow: 0 14px 32px rgba(15, 23, 42, 0.06);
  transition: transform .18s ease, box-shadow .18s ease;
}

.kpi-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.10);
}

.kpi-card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: #64748b;
  font-weight: 900;
  font-size: 13px;
}

.kpi-card-top .el-icon {
  width: 36px;
  height: 36px;
  border-radius: 14px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #fff;
}

.kpi-card strong {
  display: block;
  margin-top: 12px;
  font-size: clamp(21px, 2vw, 28px);
  line-height: 1.08;
  letter-spacing: -0.04em;
}

.kpi-card p {
  margin: 10px 0 0;
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.kpi-card.blue .el-icon { background: linear-gradient(135deg, #2563eb, #06b6d4); }
.kpi-card.green .el-icon { background: linear-gradient(135deg, #059669, #14b8a6); }
.kpi-card.red .el-icon { background: linear-gradient(135deg, #dc2626, #f97316); }
.kpi-card.orange .el-icon { background: linear-gradient(135deg, #f59e0b, #f97316); }

.workbench-grid {
  display: grid;
  grid-template-columns: minmax(0, 0.95fr) minmax(0, 1.35fr);
  gap: clamp(12px, 1.2vw, 18px);
}

.panel,
.mobile-tabs-card,
.mini-card {
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid #e2e8f0;
  border-radius: 22px;
  box-shadow: 0 14px 32px rgba(15, 23, 42, 0.06);
}

.panel {
  padding: clamp(14px, 1.2vw, 18px);
}

.panel-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}

.panel-head h2,
.mini-card-title {
  margin: 0 0 3px;
  font-size: clamp(16px, 1.35vw, 18px);
  font-weight: 950;
  letter-spacing: -0.03em;
}

.todo-list,
.simple-list,
.tab-stack {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.todo-item,
.product-row {
  width: 100%;
  border: 1px solid #e2e8f0;
  background: #fff;
  border-radius: 18px;
  padding: 12px;
  display: grid;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  text-align: left;
}

.todo-item {
  grid-template-columns: 34px 1fr 42px 64px;
}

.todo-priority {
  width: 30px;
  height: 30px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 950;
  font-size: 12px;
}

.todo-item.high .todo-priority { background: #fee2e2; color: #dc2626; }
.todo-item.medium .todo-priority { background: #fef3c7; color: #d97706; }
.todo-item.low .todo-priority { background: #e0f2fe; color: #0284c7; }

.todo-main strong,
.simple-main b {
  display: block;
  font-size: 14px;
  color: #0f172a;
}

.todo-main span,
.simple-main small {
  display: block;
  color: #64748b;
  font-size: 12px;
  margin-top: 2px;
}

.todo-count {
  font-size: 22px;
  font-weight: 950;
  text-align: center;
}

.todo-action {
  color: #2563eb;
  font-size: 12px;
  font-weight: 900;
  text-align: right;
}

.chart-wrap {
  height: 320px;
}
.chart { width: 100%; height: 100%; }

.mobile-tabs-card {
  display: none;
  max-width: 1280px;
  margin: 0 auto 18px;
  padding: 8px 12px 14px;
}

.desktop-detail-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.1fr) minmax(0, 0.9fr);
  gap: 18px;
}

.insight-columns {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 360px), 1fr));
  gap: 14px;
}

.split-list {
  display: grid;
  grid-template-columns: minmax(0, 1.08fr) minmax(0, 0.92fr);
  gap: 14px;
}

.mini-card {
  padding: 14px;
  box-shadow: none;
}

.simple-row,
.account-row,
.metric-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 9px 0;
  border-bottom: 1px dashed #e2e8f0;
}

.simple-row:last-child,
.account-row:last-child,
.metric-row:last-child { border-bottom: 0; }

.ranking-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ranking-row {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  padding: 10px;
  border-radius: 14px;
  background: #f8fafc;
  border: 1px solid #eef2f7;
}

.ranking-row strong {
  font-size: 14px;
  color: #0f172a;
}

.ranking-name {
  min-width: 0;
}

.ranking-name b {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #0f172a;
}

.ranking-name small {
  display: block;
  margin-top: 2px;
  color: #64748b;
  font-size: 12px;
}

.rank {
  width: 26px;
  height: 26px;
  border-radius: 10px;
  background: #e2e8f0;
  color: #475569;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 950;
}

.rank.top {
  background: #2563eb;
  color: #fff;
}

.product-row {
  box-sizing: border-box;
  grid-template-columns: 42px minmax(0, 1fr) auto;
  min-height: 64px;
  min-width: 0;
}

.product-index {
  width: 34px;
  height: 34px;
  border-radius: 12px;
  background: #eff6ff;
  color: #2563eb;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 950;
  letter-spacing: -0.04em;
}

.stock-pill {
  padding: 5px 9px;
  border-radius: 999px;
  background: #ecfdf5;
  color: #059669;
  font-size: 13px;
  justify-self: end;
  white-space: nowrap;
}

.stock-pill.danger {
  background: #fee2e2;
  color: #dc2626;
}

.danger-text { color: #dc2626; }
.empty-lite { color: #94a3b8; font-size: 13px; padding: 18px 0; text-align: center; }
.account-card {
  background: linear-gradient(180deg, #ffffff 0%, #f8fafc 100%);
}

.account-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.account-head small {
  color: #64748b;
  font-size: 12px;
}

.account-head strong {
  color: #0f172a;
  font-size: 18px;
  white-space: nowrap;
}

.account-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.account-item {
  padding: 10px;
  border-radius: 14px;
  background: #fff;
  border: 1px solid #eef2f7;
}

.account-row {
  padding: 0;
  border-bottom: 0;
  font-size: 13px;
}

.account-row span {
  color: #475569;
  font-weight: 800;
}

.account-row b {
  color: #0f172a;
  font-size: 13px;
}

.account-bar {
  height: 7px;
  margin-top: 8px;
  border-radius: 999px;
  background: #e2e8f0;
  overflow: hidden;
}

.account-bar i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #2563eb, #06b6d4);
}

/* 子组件内部节点需要 deep，避免 scoped 样式失效后内容挤在一起 */
:deep(.sales-overview-card) {
  overflow: hidden;
  background: linear-gradient(180deg, #ffffff 0%, #eff6ff 100%);
}

:deep(.sales-overview-head) {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

:deep(.sales-overview-head small) {
  display: block;
  margin-top: 2px;
  color: #64748b;
  font-size: 12px;
}

:deep(.sales-overview-head > span) {
  padding: 5px 10px;
  border-radius: 999px;
  background: #dbeafe;
  color: #2563eb;
  font-size: 12px;
  font-weight: 950;
  white-space: nowrap;
}

:deep(.sales-main-metric) {
  padding: 14px;
  border-radius: 18px;
  background: #2563eb;
  color: #fff;
  box-shadow: 0 12px 24px rgba(37, 99, 235, 0.22);
}

:deep(.sales-main-metric span) {
  display: block;
  font-size: 12px;
  font-weight: 900;
  opacity: .86;
}

:deep(.sales-main-metric strong) {
  display: block;
  margin-top: 6px;
  font-size: 28px;
  line-height: 1;
  letter-spacing: -0.05em;
}

:deep(.sales-main-metric small) {
  display: block;
  margin-top: 8px;
  font-size: 12px;
  opacity: .88;
}

:deep(.sales-metric-grid) {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 10px;
}

:deep(.sales-metric-item) {
  padding: 11px 12px;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.88);
  border: 1px solid #dbeafe;
}

:deep(.sales-metric-item span) {
  display: block;
  color: #64748b;
  font-size: 12px;
  font-weight: 900;
}

:deep(.sales-metric-item b) {
  display: block;
  margin-top: 5px;
  color: #0f172a;
  font-size: 15px;
  line-height: 1.15;
  word-break: break-word;
}

:deep(.simple-list),
:deep(.ranking-list),
:deep(.account-list) {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 360px;
  overflow-y: auto;
  padding-right: 2px;
}

:deep(.product-row) {
  box-sizing: border-box;
  width: 100%;
  min-height: 68px;
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr) max-content;
  align-items: start;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid #e2e8f0;
  border-radius: 16px;
  background: #fff;
  min-width: 0;
  text-align: left;
}

:deep(.product-index) {
  width: 34px;
  height: 34px;
  border-radius: 12px;
  background: #eff6ff;
  color: #2563eb;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 950;
}

:deep(.simple-main) {
  min-width: 0;
}

:deep(.simple-main b) {
  display: -webkit-box;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.35;
  overflow: hidden;
  text-overflow: ellipsis;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  word-break: normal;
  overflow-wrap: anywhere;
}

:deep(.simple-main small) {
  display: block;
  margin-top: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.35;
  overflow-wrap: anywhere;
}

:deep(.stock-pill) {
  padding: 5px 10px;
  border-radius: 999px;
  background: #ecfdf5;
  color: #059669;
  font-size: 13px;
  font-weight: 950;
  justify-self: end;
  line-height: 1.2;
  white-space: nowrap;
}

:deep(.stock-pill.danger) {
  background: #fee2e2;
  color: #dc2626;
}

:deep(.ranking-row) {
  display: grid;
  grid-template-columns: 30px minmax(160px, 1fr) max-content;
  align-items: center;
  gap: 12px;
  padding: 11px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 16px;
  background: #fff;
}

:deep(.ranking-name) {
  min-width: 0;
}

:deep(.ranking-name b) {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #0f172a;
  font-size: 14px;
  line-height: 1.35;
}

:deep(.ranking-name small) {
  display: block;
  margin-top: 4px;
  color: #64748b;
  font-size: 12px;
}

:deep(.ranking-row strong) {
  color: #0f172a;
  font-size: 15px;
  font-weight: 950;
  white-space: nowrap;
  text-align: right;
}

:deep(.rank) {
  width: 26px;
  height: 26px;
  border-radius: 10px;
  background: #e2e8f0;
  color: #475569;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 950;
}

:deep(.rank.top) {
  background: #2563eb;
  color: #fff;
}

:deep(.account-head) {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

:deep(.account-head strong) {
  color: #0f172a;
  font-size: 18px;
  white-space: nowrap;
}

:deep(.account-head small) {
  color: #64748b;
  font-size: 12px;
}

:deep(.account-item) {
  padding: 11px 12px;
  border-radius: 16px;
  background: #fff;
  border: 1px solid #e2e8f0;
}

:deep(.account-row) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0;
  border-bottom: 0;
}

:deep(.account-row span) {
  color: #475569;
  font-size: 13px;
  font-weight: 900;
}

:deep(.account-row b) {
  color: #0f172a;
  font-size: 14px;
  font-weight: 950;
  white-space: nowrap;
}

:deep(.account-bar) {
  height: 7px;
  margin-top: 8px;
  border-radius: 999px;
  background: #e2e8f0;
  overflow: hidden;
}

:deep(.account-bar i) {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #2563eb, #06b6d4);
}

@media (max-width: 1500px) {
  .desktop-detail-grid {
    grid-template-columns: 1fr;
  }

  .insight-columns,
  .split-list {
    grid-template-columns: 1fr;
  }

  :deep(.simple-list),
  :deep(.ranking-list),
  :deep(.account-list) {
    max-height: 300px;
  }
}

@media (max-width: 1180px) {
  .dashboard-header {
    align-items: stretch;
    flex-direction: column;
  }

  .header-actions {
    justify-content: flex-start;
  }

  .hero-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .workbench-grid {
    grid-template-columns: 1fr;
  }

  .chart-wrap {
    height: 280px;
  }
}

@media (max-width: 1024px) {
  .hero-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .workbench-grid,
  .desktop-detail-grid { grid-template-columns: 1fr; }
}

@media (max-width: 767px) {
  .dashboard-page { padding: 12px; }
  .dashboard-header { align-items: stretch; flex-direction: column; margin-bottom: 12px; }
  .dashboard-header h1 { font-size: 26px; }
  .header-actions { justify-content: stretch; }
  .period-tabs { width: 100%; overflow-x: auto; justify-content: space-between; }
  .period-tabs button { flex: 1; padding: 8px 10px; }
  .hero-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
  .kpi-card { border-radius: 18px; padding: 13px; }
  .kpi-card strong { font-size: 20px; margin-top: 12px; }
  .kpi-card p { font-size: 11px; line-height: 1.35; }
  .kpi-card-top .el-icon { width: 30px; height: 30px; border-radius: 11px; }
  .workbench-grid { gap: 12px; }
  .panel { padding: 14px; border-radius: 20px; }
  .trend-panel { display: none; }
  .todo-item { grid-template-columns: 30px 1fr 34px; }
  .todo-action { display: none; }
  .mobile-tabs-card { display: block; }
  .desktop-detail-grid { display: none; }
  .split-list,
  .insight-columns { grid-template-columns: 1fr; }
}
</style>
