// 工程列表页状态（T11）
//
// 职责：
//   1. 汇总卡数据（/summary，前端不重算任何数字）
//   2. 列表筛选 + 排序 + 分页（/projects）
//   3. 筛选状态与 URL query 双向同步 —— 进详情后 history.back() 回来筛选与页码不丢
//      （SPEC-ENG-001 §2.2 硬要求）
//
// 约定：所有金额/比例均由后端返回，本文件只管透传，不做业务计算。
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { fetchEngineeringProjects, fetchEngineeringSummary } from '../api/engineering'

// 六态（字典来自契约 §1.4，文案与 StatusTag 保持一致）
export const STATUS_OPTIONS = [
  { value: 1, label: '跟进中' },
  { value: 2, label: '已签约' },
  { value: 3, label: '供货中' },
  { value: 4, label: '已验收' },
  { value: 5, label: '已结清' },
  { value: 9, label: '已作废' }
]

// 项目类型（契约 §1.4）
export const PROJECT_TYPE_OPTIONS = [
  { value: 1, label: '住宅精装' },
  { value: 2, label: '商业综合体' },
  { value: 3, label: '酒店公寓' },
  { value: 4, label: '政企学校' },
  { value: 5, label: '其他' }
]

// 表格排序白名单（契约 §3.2 sort_by）
export const SORTABLE_FIELDS = ['updated_at', 'contract_total', 'pending_amount', 'next_due_date', 'net_margin_rate']

const emptyFilters = () => ({
  keyword: '',
  statuses: [],
  projectType: null,
  ownerId: null,
  signRange: [], // ['2026-01-01', '2026-06-30']
  onlyOverdue: false,
  onlyPending: false,
  includeVoid: false
})

// query 字符串 → 筛选对象（非法值一律降级为"不筛选"，不让坏链接白屏）
const parseQuery = query => {
  const f = emptyFilters()
  f.keyword = typeof query.keyword === 'string' ? query.keyword.slice(0, 50) : ''
  if (typeof query.status === 'string' && query.status) {
    f.statuses = query.status
      .split(',')
      .map(v => Number(v))
      .filter(v => STATUS_OPTIONS.some(o => o.value === v))
  }
  const pt = Number(query.project_type)
  if (PROJECT_TYPE_OPTIONS.some(o => o.value === pt)) f.projectType = pt
  const owner = Number(query.owner_id)
  if (Number.isInteger(owner) && owner > 0) f.ownerId = owner
  if (typeof query.sign_start === 'string' && typeof query.sign_end === 'string' && query.sign_start && query.sign_end) {
    f.signRange = [query.sign_start, query.sign_end]
  }
  f.onlyOverdue = query.only_overdue === '1' || query.only_overdue === 'true'
  f.onlyPending = query.only_pending === '1' || query.only_pending === 'true'
  f.includeVoid = query.include_void === '1' || query.include_void === 'true'
  return f
}

// 筛选对象 → query（空值不落 URL，保持链接干净可分享）
const buildQuery = (f, page, pageSize, sortBy, order) => {
  const q = {}
  if (f.keyword) q.keyword = f.keyword
  if (f.statuses.length) q.status = f.statuses.join(',')
  if (f.projectType) q.project_type = String(f.projectType)
  if (f.ownerId) q.owner_id = String(f.ownerId)
  if (f.signRange?.length === 2) {
    q.sign_start = f.signRange[0]
    q.sign_end = f.signRange[1]
  }
  if (f.onlyOverdue) q.only_overdue = '1'
  if (f.onlyPending) q.only_pending = '1'
  if (f.includeVoid) q.include_void = '1'
  if (page > 1) q.page = String(page)
  if (pageSize !== 20) q.page_size = String(pageSize)
  if (sortBy !== 'updated_at') q.sort_by = sortBy
  if (order !== 'desc') q.order = order
  return q
}

export function useEngineeringList() {
  const route = useRoute()
  const router = useRouter()

  const filters = reactive(parseQuery(route.query))
  const summary = ref(null)
  const list = ref([])
  const total = ref(0)
  const page = ref(Number(route.query.page) > 0 ? Number(route.query.page) : 1)
  const pageSize = ref([20, 50, 100].includes(Number(route.query.page_size)) ? Number(route.query.page_size) : 20)
  const sortBy = ref(SORTABLE_FIELDS.includes(route.query.sort_by) ? route.query.sort_by : 'updated_at')
  const order = ref(route.query.order === 'asc' ? 'asc' : 'desc')

  const loadingList = ref(false)
  const loadingSummary = ref(false)
  const firstLoaded = ref(false)

  // 移动端判定：沿用项目既有惯例（Cashier.vue）
  const isMobile = ref(window.innerWidth < 768)
  const onResize = () => {
    isMobile.value = window.innerWidth < 768
  }
  window.addEventListener('resize', onResize)
  onBeforeUnmount(() => window.removeEventListener('resize', onResize))

  // 是否处于"有任何筛选"状态：决定空态文案是"还没有项目"还是"没有符合条件的项目"
  const hasActiveFilter = computed(
    () =>
      !!filters.keyword ||
      filters.statuses.length > 0 ||
      !!filters.projectType ||
      !!filters.ownerId ||
      (filters.signRange?.length || 0) === 2 ||
      filters.onlyOverdue ||
      filters.onlyPending ||
      filters.includeVoid
  )

  const listParams = computed(() => {
    const params = { page: page.value, page_size: pageSize.value, sort_by: sortBy.value, order: order.value }
    if (filters.keyword) params.keyword = filters.keyword
    if (filters.statuses.length) params.status = filters.statuses.join(',')
    if (filters.projectType) params.project_type = filters.projectType
    if (filters.ownerId) params.owner_id = filters.ownerId
    if (filters.signRange?.length === 2) {
      params.sign_start = filters.signRange[0]
      params.sign_end = filters.signRange[1]
    }
    if (filters.onlyOverdue) params.only_overdue = true
    if (filters.onlyPending) params.only_pending = true
    if (filters.includeVoid) params.include_void = true
    return params
  })

  const loadSummary = async () => {
    loadingSummary.value = true
    try {
      const res = await fetchEngineeringSummary()
      // 拦截器返回原始 response：业务数据在 res.result
      summary.value = res?.result || null
    } catch (e) {
      summary.value = null
    } finally {
      loadingSummary.value = false
    }
  }

  const loadList = async () => {
    loadingList.value = true
    try {
      const res = await fetchEngineeringProjects(listParams.value)
      list.value = res?.result?.list || []
      total.value = Number(res?.data?.total) || 0
    } catch (e) {
      list.value = []
      total.value = 0
    } finally {
      loadingList.value = false
      firstLoaded.value = true
    }
  }

  // 筛选/排序/页大小变化 → 回第 1 页（页码变化本身不重置）
  const reloadFirstPage = async () => {
    if (page.value !== 1) {
      page.value = 1
      // page 变化会触发 watch → 统一在 watch 里发出请求，这里提前 return 避免双请求
      return
    }
    await Promise.all([loadList(), loadSummary()])
  }

  // URL 同步 + 请求：用 replace 而不是 push，避免筛选操作把浏览器历史塞满，
  // 保证"从详情返回"时退回到的是列表页本身而不是中间若干筛选态。
  // 比较时按 key 排序，避免 route.query 与本地对象键序不同导致无意义的 replace。
  const stableKey = obj =>
    Object.keys(obj)
      .sort()
      .map(k => `${k}=${obj[k]}`)
      .join('&')

  const syncUrl = () => {
    const q = buildQuery(filters, page.value, pageSize.value, sortBy.value, order.value)
    if (stableKey(q) !== stableKey(route.query)) router.replace({ query: q })
  }

  let keywordTimer = null
  watch(
    () => [filters.keyword, filters.statuses, filters.projectType, filters.ownerId, filters.signRange, filters.onlyOverdue, filters.onlyPending, filters.includeVoid, sortBy.value, order.value, pageSize.value],
    (next, prev) => {
      // 关键词变化走 300ms 防抖（§4.4）；其余控件立即查询
      const keywordChanged = next[0] !== prev?.[0]
      if (keywordChanged) {
        clearTimeout(keywordTimer)
        keywordTimer = setTimeout(() => {
          reloadFirstPage()
        }, 300)
        return
      }
      reloadFirstPage()
    },
    { deep: true }
  )

  watch(page, () => {
    loadList()
  })

  // 每次列表条件实际生效前，把状态写进 URL
  watch(
    () => listParams.value,
    () => syncUrl(),
    { deep: true, immediate: true }
  )

  const init = async () => {
    await Promise.all([loadList(), loadSummary()])
  }

  const resetFilters = () => {
    Object.assign(filters, emptyFilters())
  }

  const setPage = value => {
    page.value = value
  }

  const toggleOverdueOnly = () => {
    filters.onlyOverdue = !filters.onlyOverdue
  }

  const applyPendingOnly = () => {
    filters.onlyPending = true
    filters.statuses = []
  }

  const applyActiveStatuses = () => {
    filters.statuses = [1, 2, 3]
    filters.onlyPending = false
  }

  // 表头排序：同一列再点切换升降序
  const onSortChange = ({ prop, order: sortOrder }) => {
    if (!SORTABLE_FIELDS.includes(prop)) return
    if (!sortOrder) {
      sortBy.value = 'updated_at'
      order.value = 'desc'
      return
    }
    sortBy.value = prop
    order.value = sortOrder === 'ascending' ? 'asc' : 'desc'
  }

  // 移动端"加载更多"：追加下一页
  const loadingMore = ref(false)
  const hasMore = computed(() => list.value.length < total.value)
  const loadMore = async () => {
    if (loadingMore.value || !hasMore.value) return
    loadingMore.value = true
    try {
      const nextPage = page.value + 1
      const res = await fetchEngineeringProjects({ ...listParams.value, page: nextPage })
      list.value = list.value.concat(res?.result?.list || [])
      page.value = nextPage
      total.value = Number(res?.data?.total) || total.value
    } catch (e) {
      /* 保持当前列表，不打断浏览 */
    } finally {
      loadingMore.value = false
    }
  }

  return {
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
  }
}
