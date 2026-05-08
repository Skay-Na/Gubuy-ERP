<template>
  <div class="p-3 md:p-6 max-w-7xl mx-auto space-y-4 md:space-y-8 bg-slate-50/30 min-h-screen">
    <!-- Header Area -->
    <div class="flex flex-col md:flex-row items-start md:items-end justify-between gap-3 md:gap-4">
      <div>
        <h1 class="text-xl md:text-3xl font-black text-slate-900 tracking-tight mb-0.5 md:mb-1">数据驾驶舱</h1>
        <p class="text-[10px] md:text-sm text-slate-400 font-medium">实时监控门店运营核心指标</p>
      </div>
      <div class="w-full md:w-auto flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
        <el-radio-group v-model="period" size="small" @change="fetchData" class="flex-1 sm:flex-none">
          <el-radio-button value="today">今日</el-radio-button>
          <el-radio-button value="month">本月</el-radio-button>
          <el-radio-button value="all">累计</el-radio-button>
        </el-radio-group>
        <div class="px-2 py-1 bg-white rounded-lg border border-slate-100 text-[9px] md:text-[10px] text-slate-400 font-mono shadow-sm text-center">
          SYNC: {{ currentTime }}
        </div>
      </div>
    </div>
    
    <!-- 1. 核心业务指标 -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3 md:gap-6">
      <!-- 营收 -->
      <div class="metric-card bg-white rounded-2xl md:rounded-3xl p-3 md:p-5 border border-slate-100 shadow-sm hover:shadow-xl transition-all duration-300 relative overflow-hidden group">
        <div class="flex items-center gap-2 md:gap-4 mb-2 md:mb-4">
          <div class="w-8 h-8 md:w-12 md:h-12 rounded-xl bg-blue-600 flex items-center justify-center shadow-lg shadow-blue-200 shrink-0">
            <el-icon class="text-white text-base md:text-xl"><Money /></el-icon>
          </div>
          <div class="min-w-0">
            <div class="text-[9px] md:text-[10px] text-slate-400 font-bold uppercase truncate">业务营收</div>
            <div class="text-sm md:text-2xl font-black text-slate-900 leading-none truncate">
              <span class="text-[10px] md:text-xs mr-0.5">¥</span>{{ summary.today_revenue?.toLocaleString() || '0' }}
            </div>
          </div>
        </div>
        <div class="text-[8px] md:text-[10px] text-slate-400 truncate">成交数: <span class="text-blue-600 font-bold">{{ summary.order_count || 0 }}</span></div>
      </div>

      <!-- 国补 -->
      <div class="metric-card bg-white rounded-2xl md:rounded-3xl p-3 md:p-5 border border-slate-100 shadow-sm hover:shadow-xl transition-all duration-300 relative overflow-hidden group">
        <div class="flex items-center gap-2 md:gap-4 mb-2 md:mb-4">
          <div class="w-8 h-8 md:w-12 md:h-12 rounded-xl bg-orange-500 flex items-center justify-center shadow-lg shadow-orange-200 shrink-0">
            <el-icon class="text-white text-base md:text-xl"><Wallet /></el-icon>
          </div>
          <div class="min-w-0">
            <div class="text-[9px] md:text-[10px] text-slate-400 font-bold uppercase truncate">国补待收</div>
            <div class="text-sm md:text-2xl font-black text-slate-900 leading-none truncate">
              <span class="text-[10px] md:text-xs mr-0.5">¥</span>{{ summary.pending_subsidy?.toLocaleString() || '0' }}
            </div>
          </div>
        </div>
        <div class="text-[8px] md:text-[10px] text-slate-400 truncate">待核销金额</div>
      </div>

      <!-- 尾款 -->
      <div class="metric-card bg-white rounded-2xl md:rounded-3xl p-3 md:p-5 border border-red-50 shadow-sm hover:shadow-xl transition-all duration-300 relative overflow-hidden group">
        <div class="flex items-center gap-2 md:gap-4 mb-2 md:mb-4">
          <div class="w-8 h-8 md:w-12 md:h-12 rounded-xl bg-red-600 flex items-center justify-center shadow-lg shadow-red-200 shrink-0">
            <el-icon class="text-white text-base md:text-xl"><Bell /></el-icon>
          </div>
          <div class="min-w-0">
            <div class="text-[9px] md:text-[10px] text-slate-400 font-bold uppercase truncate">待收尾款</div>
            <div class="text-sm md:text-2xl font-black text-red-600 leading-none truncate">
              <span class="text-[10px] md:text-xs mr-0.5">¥</span>{{ summary.pending_balance?.toLocaleString() || '0' }}
            </div>
          </div>
        </div>
        <div class="text-[8px] md:text-[10px] text-slate-400 truncate">涉及 <span class="text-red-600 font-bold">{{ summary.pending_balance_count || 0 }}</span> 笔</div>
      </div>

      <!-- 支出 -->
      <div class="metric-card bg-white rounded-2xl md:rounded-3xl p-3 md:p-5 border border-slate-100 shadow-sm hover:shadow-xl transition-all duration-300 relative overflow-hidden group">
        <div class="flex items-center gap-2 md:gap-4 mb-2 md:mb-4">
          <div class="w-8 h-8 md:w-12 md:h-12 rounded-xl bg-purple-600 flex items-center justify-center shadow-lg shadow-purple-200 shrink-0">
            <el-icon class="text-white text-base md:text-xl"><Box /></el-icon>
          </div>
          <div class="min-w-0">
            <div class="text-[9px] md:text-[10px] text-slate-400 font-bold uppercase truncate">运营支出</div>
            <div class="text-sm md:text-2xl font-black text-slate-900 leading-none truncate">
              <span class="text-[10px] md:text-xs mr-0.5">¥</span>{{ summary.total_expense?.toLocaleString() || '0' }}
            </div>
          </div>
        </div>
        <div class="text-[8px] md:text-[10px] text-slate-400 truncate">各类费用开支</div>
      </div>
    </div>

    <!-- 2. 中间分析层：热销排行 + 账户分布 -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-4 md:gap-6">
      <!-- 热销排行 -->
      <div class="lg:col-span-2 bg-white rounded-2xl md:rounded-3xl p-4 md:p-6 border border-slate-100 shadow-sm">
        <div class="flex items-center justify-between mb-4 md:mb-6">
          <div class="flex items-center gap-2">
            <div class="w-1.5 h-4 md:w-2 md:h-6 bg-blue-600 rounded-full"></div>
            <h3 class="text-base md:text-lg font-black text-slate-800">明星单品排行</h3>
          </div>
        </div>
        
        <div v-if="summary.top_products?.length > 0" class="space-y-4 md:space-y-6">
          <div v-for="(item, index) in summary.top_products" :key="item.name" class="relative">
            <div class="flex justify-between items-end mb-1 md:mb-2">
              <div class="flex items-center gap-2 md:gap-3 min-w-0">
                <div class="w-5 h-5 md:w-6 md:h-6 rounded-lg bg-slate-100 flex items-center justify-center text-[10px] md:text-xs font-bold text-slate-500 shrink-0"
                  :class="{'!bg-yellow-400 !text-white': index === 0, '!bg-slate-300 !text-white': index === 1, '!bg-orange-300 !text-white': index === 2}">
                  {{ index + 1 }}
                </div>
                <span class="font-bold text-slate-700 text-xs md:text-sm truncate">{{ item.name }}</span>
              </div>
              <div class="text-right shrink-0">
                <div class="text-[10px] md:text-xs font-bold text-slate-900">{{ item.quantity }} 件</div>
                <div class="text-[8px] md:text-[10px] text-slate-400 italic">¥{{ item.revenue?.toLocaleString() }}</div>
              </div>
            </div>
            <el-progress 
              :percentage="calculatePercentage(item.quantity)" 
              :stroke-width="windowWidth < 768 ? 6 : 8" 
              :color="getProgressColor(index)" 
              :show-text="false" 
              class="!bg-slate-50 rounded-full overflow-hidden"
            />
          </div>
        </div>
        <el-empty v-else description="暂无销售数据" :image-size="60" />
      </div>

      <!-- 账户资产分布 -->
      <div class="bg-slate-900 rounded-2xl md:rounded-3xl p-4 md:p-6 shadow-2xl relative overflow-hidden">
        <div class="absolute top-0 right-0 w-24 h-24 bg-blue-500/10 rounded-full -mr-12 -mt-12 blur-2xl"></div>
        <div class="relative z-10 flex flex-col h-full">
          <div class="text-blue-400 text-[10px] font-bold uppercase tracking-widest mb-1 md:mb-2">资金账户总览</div>
          <div class="text-xl md:text-3xl font-black text-white mb-4 md:mb-8">
            <span class="text-sm md:text-lg font-bold mr-0.5 md:mr-1">¥</span>{{ totalBalance.toLocaleString() }}
          </div>
          
          <div class="flex-1 space-y-3 md:space-y-4">
            <div v-for="acc in summary.accounts" :key="acc.id" class="flex items-center gap-3 md:gap-4 group">
              <div class="w-8 h-8 md:w-10 md:h-10 rounded-xl bg-white/5 flex items-center justify-center text-base md:text-xl shrink-0">
                {{ getAccountIcon(acc.name) }}
              </div>
              <div class="flex-1 min-w-0 border-b border-white/5 pb-1 md:pb-2">
                <div class="flex justify-between items-center mb-0.5 md:mb-1">
                  <span class="text-[10px] md:text-xs font-medium text-slate-400 truncate">{{ acc.name }}</span>
                  <span class="text-[10px] md:text-sm font-bold text-white shrink-0">¥{{ acc.balance.toLocaleString() }}</span>
                </div>
                <div class="h-0.5 md:h-1 bg-white/5 rounded-full overflow-hidden">
                  <div class="h-full bg-blue-500 transition-all duration-1000" :style="{width: `${(acc.balance / totalBalance * 100) || 0}%`}"></div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 3. 行动层：异常与提醒 -->
    <div class="grid grid-cols-1 xl:grid-cols-2 gap-6">
      <!-- 催收看板 -->
      <div class="space-y-4">
        <div class="flex items-center gap-2">
          <div class="w-1 h-4 bg-red-600 rounded-full"></div>
          <span class="text-lg font-bold text-slate-800">异常单据：待收尾款 ({{ pendingOrders.length }})</span>
        </div>
        <el-card shadow="never" class="!rounded-3xl border-slate-100">
          <el-table :data="pendingOrders" style="width: 100%" v-loading="loading" size="small">
            <el-table-column prop="customer_name" label="客户" width="100">
              <template #default="{ row }">
                <span class="font-bold text-slate-700">{{ row.customer_name || '散客' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="欠款详情">
              <template #default="{ row }">
                <div class="flex items-center gap-2">
                  <span class="text-xs text-slate-400 line-through">¥{{ row.actual_pay_amount.toFixed(0) }}</span>
                  <el-icon class="text-slate-300"><Right /></el-icon>
                  <span class="text-sm font-black text-red-600">¥{{ (row.actual_pay_amount - row.deposit_amount).toFixed(2) }}</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="开单人员" width="100">
              <template #default="{ row }">
                <el-tag size="small" effect="plain">{{ row.employee?.name }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column width="80" align="center">
              <template #default>
                <el-button type="primary" link @click="router.push('/admin/orders')">催收</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </div>

      <!-- 库存告急 -->
      <div class="space-y-4">
        <div class="flex items-center gap-2">
          <div class="w-1 h-4 bg-purple-600 rounded-full"></div>
          <span class="text-lg font-bold text-slate-800">运营提醒：库存告急 ({{ warningProducts.length }})</span>
        </div>
        <el-card shadow="never" class="!rounded-3xl border-slate-100">
          <div v-if="warningProducts.length > 0" class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div v-for="row in warningProducts" :key="row.id" class="p-3 bg-slate-50 rounded-2xl flex justify-between items-center border border-transparent hover:border-purple-200 transition-all">
              <div class="min-w-0">
                <div class="text-sm font-bold text-slate-700 truncate">{{ row.name }}</div>
                <div class="text-[10px] text-slate-400">全仓剩余: <span class="text-red-500 font-bold">{{ row.main_stock + row.store_stock + row.cloud_stock }}</span></div>
              </div>
              <el-button type="primary" plain size="small" circle icon="Plus" @click="router.push('/admin/products')" />
            </div>
          </div>
          <el-empty v-else description="库存充足" :image-size="60" />
        </el-card>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import axios from 'axios'
import { Money, Wallet, Bell, Box, QuestionFilled, Right, Plus } from '@element-plus/icons-vue'

const router = useRouter()
const loading = ref(true)
const period = ref('today')
const windowWidth = ref(window.innerWidth)

const currentTime = computed(() => {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')} ${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`
})

const summary = ref({
  today_revenue: 0,
  total_expense: 0,
  pending_subsidy: 0,
  pending_balance: 0,
  pending_balance_count: 0,
  order_count: 0,
  accounts: [],
  top_products: []
})

const allProducts = ref([])
const pendingOrders = ref([])

const warningProducts = computed(() => {
  return allProducts.value.filter(p => (p.main_stock + p.store_stock + p.cloud_stock) < 5)
})

const totalBalance = computed(() => {
  return summary.value.accounts?.reduce((sum, acc) => sum + acc.balance, 0) || 0
})

const calculatePercentage = (qty) => {
  if (!summary.value.top_products || summary.value.top_products.length === 0) return 0
  const max = summary.value.top_products[0].quantity
  return (qty / max) * 100
}

const getProgressColor = (index) => {
  const colors = ['#facc15', '#94a3b8', '#fdba74', '#60a5fa', '#a78bfa']
  return colors[index] || '#e2e8f0'
}

const getAccountIcon = (name) => {
  if (name.includes('微信')) return '📱'
  if (name.includes('支付宝')) return '💳'
  if (name.includes('对公')) return '🏦'
  return '💰'
}

const formatDate = (date) => {
  if (!date) return '-'
  const d = new Date(date)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

const fetchData = async () => {
  loading.value = true
  try {
    const summaryRes = await axios.get('/api/finance/summary', {
      params: { period: period.value }
    })
    if (summaryRes.data.code === 200) {
      summary.value = summaryRes.data.data
    }

    const productsRes = await axios.get('/api/products', { params: { all: 1 } })
    if (productsRes.data.code === 200) {
      allProducts.value = productsRes.data.data
    }

    const ordersRes = await axios.get('/api/orders')
    if (ordersRes.data.code === 200) {
      pendingOrders.value = ordersRes.data.data.filter(order => 
        order.order_status === 1 && order.payment_status === 1 && order.is_installed === true
      )
    }
  } catch (error) {
    console.error('Failed to fetch dashboard data:', error)
  } finally {
    loading.value = false
  }
}

const handleResize = () => {
  windowWidth.value = window.innerWidth
}

onMounted(() => {
  fetchData()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
})
</script>

<style scoped>
.metric-card {
  backdrop-filter: blur(10px);
}
:deep(.el-card) {
  --el-card-border-color: #f1f5f9;
  --el-card-border-radius: 1.5rem;
}
:deep(.el-table th.el-table__cell) {
  background-color: #f8fafc;
  color: #64748b;
  font-weight: 600;
}
</style>
