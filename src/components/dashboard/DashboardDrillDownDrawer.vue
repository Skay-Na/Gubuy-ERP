<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Right } from '@element-plus/icons-vue'
import { fetchBreakdownDetail, fetchProfitBreakdown } from '../../api/dashboardDrill'
import {
  NET_PROFIT_VIEW_TYPE,
  formatMoney,
  formatPeriodText,
  getBreakdownMeta
} from './drillColumns'

const props = defineProps({
  visible: { type: Boolean, default: false },
  type: { type: String, default: '' },
  params: { type: Object, default: () => ({}) },
  label: { type: String, default: '' },
  fallbackRoute: { type: String, default: '' }
})
const emit = defineEmits(['close'])
const router = useRouter()

// 视图栈：净利润公式视图内可继续下钻到各组成项明细，返回时逐层弹出
const stack = ref([])
const loading = ref(false)
const error = ref('')
const list = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const totalAmount = ref(0)
const breakdownData = ref(null)

const isMobile = ref(false)
let mediaQuery = null

const current = computed(() => stack.value[stack.value.length - 1] || null)
const isNetProfit = computed(() => current.value?.type === NET_PROFIT_VIEW_TYPE)
const meta = computed(() => getBreakdownMeta(current.value?.type))
const periodText = computed(() => formatPeriodText(current.value?.params || {}))

const title = computed(() => {
  if (!current.value) return '明细'
  const base = isNetProfit.value ? '经营净利润计算过程' : (meta.value?.title || '明细')
  const label = current.value.label ? ` · ${current.value.label}` : ''
  return `${base}${label}（${periodText.value}）`
})

// 手机端仅保留 mobileVisible 列（核心三列），桌面端展示全部
const visibleColumns = computed(() => {
  const columns = meta.value?.columns || []
  return isMobile.value ? columns.filter(col => col.mobileVisible !== false) : columns
})

/** 净利润公式视图的组成行（数据来自 profit-breakdown 接口，各组成行可继续下钻） */
const netProfitRows = computed(() => {
  const data = breakdownData.value || {}
  const num = key => Number(data[key] || 0)
  return [
    { key: 'revenue', label: '销售额', amount: num('sales_revenue'), drill: { type: 'revenue' }, hint: '正常订单实收，已扣抹零优惠' },
    { key: 'cost', label: '− 货品成本', amount: num('product_cost'), drill: { type: 'cost' }, hint: '订单明细进价 × 数量', badge: data.cost_incomplete ? `含 ${data.uncosted_order_item_count || 0} 笔待核算成本明细` : '' },
    { key: 'delivery', label: '− 配送费', amount: num('delivery_fee'), drill: num('delivery_fee') ? { type: 'delivery' } : null },
    { key: 'installation', label: '− 安装费', amount: num('installation_fee'), drill: num('installation_fee') ? { type: 'installation' } : null },
    { key: 'referral', label: '− 介绍人提成', amount: num('referral_fee'), drill: num('referral_fee') ? { type: 'referral' } : null },
    { key: 'commission', label: '− 业务提成', amount: num('commission_amount'), drill: num('commission_amount') ? { type: 'commission' } : null, hint: '员工销售提成（普通单按员工/商品比例）；商会单按 1% 入池平分' },
    { key: 'trade_in', label: '− 以旧换新', amount: num('trade_in_amount'), drill: num('trade_in_amount') ? { type: 'trade_in' } : null },
    { key: 'gross_profit', label: '= 毛利', amount: num('gross_profit'), formula: true },
    { key: 'operating_expense', label: '− 经营开销', amount: num('operating_expense'), drill: { type: 'operating_expense' }, hint: '日常费用，不含采购货款/以旧换新' },
    { key: 'net_profit', label: '= 经营净利润', amount: num('net_profit'), formula: true, final: true }
  ]
})

async function load() {
  if (!current.value) return
  loading.value = true
  error.value = ''
  try {
    if (isNetProfit.value) {
      const response = await fetchProfitBreakdown(current.value.params)
      if (response.data.code !== 200) throw new Error(response.data.msg || '接口返回异常')
      breakdownData.value = response.data.data || {}
    } else {
      const response = await fetchBreakdownDetail(current.value.type, {
        ...current.value.params,
        page: page.value,
        page_size: pageSize.value
      })
      if (response.data.code !== 200) throw new Error(response.data.msg || '接口返回异常')
      const data = response.data.data || {}
      list.value = data.list || []
      total.value = Number(data.total || 0)
      totalAmount.value = Number(data.total_amount || 0)
    }
  } catch (cause) {
    console.error('Failed to fetch breakdown detail:', cause)
    error.value = '明细加载失败，请重试'
  } finally {
    loading.value = false
  }
}

/** 打开新的下钻视图（继承当前视图的期间参数） */
function drillTo(type, extraParams = {}, label = '') {
  const base = current.value?.params || {}
  stack.value.push({ type, label, params: { ...base, ...extraParams } })
  page.value = 1
  load()
}

function back() {
  if (stack.value.length <= 1) return
  stack.value.pop()
  page.value = 1
  load()
}

function goFallback() {
  if (!props.fallbackRoute) return
  emit('close')
  router.push(props.fallbackRoute)
}

function onTableFormat(col) {
  return col.formatter ? (row, column, cellValue) => col.formatter(cellValue, row) : undefined
}

// 首次打开或切换类型时重置视图栈并加载
watch(
  () => [props.visible, props.type, props.params, props.label],
  ([visible, type]) => {
    if (visible && type) {
      stack.value = [{ type, params: { ...props.params }, label: props.label }]
      page.value = 1
      load()
    }
  },
  { immediate: true }
)

onMounted(() => {
  mediaQuery = window.matchMedia('(max-width: 767px)')
  isMobile.value = mediaQuery.matches
  mediaQuery.addEventListener('change', handleMediaChange)
})

onBeforeUnmount(() => {
  mediaQuery?.removeEventListener('change', handleMediaChange)
  mediaQuery = null
})

function handleMediaChange(event) {
  isMobile.value = event.matches
}
</script>

<template>
  <el-drawer
    :model-value="visible"
    :size="isMobile ? '100%' : '760px'"
    :with-header="false"
    class="drill-drawer"
    @update:model-value="emit('close')"
  >
    <div class="drill-body">
      <header class="drill-header">
        <button v-if="stack.length > 1" class="back" aria-label="返回上一层" @click="back">‹ 返回</button>
        <div class="drill-heading">
          <span class="eyebrow">DRILL DOWN</span>
          <h3>{{ title }}</h3>
        </div>
        <div class="header-actions">
          <button v-if="fallbackRoute" class="fallback" @click="goFallback">
            <span>前往{{ fallbackRoute.includes('orders') ? '订单管理' : '财务中心' }}</span> <el-icon><Right /></el-icon>
          </button>
          <button class="close" aria-label="关闭" @click="emit('close')">✕</button>
        </div>
      </header>

      <div v-if="error" class="drill-error" role="alert">
        {{ error }}
        <button @click="load">重试</button>
      </div>

      <!-- 净利润公式汇总视图：各组成行继续下钻 -->
      <template v-else-if="isNetProfit">
        <div v-loading="loading" class="formula-list">
          <div
            v-for="row in netProfitRows"
            :key="row.key"
            class="formula-row"
            :class="{ formula: row.formula, final: row.final, clickable: row.drill }"
            @click="row.drill && drillTo(row.drill.type, {}, row.label.replace(/^[−=]\s*/, ''))"
          >
            <span class="formula-label">{{ row.label }}<small v-if="row.hint">{{ row.hint }}</small></span>
            <span class="formula-right">
              <button v-if="row.badge" class="badge" @click.stop="drillTo('cost', { only_uncosted: true }, '待核算成本')">{{ row.badge }}</button>
              <strong>{{ formatMoney(row.amount) }}</strong>
              <span v-if="row.drill" class="chevron">›</span>
            </span>
          </div>
        </div>
      </template>

      <!-- 明细列表视图 -->
      <template v-else>
        <div v-if="!loading && !list.length" class="drill-empty">
          <el-empty description="本期暂无明细数据" />
        </div>
        <el-table
          v-else
          v-loading="loading"
          :data="list"
          class="drill-table"
          size="small"
          :show-header="true"
        >
          <el-table-column
            v-for="col in visibleColumns"
            :key="col.prop"
            :prop="col.prop"
            :label="col.label"
            :align="col.align || 'left'"
            :formatter="onTableFormat(col)"
            :min-width="isMobile ? (col.prop === 'amount' ? 92 : 100) : (col.prop === 'amount' ? 110 : 120)"
            show-overflow-tooltip
          />
        </el-table>

        <div class="drill-footer">
          <div class="drill-total">
            <span>{{ meta?.amountLabel || '合计' }}</span>
            <strong>{{ formatMoney(totalAmount) }}</strong>
          </div>
          <el-pagination
            v-if="total > pageSize"
            layout="prev, pager, next"
            :total="total"
            :page-size="pageSize"
            :current-page="page"
            @current-change="value => { page = value; load() }"
          />
        </div>
      </template>
    </div>
  </el-drawer>
</template>

<style scoped>
.drill-body{display:flex;flex-direction:column;height:100%;background:#f7f9fc}
.drill-header{display:flex;align-items:center;gap:12px;padding:16px 20px;background:#fff;border-bottom:1px solid #e8edf3}
.back{flex:none;border:0;background:none;color:#635bff;font-size:12px;font-weight:700;cursor:pointer}
.eyebrow{font-size:9px;font-weight:800;letter-spacing:.14em;color:#635bff}
.drill-heading{flex:1;min-width:0}
.drill-heading h3{margin:3px 0 0;color:#172033;font-size:15px;font-weight:750;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.header-actions{display:flex;align-items:center;gap:8px;flex:none}
.fallback{display:flex;align-items:center;gap:3px;border:1px solid #e1e7ef;border-radius:8px;padding:6px 9px;background:#fff;color:#57667a;font-size:11px;font-weight:650;cursor:pointer;white-space:nowrap}
.close{border:0;background:none;color:#8390a2;font-size:14px;cursor:pointer;padding:4px}
.drill-error{display:flex;justify-content:center;align-items:center;gap:10px;padding:28px 0;color:#bd4141;font-size:13px}
.drill-error button{border:1px solid #f0c7c7;border-radius:8px;padding:5px 12px;background:#fff;color:#b02f2f;font-weight:700;cursor:pointer}
.drill-empty{padding-top:40px}
.drill-table{margin:14px 16px 0;width:auto;background:#fff;border-radius:12px}
.drill-footer{margin-top:auto;display:flex;flex-direction:column;gap:10px;padding:14px 16px 18px}
.drill-total{display:flex;justify-content:space-between;align-items:center;padding:13px 16px;background:#fff;border:1px solid #e8edf3;border-radius:12px}
.drill-total span{color:#66758a;font-size:12px;font-weight:650}
.drill-total strong{color:#172033;font-size:17px;letter-spacing:-.02em}
.formula-list{display:flex;flex-direction:column;gap:8px;padding:16px 20px 24px}
.formula-row{display:flex;justify-content:space-between;align-items:center;gap:12px;padding:13px 15px;background:#fff;border:1px solid #e8edf3;border-radius:12px}
.formula-row.clickable{cursor:pointer;transition:border-color .15s}
.formula-row.clickable:hover{border-color:#bdcdeb}
.formula-row.formula{background:#eef2f7;border-color:#e4eaf4}
.formula-row.final{background:linear-gradient(120deg,#635bff,#4b44c8);border-color:#635bff}
.formula-row.final .formula-label,.formula-row.final strong{color:#fff}
.formula-label{display:flex;flex-direction:column;color:#263449;font-size:13px;font-weight:650}
.formula-label small{margin-top:2px;color:#8390a2;font-size:10px;font-weight:400}
.formula-row.formula .formula-label{color:#172033}
.formula-right{display:flex;align-items:center;gap:8px;white-space:nowrap}
.formula-right strong{color:#172033;font-size:14px}
.badge{border:0;border-radius:999px;padding:4px 9px;background:#fff4e5;color:#c66a17;font-size:10px;font-weight:700;cursor:pointer}
.chevron{color:#a5b1c2;font-weight:700}
@media(max-width:767px){
  .drill-header{padding:13px 14px}
  .drill-heading h3{font-size:14px}
  .drill-table{margin:10px 10px 0}
  .drill-footer{padding:10px 10px 14px}
  .drill-total strong{font-size:15px}
  .formula-list{padding:12px 14px 20px}
  .fallback span{display:none}
}
</style>
