<template>
  <!--
    工程费用录入 / 编辑弹窗（SPEC-ENG-001 §9.5 录入弹窗 / 任务卡 T14）
    一个组件两个入口，靠 props.projectId 区分：
      - projectId 传数字 = 项目费用（项目锁定不可改）
      - projectId 传 null = 公共/任意项目模式（可从下拉里选项目或"公共费用"）
    不允许拆成两个弹窗，否则校验规则会分叉。
  -->
  <el-dialog
    v-model="visible"
    :title="isEdit ? '编辑工程费用' : '录入工程费用'"
    :width="isMobile ? '92%' : '560px'"
    :close-on-click-modal="false"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="eng-exp-form">
      <el-form-item label="所属项目" prop="project_id">
        <el-select
          v-model="form.project_id"
          filterable
          :disabled="projectLocked"
          placeholder="选择项目，或记为公共费用"
          class="w-full"
        >
          <el-option label="公共费用（不分摊）" :value="0" />
          <el-option v-for="p in projectOptions" :key="p.id" :label="`${p.project_no} ${p.name}`" :value="p.id" />
        </el-select>
        <p v-if="projectLocked" class="eng-exp-hint">从项目进入时项目固定，如需改挂到别的项目请到工程开销台账编辑</p>
      </el-form-item>

      <el-form-item label="费用类别" prop="category">
        <el-radio-group v-model="form.category" class="eng-exp-cats">
          <el-radio-button v-for="c in EXPENSE_CATEGORIES" :key="c.value" :value="c.value">
            {{ c.label }}
          </el-radio-button>
        </el-radio-group>
      </el-form-item>

      <div class="eng-exp-grid">
        <el-form-item label="费用金额" prop="amount">
          <el-input-number
            v-model="form.amount"
            :precision="2"
            :controls="false"
            :min="0"
            :max="9999999.99"
            class="w-full"
            placeholder="0.00"
          />
        </el-form-item>
        <el-form-item label="发生日期" prop="expense_date">
          <el-date-picker
            v-model="form.expense_date"
            type="date"
            value-format="YYYY-MM-DD"
            :disabled-date="disableFuture"
            class="w-full"
          />
        </el-form-item>
      </div>

      <el-form-item label="付款状态" prop="pay_status">
        <el-radio-group v-model="form.pay_status">
          <el-radio-button :value="1">已付</el-radio-button>
          <el-radio-button :value="0">未付（挂账）</el-radio-button>
        </el-radio-group>
      </el-form-item>

      <el-form-item v-if="form.pay_status === 1" label="付款方式" prop="pay_method">
        <el-select v-model="form.pay_method" class="w-full">
          <el-option v-for="m in EXPENSE_PAY_METHODS.filter(x => x.value !== 0)" :key="m.value" :label="m.label" :value="m.value" />
        </el-select>
        <p class="eng-exp-hint">仅作备查，不改变任何账户余额</p>
      </el-form-item>

      <div class="eng-exp-grid">
        <el-form-item label="收款方">
          <el-input v-model="form.payee" maxlength="100" placeholder="选填，如 李师傅" />
        </el-form-item>
        <el-form-item label="票据号">
          <el-input v-model="form.bill_no" maxlength="50" placeholder="选填" />
        </el-form-item>
      </div>

      <el-form-item label="备注" :prop="form.category === 7 ? 'remark' : undefined">
        <el-input
          v-model="form.remark"
          type="textarea"
          :rows="2"
          maxlength="200"
          show-word-limit
          :placeholder="form.category === 7 ? '「其他」类别必须说明这笔钱花在哪' : '选填'"
        />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="flex gap-2">
        <el-button class="flex-1 md:flex-none" @click="visible = false">取消</el-button>
        <el-button class="flex-1 md:flex-none" type="primary" :loading="saving" @click="submit">
          {{ isEdit ? '保存修改' : '保存' }}
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { createEngineeringExpense, updateEngineeringExpense } from '../../api/engineering'
import { EXPENSE_CATEGORIES, EXPENSE_PAY_METHODS } from './EngineeringExpenseTag.vue'

const props = defineProps({
  modelValue: { type: Boolean, required: true },
  projectId: { type: Number, default: null },
  projectOptions: { type: Array, default: () => [] },
  expense: { type: Object, default: null }
})

const emit = defineEmits(['update:modelValue', 'saved'])

const isMobile = ref(window.innerWidth < 768)
window.addEventListener('resize', () => {
  isMobile.value = window.innerWidth < 768
})

const formRef = ref(null)
const saving = ref(false)
const isEdit = computed(() => !!props.expense?.id)
// projectId 是数字（含 0 之外的合法 id）时锁定项目选择
const projectLocked = computed(() => typeof props.projectId === 'number' && props.projectId > 0)

const today = () => {
  const d = new Date()
  const pad = n => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

const emptyForm = () => ({
  project_id: projectLocked.value ? props.projectId : 0,
  category: 1,
  amount: null,
  expense_date: today(),
  pay_status: 1,
  pay_method: 3,
  payee: '',
  bill_no: '',
  remark: ''
})

const form = reactive(emptyForm())

const rules = {
  project_id: [{ required: true, message: '请选择所属项目', trigger: 'change' }],
  category: [{ required: true, message: '请选择费用类别', trigger: 'change' }],
  amount: [
    { required: true, message: '请填写费用金额', trigger: 'blur' },
    {
      validator: (_r, v, cb) => (Number(v) > 0 ? cb() : cb(new Error('费用金额必须大于 0'))),
      trigger: 'blur'
    }
  ],
  expense_date: [{ required: true, message: '请选择发生日期', trigger: 'change' }],
  pay_method: [{ required: true, message: '请选择付款方式', trigger: 'change' }],
  remark: [{ required: true, message: '「其他」类别请填写备注', trigger: 'blur' }]
}

const visible = computed({
  get: () => props.modelValue,
  set: v => emit('update:modelValue', v)
})

const disableFuture = date => date.getTime() > Date.now()

watch(
  () => props.modelValue,
  open => {
    if (!open) return
    formRef.value?.clearValidate()
    if (props.expense?.id) {
      const e = props.expense
      Object.assign(form, {
        project_id: e.project_id === null || e.project_id === undefined ? 0 : e.project_id,
        category: e.category || 1,
        amount: Number(e.amount) || null,
        expense_date: e.expense_date || today(),
        pay_status: e.pay_status === 0 ? 0 : 1,
        pay_method: e.pay_method || 3,
        payee: e.payee || '',
        bill_no: e.bill_no || '',
        remark: e.remark || ''
      })
    } else {
      Object.assign(form, emptyForm())
    }
  }
)

const buildPayload = () => ({
  project_id: form.project_id ? Number(form.project_id) : null,
  category: Number(form.category),
  amount: Number(form.amount),
  expense_date: form.expense_date,
  pay_status: Number(form.pay_status),
  pay_method: form.pay_status === 1 ? Number(form.pay_method) : 0,
  payee: (form.payee || '').trim(),
  bill_no: (form.bill_no || '').trim(),
  remark: (form.remark || '').trim()
})

const submit = async () => {
  try {
    await formRef.value.validate()
  } catch (e) {
    return
  }
  saving.value = true
  try {
    let res
    if (isEdit.value) {
      res = await updateEngineeringExpense(props.expense.id, buildPayload())
      ElMessage.success('费用已更新')
    } else {
      res = await createEngineeringExpense({
        ...buildPayload(),
        client_request_id: `engexp-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
      })
      ElMessage.success('费用已录入')
    }
    visible.value = false
    emit('saved', res?.result || null)
  } catch (e) {
    /* 后端 msg 由拦截器提示 */
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.eng-exp-form {
  --el-font-size-base: 13px;
}
.eng-exp-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 14px;
}
.eng-exp-cats {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.eng-exp-cats :deep(.el-radio-button__inner) {
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font-size: 12px;
  padding: 6px 10px;
  box-shadow: none;
}
.eng-exp-cats :deep(.el-radio-button.is-active .el-radio-button__inner) {
  background: #635bff;
  border-color: #635bff;
  color: #fff;
}
.eng-exp-hint {
  margin-top: 2px;
  font-size: 11px;
  line-height: 16px;
  color: #94a3b8;
}
@media (max-width: 767px) {
  .eng-exp-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
