<script setup>
import { computed } from 'vue'
import { ArrowRight, Coin, Money, WarningFilled, Wallet } from '@element-plus/icons-vue'

const props = defineProps({
  summary: { type: Object, required: true },
  totalBalance: { type: Number, required: true },
  netCashFlow: { type: Number, required: true },
  attentionCount: { type: Number, required: true },
  periodLabel: { type: String, required: true },
  loading: Boolean
})

const emit = defineEmits(['open-orders', 'open-subsidy', 'open-bookkeeping', 'open-calibrate'])

const accounts = computed(() => props.summary.accounts || [])
const formatCurrency = (value) => `¥${Number(value || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
</script>

<template>
  <section v-loading="loading" class="finance-workspace">
    <div class="hero-grid">
      <article class="cash-hero">
        <div class="hero-label"><el-icon><Wallet /></el-icon> 账面资金</div>
        <strong>{{ formatCurrency(totalBalance) }}</strong>
        <span>{{ accounts.length }} 个资金账户 · 含期初与校准金额，不代表利润</span>
      </article>
      <article class="hero-metric">
        <div class="hero-label"><el-icon><Coin /></el-icon> {{ periodLabel }}净现金流</div>
        <strong :class="netCashFlow >= 0 ? 'positive' : 'negative'">{{ netCashFlow >= 0 ? '+' : '' }}{{ formatCurrency(netCashFlow) }}</strong>
        <span>流入 {{ formatCurrency(summary.total_income) }} · 流出 {{ formatCurrency(summary.total_expense) }}</span>
      </article>
      <article class="hero-metric profit">
        <div class="hero-label"><el-icon><Money /></el-icon> {{ periodLabel }}经营净利润</div>
        <strong v-if="!summary.cost_incomplete" :class="summary.net_profit >= 0 ? 'positive' : 'negative'">{{ summary.net_profit >= 0 ? '+' : '' }}{{ formatCurrency(summary.net_profit) }}</strong>
        <strong v-else class="negative">待核算</strong>
        <span v-if="summary.cost_incomplete">{{ summary.uncosted_order_item_count || 0 }} 条订单明细成本待核，暂不展示毛利</span>
        <span v-else>毛利 {{ formatCurrency(summary.gross_profit) }} · 经营费用 {{ formatCurrency(summary.operating_expense) }}</span>
      </article>
      <article class="hero-metric attention">
        <div class="hero-label"><el-icon><WarningFilled /></el-icon> 待处理事项</div>
        <strong>{{ attentionCount }} <small>项</small></strong>
        <span>优先处理待收与待回款</span>
      </article>
    </div>

    <div class="workspace-grid">
      <article class="workspace-card receivable-card">
        <div class="card-heading"><div><span>资金待办</span><h3>哪些钱需要跟进？</h3></div><el-icon><Money /></el-icon></div>
        <button class="task-row" @click="emit('open-orders')">
          <div><b>待收尾款</b><small>{{ summary.pending_balance_count || 0 }} 笔待结清订单</small></div>
          <strong>{{ formatCurrency(summary.pending_balance) }}</strong><el-icon><ArrowRight /></el-icon>
        </button>
        <button class="task-row" @click="emit('open-subsidy')">
          <div><b>待回国补</b><small>已申报、尚未实际回款</small></div>
          <strong>{{ formatCurrency(summary.pending_subsidy) }}</strong><el-icon><ArrowRight /></el-icon>
        </button>
      </article>

      <article class="workspace-card accounts-card">
        <div class="card-heading"><div><span>资金账户</span><h3>钱现在在哪里？</h3></div><button class="text-action" @click="emit('open-bookkeeping')">查看账目</button></div>
        <div v-if="accounts.length" class="account-list">
          <button v-for="account in accounts" :key="account.id" class="account-row" @click="emit('open-calibrate', account)">
            <div class="account-avatar">{{ account.name.slice(0, 1) }}</div>
            <div><b>{{ account.name }}</b><small>{{ account.is_default_expense ? '默认支出账户' : '点击核对余额' }}</small></div>
            <strong>{{ formatCurrency(account.balance) }}</strong>
          </button>
        </div>
        <el-empty v-else description="还没有资金账户" :image-size="56" />
      </article>
    </div>
  </section>
</template>

<style scoped>
.finance-workspace { display: grid; gap: 18px; }
.hero-grid { display: grid; grid-template-columns: 1.2fr 1fr 1fr 1fr; gap: 14px; }
.cash-hero, .hero-metric { min-height: 154px; padding: 22px; border-radius: 20px; display: flex; flex-direction: column; justify-content: space-between; }
.cash-hero { color: white; background: linear-gradient(135deg, #172554, #2563eb); box-shadow: 0 14px 30px rgba(37, 99, 235, .2); }
.hero-metric { background: #fff; border: 1px solid #e8edf5; }
.hero-metric.attention { background: #fffbeb; border-color: #fde68a; }.hero-metric.profit { background: #f0fdf4; border-color: #bbf7d0; }
.hero-label { display: flex; align-items: center; gap: 7px; color: #64748b; font-size: 12px; font-weight: 700; }
.cash-hero .hero-label, .cash-hero span { color: rgba(255, 255, 255, .72); }
.cash-hero strong, .hero-metric strong { font-size: 28px; letter-spacing: -.04em; }
.hero-metric strong { color: #0f172a; }.hero-metric strong.positive { color: #059669; }.hero-metric strong.negative { color: #e11d48; }
.hero-metric small { font-size: 14px; }.cash-hero span, .hero-metric span { font-size: 12px; color: #94a3b8; }
.workspace-grid { display: grid; grid-template-columns: .9fr 1.1fr; gap: 14px; }
.workspace-card { padding: 20px; background: #fff; border: 1px solid #e8edf5; border-radius: 18px; }
.card-heading { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: 14px; }.card-heading span { color: #64748b; font-size: 12px; }.card-heading h3 { margin: 3px 0 0; color: #1e293b; font-size: 16px; }.card-heading > .el-icon { color: #94a3b8; font-size: 20px; }
.task-row, .account-row { width: 100%; display: grid; align-items: center; border: 0; background: transparent; text-align: left; cursor: pointer; }
.task-row { grid-template-columns: 1fr auto 18px; gap: 10px; padding: 14px 0; border-top: 1px solid #f1f5f9; }.task-row b, .account-row b { display: block; color: #334155; font-size: 14px; }.task-row small, .account-row small { display: block; margin-top: 3px; color: #94a3b8; font-size: 11px; }.task-row strong { color: #0f172a; font-size: 15px; }.task-row .el-icon { color: #94a3b8; }
.account-list { display: grid; gap: 6px; }.account-row { grid-template-columns: 34px 1fr auto; gap: 10px; padding: 9px 8px; border-radius: 12px; }.account-row:hover { background: #f8fafc; }.account-avatar { display: grid; width: 32px; height: 32px; place-items: center; border-radius: 10px; color: #2563eb; background: #eff6ff; font-size: 13px; font-weight: 800; }.account-row strong { color: #0f172a; font-size: 14px; }.text-action { border: 0; background: transparent; color: #2563eb; cursor: pointer; font-size: 12px; font-weight: 700; }
@media (max-width: 760px) { .hero-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }.workspace-grid { grid-template-columns: 1fr; gap: 10px; }.cash-hero, .hero-metric.attention { grid-column: 1 / -1; }.cash-hero { min-height: 122px; }.hero-metric.attention { min-height: 94px; }.hero-metric { min-height: 118px; padding: 15px; border-radius: 16px; }.cash-hero { padding: 18px; border-radius: 18px; }.cash-hero strong { font-size: 28px; }.hero-metric strong { font-size: 20px; }.hero-metric span { font-size: 10px; line-height: 1.4; }.workspace-card { padding: 16px; border-radius: 16px; }.task-row { min-height: 64px; }.account-row { min-height: 52px; padding: 10px 6px; }.account-row strong { font-size: 13px; }.account-avatar { width: 34px; height: 34px; } }
</style>
