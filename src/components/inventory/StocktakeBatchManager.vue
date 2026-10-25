<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Document, Printer, Refresh } from '@element-plus/icons-vue'
import {
  fetchStocktakeBatches,
  createStocktakeBatch,
  fetchStocktakeBatch,
  updateStocktakeBatchItems,
  submitStocktakeBatch,
  refreshStocktakeBatchItem,
  printStocktakeBatch
} from '../../api/inventory'
import StocktakePrintSheet from './StocktakePrintSheet.vue'

const props = defineProps({
  warehouseType: { type: Number, required: true },
  categoryId: { type: [Number, String], default: '' },
  keyword: { type: String, default: '' }
})

const emit = defineEmits(['inventory-updated', 'blind-mode-change'])

const batches = ref([])
const batchesLoading = ref(false)
const blindStocktakeMode = ref(true)
const historyVisible = ref(false)
const detailVisible = ref(false)
const activeBatch = ref(null)
const draftItems = ref([])
const stocktakeTypeDraft = ref(1)
const creating = ref(false)
const saving = ref(false)
const submitting = ref(false)
const printing = ref(false)

const warehouseName = (type) => ({ 2: '门店', 3: '云仓' })[type] || '未知仓库'
const completedItemCount = computed(() => draftItems.value.filter((item) => ['applied', 'approved', 'rejected'].includes(item.status)).length)
const pendingCount = computed(() => draftItems.value.filter((item) => item.status === 'pending_review').length)
const conflictCount = computed(() => draftItems.value.filter((item) => item.status === 'conflict').length)
const uncountedCount = computed(() => draftItems.value.filter((item) => item.status === 'uncounted').length)
const filledUncountedCount = computed(() => draftItems.value.filter((item) => item.status === 'uncounted' && String(item.actual_input ?? '').trim() !== '').length)
const hasOwnActiveBatch = computed(() => batches.value.some((batch) => batch.status === 'in_progress'))

const statusInfo = (status) => ({
  uncounted: { label: '待录入', type: 'info' },
  applied: { label: '已调整', type: 'success' },
  pending_review: { label: '历史待审核', type: 'warning' },
  approved: { label: '审核通过', type: 'success' },
  rejected: { label: '审核驳回', type: 'danger' },
  conflict: { label: '库存有变化', type: 'danger' },
  error: { label: '提交失败', type: 'danger' }
})[status] || { label: status || '未知', type: 'info' }

const batchStatusInfo = (status) => status === 'completed'
  ? { label: '已完成', type: 'success' }
  : status === 'terminated'
    ? { label: '已终止', type: 'info' }
    : { label: '进行中', type: 'warning' }
const activeBatchEditable = computed(() => activeBatch.value?.status === 'in_progress')

const loadBatches = async () => {
  batchesLoading.value = true
  try {
    const res = await fetchStocktakeBatches()
    if (res.data.code === 200) {
      batches.value = res.data.data || []
      blindStocktakeMode.value = Boolean(res.data.blind_stocktake_mode) || hasOwnActiveBatch.value
      emit('blind-mode-change', blindStocktakeMode.value)
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '获取盘库单失败')
  } finally {
    batchesLoading.value = false
  }
}

const createBatch = async () => {
  if (creating.value) return
  creating.value = true
  try {
    const res = await createStocktakeBatch({
      warehouse_type: props.warehouseType,
      category_id: props.categoryId ? String(props.categoryId) : undefined,
      keyword: props.keyword.trim()
    })
    if (res.data.code !== 201) {
      ElMessage.error(res.data.msg || '创建盘库单失败')
      return
    }
    setActiveBatch(res.data.data)
    emit('blind-mode-change', true)
    detailVisible.value = true
    await loadBatches()
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '创建盘库单失败')
  } finally {
    creating.value = false
  }
}

const setActiveBatch = (batch) => {
  activeBatch.value = batch
  stocktakeTypeDraft.value = Number(batch.stocktake_type) || 1
  draftItems.value = (batch.items || []).map((item) => ({
    ...item,
    actual_input: item.actual_stock == null ? '' : String(item.actual_stock)
  }))
}

const openBatch = async (id) => {
  try {
    const res = await fetchStocktakeBatch(id)
    if (res.data.code !== 200) {
      ElMessage.error(res.data.msg || '获取盘库单详情失败')
      return
    }
    setActiveBatch(res.data.data)
    historyVisible.value = false
    detailVisible.value = true
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '获取盘库单详情失败')
  }
}

const refreshActiveBatch = async () => {
  if (!activeBatch.value) return
  const res = await fetchStocktakeBatch(activeBatch.value.id)
  if (res.data.code === 200) setActiveBatch(res.data.data)
}

const saveItems = async (showMessage = true) => {
  if (!activeBatch.value || saving.value) return false
  const editableItems = draftItems.value.filter((item) => item.status === 'uncounted')
  if (editableItems.length === 0) return true
  const items = []
  for (const item of editableItems) {
    const rawValue = String(item.actual_input ?? '').trim()
    let actualStock = null
    if (rawValue !== '') {
      actualStock = Number(rawValue)
      if (!Number.isInteger(actualStock) || actualStock < 0) {
        ElMessage.warning(`「${item.product_name}」的实盘数量必须是大于等于 0 的整数`)
        return false
      }
    }
    items.push({ id: item.id, actual_stock: actualStock, remark: String(item.remark || '').trim() })
  }

  saving.value = true
  try {
    const res = await updateStocktakeBatchItems(activeBatch.value.id, {
      stocktake_type: stocktakeTypeDraft.value,
      items
    })
    if (res.data.code !== 200) {
      ElMessage.error(res.data.msg || '保存盘点数量失败')
      return false
    }
    await refreshActiveBatch()
    if (showMessage) ElMessage.success('盘点数量已保存')
    return true
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '保存盘点数量失败')
    return false
  } finally {
    saving.value = false
  }
}

const submitBatch = async () => {
  if (!activeBatch.value || submitting.value) return
  if (filledUncountedCount.value === 0) {
    ElMessage.warning('请先录入已清点商品的实盘数量')
    return
  }
  const saved = await saveItems(false)
  if (!saved) return
  submitting.value = true
  try {
    const res = await submitStocktakeBatch(activeBatch.value.id)
    if (res.data.code !== 200) {
      ElMessage.error(res.data.msg || '提交盘点结果失败')
      return
    }
    const results = res.data.data?.results || []
    const applied = results.filter((item) => item.status === 'applied').length
    const review = results.filter((item) => item.status === 'pending_review').length
    const conflicts = results.filter((item) => item.status === 'conflict').length
    const errors = results.filter((item) => item.status === 'error')
    await Promise.all([refreshActiveBatch(), loadBatches()])
    const summary = [`已调整 ${applied} 项`, `库存冲突 ${conflicts} 项`]
    if (review > 0) summary.push(`历史待审核 ${review} 项`)
    if (uncountedCount.value > 0) summary.push(`尚未提交 ${uncountedCount.value} 项`)
    if (errors.length > 0) ElMessage.warning(summary.join('，'))
    else ElMessage.success(summary.join('，'))
    if (applied > 0) emit('inventory-updated')
    if (errors.length > 0) {
      ElMessageBox.alert(errors.map((item) => `「${item.product_name}」：${item.message}`).join('\n'), '部分商品未提交', { type: 'warning' })
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '提交盘点结果失败')
  } finally {
    submitting.value = false
  }
}

const refreshConflict = async (item) => {
  try {
    await ElMessageBox.confirm(
      `刷新「${item.product_name}」的库存快照后，需要重新清点该商品。继续吗？`,
      '重新清点',
      { confirmButtonText: '刷新并重盘', cancelButtonText: '暂不处理', type: 'warning' }
    )
    const res = await refreshStocktakeBatchItem(activeBatch.value.id, item.id)
    if (res.data.code === 200) {
      ElMessage.success(res.data.msg || '已刷新，请重新清点')
      await Promise.all([refreshActiveBatch(), loadBatches()])
    } else {
      ElMessage.error(res.data.msg || '刷新失败')
    }
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error.response?.data?.msg || '刷新失败')
  }
}

let pendingPrintBatchId = null

const handleAfterPrint = async () => {
  clearPrintMode()
  window.removeEventListener('afterprint', handleAfterPrint)
  const batchId = pendingPrintBatchId
  pendingPrintBatchId = null
  if (!batchId) {
    printing.value = false
    return
  }

  try {
    await ElMessageBox.confirm(
      '请确认打印机已成功输出纸单。选择“已成功打印”后才会记录打印次数；取消或打印失败无需记录。',
      '确认打印结果',
      { confirmButtonText: '已成功打印', cancelButtonText: '取消或失败', distinguishCancelAndClose: true, type: 'info' }
    )
    const res = await printStocktakeBatch(batchId, { confirmed: true })
    if (res.data.code !== 200) {
      ElMessage.error(res.data.msg || '记录打印状态失败')
      return
    }
    if (activeBatch.value?.id === batchId) {
      activeBatch.value.print_count = Number(activeBatch.value.print_count || 0) + 1
    }
    ElMessage.success('已记录本次打印')
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error.response?.data?.msg || '记录打印状态失败')
    }
  } finally {
    printing.value = false
  }
}

const printBatch = async () => {
  if (!activeBatch.value || printing.value) return
  printing.value = true
  pendingPrintBatchId = activeBatch.value.id
  try {
    await nextTick()
    document.body.classList.add('stocktake-printing')
    window.addEventListener('afterprint', handleAfterPrint, { once: true })
    window.print()
  } catch (error) {
    pendingPrintBatchId = null
    clearPrintMode()
    window.removeEventListener('afterprint', handleAfterPrint)
    printing.value = false
    ElMessage.error(error.response?.data?.msg || '打开打印窗口失败')
  }
}

const formatDate = (value) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '-'
const clearPrintMode = () => document.body.classList.remove('stocktake-printing')

onMounted(loadBatches)
onUnmounted(() => {
  clearPrintMode()
  pendingPrintBatchId = null
  window.removeEventListener('afterprint', handleAfterPrint)
})
</script>

<template>
  <div class="flex flex-wrap items-center gap-2">
    <el-button type="primary" plain :loading="creating" :disabled="batchesLoading || blindStocktakeMode" @click="createBatch">
      <el-icon><Printer /></el-icon><span class="ml-1">生成纸质盘库单</span>
    </el-button>
    <el-button :loading="batchesLoading" @click="historyVisible = true; loadBatches()">
      <el-icon><Document /></el-icon><span class="ml-1">我的盘库单</span>
    </el-button>
    <span v-if="blindStocktakeMode && !hasOwnActiveBatch" class="text-xs text-amber-700">其他员工正在盘库，库存信息暂时隐藏</span>
    <span v-else class="text-xs text-slate-400">按名称/型号和分类筛选；包含零库存商品，不受库存状态筛选影响</span>

    <el-dialog v-model="historyVisible" title="我的盘库单" width="min(900px, calc(100vw - 24px))" destroy-on-close>
      <div class="mb-3 flex items-center justify-between gap-3 text-sm text-slate-500">
        <span>每张盘库单保存商品和库存快照；打印纸单不显示账面数量。</span>
        <el-button :icon="Refresh" :loading="batchesLoading" @click="loadBatches">刷新</el-button>
      </div>
      <el-table v-loading="batchesLoading" :data="batches" stripe>
        <el-table-column label="盘库单号" min-width="190">
          <template #default="{ row }"><span class="font-mono text-xs">{{ row.batch_no }}</span></template>
        </el-table-column>
        <el-table-column label="仓库" width="90"><template #default="{ row }">{{ warehouseName(row.warehouse_type) }}</template></el-table-column>
        <el-table-column label="范围" prop="scope_label" min-width="140" />
        <el-table-column label="商品 / 已录入" width="120" align="center">
          <template #default="{ row }">{{ row.item_count }} / {{ row.counted_count }}</template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }"><el-tag :type="batchStatusInfo(row.status).type">{{ batchStatusInfo(row.status).label }}</el-tag></template>
        </el-table-column>
        <el-table-column label="创建时间" min-width="165"><template #default="{ row }">{{ formatDate(row.created_at) }}</template></el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openBatch(row.id)">{{ row.status === 'in_progress' ? '查看 / 继续' : '查看详情' }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!batchesLoading && batches.length === 0" description="暂无盘库单" />
      <template #footer><el-button @click="historyVisible = false">关闭</el-button></template>
    </el-dialog>

    <el-dialog
      v-model="detailVisible"
      :title="activeBatch ? `盘库单 · ${activeBatch.batch_no}` : '盘库单'"
      width="min(1100px, calc(100vw - 24px))"
      top="4vh"
      destroy-on-close
    >
      <template v-if="activeBatch">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-3 rounded-xl bg-slate-50 p-3">
          <div class="space-y-1 text-sm text-slate-600">
            <div><span class="font-bold">仓库：</span>{{ warehouseName(activeBatch.warehouse_type) }}　<span class="font-bold">范围：</span>{{ activeBatch.scope_label }}</div>
            <div><span class="font-bold">清点进度：</span>{{ completedItemCount }} / {{ draftItems.length }} 已完成，<template v-if="pendingCount > 0">{{ pendingCount }} 项历史待审核，</template>{{ conflictCount }} 项需重盘，{{ uncountedCount }} 项待录入</div>
            <div class="text-xs text-slate-400">本单纸面不显示账面数量；库存有变化时请刷新并重新清点冲突商品。</div>
            <div v-if="activeBatch.status === 'terminated'" class="text-xs font-semibold text-amber-700">本单已被管理员终止，清点记录保留但不能继续录入或提交。</div>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm text-slate-500">差异性质</span>
            <el-select v-model="stocktakeTypeDraft" class="!w-36" :disabled="!activeBatchEditable">
              <el-option label="正常损耗" :value="1" />
              <el-option label="业务调整" :value="2" />
            </el-select>
          </div>
        </div>

        <div class="max-h-[56vh] overflow-auto rounded-lg border border-slate-100">
          <el-table :data="draftItems" stripe>
            <el-table-column label="商品" min-width="220">
              <template #default="{ row }">
                <div class="font-bold text-slate-800">{{ row.product_name }}</div>
                <div class="mt-1 text-xs text-slate-400">编号 {{ row.product_id }}<span v-if="row.product_remark"> · {{ row.product_remark }}</span></div>
              </template>
            </el-table-column>
            <el-table-column label="实盘数量" width="145" align="center">
              <template #default="{ row }">
                <el-input v-if="row.status === 'uncounted' && activeBatchEditable" v-model="row.actual_input" inputmode="numeric" placeholder="未盘；无货填 0" />
                <span v-else>{{ row.actual_stock ?? '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="差异说明" min-width="190">
              <template #default="{ row }">
                <el-input v-if="row.status === 'uncounted' && activeBatchEditable" v-model="row.remark" maxlength="255" placeholder="数量有差异时必填" />
                <span v-else class="text-sm text-slate-600">{{ row.remark || '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="125" align="center">
              <template #default="{ row }"><el-tag size="small" :type="statusInfo(row.status).type">{{ statusInfo(row.status).label }}</el-tag></template>
            </el-table-column>
            <el-table-column label="操作" width="100" fixed="right">
              <template #default="{ row }">
                <el-button v-if="row.status === 'conflict' && activeBatchEditable" link type="warning" @click="refreshConflict(row)">刷新重盘</el-button>
                <span v-else-if="row.status === 'pending_review'" class="text-xs text-slate-400">历史待审核</span>
                <span v-else class="text-xs text-slate-400">-</span>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </template>

      <StocktakePrintSheet :batch="activeBatch" />

      <template #footer>
        <div class="flex flex-wrap justify-between gap-2">
          <div class="flex gap-2">
            <el-button :loading="printing" :disabled="!activeBatchEditable" @click="printBatch">
              <el-icon><Printer /></el-icon><span class="ml-1">{{ activeBatch?.print_count ? '补打盘库单' : '打印盲盘单' }}</span>
            </el-button>
            <el-button :loading="saving" :disabled="!activeBatchEditable" @click="saveItems()">保存录入</el-button>
          </div>
          <div class="flex gap-2">
            <el-button @click="detailVisible = false">关闭</el-button>
            <el-button type="primary" :loading="submitting" :disabled="!activeBatchEditable" @click="submitBatch">提交已盘商品</el-button>
          </div>
        </div>
      </template>
    </el-dialog>
  </div>
</template>
