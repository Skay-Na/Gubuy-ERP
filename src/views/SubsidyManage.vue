<template>
  <div class="p-6 max-w-7xl mx-auto">
    <!-- Header Section -->
    <div class="mb-8">
      <div class="flex justify-between items-end gap-4 mb-6">
        <div>
          <h1 class="text-2xl md:text-3xl font-black text-slate-900 tracking-tight mb-1">国补核销管理</h1>
          <p class="text-sm text-slate-400">追踪国家补贴产生、回款进度及批量结算记录</p>
        </div>
        <el-button type="primary" size="large" class="!rounded-xl shadow-lg shadow-blue-100" @click="openSettleDialog">
          <el-icon class="mr-2"><Plus /></el-icon>录入回款记录
        </el-button>
      </div>

      <!-- Stats Cards -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
        <el-card shadow="never" class="!rounded-2xl border-slate-100 bg-white">
          <div class="flex items-center gap-4">
            <div class="p-3 bg-blue-50 rounded-xl text-blue-600">
              <el-icon size="24"><PieChart /></el-icon>
            </div>
            <div>
              <div class="text-xs text-slate-400 font-bold uppercase tracking-wider mb-1">累计产生国补</div>
              <div class="text-2xl font-black text-slate-900">¥{{ summary.total_generated?.toLocaleString() || '0.00' }}</div>
            </div>
          </div>
        </el-card>

        <el-card shadow="never" class="!rounded-2xl border-slate-100 bg-white">
          <div class="flex items-center gap-4">
            <div class="p-3 bg-green-50 rounded-xl text-green-600">
              <el-icon size="24"><Checked /></el-icon>
            </div>
            <div>
              <div class="text-xs text-slate-400 font-bold uppercase tracking-wider mb-1">已回款金额</div>
              <div class="text-2xl font-black text-slate-900">¥{{ summary.total_received?.toLocaleString() || '0.00' }}</div>
            </div>
          </div>
        </el-card>

        <el-card shadow="never" class="!rounded-2xl border-orange-100 bg-orange-50/30">
          <div class="flex items-center gap-4">
            <div class="p-3 bg-orange-100 rounded-xl text-orange-600">
              <el-icon size="24"><Timer /></el-icon>
            </div>
            <div>
              <div class="text-xs text-slate-500 font-bold uppercase tracking-wider mb-1">待收回款余额</div>
              <div class="text-2xl font-black text-orange-600">¥{{ summary.pending_balance?.toLocaleString() || '0.00' }}</div>
            </div>
          </div>
        </el-card>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-8">
      <!-- Left: Settlement History -->
      <div class="lg:col-span-2">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-bold text-slate-800 flex items-center gap-2">
            <el-icon><List /></el-icon>历史回款明细
          </h3>
        </div>
        <el-card shadow="never" class="!rounded-2xl border-slate-100 overflow-hidden" body-class="!p-0">
          <el-table :data="settlements" v-loading="loading" style="width: 100%">
            <el-table-column label="回款日期" width="180">
              <template #default="{ row }">
                <span class="text-slate-600">{{ new Date(row.created_at).toLocaleString() }}</span>
              </template>
            </el-table-column>
            <el-table-column label="回款金额" min-width="150">
              <template #default="{ row }">
                <span class="font-black text-slate-900">¥{{ row.amount.toFixed(2) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="收款账户" width="150">
              <template #default="{ row }">
                <el-tag size="small" effect="plain" class="!rounded-lg">{{ row.account?.name }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="remark" label="备注" show-overflow-tooltip />
          </el-table>
        </el-card>
      </div>

      <!-- Right: Documentation Progress -->
      <div class="lg:col-span-1">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-bold text-slate-800 flex items-center gap-2">
            <el-icon><DocumentChecked /></el-icon>资料提报进度
          </h3>
        </div>
        <el-card shadow="never" class="!rounded-2xl border-slate-100">
          <div class="space-y-6">
            <div v-for="item in docProgress" :key="item.label" class="flex flex-col gap-2">
              <div class="flex justify-between items-end">
                <span class="text-sm font-bold text-slate-600">{{ item.label }}</span>
                <span class="text-xs text-slate-400">¥{{ item.amount.toLocaleString() }}</span>
              </div>
              <el-progress 
                :percentage="Math.min(100, (item.amount / summary.total_generated * 100))" 
                :color="item.color"
                :stroke-width="10"
                :show-text="false"
                class="!rounded-full"
              />
              <div class="text-[10px] text-slate-400 text-right">{{ item.count }} 个订单</div>
            </div>
            
            <div class="pt-4 border-t border-slate-100">
              <p class="text-[11px] text-slate-400 leading-relaxed">
                * 注意：此处的资料进度仅用于跟踪申报流程，财务回款金额以实际录入的打款记录为准。
              </p>
            </div>
          </div>
        </el-card>
      </div>
    </div>

    <!-- Settle Dialog -->
    <el-dialog
      v-model="dialogVisible"
      title="录入国补回款"
      width="450px"
      class="!rounded-2xl"
    >
      <el-form :model="form" label-position="top">
        <el-form-item label="回款金额" required>
          <el-input-number v-model="form.amount" :min="0.01" :precision="2" :controls="false" class="!w-full" placeholder="输入到账金额" />
        </el-form-item>
        <el-form-item label="收款账户" required>
          <el-select v-model="form.account_id" placeholder="选择存入账户" class="!w-full">
            <el-option v-for="acc in accounts" :key="acc.id" :label="acc.name" :value="acc.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="备注说明">
          <el-input v-model="form.remark" type="textarea" placeholder="例：2024年三季度第一批回款" rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="flex gap-2">
          <el-button class="flex-1 !rounded-xl" @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" class="flex-1 !rounded-xl" @click="handleSettle" :loading="submitting">确认录入</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { Plus, PieChart, Checked, Timer, List, DocumentChecked } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'

const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const summary = ref({})
const settlements = ref([])
const accounts = ref([])
const orders = ref([]) // 用于计算资料进度

const form = ref({
  amount: 0,
  account_id: null,
  remark: ''
})

const fetchSummary = async () => {
  const res = await axios.get('/api/subsidy/summary')
  if (res.data.code === 200) {
    summary.value = res.data.data
  }
}

const fetchSettlements = async () => {
  loading.value = true
  const res = await axios.get('/api/subsidy/settlements')
  if (res.data.code === 200) {
    settlements.value = res.data.data
  }
  loading.value = false
}

const fetchAccounts = async () => {
  const res = await axios.get('/api/accounts')
  if (res.data.code === 200) {
    accounts.value = res.data.data
  }
}

const fetchOrders = async () => {
  // 获取所有带国补的订单来计算进度
  const res = await axios.get('/api/orders')
  if (res.data.code === 200) {
    orders.value = res.data.data.filter(o => o.subsidy_amount > 0 && o.order_status === 1)
  }
}

const docProgress = computed(() => {
  const pending = orders.value.filter(o => o.subsidy_status === 1)
  const submitted = orders.value.filter(o => o.subsidy_status === 2)
  const finished = orders.value.filter(o => o.subsidy_status === 3)

  return [
    { 
      label: '资料待提交', 
      amount: pending.reduce((s, o) => s + o.subsidy_amount, 0), 
      count: pending.length,
      color: '#94a3b8'
    },
    { 
      label: '资料已提交政府', 
      amount: (submitted.reduce((s, o) => s + o.subsidy_amount, 0) + finished.reduce((s, o) => s + o.subsidy_amount, 0)), 
      count: (submitted.length + finished.length),
      color: '#3b82f6'
    }
  ]
})

const openSettleDialog = () => {
  form.value = { amount: 0, account_id: null, remark: '' }
  dialogVisible.value = true
}

const handleSettle = async () => {
  if (!form.value.amount || !form.value.account_id) {
    return ElMessage.warning('请填写完整信息')
  }

  try {
    submitting.value = true
    const res = await axios.post('/api/subsidy/settle', form.value)
    if (res.data.code === 200) {
      ElMessage.success('录入成功')
      dialogVisible.value = false
      fetchSummary()
      fetchSettlements()
    }
  } catch (error) {
    ElMessage.error('录入失败')
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  fetchSummary()
  fetchSettlements()
  fetchAccounts()
  fetchOrders()
})
</script>

<style scoped>
:deep(.el-card) {
  border: 1px solid #f1f5f9;
}
:deep(.el-table th) {
  background-color: #f8fafc !important;
  color: #64748b;
  font-weight: bold;
}
</style>
