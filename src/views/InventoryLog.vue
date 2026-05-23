<template>
  <div class="inventory-log-container">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
      <div>
        <h1 class="text-2xl font-black text-slate-900 tracking-tight">库存流水审计</h1>
        <p class="text-xs text-slate-400 mt-1">追踪入库、出库、调拨及盘点历史</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <el-input 
          v-model="filters.keyword" 
          placeholder="搜索商品或备注..." 
          clearable 
          class="!w-full md:!w-64"
          @input="handleKeywordInput"
          @clear="handleFilter"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        <el-select v-model="filters.log_type" placeholder="变动类型" clearable class="!w-[calc(50%-4px)] md:!w-32" @change="handleFilter">
          <el-option label="采购入库" value="purchase" />
          <el-option label="销售出库" value="sale" />
          <el-option label="库存调拨" value="transfer" />
          <el-option label="盘点调整" value="stocktake" />
          <el-option label="取消退库" value="cancel" />
        </el-select>
        <el-select v-model="filters.warehouse_type" placeholder="选择仓库" clearable class="!w-[calc(50%-4px)] md:!w-32" @change="handleFilter">
          <el-option label="主仓" :value="1" />
          <el-option label="门店" :value="2" />
          <el-option label="云仓" :value="3" />
          <el-option label="样机" :value="4" />
        </el-select>
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
              <el-tag size="small" effect="plain" class="!rounded-lg !px-1.5 self-start text-[10px]">{{ row.product?.remark || '-' }}</el-tag>
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
        <el-table-column label="变动单价" width="100" align="right">
          <template #default="{ row }">
            <span v-if="row.unit_cost" class="font-mono text-xs text-slate-600">¥{{ row.unit_cost.toFixed(2) }}</span>
            <span v-else class="text-slate-300">-</span>
          </template>
        </el-table-column>
        <el-table-column label="库存快照" width="120" align="center">
          <template #default="{ row }">
             <span class="text-xs text-slate-400">{{ row.before_qty }} &rarr; <span class="font-bold text-slate-700">{{ row.after_qty }}</span></span>
          </template>
        </el-table-column>
        <el-table-column prop="remark" label="操作备注" min-width="200" show-overflow-tooltip />
        <el-table-column label="操作" width="110" align="center">
          <template #default="{ row }">
            <div class="flex flex-col gap-1 items-center justify-center">
              <el-button 
                v-if="row.log_type === 'purchase' && row.related_no" 
                type="danger" 
                link 
                size="small"
                class="!mx-0"
                @click="handleCancelInbound(row)"
              >
                撤销入库
              </el-button>
              <el-button 
                v-if="row.log_type === 'purchase' || row.log_type === 'purchase_void'" 
                type="primary" 
                link 
                size="small"
                class="!mx-0"
                @click="handleUpdatePrice(row)"
              >
                修正进价
              </el-button>
              <span v-if="row.log_type !== 'purchase' && row.log_type !== 'purchase_void'" class="text-slate-300">-</span>
            </div>
          </template>
        </el-table-column>
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
          <!-- Mobile action -->
          <div v-if="row.log_type === 'purchase' || row.log_type === 'purchase_void'" class="mt-3 pt-3 border-t border-slate-100 flex justify-end gap-2">
            <el-button 
              v-if="row.log_type === 'purchase' && row.related_no"
              type="danger" 
              plain
              size="small"
              class="!rounded-xl"
              @click="handleCancelInbound(row)"
            >
              撤销入库
            </el-button>
            <el-button 
              type="primary" 
              plain
              size="small"
              class="!rounded-xl"
              @click="handleUpdatePrice(row)"
            >
              修正进价
            </el-button>
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
import { ElMessage, ElMessageBox } from 'element-plus'

const loading = ref(false)
const logList = ref([])
const filters = reactive({
  keyword: '',
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

let searchTimer = null
const handleKeywordInput = () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    fetchLogs()
  }, 500)
}

const handleFilter = () => {
  fetchLogs()
}

const resetFilters = () => {
  filters.keyword = ''
  filters.log_type = ''
  filters.warehouse_type = ''
  filters.product_id = ''
  fetchLogs()
}

const handleCancelInbound = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要撤销商品【${row.product?.name || '未知商品'}】的这笔采购入库记录吗？<br/>` +
      `系统将自动执行以下操作：<br/>` +
      `1. 从当前 <b class="text-red-600">${getWarehouseName(row.warehouse_type)}</b> 扣减该商品数量 <b class="text-red-600">${row.change_qty}</b> 件；<br/>` +
      `2. 原路退还采购货款 <b class="text-green-600">¥${(row.change_qty * row.unit_cost).toFixed(2)}</b> 至原付款账户；<br/>` +
      `3. 自动重新核算该商品的移动加权平均成本。<br/><br/>` +
      `<span class="text-xs text-gray-400">* 此操作不可逆，且要求该仓库当前可用库存充足。</span>`,
      '确认撤销入库记录',
      {
        confirmButtonText: '确定撤销',
        cancelButtonText: '取消',
        type: 'warning',
        dangerouslyUseHTMLString: true,
        confirmButtonClass: 'el-button--danger'
      }
    )

    loading.value = true
    const res = await axios.post(`/api/inbounds/cancel-by-log/${row.id}`)
    if (res.data.code === 200) {
      ElMessage.success(res.data.msg || '入库流水撤销成功！')
      fetchLogs()
    } else {
      ElMessage.error(res.data.msg || '撤销失败')
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Cancel inbound error:', error)
      ElMessage.error(error.response?.data?.msg || '操作异常，请检查网络或库存')
    }
  } finally {
    loading.value = false
  }
}

const handleUpdatePrice = async (row) => {
  try {
    const { value: newPriceStr } = await ElMessageBox.prompt(
      `请输入该流水【${row.product?.name || '未知商品'}】的正确入库进价（元）：`,
      '手动修正历史入库价格',
      {
        confirmButtonText: '确认修正',
        cancelButtonText: '取消',
        inputPattern: /^\d+(\.\d{1,2})?$/,
        inputErrorMessage: '请输入有效的价格（最多2位小数）',
        inputValue: row.unit_cost ? row.unit_cost.toString() : ''
      }
    )
    
    if (!newPriceStr) return
    const newPrice = parseFloat(newPriceStr)
    if (isNaN(newPrice) || newPrice <= 0) {
      return ElMessage.warning('价格必须大于0')
    }
    
    loading.value = true
    const res = await axios.post(`/api/inbounds/update-price/${row.id}`, {
      new_price: newPrice
    })
    
    if (res.data.code === 200) {
      ElMessage.success('进价修正及系统平均成本重算成功！')
      fetchLogs()
    } else {
      ElMessage.error(res.data.msg || '修正失败')
    }
  } catch (err) {
    if (err !== 'cancel') {
      console.error(err)
      ElMessage.error('修正操作异常')
    }
  } finally {
    loading.value = false
  }
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
