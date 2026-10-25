<script setup>
import { computed, ref, watch } from 'vue'
import dayjs from 'dayjs'
import { ElMessage } from 'element-plus'
import {
  fetchOffboardings,
  createOffboarding,
  updateOffboardingTask,
  settleOffboarding
} from '../../api/employees'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  employee: { type: Object, default: null },
  employees: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:modelValue', 'updated'])

const loading = ref(false)
const saving = ref(false)
const caseData = ref(null)
const tasks = ref([])
const alerts = ref([])
const taskNotes = ref({})
const form = ref({ last_working_date: dayjs().format('YYYY-MM-DD'), holiday_days: 0, successor_employee_id: null, notes: '' })
const today = dayjs().format('YYYY-MM-DD')
const eligibleSuccessors = computed(() => props.employees.filter(row =>
  row.id !== props.employee?.id && !row.is_outsourced &&
  ['active', ''].includes(row.employment_status || '')
))
const allTasksDone = computed(() => tasks.value.length > 0 && tasks.value.every(task => task.status === 'done'))
const canSettle = computed(() => caseData.value?.status === 'in_progress' && allTasksDone.value && caseData.value.last_working_date < today)
const openAlerts = computed(() => alerts.value.filter(alert => alert.status === 'open'))
const amount = value => `¥${Number(value || 0).toFixed(2)}`
const dateText = value => value ? dayjs(value).format('YYYY-MM-DD') : '—'
const alertLabel = alert => ({
  pending_fulfillment: '待履约订单',
  order_cancelled: '离职后取消',
  order_reversed: '离职后红冲',
  order_refund: '离职后退款'
}[alert.alert_type] || '订单变更')

watch(() => [props.modelValue, props.employee?.id], async ([visible]) => {
  if (visible && props.employee?.id) await loadCase()
}, { immediate: true })

function close() { emit('update:modelValue', false) }

async function loadCase() {
  loading.value = true
  caseData.value = null
  tasks.value = []
  alerts.value = []
  try {
    const response = await fetchOffboardings(props.employee.id)
    if (response.data.code !== 200) throw new Error('交接记录加载失败')
    const data = response.data.data
    if (data) {
      caseData.value = data.offboarding
      tasks.value = data.tasks || []
      alerts.value = data.alerts || []
      taskNotes.value = Object.fromEntries(tasks.value.map(task => [task.id, task.notes || '']))
    } else {
      form.value = { last_working_date: dayjs().format('YYYY-MM-DD'), holiday_days: 0, successor_employee_id: null, notes: '' }
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || error.message || '交接记录加载失败')
  } finally {
    loading.value = false
  }
}

async function startOffboarding() {
  if (!form.value.last_working_date) return ElMessage.warning('请选择最后工作日')
  saving.value = true
  try {
    const response = await createOffboarding(props.employee.id, form.value)
    if (response.data.code !== 200) throw new Error(response.data.msg || '发起交接失败')
    ElMessage.success('离职交接已发起')
    emit('updated')
    await loadCase()
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || error.message || '发起交接失败')
  } finally {
    saving.value = false
  }
}

async function updateTask(task, done) {
  const notes = String(taskNotes.value[task.id] || '').trim()
  if (done && !notes) return ElMessage.warning('请填写验收说明后再完成任务')
  saving.value = true
  try {
    const response = await updateOffboardingTask(caseData.value.id, task.id, { done, notes })
    if (response.data.code !== 200) throw new Error(response.data.msg || '任务更新失败')
    await loadCase()
    emit('updated')
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || error.message || '任务更新失败')
  } finally {
    saving.value = false
  }
}

async function settle() {
  saving.value = true
  try {
    const response = await settleOffboarding(caseData.value.id)
    if (response.data.code !== 200) throw new Error(response.data.msg || '结算确认失败')
    ElMessage.success('离职交接与应付金额已确认')
    caseData.value = response.data.data
    emit('updated')
    await loadCase()
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || error.message || '结算确认失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <el-dialog :model-value="modelValue" :title="`离职交接 · ${employee?.name || ''}`" width="min(760px, 94vw)" destroy-on-close @close="close">
    <div v-loading="loading" class="offboarding-dialog">
      <template v-if="!caseData">
        <el-alert v-if="employee?.employment_status === 'departed'" type="info" :closable="false" title="该员工暂无离职交接单，仅能查看员工档案。" />
        <template v-else>
        <p class="intro">登记最后工作日和本月假期天数，系统会生成交接清单并标记该员工关联的未结订单。</p>
        <el-form label-position="top" class="start-form">
          <el-form-item label="最后工作日" required>
            <el-date-picker v-model="form.last_working_date" type="date" value-format="YYYY-MM-DD" format="YYYY-MM-DD" :clearable="false" />
          </el-form-item>
          <el-form-item label="当月假期天数">
            <el-input-number v-model="form.holiday_days" :min="0" :max="30" :precision="0" />
            <small>用于底薪公式：底薪 ÷（当月天数 − 假期天数）× 当月签到天数</small>
          </el-form-item>
          <el-form-item label="接手人（可留空）">
            <el-select v-model="form.successor_employee_id" clearable placeholder="选择交接接手人">
              <el-option v-for="person in eligibleSuccessors" :key="person.id" :label="`${person.name} · ${person.emp_no}`" :value="person.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="交接备注"><el-input v-model="form.notes" type="textarea" :rows="3" maxlength="500" show-word-limit /></el-form-item>
        </el-form>
        <div class="policy-note">最后工作日前，员工终端仍可使用；最后工作日结束后停用。带薪假目前不自动折算为签到天数，薪资只按签到记录计算。</div>
        </template>
      </template>

      <template v-else>
        <div class="case-summary">
          <div><span>状态</span><el-tag :type="caseData.status === 'completed' ? 'success' : 'warning'">{{ caseData.status === 'completed' ? '已结算' : '交接中' }}</el-tag></div>
          <div><span>最后工作日</span><strong>{{ dateText(caseData.last_working_date) }}</strong></div>
          <div><span>本月假期</span><strong>{{ caseData.holiday_days }} 天</strong></div>
          <div v-if="caseData.status === 'completed'"><span>签到天数</span><strong>{{ caseData.signed_in_days }} 天</strong></div>
        </div>
        <section class="section-block">
          <div class="section-heading"><h3>交接清单</h3><span>{{ tasks.filter(task => task.status === 'done').length }} / {{ tasks.length }} 项验收</span></div>
          <article v-for="task in tasks" :key="task.id" class="task-row">
            <div class="task-copy"><strong>{{ task.title }}</strong><el-tag size="small" :type="task.status === 'done' ? 'success' : 'info'">{{ task.status === 'done' ? '已完成' : '待处理' }}</el-tag></div>
            <el-input v-if="caseData.status === 'in_progress'" v-model="taskNotes[task.id]" size="small" placeholder="填写验收说明" />
            <p v-else-if="task.notes" class="task-note">{{ task.notes }}</p>
            <el-button v-if="caseData.status === 'in_progress'" size="small" :type="task.status === 'done' ? 'default' : 'primary'" :loading="saving" @click="updateTask(task, task.status !== 'done')">{{ task.status === 'done' ? '重新打开' : '验收完成' }}</el-button>
          </article>
        </section>

        <section class="section-block alert-block">
          <div class="section-heading"><h3>关联订单提醒</h3><span>{{ openAlerts.length }} 条待处理</span></div>
          <div v-if="alerts.length" class="alert-list">
            <article v-for="alert in alerts" :key="alert.id" class="alert-row" :class="{ resolved: alert.status !== 'open' }">
              <div class="alert-title"><el-tag size="small" type="danger">{{ alertLabel(alert) }}</el-tag><strong>{{ alert.order_no }}</strong><el-tag v-if="alert.status !== 'open'" size="small" type="success">已复核</el-tag></div>
              <p>{{ alert.message }}</p>
              <small>涉及提成 {{ amount(alert.commission_amount) }}<template v-if="alert.refund_amount"> · 退款 {{ amount(alert.refund_amount) }}</template></small>
              <p v-if="alert.resolution" class="task-note">复核：{{ alert.resolution }}</p>
            </article>
          </div>
          <el-empty v-else description="暂无关联订单提醒" :image-size="56" />
        </section>

        <section class="settlement-box">
          <template v-if="caseData.status === 'completed'">
            <div class="settlement-amount">应付金额 <strong>{{ amount(caseData.final_payable_snapshot) }}</strong></div>
            <p>底薪 {{ amount(caseData.calculated_base_salary) }} + 提成 {{ amount(caseData.commission_snapshot) }}。实际付款仍以财务流水为准。</p>
          </template>
          <template v-else>
            <p>最后工作日结束且清单全部验收后，可确认离职应付金额。确认后任职状态转为已离职。</p>
            <el-button type="primary" :disabled="!canSettle" :loading="saving" @click="settle">确认交接并计算应付金额</el-button>
          </template>
        </section>
      </template>
    </div>
    <template #footer><el-button @click="close">关闭</el-button><el-button v-if="!caseData && employee?.employment_status !== 'departed'" type="primary" :loading="saving" @click="startOffboarding">发起交接</el-button></template>
  </el-dialog>
</template>

<style scoped>
.offboarding-dialog{min-height:180px;color:#34445a}.intro,.policy-note,.settlement-box p{margin:0 0 16px;color:#718096;font-size:13px;line-height:1.7}.start-form{display:grid;grid-template-columns:1fr 1fr;gap:0 18px}.start-form :deep(.el-form-item:nth-child(4)){grid-column:1 / -1}.start-form :deep(.el-date-editor),.start-form :deep(.el-select){width:100%}.start-form small{display:block;margin-top:5px;color:#8995a6;font-size:11px}.policy-note{padding:11px 13px;border-radius:10px;background:#f4f7fb;margin:0}.case-summary{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;padding:14px;border:1px solid #e8edf3;border-radius:12px;background:#fbfcfe}.case-summary>div{display:flex;flex-direction:column;gap:7px}.case-summary span,.section-heading span{font-size:11px;color:#8a96a7}.case-summary strong{font-size:13px;color:#29384d}.section-block{margin-top:20px}.section-heading{display:flex;align-items:center;justify-content:space-between;margin-bottom:10px}.section-heading h3{margin:0;font-size:14px;color:#26364d}.task-row{display:grid;grid-template-columns:minmax(180px,1fr) minmax(150px,.8fr) auto;align-items:center;gap:10px;padding:10px 0;border-top:1px solid #edf1f5}.task-copy,.alert-title{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.task-copy strong,.alert-title strong{font-size:12px;color:#34445a}.task-note{grid-column:1 / -1;margin:0;color:#778499;font-size:12px;line-height:1.5}.alert-block{padding:14px;border:1px solid #f2d4d2;border-radius:12px;background:#fffafa}.alert-list{display:grid;gap:8px}.alert-row{padding:11px;border-radius:9px;background:white;border:1px solid #f1dfdd}.alert-row.resolved{border-color:#e4ebea;background:#fbfdfc}.alert-row p{margin:7px 0;color:#7a8797;font-size:12px;line-height:1.5}.alert-row small{color:#a34d43;font-size:11px}.settlement-box{margin-top:18px;padding:14px;border-radius:12px;background:#f5f8fc}.settlement-box p{margin-bottom:10px}.settlement-amount{display:flex;align-items:center;gap:12px;color:#617087;font-size:12px}.settlement-amount strong{color:#138f74;font-size:22px}@media(max-width:600px){.start-form{display:block}.case-summary{grid-template-columns:repeat(2,1fr)}.task-row{grid-template-columns:1fr auto}.task-row :deep(.el-input){grid-column:1 / -1;grid-row:2}.task-note{grid-column:1 / -1}}
</style>
