import { computed, shallowRef } from 'vue'
import request from '../utils/request'
import { buildDashboardTodos, buildProductInsights } from '../utils/dashboard'

const emptySummary = () => ({ accounts: [], top_products: [], employee_rankings: [], sales_trend: [] })

export function useDashboard() {
  const summary = shallowRef(emptySummary())
  const products = shallowRef([])
  const orders = shallowRef([])
  const pendingInbounds = shallowRef([])
  const loading = shallowRef(false)
  const error = shallowRef('')
  const updatedAt = shallowRef(null)
  let requestID = 0

  const todos = computed(() => buildDashboardTodos({
    summary: summary.value,
    pendingOrders: orders.value.filter(order => Number(order.order_status) === 1 && Number(order.payment_status) === 1),
    pendingInbounds: pendingInbounds.value,
    products: products.value,
    orders: orders.value
  }))
  const productInsights = computed(() => buildProductInsights({
    topProducts: summary.value.top_products || [],
    products: products.value
  }))
  const totalBalance = computed(() => (summary.value.accounts || []).reduce((sum, account) => sum + Number(account.balance || 0), 0))

  async function load(period, dates = []) {
    if (period === 'custom' && (!dates[0] || !dates[1])) return
    const currentID = ++requestID
    loading.value = true
    error.value = ''
    const params = { period }
    if (period === 'custom') {
      params.start_date = dates[0]
      params.end_date = dates[1]
    }
    try {
      const responses = await Promise.all([
        request.get('/api/finance/summary', { params }),
        request.get('/api/products', { params: { all: 1 } }),
        request.get('/api/orders'),
        request.get('/api/pending-inbounds', { params: { status: 'pending' } })
      ])
      if (currentID !== requestID) return
      if (responses.some(response => response.data.code !== 200)) throw new Error('接口返回异常')
      summary.value = responses[0].data.data || emptySummary()
      products.value = responses[1].data.data || []
      orders.value = responses[2].data.data || []
      pendingInbounds.value = responses[3].data.data || []
      updatedAt.value = new Date()
    } catch (cause) {
      if (currentID === requestID) {
        console.error('Failed to fetch dashboard data:', cause)
        error.value = '数据加载失败，请重试'
      }
    } finally {
      if (currentID === requestID) loading.value = false
    }
  }

  return { summary, todos, productInsights, totalBalance, loading, error, updatedAt, load }
}
