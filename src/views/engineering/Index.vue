<template>
  <!--
    工程列表页（SPEC-ENG-001 §4 / 任务卡 T11）
    数据来源：GET /summary（6 张卡）+ GET /projects（列表）；前端不重算任何汇总数字。
    筛选与页码写在 URL query 里，从详情 history.back() 回来保持原样。
  -->
  <div class="min-h-screen bg-slate-50/60">
    <div class="mx-auto w-full max-w-[1440px] px-4 py-6 md:px-8 md:py-8">
      <!-- 页头（§4.2） -->
      <header class="mb-5 flex flex-col gap-3 md:mb-6 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 class="text-2xl font-black text-slate-900 md:text-3xl">工程项目</h1>
          <p class="mt-1 text-sm text-slate-400">工程机批量供货：合同、清单、分期回款、发货</p>
        </div>
        <div class="flex items-center gap-2">
          <router-link
            to="/admin/engineering/expenses"
            class="inline-flex h-10 flex-1 items-center justify-center gap-1.5 rounded-xl border border-slate-200 bg-white px-4 text-sm font-medium text-slate-600 hover:border-slate-300 md:flex-none"
          >
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M3 8.5A2.5 2.5 0 015.5 6h13A2.5 2.5 0 0121 8.5v9a2.5 2.5 0 01-2.5 2.5h-13A2.5 2.5 0 013 17.5v-9z"
                stroke="currentColor"
                stroke-width="1.6"
              />
              <path d="M16 13.5h2" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
            </svg>
            工程开销
          </router-link>
          <button
            type="button"
            class="inline-flex h-10 flex-1 items-center justify-center gap-1 rounded-xl px-4 text-sm font-medium text-white hover:opacity-90 md:flex-none"
            style="background: #635bff"
            @click="openCreate"
          >
            <span class="text-base leading-none">+</span>
            新建工程
          </button>
        </div>
      </header>

      <!-- 6 张汇总卡（§4.3） -->
      <EngineeringStatCards
        :summary="summary"
        :loading="loadingSummary"
        class="mb-4"
        @pick-active="applyActiveStatuses"
        @pick-pending="applyPendingOnly"
        @pick-overdue="toggleOverdueOnly"
        @open-expenses="goExpenses"
      />

      <!-- 筛选条：PC 内联；移动端折叠为按钮 + 抽屉（§4.4 / §10） -->
      <div class="mb-4 hidden md:block">
        <EngineeringFilterBar :filters="filters" :employees="employees" @reset="resetFilters" />
      </div>
      <div class="mb-4 flex items-center gap-2 md:hidden">
        <el-button class="flex-1" @click="filterDrawer = true">
          筛选<span v-if="activeFilterCount" class="ml-1 text-[11px]" style="color: #635bff">· {{ activeFilterCount }}</span>
        </el-button>
        <el-button v-if="hasActiveFilter" class="flex-1" @click="resetFilters">清空筛选</el-button>
      </div>

      <!-- PC 表格 -->
      <div v-if="!isMobile" class="mb-3">
        <EngineeringListTable
          :list="list"
          :loading="loadingList"
          :has-active-filter="hasActiveFilter"
          :sort-by="sortBy"
          :order="order"
          @row-click="row => goDetail(row.id)"
          @view="row => goDetail(row.id)"
          @edit="openEdit"
          @void="openVoid"
          @delete="confirmDelete"
          @reset="resetFilters"
          @create="openCreate"
          @sort-change="onSortChange"
        />
      </div>

      <!-- 移动卡片流 -->
      <div v-else class="mb-3">
        <EngineeringListCards
          :list="list"
          :has-more="hasMore"
          :loading-more="loadingMore"
          :first-loaded="firstLoaded"
          :has-active-filter="hasActiveFilter"
          @card-click="row => goDetail(row.id)"
          @load-more="loadMore"
          @reset="resetFilters"
          @create="openCreate"
        />
      </div>

      <!-- PC 分页 -->
      <div v-if="!isMobile && total > 0" class="flex justify-end">
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

      <!-- 移动端筛选抽屉 -->
      <el-drawer v-model="filterDrawer" title="筛选" direction="btt" size="80%">
        <EngineeringFilterBar :filters="filters" :employees="employees" @reset="resetFilters" />
        <template #footer>
          <div class="flex gap-2">
            <el-button class="flex-1" @click="resetFilters">重置</el-button>
            <el-button class="flex-1" type="primary" @click="filterDrawer = false">完成</el-button>
          </div>
        </template>
      </el-drawer>

      <!-- 建档 / 编辑抽屉（T15） -->
      <EngineeringProjectDrawer
        v-model="drawerVisible"
        :project="editingProject"
        :employees="employees"
        @saved="onProjectSaved"
      />

      <!-- 作废弹窗（T15） -->
      <EngineeringVoidDialog v-model="voidVisible" :project="voidingProject" @saved="onVoided" />
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteEngineeringProject } from '../../api/engineering'
import { fetchSimpleEmployees } from '../../api/employees'
import { useEngineeringList } from '../../composables/useEngineeringList'
import EngineeringStatCards from '../../components/engineering/EngineeringStatCards.vue'
import EngineeringFilterBar from '../../components/engineering/EngineeringFilterBar.vue'
import EngineeringListTable from '../../components/engineering/EngineeringListTable.vue'
import EngineeringListCards from '../../components/engineering/EngineeringListCards.vue'
import EngineeringProjectDrawer from '../../components/engineering/EngineeringProjectDrawer.vue'
import EngineeringVoidDialog from '../../components/engineering/EngineeringVoidDialog.vue'

const router = useRouter()

const {
  filters,
  summary,
  list,
  total,
  page,
  pageSize,
  sortBy,
  order,
  loadingList,
  loadingSummary,
  firstLoaded,
  isMobile,
  hasActiveFilter,
  loadingMore,
  hasMore,
  init,
  loadSummary,
  loadList,
  resetFilters,
  setPage,
  toggleOverdueOnly,
  applyPendingOnly,
  applyActiveStatuses,
  onSortChange,
  loadMore
} = useEngineeringList()

const employees = ref([])
const filterDrawer = ref(false)

const activeFilterCount = computed(() => {
  let n = 0
  if (filters.keyword) n += 1
  if (filters.statuses.length) n += 1
  if (filters.projectType) n += 1
  if (filters.ownerId) n += 1
  if ((filters.signRange?.length || 0) === 2) n += 1
  if (filters.onlyOverdue) n += 1
  if (filters.onlyPending) n += 1
  if (filters.includeVoid) n += 1
  return n
})

const goDetail = id => router.push(`/admin/engineering/${id}`)

// 汇总卡 5 点击：带本年区间跳台账页（区间由台账页直接使用）
const goExpenses = () => {
  const year = new Date().getFullYear()
  router.push({
    path: '/admin/engineering/expenses',
    query: { date_start: `${year}-01-01`, date_end: `${year}-12-31` }
  })
}

// ---------- 建档 / 编辑 ----------
const drawerVisible = ref(false)
const editingProject = ref(null)

const openCreate = () => {
  editingProject.value = null
  drawerVisible.value = true
}

const openEdit = row => {
  editingProject.value = row
  drawerVisible.value = true
}

const onProjectSaved = payload => {
  drawerVisible.value = false
  // 建档成功 → 直接进详情（建档后下一步通常就是加清单，§6）
  if (payload?.created && payload?.id) {
    goDetail(payload.id)
    return
  }
  loadList()
  loadSummary()
}

// ---------- 作废 ----------
const voidVisible = ref(false)
const voidingProject = ref(null)

const openVoid = row => {
  voidingProject.value = row
  voidVisible.value = true
}

const onVoided = () => {
  voidVisible.value = false
  loadList()
  loadSummary()
}

// ---------- 删除 ----------
const confirmDelete = async row => {
  try {
    await ElMessageBox.confirm(
      `确定删除「${row.name}」？删除后不可恢复。仅跟进中且无回款、无出库的项目可删除。`,
      '删除工程',
      { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消', confirmButtonClass: 'el-button--danger' }
    )
  } catch (e) {
    return
  }
  try {
    await deleteEngineeringProject(row.id)
    ElMessage.success('项目已删除')
    // 删掉当前页最后一条时回退一页，避免停在空白页
    if (list.value.length === 1 && page.value > 1) {
      page.value -= 1
    } else {
      loadList()
    }
    loadSummary()
  } catch (e) {
    /* 错误提示由 request 拦截器统一处理 */
  }
}

onMounted(async () => {
  init()
  try {
    const res = await fetchSimpleEmployees()
    employees.value = res?.result || []
  } catch (e) {
    employees.value = []
  }
})
</script>
