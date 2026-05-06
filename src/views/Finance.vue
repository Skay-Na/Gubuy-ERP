<template>
  <div class="p-4 md:p-8 max-w-7xl mx-auto space-y-6 md:space-y-8">
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
      <div>
        <h2 class="text-2xl md:text-3xl font-black text-slate-900 tracking-tight">财务数据总览</h2>
        <p class="text-xs text-slate-400 mt-1">资金流水监控与账户余额多维审计</p>
      </div>
      <div class="flex gap-2 w-full md:w-auto">
        <el-button type="success" plain @click="accountDialogVisible = true" class="flex-1 md:flex-none !rounded-xl">
          <el-icon class="mr-1"><Management /></el-icon> 账户管理
        </el-button>
        <el-button type="warning" plain @click="transferDialogVisible = true" class="flex-1 md:flex-none !rounded-xl">
          <el-icon class="mr-1"><Switch /></el-icon> 资金划拨
        </el-button>
      </div>
    </div>
    
    <!-- 账户账面参考余额卡片 -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 md:gap-6">
      <div v-for="(acc, index) in accounts" :key="acc.id" 
           class="account-premium-card group" :class="[getAccountColorClass(index), isMobile ? 'p-4 min-h-[120px]' : 'p-6 min-h-[160px]']">
        <div class="flex justify-between items-start mb-2 md:mb-4">
          <span class="text-sm md:text-base font-bold opacity-90">{{ acc.name }}</span>
          <el-icon class="text-xl opacity-20 group-hover:opacity-100 transition-opacity"><Wallet /></el-icon>
        </div>
        <div class="mb-4">
          <div class="flex items-baseline gap-1">
            <span class="text-xs md:text-lg opacity-70">¥</span>
            <span class="text-xl md:text-3xl font-black tracking-tighter">{{ acc.balance?.toLocaleString('zh-CN', {minimumFractionDigits: 2, maximumFractionDigits: 2}) }}</span>
          </div>
          <div class="text-[10px] md:text-xs opacity-60 font-medium">账户参考余额</div>
        </div>
        <el-button
          size="small"
          class="calibrate-glass-btn !absolute bottom-3 right-3 md:bottom-4 md:right-4"
          @click.stop="openCalibrate(acc)"
        >
          <el-icon class="mr-1"><Checked /></el-icon>校准
        </el-button>
      </div>
    </div>

    <!-- 数据大盘区 -->
    <div class="space-y-4">
      <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 px-1">
        <h3 class="text-[10px] md:text-xs font-black text-slate-400 uppercase tracking-[0.2em] flex items-center">
          <el-icon class="mr-2 text-blue-500"><TrendCharts /></el-icon>
          业务经营概览
        </h3>
        <el-radio-group v-model="period" size="small" @change="fetchData" class="period-switcher w-full sm:w-auto">
          <el-radio-button value="today">今日</el-radio-button>
          <el-radio-button value="month">本月</el-radio-button>
          <el-radio-button value="all">全部</el-radio-button>
        </el-radio-group>
      </div>

      <div class="grid grid-cols-2 lg:grid-cols-4 gap-3 md:gap-6">
        <div class="stat-card blue-theme">
          <div class="stat-header">
            <span class="stat-label">{{ periodLabel }}额</span>
            <el-icon class="opacity-30"><TrendCharts /></el-icon>
          </div>
          <div class="stat-body">
            <span class="text-lg md:text-3xl font-black truncate">¥{{ summary.today_revenue?.toLocaleString(undefined, {minimumFractionDigits: 0}) || '0' }}</span>
          </div>
        </div>
        
        <div class="stat-card red-theme">
          <div class="stat-header">
            <span class="stat-label">{{ periodLabel }}支</span>
            <el-icon class="opacity-30"><Money /></el-icon>
          </div>
          <div class="stat-body">
            <span class="text-lg md:text-3xl font-black truncate">¥{{ summary.total_expense?.toLocaleString(undefined, {minimumFractionDigits: 0}) || '0' }}</span>
          </div>
        </div>

        <div class="stat-card orange-theme">
          <div class="stat-header">
            <span class="stat-label">待收补</span>
            <el-icon class="opacity-30"><Checked /></el-icon>
          </div>
          <div class="stat-body">
            <span class="text-lg md:text-3xl font-black truncate">¥{{ summary.pending_subsidy?.toLocaleString(undefined, {minimumFractionDigits: 0}) || '0' }}</span>
          </div>
        </div>

        <div class="stat-card green-theme">
          <div class="stat-header">
            <span class="stat-label">订单数</span>
            <el-icon class="opacity-30"><List /></el-icon>
          </div>
          <div class="stat-body">
            <span class="text-lg md:text-3xl font-black">{{ summary.order_count || 0 }}</span>
            <span class="text-[10px] ml-1 font-normal opacity-60">笔</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Tabs Section -->
    <el-tabs type="border-card" class="mt-8 shadow-sm rounded-xl overflow-hidden">
      <el-tab-pane label="全口径财务流水 (Ledger)">
        <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
          <div class="flex flex-wrap items-center gap-2">
            <el-select v-model="filterFlowType" placeholder="资金流向" clearable class="!w-full md:!w-32" @change="fetchData">
              <el-option label="全部" value="" />
              <el-option label="收入" :value="1" />
              <el-option label="支出" :value="2" />
              <el-option label="内部调拨" :value="3" />
              <el-option label="系统校准" :value="4" />
            </el-select>
            <el-input v-model="filterCategory" placeholder="搜索分类/备注" clearable class="!w-full md:!w-48" @keyup.enter="fetchData" @clear="fetchData">
              <template #prefix><el-icon><Search /></el-icon></template>
            </el-input>
          </div>
          <el-button type="primary" @click="dialogVisible = true" class="shadow-sm !rounded-xl h-10">
            <el-icon class="mr-1"><Edit /></el-icon> 记一笔
          </el-button>
        </div>

        <!-- PC Table View -->
        <el-table :data="financialLogs" border stripe style="width: 100%" v-loading="loading" max-height="650" class="desktop-only">
          <el-table-column prop="created_at" label="发生时间" width="180">
            <template #default="{ row }">
              {{ formatDate(row.created_at) }}
            </template>
          </el-table-column>
          <el-table-column label="资金流向" width="100" align="center">
            <template #default="{ row }">
              <el-tag :type="getFlowTypeColor(row.flow_type)" effect="dark">{{ getFlowTypeName(row.flow_type) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="category" label="业务分类" width="150">
            <template #default="{ row }">
              <el-tag type="info" plain>{{ row.category }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="关联账户" width="120">
            <template #default="{ row }">
              <el-tag type="info" plain>{{ row.account?.name || '-' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="amount" label="变动金额" width="150" align="right">
            <template #default="{ row }">
              <span class="font-bold text-lg" :class="getAmountColorClass(row.flow_type)">
                {{ getAmountPrefix(row.flow_type) }}¥ {{ row.amount.toFixed(2) }}
              </span>
            </template>
          </el-table-column>
          <el-table-column prop="balance_after" label="变动后余额" width="150" align="right">
            <template #default="{ row }">
              <span class="text-slate-500 font-mono">¥ {{ row.balance_after.toFixed(2) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="related_no" label="关联单号" width="180">
            <template #default="{ row }">
              <span class="text-xs text-slate-400 font-mono">{{ row.related_no || '-' }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="remark" label="备注说明" min-width="250" show-overflow-tooltip />
        </el-table>

        <!-- Mobile Card View -->
        <div class="mobile-only space-y-4" v-loading="loading">
          <div v-for="row in financialLogs" :key="row.id" class="bg-white p-4 rounded-2xl border border-slate-100 shadow-sm">
            <div class="flex justify-between items-start mb-3">
              <div class="flex flex-col gap-1">
                <div class="flex items-center gap-2">
                  <el-tag :type="getFlowTypeColor(row.flow_type)" size="small" effect="dark">{{ getFlowTypeName(row.flow_type) }}</el-tag>
                  <span class="font-bold text-slate-800">{{ row.category }}</span>
                </div>
                <span class="text-[10px] text-slate-400">{{ formatDate(row.created_at) }}</span>
              </div>
              <div class="text-right">
                <div class="font-black text-lg" :class="getAmountColorClass(row.flow_type)">
                  {{ getAmountPrefix(row.flow_type) }}¥{{ row.amount.toFixed(2) }}
                </div>
                <div class="text-[10px] text-slate-400">余额: ¥{{ row.balance_after.toFixed(2) }}</div>
              </div>
            </div>
            <div class="bg-slate-50 p-3 rounded-xl">
              <div class="text-xs text-slate-500 line-clamp-2">{{ row.remark || '无备注' }}</div>
              <div v-if="row.related_no" class="mt-2 text-[10px] text-slate-400 font-mono">单号: {{ row.related_no }}</div>
            </div>
          </div>
          <el-empty v-if="!loading && financialLogs.length === 0" description="暂无记录" />
        </div>
      </el-tab-pane>

      <el-tab-pane label="资金划拨记录">
        <div class="p-2">
          <h3 class="text-lg font-bold text-gray-700 mb-4">内部调拨历史</h3>
          <el-table :data="transfers" border stripe style="width: 100%" v-loading="loading">
            <el-table-column prop="created_at" label="发生时间" width="180">
              <template #default="{ row }">
                {{ formatDate(row.created_at) }}
              </template>
            </el-table-column>
            <el-table-column label="转出账户" width="150">
              <template #default="{ row }">
                <el-tag type="danger" effect="plain">{{ row.from_account?.name }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="转入账户" width="150">
              <template #default="{ row }">
                <el-tag type="success" effect="plain">{{ row.to_account?.name }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="amount" label="划拨金额" width="150">
              <template #default="{ row }">
                <span class="font-bold text-primary">¥ {{ row.amount.toFixed(2) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="remark" label="备注说明" min-width="250" show-overflow-tooltip />
          </el-table>
        </div>
      </el-tab-pane>
    </el-tabs>

    <!-- 记账弹窗 -->
    <el-dialog v-model="dialogVisible" title="记录日常支出" width="500px" @closed="resetForm" destroy-on-close>
      <el-form :model="form" label-width="100px" :rules="rules" ref="formRef">
        <el-form-item label="支出类型" prop="expense_type">
          <el-select v-model="form.expense_type" placeholder="请选择支出类型" class="w-full">
            <el-option label="房租" value="房租" />
            <el-option label="水电费" value="水电费" />
            <el-option label="员工薪资" value="员工薪资" />
            <el-option label="送货安装费" value="送货安装费" />
            <el-option label="其他" value="其他" />
          </el-select>
        </el-form-item>
        <el-form-item label="支出账户" prop="account_id">
          <el-select v-model="form.account_id" placeholder="请选择付款账户" class="w-full">
            <el-option v-for="acc in accounts" :key="acc.id" :label="acc.name" :value="acc.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="支出金额" prop="amount">
          <el-input-number v-model="form.amount" :precision="2" :step="100" :min="0" class="w-full" controls-position="right" />
        </el-form-item>
        <el-form-item label="备注说明" prop="remark">
          <el-input v-model="form.remark" type="textarea" rows="3" placeholder="请输入支出涉及的具体事项（选填）" />
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="handleSubmit" :loading="submitting">确认提交</el-button>
        </span>
      </template>
    </el-dialog>

    <!-- 新增账户弹窗 -->
    <el-dialog v-model="accountDialogVisible" title="新增/管理资金账户" width="450px" destroy-on-close>
      <el-form :model="accountForm" label-width="100px" ref="accountFormRef">
        <el-form-item label="账户名称" prop="name" :rules="[{required: true, message: '请输入账户名称'}]">
          <el-input v-model="accountForm.name" placeholder="如：厂家A余额账户、备用金等" />
        </el-form-item>
        <el-form-item label="初始余额" prop="balance">
          <el-input-number v-model="accountForm.balance" :precision="2" :min="0" class="w-full" controls-position="right" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="accountDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleCreateAccount" :loading="submitting">创建账户</el-button>
      </template>
    </el-dialog>

    <!-- 资金划拨弹窗 -->
    <el-dialog v-model="transferDialogVisible" title="账户资金划拨" width="500px" destroy-on-close>
      <el-alert title="资金划拨仅在内部账户间移动，不计入系统损益支出。" type="info" show-icon :closable="false" class="mb-4" />
      <el-form :model="transferForm" label-width="100px" ref="transferFormRef">
        <el-form-item label="转出账户" prop="from_account_id" :rules="[{required: true, message: '请选择源账户'}]">
          <el-select v-model="transferForm.from_account_id" placeholder="从哪个账户转出" class="w-full">
            <el-option v-for="acc in accounts" :key="acc.id" :label="acc.name + ' (¥' + acc.balance.toFixed(2) + ')'" :value="acc.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="转入账户" prop="to_account_id" :rules="[{required: true, message: '请选择目标账户'}]">
          <el-select v-model="transferForm.to_account_id" placeholder="转入到哪个账户" class="w-full">
            <el-option v-for="acc in accounts" :key="acc.id" :label="acc.name" :value="acc.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="划拨金额" prop="amount" :rules="[{required: true, message: '请输入金额'}]">
          <el-input-number v-model="transferForm.amount" :precision="2" :min="0.01" class="w-full" controls-position="right" />
        </el-form-item>
        <el-form-item label="备注说明" prop="remark">
          <el-input v-model="transferForm.remark" placeholder="如：厂家货款预充值" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="transferDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleTransfer" :loading="submitting">确认划拨</el-button>
      </template>
    </el-dialog>
    <!-- 账户余额校准弹窗 -->
    <el-dialog v-model="calibrateDialogVisible" :title="`一键校准：${calibrateForm.accountName}`" width="480px" destroy-on-close>
      <el-alert
        title="此功能用于一次性将系统余额对齐为真实余额，系统将自动补录差额记录"
        type="info" show-icon :closable="false" class="mb-4"
      />
      <el-form :model="calibrateForm" label-position="top">
        <el-form-item label="当前账面参考余额">
          <div class="text-2xl font-bold text-slate-500">￥ {{ calibrateForm.currentBalance.toFixed(2) }}</div>
        </el-form-item>
        <el-form-item label="真实余额（您手机/支付宝中的实际金额）">
          <el-input-number
            v-model="calibrateForm.realBalance"
            :precision="2" :min="0" :step="100"
            class="w-full" controls-position="right"
          />
          <div v-if="calibrateDiff !== 0" class="mt-2 text-sm" :class="calibrateDiff > 0 ? 'text-green-600' : 'text-red-500'">
            差异：{{ calibrateDiff > 0 ? '+' : '' }}{{ calibrateDiff.toFixed(2) }}
            （{{ calibrateDiff > 0 ? '将补录为收入' : '可能有未记录的支出（如个人消费）' }}）
          </div>
          <div v-if="calibrateDiff === 0" class="mt-2 text-sm text-green-600">✔ 与系统完全一致，无需校准</div>
        </el-form-item>
        <el-form-item label="备注（可选）">
          <el-input v-model="calibrateForm.remark" placeholder="如：月底对账，暂时无法考证每笔个人消费" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="calibrateDialogVisible = false">取消</el-button>
        <el-button type="warning" :loading="submitting" @click="handleCalibrate" :disabled="calibrateDiff === 0">
          确认校准
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, reactive, computed } from 'vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'
import { 
  TrendCharts, 
  Money, 
  Checked, 
  List, 
  Plus,
  Wallet,
  Management,
  Search,
  Edit,
  Switch
} from '@element-plus/icons-vue'

const summary = ref({})
const financialLogs = ref([])
const transfers = ref([])
const accounts = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const accountDialogVisible = ref(false)
const transferDialogVisible = ref(false)
const calibrateDialogVisible = ref(false)
const submitting = ref(false)
const isMobile = ref(window.innerWidth < 768)
const period = ref('today')

window.addEventListener('resize', () => {
  isMobile.value = window.innerWidth < 768
})

const periodLabel = computed(() => {
  const map = { 'today': '今日', 'month': '本月', 'all': '累计' }
  return map[period.value]
})

// 校准表单
const calibrateForm = reactive({
  accountId: null,
  accountName: '',
  currentBalance: 0,
  realBalance: 0,
  remark: ''
})

// 实时计算差额
const calibrateDiff = computed(() => {
  return calibrateForm.realBalance - calibrateForm.currentBalance
})

const formRef = ref(null)
const form = reactive({
  expense_type: '',
  amount: 0,
  remark: '',
  account_id: null
})

const accountFormRef = ref(null)
const accountForm = reactive({
  name: '',
  balance: 0
})

const transferFormRef = ref(null)
const transferForm = reactive({
  from_account_id: null,
  to_account_id: null,
  amount: 0,
  remark: ''
})

const rules = {
  expense_type: [{ required: true, message: '请选择支出类型', trigger: 'change' }],
  amount: [{ required: true, message: '请输入有效金额', trigger: 'blur' }],
  account_id: [{ required: true, message: '请选择付款账户', trigger: 'change' }]
}

const filterAccount = ref('')
const filterFlowType = ref('')
const filterCategory = ref('')

const fetchData = async () => {
  loading.value = true
  try {
    const [summaryRes, logRes, transferRes] = await Promise.all([
      axios.get(`/api/finance/summary?period=${period.value}`),
      axios.get('/api/finance/logs', {
        params: {
          account_id: filterAccount.value,
          flow_type: filterFlowType.value,
          category: filterCategory.value
        }
      }),
      axios.get('/api/accounts/transfers')
    ])
    summary.value = summaryRes.data.code === 200 ? summaryRes.data.data : summaryRes.data
    accounts.value = summary.value.accounts || []
    financialLogs.value = logRes.data.data || []
    transfers.value = transferRes.data.data || []
  } catch (err) {
    ElMessage.error('数据加载失败')
  } finally {
    loading.value = false
  }
}

const handleSubmit = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (valid) {
      submitting.value = true
      try {
        await axios.post('/api/expenses', form)
        ElMessage.success('支出记录已保存')
        dialogVisible.value = false
        fetchData()
      } catch (err) {
        ElMessage.error('保存失败')
      } finally {
        submitting.value = false
      }
    }
  })
}

const handleCreateAccount = async () => {
  if (!accountFormRef.value) return
  await accountFormRef.value.validate(async (valid) => {
    if (valid) {
      submitting.value = true
      try {
        await axios.post('/api/accounts', accountForm)
        ElMessage.success('账户创建成功')
        accountDialogVisible.value = false
        accountForm.name = ''
        accountForm.balance = 0
        fetchData()
      } catch (err) {
        ElMessage.error('创建失败')
      } finally {
        submitting.value = false
      }
    }
  })
}

const handleTransfer = async () => {
  if (!transferFormRef.value) return
  await transferFormRef.value.validate(async (valid) => {
    if (valid) {
      if (transferForm.from_account_id === transferForm.to_account_id) {
        return ElMessage.warning('转出和转入账户不能相同')
      }
      submitting.value = true
      try {
        const res = await axios.post('/api/accounts/transfer', transferForm)
        if (res.data.code === 200) {
          ElMessage.success('资金划拨成功')
          transferDialogVisible.value = false
          transferForm.amount = 0
          transferForm.remark = ''
          fetchData()
        } else {
          ElMessage.error(res.data.msg)
        }
      } catch (err) {
        ElMessage.error(err.response?.data?.msg || '划拨失败')
      } finally {
        submitting.value = false
      }
    }
  })
}

const resetForm = () => {
  form.expense_type = ''
  form.amount = 0
  form.remark = ''
  form.account_id = null
  if (formRef.value) formRef.value.resetFields()
}

const formatDate = (dateStr) => {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit'
  })
}

const getFlowTypeName = (type) => {
  const map = { 1: '收入', 2: '支出', 3: '调拨', 4: '校准' }
  return map[type] || '未知'
}

const getFlowTypeColor = (type) => {
  const map = { 1: 'success', 2: 'danger', 3: 'warning', 4: 'info' }
  return map[type] || 'info'
}

const getAmountColorClass = (type) => {
  const map = { 1: 'text-emerald-500', 2: 'text-rose-500', 3: 'text-amber-500', 4: 'text-slate-500' }
  return map[type] || 'text-slate-500'
}

const getAmountPrefix = (type) => {
  const map = { 1: '+ ', 2: '- ', 3: '', 4: '' }
  return map[type] || ''
}

const getTagType = (type) => {
  const map = {
    '房租': 'danger', '水电费': 'warning', '员工薪资': 'success', '送货安装费': 'info', '采购货款': 'primary'
  }
  return map[type] || ''
}

const getAccountColorClass = (index) => {
  const colors = [
    'from-blue-500 to-blue-600',
    'from-green-500 to-green-600',
    'from-purple-500 to-purple-600',
    'from-orange-500 to-orange-600',
    'from-cyan-500 to-cyan-600',
    'from-pink-500 to-pink-600'
  ]
  return colors[index % colors.length]
}

// 打开余额校准弹窗
const openCalibrate = (acc) => {
  calibrateForm.accountId = acc.id
  calibrateForm.accountName = acc.name
  calibrateForm.currentBalance = acc.balance
  calibrateForm.realBalance = acc.balance
  calibrateForm.remark = ''
  calibrateDialogVisible.value = true
}

// 提交余额校准
const handleCalibrate = async () => {
  if (calibrateDiff.value === 0) return
  submitting.value = true
  try {
    const res = await axios.put(
      `/api/accounts/${calibrateForm.accountId}/calibrate`,
      { real_balance: calibrateForm.realBalance, remark: calibrateForm.remark }
    )
    if (res.data.code === 200) {
      ElMessage.success(res.data.msg)
      calibrateDialogVisible.value = false
      fetchData()
    } else {
      ElMessage.error(res.data.msg || '校准失败')
    }
  } catch (err) {
    ElMessage.error(err.response?.data?.msg || '网络异常')
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>

<style scoped>
.account-premium-card {
  position: relative;
  border-radius: 24px;
  background: linear-gradient(135deg, var(--tw-gradient-from) 0%, var(--tw-gradient-to) 100%);
  color: white;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.1);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  overflow: hidden;
}

.account-premium-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
}

.calibrate-glass-btn {
  background: rgba(255, 255, 255, 0.15) !important;
  backdrop-filter: blur(8px);
  border: 1px solid rgba(255, 255, 255, 0.2) !important;
  color: white !important;
  border-radius: 12px;
  font-weight: 600;
  transition: all 0.2s;
}

.calibrate-glass-btn:hover {
  background: rgba(255, 255, 255, 0.25) !important;
  border-color: rgba(255, 255, 255, 0.4) !important;
}

.stat-card {
  background: white;
  padding: 16px;
  border-radius: 20px;
  border: 1px solid #f1f5f9;
  display: flex;
  flex-direction: column;
  gap: 12px;
  transition: all 0.3s ease;
}

@media (min-width: 768px) {
  .stat-card {
    padding: 24px;
    border-radius: 24px;
  }
}

.stat-card:hover {
  border-color: #e2e8f0;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.05);
}

.stat-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.stat-label {
  font-size: 10px;
  font-weight: 700;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

@media (min-width: 768px) {
  .stat-label {
    font-size: 12px;
  }
}

.blue-theme { border-left: 4px solid #3b82f6; }
.red-theme { border-left: 4px solid #ef4444; }
.orange-theme { border-left: 4px solid #f59e0b; }
.green-theme { border-left: 4px solid #10b981; }

.period-switcher :deep(.el-radio-button__inner) {
  border-radius: 10px !important;
  margin: 0 2px;
  border: 1px solid transparent !important;
  background: #f8fafc;
  font-size: 11px;
}

.period-switcher :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  background: #0f172a !important;
  color: white !important;
  box-shadow: none !important;
}

:deep(.el-tabs--border-card) {
  border: 1px solid #f1f5f9;
  box-shadow: none;
}

:deep(.el-tabs__header) {
  background-color: #f8fafc;
  border-bottom: 1px solid #f1f5f9;
}

/* Gradients for accounts */
.from-blue-500.to-blue-600 { --tw-gradient-from: #3b82f6; --tw-gradient-to: #1d4ed8; }
.from-green-500.to-green-600 { --tw-gradient-from: #10b981; --tw-gradient-to: #059669; }
.from-purple-500.to-purple-600 { --tw-gradient-from: #8b5cf6; --tw-gradient-to: #6d28d9; }
.from-orange-500.to-orange-600 { --tw-gradient-from: #f59e0b; --tw-gradient-to: #d97706; }
.from-cyan-500.to-cyan-600 { --tw-gradient-from: #06b6d4; --tw-gradient-to: #0891b2; }
.from-pink-500.to-pink-600 { --tw-gradient-from: #ec4899; --tw-gradient-to: #be185d; }
</style>
