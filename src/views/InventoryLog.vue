<template>
  <div class="inventory-log-container">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
      <div>
        <h1 class="text-2xl font-black text-slate-900 tracking-tight">库存流水审计</h1>
        <p class="text-xs text-slate-400 mt-1">追踪入库、出库、调拨及盘点历史</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <el-select v-model="filters.log_type" placeholder="变动类型" clearable class="!w-[calc(50%-4px)] md:!w-32">
          <el-option label="采购入库" value="purchase" />
          <el-option label="销售出库" value="sale" />
          <el-option label="库存调拨" value="transfer" />
          <el-option label="盘点调整" value="stocktake" />
          <el-option label="取消退库" value="cancel" />
        </el-select>
        <el-select v-model="filters.warehouse_type" placeholder="选择仓库" clearable class="!w-[calc(50%-4px)] md:!w-32">
          <el-option label="主仓" :value="1" />
          <el-option label="门店" :value="2" />
          <el-option label="云仓" :value="3" />
          <el-option label="样机" :value="4" />
        </el-select>
        <el-button type="primary" @click="handleFilter" class="!rounded-xl h-10 flex-1 md:flex-none">
          <el-icon class="mr-1"><Search /></el-icon>筛选
        </el-button>
        <el-button @click="resetFilters" class="!rounded-xl h-10">重置</el-button>
      </div>
    </div>

    <!-- Data Table Card -->
    <el-card class="!rounded-2xl border-slate-100 shadow-sm overflow-hidden" shadow="never" body-class="!p-0">
      <!-- PC View -->
      <el-table 
        v-loading="loading"
        :data="logList" 
        style="width: 100%" 
        border
        stripe
        highlight-current-row
        class="desktop-only hidden md:block"
      >
        <el-table-column label="变动时间" width="180">
          <template #default="{ row }">
            <span class="text-slate-500 font-mono text-xs">{{ new Date(row.created_at).toLocaleString('zh-CN', { hour12: false }) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="相关商品" min-width="200">
          <template #default="{ row }">
            <div class="flex flex-col">
              <span class="font-bold text-slate-800 text-sm">{{ row.product?.name || '未知商品' }}</span>
              <el-tag size="small" effect="plain" class="!rounded-lg !px-1.5 self-start text-[10px]">{{ row.product?.sku }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="仓库" width="100" align="center">
          <template #default="{ row }">
            <el-tag size="small" type="info" effect="plain">{{ getWarehouseName(row.warehouse_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="变动类型" width="100" align="center">
          <template #default="{ row }">
            <el-tag size="small" :type="getLogTypeTag(row.log_type).type" effect="dark">
              {{ getLogTypeTag(row.log_type).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="变动数量" width="100" align="center">
          <template #default="{ row }">
            <span class="font-bold text-base" :class="row.change_qty > 0 ? 'text-blue-600' : 'text-red-500'">
              {{ row.change_qty > 0 ? '+' : '' }}{{ row.change_qty }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="库存快照" width="120" align="center">
          <template #default="{ row }">
             <span class="text-xs text-slate-400">{{ row.before_qty }} &rarr; <span class="font-bold text-slate-700">{{ row.after_qty }}</span></span>
          </template>
        </el-table-column>
        <el-table-column prop="remark" label="操作备注" min-width="200" show-overflow-tooltip />
      </el-table>

      <!-- Mobile View -->
      <div class="mobile-only md:hidden p-4 space-y-4" v-loading="loading">
        <div v-for="row in logList" :key="row.id" class="bg-white border border-slate-100 rounded-2xl p-4 shadow-sm">
          <div class="flex justify-between items-start mb-3">
            <div class="flex-1 min-w-0 pr-2">
              <div class="font-bold text-slate-800 text-sm leading-tight mb-1">{{ row.product?.name }}</div>
              <div class="flex items-center gap-2">
                <el-tag size="small" :type="getLogTypeTag(row.log_type).type" effect="dark">{{ getLogTypeTag(row.log_type).label }}</el-tag>
                <span class="text-[10px] text-slate-400">{{ getWarehouseName(row.warehouse_type) }}</span>
              </div>
            </div>
            <div class="text-right flex-shrink-0">
              <div class="font-black text-lg" :class="row.change_qty > 0 ? 'text-blue-600' : 'text-red-500'">
                {{ row.change_qty > 0 ? '+' : '' }}{{ row.change_qty }}
              </div>
              <div class="text-[10px] text-slate-400">{{ row.before_qty }} &rarr; {{ row.after_qty }}</div>
            </div>
          </div>
          <div class="bg-slate-50 p-3 rounded-xl">
             <div class="text-[11px] text-slate-500 line-clamp-2">{{ row.remark || '无备注' }}</div>
             <div class="mt-2 text-[10px] text-slate-400 font-mono">{{ new Date(row.created_at).toLocaleString('zh-CN', { hour12: false }) }}</div>
          </div>
        </div>
        <el-empty v-if="!loading && logList.length === 0" description="暂无变动记录" />
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, reactive } from 'vue'
import { Search, Right } from '@element-plus/icons-vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'

const loading = ref(false)
const logList = ref([])
const filters = reactive({
  log_type: '',
  warehouse_type: '',
  product_id: ''
})

const fetchLogs = async () => {
  loading.value = true
  try {
    const res = await axios.get('/api/inventory/logs', {
      params: filters
    })
    if (res.data.code === 200) {
      logList.value = res.data.data
    }
  } catch (error) {
    ElMessage.error('获取库存流水失败')
  } finally {
    loading.value = false
  }
}

const handleFilter = () => {
  fetchLogs()
}

const resetFilters = () => {
  filters.log_type = ''
  filters.warehouse_type = ''
  filters.product_id = ''
  fetchLogs()
}

const getWarehouseName = (type) => {
  const map = { 1: '主仓', 2: '门店', 3: '云仓', 4: '样机' }
  return map[type] || '未知'
}

const getLogTypeTag = (type) => {
  const map = {
    'purchase': { label: '入库', type: 'success' },
    'sale': { label: '销售', type: 'primary' },
    'transfer': { label: '调拨', type: 'warning' },
    'stocktake': { label: '盘点', type: 'info' },
    'cancel': { label: '退库', type: 'danger' }
  }
  return map[type] || { label: type, type: 'info' }
}

onMounted(() => {
  fetchLogs()
})
</script>

<style scoped>
.inventory-log-container {
  @apply space-y-6;
}

.header-section {
  @apply flex flex-col md:flex-row md:items-center justify-between gap-4;
}

.page-title {
  @apply text-2xl font-bold text-slate-900 tracking-tight;
}

.page-subtitle {
  @apply text-sm text-slate-500 mt-1;
}

.filter-wrapper {
  @apply flex flex-wrap items-center gap-2;
}

.filter-item {
  @apply w-40;
}

.table-card {
  @apply border-none rounded-2xl shadow-sm bg-white/50 backdrop-blur-sm;
}

.product-info {
  @apply flex flex-col gap-1;
}

.product-name {
  @apply text-sm font-medium text-slate-800;
}

.sku-tag {
  @apply self-start !h-5 !leading-4 !px-1.5 border-slate-200 text-slate-400;
}

.change-qty {
  @apply font-bold text-lg;
}

.snapshot-view {
  @apply flex items-center justify-center gap-2 text-xs text-slate-400;
}

.snapshot-view .after {
  @apply text-slate-700;
}

:deep(.el-table) {
  --el-table-border-color: #f1f5f9;
  @apply rounded-xl overflow-hidden;
}

:deep(.el-table__header-wrapper th) {
  @apply bg-slate-50 text-slate-500 font-semibold py-4;
}
</style>
