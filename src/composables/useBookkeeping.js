import { computed, readonly, ref } from 'vue'
import { fetchFinanceSummary, fetchFinanceLogs, createBookkeepingEntry } from '../api/finance'

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
// 台账账户（经营开销 / 历史调整台账）只用于记账口径，余额是负的累计支出，不代表真实资金。
// 账户总余额必须排除它们；对外展示「N 个资金账户」时也要用同一个口径，
// 否则会出现"写 5 个账户、实际只加 4 个余额"的不一致。
const LEDGER_ACCOUNT_NAMES = ['经营开销', '历史调整台账']
const fundAccounts = computed(() =>
  accounts.value.filter((account) => !LEDGER_ACCOUNT_NAMES.includes(account.name))
)
const netAmount = computed(() =>
  Number(summary.value.total_income || 0) - Number(summary.value.total_expense || 0)
)
const totalBalance = computed(() =>
  fundAccounts.value.reduce((sum, account) => sum + Number(account.balance || 0), 0)
)

  async function loadSummary(period = 'month', customRange = []) {
    loading.value = true
    try {
      const params = { period }
      if (period === 'custom') {
        params.start_date = customRange[0]
        params.end_date = customRange[1]
      }
      const response = await fetchFinanceSummary(params)
      summary.value = response.data.code === 200 ? response.data.data : emptySummary
    } finally {
      loading.value = false
    }
  }

  async function loadEntries(filters = {}) {
    ledgerLoading.value = true
    try {
      const response = await fetchFinanceLogs(filters)
      entries.value = response.data.data || []
      total.value = Number(response.data.total || 0)
    } finally {
      ledgerLoading.value = false
    }
  }

  async function createEntry(payload) {
    const response = await createBookkeepingEntry(payload)
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
    fundAccounts,
    netAmount,
    totalBalance,
    loadSummary,
    loadEntries,
    createEntry,
    refresh
  }
}
