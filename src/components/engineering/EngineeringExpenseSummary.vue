<template>
  <!--
    工程费用汇总条 + 类别分布 chip（SPEC-ENG-001 §5.7 / §9.5）
    项目详情「费用 Tab」与「工程开销台账页」共用：
      - showYear=true  台账页用（多一个"本年"指标）
      - showYear=false 项目 Tab 用（项目维度没有"本年"概念）
  -->
  <div class="eng-exp-summary">
    <div class="eng-exp-summary-row">
      <div class="eng-exp-metrics">
        <div class="eng-exp-metric eng-exp-metric--main">
          <p class="eng-exp-label">费用合计</p>
          <p class="eng-exp-value">{{ formatCardMoney(summary.total) }}</p>
        </div>
        <div class="eng-exp-metric">
          <p class="eng-exp-label">已付</p>
          <p class="eng-exp-value" style="color: #15803d">{{ formatCardMoney(summary.paid) }}</p>
        </div>
        <div class="eng-exp-metric">
          <p class="eng-exp-label">未付</p>
          <p class="eng-exp-value" style="color: #d97706">{{ formatCardMoney(summary.unpaid) }}</p>
          <p class="eng-exp-sub">挂账待付</p>
        </div>
        <div v-if="showYear" class="eng-exp-metric">
          <p class="eng-exp-label">本年开销</p>
          <p class="eng-exp-value">{{ formatCardMoney(summary.year_total) }}</p>
          <p class="eng-exp-sub">固定口径 · 不受筛选影响</p>
        </div>
      </div>
      <div class="eng-exp-actions">
        <slot name="actions" />
      </div>
    </div>

    <!-- 类别分布：金额为 0 的类目灰显且不可点（§5.7） -->
    <div v-if="byCategory.length" class="eng-exp-chips">
      <button
        v-for="chip in chips"
        :key="chip.category"
        type="button"
        class="eng-exp-chip"
        :class="[
          chip.disabled ? 'eng-exp-chip--disabled' : '',
          activeCategory === chip.category ? 'eng-exp-chip--active' : ''
        ]"
        :disabled="chip.disabled"
        @click="emit('pick-category', activeCategory === chip.category ? null : chip.category)"
      >
        <span class="eng-exp-chip-name">{{ chip.label }}</span>
        <span class="eng-exp-chip-amount">{{ formatCardMoney(chip.amount) }}</span>
        <span v-if="chip.percent !== null" class="eng-exp-chip-pct">{{ chip.percent }}%</span>
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { EXPENSE_CATEGORY_MAP } from './EngineeringExpenseTag.vue'
import { formatCardMoney } from '../../utils/engineeringFormat'

const props = defineProps({
  summary: { type: Object, required: true },
  byCategory: { type: Array, default: () => [] },
  showYear: { type: Boolean, default: true },
  activeCategory: { type: Number, default: null }
})

const emit = defineEmits(['pick-category'])

const total = computed(() => Number(props.summary?.total) || 0)

// 恒 7 项：后端 by_category 已补零，这里只做展示映射与占比
const chips = computed(() =>
  props.byCategory.map(row => {
    const amount = Number(row.amount) || 0
    const meta = EXPENSE_CATEGORY_MAP[row.category] || { label: row.category_label || '—' }
    return {
      category: row.category,
      label: meta.label || row.category_label,
      amount,
      percent: total.value > 0 && amount > 0 ? Math.round((amount / total.value) * 100) : null,
      disabled: amount <= 0
    }
  })
)
</script>

<style scoped>
.eng-exp-summary {
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 14px 16px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.eng-exp-summary-row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
}
.eng-exp-metrics {
  display: flex;
  flex-wrap: wrap;
  gap: 26px;
}
.eng-exp-label {
  font-size: 11px;
  color: #94a3b8;
  line-height: 16px;
}
.eng-exp-value {
  font-size: 16px;
  font-weight: 700;
  color: #0f172a;
  line-height: 24px;
  font-variant-numeric: tabular-nums;
}
.eng-exp-metric--main .eng-exp-value {
  font-size: 18px;
  font-weight: 900;
}
.eng-exp-sub {
  font-size: 10px;
  color: #cbd5e1;
  line-height: 14px;
}
.eng-exp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}
.eng-exp-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border-radius: 999px;
  border: 1px solid #f1f5f9;
  background: #f8fafc;
  padding: 4px 10px;
  font-size: 12px;
  color: #475569;
  cursor: pointer;
  transition: all 0.15s ease;
}
.eng-exp-chip:hover:not(.eng-exp-chip--disabled) {
  border-color: #ddd9ff;
}
.eng-exp-chip--disabled {
  color: #cbd5e1;
  cursor: not-allowed;
}
.eng-exp-chip--active {
  background: #635bff;
  border-color: #635bff;
  color: #fff;
}
.eng-exp-chip-name {
  font-weight: 500;
}
.eng-exp-chip-amount {
  font-variant-numeric: tabular-nums;
}
.eng-exp-chip-pct {
  font-size: 10px;
  opacity: 0.75;
}
@media (max-width: 767px) {
  .eng-exp-metrics {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
    width: 100%;
  }
  .eng-exp-actions {
    width: 100%;
  }
}
</style>
