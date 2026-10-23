<script setup>
defineProps({
  operations: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  undoingId: { type: Number, default: null }
})

const emit = defineEmits(['undo'])

function formatTime(value) {
  if (!value) return '时间未知'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(new Date(value))
}
</script>

<template>
  <div v-loading="loading" class="history-list">
    <el-empty v-if="!loading && operations.length === 0" description="暂无可撤销的历史操作" />

    <div v-else class="space-y-3">
      <article
        v-for="operation in operations"
        :key="operation.id"
        class="history-card"
      >
        <div class="history-icon" :class="operation.undone_at ? 'history-icon-undone' : ''">
          <el-icon :size="19"><Clock /></el-icon>
        </div>

        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
            <h3 class="history-title">{{ operation.title }}</h3>
            <el-tag v-if="operation.undone_at" size="small" type="info" effect="plain">已撤销</el-tag>
          </div>
          <p class="history-meta">
            {{ operation.detail || `${operation.username || '系统'}于${formatTime(operation.created_at)}完成该操作，相关数据已更新` }}
          </p>
        </div>

        <div class="flex-shrink-0">
          <el-button
            v-if="operation.can_undo"
            type="primary"
            plain
            round
            :loading="undoingId === operation.id"
            :disabled="undoingId !== null && undoingId !== operation.id"
            @click="emit('undo', operation)"
          >
            撤销
          </el-button>
          <span v-else-if="operation.undone_at" class="history-status">已恢复到操作前</span>
        </div>
      </article>
    </div>
  </div>
</template>

<style scoped>
.history-list {
  min-height: 280px;
}

.history-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px 18px;
  border: 1px solid #e2e8f0;
  border-radius: 18px;
  background: rgb(255 255 255 / 92%);
}

.history-icon {
  display: flex;
  width: 42px;
  height: 42px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 14px;
  background: #eff6ff;
  color: #2563eb;
}

.history-icon-undone {
  background: #f1f5f9;
  color: #94a3b8;
}

.history-title {
  margin: 0;
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
}

.history-meta {
  margin: 5px 0 0;
  color: #94a3b8;
  font-size: 12px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}

.history-status {
  color: #94a3b8;
  font-size: 12px;
}

@media (max-width: 520px) {
  .history-card {
    gap: 10px;
    padding: 13px;
  }

  .history-icon {
    width: 36px;
    height: 36px;
  }
}
</style>
