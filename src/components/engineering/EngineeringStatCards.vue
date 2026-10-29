<template>
  <!--
    6 张汇总卡（SPEC-ENG-001 §4.3）
    数字全部来自 GET /summary，前端不做任何加减乘除（TASK-ENG-001 T11 禁区）。
    第 5、6 张口径不同（开销含公共费用 / 净利不含），副文案与 tooltip 必须写明。
  -->
  <div class="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
    <component
      :is="card.clickable ? 'button' : 'div'"
      v-for="card in cards"
      :key="card.key"
      :type="card.clickable ? 'button' : undefined"
      class="eng-stat-card"
      :class="card.clickable ? 'eng-stat-card--clickable' : ''"
      :title="card.tip"
      @click="card.clickable && card.onClick && card.onClick()"
    >
      <p class="eng-stat-title">{{ card.title }}</p>
      <p class="eng-stat-value" :style="card.danger && card.hasValue ? { color: '#dc2626' } : null">
        <span v-if="loading && !summary" class="eng-skeleton eng-skeleton--value" />
        <template v-else>{{ card.value }}</template>
      </p>
      <p class="eng-stat-sub">
        <span v-if="loading && !summary" class="eng-skeleton eng-skeleton--sub" />
        <template v-else>{{ card.sub }}</template>
      </p>
    </component>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { formatCardMoney, formatPercent } from '../../utils/engineeringFormat'

const props = defineProps({
  summary: { type: Object, default: null },
  loading: { type: Boolean, default: false }
})

const emit = defineEmits(['pick-active', 'pick-pending', 'pick-overdue', 'open-expenses'])

const s = computed(() => props.summary || {})

const cards = computed(() => {
  const d = s.value
  const summaryEmpty = !d || Object.keys(d).length === 0
  return [
    {
      key: 'active',
      title: '在跟项目',
      value: summaryEmpty ? '—' : `${Number(d.active_count) || 0} 个`,
      sub: summaryEmpty ? '—' : `合同额 ${formatCardMoney(d.active_amount)}（含跟进中预估）`,
      tip: '状态为 跟进中 / 已签约 / 供货中 的项目；合同额含跟进阶段的预估额',
      clickable: true,
      onClick: () => emit('pick-active')
    },
    {
      key: 'pending',
      title: '待回款',
      value: summaryEmpty ? '—' : formatCardMoney(d.pending_amount),
      sub: summaryEmpty ? '—' : `覆盖 ${Number(d.pending_project_count) || 0} 个项目`,
      tip: 'Σ(合同额 − 已实收)，统计状态 已签约 / 供货中 / 已验收',
      clickable: true,
      onClick: () => emit('pick-pending')
    },
    {
      key: 'overdue',
      title: '逾期',
      value: summaryEmpty ? '—' : formatCardMoney(d.overdue_amount),
      sub: summaryEmpty ? '—' : `${Number(d.overdue_term_count) || 0} 个节点逾期`,
      tip: '计划回款日期已过但仍未收齐的节点，未收金额合计',
      clickable: true,
      danger: Number(d.overdue_amount) > 0,
      hasValue: true,
      onClick: () => emit('pick-overdue')
    },
    {
      key: 'yearReceived',
      title: '本年实收',
      value: summaryEmpty ? '—' : formatCardMoney(d.year_received),
      sub: summaryEmpty ? '—' : `本年 ${Number(d.year_receipt_count) || 0} 笔`,
      tip: '本自然年登记的回款合计（含冲销负数）',
      clickable: false
    },
    {
      key: 'yearExpense',
      title: '本年工程开销',
      value: summaryEmpty ? '—' : formatCardMoney(d.year_expense),
      sub: '含公共费用 · 点击看台账',
      tip: '本自然年工程费用合计（项目费用 + 公共费用，已付未付都算）',
      clickable: true,
      onClick: () => emit('open-expenses')
    },
    {
      key: 'netMargin',
      title: '平均净利率',
      value: summaryEmpty ? '—' : formatPercent(d.avg_net_margin_rate),
      sub: '已签约项目，未含公共费用',
      tip: '净利率 = (合同额 − 设备成本 − 本项目费用) / 合同额，公共费用未分摊到项目',
      clickable: false,
      danger: Number(d.avg_net_margin_rate) < 0,
      hasValue: true
    }
  ]
})
</script>

<style scoped>
.eng-stat-card {
  display: block;
  width: 100%;
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 14px 16px;
  text-align: left;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}
.eng-stat-card--clickable {
  cursor: pointer;
}
.eng-stat-card--clickable:hover {
  border-color: #e2e8f0;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.06);
}
.eng-stat-title {
  font-size: 12px;
  color: #64748b;
  line-height: 18px;
}
.eng-stat-value {
  margin-top: 4px;
  font-size: 20px;
  font-weight: 900;
  color: #0f172a;
  line-height: 28px;
  font-variant-numeric: tabular-nums;
}
.eng-stat-sub {
  margin-top: 2px;
  font-size: 11px;
  line-height: 16px;
  color: #94a3b8;
}
.eng-skeleton {
  display: inline-block;
  border-radius: 6px;
  background: #f1f5f9;
}
.eng-skeleton--value {
  width: 68px;
  height: 18px;
}
.eng-skeleton--sub {
  width: 100px;
  height: 10px;
}
</style>
