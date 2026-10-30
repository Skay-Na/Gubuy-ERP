<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessageBox } from 'element-plus'
import { useOperationHistory } from '../composables/useOperationHistory'
import { OPERATION_CATEGORIES } from '../constants/operationCategories'
import OperationHistoryList from '../components/history/OperationHistoryList.vue'

const { operations, loading, undoingId, loadOperations, undoOperation } = useOperationHistory()

const activeCategory = ref('all')
const keyword = ref('')

// 各分类的数量徽标，只展示有记录的分类，避免空分类干扰
const categoryCounts = computed(() => {
  const counts = {}
  for (const operation of operations.value) {
    const key = operation.category || 'other'
    counts[key] = (counts[key] || 0) + 1
  }
  return counts
})

const visibleCategories = computed(() =>
  OPERATION_CATEGORIES.filter((item) => (categoryCounts.value[item.key] || 0) > 0)
)

const filteredOperations = computed(() => {
  const text = keyword.value.trim().toLowerCase()
  return operations.value.filter((operation) => {
    if (activeCategory.value !== 'all' && (operation.category || 'other') !== activeCategory.value) {
      return false
    }
    if (!text) return true
    return [operation.title, operation.detail, operation.username]
      .some((field) => (field || '').toLowerCase().includes(text))
  })
})

async function confirmUndo(operation) {
  try {
    await ElMessageBox.confirm(
      `确定撤销“${operation.title}”吗？系统会检查相关数据是否已被后续操作修改。`,
      '撤销历史操作',
      {
        confirmButtonText: '确认撤销',
        cancelButtonText: '保留操作',
        type: 'warning'
      }
    )
  } catch {
    return
  }

  await undoOperation(operation)
}

onMounted(loadOperations)
</script>

<template>
  <section class="mx-auto max-w-5xl space-y-6 pb-10">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="mb-2 text-xs font-bold uppercase tracking-[0.18em] text-blue-600">操作记录</p>
        <h1 class="text-2xl font-bold tracking-tight text-slate-900 md:text-3xl">历史操作</h1>
        <p class="mt-2 max-w-2xl text-sm leading-6 text-slate-500">
          覆盖订单、商品库存、采购、财务账户、员工资料、考勤和盘点等主要业务操作。撤销前会检查相关数据；已有后续改动时，系统会阻止撤销。
        </p>
      </div>
      <el-button :loading="loading" round @click="loadOperations">
        <el-icon class="mr-1"><Refresh /></el-icon>
        刷新记录
      </el-button>
    </header>

    <div class="rounded-3xl border border-slate-200/80 bg-white/70 p-3 shadow-sm md:p-5">
      <div class="history-toolbar">
        <div class="flex flex-1 flex-wrap items-center gap-2">
          <button
            type="button"
            class="history-chip"
            :class="{ 'history-chip-active': activeCategory === 'all' }"
            @click="activeCategory = 'all'"
          >
            全部
            <span class="history-chip-count">{{ operations.length }}</span>
          </button>
          <button
            v-for="category in visibleCategories"
            :key="category.key"
            type="button"
            class="history-chip"
            :class="{ 'history-chip-active': activeCategory === category.key }"
            @click="activeCategory = category.key"
          >
            {{ category.label }}
            <span class="history-chip-count">{{ categoryCounts[category.key] }}</span>
          </button>
        </div>
        <el-input
          v-model="keyword"
          class="history-search"
          placeholder="搜索标题、详情或操作人"
          clearable
          prefix-icon="Search"
        />
      </div>

      <OperationHistoryList
        :operations="filteredOperations"
        :loading="loading"
        :undoing-id="undoingId"
        @undo="confirmUndo"
      />
    </div>
  </section>
</template>

<style scoped>
.history-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.history-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  border: 1px solid #e2e8f0;
  border-radius: 999px;
  background: #ffffff;
  color: #475569;
  font-size: 13px;
  font-weight: 600;
  line-height: 1;
  cursor: pointer;
  transition: all 0.15s ease;
}

.history-chip:hover {
  border-color: #93c5fd;
  color: #2563eb;
}

.history-chip-active {
  border-color: #2563eb;
  background: #2563eb;
  color: #ffffff;
}

.history-chip-count {
  font-size: 12px;
  font-weight: 700;
  color: #94a3b8;
}

.history-chip-active .history-chip-count {
  color: rgb(255 255 255 / 78%);
}

.history-search {
  width: 220px;
  flex: 0 0 auto;
}

@media (max-width: 640px) {
  .history-search {
    width: 100%;
  }
}
</style>
