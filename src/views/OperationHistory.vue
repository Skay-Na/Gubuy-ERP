<script setup>
import { onMounted } from 'vue'
import { ElMessageBox } from 'element-plus'
import { useOperationHistory } from '../composables/useOperationHistory'
import OperationHistoryList from '../components/history/OperationHistoryList.vue'

const { operations, loading, undoingId, loadOperations, undoOperation } = useOperationHistory()

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
      <OperationHistoryList
        :operations="operations"
        :loading="loading"
        :undoing-id="undoingId"
        @undo="confirmUndo"
      />
    </div>
  </section>
</template>
