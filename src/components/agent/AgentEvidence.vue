<script setup>
import { computed } from 'vue'
const props = defineProps({ evidence: { type: String, default: '' }, status: { type: String, default: '' } })
const items = computed(() => { try { const data = JSON.parse(props.evidence || '[]'); return Array.isArray(data) ? data : [] } catch { return [] } })
</script>

<template>
  <div v-if="status === 'incomplete'" class="mt-2 text-xs text-amber-700">本次分析未完整完成</div>
  <details v-if="items.length" class="mt-2 border-t pt-2 text-xs text-slate-500">
    <summary class="cursor-pointer">查看查询依据（{{ items.length }} 项）</summary>
    <section v-for="item in items" :key="item.id" class="mt-3">
      <p class="font-medium">{{ item.id }} · {{ item.source }}</p>
      <p v-for="period in item.periods || []" :key="period.start + period.end">{{ period.label }} {{ period.start }} 至 {{ period.end }}</p>
      <details class="mt-1"><summary class="cursor-pointer">原始查询详情</summary><pre class="max-h-64 overflow-auto whitespace-pre-wrap break-all rounded bg-slate-50 p-2">{{ item.result }}</pre></details>
    </section>
  </details>
</template>
