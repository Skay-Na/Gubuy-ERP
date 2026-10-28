<script setup>
import { computed, reactive, watch } from 'vue'
import { Download, Search } from '@element-plus/icons-vue'

const props = defineProps({
  accounts: { type: Array, default: () => [] },
  rows: { type: Array, default: () => [] },
  total: { type: Number, default: 0 },
  loading: Boolean,
  initialFilters: { type: Object, default: () => ({}) }
})

const emit = defineEmits(['search', 'export'])
const filters = reactive({
  account_id: '',
  flow_type: '',
  source_type: '',
  keyword: '',
  date_range: [],
  page: 1,
  page_size: 20
})

const pageCount = computed(() => Math.max(1, Math.ceil(props.total / filters.page_size)))

watch(
  () => props.initialFilters,
  (value) => {
    if (!value) return
    filters.account_id = value.account_id || ''
    filters.flow_type = value.flow_type || ''
    filters.source_type = value.source_type || ''
    filters.keyword = value.keyword || ''
    filters.page = Number(value.page) > 0 ? Number(value.page) : 1
    filters.page_size = Number(value.page_size) > 0 ? Number(value.page_size) : 20
    filters.date_range = value.start_date || value.end_date
      ? [value.start_date || '', value.end_date || '']
      : []
  },
  { immediate: true, deep: true }
)

watch(() => filters.page_size, () => submit())

function toQuery() {
  return {
    account_id: filters.account_id || undefined,
    flow_type: filters.flow_type || undefined,
    source_type: filters.source_type || undefined,
    keyword: filters.keyword.trim() || undefined,
    start_date: filters.date_range?.[0] || undefined,
    end_date: filters.date_range?.[1] || undefined,
    page: filters.page,
    page_size: filters.page_size
  }
}

function submit() {
  filters.page = 1
  emit('search', toQuery())
}

function reset() {
  Object.assign(filters, {
    account_id: '',
    flow_type: '',
    source_type: '',
    keyword: '',
    date_range: [],
    page: 1
  })
  emit('search', toQuery())
}

function changePage(page) {
  filters.page = page
  emit('search', toQuery())
}

function formatCurrency(value) {
  return `¥${Number(value || 0).toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })}`
}

function formatDate(value) {
  return value
    ? new Date(value).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
    : '-'
}

function flowName(type) {
  return ({ 1: '收入', 2: '支出', 3: '调拨', 4: '校准' })[type] || '其他'
}

function flowTag(type) {
  return ({ 1: 'success', 2: 'danger', 3: 'warning', 4: 'info' })[type] || 'info'
}

function amountClass(type) {
  return type === 1 ? 'income' : type === 2 ? 'expense' : 'neutral'
}

function amountPrefix(type) {
  return type === 1 ? '+' : type === 2 ? '-' : ''
}
</script>

<template>
  <section class="ledger-card">
    <div class="ledger-heading">
      <div><span>账目明细</span><h2>所有资金变动，一处核对</h2></div>
      <el-button plain :icon="Download" @click="emit('export', toQuery())">导出明细</el-button>
    </div>

    <div class="ledger-filters">
      <el-select v-model="filters.flow_type" clearable placeholder="全部收支"><el-option label="收入" :value="1" /><el-option label="支出" :value="2" /><el-option label="内部调拨" :value="3" /><el-option label="余额校准" :value="4" /></el-select>
      <el-select v-model="filters.account_id" clearable placeholder="全部账户"><el-option v-for="account in accounts" :key="account.id" :label="account.name" :value="account.id" /></el-select>
      <el-select v-model="filters.source_type" clearable placeholder="全部来源"><el-option label="手工记账" value="manual" /><el-option label="业务自动生成" value="system" /></el-select>
      <el-date-picker v-model="filters.date_range" type="daterange" value-format="YYYY-MM-DD" start-placeholder="开始日期" end-placeholder="结束日期" />
      <el-input v-model="filters.keyword" clearable placeholder="分类、往来对象、单号、备注" @keyup.enter="submit"><template #prefix><el-icon><Search /></el-icon></template></el-input>
      <el-button type="primary" @click="submit">查询</el-button>
      <el-button text @click="reset">重置</el-button>
    </div>

    <el-table :data="rows" v-loading="loading" class="desktop-ledger" empty-text="没有符合条件的账目">
      <el-table-column label="时间" width="125"><template #default="{ row }">{{ formatDate(row.created_at) }}</template></el-table-column>
      <el-table-column label="收支" width="80" align="center"><template #default="{ row }"><el-tag :type="flowTag(row.flow_type)" effect="plain">{{ flowName(row.flow_type) }}</el-tag></template></el-table-column>
      <el-table-column prop="category" label="分类" min-width="120" />
      <el-table-column label="金额" width="145" align="right"><template #default="{ row }"><b :class="amountClass(row.flow_type)">{{ amountPrefix(row.flow_type) }}{{ formatCurrency(row.amount) }}</b></template></el-table-column>
      <el-table-column label="账户 / 余额" min-width="150"><template #default="{ row }"><div class="stacked-cell"><b>{{ row.account?.name || '-' }}</b><small>余 {{ formatCurrency(row.balance_after) }}</small></div></template></el-table-column>
      <el-table-column label="往来 / 关联" min-width="170"><template #default="{ row }"><div class="stacked-cell"><span>{{ row.counterparty || row.remark || '-' }}</span><small v-if="row.related_no">{{ row.related_no }}</small></div></template></el-table-column>
      <el-table-column label="来源" width="100"><template #default="{ row }"><span class="source-pill" :class="row.source_type === 'manual' ? 'manual' : ''">{{ row.source_type === 'manual' ? '手工' : '自动' }}</span></template></el-table-column>
    </el-table>

    <div v-loading="loading" class="mobile-ledger">
      <article v-for="row in rows" :key="row.id" class="ledger-item">
        <div class="ledger-item-top"><div><el-tag :type="flowTag(row.flow_type)" effect="plain" size="small">{{ flowName(row.flow_type) }}</el-tag><span class="source-pill" :class="row.source_type === 'manual' ? 'manual' : ''">{{ row.source_type === 'manual' ? '手工' : '自动' }}</span></div><strong :class="amountClass(row.flow_type)">{{ amountPrefix(row.flow_type) }}{{ formatCurrency(row.amount) }}</strong></div>
        <div class="ledger-item-title"><b>{{ row.category }}</b><span>{{ row.account?.name || '未关联账户' }}</span></div>
        <p v-if="row.counterparty || row.remark">{{ row.counterparty ? `往来：${row.counterparty}` : row.remark }}</p>
        <div class="ledger-item-meta"><span>{{ formatDate(row.created_at) }}</span><span v-if="row.related_no">{{ row.related_no }}</span><span>余额 {{ formatCurrency(row.balance_after) }}</span></div>
      </article>
      <el-empty v-if="!loading && !rows.length" description="没有符合条件的账目" :image-size="64" />
    </div>

    <div class="ledger-footer">
      <span>共 {{ total }} 条记录</span>
      <el-pagination v-if="total > filters.page_size" background layout="prev, pager, next" :current-page="filters.page" :page-count="pageCount" @current-change="changePage" />
    </div>
  </section>
</template>

<style scoped>
.ledger-card { width: 100%; min-width: 0; padding: 20px; border: 1px solid #e2e8f0; border-radius: 20px; box-sizing: border-box; background: #fff; }
.ledger-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }.ledger-heading span { color: #635bff; font-size: 11px; font-weight: 900; letter-spacing: .12em; }.ledger-heading h2 { margin: 4px 0 0; color: #0f172a; font-size: 18px; }
.ledger-filters { display: grid; grid-template-columns: 110px 125px 135px minmax(210px, 1fr) minmax(200px, 1fr) auto auto; gap: 8px; margin: 18px 0; }.desktop-ledger { width: 100%; }.mobile-ledger { display: none; }.stacked-cell { display: grid; gap: 3px; }.stacked-cell b, .stacked-cell span { color: #475569; font-size: 13px; }.stacked-cell small { color: #94a3b8; font-size: 10px; }.income { color: #059669; }.expense { color: #e11d48; }.neutral { color: #475569; }.source-pill { display: inline-flex; padding: 3px 7px; border-radius: 999px; color: #64748b; background: #f1f5f9; font-size: 10px; font-weight: 800; }.source-pill.manual { color: #4b44c8; background: #e8e9ff; }.ledger-footer { display: flex; align-items: center; justify-content: space-between; margin-top: 16px; color: #94a3b8; font-size: 12px; }
@media (max-width: 1080px) { .ledger-filters { grid-template-columns: repeat(3, minmax(0, 1fr)); }.ledger-filters :deep(.el-date-editor) { width: 100%; } }
@media (max-width: 620px) { .ledger-card { padding: 15px; border-radius: 17px; overflow: hidden; }.ledger-heading { min-width: 0; }.ledger-heading > div { min-width: 0; }.ledger-heading h2 { font-size: 16px; white-space: normal; }.ledger-heading :deep(.el-button) { flex: 0 0 auto; padding-inline: 10px; }.ledger-filters { grid-template-columns: repeat(2, minmax(0, 1fr)); margin: 15px 0; }.ledger-filters > * { min-width: 0; }.ledger-filters > :nth-child(4), .ledger-filters > :nth-child(5) { grid-column: 1 / -1; }.ledger-filters > :nth-child(4) { width: 100%; min-width: 0; }.ledger-filters > :nth-child(6), .ledger-filters > :nth-child(7) { width: 100%; margin-left: 0; }.desktop-ledger { display: none; }.mobile-ledger { display: grid; min-width: 0; gap: 9px; }.ledger-item { min-width: 0; padding: 13px; border: 1px solid #e2e8f0; border-radius: 14px; background: #fff; }.ledger-item-top, .ledger-item-title, .ledger-item-meta { display: flex; min-width: 0; align-items: center; justify-content: space-between; gap: 9px; }.ledger-item-top > div { display: flex; gap: 6px; }.ledger-item-top strong { font-size: 17px; }.ledger-item-title { margin-top: 12px; color: #334155; }.ledger-item-title span { color: #64748b; font-size: 11px; }.ledger-item p { margin: 8px 0; color: #64748b; font-size: 12px; overflow-wrap: anywhere; }.ledger-item-meta { justify-content: flex-start; flex-wrap: wrap; color: #94a3b8; font-size: 10px; }.ledger-footer { min-width: 0; align-items: flex-start; flex-direction: column; gap: 10px; } }
@media (min-width: 621px) and (max-width: 1279px) {
  .desktop-ledger { display: none; }
  .mobile-ledger { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
  .ledger-item { min-width: 0; padding: 13px; border: 1px solid #e2e8f0; border-radius: 14px; background: #fff; }
  .ledger-item-top, .ledger-item-title, .ledger-item-meta { display: flex; align-items: center; justify-content: space-between; gap: 9px; }
  .ledger-item-top > div { display: flex; gap: 6px; }
  .ledger-item-title { margin-top: 12px; color: #334155; }
  .ledger-item p { margin: 8px 0; color: #64748b; font-size: 12px; }
  .ledger-item-meta { justify-content: flex-start; flex-wrap: wrap; color: #94a3b8; font-size: 10px; }
}
</style>
