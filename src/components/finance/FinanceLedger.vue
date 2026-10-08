<script setup>
import { computed, reactive, watch } from 'vue'
import { Download, Search } from '@element-plus/icons-vue'

const props = defineProps({ accounts: { type: Array, default: () => [] }, rows: { type: Array, default: () => [] }, total: { type: Number, default: 0 }, loading: Boolean })
const emit = defineEmits(['search', 'export'])
const filters = reactive({ account_id: '', flow_type: '', keyword: '', date_range: [], page: 1, page_size: 20 })
const pageCount = computed(() => Math.max(1, Math.ceil(props.total / filters.page_size)))

function submit() { filters.page = 1; emit('search', toQuery()) }
function reset() { Object.assign(filters, { account_id: '', flow_type: '', keyword: '', date_range: [], page: 1 }); emit('search', toQuery()) }
function toQuery() { return { account_id: filters.account_id, flow_type: filters.flow_type, keyword: filters.keyword, start_date: filters.date_range?.[0], end_date: filters.date_range?.[1], page: filters.page, page_size: filters.page_size } }
function changePage(page) { filters.page = page; emit('search', toQuery()) }
function formatCurrency(value) { return `¥${Number(value || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}` }
function formatDate(value) { return value ? new Date(value).toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '-' }
function flowName(type) { return ({ 1: '收入', 2: '支出', 3: '调拨', 4: '校准' })[type] || '未知' }
function flowTag(type) { return ({ 1: 'success', 2: 'danger', 3: 'warning', 4: 'info' })[type] || 'info' }
function amountClass(type) { return type === 1 ? 'income' : type === 2 ? 'expense' : '' }
function amountPrefix(type) { return type === 1 ? '+' : type === 2 ? '-' : '' }
watch(() => filters.page_size, submit)
</script>

<template>
  <section class="ledger-card">
    <div class="ledger-heading"><div><span>资金流水</span><h3>每一笔资金变动都可追溯</h3></div><el-button plain :icon="Download" @click="emit('export', toQuery())">导出</el-button></div>
    <div class="ledger-filters">
      <el-select v-model="filters.account_id" clearable placeholder="全部账户"><el-option v-for="account in accounts" :key="account.id" :label="account.name" :value="account.id" /></el-select>
      <el-select v-model="filters.flow_type" clearable placeholder="全部流向"><el-option label="收入" :value="1" /><el-option label="支出" :value="2" /><el-option label="内部调拨" :value="3" /><el-option label="系统校准" :value="4" /></el-select>
      <el-date-picker v-model="filters.date_range" type="daterange" value-format="YYYY-MM-DD" start-placeholder="开始日期" end-placeholder="结束日期" />
      <el-input v-model="filters.keyword" clearable placeholder="分类、单号或备注" @keyup.enter="submit"><template #prefix><el-icon><Search /></el-icon></template></el-input>
      <el-button type="primary" @click="submit">筛选</el-button><el-button text @click="reset">重置</el-button>
    </div>
    <el-table :data="rows" v-loading="loading" class="ledger-table desktop-ledger" empty-text="没有符合条件的流水">
      <el-table-column label="发生时间" width="170"><template #default="{ row }">{{ formatDate(row.created_at) }}</template></el-table-column>
      <el-table-column label="资金变动" width="110" align="center"><template #default="{ row }"><el-tag :type="flowTag(row.flow_type)" effect="plain">{{ flowName(row.flow_type) }}</el-tag></template></el-table-column>
      <el-table-column prop="category" label="分类" min-width="130" /><el-table-column label="账户" width="130"><template #default="{ row }">{{ row.account?.name || '-' }}</template></el-table-column>
      <el-table-column label="金额" width="145" align="right"><template #default="{ row }"><b :class="amountClass(row.flow_type)">{{ amountPrefix(row.flow_type) }}{{ formatCurrency(row.amount) }}</b></template></el-table-column>
      <el-table-column label="关联信息" min-width="210"><template #default="{ row }"><div class="related-cell"><span>{{ row.remark || '-' }}</span><small v-if="row.related_no">{{ row.related_no }}</small></div></template></el-table-column>
    </el-table>
    <div v-loading="loading" class="mobile-ledger">
      <article v-for="row in rows" :key="row.id" class="ledger-item">
        <div class="ledger-item-top"><el-tag :type="flowTag(row.flow_type)" effect="plain" size="small">{{ flowName(row.flow_type) }}</el-tag><strong :class="amountClass(row.flow_type)">{{ amountPrefix(row.flow_type) }}{{ formatCurrency(row.amount) }}</strong></div>
        <div class="ledger-item-main"><b>{{ row.category }}</b><span>{{ row.account?.name || '未关联账户' }}</span></div>
        <p>{{ row.remark || '无备注' }}</p>
        <div class="ledger-item-meta"><span>{{ formatDate(row.created_at) }}</span><span v-if="row.related_no">{{ row.related_no }}</span></div>
      </article>
      <el-empty v-if="!loading && !rows.length" description="没有符合条件的流水" :image-size="64" />
    </div>
    <div class="ledger-footer"><span>共 {{ total }} 条记录</span><el-pagination v-if="total > filters.page_size" background layout="prev, pager, next" :current-page="filters.page" :page-count="pageCount" @current-change="changePage" /></div>
  </section>
</template>

<style scoped>
.ledger-card { padding: 20px; background: #fff; border: 1px solid #e8edf5; border-radius: 18px; }.ledger-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }.ledger-heading span { color: #64748b; font-size: 12px; }.ledger-heading h3 { margin: 3px 0 0; color: #1e293b; font-size: 16px; }.ledger-filters { display: grid; grid-template-columns: 140px 130px minmax(230px, 1fr) minmax(180px, 1fr) auto auto; gap: 8px; margin: 18px 0; }.ledger-table { width: 100%; }.mobile-ledger { display: none; }.related-cell { display: grid; gap: 3px; color: #475569; }.related-cell small { color: #94a3b8; font-family: monospace; }.income { color: #059669; }.expense { color: #e11d48; }.ledger-footer { display: flex; justify-content: space-between; align-items: center; margin-top: 16px; color: #94a3b8; font-size: 12px; } @media (max-width: 960px) { .ledger-filters { grid-template-columns: repeat(2, minmax(0, 1fr)); }.ledger-filters > :nth-last-child(-n+2) { width: fit-content; } } @media (max-width: 560px) { .ledger-card { padding: 15px; border-radius: 16px; }.ledger-heading h3 { font-size: 15px; }.ledger-filters { grid-template-columns: 1fr; margin: 15px 0; }.ledger-filters :deep(.el-date-editor) { width: 100%; }.ledger-filters > :nth-last-child(-n+2) { width: 100%; }.ledger-filters > :nth-last-child(-n+2) :deep(.el-button) { width: 100%; }.desktop-ledger { display: none; }.mobile-ledger { display: grid; gap: 9px; }.ledger-item { padding: 13px; border: 1px solid #e8edf5; border-radius: 14px; }.ledger-item-top, .ledger-item-main, .ledger-item-meta { display: flex; align-items: center; justify-content: space-between; gap: 10px; }.ledger-item-top strong { font-size: 16px; }.ledger-item-main { margin-top: 11px; color: #334155; }.ledger-item-main b { font-size: 14px; }.ledger-item-main span, .ledger-item-meta { color: #94a3b8; font-size: 11px; }.ledger-item p { margin: 7px 0 10px; color: #64748b; font-size: 12px; line-height: 1.45; }.ledger-item-meta { justify-content: flex-start; flex-wrap: wrap; }.ledger-footer { align-items: flex-start; flex-direction: column; gap: 10px; } }
@media (min-width: 561px) and (max-width: 1279px) {
  .desktop-ledger { display: none; }
  .mobile-ledger { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
  .ledger-item { min-width: 0; padding: 13px; border: 1px solid #e8edf5; border-radius: 14px; }
  .ledger-item-top, .ledger-item-main, .ledger-item-meta { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  .ledger-item-main { margin-top: 11px; color: #334155; }
  .ledger-item-main span, .ledger-item-meta { color: #94a3b8; font-size: 11px; }
  .ledger-item p { margin: 7px 0 10px; color: #64748b; font-size: 12px; }
  .ledger-item-meta { justify-content: flex-start; flex-wrap: wrap; }
}
</style>
