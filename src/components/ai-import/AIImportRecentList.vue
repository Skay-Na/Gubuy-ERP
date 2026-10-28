<script setup>
defineProps({ items: { type: Array, default: () => [] }, deletingId: { type: Number, default: null } })
const emit = defineEmits(['select', 'delete'])

function formatTime(value) {
  return new Date(value).toLocaleString('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}
</script>

<template>
  <section v-if="items.length" class="recent-panel">
    <div class="recent-heading"><div><span>最近导入</span><small>继续核对未完成的草稿</small></div><b>{{ items.length }}</b></div>
    <div class="recent-scroll">
      <div v-for="item in items" :key="item.id" class="recent-item">
        <button class="recent-open" @click="emit('select', item.id)"><span class="recent-icon">{{ item.kind === 'expense' ? '账' : '单' }}</span><span class="recent-copy"><strong>草稿 #{{ item.id }}</strong><small>{{ formatTime(item.created_at) }}</small></span><span class="recent-status" :class="{ saved: item.status === 'saved' }">{{ item.status === 'saved' ? '已保存' : '待核对' }}</span></button>
        <button class="recent-delete" :disabled="deletingId === item.id" :aria-label="`删除草稿 ${item.id}`" @click="emit('delete', item)">{{ deletingId === item.id ? '…' : '×' }}</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.recent-panel{width:100%;min-width:0;max-width:100%;padding:18px;border:1px solid #e2e8f0;border-radius:18px;background:#fff;box-shadow:0 12px 32px rgba(15,23,42,.06);box-sizing:border-box;overflow:hidden}.recent-heading{display:flex;min-width:0;align-items:center;justify-content:space-between;margin-bottom:12px}.recent-heading div{min-width:0;display:grid;gap:2px}.recent-heading span{font-weight:800;color:#0f172a}.recent-heading small{color:#94a3b8}.recent-heading b{display:grid;width:28px;height:28px;place-items:center;border-radius:9px;background:#eef0ff;color:#635bff}.recent-scroll{min-width:0;max-width:100%;display:grid;gap:8px}.recent-item{position:relative;display:flex;width:100%;min-width:0;border-radius:12px;background:#f8fafc;overflow:hidden}.recent-item:hover{background:#eef0ff}.recent-open{display:flex;min-width:0;flex:1;align-items:center;gap:10px;padding:10px 38px 10px 10px;border:0;background:transparent;text-align:left;cursor:pointer}.recent-delete{position:absolute;top:50%;right:7px;display:grid;width:28px;height:28px;place-items:center;transform:translateY(-50%);border:0;border-radius:9px;background:#fff;color:#94a3b8;font-size:20px;line-height:1;cursor:pointer}.recent-delete:hover{background:#fef2f2;color:#dc2626}.recent-delete:disabled{cursor:wait;opacity:.55}.recent-icon{display:grid;width:34px;height:34px;flex:0 0 auto;place-items:center;border-radius:10px;background:#e8e9ff;color:#635bff;font-weight:800}.recent-copy{display:grid;min-width:0;flex:1}.recent-copy strong,.recent-copy small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.recent-copy strong{color:#334155}.recent-copy small{color:#94a3b8}.recent-status{flex:none;padding:4px 8px;border-radius:999px;background:#fff7ed;color:#ea580c;font-size:11px;font-weight:700}.recent-status.saved{background:#ecfdf5;color:#059669}@media(max-width:760px){.recent-panel{padding:14px}.recent-scroll{display:flex;overflow-x:auto;overscroll-behavior-inline:contain}.recent-item{width:230px;min-width:230px}}
</style>
