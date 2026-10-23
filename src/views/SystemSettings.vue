<template>
  <div class="space-y-8 animate-in fade-in slide-in-from-bottom-4 duration-700">
    <!-- Header -->
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
      <div>
        <h1 class="text-3xl font-black text-slate-900 tracking-tight">系统设置</h1>
        <p class="text-slate-500 mt-1">管理系统全局配置与底层维护操作</p>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-8">
      <!-- Left Column: General Info -->
      <div class="lg:col-span-2 space-y-8">
        <AIProviderSettings />
        <el-card shadow="never" class="!rounded-[2rem] border-slate-200/60 overflow-hidden relative">
          <div class="absolute top-0 right-0 p-8 opacity-5">
            <el-icon size="120"><Setting /></el-icon>
          </div>
          <h3 class="text-lg font-bold mb-6 flex items-center">
            <div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center mr-3">
              <el-icon><Monitor /></el-icon>
            </div>
            系统信息
          </h3>
          <div class="space-y-4">
            <div class="flex justify-between py-3 border-b border-slate-50">
              <span class="text-slate-500">软件版本</span>
              <span class="font-mono font-bold">v2.0.4-stable</span>
            </div>
            <div class="flex justify-between py-3 border-b border-slate-50">
              <span class="text-slate-500">后端运行环境</span>
              <span class="font-mono text-blue-600">Go / Gin / GORM</span>
            </div>
            <div class="flex justify-between py-3 border-b border-slate-50">
              <span class="text-slate-500">前端技术栈</span>
              <span class="font-mono text-indigo-600">Vue 3 / Vite / Tailwind</span>
            </div>
          </div>
        </el-card>

        <el-card shadow="never" class="!rounded-[2rem] border-slate-200/60">
          <h3 class="text-lg font-bold mb-6 flex items-center">
            <div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center mr-3">
              <el-icon><Download /></el-icon>
            </div>
            数据维护
          </h3>
          <div class="flex items-center justify-between p-6 bg-slate-50 rounded-2xl border border-slate-100">
            <div>
              <p class="font-bold text-slate-800">全量数据备份</p>
              <p class="text-xs text-slate-500 mt-1">导出当前数据库的完整 SQL 备份文件</p>
            </div>
            <el-button 
              type="primary" 
              plain
              class="!rounded-xl font-bold"
              :loading="backupDownloading"
              @click="handleDownloadBackup"
            >
              {{ backupDownloading ? '正在生成备份…' : '立即下载备份' }}
            </el-button>
          </div>

          <div class="flex items-center justify-between p-6 bg-slate-50 rounded-2xl border border-slate-100 mt-4">
            <div>
              <p class="font-bold text-slate-800">全量数据恢复</p>
              <p class="text-xs text-slate-500 mt-1">从 SQL 备份文件恢复 (会覆盖现有数据)</p>
            </div>
            <el-upload
              :show-file-list="false"
              :before-upload="beforeRestoreUpload"
              :http-request="handleRestoreUpload"
              accept=".sql"
            >
              <el-button 
                type="warning" 
                plain
                class="!rounded-xl font-bold"
                :loading="restoring"
              >
                上传并恢复
              </el-button>
            </el-upload>
          </div>

          <div class="flex flex-col gap-4 p-6 bg-amber-50/60 rounded-2xl border border-amber-100 mt-4 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p class="font-bold text-slate-800">历史零成本修复</p>
              <p class="text-xs text-slate-600 mt-1">仅展示存在有效零成本订单的商品及相关历史入库记录，支持逐笔修复订单成本快照。</p>
            </div>
            <el-button type="warning" plain class="!rounded-xl font-bold shrink-0" @click="openHistoricalCostRepair">
              查看待修复商品
            </el-button>
          </div>
        </el-card>

        <el-card shadow="never" class="!rounded-[2rem] border-slate-200/60">
          <h3 class="text-lg font-bold mb-6 flex items-center">
            <div class="w-8 h-8 rounded-lg bg-emerald-50 text-emerald-600 flex items-center justify-center mr-3">
              <el-icon><Lock /></el-icon>
            </div>
            安全策略
          </h3>
          <div class="p-6 bg-slate-50 rounded-2xl border border-slate-100">
            <p class="text-sm text-slate-600 leading-relaxed">
              当前系统采用本地密码验证模式。为了保证数据安全，建议定期修改管理员密码，并确保数据库连接字符串已加密或通过环境变量管理。
            </p>
          </div>
        </el-card>
      </div>

      <!-- Right Column: Danger Zone -->
      <div class="space-y-8">
        <el-card shadow="never" class="!rounded-[2rem] border-red-100 bg-red-50/30 overflow-hidden relative">
          <div class="absolute -right-4 -bottom-4 opacity-10 text-red-600 rotate-12">
            <el-icon size="100"><Warning /></el-icon>
          </div>
          
          <h3 class="text-lg font-bold text-red-600 mb-6 flex items-center">
            <div class="w-8 h-8 rounded-lg bg-red-100 text-red-600 flex items-center justify-center mr-3">
              <el-icon><Delete /></el-icon>
            </div>
            危险区域
          </h3>
          
          <div class="space-y-4 relative z-10">
            <p class="text-xs text-red-500 font-medium uppercase tracking-wider">该操作不可逆</p>
            <p class="text-sm text-slate-600 leading-relaxed">
              <strong>恢复出厂设置</strong> 将清空所有业务数据（包括订单、商品、财务记录及管理员账号）。执行后系统将返回初始状态。
            </p>
            
            <el-button 
              type="danger" 
              class="w-full !rounded-xl !h-12 font-bold shadow-lg shadow-red-500/20 mt-4"
              @click="showResetConfirm = true"
            >
              开始恢复出厂设置
            </el-button>
          </div>
        </el-card>
      </div>
    </div>

    <!-- Reset Confirmation Dialog -->
    <el-dialog
      v-model="showResetConfirm"
      title="确定要恢复出厂设置吗？"
      width="400px"
      center
      destroy-on-close
      class="rounded-[2rem]"
    >
      <div class="py-2">
        <div class="p-4 bg-red-50 rounded-2xl border border-red-100 mb-6">
          <p class="text-xs text-red-600 leading-relaxed">
            警告：此操作将永久删除数据库中的所有记录。如果你确认要继续，请在下方输入 <span class="font-black font-mono">RESET</span>。
          </p>
        </div>
        
        <el-input 
          v-model="confirmText" 
          placeholder="请输入 RESET" 
          size="large"
          class="mb-6"
        />

        <div class="flex gap-4">
          <el-button 
            class="flex-1 !rounded-xl !h-12 font-bold"
            @click="showResetConfirm = false"
          >
            取消
          </el-button>
          <el-button 
            type="danger" 
            class="flex-1 !rounded-xl !h-12 font-bold shadow-lg shadow-red-500/20"
            :disabled="confirmText !== 'RESET'"
            :loading="resetting"
            @click="handleFactoryReset"
          >
            确认重置
          </el-button>
        </div>
      </div>
    </el-dialog>

    <el-dialog v-model="historicalRepairVisible" title="历史成本修复" width="1000px" destroy-on-close @closed="resetHistoricalRepairSearch">
      <div class="mb-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-900">
        仅展示关联有效零成本订单的商品档案、入库流水和订单明细。订单成本必须按订单逐笔核对，只会更新选中的订单明细及该订单对应的销售流水。修复库存成本不会批量改写历史订单，也不会改变库存数量或账户余额。补录金额超过核验参考 5 倍时需再次输入金额确认。
      </div>
      <div class="mb-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <el-input
          v-model="historicalRepairSearchQuery"
          clearable
          :prefix-icon="Search"
          placeholder="搜索商品名称、编码、订单号或时间"
          class="sm:max-w-md"
        />
        <span class="shrink-0 text-xs text-slate-500">显示 {{ filteredHistoricalRepairCandidates.length }} / {{ orderBackedHistoricalRepairCandidates.length }} 条</span>
      </div>
      <el-table :data="filteredHistoricalRepairCandidates" v-loading="historicalRepairLoading" max-height="460" class="w-full">
        <el-table-column label="商品 / 修复对象" min-width="210">
          <template #default="{ row }">
            <p class="font-bold text-slate-800">{{ row.product?.name || `商品 #${row.product_id}` }}</p>
            <p class="mt-1 text-xs text-slate-400">
              <template v-if="row.candidate_type === 'product'">{{ formatDate(row.created_at) }} · 商品档案（无有效入库成本）</template>
              <template v-else-if="row.candidate_type === 'order_item'">{{ formatDate(row.created_at) }} · 订单 {{ row.order_no || '单号缺失' }} · 成交 ¥{{ Number(row.order_unit_price || 0).toFixed(2) }} · 成本待核</template>
              <template v-else>{{ formatDate(row.created_at) }} · {{ row.log_type === 'stocktake' ? '库存盘点增加' : '采购入库' }}</template>
            </p>
          </template>
        </el-table-column>
        <el-table-column label="数量" width="100" align="center"><template #default="{ row }"><span v-if="row.candidate_type === 'product'">库存 {{ row.stock_qty || 0 }}</span><span v-else-if="row.candidate_type === 'order_item'">×{{ row.order_quantity }}</span><span v-else>+{{ row.change_qty }}</span></template></el-table-column>
        <el-table-column label="零成本订单" width="120" align="center"><template #default="{ row }"><span v-if="row.candidate_type === 'order_item'" class="font-bold text-red-600">当前订单明细</span><span v-else :class="row.zero_cost_order_count ? 'font-bold text-amber-700' : 'text-slate-400'">{{ row.zero_cost_order_count }} 条待核</span></template></el-table-column>
        <el-table-column label="补录单价" width="175"><template #default="{ row }"><el-input-number v-model="repairUnitCosts[historicalRepairCandidateKey(row)]" :min="0.01" :precision="2" controls-position="right" class="!w-full" /><p v-if="row.cost_reference > 0" class="mt-1 text-[11px] text-slate-500">核验参考 ¥{{ Number(row.cost_reference).toFixed(2) }}</p></template></el-table-column>
        <el-table-column label="修复范围" min-width="135" align="center"><template #default="{ row }"><span v-if="row.candidate_type === 'order_item'" class="text-xs text-blue-700">本订单 + 对应流水</span><span v-else class="text-xs text-slate-500">仅当前成本记录</span></template></el-table-column>
        <el-table-column label="操作" width="130" fixed="right"><template #default="{ row }"><el-button type="primary" link :loading="historicalRepairSubmittingId === historicalRepairCandidateKey(row)" @click="submitHistoricalCostRepair(row)">{{ row.candidate_type === 'product' ? '设置商品成本' : row.candidate_type === 'order_item' ? '修复此订单' : '修复库存成本' }}</el-button></template></el-table-column>
      </el-table>
      <el-empty v-if="!historicalRepairLoading && orderBackedHistoricalRepairCandidates.length === 0" description="没有关联有效待核订单的商品或记录" :image-size="80" />
      <el-empty v-else-if="!historicalRepairLoading && filteredHistoricalRepairCandidates.length === 0" description="没有找到匹配的有效订单商品" :image-size="80" />
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Setting, Monitor, Lock, Warning, Delete, Download, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import request from '../utils/request'
import AIProviderSettings from '../components/settings/AIProviderSettings.vue'

const showResetConfirm = ref(false)
const route = useRoute()
const confirmText = ref('')
const resetting = ref(false)
const backupDownloading = ref(false)
const restoring = ref(false)
const historicalRepairVisible = ref(false)
const historicalRepairLoading = ref(false)
const historicalRepairCandidates = ref([])
const historicalRepairSearchQuery = ref('')
const historicalRepairSubmittingId = ref(null)
const repairUnitCosts = ref({})
const historicalCostOutlierMultiplier = 5

const historicalRepairCandidateKey = (row) => row.candidate_id || `log:${row.id}`

const resolveFilename = (headers) => {
  const disposition = headers['content-disposition'] || headers['Content-Disposition']
  if (!disposition) return `gubuy_erp_backup_${Date.now()}.sql`

  const utf8Match = disposition.match(/filename\*=UTF-8''([^;]+)/i)
  if (utf8Match?.[1]) {
    return decodeURIComponent(utf8Match[1])
  }

  const plainMatch = disposition.match(/filename="?([^"]+)"?/i)
  return plainMatch?.[1] || `gubuy_erp_backup_${Date.now()}.sql`
}

const downloadBlob = (blob, filename) => {
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.URL.revokeObjectURL(url)
}

const getRequestErrorMessage = async (error, fallback) => {
  const responseData = error.response?.data
  if (responseData instanceof Blob) {
    try {
      const payload = JSON.parse(await responseData.text())
      return payload.msg || fallback
    } catch {
      return fallback
    }
  }

  return responseData?.msg || fallback
}

const handleDownloadBackup = async () => {
  if (backupDownloading.value) return

  backupDownloading.value = true
  try {
    const res = await request.get('/api/system/backup', {
      responseType: 'blob',
      timeout: 135000
    })
    downloadBlob(res.data, resolveFilename(res.headers))
    ElMessage.success('备份下载成功')
  } catch (error) {
    ElMessage.error({ message: await getRequestErrorMessage(error, '备份下载失败，请稍后重试'), grouping: true })
  } finally {
    backupDownloading.value = false
  }
}

const beforeRestoreUpload = (file) => {
  return new Promise((resolve, reject) => {
    ElMessageBox.confirm(
      '警告：从备份恢复将彻底覆盖当前所有业务数据！此操作无法撤销。建议在恢复前先导出当前数据的备份。',
      '确认恢复数据',
      {
        confirmButtonText: '我明白风险，开始恢复',
        cancelButtonText: '取消',
        type: 'warning',
        confirmButtonClass: 'el-button--danger',
        roundButton: true
      }
    ).then(() => {
      restoring.value = true
      resolve(true)
    }).catch(() => {
      reject(false)
    })
  })
}

const handleRestoreUpload = async ({ file }) => {
  const formData = new FormData()
  formData.append('file', file)

  try {
    const res = await request.post('/api/system/restore', formData, {
      headers: {
        'Content-Type': 'multipart/form-data'
      },
      timeout: 660000
    })
    restoring.value = false

    if (res.data.code === 200) {
      ElMessage.success({
        message: '数据恢复成功，系统即将刷新',
        duration: 3000,
        onClose: () => {
          window.location.reload()
        }
      })
      return
    }

    ElMessage.error(res.data.msg || '恢复失败')
  } catch (error) {
    restoring.value = false
    ElMessage.error(error.response?.data?.msg || '上传恢复失败，请检查文件格式或网络连接')
  }
}

const handleFactoryReset = async () => {
  if (confirmText.value !== 'RESET') return

  resetting.value = true
  try {
    const res = await request.post('/api/system/reset', {
      confirm: confirmText.value
    })
    if (res.data.code === 200) {
      ElMessage.success('系统已恢复出厂设置，即将跳转...')
      setTimeout(() => {
        window.location.href = '/' // 强制刷新并回到入口页
      }, 2000)
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '操作失败')
  } finally {
    resetting.value = false
  }
}
const formatDate = (value) => value ? new Date(value).toLocaleString('zh-CN') : '-'
const isHistoricalCostOutlier = (unitCost, costReference) => costReference > 0 && unitCost > costReference * historicalCostOutlierMultiplier

const historicalRepairCandidateSearchText = (row) => {
  const candidateLabel = row.candidate_type === 'product'
    ? '商品档案'
    : row.candidate_type === 'order_item'
      ? '订单明细'
      : row.log_type === 'stocktake'
        ? '库存盘点增加 盘点'
        : '采购入库'

  return [
    row.product?.name,
    row.product?.remark,
    row.product_id,
    row.order_no,
    row.related_no,
    row.candidate_id,
    row.candidate_type,
    candidateLabel,
    row.log_type,
    row.created_at,
    formatDate(row.created_at),
    row.change_qty,
    row.order_quantity,
    row.order_unit_price,
    row.cost_reference
  ].filter(value => value !== null && value !== undefined).join(' ').toLocaleLowerCase()
}

const orderBackedHistoricalRepairCandidates = computed(() =>
  historicalRepairCandidates.value.filter(row => Number(row.zero_cost_order_count) > 0)
)

const filteredHistoricalRepairCandidates = computed(() => {
  const terms = historicalRepairSearchQuery.value.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
  if (terms.length === 0) return orderBackedHistoricalRepairCandidates.value

  return orderBackedHistoricalRepairCandidates.value.filter(row => {
    const searchText = historicalRepairCandidateSearchText(row)
    return terms.every(term => searchText.includes(term))
  })
})

const resetHistoricalRepairSearch = () => {
  historicalRepairSearchQuery.value = ''
}

const openHistoricalCostRepair = async () => {
  historicalRepairVisible.value = true
  historicalRepairLoading.value = true
  try {
    const res = await request.get('/api/system/historical-cost-repair/candidates')
    if (res.data.code === 200) {
      historicalRepairCandidates.value = res.data.data || []
      repairUnitCosts.value = Object.fromEntries(historicalRepairCandidates.value.map(row => [historicalRepairCandidateKey(row), undefined]))
      return
    }
    ElMessage.error(res.data.msg || '读取待修复记录失败')
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '读取待修复记录失败')
  } finally {
    historicalRepairLoading.value = false
  }
}

const submitHistoricalCostRepair = async (row) => {
  const candidateKey = historicalRepairCandidateKey(row)
  const isProductCandidate = row.candidate_type === 'product'
  const isOrderItemCandidate = row.candidate_type === 'order_item'
  const unitCost = Number(repairUnitCosts.value[candidateKey])
  if (!(unitCost > 0)) {
    ElMessage.warning('请先填写大于 0 的成本单价')
    return
  }
  const costReference = Number(row.cost_reference) || 0
  let outlierConfirm = ''

  if (isHistoricalCostOutlier(unitCost, costReference)) {
    const repairScope = isProductCandidate
      ? '本次只设置商品档案平均成本；'
      : isOrderItemCandidate
        ? `本次只修复订单 ${row.order_no || ''} 的这一条明细及对应销售流水；`
        : '本次只修复当前库存流水；'
    try {
      const { value } = await ElMessageBox.prompt(
        `补录单价 ¥${unitCost.toFixed(2)} 已超过核验参考 ¥${costReference.toFixed(2)} 的 ${historicalCostOutlierMultiplier} 倍。请先核对商品成本凭证；${repairScope}库存数量和账户余额不变。确认该金额无误后，再输入 ¥${unitCost.toFixed(2)} 继续。`,
        '检测到异常高成本',
        {
          confirmButtonText: '确认金额无误',
          cancelButtonText: '取消',
          inputPlaceholder: unitCost.toFixed(2),
          inputPattern: /^\d+(\.\d{1,2})?$/,
          inputErrorMessage: '请输入有效金额，最多两位小数',
          type: 'warning'
        }
      )
      if (Number(value).toFixed(2) !== unitCost.toFixed(2)) {
        ElMessage.warning('输入金额与待修复单价不一致，已取消修复')
        return
      }
      outlierConfirm = unitCost.toFixed(2)
    } catch {
      return
    }
  }

  try {
    if (!outlierConfirm) {
      await ElMessageBox.confirm(
        isProductCandidate
          ? `将把商品档案的平均成本设为 ¥${unitCost.toFixed(2)}，并建立当前库存成本基准；不会更改最新进价、订单成本、库存数量或账户余额。`
          : isOrderItemCandidate
            ? `将把订单 ${row.order_no || ''} 中该商品的 ${row.order_quantity} 件明细成本设为 ¥${unitCost.toFixed(2)}，并同步该订单对应的销售流水。其他订单、库存数量和账户余额不会变化。`
            : `将只修复该条库存流水成本为 ¥${unitCost.toFixed(2)}，不会批量修改订单、库存数量或账户余额。`,
        '确认历史成本修复',
        { confirmButtonText: '确认修复', cancelButtonText: '取消', type: 'warning' }
      )
    }
  } catch {
    return
  }

  historicalRepairSubmittingId.value = candidateKey
  try {
    const res = await request.post('/api/system/historical-cost-repair', {
      candidate_type: row.candidate_type,
      ...(isProductCandidate ? { product_id: row.product_id } : isOrderItemCandidate ? { order_item_id: row.order_item_id } : { log_id: row.id }),
      unit_cost: unitCost,
      outlier_confirm: outlierConfirm,
      remark: isProductCandidate ? '系统设置 - 商品档案成本基准修复' : isOrderItemCandidate ? `系统设置 - 订单 ${row.order_no || ''} 逐笔成本修复` : '系统设置 - 历史库存成本修复',
      confirm: 'REPAIR'
    })
    if (res.data.code === 200) {
      ElMessage.success(isOrderItemCandidate
        ? `订单明细已修复；同步销售流水 ${res.data.data?.repaired_sale_count || 0} 条`
        : '成本记录已修复')
      await openHistoricalCostRepair()
      return
    }
    ElMessage.error(res.data.msg || '修复失败')
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '修复失败')
  } finally {
    historicalRepairSubmittingId.value = null
  }
}

onMounted(() => {
  if (route.query.open === 'historical-cost-repair') {
    openHistoricalCostRepair()
  }
})
</script>

<style scoped>
:deep(.el-card) {
  border: 1px solid rgba(226, 232, 240, 0.6);
}
</style>
