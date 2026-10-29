<script setup>
import { computed, onMounted, ref } from 'vue'
import { EditPen, Plus, Refresh } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import BookkeepingEntryForm from '../components/bookkeeping/BookkeepingEntryForm.vue'
import BookkeepingLedger from '../components/bookkeeping/BookkeepingLedger.vue'
import BookkeepingSummary from '../components/bookkeeping/BookkeepingSummary.vue'
import { useBookkeeping } from '../composables/useBookkeeping'
import { downloadExcel } from '../utils/export'

const period = ref('month')
const customDateRange = ref([])
const mobileEntryOpen = ref(false)
const submitting = ref(false)
const resetToken = ref(0)
const ledgerFilters = ref(defaultLedgerFilters())

const {
  summary,
  entries,
  total,
  loading,
  ledgerLoading,
  accounts,
  fundAccounts,
  netAmount,
  totalBalance,
  loadSummary,
  loadEntries,
  createEntry,
  refresh
} = useBookkeeping()

const periodLabel = computed(() => ({
  today: '今日',
  month: '本月',
  year: '本年',
  all: '累计',
  custom: '所选期间'
})[period.value] || '本期')

function formatDate(date) {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function defaultLedgerFilters() {
  const now = new Date()
  const firstDay = new Date(now.getFullYear(), now.getMonth(), 1)
  return {
    start_date: formatDate(firstDay),
    end_date: formatDate(now),
    page: 1,
    page_size: 20
  }
}

function periodFilters(value) {
  const now = new Date()
  const baseFilters = {
    ...ledgerFilters.value,
    account_id: ledgerFilters.value.account_id || '',
    flow_type: ledgerFilters.value.flow_type || '',
    source_type: ledgerFilters.value.source_type || '',
    keyword: ledgerFilters.value.keyword || '',
    page: 1,
    page_size: ledgerFilters.value.page_size || 20
  }
  delete baseFilters.start_date
  delete baseFilters.end_date
  if (value === 'all') return baseFilters
  if (value === 'custom') {
    return {
      ...baseFilters,
      start_date: customDateRange.value?.[0],
      end_date: customDateRange.value?.[1]
    }
  }
  let start = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  if (value === 'month') start = new Date(now.getFullYear(), now.getMonth(), 1)
  if (value === 'year') start = new Date(now.getFullYear(), 0, 1)
  return { ...baseFilters, start_date: formatDate(start), end_date: formatDate(now) }
}

async function changePeriod() {
  if (period.value === 'custom' && customDateRange.value.length !== 2) return
  ledgerFilters.value = periodFilters(period.value)
  try {
    await refresh({
      period: period.value,
      customRange: customDateRange.value,
      filters: ledgerFilters.value
    })
  } catch {
    ElMessage.error('记账数据加载失败，请稍后重试')
  }
}

async function saveEntry(payload) {
  submitting.value = true
  try {
    await createEntry(payload)
    resetToken.value += 1
    mobileEntryOpen.value = false
    ElMessage.success(payload.entry_type === 1 ? '收入已入账' : '支出已入账')
    await refresh({
      period: period.value,
      customRange: customDateRange.value,
      filters: ledgerFilters.value
    })
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || error.message || '记账失败')
  } finally {
    submitting.value = false
  }
}

async function searchLedger(filters) {
  ledgerFilters.value = filters
  try {
    await loadEntries(filters)
  } catch {
    ElMessage.error('账目加载失败，请稍后重试')
  }
}

function exportLedger(filters) {
  downloadExcel('/api/finance/logs/export', filters)
}

async function reload() {
  try {
    await Promise.all([
      loadSummary(period.value, customDateRange.value),
      loadEntries(ledgerFilters.value)
    ])
  } catch {
    ElMessage.error('刷新失败，请稍后重试')
  }
}

onMounted(changePeriod)
</script>

<template>
  <main class="bookkeeping-page">
    <header class="page-header">
      <div>
        <div class="page-kicker">BOOKKEEPING</div>
        <h1>专业记账</h1>
        <p>快速录入每笔收支，统一核对业务自动流水与手工账目。</p>
      </div>
      <div class="header-actions">
        <el-button @click="$router.push('/admin/ai-import?kind=expense')">拍照记账</el-button>
        <el-select v-model="period" class="period-select" @change="changePeriod">
          <el-option label="今日" value="today" /><el-option label="本月" value="month" /><el-option label="本年" value="year" /><el-option label="累计" value="all" /><el-option label="自定义" value="custom" />
        </el-select>
        <el-button :icon="Refresh" :loading="loading || ledgerLoading" @click="reload">刷新</el-button>
      </div>
    </header>

    <div v-if="period === 'custom'" class="custom-period">
      <span>统计期间</span>
      <el-date-picker v-model="customDateRange" type="daterange" value-format="YYYY-MM-DD" start-placeholder="开始日期" end-placeholder="结束日期" @change="changePeriod" />
    </div>

    <!-- 分账提示：工程开销独立成账，这里不提供入口，避免两边重复记账（SRS R11） -->
    <div class="engineering-expense-hint">
      <span class="hint-dot" aria-hidden="true"></span>
      <span>工程开销请到「<router-link to="/admin/engineering/expenses">工程 → 工程开销</router-link>」记录，避免两边重复记账。</span>
    </div>

    <BookkeepingSummary :summary="summary" :net-amount="netAmount" :total-balance="totalBalance" :fund-account-count="fundAccounts.length" :period-label="periodLabel" :loading="loading" />

    <section class="desktop-entry-card">
      <div class="entry-card-copy"><div class="entry-card-icon"><el-icon><EditPen /></el-icon></div><div><span>快速录入</span><h2>记一笔新账</h2><p>金额和分类优先，其他信息按需补充。</p></div></div>
      <BookkeepingEntryForm :accounts="accounts" :submitting="submitting" :reset-token="resetToken" @submit="saveEntry" />
    </section>

    <BookkeepingLedger :accounts="accounts" :rows="entries" :total="total" :loading="ledgerLoading" :initial-filters="ledgerFilters" @search="searchLedger" @export="exportLedger" />

    <button type="button" class="mobile-add-button" @click="mobileEntryOpen = true"><el-icon><Plus /></el-icon><span>记一笔</span></button>
    <el-drawer v-model="mobileEntryOpen" direction="btt" size="88%" :with-header="false" class="bookkeeping-drawer" destroy-on-close>
      <div class="mobile-entry-sheet">
        <div class="sheet-handle"></div>
        <div class="sheet-heading"><div><span>快速记账</span><h2>记一笔新账</h2></div><button type="button" @click="mobileEntryOpen = false">关闭</button></div>
        <BookkeepingEntryForm mobile :accounts="accounts" :submitting="submitting" :reset-token="resetToken" @submit="saveEntry" />
      </div>
    </el-drawer>
  </main>
</template>

<style scoped>
.bookkeeping-page { display: grid; width: 100%; max-width: 1320px; min-width: 0; min-height: 100%; margin: 0 auto; padding: 28px; gap: 18px; overflow-x: clip; box-sizing: border-box; background: #f8fafc; }
.page-header { display: flex; align-items: center; justify-content: space-between; gap: 20px; }.page-kicker { color: #635bff; font-size: 11px; font-weight: 900; letter-spacing: .15em; }.page-header h1 { margin: 4px 0; color: #0f172a; font-size: 29px; letter-spacing: -.04em; }.page-header p { margin: 0; color: #64748b; font-size: 13px; }.header-actions { display: flex; gap: 8px; }.period-select { width: 112px; }.custom-period { display: flex; align-items: center; justify-content: flex-end; gap: 10px; color: #64748b; font-size: 12px; }
.engineering-expense-hint { display: flex; align-items: center; gap: 8px; padding: 9px 13px; border: 1px solid #e2e8f0; border-radius: 12px; color: #475569; background: #f8fafc; font-size: 12px; line-height: 1.5; }
.engineering-expense-hint .hint-dot { flex: 0 0 auto; width: 6px; height: 6px; border-radius: 999px; background: #635bff; }
.engineering-expense-hint a { color: #635bff; font-weight: 700; text-decoration: none; }
.engineering-expense-hint a:hover { text-decoration: underline; }
.desktop-entry-card { display: grid; grid-template-columns: 220px minmax(0, 1fr); gap: 24px; padding: 22px; border: 1px solid #dcdaff; border-radius: 20px; background: linear-gradient(120deg, #eef0ff, #fff 42%); box-shadow: 0 12px 26px rgba(15, 23, 42, .05); }.entry-card-copy { display: flex; align-items: flex-start; gap: 11px; }.entry-card-icon { display: grid; flex: 0 0 40px; width: 40px; height: 40px; place-items: center; border-radius: 13px; color: #fff; background: #635bff; }.entry-card-copy span { color: #635bff; font-size: 10px; font-weight: 900; letter-spacing: .1em; }.entry-card-copy h2 { margin: 3px 0; color: #0f172a; font-size: 18px; }.entry-card-copy p { margin: 0; color: #64748b; font-size: 11px; line-height: 1.5; }.mobile-add-button { display: none; }.mobile-entry-sheet { padding: 8px 16px max(16px, env(safe-area-inset-bottom)); }.sheet-handle { width: 42px; height: 5px; margin: 0 auto 12px; border-radius: 999px; background: #cbd5e1; }.sheet-heading { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }.sheet-heading span { color: #635bff; font-size: 10px; font-weight: 900; letter-spacing: .1em; }.sheet-heading h2 { margin: 2px 0 0; color: #0f172a; font-size: 19px; }.sheet-heading button { border: 0; background: transparent; color: #64748b; font-size: 13px; }
@media (max-width: 880px) { .desktop-entry-card { grid-template-columns: 1fr; } }
@media (max-width: 620px) { .bookkeeping-page { padding: 15px 12px 94px; gap: 13px; }.page-header { min-width: 0; align-items: flex-start; flex-direction: column; gap: 11px; }.page-header > div:first-child { min-width: 0; max-width: 100%; }.page-header h1 { font-size: 24px; }.page-header p { max-width: 100%; font-size: 11px; white-space: normal; }.header-actions { width: 100%; min-width: 0; }.header-actions :deep(.el-button) { margin-left: 0; padding-inline: 12px; }.period-select { min-width: 0; flex: 1; }.custom-period { min-width: 0; align-items: flex-start; flex-direction: column; }.custom-period :deep(.el-date-editor) { width: 100%; min-width: 0; }.desktop-entry-card { display: none; }.mobile-add-button { position: fixed; right: 18px; bottom: max(20px, env(safe-area-inset-bottom)); z-index: 40; display: inline-flex; min-height: 52px; align-items: center; gap: 7px; padding: 0 20px; border: 0; border-radius: 999px; color: #fff; background: #635bff; font-size: 14px; font-weight: 900; box-shadow: 0 14px 30px rgba(37, 99, 235, .34); }.mobile-entry-sheet { width: 100%; height: 100%; min-width: 0; padding-inline: 16px; overflow-x: hidden; overflow-y: auto; box-sizing: border-box; } }
</style>

<style>
.bookkeeping-drawer { width: 100% !important; max-width: 100vw; border-radius: 22px 22px 0 0; overflow: hidden; }
.bookkeeping-drawer .el-drawer__body { min-width: 0; padding: 0; overflow-x: hidden; }
</style>
