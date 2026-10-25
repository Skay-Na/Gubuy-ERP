<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { fetchPayrollConfirmations, confirmPayroll } from '../../api/employees'

const props = defineProps({
  month: { type: String, required: true },
  rows: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  leaveSaving: { type: Boolean, default: false },
  isMobile: { type: Boolean, default: false }
})
const emit = defineEmits(['month-change', 'refresh', 'leave-change', 'open-detail', 'view-order'])

const query = ref('')
const selectedId = ref(null)
const leaveDays = ref(0)
const money = value => `¥${Number(value || 0).toFixed(2)}`
const productModels = order => (order.product_models || []).join('、') || '未记录商品型号'
const employeeRows = computed(() => props.rows.filter(row => {
  const keyword = query.value.trim().toLowerCase()
  return !keyword || `${row.employee_name} ${row.emp_no}`.toLowerCase().includes(keyword)
}))
const selected = computed(() => employeeRows.value.find(row => row.employee_id === selectedId.value) || employeeRows.value[0] || null)
const summary = computed(() => props.rows.reduce((total, row) => {
  total.salary += Math.round(Number(row.final_salary || 0) * 100)
  total.personalCommission += Math.round(Number(row.personal_commission || 0) * 100)
  total.chamberCommission += Math.round(Number(row.chamber_commission || 0) * 100)
  total.deduction += Math.round(Number(row.deduction || 0) * 100)
  return total
}, { salary: 0, personalCommission: 0, chamberCommission: 0, deduction: 0 }))

// ---------- 工资确认发放 ----------
const confirmDialogVisible = ref(false)
const confirming = ref(false)
const confirmations = ref([])
const confirmationsLoading = ref(false)
const actualPay = ref({}) // employee_id -> 实付额（默认本月实发，可改）

const confirmedByEmployee = computed(() => {
  const map = {}
  for (const record of confirmations.value) map[record.employee_id] = record
  return map
})
const unconfirmedRows = computed(() =>
  props.rows.filter(row => !confirmedByEmployee.value[row.employee_id])
)
const confirmTotal = computed(() =>
  unconfirmedRows.value.reduce((total, row) => total + Number(actualPay.value[row.employee_id] ?? row.final_salary ?? 0), 0)
)

const fetchConfirmations = async () => {
  confirmationsLoading.value = true
  try {
    const res = await fetchPayrollConfirmations({ month: props.month })
    if (res.data.code === 200) confirmations.value = res.data.data || []
  } catch {
    confirmations.value = []
  } finally {
    confirmationsLoading.value = false
  }
}

const openConfirmDialog = () => {
  if (!props.rows.length) return ElMessage.warning('本月没有在职员工薪资数据')
  const amounts = {}
  for (const row of props.rows) {
    amounts[row.employee_id] = Number(actualPay.value[row.employee_id] ?? row.final_salary ?? 0)
  }
  actualPay.value = amounts
  confirmDialogVisible.value = true
}

const submitConfirm = async () => {
  const items = unconfirmedRows.value
    .map(row => ({
      employee_id: row.employee_id,
      amount: Math.round(Number(actualPay.value[row.employee_id] ?? row.final_salary ?? 0) * 100) / 100
    }))
    .filter(item => item.amount > 0)
  if (!items.length) return ElMessage.warning('没有待确认的员工')
  try {
    await ElMessageBox.confirm(
      `确认 ${props.month} 月已向 ${items.length} 名员工发放工资，合计 ${money(confirmTotal.value)}？确认后计入经营开销账，不可在本页重复确认。`,
      '确认工资发放',
      { confirmButtonText: '确认发放', cancelButtonText: '取消', type: 'warning' }
    )
    confirming.value = true
    const res = await confirmPayroll({ month: props.month, items })
    if (res.data.code === 200) {
      ElMessage.success(res.data.msg || '工资确认成功')
      confirmDialogVisible.value = false
      await fetchConfirmations()
      emit('refresh')
    } else {
      ElMessage.error(res.data.msg || '工资确认失败')
    }
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(error.response?.data?.msg || '工资确认失败，请稍后重试')
    }
  } finally {
    confirming.value = false
  }
}

watch(() => props.month, () => { fetchConfirmations() }, { immediate: true })

watch(selected, row => { leaveDays.value = row?.manual_leave_days ?? 0 }, { immediate: true })
const saveLeave = days => {
  if (!selected.value) return
  const next = Math.max(0, Math.min(31, Number(days) || 0))
  leaveDays.value = next
  if (next !== selected.value.manual_leave_days) emit('leave-change', selected.value, next)
}
</script>

<template>
  <div class="payroll-workspace" v-loading="loading">
    <header class="workspace-header">
      <div>
        <p class="eyebrow">PAYROLL / 薪资结算</p>
        <h2>{{ month }} 月薪资</h2>
        <p class="muted">选择员工核对提成和考勤，调整请假天数后立即重新计算。</p>
      </div>
      <div class="month-actions">
        <input v-if="isMobile" type="month" :value="month" aria-label="核算月份" @change="emit('month-change', $event.target.value)" />
        <el-date-picker v-else :model-value="month" type="month" format="YYYY-MM" value-format="YYYY-MM" aria-label="核算月份" @update:model-value="emit('month-change', $event)" />
        <el-button @click="emit('refresh')">刷新</el-button>
        <el-button type="primary" :disabled="unconfirmedRows.length === 0" @click="openConfirmDialog">确认本月已发放</el-button>
      </div>
    </header>

    <div v-if="confirmations.length" class="confirm-strip">
      <span class="confirm-strip-label">本月已确认发放 {{ confirmations.length }} 人 · 合计 {{ money(confirmations.reduce((s, r) => s + Number(r.amount || 0), 0)) }}</span>
    </div>

    <div class="payroll-metrics">
      <div class="metric main"><span>本月应发</span><strong>{{ money(summary.salary / 100) }}</strong><small>{{ rows.length }} 位在职员工</small></div>
      <div class="metric"><span>个人提成合计</span><strong>{{ money(summary.personalCommission / 100) }}</strong><small>员工个人订单提成</small></div>
      <div class="metric"><span>商会提成合计</span><strong>{{ money(summary.chamberCommission / 100) }}</strong><small>商会订单员工平分</small></div>
      <div class="metric"><span>超假扣款</span><strong>{{ money(summary.deduction / 100) }}</strong><small>每月 4 天以内不扣款</small></div>
    </div>

    <div class="workspace-body">
      <section class="roster" aria-label="薪资员工列表">
        <div class="section-top"><div><h3>员工</h3><span>{{ employeeRows.length }} 人</span></div></div>
        <el-input v-model="query" clearable placeholder="搜索姓名或工号" class="roster-search" />
        <div class="roster-list">
          <button v-for="row in employeeRows" :key="row.employee_id" type="button" class="roster-item" :class="{ selected: selected?.employee_id === row.employee_id }" @click="selectedId = row.employee_id">
            <span class="avatar">{{ row.employee_name?.slice(0, 1) }}</span>
            <span class="roster-identity"><strong>{{ row.employee_name }}</strong><small>#{{ row.emp_no }} · 提成 {{ money(row.total_commission) }}</small></span>
            <span class="roster-pay">{{ money(row.final_salary) }}</span>
          </button>
          <el-empty v-if="!loading && !employeeRows.length" :description="query ? '没有匹配的员工' : '本月暂无在职员工'" />
        </div>
      </section>

      <section class="employee-pay" aria-label="员工薪资核对">
        <template v-if="selected">
          <div class="employee-pay-header">
            <div><p class="eyebrow">员工核对</p><h3>{{ selected.employee_name }} <small>#{{ selected.emp_no }}</small></h3></div>
            <el-button type="primary" plain @click="emit('open-detail', selected)">查看订单明细</el-button>
          </div>
          <div class="take-home"><span>本月实发</span><strong>{{ money(selected.final_salary) }}</strong></div>
          <div class="breakdown-row"><span>基本底薪</span><strong>{{ money(selected.base_salary) }}</strong></div>
          <div class="breakdown-row"><span>个人订单提成</span><strong>+ {{ money(selected.personal_commission) }}</strong></div>
          <div class="breakdown-row"><span>商会订单平分</span><strong>+ {{ money(selected.chamber_commission) }}</strong></div>
          <div class="breakdown-row deduction"><span>超假扣款</span><strong>- {{ money(selected.deduction) }}</strong></div>

          <div class="attendance-box">
            <div class="attendance-title"><h4>请假与考勤</h4><span>打卡估算 {{ selected.absent_days || 0 }} 天</span></div>
            <p>每月免费请假 4 天；超过部分按 {{ money(selected.daily_salary) }}/天扣款。</p>
            <div class="leave-control">
              <div class="leave-stepper">
                <el-button aria-label="请假天数减一" :disabled="leaveSaving" @click="saveLeave(leaveDays - 1)">−</el-button>
                <el-input-number v-model="leaveDays" :min="0" :max="31" :controls="false" :disabled="leaveSaving" aria-label="请假天数" @change="saveLeave" />
                <span>天</span>
                <el-button aria-label="请假天数加一" :disabled="leaveSaving" @click="saveLeave(leaveDays + 1)">+</el-button>
              </div>
              <el-button text type="primary" :disabled="leaveSaving" @click="saveLeave(selected.absent_days || 0)">按打卡估算填写</el-button>
            </div>
            <span class="leave-result">当前扣款 {{ selected.deduction_days || 0 }} 天 · {{ money(selected.deduction) }}</span>
          </div>
          <div class="orders-preview">
            <div class="attendance-title"><h4>提成订单</h4><span>{{ selected.commission_details?.length || 0 }} 笔</span></div>
            <div v-for="order in (selected.commission_details || []).slice(0, 4)" :key="order.order_id" class="order-line">
              <span>
                <button type="button" class="order-link" @click="emit('view-order', order)">{{ order.order_no }}</button>
                <small>{{ order.order_type === 2 ? '商会平分' : '个人订单' }} · {{ order.customer_name || '散客' }}</small>
                <small>型号：{{ productModels(order) }}</small>
              </span>
              <b>+ {{ money(order.commission_amount) }}</b>
            </div>
            <el-button v-if="selected.commission_details?.length > 4" text type="primary" @click="emit('open-detail', selected)">查看全部订单</el-button>
            <p v-if="!selected.commission_details?.length" class="muted">本月没有计入提成的订单</p>
          </div>
        </template>
        <el-empty v-else :description="query ? '清除搜索以选择员工' : '选择月份后查看员工薪资'" />
      </section>
    </div>

    <!-- 确认发放记录台账 -->
    <section class="confirm-ledger" aria-label="工资确认记录">
      <div class="section-top"><h3>工资确认记录（{{ month }}）</h3><span>计入经营开销账</span></div>
      <el-table :data="confirmations" v-loading="confirmationsLoading" size="small" style="width: 100%; margin-top: 12px" empty-text="本月还没有确认过发放">
        <el-table-column prop="employee_name" label="姓名" width="110" />
        <el-table-column prop="emp_no" label="工号" width="90" />
        <el-table-column prop="employee_role" label="岗位" width="110" />
        <el-table-column label="实发工资" width="130">
          <template #default="{ row }"><strong>{{ money(row.amount) }}</strong></template>
        </el-table-column>
        <el-table-column prop="created_by" label="操作人" width="120" />
        <el-table-column label="确认时间">
          <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
        </el-table-column>
      </el-table>
    </section>

    <!-- 确认发放对话框 -->
    <el-dialog v-model="confirmDialogVisible" :title="`确认 ${month} 月工资已发放`" width="640px" append-to-body>
      <el-alert
        :title="`确认后按实付额计入经营开销账（不动资金账户余额）；已确认员工不可重复确认。本笔合计 ${money(confirmTotal)}`"
        type="info"
        :closable="false"
        show-icon
        class="confirm-alert"
      />
      <el-table :data="props.rows" size="small" max-height="420" style="width: 100%">
        <el-table-column prop="employee_name" label="姓名" width="100" />
        <el-table-column prop="emp_no" label="工号" width="90" />
        <el-table-column label="本月实发" width="120">
          <template #default="{ row }">{{ money(row.final_salary) }}</template>
        </el-table-column>
        <el-table-column label="实付额（可改）">
          <template #default="{ row }">
            <div class="confirm-pay-cell">
              <el-input-number
                v-model="actualPay[row.employee_id]"
                :min="0"
                :precision="2"
                :controls="false"
                size="small"
                :disabled="!!confirmedByEmployee[row.employee_id]"
              />
              <el-tag v-if="confirmedByEmployee[row.employee_id]" size="small" type="success" effect="plain">已确认</el-tag>
            </div>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="confirmDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="confirming" :disabled="unconfirmedRows.length === 0" @click="submitConfirm">确认发放</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.confirm-strip{display:flex;justify-content:flex-end;margin:-6px 0 14px}.confirm-strip-label{color:#2563eb;font-size:12px;font-weight:700}.confirm-ledger{margin-top:16px;background:#fff;border:1px solid #e4eaf2;border-radius:16px;padding:20px}.confirm-ledger h3{display:inline;margin:0;font-size:18px}.confirm-ledger span{margin-left:8px;color:#8490a4;font-size:12px}.confirm-alert{margin-bottom:14px}.confirm-pay-cell{display:flex;align-items:center;gap:8px}.confirm-pay-cell .el-input-number{width:140px}
.payroll-workspace{min-height:100%;padding:24px;background:#f5f7fb;color:#17243d}.workspace-header,.section-top,.employee-pay-header,.attendance-title,.leave-control{display:flex;align-items:center;justify-content:space-between;gap:16px}.workspace-header{margin-bottom:22px}.workspace-header h2{margin:3px 0;font-size:25px;letter-spacing:-.03em}.eyebrow{margin:0;color:#55708f;font-size:11px;font-weight:800;letter-spacing:.12em}.muted,.workspace-header p.muted{color:#728198;font-size:13px}.month-actions{display:flex;gap:8px}.month-actions input{min-height:34px;border:1px solid #d7dfeb;border-radius:8px;padding:0 8px}.payroll-metrics{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;margin-bottom:16px}.metric{display:flex;flex-direction:column;gap:8px;padding:18px;background:#fff;border:1px solid #e4eaf2;border-radius:15px}.metric.main{background:#153760;color:white;border-color:#153760}.metric span{font-size:12px;opacity:.7}.metric strong{font-size:25px;letter-spacing:-.04em}.metric small{font-size:11px;opacity:.6}.workspace-body{display:grid;grid-template-columns:minmax(270px,350px) minmax(0,1fr);gap:16px;align-items:start}.roster,.employee-pay{background:#fff;border:1px solid #e4eaf2;border-radius:16px;padding:20px;min-height:490px}.section-top h3,.employee-pay h3{display:inline;margin:0;font-size:18px}.section-top span,.employee-pay h3 small{margin-left:8px;color:#8490a4;font-size:12px;font-weight:500}.roster-search{margin:16px 0}.roster-list{max-height:calc(100vh - 370px);overflow:auto}.roster-item{display:flex;align-items:center;gap:11px;width:100%;padding:12px 9px;background:transparent;border:1px solid transparent;border-bottom-color:#edf1f5;border-radius:10px;text-align:left;cursor:pointer}.roster-item:hover,.roster-item.selected{background:#eef5ff;border-color:#c9defa}.avatar{display:grid;place-items:center;flex:none;width:38px;height:38px;border-radius:11px;background:#e8eef8;color:#315e9d;font-weight:800}.roster-identity{display:flex;flex-direction:column;gap:4px;min-width:0}.roster-identity strong{font-size:14px}.roster-identity small{color:#7b8798;font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.roster-pay{margin-left:auto;white-space:nowrap;color:#153760;font-size:13px;font-weight:800}.employee-pay-header{margin-bottom:18px}.take-home{display:flex;align-items:center;justify-content:space-between;margin-bottom:10px;padding:20px;border-radius:12px;background:#eff5ff;color:#153760}.take-home span{font-size:13px}.take-home strong{font-size:26px}.breakdown-row{display:flex;justify-content:space-between;padding:10px 4px;border-bottom:1px solid #edf1f5;font-size:13px}.breakdown-row strong{color:#273b5c}.breakdown-row.deduction strong{color:#d55353}.attendance-box,.orders-preview{margin-top:20px;padding:17px;border:1px solid #e5eaf2;border-radius:12px}.attendance-title h4{margin:0;font-size:14px}.attendance-title span,.attendance-box p,.leave-result{color:#7b8798;font-size:12px}.attendance-box p{margin:8px 0 14px}.leave-stepper{display:flex;align-items:center;gap:6px}.leave-stepper :deep(.el-input-number){width:65px}.leave-result{display:block;margin-top:10px}.order-line{display:flex;justify-content:space-between;gap:12px;padding:10px 0;border-bottom:1px solid #edf1f5;font-size:12px}.order-line span{display:flex;flex-direction:column;gap:4px;min-width:0}.order-line small{color:#8490a4;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.order-line b{color:#2260a2;white-space:nowrap}.order-link{align-self:flex-start;padding:0;border:0;background:transparent;color:#2563eb;font:inherit;font-weight:800;cursor:pointer;text-decoration:underline;text-underline-offset:3px}.orders-preview .muted{margin-bottom:0}@media(max-width:900px){.payroll-workspace{padding:12px}.workspace-header{align-items:flex-start;flex-direction:column}.payroll-metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.workspace-body{grid-template-columns:1fr}.roster,.employee-pay{min-height:0}.roster-list{max-height:280px}}@media(max-width:520px){.employee-pay-header,.leave-control{align-items:flex-start;flex-direction:column}.payroll-metrics{grid-template-columns:1fr}.payroll-metrics .metric strong{font-size:20px}}
</style>
