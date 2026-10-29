<template>
  <!--
    回款登记弹窗（SPEC-ENG-001 §7，任务卡 T15）
    从头部「登记回款」打开时默认选中最近一个未收/部分收节点；从节点行打开时锁定该节点。
  -->
  <el-dialog v-model="visible" title="登记回款" :width="isMobile ? '92%' : '520px'" :close-on-click-modal="false">
    <div class="eng-receipt-body">
      <!-- 节点信息 + 剩余应收 -->
      <div class="eng-term-box">
        <div class="eng-term-head">
          <el-select
            v-model="form.term_id"
            placeholder="选择付款节点"
            :disabled="!!term"
            class="w-full"
            @change="onTermChange"
          >
            <el-option
              v-for="t in termOptions"
              :key="t.id"
              :label="`${t.name}（未收 ${formatEngMoney(t.pending_amount)}）`"
              :value="t.id"
            />
          </el-select>
        </div>
        <div class="eng-term-nums">
          <span>计划 {{ formatEngMoney(currentTerm?.planned_amount) }}</span>
          <span>已收 {{ formatEngMoney(currentTerm?.received_amount) }}</span>
        </div>
        <p class="eng-term-remain">剩余应收 {{ formatEngMoney(remain) }}</p>
      </div>

      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="eng-receipt-form">
        <el-form-item label="实收金额" prop="amount">
          <div class="eng-amount-row">
            <el-input-number
              v-model="form.amount"
              :precision="2"
              :controls="false"
              :min="0"
              :max="remain || undefined"
              class="flex-1"
            />
            <el-button text @click="form.amount = remain">全额</el-button>
          </div>
          <p v-if="overLimit" class="eng-err">不能超过该节点剩余应收 {{ formatEngMoney(remain) }}</p>
        </el-form-item>

        <div class="eng-grid">
          <el-form-item label="到账日期" prop="payment_date">
            <el-date-picker
              v-model="form.payment_date"
              type="date"
              value-format="YYYY-MM-DD"
              :disabled-date="disableFuture"
              class="w-full"
            />
          </el-form-item>
          <el-form-item label="收款方式" prop="method">
            <el-select v-model="form.method" class="w-full">
              <el-option v-for="m in METHODS" :key="m.value" :label="m.label" :value="m.value" />
            </el-select>
          </el-form-item>
        </div>

        <el-form-item label="票据号">
          <el-input
            v-model="form.bill_no"
            maxlength="50"
            :placeholder="form.method === 2 ? '建议填写票据号与到期日' : '选填'"
          />
        </el-form-item>

        <el-form-item label="备注">
          <el-input v-model="form.remark" type="textarea" :rows="2" maxlength="200" show-word-limit placeholder="选填" />
        </el-form-item>
      </el-form>
    </div>

    <template #footer>
      <div class="flex gap-2">
        <el-button class="flex-1 md:flex-none" @click="visible = false">取消</el-button>
        <el-button class="flex-1 md:flex-none" type="primary" :loading="saving" :disabled="!form.term_id" @click="submit">
          确认收款
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { formatEngMoney } from '../../utils/engineeringFormat'

const props = defineProps({
  modelValue: { type: Boolean, required: true },
  terms: { type: Array, default: () => [] },
  term: { type: Object, default: null },
  saving: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue', 'confirm'])

const isMobile = ref(window.innerWidth < 768)
window.addEventListener('resize', () => {
  isMobile.value = window.innerWidth < 768
})

const METHODS = [
  { value: 1, label: '银行转账' },
  { value: 2, label: '承兑汇票' },
  { value: 3, label: '现金' },
  { value: 4, label: '支票' },
  { value: 5, label: '其他' }
]

const formRef = ref(null)

const today = () => {
  const d = new Date()
  const pad = n => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

const form = reactive({ term_id: null, amount: null, payment_date: today(), method: 1, bill_no: '', remark: '' })

// 只有还有未收金额的节点可以登记
const termOptions = computed(() => props.terms.filter(t => Number(t.pending_amount) > 0))
const currentTerm = computed(() => props.terms.find(t => t.id === form.term_id) || null)
const remain = computed(() => Math.max(0, Number(currentTerm.value?.pending_amount) || 0))

const rules = {
  amount: [
    { required: true, message: '请填写实收金额', trigger: 'blur' },
    {
      validator: (_r, v, cb) => {
        const num = Number(v)
        if (!(num > 0)) return cb(new Error('实收金额必须大于 0'))
        if (num > remain.value + 0.001) return cb(new Error(`不能超过该节点剩余应收 ${formatEngMoney(remain.value)}`))
        return cb()
      },
      trigger: 'blur'
    }
  ],
  payment_date: [{ required: true, message: '请选择到账日期', trigger: 'change' }],
  method: [{ required: true, message: '请选择收款方式', trigger: 'change' }]
}

const overLimit = computed(() => Number(form.amount) > remain.value + 0.001)

const visible = computed({
  get: () => props.modelValue,
  set: v => emit('update:modelValue', v)
})

const disableFuture = date => date.getTime() > Date.now()

const onTermChange = () => {
  form.amount = remain.value
}

watch(
  () => props.modelValue,
  open => {
    if (!open) return
    formRef.value?.clearValidate()
    form.payment_date = today()
    form.method = 1
    form.bill_no = ''
    form.remark = ''
    // 优先锁定传入的节点；否则取"最近一个未收节点"（按计划日期最早未收）
    const locked = props.term && Number(props.term.pending_amount) > 0 ? props.term : null
    const pick = locked || termOptions.value[0] || null
    form.term_id = pick ? pick.id : null
    form.amount = pick ? Math.max(0, Number(pick.pending_amount) || 0) : null
  }
)

const submit = async () => {
  try {
    await formRef.value.validate()
  } catch (e) {
    return
  }
  emit('confirm', {
    term_id: form.term_id,
    amount: Number(form.amount),
    payment_date: form.payment_date,
    method: Number(form.method),
    bill_no: (form.bill_no || '').trim(),
    remark: (form.remark || '').trim(),
    client_request_id: `engrc-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  })
}

// 父层在成功后关闭弹窗并提示
const close = () => {
  visible.value = false
}
defineExpose({ close })
</script>

<style scoped>
.eng-receipt-body {
  font-size: 13px;
}
.eng-term-box {
  border-radius: 12px;
  background: #f8fafc;
  padding: 12px 14px;
  margin-bottom: 14px;
}
.eng-term-head {
  margin-bottom: 8px;
}
.eng-term-nums {
  display: flex;
  gap: 16px;
  font-size: 12px;
  color: #64748b;
}
.eng-term-remain {
  margin-top: 6px;
  font-size: 20px;
  font-weight: 900;
  color: #635bff;
  font-variant-numeric: tabular-nums;
}
.eng-receipt-form {
  --el-font-size-base: 13px;
}
.eng-amount-row {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
}
.eng-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 14px;
}
.eng-err {
  margin-top: 2px;
  font-size: 11px;
  color: #dc2626;
}
@media (max-width: 767px) {
  .eng-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
