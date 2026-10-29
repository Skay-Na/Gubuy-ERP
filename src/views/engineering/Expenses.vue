<template>
  <!--
    工程开销台账页（SPEC-ENG-001 §9.5，任务卡 T14）
    独立成账：与日常经营开销（/admin/bookkeeping 那边）是两套账，互不重复。
    汇总卡「本年工程开销」点击后带 date_start/date_end 直达本页。
  -->
  <div class="min-h-screen bg-slate-50/60">
    <div class="mx-auto w-full max-w-[1440px] px-4 py-6 md:px-8 md:py-8">
      <!-- 页头 -->
      <header class="mb-5 flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div>
          <button type="button" class="eng-back" @click="goBackList">← 工程列表</button>
          <h1 class="mt-2 text-2xl font-black text-slate-900 md:text-3xl">工程开销</h1>
          <p class="mt-1 text-sm text-slate-400">全部工程的实际支出，与日常经营开销分开记账</p>
        </div>
        <button
          type="button"
          class="inline-flex h-10 items-center justify-center rounded-xl px-4 text-sm font-medium text-white hover:opacity-90"
          style="background: #635bff"
          @click="openCreate"
        >
          + 录入费用
        </button>
      </header>

      <!-- 汇总条 + 类别分布 -->
      <EngineeringExpenseSummary
        :summary="summary"
        :by-category="byCategory"
        :show-year="true"
        :active-category="activeCategory"
        class="mb-4"
        @pick-category="onPickCategory"
      />

      <!-- 筛选条 -->
      <div class="eng-filter">
        <el-input
          :model-value="filters.keyword"
          class="eng-filter-keyword"
          placeholder="搜项目名 / 收款方 / 备注"
          clearable
          maxlength="50"
          @update:model-value="v => (filters.keyword = v || '')"
        />
        <el-select
          :model-value="filters.projectId"
          class="eng-filter-project"
          placeholder="项目"
          clearable
          filterable
          @update:model-value="v => (filters.projectId = v === '' || v === undefined ? null : v)"
        >
          <el-option label="公共费用（不分摊）" value="none" />
          <el-option v-for="p in projectOptions" :key="p.id" :label="`${p.project_no} ${p.name}`" :value="p.id" />
        </el-select>
        <el-select
          :model-value="filters.payStatus"
          class="eng-filter-pay"
          placeholder="付款状态"
          clearable
          @update:model-value="v => (filters.payStatus = v === '' || v === undefined ? null : v)"
        >
          <el-option label="已付" :value="1" />
          <el-option label="未付" :value="0" />
        </el-select>
        <el-date-picker
          :model-value="filters.dateRange"
          class="eng-filter-date"
          type="daterange"
          value-format="YYYY-MM-DD"
          range-separator="至"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          unlink-panels
          @update:model-value="v => (filters.dateRange = v || [])"
        />
        <el-button text class="eng-filter-reset" @click="resetFilters">重置</el-button>
      </div>

      <!-- 明细表 -->
      <div v-if="!isMobile" class="eng-table-card">
        <el-table v-loading="loading" :data="list" stripe class="eng-table">
          <el-table-column label="发生日期" width="100">
            <template #default="{ row }">{{ formatEngDate(row.expense_date) }}</template>
          </el-table-column>
          <el-table-column label="类别" width="120">
            <template #default="{ row }">
              <EngineeringExpenseTag :category="row.category" />
            </template>
          </el-table-column>
          <el-table-column label="所属项目" width="210" show-overflow-tooltip>
            <template #default="{ row }">
              <button
                v-if="row.project_id"
                type="button"
                class="eng-project-link"
                @click="goDetail(row.project_id)"
              >
                {{ row.project_name }}
              </button>
              <span v-else class="eng-public-chip">公共费用</span>
            </template>
          </el-table-column>
          <el-table-column label="金额" width="120" align="right">
            <template #default="{ row }">
              <span class="eng-money eng-money--strong">{{ formatEngMoney(row.amount) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="付款状态" width="84" align="center">
            <template #default="{ row }">
              <span class="eng-pay-tag" :style="payStyle(row.pay_status)">
                {{ row.pay_status_label || (row.pay_status === 1 ? '已付' : '未付') }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="付款方式" width="80">
            <template #default="{ row }">{{ row.pay_status === 1 ? row.pay_method_label || '—' : '—' }}</template>
          </el-table-column>
          <el-table-column label="收款方" width="88">
            <template #default="{ row }">{{ row.payee || '—' }}</template>
          </el-table-column>
          <el-table-column label="操作人" width="72">
            <template #default="{ row }">{{ row.operator_name || '—' }}</template>
          </el-table-column>
          <el-table-column label="录入时间" width="90">
            <template #default="{ row }">{{ shortTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="120" align="right">
            <template #default="{ row }">
              <el-button
                link
                type="primary"
                size="small"
                :disabled="!canEdit(row)"
                @click="openEdit(row)"
              >
                编辑
              </el-button>
              <el-button link type="danger" size="small" :disabled="!canEdit(row)" @click="confirmDelete(row)">
                删除
              </el-button>
            </template>
          </el-table-column>

          <template #empty>
            <div class="eng-empty">
              <p class="eng-empty-title">{{ hasActiveFilter ? '没有符合条件的费用' : '还没有工程费用记录' }}</p>
              <p class="eng-empty-sub">
                {{ hasActiveFilter ? '试试放宽筛选条件' : '安装、运输、辅材这些实际支出记在这里' }}
              </p>
              <el-button v-if="hasActiveFilter" class="mt-3" size="small" @click="resetFilters">清空筛选</el-button>
              <el-button v-else type="primary" class="mt-3" size="small" @click="openCreate">录入第一笔费用</el-button>
            </div>
          </template>
        </el-table>
        <div class="eng-table-foot">
          共 {{ total }} 笔 · 合计 <b>{{ formatEngMoney(summary.total) }}</b>
        </div>
      </div>

      <!-- 移动端卡片流 -->
      <div v-else class="eng-mobile-list">
        <div v-for="row in list" :key="row.id" class="eng-mobile-card" @click="canEdit(row) ? openEdit(row) : null">
          <div class="eng-mobile-head">
            <EngineeringExpenseTag :category="row.category" />
            <span class="eng-money eng-money--strong">{{ formatEngMoney(row.amount) }}</span>
          </div>
          <p class="eng-mobile-project">
            <template v-if="row.project_id">{{ row.project_name }}</template>
            <span v-else class="eng-public-chip">公共费用</span>
          </p>
          <p class="eng-mobile-sub">
            {{ formatEngDate(row.expense_date) }} · {{ row.pay_status_label || (row.pay_status === 1 ? '已付' : '未付') }}
            <template v-if="row.pay_status === 1"> · {{ row.pay_method_label }}</template>
            <template v-if="row.payee"> · {{ row.payee }}</template>
          </p>
        </div>
        <div v-if="firstLoaded && !list.length" class="eng-mobile-empty">
          {{ hasActiveFilter ? '没有符合条件的费用' : '还没有工程费用记录' }}
          <el-button v-if="!hasActiveFilter" type="primary" size="small" class="mt-2" @click="openCreate">
            录入第一笔费用
          </el-button>
          <el-button v-else size="small" class="mt-2" @click="resetFilters">清空筛选</el-button>
        </div>
        <div v-if="total > list.length" class="eng-mobile-more">
          <el-button size="small" :loading="loadingMore" @click="loadMore">
            加载更多（{{ list.length }}/{{ total }}）
          </el-button>
        </div>
      </div>

      <!-- PC 分页 -->
      <div v-if="!isMobile && total > 0" class="mt-3 flex justify-end">
        <el-pagination
          :current-page="page"
          :page-size="pageSize"
          :page-sizes="[20, 50, 100]"
          :total="total"
          layout="total, sizes, prev, pager, next"
          background
          @current-change="setPage"
          @size-change="v => (pageSize = v)"
        />
      </div>

      <EngineeringExpenseDialog
        v-model="dialogVisible"
        :project-id="null"
        :project-options="projectOptions"
        :expense="editingExpense"
        @saved="onSaved"
      />
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteEngineeringExpense } from '../../api/engineering'
import { useEngineeringExpenses } from '../../composables/useEngineeringExpenses'
import { formatEngDate, formatEngMoney } from '../../utils/engineeringFormat'
import EngineeringExpenseSummary from '../../components/engineering/EngineeringExpenseSummary.vue'
import EngineeringExpenseTag from '../../components/engineering/EngineeringExpenseTag.vue'
import EngineeringExpenseDialog from '../../components/engineering/EngineeringExpenseDialog.vue'

const router = useRouter()

const {
  filters,
  list,
  total,
  summary,
  byCategory,
  page,
  pageSize,
  loading,
  firstLoaded,
  projectOptions,
  isMobile,
  hasActiveFilter,
  loadingMore,
  init,
  load,
  loadMore,
  resetFilters,
  setPage,
  toggleCategory
} = useEngineeringExpenses()

// 类别 chip 与筛选多选联动：chip 选中态直接反映 filters.categories
const activeCategory = computed(() => (filters.categories.length === 1 ? filters.categories[0] : null))
const onPickCategory = category => toggleCategory(category)

const goBackList = () => router.push('/admin/engineering')
const goDetail = id => router.push(`/admin/engineering/${id}`)

const payStyle = status =>
  Number(status) === 1 ? { color: '#15803d', background: '#e8f7ee' } : { color: '#d97706', background: '#fffbeb' }

const shortTime = value => {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '—'
  const pad = n => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// 已作废项目的费用不允许改（后端会拦，前端先给出不可点状态）
const canEdit = row => true

const dialogVisible = ref(false)
const editingExpense = ref(null)

const openCreate = () => {
  editingExpense.value = null
  dialogVisible.value = true
}

const openEdit = row => {
  editingExpense.value = row
  dialogVisible.value = true
}

const onSaved = () => {
  dialogVisible.value = false
  load()
}

const confirmDelete = async row => {
  const label = row.project_id ? row.project_name : '公共费用'
  try {
    await ElMessageBox.confirm(
      `删除这笔 ${formatEngMoney(row.amount)} 的${row.category_label || '费用'}（${label}）？删除后项目净利会重算，操作记录可追溯。`,
      '删除工程费用',
      { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消', confirmButtonClass: 'el-button--danger' }
    )
  } catch (e) {
    return
  }
  try {
    await deleteEngineeringExpense(row.id)
    ElMessage.success('费用已删除')
    load()
  } catch (e) {
    /* 拦截器已提示 */
  }
}

// 移动端"加载更多"交给 composable 追加（这里不再改 page，避免覆盖已加载数据）

onMounted(init)
</script>

<style scoped>
.eng-back {
  font-size: 13px;
  color: #64748b;
  background: none;
  border: none;
  padding: 0;
  cursor: pointer;
}
.eng-back:hover {
  color: #635bff;
}
.eng-filter {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 12px 14px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.eng-filter-keyword {
  flex: 1 1 220px;
  min-width: 220px;
}
.eng-filter-project {
  width: 200px;
}
.eng-filter-pay {
  width: 130px;
}
.eng-filter-date {
  width: 260px;
}
.eng-filter-reset {
  margin-left: auto;
  color: #64748b;
}
.eng-table-card {
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  background: #fff;
  overflow: hidden;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.eng-table {
  font-size: 13px;
}
/* Element Plus 单元格左右各 12px padding，11 列会白白吃掉 260px，
   工程表格列多，统一收到 8px，给数据列留出可读宽度 */
.eng-table :deep(.el-table__cell .cell) {
  padding-left: 8px;
  padding-right: 8px;
}

/* 行内两个链接按钮默认带 12px 间距，会把窄列挤到换行，这里收紧 */
.eng-table :deep(.el-button + .el-button) {
  margin-left: 4px;
}
.eng-table :deep(.el-button.is-link) {
  padding: 0 1px;
  font-size: 12px;
}
/* 末列（操作）右对齐时文字会贴到卡片边框，留 14px 呼吸位 */
.eng-table :deep(.el-table__cell:last-child .cell) {
  padding-right: 14px;
}
.eng-table-foot {
  border-top: 1px solid #f8fafc;
  padding: 10px 14px;
  text-align: right;
  font-size: 12px;
  color: #64748b;
}
.eng-table-foot b {
  color: #0f172a;
  font-size: 13px;
}
.eng-project-link {
  border: none;
  background: none;
  padding: 0;
  font-size: 13px;
  color: #635bff;
  cursor: pointer;
  text-align: left;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.eng-project-link:hover {
  text-decoration: underline;
}
.eng-public-chip {
  display: inline-flex;
  border-radius: 999px;
  border: 1px solid #e2e8f0;
  padding: 1px 8px;
  font-size: 11px;
  color: #94a3b8;
  background: #fff;
}
.eng-money {
  font-variant-numeric: tabular-nums;
  color: #334155;
}
.eng-money--strong {
  font-weight: 600;
  color: #0f172a;
}
.eng-pay-tag {
  display: inline-flex;
  border-radius: 999px;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  line-height: 18px;
}
.eng-empty {
  padding: 34px 12px;
  text-align: center;
}
.eng-empty-title {
  font-size: 14px;
  font-weight: 600;
  color: #334155;
}
.eng-empty-sub {
  margin-top: 4px;
  font-size: 12px;
  color: #94a3b8;
}
.eng-mobile-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.eng-mobile-card {
  border-radius: 14px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 12px 14px;
  cursor: pointer;
}
.eng-mobile-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.eng-mobile-project {
  margin-top: 6px;
  font-size: 12px;
  color: #475569;
}
.eng-mobile-sub {
  margin-top: 4px;
  font-size: 11px;
  color: #94a3b8;
}
.eng-mobile-empty {
  padding: 24px;
  text-align: center;
  font-size: 12px;
  color: #94a3b8;
}
.eng-mobile-more {
  text-align: center;
  padding: 6px 0;
}
@media (max-width: 767px) {
  .eng-filter {
    flex-direction: column;
    align-items: stretch;
  }
  .eng-filter-keyword,
  .eng-filter-project,
  .eng-filter-pay,
  .eng-filter-date {
    width: 100%;
    min-width: 0;
    flex: 0 0 auto;
  }
  .eng-filter-reset {
    margin-left: 0;
  }
}
</style>
