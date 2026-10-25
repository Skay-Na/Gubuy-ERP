import { computed, readonly, ref } from 'vue'
import request from '../utils/request'

const emptySummary = {
  accounts: [],
  pending_balance: 0,
  pending_balance_count: 0,
  pending_subsidy: 0,
  total_income: 0,
  total_expense: 0
}

export function useFinanceWorkspace() {
  const summary = ref(emptySummary)
  const ledger = ref([])
  const ledgerTotal = ref(0)
  const loading = ref(false)
  const ledgerLoading = ref(false)

  const totalBalance = computed(() =>
    (summary.value.accounts || [])
      .filter((account) => account.name !== '经营开销')
      .reduce((total, account) => total + Number(account.balance || 0), 0)
  )
  const netCashFlow = computed(() => Number(summary.value.total_income || 0) - Number(summary.value.total_expense || 0))
  const attentionCount = computed(() =>
    Number(summary.value.pending_balance_count || 0) + (Number(summary.value.pending_subsidy || 0) > 0 ? 1 : 0)
  )

  async function loadWorkspace(period) {
    loading.value = true
    try {
      const response = await request.get('/api/finance/summary', { params: { period } })
      summary.value = response.data.code === 200 ? response.data.data : emptySummary
    } finally {
      loading.value = false
    }
  }

  async function loadLedger(filters = {}) {
    ledgerLoading.value = true
    try {
      const response = await request.get('/api/finance/logs', { params: filters })
      ledger.value = response.data.data || []
      ledgerTotal.value = Number(response.data.total || 0)
    } finally {
      ledgerLoading.value = false
    }
  }

  async function refresh(period, filters) {
    await Promise.all([loadWorkspace(period), loadLedger(filters)])
  }

  return {
    summary: readonly(summary),
    ledger: readonly(ledger),
    ledgerTotal: readonly(ledgerTotal),
    loading: readonly(loading),
    ledgerLoading: readonly(ledgerLoading),
    totalBalance,
    netCashFlow,
    attentionCount,
    loadWorkspace,
    loadLedger,
    refresh
  }
}
