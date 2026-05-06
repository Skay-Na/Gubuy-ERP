<template>
  <div class="p-6 max-w-7xl mx-auto">
    <!-- Header Section -->
    <div class="mb-8">
      <div class="flex flex-col md:flex-row justify-between items-start md:items-end gap-4 mb-6">
        <div>
          <h1 class="text-2xl md:text-3xl font-black text-slate-900 tracking-tight mb-1 whitespace-nowrap">订单管理与历史</h1>
          <p class="text-sm text-slate-400">查看和管理所有的销售订单及款项流转</p>
        </div>
        <div class="flex flex-wrap items-center gap-3 w-full md:w-auto">
          <el-date-picker
            v-model="dateRange"
            type="daterange"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            class="!w-full md:!w-72 shadow-sm"
            @change="fetchOrders"
          />
          <div class="flex w-full md:w-auto gap-2">
            <el-input 
              v-model="searchKeyword" 
              placeholder="搜索订单/客户/电话" 
              clearable 
              class="flex-1 md:!w-64"
              @input="handleSearchInput"
            >
              <template #prefix><el-icon><Search /></el-icon></template>
            </el-input>
          </div>
        </div>
      </div>

      <!-- 快捷状态分类 Tab -->
      <div class="flex items-center gap-2 overflow-x-auto no-scrollbar pb-1">
        <div 
          v-for="tab in filterTabs" 
          :key="tab.value"
          @click="selectTab(tab.value)"
          class="px-5 py-2 rounded-xl text-sm font-bold whitespace-nowrap cursor-pointer transition-all border"
          :class="activeTab === tab.value 
            ? 'bg-blue-600 text-white border-blue-600 shadow-md shadow-blue-100' 
            : 'bg-white text-slate-500 border-slate-100 hover:border-slate-200'"
        >
          {{ tab.label }}
        </div>
      </div>
    </div>

    <!-- Data Table Card -->
    <el-card shadow="never" class="!rounded-2xl border-slate-100 shadow-sm overflow-hidden" body-class="!p-0">
      <!-- PC View -->
      <el-table 
        :data="orders" 
        v-loading="loading" 
        style="width: 100%" 
        class="desktop-only hidden md:block"
        :header-cell-style="{ background: '#f8fafc', color: '#475569', fontWeight: '600', height: '54px' }"
        :row-style="{ height: '64px' }"
        :row-class-name="({row}) => row.order_status === 2 ? 'opacity-50 grayscale' : ''"
      >
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="px-8 py-6 bg-slate-50/50">
              <div class="flex justify-between items-center mb-4">
                <h4 class="text-base font-bold text-slate-800">订单深度明细</h4>
                <div class="flex gap-2">
                  <el-tag v-if="row.order_status === 2" type="info" effect="dark">已废弃单据</el-tag>
                  <el-tag v-else type="success" effect="dark">有效业务单据</el-tag>
                </div>
              </div>

              <el-table :data="row.order_items" size="small" border class="shadow-sm rounded-lg overflow-hidden mb-6">
                <el-table-column label="商品信息" min-width="250">
                  <template #default="scope">
                    <div class="flex flex-col">
                      <span class="font-bold text-slate-700">{{ scope.row.product?.name || '未知商品' }}</span>
                      <span class="text-xs text-slate-400">{{ scope.row.product?.specification || '-' }} / {{ scope.row.product?.model || '-' }}</span>
                    </div>
                  </template>
                </el-table-column>
                <el-table-column label="单价" width="150" align="center">
                  <template #default="scope">
                    <span :class="{'line-through text-slate-400': scope.row.is_gift, 'font-bold': !scope.row.is_gift}">¥{{ scope.row.unit_price.toFixed(2) }}</span>
                    <el-tag v-if="scope.row.is_gift" size="small" type="danger" effect="dark" class="ml-1">赠</el-tag>
                  </template>
                </el-table-column>
                <el-table-column prop="quantity" label="数量" width="100" align="center" />
                <el-table-column label="小计" width="120" align="right">
                  <template #default="scope">
                    <span class="font-bold text-slate-700">¥{{ (scope.row.unit_price * scope.row.quantity).toFixed(2) }}</span>
                  </template>
                </el-table-column>
              </el-table>
              
              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <!-- 核心财务信息 -->
                <el-descriptions title="核心账务" :column="1" border size="small" class="bg-white p-3 rounded-xl shadow-sm">
                  <el-descriptions-item label="结算金额">
                    <span class="font-bold text-slate-900">¥{{ row.actual_pay_amount.toFixed(2) }}</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="实收金额">
                    <span class="text-blue-600 font-bold">¥{{ row.payment_status === 2 ? row.actual_pay_amount.toFixed(2) : row.deposit_amount.toFixed(2) }}</span>
                    <el-tag size="small" class="ml-2" :type="row.payment_status === 2 ? 'success' : 'warning'">
                      {{ row.payment_status === 2 ? '已结清' : '仅付定金' }}
                    </el-tag>
                  </el-descriptions-item>
                  <el-descriptions-item label="支付方式">
                    {{ getPaymentMethodLabel(row.payment_method) }}
                    <span v-if="row.account" class="text-slate-400 text-[10px] ml-1">({{ row.account.name }})</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="开单人员">
                    {{ row.employee?.name || '未知' }} (工号: {{ row.employee?.emp_no || '-' }})
                  </el-descriptions-item>
                </el-descriptions>

                <!-- 履约与售后 -->
                <el-descriptions title="履约与售后" :column="1" border size="small" class="bg-white p-3 rounded-xl shadow-sm">
                  <el-descriptions-item label="交付方式">
                    <el-tag size="small" effect="plain">{{ getDeliveryMethodLabel(row.delivery_method) }}</el-tag>
                  </el-descriptions-item>
                  <el-descriptions-item label="安装状态">
                    <span :class="row.is_installed ? 'text-green-600' : 'text-slate-400'">
                      {{ row.is_installed ? '✅ 已确认安装' : '⏳ 待安装' }}
                    </span>
                  </el-descriptions-item>
                  <el-descriptions-item label="安装人员" v-if="row.is_installed">
                    {{ row.installer?.name }}
                  </el-descriptions-item>
                  <el-descriptions-item label="安装时间" v-if="row.is_installed">
                    {{ formatDate(row.install_time) }}
                  </el-descriptions-item>
                </el-descriptions>

                <!-- 国补资料 -->
                <el-descriptions title="国补政策" :column="1" border size="small" class="bg-white p-3 rounded-xl shadow-sm">
                  <el-descriptions-item label="是否包含补贴">
                    {{ row.subsidy_amount > 0 ? '是' : '否' }}
                  </el-descriptions-item>
                  <el-descriptions-item label="补贴金额" v-if="row.subsidy_amount > 0">
                    <span class="text-orange-500 font-bold">¥{{ row.subsidy_amount.toFixed(2) }}</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="资料进度" v-if="row.subsidy_amount > 0">
                    <el-tag :type="getSubsidyType(row.subsidy_status)">{{ getSubsidyLabel(row.subsidy_status) }}</el-tag>
                  </el-descriptions-item>
                </el-descriptions>

                <!-- 客户与市场 -->
                <el-descriptions title="客户与市场" :column="1" border size="small" class="bg-white p-3 rounded-xl shadow-sm">
                  <el-descriptions-item label="客户名称">{{ row.customer_name || '散客' }}</el-descriptions-item>
                  <el-descriptions-item label="联系电话">{{ row.customer_phone || '-' }}</el-descriptions-item>
                  <el-descriptions-item label="推荐渠道">{{ row.referrer_name || '无' }}</el-descriptions-item>
                  <el-descriptions-item label="渠道奖励">
                    ¥{{ (row.referral_fee || 0).toFixed(2) }} 
                    <el-tag v-if="row.referral_fee > 0" size="small" :type="row.is_referral_fee_paid ? 'success' : 'info'" class="ml-2">
                      {{ row.is_referral_fee_paid ? '已发放' : '待结算' }}
                    </el-tag>
                  </el-descriptions-item>
                </el-descriptions>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="order_no" label="订单号" min-width="180" />
        <el-table-column label="客户信息" min-width="150">
          <template #default="{ row }">
            <div class="flex flex-col">
              <span class="font-bold text-slate-700">{{ row.customer_name || '散客' }}</span>
              <span class="text-[10px] text-slate-400 font-mono">{{ row.customer_phone || '-' }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="订单金额" width="120">
          <template #default="{ row }">
            <span class="font-bold text-slate-900">¥{{ row.actual_pay_amount.toFixed(2) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="支付状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.order_status === 2" type="info">已取消</el-tag>
            <el-tag v-else :type="row.payment_status === 2 ? 'success' : 'warning'">
              {{ row.payment_status === 1 ? '定金' : '全款' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="安装状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.order_status !== 2" :type="row.is_installed ? 'primary' : 'info'" effect="plain">
              {{ row.is_installed ? '已安装' : '待安装' }}
            </el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="国补状态" width="110" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.order_status !== 2 && row.subsidy_status !== 0" :type="getSubsidyType(row.subsidy_status)" effect="light">
              {{ getSubsidyLabel(row.subsidy_status) }}
            </el-tag>
            <span v-else-if="row.subsidy_status === 0" class="text-xs text-slate-300">无补贴</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right" align="center">
          <template #default="{ row }">
            <div v-if="row.order_status === 1" class="flex items-center justify-center gap-2">
              <el-button v-if="!row.is_installed" type="primary" size="small" @click="handleInstall(row)">安装</el-button>
              <el-button v-if="row.payment_status === 1" type="success" size="small" @click="handlePayBalance(row)">结清</el-button>
              <el-dropdown trigger="click">
                <el-button size="small" class="px-2"><el-icon><MoreFilled /></el-icon></el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item v-if="row.subsidy_status === 1" @click="handleSubmitSubsidy(row)">提交国补</el-dropdown-item>
                    <el-dropdown-item v-if="row.subsidy_status === 2" @click="handleVerify(row)">国补核销</el-dropdown-item>
                    <el-dropdown-item divided type="danger" @click="handleCancel(row)" class="!text-red-500">取消订单</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
            <span v-else class="text-xs text-slate-400">-</span>
          </template>
        </el-table-column>
      </el-table>

      <!-- Mobile View -->
      <div class="mobile-only md:hidden p-4 space-y-4" v-loading="loading">
        <div v-for="row in orders" :key="row.id" class="bg-white border border-slate-100 rounded-2xl p-4 shadow-sm" :class="row.order_status === 2 ? 'opacity-60' : ''">
          <div class="flex justify-between items-start mb-3">
            <div class="flex-1 min-w-0 pr-2">
              <div class="text-[10px] text-slate-400 font-mono mb-1">{{ row.order_no }}</div>
              <div class="font-bold text-slate-800 text-base leading-tight">{{ row.customer_name || '散客' }}</div>
            </div>
            <div class="text-right flex-shrink-0">
              <div class="text-slate-900 font-black text-lg">¥{{ row.actual_pay_amount.toFixed(2) }}</div>
            </div>
          </div>

          <div class="flex flex-wrap gap-2 mb-4">
             <el-tag size="small" :type="row.order_status === 2 ? 'info' : (row.payment_status === 2 ? 'success' : 'warning')">
               {{ row.order_status === 2 ? '已取消' : (row.payment_status === 1 ? '定金' : '全款') }}
             </el-tag>
             <el-tag size="small" v-if="row.order_status !== 2" :type="getSubsidyType(row.subsidy_status)">国补:{{ getSubsidyLabel(row.subsidy_status) }}</el-tag>
             <el-tag size="small" v-if="row.order_status !== 2" :type="row.is_installed ? 'primary' : 'info'">{{ row.is_installed ? '已安装' : '待安装' }}</el-tag>
          </div>

          <div v-if="row.order_status === 1" class="flex justify-end gap-2 pt-3 border-t border-slate-100">
            <el-button v-if="!row.is_installed" size="small" type="primary" @click="handleInstall(row)" class="!rounded-lg">确认安装</el-button>
            <el-button v-if="row.payment_status === 1" size="small" type="success" @click="handlePayBalance(row)" class="!rounded-lg">结尾款</el-button>
            <el-dropdown trigger="click">
              <el-button size="small" class="!rounded-lg px-2"><el-icon><MoreFilled /></el-icon></el-button>
              <template #dropdown>
                <el-dropdown-menu>
                   <el-dropdown-item v-if="row.subsidy_status === 1" @click="handleSubmitSubsidy(row)">提交国补</el-dropdown-item>
                   <el-dropdown-item v-if="row.subsidy_status === 2" @click="handleVerify(row)">国补核销</el-dropdown-item>
                   <el-dropdown-item divided type="danger" @click="handleCancel(row)" class="!text-red-500">取消订单</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
        <el-empty v-if="!loading && orders.length === 0" description="暂无订单" />
      </div>
    </el-card>

    <!-- 结清尾款弹窗 -->
    <el-dialog v-model="balanceDialogVisible" title="结清尾款" width="500px" destroy-on-close class="!rounded-2xl">
      <template v-if="balanceTargetOrder">
        <el-alert title="结算说明" type="info" :closable="false" show-icon class="mb-4 !rounded-xl">系统将先退还定金，再按全款金额入账。</el-alert>
        <el-descriptions :column="1" border class="mb-4">
          <el-descriptions-item label="订单号">{{ balanceTargetOrder.order_no }}</el-descriptions-item>
          <el-descriptions-item label="客户">{{ balanceTargetOrder.customer_name || '散客' }}</el-descriptions-item>
          <el-descriptions-item label="待收全款"><span class="text-green-600 font-bold">¥{{ balanceTargetOrder.actual_pay_amount.toFixed(2) }}</span></el-descriptions-item>
        </el-descriptions>
        <div class="bg-slate-50 p-4 rounded-xl border border-slate-100">
          <label class="block text-sm font-bold text-slate-700 mb-2">确认全款支付方式</label>
          <el-select v-model="finalPaymentMethod" class="w-full">
            <el-option v-for="opt in paymentMethodOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </div>
      </template>
      <template #footer>
        <el-button @click="balanceDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="confirmPayBalance">确认结算</el-button>
      </template>
    </el-dialog>

    <!-- 确认安装弹窗 -->
    <el-dialog v-model="installDialogVisible" title="确认安装" width="400px" destroy-on-close class="!rounded-2xl">
      <div class="mb-4">
        <label class="block text-sm font-bold text-slate-700 mb-2">选择安装师傅</label>
        <el-select v-model="selectedInstallerId" class="w-full" placeholder="请选择师傅">
          <el-option v-for="emp in employees" :key="emp.id" :label="emp.name" :value="emp.id" />
        </el-select>
      </div>
      <template #footer>
        <el-button @click="installDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="confirmInstall" :disabled="!selectedInstallerId">确认提交</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search, InfoFilled, MoreFilled } from '@element-plus/icons-vue'
import dayjs from 'dayjs'

const orders = ref([])
const loading = ref(false)
const searchKeyword = ref('')
const dateRange = ref([])
const activeTab = ref('all')

const filterTabs = [
  { label: '全部订单', value: 'all' },
  { label: '待确认安装', value: 'pending_install' },
  { label: '待催收尾款', value: 'pending_balance' },
  { label: '待提交国补', value: 'pending_subsidy' },
  { label: '已结清全款', value: 'settled' },
  { label: '已取消', value: 'cancelled' },
]

const selectTab = (val) => {
  activeTab.value = val
  fetchOrders()
}

let searchTimer = null
const handleSearchInput = () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    fetchOrders()
  }, 500)
}

const fetchOrders = async () => {
  loading.value = true
  try {
    const params = { 
      keyword: searchKeyword.value,
    }
    
    if (dateRange.value && dateRange.value.length === 2) {
      params.start_date = dayjs(dateRange.value[0]).startOf('day').toISOString()
      params.end_date = dayjs(dateRange.value[1]).endOf('day').toISOString()
    }

    const res = await axios.get('/api/orders', { params })
    if (res.data.code === 200) {
      let rawData = res.data.data
      
      // 前端进行状态聚合筛选
      if (activeTab.value === 'pending_install') {
        rawData = rawData.filter(o => o.order_status === 1 && !o.is_installed)
      } else if (activeTab.value === 'pending_balance') {
        rawData = rawData.filter(o => o.order_status === 1 && o.payment_status === 1 && o.is_installed)
      } else if (activeTab.value === 'pending_subsidy') {
        rawData = rawData.filter(o => o.order_status === 1 && o.subsidy_status === 1)
      } else if (activeTab.value === 'settled') {
        rawData = rawData.filter(o => o.order_status === 1 && o.payment_status === 2)
      } else if (activeTab.value === 'cancelled') {
        rawData = rawData.filter(o => o.order_status === 2)
      }
      
      orders.value = rawData
    }
  } catch (error) {
    ElMessage.error('获取订单列表失败')
  } finally {
    loading.value = false
  }
}

const formatDate = (date) => dayjs(date).format('YYYY-MM-DD HH:mm')

const getSubsidyLabel = (status) => {
  const map = { 0: '无', 1: '待提交', 2: '已提交', 3: '已回款' }
  return map[status] || '未知'
}

const getPaymentMethodLabel = (method) => {
  const map = { 1: '支付宝', 2: '微信', 3: '公户' }
  return map[method] || '其他'
}

const getDeliveryMethodLabel = (method) => {
  const map = { 1: '门店自提', 2: '主仓发货', 3: '云仓代发' }
  return map[method] || '其他'
}

const getSubsidyType = (status) => {
  const map = { 0: 'info', 1: 'danger', 2: 'warning', 3: 'success' }
  return map[status] || 'info'
}

const handleSubmitSubsidy = (row) => {
  ElMessageBox.confirm('确认已向政府系统提交资料？', '提交资料', { type: 'info' }).then(async () => {
    try {
      const res = await axios.put(`/api/orders/${row.id}/subsidy`, { target_status: 2 })
      if (res.data.code === 200) { fetchOrders(); ElMessage.success('提交成功') }
    } catch (error) { ElMessage.error('操作失败') }
  })
}

const handleVerify = (row) => {
  ElMessageBox.confirm('确认国补资金已打入对公账户？', '确认核销', { type: 'warning' }).then(async () => {
    try {
      const res = await axios.put(`/api/orders/${row.id}/subsidy`, { target_status: 3 })
      if (res.data.code === 200) { fetchOrders(); ElMessage.success('核销成功') }
    } catch (error) { ElMessage.error('核销失败') }
  })
}

const balanceDialogVisible = ref(false)
const balanceTargetOrder = ref(null)
const finalPaymentMethod = ref(1)
const paymentMethodOptions = [
  { value: 1, label: '支付宝' },
  { value: 2, label: '微信' },
  { value: 3, label: '公户' },
]

const handlePayBalance = (row) => {
  balanceTargetOrder.value = row
  finalPaymentMethod.value = row.payment_method
  balanceDialogVisible.value = true
}

const confirmPayBalance = async () => {
  if (!balanceTargetOrder.value) return
  try {
    const res = await axios.put(`/api/orders/${balanceTargetOrder.value.id}/balance`, {
      final_payment_method: finalPaymentMethod.value
    })
    if (res.data.code === 200) {
      ElMessage.success('结算完成')
      balanceDialogVisible.value = false
      fetchOrders()
    }
  } catch (error) { ElMessage.error(error.response?.data?.msg || '操作失败') }
}

const handleCancel = (row) => {
  ElMessageBox.confirm(`确认取消订单 ${row.order_no}？此操作将退还已收金额并回滚库存！`, '高危操作', { type: 'error' }).then(async () => {
    try {
      const res = await axios.put(`/api/orders/${row.id}/cancel`)
      if (res.data.code === 200) { fetchOrders(); ElMessage.success('订单已取消') }
    } catch (error) { ElMessage.error('取消失败') }
  })
}

const installDialogVisible = ref(false)
const installTargetOrder = ref(null)
const selectedInstallerId = ref(null)
const employees = ref([])

const fetchEmployees = async () => {
  try {
    const res = await axios.get('/api/employees')
    if (res.data.code === 200) employees.value = res.data.data
  } catch (error) {}
}

const handleInstall = (row) => {
  installTargetOrder.value = row
  selectedInstallerId.value = null
  if (employees.value.length === 0) fetchEmployees()
  installDialogVisible.value = true
}

const confirmInstall = async () => {
  if (!selectedInstallerId.value) return
  try {
    const res = await axios.put(`/api/orders/${installTargetOrder.value.id}/install`, {
      installer_id: selectedInstallerId.value
    })
    if (res.data.code === 200) {
      ElMessage.success('安装确认成功')
      installDialogVisible.value = false
      fetchOrders()
    }
  } catch (error) { ElMessage.error('操作失败') }
}

onMounted(() => fetchOrders())
</script>

<style scoped>
:deep(.el-card__body) { padding: 0; }
</style>
