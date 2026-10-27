import { computed, shallowRef } from 'vue'
import { fetchFinanceSummary } from '../api/finance'
import { fetchOffboardingAlerts } from '../api/employees'

const emptySummary = () => ({ accounts: [], top_products: [], employee_rankings: [], sales_trend: [] })

const toISODate = (date) => `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`

// 计算上一周期：今日→昨日，本月→上月，上月→上上月，本年→去年，自定义→等长前移
export function prevPeriodParams(period, dates = []) {
  if (period === 'today') {
    const yesterday = new Date()
    yesterday.setDate(yesterday.getDate() - 1)
    return { period: 'custom', start_date: toISODate(yesterday), end_date: toISODate(yesterday) }
  }
  if (period === 'month') return { period: 'last_month' }
  if (period === 'last_month') {
    const now = new Date()
    return {
      period: 'custom',
      start_date: toISODate(new Date(now.getFullYear(), now.getMonth() - 2, 1)),
      end_date: toISODate(new Date(now.getFullYear(), now.getMonth() - 1, 0))
    }
  }
  if (period === 'year') {
    const lastYear = new Date().getFullYear() - 1
    return { period: 'custom', start_date: `${lastYear}-01-01`, end_date: `${lastYear}-12-31` }
  }
  if (period === 'custom' && dates[0] && dates[1]) {
    const start = new Date(`${dates[0]}T00:00:00`)
    const end = new Date(`${dates[1]}T00:00:00`)
    if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime()) || end < start) return null
    const days = Math.round((end - start) / 86400000) + 1
    const prevEnd = new Date(start)
    prevEnd.setDate(prevEnd.getDate() - 1)
    const prevStart = new Date(prevEnd)
    prevStart.setDate(prevStart.getDate() - (days - 1))
    return { period: 'custom', start_date: toISODate(prevStart), end_date: toISODate(prevEnd) }
  }
  return null
}

export function useDashboard() {
  const summary = shallowRef(emptySummary())
  const prevSummary = shallowRef(null)
  const loading = shallowRef(false)
  const error = shallowRef('')
  const updatedAt = shallowRef(null)
  const employeeOffboardingAlerts = shallowRef([])
  const employeeOffboardingAlertsError = shallowRef('')
  const employeeOffboardingAlertsLoading = shallowRef(false)
  let requestID = 0

  const totalBalance = computed(() => (summary.value.accounts || [])
    .filter((account) => account.name !== '经营开销')
    .reduce((sum, account) => sum + Number(account.balance || 0), 0))

  async function load(period, dates = []) {
    if (period === 'custom' && (!dates[0] || !dates[1])) return
    const currentID = ++requestID
    loading.value = true
    error.value = ''
    loadEmployeeOffboardingAlerts()
    const params = { period }
    if (period === 'custom') {
      params.start_date = dates[0]
      params.end_date = dates[1]
    }
    try {
      // 环比：同一接口查上一周期，失败只影响对比展示，不阻断当期数据
      const prevParams = prevPeriodParams(period, dates)
      const [response, prevResponse] = await Promise.all([
        fetchFinanceSummary(params),
        prevParams
          ? fetchFinanceSummary(prevParams).catch(() => null)
          : Promise.resolve(null)
      ])
      if (currentID !== requestID) return
      if (response.data.code !== 200) throw new Error('接口返回异常')
      summary.value = response.data.data || emptySummary()
      prevSummary.value = prevResponse?.data?.code === 200 ? (prevResponse.data.data || null) : null
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

  async function loadEmployeeOffboardingAlerts() {
    employeeOffboardingAlertsLoading.value = true
    employeeOffboardingAlertsError.value = ''
    try {
      const response = await fetchOffboardingAlerts()
      if (response.data.code !== 200) throw new Error('离职订单提醒加载失败')
      employeeOffboardingAlerts.value = response.data.data || []
    } catch (cause) {
      console.error('Failed to fetch employee offboarding alerts:', cause)
      employeeOffboardingAlertsError.value = '离职订单提醒加载失败'
    } finally {
      employeeOffboardingAlertsLoading.value = false
    }
  }

  return { summary, prevSummary, totalBalance, loading, error, updatedAt, employeeOffboardingAlerts, employeeOffboardingAlertsError, employeeOffboardingAlertsLoading, load, loadEmployeeOffboardingAlerts }
}
