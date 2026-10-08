<script setup>
import { computed } from 'vue'
import { Bottom, Top, TrendCharts, Wallet } from '@element-plus/icons-vue'

const props = defineProps({
  summary: { type: Object, default: () => ({}) },
  netAmount: { type: Number, default: 0 },
  totalBalance: { type: Number, default: 0 },
  periodLabel: { type: String, default: '本月' },
  loading: Boolean
})

const cards = computed(() => [
  {
    key: 'income',
    label: `${props.periodLabel}收入`,
    value: Number(props.summary.total_income || 0),
    helper: '销售回款与手工收入',
    icon: Top,
    tone: 'income'
  },
  {
    key: 'expense',
    label: `${props.periodLabel}支出`,
    value: Number(props.summary.total_expense || 0),
    helper: '采购、经营与手工支出',
    icon: Bottom,
    tone: 'expense'
  },
  {
    key: 'net',
    label: `${props.periodLabel}净流入`,
    value: props.netAmount,
    helper: '期间收入减去期间支出',
    icon: TrendCharts,
    tone: props.netAmount >= 0 ? 'net-positive' : 'net-negative'
  },
  {
    key: 'balance',
    label: '账户总余额',
    value: props.totalBalance,
    helper: `${props.summary.accounts?.length || 0} 个资金账户`,
    icon: Wallet,
    tone: 'balance'
  }
])

function formatCurrency(value) {
  return `¥${Number(value || 0).toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })}`
}
</script>

<template>
  <section v-loading="loading" class="summary-grid" aria-label="记账汇总">
    <article v-for="card in cards" :key="card.key" class="summary-card" :class="card.tone">
      <div class="summary-card-heading">
        <span>{{ card.label }}</span>
        <el-icon><component :is="card.icon" /></el-icon>
      </div>
      <strong>{{ formatCurrency(card.value) }}</strong>
      <small>{{ card.helper }}</small>
    </article>
  </section>
</template>

<style scoped>
.summary-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; min-height: 126px; }
.summary-card { display: flex; min-width: 0; min-height: 126px; flex-direction: column; justify-content: space-between; padding: 18px; border: 1px solid #e2e8f0; border-radius: 18px; background: #fff; }
.summary-card-heading { display: flex; align-items: center; justify-content: space-between; gap: 10px; color: #64748b; font-size: 12px; font-weight: 800; }
.summary-card-heading .el-icon { display: grid; width: 30px; height: 30px; place-items: center; border-radius: 10px; background: #f1f5f9; font-size: 16px; }
.summary-card strong { overflow: hidden; color: #0f172a; font-size: clamp(20px, 2vw, 28px); letter-spacing: -.04em; text-overflow: ellipsis; }
.summary-card small { color: #94a3b8; font-size: 11px; }
.summary-card.income { border-color: #bbf7d0; background: linear-gradient(145deg, #f0fdf4, #fff); }
.summary-card.income .summary-card-heading .el-icon, .summary-card.net-positive .summary-card-heading .el-icon { color: #047857; background: #d1fae5; }
.summary-card.expense { border-color: #fecdd3; background: linear-gradient(145deg, #fff1f2, #fff); }
.summary-card.expense .summary-card-heading .el-icon, .summary-card.net-negative .summary-card-heading .el-icon { color: #be123c; background: #ffe4e6; }
.summary-card.balance { color: #fff; border-color: #1e293b; background: linear-gradient(135deg, #0f172a, #334155); }
.summary-card.balance strong { color: #fff; }.summary-card.balance .summary-card-heading, .summary-card.balance small { color: #cbd5e1; }.summary-card.balance .summary-card-heading .el-icon { color: #fff; background: rgba(255, 255, 255, .12); }
@media (max-width: 920px) { .summary-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 560px) { .summary-grid { gap: 9px; }.summary-card { min-height: 110px; padding: 14px; border-radius: 15px; }.summary-card strong { font-size: 19px; }.summary-card small { font-size: 10px; }.summary-card-heading .el-icon { width: 27px; height: 27px; } }
</style>
