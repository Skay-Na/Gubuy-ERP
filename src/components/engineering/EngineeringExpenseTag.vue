<template>
  <!--
    工程费用类别标签（全模块唯一色相来源，SPEC-ENG-001 §5.7）
    冻结契约（TASK-ENG-001 §2.3）：props 只有 category，无 emits。
    同文件导出 EXPENSE_CATEGORIES，其它组件（费用弹窗、台账页、筛选条）一律引用它，
    禁止再抄一份 7 类字典。
  -->
  <span class="eng-expense-tag">
    <span class="eng-expense-dot" :style="{ background: meta.color }" />
    {{ meta.label }}
  </span>
</template>

<script>
// 7 类固定色相（§5.7 表）：值不可随意改动，颜色语义已与设计稿对齐
export const EXPENSE_CATEGORIES = [
  { value: 1, label: '安装人工', color: '#635BFF' },
  { value: 2, label: '运输物流', color: '#0EA5E9' },
  { value: 3, label: '辅材耗材', color: '#F59E0B' },
  { value: 4, label: '现场施工', color: '#10B981' },
  { value: 5, label: '差旅招待', color: '#8B5CF6' },
  { value: 6, label: '外包服务', color: '#EC4899' },
  { value: 7, label: '其他', color: '#94A3B8' }
]

export const EXPENSE_CATEGORY_MAP = EXPENSE_CATEGORIES.reduce((acc, item) => {
  acc[item.value] = item
  return acc
}, {})

// 付款方式字典（仅备查，不动任何账户余额）
export const EXPENSE_PAY_METHODS = [
  { value: 0, label: '未指定' },
  { value: 1, label: '微信' },
  { value: 2, label: '支付宝' },
  { value: 3, label: '公户' },
  { value: 4, label: '现金' },
  { value: 5, label: '其他' }
]
</script>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  category: { type: Number, required: true }
})

const meta = computed(() => EXPENSE_CATEGORY_MAP[props.category] || { label: '—', color: '#94A3B8' })
</script>

<style scoped>
.eng-expense-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #334155;
  white-space: nowrap;
}
.eng-expense-dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  flex: 0 0 auto;
}
</style>
