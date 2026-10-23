<script setup>
import { computed } from 'vue'
import { formatCurrency } from '../../utils/dashboard'

const props = defineProps({ summary: { type: Object, required: true }, totalBalance: { type: Number, default: 0 } })
const emit = defineEmits(['navigate'])
const rankings = computed(() => (props.summary.employee_rankings || []).slice(0, 5))
const accounts = computed(() => (props.summary.accounts || []).slice(0, 5))
</script>

<template>
  <div class="detail-grid">
    <section class="panel" aria-labelledby="ranking-title">
      <div class="panel-heading"><div><span class="eyebrow">TEAM</span><h2 id="ranking-title">本期业绩</h2></div><button class="more" @click="emit('navigate', '/admin/employees')">员工管理 ↗</button></div>
      <div v-if="rankings.length" class="rows">
        <div v-for="(item, index) in rankings" :key="item.employee_id ?? index" class="row rank-row">
          <span class="rank" :class="{ first: index === 0 }">{{ String(index + 1).padStart(2, '0') }}</span>
          <span class="item-name">{{ item.name }}<small>{{ item.order_count || 0 }} 单</small></span>
          <strong>{{ formatCurrency(item.revenue) }}</strong>
        </div>
      </div>
      <p v-else class="empty">所选期间暂无员工业绩</p>
    </section>
    <section class="panel" aria-labelledby="account-title">
      <div class="panel-heading"><div><span class="eyebrow">CASH POSITION</span><h2 id="account-title">账户余额</h2></div><button class="more" @click="emit('navigate', '/admin/finance')">资金工作台 ↗</button></div>
      <strong class="balance">{{ formatCurrency(totalBalance) }}</strong>
      <div v-if="accounts.length" class="rows accounts">
        <div v-for="item in accounts" :key="item.id" class="row account-row"><span>{{ item.name }}</span><strong>{{ formatCurrency(item.balance) }}</strong></div>
      </div>
      <p v-else class="empty">暂无账户数据</p>
    </section>
  </div>
</template>

<style scoped>
.detail-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.panel{min-width:0;background:#fff;border:1px solid #e8edf3;border-radius:18px;padding:22px}.panel-heading{display:flex;justify-content:space-between;align-items:flex-end;gap:12px}.eyebrow{font-size:10px;font-weight:800;letter-spacing:.14em;color:#94a3b8}h2{margin:5px 0 0;color:#172033;font-size:18px;font-weight:750}.more{border:0;background:none;color:#2563eb;font-size:12px;font-weight:650;white-space:nowrap;cursor:pointer}.rows{margin-top:17px}.row{display:flex;align-items:center;gap:10px;min-width:0;padding:11px 0;border-bottom:1px solid #f0f3f7;color:#475569;font-size:13px}.row:last-child{border-bottom:0}.item-name{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#263449;font-weight:650}.item-name small{display:block;margin-top:2px;color:#94a3b8;font-size:11px;font-weight:400}.rank{color:#aab5c2;font-weight:750}.rank.first{color:#2563eb}.rank-row strong,.account-row strong{color:#263449;font-size:13px;white-space:nowrap}.balance{display:block;margin-top:18px;color:#172033;font-size:26px;letter-spacing:-.04em}.accounts{margin-top:10px}.account-row{justify-content:space-between}.empty{margin:28px 0 6px;color:#94a3b8;font-size:13px}
@media(max-width:1100px){.detail-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.detail-grid .panel:last-child{grid-column:1/-1}}
@media(max-width:767px){.detail-grid{grid-template-columns:1fr;gap:12px}.detail-grid .panel:last-child{grid-column:auto}.panel{padding:18px}}
</style>
