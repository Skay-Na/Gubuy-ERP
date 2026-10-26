<script setup>
import { computed, nextTick, ref, shallowRef, useTemplateRef, watch } from 'vue'
import dayjs from 'dayjs'
import { ElMessage } from 'element-plus'
import { Share } from '@element-plus/icons-vue'
import { fetchPayrollHistory } from '../../api/employees'
import PayrollPdfDocument from './PayrollPdfDocument.vue'
import { createPayrollPdf, payrollPdfFilename, sharePayrollPdf } from '../../utils/payrollPdf'

const props = defineProps({
  employees: { type: Array, default: () => [] },
  isMobile: { type: Boolean, default: false }
})
const emit = defineEmits(['view-order'])

const selectedEmployeeId = ref(null)
const year = ref(dayjs().year())
const months = ref([])
const loading = ref(false)
let requestId = 0

const formalEmployees = computed(() => props.employees.filter(row => !row.is_outsourced))
const selectedEmployee = computed(() => formalEmployees.value.find(row => row.id === selectedEmployeeId.value) || null)
const yearOptions = computed(() => {
  const current = dayjs().year()
  return [current + 1, current, current - 1, current - 2]
})

const statusMeta = {
  confirmed: { label: '已发放·定稿', type: 'success' },
  paid: { label: '已发放·台账', type: 'primary' },
  unpaid: { label: '待确认', type: 'warning' },
  upcoming: { label: '未到发薪', type: 'info' }
}

const money = value => `¥${Number(value || 0).toFixed(2)}`
const paidSourceLabel = { confirmation: '确认发放台账', ledger: '工资流水台账', none: '—' }
const orderDate = value => (value ? dayjs(value).format('YYYY-MM-DD') : '-')
const productModels = order => (order.product_models || []).join('、') || '未记录型号'

const fetchHistory = async () => {
  if (!selectedEmployeeId.value) {
    months.value = []
    return
  }
  const current = ++requestId
  loading.value = true
  try {
    const res = await fetchPayrollHistory(selectedEmployeeId.value, year.value)
    if (current === requestId && res.data.code === 200) {
      months.value = res.data.data.months || []
    }
  } catch (error) {
    if (current === requestId) {
      months.value = []
      ElMessage.error(error.response?.data?.msg || '获取工资条失败')
    }
  } finally {
    if (current === requestId) loading.value = false
  }
}

watch([selectedEmployeeId, year], fetchHistory)

// 差额非零高亮：正=少发，负=多发
const differenceClass = row => {
  if (!row.difference) return ''
  return row.difference > 0 ? 'diff-under' : 'diff-over'
}

// ---------- 单月工资条弹窗 + PDF ----------
const slipVisible = ref(false)
const slipMonth = shallowRef(null)
const pdfDocument = useTemplateRef('pdfDocument')
const sharing = shallowRef(false)

const slipEmployee = computed(() => {
  if (!slipMonth.value || !selectedEmployee.value) return null
  return {
    employee_name: selectedEmployee.value.name,
    emp_no: selectedEmployee.value.emp_no,
    base_salary: slipMonth.value.base_salary,
    personal_commission: slipMonth.value.personal_commission,
    chamber_commission: slipMonth.value.chamber_commission,
    deduction: slipMonth.value.deduction,
    final_salary: slipMonth.value.payable,
    leave_days: slipMonth.value.leave_days,
    free_leave_days: slipMonth.value.free_leave_days,
    deduction_days: slipMonth.value.deduction_days,
    daily_salary: slipMonth.value.daily_salary,
    commission_details: slipMonth.value.commission_details || []
  }
})

const openSlip = row => {
  slipMonth.value = row
  slipVisible.value = true
}

const handleShare = async () => {
  if (!slipEmployee.value || sharing.value) return
  sharing.value = true
  try {
    await nextTick()
    await document.fonts?.ready
    const pages = pdfDocument.value?.getPageElements() || []
    const blob = await createPayrollPdf(pages)
    const filename = payrollPdfFilename(slipEmployee.value, slipMonth.value.month)
    const result = await sharePayrollPdf({ blob, filename, employee: slipEmployee.value, month: slipMonth.value.month })
    if (result === 'downloaded') ElMessage.success('当前浏览器不支持分享 PDF，已为你下载文件')
    if (result === 'shared') ElMessage.success('工资条 PDF 已发送到系统分享面板')
  } catch (error) {
    ElMessage.error(error?.message || '生成工资条 PDF 失败')
  } finally {
    sharing.value = false
  }
}

defineExpose({ fetchHistory })
</script>

<template>
  <div class="payroll-history">
    <div class="history-controls">
      <div class="employee-picker">
        <button
          v-for="row in formalEmployees"
          :key="row.id"
          type="button"
          :class="['employee-chip', { active: row.id === selectedEmployeeId, departed: row.employment_status === 'departed' }]"
          @click="selectedEmployeeId = row.id"
        >
          <span class="chip-name">{{ row.name }}</span>
          <small class="chip-meta">{{ row.employment_status === 'departed' ? '已离职' : row.emp_no || '无工号' }}</small>
        </button>
      </div>
      <div class="year-picker">
        <button
          v-for="item in yearOptions"
          :key="item"
          type="button"
          :class="['year-chip', { active: item === year }]"
          @click="year = item"
        >{{ item }} 年</button>
      </div>
      <span v-if="selectedEmployee" class="history-hint">
        应发 = 按订单数据实时回算（小数抹零）；实付 = 确认发放台账 / 工资流水归属；差额一眼看出哪个月发多发少
      </span>
    </div>

    <el-empty v-if="!selectedEmployeeId" description="点选上方员工姓名查看逐月工资条" />
    <el-table
      v-else
      :data="months"
      v-loading="loading"
      border
      stripe
      size="small"
      row-key="month"
      class="history-table"
    >
      <el-table-column type="expand">
        <template #default="{ row }">
          <div class="expand-body">
            <h4>提成订单明细（点击订单号查看当时订单详细情况）</h4>
            <el-table :data="row.commission_details || []" border size="small" max-height="320">
              <el-table-column label="订单号 / 日期" min-width="210">
                <template #default="{ row: order }">
                  <button type="button" class="order-link" @click="emit('view-order', order)">{{ order.order_no }}</button>
                  <small class="order-meta">{{ orderDate(order.order_date) }} · 客户：{{ order.customer_name || '散客' }}</small>
                  <small class="order-meta">型号：{{ productModels(order) }}</small>
                </template>
              </el-table-column>
              <el-table-column label="来源" width="100" align="center">
                <template #default="{ row: order }">
                  <el-tag :type="order.order_type === 2 ? 'warning' : 'info'" size="small">
                    {{ order.order_type === 2 ? '商会平分' : '个人订单' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="订单金额" min-width="100" align="right">
                <template #default="{ row: order }">{{ money(order.order_amount) }}</template>
              </el-table-column>
              <el-table-column label="本人提成" min-width="100" align="right">
                <template #default="{ row: order }"><strong>{{ money(order.commission_amount) }}</strong></template>
              </el-table-column>
              <template #empty>本月没有计入提成的订单</template>
            </el-table>

            <h4>实付构成（{{ paidSourceLabel[row.paid_source] || '—' }}）</h4>
            <el-table :data="row.paid_entries || []" border size="small" max-height="220">
              <el-table-column label="支付日期" width="120">
                <template #default="{ row: entry }">{{ orderDate(entry.paid_at) }}</template>
              </el-table-column>
              <el-table-column label="单号" min-width="150">
                <template #default="{ row: entry }">{{ entry.related_no || '—' }}</template>
              </el-table-column>
              <el-table-column label="备注" min-width="320">
                <template #default="{ row: entry }">{{ entry.remark || '—' }}</template>
              </el-table-column>
              <el-table-column label="金额" width="110" align="right">
                <template #default="{ row: entry }">{{ money(entry.amount) }}</template>
              </el-table-column>
              <template #empty>没有归属到该薪资月的工资流水</template>
            </el-table>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="month" label="薪资月" width="88" />
      <el-table-column v-if="!isMobile" label="底薪" width="90" align="right">
        <template #default="{ row }">{{ money(row.base_salary) }}</template>
      </el-table-column>
      <el-table-column v-if="!isMobile" label="个人提成" width="100" align="right">
        <template #default="{ row }">{{ money(row.personal_commission) }}</template>
      </el-table-column>
      <el-table-column v-if="!isMobile" label="商会分成" width="95" align="right">
        <template #default="{ row }">{{ money(row.chamber_commission) }}</template>
      </el-table-column>
      <el-table-column v-if="!isMobile" label="扣款" width="90" align="right">
        <template #default="{ row }">{{ row.deduction ? `-${money(row.deduction)}` : '—' }}</template>
      </el-table-column>
      <el-table-column label="应发" :width="isMobile ? 90 : 110" align="right">
        <template #default="{ row }"><strong>{{ money(row.payable) }}</strong></template>
      </el-table-column>
      <el-table-column label="实付" :width="isMobile ? 90 : 110" align="right">
        <template #default="{ row }">{{ money(row.paid) }}</template>
      </el-table-column>
      <el-table-column label="差额" :width="isMobile ? 96 : 110" align="right">
        <template #default="{ row }">
          <span v-if="row.status === 'upcoming'" class="difference-none">未到发薪</span>
          <span v-else-if="row.difference" :class="['difference', differenceClass(row)]">
            {{ row.difference > 0 ? '少发 ' : '多发 ' }}{{ money(Math.abs(row.difference)) }}
          </span>
          <span v-else class="difference-ok">对平</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" :width="isMobile ? 92 : 120" align="center">
        <template #default="{ row }">
          <el-tag :type="statusMeta[row.status]?.type || 'info'" size="small">
            {{ statusMeta[row.status]?.label || row.status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" :width="isMobile ? 76 : 110" align="center">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click="openSlip(row)">工资条</el-button>
        </template>
      </el-table-column>
      <template #empty>该年份没有工资记录</template>
    </el-table>

    <el-dialog v-model="slipVisible" width="min(960px, 96vw)" append-to-body destroy-on-close>
      <template #header>
        <div class="slip-header">
          <strong>{{ selectedEmployee?.name || '员工' }} · {{ slipMonth?.month }} 工资条</strong>
          <el-button type="primary" :icon="Share" :loading="sharing" @click="handleShare">
            {{ sharing ? '正在生成 PDF' : '分享 PDF' }}
          </el-button>
        </div>
      </template>
      <template v-if="slipMonth && slipEmployee">
        <div class="salary-breakdown">
          <div><span>基本底薪</span><strong>{{ money(slipMonth.base_salary) }}</strong></div>
          <div><span>个人订单提成</span><strong>{{ money(slipMonth.personal_commission) }}</strong></div>
          <div><span>商会订单分摊</span><strong>{{ money(slipMonth.chamber_commission) }}</strong></div>
          <div><span>超假扣款</span><strong class="deduction">-{{ money(slipMonth.deduction) }}</strong></div>
          <div class="salary-total"><span>应发</span><strong>{{ money(slipMonth.payable) }}</strong></div>
          <div class="salary-total"><span>实付</span><strong>{{ money(slipMonth.paid) }}</strong></div>
        </div>
        <p class="slip-note">
          请假 {{ slipMonth.leave_days }} 天（免费 {{ slipMonth.free_leave_days }} 天），扣款 {{ slipMonth.deduction_days }} 天 × {{ money(slipMonth.daily_salary) }}/天。
          {{ slipMonth.finalized ? '本月工资条为确认发放时的定稿数据。' : '本月尚未确认发放，金额按当前订单数据实时回算。' }}
        </p>
        <h3 class="detail-heading">提成订单明细</h3>
        <el-table :data="slipMonth.commission_details || []" border stripe size="small" max-height="360">
          <el-table-column label="订单号 / 日期" min-width="220">
            <template #default="{ row: order }">
              <button type="button" class="order-link" @click="emit('view-order', order)">{{ order.order_no }}</button>
              <small class="order-meta">{{ orderDate(order.order_date) }} · 客户：{{ order.customer_name || '散客' }}</small>
              <small class="order-meta">型号：{{ productModels(order) }}</small>
            </template>
          </el-table-column>
          <el-table-column label="来源" width="100" align="center">
            <template #default="{ row: order }">
              <el-tag :type="order.order_type === 2 ? 'warning' : 'info'" size="small">
                {{ order.order_type === 2 ? '商会平分' : '个人订单' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="订单金额" min-width="100" align="right">
            <template #default="{ row: order }">{{ money(order.order_amount) }}</template>
          </el-table-column>
          <el-table-column label="本人提成" min-width="100" align="right">
            <template #default="{ row: order }"><strong>{{ money(order.commission_amount) }}</strong></template>
          </el-table-column>
          <template #empty>本月没有计入提成的订单</template>
        </el-table>
        <PayrollPdfDocument ref="pdfDocument" :employee="slipEmployee" :month="slipMonth.month" />
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.history-controls { display: flex; flex-direction: column; gap: 10px; margin-bottom: 14px; }
.employee-picker { display: flex; flex-wrap: wrap; gap: 8px; }
.employee-chip {
  display: inline-flex; flex-direction: column; align-items: flex-start; gap: 1px;
  padding: 7px 14px; border: 1px solid #dbe3ee; border-radius: 10px; background: #fff;
  cursor: pointer; transition: all .15s ease; text-align: left;
}
.employee-chip .chip-name { font-weight: 700; font-size: 14px; color: #334155; }
.employee-chip .chip-meta { font-size: 11px; color: #8490a4; }
.employee-chip:hover { border-color: #93b4f8; background: #f5f8ff; }
.employee-chip.active { border-color: #2563eb; background: #eff6ff; }
.employee-chip.active .chip-name { color: #1d4ed8; }
.employee-chip.departed .chip-name { color: #94a3b8; }
.year-picker { display: flex; flex-wrap: wrap; gap: 8px; }
.year-chip {
  padding: 5px 14px; border: 1px solid #dbe3ee; border-radius: 999px; background: #fff;
  font-size: 13px; color: #475569; cursor: pointer; transition: all .15s ease;
}
.year-chip:hover { border-color: #93b4f8; background: #f5f8ff; }
.year-chip.active { border-color: #2563eb; background: #eff6ff; color: #1d4ed8; font-weight: 700; }
.history-hint { color: #8490a4; font-size: 12px; }
.expand-body { padding: 6px 12px 14px; }
.expand-body h4 { margin: 14px 0 8px; color: #334155; font-size: 13px; }
.expand-body h4:first-child { margin-top: 4px; }
.order-link { padding: 0; border: 0; background: transparent; color: #2563eb; font: inherit; font-weight: 800; cursor: pointer; text-decoration: underline; text-underline-offset: 3px; }
.order-meta { display: block; margin-top: 3px; color: #7b8798; font-size: 11px; line-height: 1.35; }
.difference { font-weight: 700; }
.diff-under { color: #ef4444; }
.diff-over { color: #d97706; }
.difference-ok { color: #16a34a; font-size: 12px; }
.difference-none { color: #94a3b8; font-size: 12px; }
.slip-header { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding-right: 28px; }
.slip-header strong { color: #17243d; font-size: 17px; }
.salary-breakdown { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 10px; }
.salary-breakdown > div { display: flex; flex-direction: column; gap: 6px; padding: 12px; border: 1px solid #e2e8f0; border-radius: 10px; background: #f8fafc; }
.salary-breakdown span { color: #64748b; font-size: 12px; }
.salary-breakdown strong { color: #0f172a; font-size: 17px; }
.salary-breakdown .deduction { color: #ef4444; }
.salary-total { background: #eff6ff; border-color: #bfdbfe; }
.salary-total strong { color: #2563eb; }
.slip-note { margin: 12px 0 16px; color: #64748b; font-size: 12px; }
.detail-heading { margin: 0 0 10px; color: #334155; font-size: 14px; font-weight: 700; }
@media (max-width: 760px) {
  .employee-picker, .year-picker { width: 100%; }
}
</style>
