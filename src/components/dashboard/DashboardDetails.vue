<script setup>
import { computed, ref } from 'vue'
import { formatCurrency } from '../../utils/dashboard'

const props = defineProps({ summary: { type: Object, required: true }, totalBalance: { type: Number, default: 0 } })
const emit = defineEmits(['navigate', 'drill'])

const RANKING_PREVIEW = 5
const rankings = computed(() => props.summary.employee_rankings || [])
const hasMoreRankings = computed(() => rankings.value.length > RANKING_PREVIEW)
const rankingsExpanded = ref(false)
const visibleRankings = computed(() => (rankingsExpanded.value ? rankings.value : rankings.value.slice(0, RANKING_PREVIEW)))

/** 员工业绩行钻取：归属 ID 约定与后端排行口径一致（商会订单=999999，老板客户=boss，其余为真实员工ID） */
function drillRanking(item) {
  let employeeId = 'boss'
  if (item.name === '商会订单') employeeId = '999999'
  else if (Number(item.employee_id) > 0) employeeId = String(item.employee_id)
  emit('drill', { type: 'employee_orders', params: { employee_id: employeeId }, label: item.name })
}

/** 账户余额行钻取：查看该账户期间流水（账户列表已排除"经营开销"） */
function drillAccount(item) {
  emit('drill', { type: 'account_logs', params: { account_id: item.id }, label: item.name })
}

// 与 totalBalance 同口径：排除经营开销（开销账户余额为负数大额，计入列表会让"总额≠各项之和"引人误会）
const accounts = computed(() => (props.summary.accounts || []).filter(item => item.name !== '经营开销'))
const topProducts = computed(() => (props.summary.top_products || []).slice(0, 5))
const categorySales = computed(() => (props.summary.category_sales || []).slice(0, 5))
const maxCategoryRevenue = computed(() => Math.max(...categorySales.value.map(item => Number(item.revenue || 0)), 0))
const categoryShare = item => {
  const max = maxCategoryRevenue.value
  return max > 0 ? Math.max(4, Math.round(Number(item.revenue || 0) / max * 100)) : 0
}
const grossNote = item => {
  if (Number(item.uncosted_order_item_count || 0) > 0) return { text: '毛利待核', partial: true }
  return { text: `毛利 ${formatCurrency(item.gross_profit)}`, partial: false }
}
</script>

<template>
  <div class="detail-grid">
    <section class="panel" aria-labelledby="ranking-title">
      <div class="panel-heading"><div><span class="eyebrow">TEAM</span><h2 id="ranking-title">本期业绩</h2></div><button class="more" @click="emit('navigate', '/admin/employees')">员工管理 ↗</button></div>
      <div v-if="visibleRankings.length" class="rows">
        <div v-for="(item, index) in visibleRankings" :key="item.employee_id ?? index" class="row rank-row clickable" @click="drillRanking(item)">
          <span class="rank" :class="{ gold: index === 0, silver: index === 1, bronze: index === 2 }">{{ index < 3 ? index + 1 : String(index + 1).padStart(2, '0') }}</span>
          <span class="item-name">{{ item.name }}<small>{{ item.order_count || 0 }} 单</small></span>
          <span class="row-figures">
            <strong>{{ formatCurrency(item.revenue) }}</strong>
            <small :class="{ partial: grossNote(item).partial }">{{ grossNote(item).text }}</small>
          </span>
        </div>
        <button v-if="hasMoreRankings" class="expand-toggle" @click="rankingsExpanded = !rankingsExpanded">
          {{ rankingsExpanded ? '收起' : `展开全部 ${rankings.length} 人` }}
        </button>
      </div>
      <p v-else class="empty">所选期间暂无员工业绩</p>
    </section>
    <section class="panel" aria-labelledby="account-title">
      <div class="panel-heading"><div><span class="eyebrow">CASH POSITION</span><h2 id="account-title">账户余额</h2></div><button class="more" @click="emit('navigate', '/admin/finance')">资金工作台 ↗</button></div>
      <strong class="balance">{{ formatCurrency(totalBalance) }}</strong>
      <div v-if="accounts.length" class="rows accounts">
        <div v-for="item in accounts" :key="item.id" class="row account-row clickable" @click="drillAccount(item)"><span>{{ item.name }}</span><strong>{{ formatCurrency(item.balance) }}</strong></div>
      </div>
      <p v-else class="empty">暂无账户数据</p>
    </section>
    <section class="panel" aria-labelledby="product-title">
      <div class="panel-heading"><div><span class="eyebrow">BEST SELLERS</span><h2 id="product-title">热销商品</h2></div><button class="more" @click="emit('navigate', '/admin/products')">商品工作台 ↗</button></div>
      <div v-if="topProducts.length" class="rows">
        <div v-for="(item, index) in topProducts" :key="item.product_id ?? index" class="row product-row">
          <span class="rank" :class="{ gold: index === 0, silver: index === 1, bronze: index === 2 }">{{ index < 3 ? index + 1 : String(index + 1).padStart(2, '0') }}</span>
          <span class="item-name">{{ item.name }}<small>{{ item.quantity || 0 }} 件</small></span>
          <strong>{{ formatCurrency(item.revenue) }}</strong>
        </div>
      </div>
      <p v-else class="empty">所选期间暂无商品销量</p>
    </section>
    <section class="panel" aria-labelledby="category-title">
      <div class="panel-heading"><div><span class="eyebrow">MIX</span><h2 id="category-title">分类销售占比</h2></div></div>
      <div v-if="categorySales.length" class="rows">
        <div v-for="(item, index) in categorySales" :key="item.name ?? index" class="row category-row">
          <span class="item-name">{{ item.name }}</span>
          <span class="category-bar" aria-hidden="true"><i :style="{ width: `${categoryShare(item)}%` }"></i></span>
          <strong>{{ formatCurrency(item.revenue) }}</strong>
        </div>
      </div>
      <p v-else class="empty">所选期间暂无分类销售数据</p>
    </section>
  </div>
</template>

<style scoped>
.detail-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.panel{min-width:0;background:#fff;border:1px solid #e3e8ee;border-radius:12px;padding:22px;box-shadow:0 1px 3px rgba(10,37,64,.06)}.panel-heading{display:flex;justify-content:space-between;align-items:flex-end;gap:12px}.eyebrow{font-size:10px;font-weight:800;letter-spacing:.14em;color:#94a3b8}h2{margin:5px 0 0;color:#172033;font-size:18px;font-weight:750}.more{border:0;background:none;color:#635bff;font-size:12px;font-weight:650;white-space:nowrap;cursor:pointer}.rows{margin-top:16px}.row{display:flex;align-items:center;gap:10px;min-width:0;padding:11px 0;border-bottom:1px solid #f0f3f7;color:#475569;font-size:13px}.row:last-child{border-bottom:0}.item-name{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#263449;font-weight:650}.item-name small{display:block;margin-top:2px;color:#94a3b8;font-size:11px;font-weight:400}.rank{display:inline-flex;align-items:center;justify-content:center;flex:none;width:22px;height:22px;border-radius:7px;background:#f1f5f9;color:#8390a2;font-size:11px;font-weight:750;font-variant-numeric:tabular-nums}.rank.gold{background:linear-gradient(135deg,#fbbf24,#f59e0b);color:#fff}.rank.silver{background:linear-gradient(135deg,#cbd5e1,#94a3b8);color:#fff}.rank.bronze{background:linear-gradient(135deg,#d97706,#b45309);color:#fff}.rank-row strong,.account-row strong,.product-row strong{color:#263449;font-size:13px;white-space:nowrap;font-variant-numeric:tabular-nums}.balance{font-variant-numeric:tabular-nums}.row-figures{display:flex;flex-direction:column;align-items:flex-end;gap:2px;white-space:nowrap}.row-figures small{color:#64748b;font-size:10px;font-weight:600}.row-figures small.partial{color:#c66a17}.expand-toggle{display:block;width:100%;margin-top:10px;padding:8px 0;border:0;border-radius:9px;background:#eef2f7;color:#425466;font-size:12px;font-weight:700;cursor:pointer}.expand-toggle:hover{background:#e4eaf4}.balance{display:block;margin-top:16px;color:#172033;font-size:26px;letter-spacing:-.04em}.accounts{margin-top:10px}.account-row{justify-content:space-between}.category-row{gap:12px}.category-bar{flex:1.2;min-width:0;height:8px;border-radius:999px;background:#eef2f7;overflow:hidden}.category-bar i{display:block;height:100%;border-radius:999px;background:linear-gradient(90deg,#635bff,#8b85ff)}.row.clickable{cursor:pointer}.row.clickable:hover .item-name{color:#635bff}.empty{margin:28px 0 6px;color:#94a3b8;font-size:13px}
@media(max-width:1100px){.detail-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.detail-grid .panel:nth-child(n+3){grid-column:1/-1}}
@media(max-width:767px){.detail-grid{grid-template-columns:1fr;gap:12px}.detail-grid .panel:nth-child(n+3){grid-column:auto}.panel{padding:15px}.category-bar{flex:0.8}}
</style>
