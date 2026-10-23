<script setup>
import { ref } from 'vue'
import { Delete, Document, Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import request from '../../utils/request'

const visible = ref(false)
const loading = ref(false)
const batches = ref([])
const actionBatchId = ref(null)

const warehouseName = (type) => ({ 2: '门店', 3: '云仓' })[type] || '未知仓库'
const statusInfo = (status) => ({
  in_progress: { label: '进行中', type: 'warning' },
  completed: { label: '已完成', type: 'success' },
  terminated: { label: '已终止', type: 'info' }
})[status] || { label: status || '未知', type: 'info' }
const formatDate = (value) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '-'

const loadBatches = async () => {
  loading.value = true
  try {
    const res = await request.get('/api/admin/stocktake-batches')
    if (res.data.code !== 200) {
      ElMessage.error(res.data.msg || '获取盘库单失败')
      return
    }
    batches.value = res.data.data || []
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '获取盘库单失败')
  } finally {
    loading.value = false
  }
}

const openManager = async () => {
  visible.value = true
  await loadBatches()
}

const terminateBatch = async (batch) => {
  try {
    await ElMessageBox.confirm(
      `终止「${batch.batch_no}」后，员工将不能继续录入或提交；已录入的信息会保留，盲盘状态会解除。`,
      '终止盘库单',
      { confirmButtonText: '确认终止', cancelButtonText: '返回', type: 'warning' }
    )
    actionBatchId.value = batch.id
    const res = await request.post(`/api/admin/stocktake-batches/${batch.id}/terminate`)
    if (res.data.code !== 200) {
      ElMessage.error(res.data.msg || '终止盘库单失败')
      return
    }
    ElMessage.success(res.data.msg || '盘库单已终止')
    await loadBatches()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error.response?.data?.msg || '终止盘库单失败')
    }
  } finally {
    actionBatchId.value = null
  }
}

const deleteBatch = async (batch) => {
  try {
    await ElMessageBox.confirm(
      `将永久删除「${batch.batch_no}」及其清点信息。已产生库存调整或关联审核记录的盘库单不能删除。`,
      '删除盘库单',
      { confirmButtonText: '永久删除', cancelButtonText: '取消', type: 'warning', confirmButtonClass: 'el-button--danger' }
    )
    actionBatchId.value = batch.id
    const res = await request.delete(`/api/admin/stocktake-batches/${batch.id}`)
    if (res.data.code !== 200) {
      ElMessage.error(res.data.msg || '删除盘库单失败')
      return
    }
    ElMessage.success(res.data.msg || '盘库单已删除')
    await loadBatches()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error.response?.data?.msg || '删除盘库单失败')
    }
  } finally {
    actionBatchId.value = null
  }
}
</script>

<template>
  <el-button type="primary" plain class="!rounded-xl h-10" @click="openManager">
    <el-icon class="mr-1"><Document /></el-icon>盘库单管理
  </el-button>

  <el-dialog v-model="visible" title="盘库单管理" width="min(1100px, calc(100vw - 24px))" destroy-on-close>
    <div class="mb-3 flex flex-wrap items-center justify-between gap-2 text-sm text-slate-500">
      <span>终止会保留清点数据；已关联库存调整或审核记录的盘库单不能删除。</span>
      <el-button :icon="Refresh" :loading="loading" @click="loadBatches">刷新</el-button>
    </div>
    <el-table v-loading="loading" :data="batches" stripe max-height="65vh">
      <el-table-column label="盘库单号" min-width="175">
        <template #default="{ row }"><span class="font-mono text-xs">{{ row.batch_no }}</span></template>
      </el-table-column>
      <el-table-column label="仓库" width="80" align="center">
        <template #default="{ row }">{{ warehouseName(row.warehouse_type) }}</template>
      </el-table-column>
      <el-table-column label="范围" min-width="130" show-overflow-tooltip>
        <template #default="{ row }">{{ row.scope_label }}</template>
      </el-table-column>
      <el-table-column label="创建员工" width="100" align="center">
        <template #default="{ row }">{{ row.operator_name || '-' }}</template>
      </el-table-column>
      <el-table-column label="商品 / 已录入" width="120" align="center">
        <template #default="{ row }">{{ row.item_count }} / {{ row.counted_count }}</template>
      </el-table-column>
      <el-table-column label="状态" width="95" align="center">
        <template #default="{ row }"><el-tag :type="statusInfo(row.status).type">{{ statusInfo(row.status).label }}</el-tag></template>
      </el-table-column>
      <el-table-column label="创建时间" min-width="155">
        <template #default="{ row }">{{ formatDate(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="155" fixed="right" align="center">
        <template #default="{ row }">
          <div class="flex justify-center gap-1">
            <el-button
              v-if="row.status === 'in_progress'"
              link
              type="warning"
              :loading="actionBatchId === row.id"
              @click="terminateBatch(row)"
            >终止</el-button>
            <el-button
              link
              type="danger"
              :loading="actionBatchId === row.id"
              :disabled="row.status === 'completed'"
              @click="deleteBatch(row)"
            >
              <el-icon><Delete /></el-icon>删除
            </el-button>
          </div>
        </template>
      </el-table-column>
    </el-table>
    <el-empty v-if="!loading && batches.length === 0" description="暂无盘库单" />
    <template #footer><el-button @click="visible = false">关闭</el-button></template>
  </el-dialog>
</template>
