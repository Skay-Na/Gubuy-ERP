import { computed, shallowRef } from 'vue'
import request from '../utils/request'

const emptySummary = () => ({ accounts: [], top_products: [], employee_rankings: [], sales_trend: [] })

export function useDashboard() {
  const summary = shallowRef(emptySummary())
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
      const response = await request.get('/api/finance/summary', { params })
      if (currentID !== requestID) return
      if (response.data.code !== 200) throw new Error('接口返回异常')
      summary.value = response.data.data || emptySummary()
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
      const response = await request.get('/api/employee-offboarding-alerts')
      if (response.data.code !== 200) throw new Error('离职订单提醒加载失败')
      employeeOffboardingAlerts.value = response.data.data || []
    } catch (cause) {
      console.error('Failed to fetch employee offboarding alerts:', cause)
      employeeOffboardingAlertsError.value = '离职订单提醒加载失败'
    } finally {
      employeeOffboardingAlertsLoading.value = false
    }
  }

  return { summary, totalBalance, loading, error, updatedAt, employeeOffboardingAlerts, employeeOffboardingAlertsError, employeeOffboardingAlertsLoading, load, loadEmployeeOffboardingAlerts }
}
