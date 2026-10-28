<script setup>
import { computed, nextTick, shallowRef, useTemplateRef } from 'vue'
import dayjs from 'dayjs'
import { Share } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import PayrollPdfDocument from './PayrollPdfDocument.vue'
import { createPayrollPdf, payrollPdfFilename, sharePayrollPdf } from '../../utils/payrollPdf'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  employee: { type: Object, default: null },
  month: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue', 'view-order'])
const pdfDocument = useTemplateRef('pdfDocument')
const sharing = shallowRef(false)
const visible = computed({
  get: () => props.modelValue,
  set: value => emit('update:modelValue', value)
})

const money = value => `¥${Number(value || 0).toFixed(2)}`
const orderDate = value => value ? dayjs(value).format('MM-DD HH:mm') : '-'
const productModels = order => (order.product_models || []).join('、') || '未记录商品型号'

const handleShare = async () => {
  if (!props.employee || sharing.value) return
  sharing.value = true
  try {
    await nextTick()
    await document.fonts?.ready
    const pages = pdfDocument.value?.getPageElements() || []
    const blob = await createPayrollPdf(pages)
    const filename = payrollPdfFilename(props.employee, props.month)
    const result = await sharePayrollPdf({
      blob,
      filename,
      employee: props.employee,
      month: props.month
    })
    if (result === 'downloaded') ElMessage.success('当前浏览器不支持分享 PDF，已为你下载文件')
    if (result === 'shared') ElMessage.success('薪资明细已发送到系统分享面板')
  } catch (error) {
    ElMessage.error(error?.message || '生成薪资明细 PDF 失败')
  } finally {
    sharing.value = false
  }
}
</script>

<template>
  <el-dialog v-model="visible" width="min(960px, 96vw)" append-to-body destroy-on-close>
    <template #header>
      <div class="dialog-header">
        <strong>{{ employee?.employee_name || '员工' }} · {{ month }} 薪资明细</strong>
        <el-button type="primary" :icon="Share" :loading="sharing" :disabled="!employee" @click="handleShare">
          {{ sharing ? '正在生成 PDF' : '分享 PDF' }}
        </el-button>
      </div>
    </template>
    <template v-if="employee">
      <div class="salary-breakdown">
        <div><span>基本底薪</span><strong>{{ money(employee.base_salary) }}</strong></div>
        <div><span>个人订单提成</span><strong>{{ money(employee.personal_commission) }}</strong></div>
        <div><span>商会订单分摊</span><strong>{{ money(employee.chamber_commission) }}</strong></div>
        <div><span>超假扣款</span><strong class="deduction">-{{ money(employee.deduction) }}</strong></div>
        <div class="salary-total"><span>本月实发</span><strong>{{ money(employee.final_salary) }}</strong></div>
      </div>
      <p class="salary-note">
        请假 {{ employee.leave_days || 0 }} 天，免费 {{ employee.free_leave_days || 0 }} 天，
        扣款 {{ employee.deduction_days || 0 }} 天 × {{ money(employee.daily_salary) }}/天。
      </p>
      <h3 class="detail-heading">提成订单明细</h3>
      <el-table :data="employee.commission_details || []" border stripe size="small" max-height="420">
        <el-table-column label="订单号 / 时间" min-width="220">
          <template #default="{ row }">
            <button type="button" class="order-link" @click="emit('view-order', row)">{{ row.order_no }}</button>
            <small class="order-meta">{{ orderDate(row.order_date) }}</small>
            <small class="order-meta">客户：{{ row.customer_name || '散客' }}</small>
            <small class="order-models">型号：{{ productModels(row) }}</small>
          </template>
        </el-table-column>
        <el-table-column label="来源" min-width="100">
          <template #default="{ row }">
            <el-tag :type="row.order_type === 2 ? 'warning' : 'info'" size="small">
              {{ row.order_type === 2 ? '商会平分' : '个人订单' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="订单金额" min-width="110" align="right">
          <template #default="{ row }">{{ money(row.order_amount) }}</template>
        </el-table-column>
        <el-table-column label="订单提成" min-width="110" align="right">
          <template #default="{ row }">{{ money(row.commission_pool) }}</template>
        </el-table-column>
        <el-table-column label="分摊人数" min-width="90" align="center">
          <template #default="{ row }">{{ row.order_type === 2 ? row.participant_count : '—' }}</template>
        </el-table-column>
        <el-table-column label="本人所得" min-width="110" align="right">
          <template #default="{ row }"><strong>{{ money(row.commission_amount) }}</strong></template>
        </el-table-column>
        <template #empty>本月没有计入提成的订单</template>
      </el-table>
      <PayrollPdfDocument ref="pdfDocument" :employee="employee" :month="month" />
    </template>
  </el-dialog>
</template>

<style scoped>
.dialog-header { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding-right: 28px; }
.dialog-header strong { color: #17243d; font-size: 17px; }
.salary-breakdown {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(145px, 1fr));
  gap: 10px;
}
.salary-breakdown > div {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  background: #f8fafc;
}
.salary-breakdown span, .salary-note { color: #64748b; font-size: 12px; }
.salary-breakdown strong { color: #0f172a; font-size: 17px; }
.salary-breakdown .deduction { color: #ef4444; }
.salary-breakdown .salary-total { background: #eef0ff; border-color: #dcdaff; }
.salary-breakdown .salary-total strong { color: #635bff; }
.salary-note { margin: 12px 0 18px; }
.detail-heading { margin: 0 0 10px; color: #334155; font-size: 14px; font-weight: 700; }
.order-link { padding: 0; border: 0; background: transparent; color: #635bff; font: inherit; font-weight: 800; cursor: pointer; text-decoration: underline; text-underline-offset: 3px; }
.order-meta, .order-models { display: block; margin-top: 3px; color: #7b8798; font-size: 11px; line-height: 1.35; }
.order-models { color: #52657e; }
@media (max-width: 560px) {
  .dialog-header { align-items: flex-start; flex-direction: column; padding-right: 22px; }
  .dialog-header :deep(.el-button) { width: 100%; margin-left: 0; }
}
</style>
