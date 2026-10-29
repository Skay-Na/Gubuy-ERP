<template>
  <!--
    工程状态标签（全模块唯一色值来源，SPEC-ENG-001 §3.2）
    冻结契约（TASK-ENG-001 §2.3）：props 只有 status，无 emits，禁止外部传色值。
    其它页面/Tab 一律使用本组件，不得再写一份色值映射。
  -->
  <span class="eng-status-tag" :style="tagStyle">{{ label }}</span>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  status: { type: Number, required: true }
})

// §3.2 状态色：1 跟进中 / 2 已签约 / 3 供货中 / 4 已验收 / 5 已结清 / 9 已作废
const STATUS_MAP = {
  1: { label: '跟进中', color: '#64748b', bg: '#f1f5f9', strike: false },
  2: { label: '已签约', color: '#2563eb', bg: '#eff6ff', strike: false },
  3: { label: '供货中', color: '#d97706', bg: '#fffbeb', strike: false },
  4: { label: '已验收', color: '#635bff', bg: '#eef0ff', strike: false },
  5: { label: '已结清', color: '#15803d', bg: '#e8f7ee', strike: false },
  9: { label: '已作废', color: '#94a3b8', bg: '#f8fafc', strike: true }
}

const meta = computed(() => STATUS_MAP[props.status] || { label: '—', color: '#94a3b8', bg: '#f8fafc', strike: false })
const label = computed(() => meta.value.label)
const tagStyle = computed(() => ({
  color: meta.value.color,
  background: meta.value.bg,
  textDecoration: meta.value.strike ? 'line-through' : 'none'
}))
</script>

<style scoped>
.eng-status-tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  line-height: 18px;
  white-space: nowrap;
}
</style>
