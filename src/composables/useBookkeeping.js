import { computed, readonly, ref } from 'vue'
import request from '../utils/request'

const emptySummary = {
  accounts: [],
  total_income: 0,
  total_expense: 0
}

export function useBookkeeping() {
  const summary = ref(emptySummary)
  const entries = ref([])
  const total = ref(0)
  const loading = ref(false)
  const ledgerLoading = ref(false)

  const accounts = computed(() => summary.value.accounts || [])
  const netAmount = computed(() =>
    Number(summary.value.total_income || 0) - Number(summary.value.total_expense || 0)
  )
  const totalBalance = computed(() =>
    accounts.value.reduce((sum, account) => sum + Number(account.balance || 0), 0)
  )

  async function loadSummary(period = 'month', customRange = []) {
    loading.value = true
    try {
      const params = { period }
      if (period === 'custom') {
        params.start_date = customRange[0]
        params.end_date = customRange[1]
      }
      const response = await request.get('/api/finance/summary', { params })
      summary.value = response.data.code === 200 ? response.data.data : emptySummary
    } finally {
      loading.value = false
    }
  }

  async function loadEntries(filters = {}) {
    ledgerLoading.value = true
    try {
      const response = await request.get('/api/finance/logs', { params: filters })
      entries.value = response.data.data || []
      total.value = Number(response.data.total || 0)
    } finally {
      ledgerLoading.value = false
    }
  }

  async function createEntry(payload) {
    const response = await request.post('/api/bookkeeping/entries', payload)
    if (response.data.code !== 200) {
      throw new Error(response.data.msg || '记账失败')
    }
    return response.data.data
  }

  async function refresh({ period = 'month', customRange = [], filters = {} } = {}) {
    await Promise.all([
      loadSummary(period, customRange),
      loadEntries(filters)
    ])
  }

  return {
    summary: readonly(summary),
    entries: readonly(entries),
    total: readonly(total),
    loading: readonly(loading),
    ledgerLoading: readonly(ledgerLoading),
    accounts,
    netAmount,
    totalBalance,
    loadSummary,
    loadEntries,
    createEntry,
    refresh
  }
}
