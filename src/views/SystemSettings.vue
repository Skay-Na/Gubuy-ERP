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
              @click="handleDownloadBackup"
            >
              立即下载备份
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
              <p class="text-xs text-slate-600 mt-1">补录旧员工增加库存的成本，并可同步修复受影响订单的成本快照；不会改变库存数量或账户余额。</p>
            </div>
            <el-button type="warning" plain class="!rounded-xl font-bold shrink-0" @click="openHistoricalCostRepair">
              查看待修复记录
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

    <el-dialog v-model="historicalRepairVisible" title="历史零成本修复" width="900px" destroy-on-close>
      <div class="mb-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-900">
        仅展示“增加库存且成本为 0”的历史记录。<b>没有受影响订单的记录也可以直接修复库存成本。</b>修复会回写该条入库成本、重算商品成本；存在 0 成本订单时才可选择同步修复订单。库存数量、付款账户余额不会变化。
      </div>
      <el-table :data="historicalRepairCandidates" v-loading="historicalRepairLoading" max-height="460" class="w-full">
        <el-table-column label="商品 / 历史流水" min-width="210">
          <template #default="{ row }">
            <p class="font-bold text-slate-800">{{ row.product?.name || `商品 #${row.product_id}` }}</p>
            <p class="mt-1 text-xs text-slate-400">{{ formatDate(row.created_at) }} · {{ row.log_type === 'stocktake' ? '旧库存调整' : '采购入库' }}</p>
          </template>
        </el-table-column>
        <el-table-column label="数量" width="70" align="center"><template #default="{ row }">+{{ row.change_qty }}</template></el-table-column>
        <el-table-column label="受影响订单" width="100" align="center"><template #default="{ row }"><span :class="row.zero_cost_order_count ? 'font-bold text-red-600' : 'text-slate-400'">{{ row.zero_cost_order_count }} 条</span></template></el-table-column>
        <el-table-column label="补录单价" width="150"><template #default="{ row }"><el-input-number v-model="repairUnitCosts[row.id]" :min="0.01" :precision="2" controls-position="right" class="!w-full" /></template></el-table-column>
        <el-table-column label="同步订单成本" width="120" align="center"><template #default="{ row }"><el-checkbox v-if="row.zero_cost_order_count" v-model="repairOrderCosts[row.id]">修复订单</el-checkbox><span v-else class="text-xs text-slate-400">无需修订单</span></template></el-table-column>
        <el-table-column label="操作" width="120" fixed="right"><template #default="{ row }"><el-button type="primary" link :loading="historicalRepairSubmittingId === row.id" @click="submitHistoricalCostRepair(row)">{{ row.zero_cost_order_count ? '确认修复' : '修复库存成本' }}</el-button></template></el-table-column>
      </el-table>
      <el-empty v-if="!historicalRepairLoading && historicalRepairCandidates.length === 0" description="没有需要修复的历史零成本记录" :image-size="80" />
    </el-dialog>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { Setting, Monitor, Lock, Warning, Delete, Download } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import request from '../utils/request'
import AIProviderSettings from '../components/settings/AIProviderSettings.vue'

const showResetConfirm = ref(false)
const confirmText = ref('')
const resetting = ref(false)
const restoring = ref(false)
const historicalRepairVisible = ref(false)
const historicalRepairLoading = ref(false)
const historicalRepairCandidates = ref([])
const historicalRepairSubmittingId = ref(null)
const repairUnitCosts = ref({})
const repairOrderCosts = ref({})

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

const handleDownloadBackup = async () => {
  try {
    const res = await request.get('/api/system/backup', {
      responseType: 'blob',
      timeout: 120000
    })
    downloadBlob(res.data, resolveFilename(res.headers))
    ElMessage.success('备份下载成功')
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '备份下载失败，请稍后重试')
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
      timeout: 300000
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

const openHistoricalCostRepair = async () => {
  historicalRepairVisible.value = true
  historicalRepairLoading.value = true
  try {
    const res = await request.get('/api/system/historical-cost-repair/candidates')
    if (res.data.code === 200) {
      historicalRepairCandidates.value = res.data.data || []
      repairUnitCosts.value = Object.fromEntries(historicalRepairCandidates.value.map(row => [row.id, undefined]))
      repairOrderCosts.value = Object.fromEntries(historicalRepairCandidates.value.map(row => [row.id, Number(row.zero_cost_order_count) > 0]))
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
  const unitCost = Number(repairUnitCosts.value[row.id])
  if (!(unitCost > 0)) {
    ElMessage.warning('请先填写大于 0 的实际入库单价')
    return
  }
  const repairOrders = Boolean(repairOrderCosts.value[row.id])
  try {
    await ElMessageBox.confirm(
      repairOrders
        ? `将修复该条库存流水，并把 ${row.zero_cost_order_count} 条零成本订单明细更新为 ¥${unitCost.toFixed(2)}。库存和账户余额不会变化。`
        : `将只修复该条库存流水成本为 ¥${unitCost.toFixed(2)}，不会改订单成本、库存或账户余额。`,
      '确认历史成本修复',
      { confirmButtonText: '确认修复', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }

  historicalRepairSubmittingId.value = row.id
  try {
    const res = await request.post('/api/system/historical-cost-repair', {
      log_id: row.id,
      unit_cost: unitCost,
      repair_order_costs: repairOrders,
      remark: '系统设置 - 历史零成本修复',
      confirm: 'REPAIR'
    })
    if (res.data.code === 200) {
      ElMessage.success(`修复完成：订单成本 ${res.data.data?.repaired_order_count || 0} 条，销售流水 ${res.data.data?.repaired_sale_count || 0} 条`)
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
</script>

<style scoped>
:deep(.el-card) {
  border: 1px solid rgba(226, 232, 240, 0.6);
}
</style>
