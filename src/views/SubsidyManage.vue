<template>
  <div class="subsidy-page p-4 sm:p-6 max-w-7xl mx-auto">
    <!-- Header Section -->
    <div class="mb-8">
      <div class="flex justify-between items-end gap-4 mb-6">
        <div>
          <h1 class="text-2xl md:text-3xl font-black text-slate-900 tracking-tight mb-1">国补核销管理</h1>
          <p class="text-sm text-slate-400">追踪国补订单原价应回款、到账进度及批量结算记录</p>
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
              <div class="text-xs text-slate-400 font-bold uppercase tracking-wider mb-1">累计应回款</div>
              <div class="text-2xl font-black text-slate-900">¥{{ formatMoney(summary.total_generated) }}</div>
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
              <div class="text-2xl font-black text-slate-900">¥{{ formatMoney(summary.total_received) }}</div>
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
              <div class="text-2xl font-black text-orange-600">¥{{ formatMoney(summary.pending_balance) }}</div>
            </div>
          </div>
        </el-card>
      </div>

      <!-- 核销进度总览：三段比例条（已回款 / 已提交待回 / 待提交） -->
      <el-card shadow="never" class="!rounded-2xl border-slate-100 mb-8">
        <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
          <h3 class="text-base font-bold text-slate-800 flex items-center gap-2">
            <el-icon><TrendCharts /></el-icon>核销进度
          </h3>
          <span class="text-xs text-slate-400">累计应回款 ¥{{ formatMoney(summary.total_generated) }}</span>
        </div>
        <div class="progress-track" role="img" :aria-label="`已回款 ${progressSegments[0].pct.toFixed(1)}%，已提交待回 ${progressSegments[1].pct.toFixed(1)}%，待提交 ${progressSegments[2].pct.toFixed(1)}%`">
          <div
            v-for="seg in progressSegments"
            :key="seg.label"
            class="progress-seg"
            :style="{ width: seg.pct + '%', background: seg.color }"
            :title="`${seg.label} ¥${formatMoney(seg.amount)}（${seg.count} 笔）`"
          ></div>
        </div>
        <div class="flex flex-wrap gap-x-6 gap-y-1.5 mt-3 text-xs text-slate-600">
          <span v-for="seg in progressSegments" :key="seg.label" class="inline-flex items-center gap-1.5">
            <i class="w-2 h-2 rounded-full shrink-0" :style="{ background: seg.color }"></i>
            <b>{{ seg.label }}</b>
            <span>{{ seg.count }} 笔 · ¥{{ formatMoney(seg.amount) }}</span>
            <span class="text-slate-400">{{ seg.pct.toFixed(1) }}%</span>
          </span>
        </div>
      </el-card>
    </div>

    <!-- 选单核销区 -->
    <el-card shadow="never" class="!rounded-2xl border-blue-100 mb-8">
      <div class="flex flex-wrap items-center justify-between gap-3 mb-4">
        <h3 class="text-lg font-bold text-slate-800 flex items-center gap-2">
          <el-icon><Coin /></el-icon>选单核销
        </h3>
        <div class="allocate-toolbar flex flex-wrap items-center gap-2">
          <span class="text-xs text-slate-500 font-bold whitespace-nowrap">回款池</span>
          <el-select v-model="allocatePoolId" placeholder="选择要核销的回款记录" class="w-full sm:!w-72" size="default">
            <el-option
              v-for="s in pools"
              :key="s.id"
              :label="`回款 #${s.id} ¥${formatMoney(s.amount)}（余 ¥${formatMoney(poolRemainingOf(s))}）`"
              :value="s.id"
              :disabled="poolRemainingOf(s) <= 0.005"
            />
          </el-select>
          <span class="text-xs text-slate-500 font-bold whitespace-nowrap">本笔核销总额</span>
          <el-input-number v-model="allocateTotal" :min="0" :precision="2" :controls="false" class="w-full sm:!w-36" placeholder="金额" />
          <el-button class="!rounded-xl" :disabled="selectedOrders.length === 0" @click="autoAllocate">按待回额自动分配</el-button>
          <el-button type="primary" class="!rounded-xl" :loading="submittingAllocate" :disabled="!canSubmitAllocate" @click="submitAllocation">提交核销</el-button>
        </div>
      </div>
      <el-table
        ref="allocateTableRef"
        :data="pendingOrders"
        v-loading="loadingOrders"
        style="width: 100%"
        @selection-change="onAllocateSelectionChange"
      >
        <el-table-column type="selection" width="45" />
        <el-table-column prop="order_no" label="单号" min-width="150" />
        <el-table-column prop="customer_name" label="客户姓名" min-width="110" />
        <el-table-column label="录单日期" width="120">
          <template #default="{ row }">
            <span class="text-slate-600">{{ formatDate(row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="待回金额" width="130">
          <template #default="{ row }">
            <span class="font-black text-orange-600">¥{{ formatMoney(remainingOf(row)) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="本笔核销金额" width="170">
          <template #default="{ row }">
            <el-input-number
              v-model="allocAmounts[row.id]"
              :min="0"
              :max="remainingOf(row)"
              :precision="2"
              :controls="false"
              size="small"
              class="!w-full"
              placeholder="0.00"
            />
          </template>
        </el-table-column>
      </el-table>
      <div v-if="pendingTotal > 0" class="flex justify-center pt-3">
        <el-pagination
          v-model:current-page="pendingPage"
          v-model:page-size="pendingPageSize"
          :total="pendingTotal"
          :page-sizes="[15, 30, 50]"
          layout="total, sizes, prev, pager, next"
          background
          small
          @current-change="handlePendingPageChange"
          @size-change="handlePendingSizeChange"
        />
      </div>
      <div class="text-[11px] text-slate-400 mt-3 leading-relaxed">
        <div>提示：客户 85% 货款开单时已入账，此处只录政府打款中的<b class="text-slate-500">补贴部分</b>（各单待回金额合计）；若录入政府打款全额，多余部分会留在回款池无法核销。</div>
        <div class="mt-1">* 勾选订单后输入本笔核销总额可自动按录单顺序逐单匹配（可手调）；收满的订单自动标记为"已回款"。核销只做回款池到订单的分配，不重复记账。</div>
      </div>
    </el-card>

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
                <span class="text-slate-600">{{ formatDateTime(row.created_at) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="回款金额" min-width="150">
              <template #default="{ row }">
                <span class="font-black text-slate-900">¥{{ formatMoney(row.amount) }}</span>
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
                <span class="text-xs text-slate-400">¥{{ formatMoney(item.amount) }}</span>
              </div>
              <el-progress 
                :percentage="getProgressPercentage(item.amount)" 
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

    <!-- 已核销记录 -->
    <el-card shadow="never" class="!rounded-2xl border-slate-100 overflow-hidden mt-8" body-class="!p-0">
      <div class="flex items-center justify-between p-5 pb-3">
        <h3 class="text-lg font-bold text-slate-800 flex items-center gap-2">
          <el-icon><CircleCheck /></el-icon>已核销记录（按单）
        </h3>
      </div>
      <el-table :data="receipts" v-loading="loadingReceipts" style="width: 100%">
        <el-table-column label="核销时间" width="180">
          <template #default="{ row }">
            <span class="text-slate-600">{{ formatDateTime(row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="order_no" label="单号" min-width="150" />
        <el-table-column prop="customer_name" label="客户姓名" min-width="110" />
        <el-table-column label="核销金额" min-width="130">
          <template #default="{ row }">
            <span class="font-black text-green-600">¥{{ formatMoney(row.allocated_amt || 0) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="来源回款池" width="120">
          <template #default="{ row }">
            <el-tag size="small" effect="plain" class="!rounded-lg">回款 #{{ row.settlement_id }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_by" label="操作人" width="120" />
      </el-table>
    </el-card>

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
import { Plus, PieChart, Checked, Timer, List, DocumentChecked, Coin, CircleCheck, TrendCharts } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { fetchSubsidySummary, fetchSubsidySettlements, fetchSubsidyReceipts, allocateSubsidy, settleSubsidy } from '../api/subsidy'
import { fetchAccounts as fetchAccountsApi } from '../api/finance'
import { fetchOrders as fetchOrdersApi } from '../api/orders'
import { formatMoney, formatDate, formatDateTime } from '../utils/format'

const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const summary = ref({})
const settlements = ref([])
const accounts = ref([])
const orders = ref([]) // 用于计算资料进度

// 选单核销状态
const loadingOrders = ref(false)
const loadingReceipts = ref(false)
const submittingAllocate = ref(false)
const pendingOrders = ref([])   // status=2 且仍有待回款的订单（录单日期升序，服务端分页）
const selectedOrders = ref([])  // 表格勾选行
const allocatePoolId = ref(null)
const allocateTotal = ref(0)
const allocAmounts = ref({})    // order_id -> 本笔核销金额
const receipts = ref([])        // 已核销记录
// 待核销订单分页
const pendingPage = ref(1)
const pendingPageSize = ref(15)
const pendingTotal = ref(0)

const form = ref({
  amount: 0,
  account_id: null,
  remark: ''
})

const getProgressPercentage = (amount) => {
  const total = summary.value.total_generated || 0
  if (total <= 0) return 0
  return Math.min(100, (amount / total) * 100)
}

const fetchSummary = async () => {
  const res = await fetchSubsidySummary()
  if (res.data.code === 200) {
    summary.value = res.data.data
  }
}

const fetchSettlements = async () => {
  loading.value = true
  const res = await fetchSubsidySettlements()
  if (res.data.code === 200) {
    settlements.value = res.data.data
  }
  loading.value = false
}

const fetchAccounts = async () => {
  const res = await fetchAccountsApi()
  if (res.data.code === 200) {
    accounts.value = res.data.data
  }
}

const fetchOrders = async () => {
  // 获取所有带国补的订单来计算进度（light 模式只预载员工，减少订单项/商品/账户等关联查询）
  const res = await fetchOrdersApi({ fields: 'light' })
  if (res.data.code === 200) {
    orders.value = (res.data.data || []).filter(o => o.subsidy_amount > 0 && o.order_status === 1)
  }
}

// ---------- 选单核销 ----------


// 订单剩余待回金额 = subsidy_pending_amount - subsidy_received_amt（统一口径）
const remainingOf = (order) => {
  const remaining = Number(order.subsidy_pending_amount || 0) - Number(order.subsidy_received_amt || 0)
  return Math.max(0, Math.round(remaining * 100) / 100)
}

// 回款池列表：附剩余可核销额度
const pools = computed(() => settlements.value)

const poolRemainingOf = (settlement) => {
  const remaining = Number(settlement.amount || 0) - Number(settlement.allocated_amt || 0)
  return Math.max(0, Math.round(remaining * 100) / 100)
}

const selectedPool = computed(() =>
  pools.value.find(s => s.id === allocatePoolId.value) || null
)

const canSubmitAllocate = computed(() => {
  if (!allocatePoolId.value || selectedOrders.value.length === 0) return false
  return allocationItems.value.length > 0
})

const allocationItems = computed(() =>
  selectedOrders.value
    .map(order => ({
      order_id: order.id,
      amount: Math.round(Number(allocAmounts.value[order.id] || 0) * 100) / 100
    }))
    .filter(item => item.amount > 0)
)

const fetchPendingOrders = async () => {
  loadingOrders.value = true
  try {
    const res = await fetchOrdersApi({
      subsidy_status: 2,
      page: pendingPage.value,
      page_size: pendingPageSize.value
    })
    if (res.data.code === 200) {
      pendingOrders.value = (res.data.data || [])
        .filter(o => o.order_status === 1 && remainingOf(o) > 0.005)
        .sort((a, b) => new Date(a.created_at) - new Date(b.created_at))
      pendingTotal.value = Number(res.data.total) || pendingOrders.value.length
    }
  } finally {
    loadingOrders.value = false
  }
}

const handlePendingPageChange = () => {
  // 翻页清空勾选与已填金额，避免跨页误核销
  selectedOrders.value = []
  allocAmounts.value = {}
  fetchPendingOrders()
}

const handlePendingSizeChange = () => {
  pendingPage.value = 1
  handlePendingPageChange()
}

const fetchReceipts = async () => {
  loadingReceipts.value = true
  try {
    const res = await fetchSubsidyReceipts()
    if (res.data.code === 200) {
      receipts.value = res.data.data || []
    }
  } finally {
    loadingReceipts.value = false
  }
}

const onAllocateSelectionChange = (rows) => {
  selectedOrders.value = rows
}

// 按录单顺序逐单吃满：输入本笔核销总额后自动分配（可手调）
const autoAllocate = () => {
  const pool = selectedPool.value
  if (!pool) return ElMessage.warning('请先选择回款池')
  if (!allocateTotal.value || allocateTotal.value <= 0) return ElMessage.warning('请输入本笔核销总额')
  const poolRemaining = poolRemainingOf(pool)
  if (allocateTotal.value > poolRemaining + 0.005) {
    return ElMessage.warning(`回款池仅剩可核销 ¥${formatMoney(poolRemaining)}`)
  }
  let left = Math.min(allocateTotal.value, poolRemaining)
  const amounts = {}
  for (const order of selectedOrders.value) {
    if (left <= 0.005) break
    const alloc = Math.min(remainingOf(order), left)
    amounts[order.id] = Number(alloc.toFixed(2))
    left = Math.round((left - alloc) * 100) / 100
  }
  allocAmounts.value = amounts
  if (left > 0.005) {
    ElMessage.info(`已按选中订单待回额分配 ¥${formatMoney(Number(allocateTotal.value) - left)}，剩余金额超出选中订单待回总额`)
  }
}

const submitAllocation = async () => {
  const pool = selectedPool.value
  if (!pool) return ElMessage.warning('请先选择回款池')
  const items = allocationItems.value
  if (!items.length) return ElMessage.warning('请至少为一笔订单填写核销金额')
  const total = items.reduce((s, i) => s + i.amount, 0)
  if (total > poolRemainingOf(pool) + 0.005) {
    return ElMessage.warning(`核销总额超出回款池剩余 ¥${formatMoney(poolRemainingOf(pool))}`)
  }
  try {
    // 软提醒：录入金额明显大于选中订单待回补贴合计（如政府打款全额）时再次确认
    const selectedPendingSum = selectedOrders.value.reduce((sum, order) => sum + remainingOf(order), 0)
    if (selectedPendingSum > 0 && allocateTotal.value > selectedPendingSum * 2) {
      await ElMessageBox.confirm(
        `录入金额 ¥${formatMoney(allocateTotal.value)} 明显大于选中订单待回补贴合计 ¥${formatMoney(selectedPendingSum)}，多余部分会留在回款池。确认录入的是补贴部分、无误？`,
        '金额核对',
        { confirmButtonText: '确认无误', cancelButtonText: '再检查一下', type: 'warning' }
      )
    }
    await ElMessageBox.confirm(
      `确认从回款 #${pool.id} 核销 ¥${formatMoney(total)} 到 ${items.length} 笔订单吗？`,
      '确认核销',
      { confirmButtonText: '确认核销', cancelButtonText: '取消', type: 'warning' }
    )
    submittingAllocate.value = true
    const res = await allocateSubsidy({
      settlement_id: pool.id,
      allocations: items
    })
    if (res.data.code === 200) {
      ElMessage.success(res.data.msg || '核销成功')
      allocAmounts.value = {}
      selectedOrders.value = []
      allocateTotal.value = 0
      fetchSummary()
      fetchSettlements()
      fetchPendingOrders()
      fetchReceipts()
      fetchOrders()
    } else {
      ElMessage.error(res.data.msg || '核销失败')
    }
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(error.response?.data?.msg || '核销失败，请稍后重试')
    }
  } finally {
    submittingAllocate.value = false
  }
}

const docProgress = computed(() => {
  const pending = orders.value.filter(o => o.subsidy_status === 1)
  const submitted = orders.value.filter(o => o.subsidy_status === 2)
  const finished = orders.value.filter(o => o.subsidy_status === 3)

  // 金额口径与"累计应回款"一致：使用国补待回款口径（subsidy_pending_amount），
  // 不再使用订单 total_amount，避免把非国补部分计入资料提报进度。
  return [
    {
      label: '资料待提交',
      amount: pending.reduce((s, o) => s + Number(o.subsidy_pending_amount || 0), 0),
      count: pending.length,
      color: '#94a3b8'
    },
    {
      label: '资料已提交政府',
      amount: (submitted.reduce((s, o) => s + Number(o.subsidy_pending_amount || 0), 0) + finished.reduce((s, o) => s + Number(o.subsidy_pending_amount || 0), 0)),
      count: (submitted.length + finished.length),
      color: '#3b82f6'
    }
  ]
})

// ---------- 核销进度总览（三段比例条） ----------
// 已回款：以回款池实际录入为准（summary.total_received，权威口径）；
// 在途/待提交：按订单剩余待回额（subsidy_pending_amount - subsidy_received_amt）
const progressSegments = computed(() => {
  const finished = orders.value.filter(o => o.subsidy_status === 3)
  const submitted = orders.value.filter(o => o.subsidy_status === 2)
  const pending = orders.value.filter(o => o.subsidy_status === 1)
  const segments = [
    { label: '已回款', amount: Number(summary.value.total_received || 0), count: finished.length, color: '#10b981' },
    { label: '已提交待回', amount: submitted.reduce((s, o) => s + remainingOf(o), 0), count: submitted.length, color: '#3b82f6' },
    { label: '待提交', amount: pending.reduce((s, o) => s + remainingOf(o), 0), count: pending.length, color: '#f59e0b' }
  ]
  const total = segments.reduce((sum, seg) => sum + seg.amount, 0)
  for (const seg of segments) {
    seg.pct = total > 0 ? (seg.amount / total) * 100 : 0
  }
  return segments
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
    const res = await settleSubsidy(form.value)
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
  fetchPendingOrders()
  fetchReceipts()
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

/* 核销进度三段比例条 */
.progress-track {
  display: flex;
  height: 12px;
  border-radius: 999px;
  overflow: hidden;
  background: #f1f5f9;
}
.progress-seg {
  min-width: 2px;
  transition: width .4s ease;
}

/* 窄屏（<640px）移动端适配：选单核销区不溢出，提示文案正常折行 */
@media (max-width: 640px) {
  .subsidy-page {
    overflow-x: clip;
    max-width: 100vw;
    box-sizing: border-box;
  }
  .subsidy-page :deep(.el-card__body) {
    padding: 14px;
  }
  /* 回款池/总额/按钮纵向排列，避免横向挤压溢出 */
  .allocate-toolbar {
    display: grid;
    grid-template-columns: 1fr;
    gap: 8px;
  }
  .allocate-toolbar :deep(.el-select),
  .allocate-toolbar :deep(.el-input-number) {
    width: 100%;
  }
  .allocate-toolbar :deep(.el-button) {
    width: 100%;
    margin-left: 0;
  }
  /* 明细文案与提示自然折行 */
  .subsidy-page :deep(.el-table) {
    min-width: 0;
  }
}
</style>
