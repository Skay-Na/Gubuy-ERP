// 工程开销台账页状态（T14）
//
// 与列表页同一套路：筛选写 URL query，刷新不丢、可分享。
// 汇总卡「本年工程开销」点击后带 date_start/date_end 直达，本文件负责把 query 读回筛选。
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { fetchEngineeringExpenses, fetchEngineeringProjects } from '../api/engineering'

const emptyFilters = () => ({
  keyword: '',
  projectId: null, // 数字 = 具体项目；'none' = 公共费用；null = 全部
  categories: [],
  dateRange: [],
  payStatus: null // 1 已付 / 0 未付 / null 全部
})

const parseQuery = query => {
  const f = emptyFilters()
  f.keyword = typeof query.keyword === 'string' ? query.keyword.slice(0, 50) : ''
  if (query.project_id === 'none') f.projectId = 'none'
  else {
    const pid = Number(query.project_id)
    if (Number.isInteger(pid) && pid > 0) f.projectId = pid
  }
  if (typeof query.category === 'string' && query.category) {
    f.categories = query.category
      .split(',')
      .map(v => Number(v))
      .filter(v => v >= 1 && v <= 7)
  }
  if (typeof query.date_start === 'string' && typeof query.date_end === 'string' && query.date_start && query.date_end) {
    f.dateRange = [query.date_start, query.date_end]
  }
  if (query.pay_status === '1' || query.pay_status === '0') f.payStatus = Number(query.pay_status)
  return f
}

const buildQuery = (f, page, pageSize) => {
  const q = {}
  if (f.keyword) q.keyword = f.keyword
  if (f.projectId === 'none') q.project_id = 'none'
  else if (f.projectId) q.project_id = String(f.projectId)
  if (f.categories.length) q.category = f.categories.join(',')
  if (f.dateRange?.length === 2) {
    q.date_start = f.dateRange[0]
    q.date_end = f.dateRange[1]
  }
  if (f.payStatus === 0 || f.payStatus === 1) q.pay_status = String(f.payStatus)
  if (page > 1) q.page = String(page)
  if (pageSize !== 20) q.page_size = String(pageSize)
  return q
}

export function useEngineeringExpenses() {
  const route = useRoute()
  const router = useRouter()

  const filters = reactive(parseQuery(route.query))
  const list = ref([])
  const total = ref(0)
  const summary = ref({ total: 0, paid: 0, unpaid: 0, year_total: 0, count: 0 })
  const byCategory = ref([])
  const page = ref(Number(route.query.page) > 0 ? Number(route.query.page) : 1)
  const pageSize = ref([20, 50, 100].includes(Number(route.query.page_size)) ? Number(route.query.page_size) : 20)
  const loading = ref(false)
  const firstLoaded = ref(false)
  const projectOptions = ref([])

  const isMobile = ref(window.innerWidth < 768)
  window.addEventListener('resize', () => {
    isMobile.value = window.innerWidth < 768
  })

  const params = computed(() => {
    const p = { page: page.value, page_size: pageSize.value }
    if (filters.keyword) p.keyword = filters.keyword
    if (filters.projectId === 'none') p.project_id = 'none'
    else if (filters.projectId) p.project_id = filters.projectId
    if (filters.categories.length) p.category = filters.categories.join(',')
    if (filters.dateRange?.length === 2) {
      p.date_start = filters.dateRange[0]
      p.date_end = filters.dateRange[1]
    }
    if (filters.payStatus === 0 || filters.payStatus === 1) p.pay_status = filters.payStatus
    return p
  })

  const hasActiveFilter = computed(
    () =>
      !!filters.keyword ||
      !!filters.projectId ||
      filters.categories.length > 0 ||
      (filters.dateRange?.length || 0) === 2 ||
      filters.payStatus === 0 ||
      filters.payStatus === 1
  )

  const load = async () => {
    loading.value = true
    try {
      const res = await fetchEngineeringExpenses(params.value)
      // 拦截器返回原始 response：业务数据在 res.result，总计在 res.data.total
      list.value = res?.result?.list || []
      byCategory.value = res?.result?.by_category || []
      summary.value = res?.result?.summary || { total: 0, paid: 0, unpaid: 0, year_total: 0, count: 0 }
      total.value = Number(res?.data?.total) || 0
    } catch (e) {
      list.value = []
      total.value = 0
    } finally {
      loading.value = false
      firstLoaded.value = true
    }
  }

  const loadProjectOptions = async () => {
    try {
      const res = await fetchEngineeringProjects({ page: 1, page_size: 100, sort_by: 'updated_at', order: 'desc' })
      projectOptions.value = res?.result?.list || []
    } catch (e) {
      projectOptions.value = []
    }
  }

  const reloadFirstPage = async () => {
    if (page.value !== 1) {
      page.value = 1
      return
    }
    await load()
  }

  const stableKey = obj =>
    Object.keys(obj)
      .sort()
      .map(k => `${k}=${obj[k]}`)
      .join('&')

  const syncUrl = () => {
    const q = buildQuery(filters, page.value, pageSize.value)
    if (stableKey(q) !== stableKey(route.query)) router.replace({ query: q })
  }

  let keywordTimer = null
  watch(
    () => [
      filters.keyword,
      filters.projectId,
      filters.categories,
      filters.dateRange,
      filters.payStatus,
      pageSize.value
    ],
    (next, prev) => {
      if (next[0] !== prev?.[0]) {
        clearTimeout(keywordTimer)
        keywordTimer = setTimeout(reloadFirstPage, 300)
        return
      }
      reloadFirstPage()
    },
    { deep: true }
  )

  watch(page, () => load())
  watch(() => params.value, syncUrl, { deep: true, immediate: true })

  const init = async () => {
    await Promise.all([load(), loadProjectOptions()])
  }

  const resetFilters = () => {
    Object.assign(filters, emptyFilters())
  }

  const setPage = value => {
    page.value = value
  }

  // 类别 chip 与筛选条联动：点 chip 等价于切多选筛选
  const toggleCategory = category => {
    if (category === null) {
      filters.categories = []
      return
    }
    filters.categories = [category]
  }

  // 移动端"加载更多"：追加下一页（不能复用 load，那会覆盖已加载的数据）
  const loadingMore = ref(false)
  const hasMore = computed(() => list.value.length < total.value)
  const loadMore = async () => {
    if (loadingMore.value || !hasMore.value) return
    loadingMore.value = true
    try {
      const res = await fetchEngineeringExpenses({ ...params.value, page: page.value + 1 })
      list.value = list.value.concat(res?.result?.list || [])
      page.value += 1
      total.value = Number(res?.data?.total) || total.value
    } catch (e) {
      /* 保持现有列表 */
    } finally {
      loadingMore.value = false
    }
  }

  return {
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
    hasMore,
    init,
    load,
    loadMore,
    resetFilters,
    setPage,
    toggleCategory
  }
}
