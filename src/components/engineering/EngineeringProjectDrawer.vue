<template>
  <!--
    新建 / 编辑工程抽屉（SPEC-ENG-001 §6，任务卡 T15）
    一个组件两种模式：project 为 null = 新建，传对象 = 编辑。
    项目编号留空由后端生成（GC-YYYYMMDD-NNN，重复自动重试），前端不做号段计算。
  -->
  <el-drawer
    v-model="visible"
    :title="isEdit ? '编辑工程项目' : '新建工程项目'"
    :size="isMobile ? '100%' : '720px'"
    :close-on-click-modal="false"
    :before-close="handleBeforeClose"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="eng-form">
      <!-- ① 项目信息 -->
      <p class="eng-section-title">① 项目信息</p>
      <div class="eng-grid">
        <el-form-item label="项目编号" prop="project_no">
          <div class="eng-no-row">
            <el-input v-model="form.project_no" maxlength="30" placeholder="留空自动生成" :disabled="isEdit" />
            <el-button v-if="!isEdit" text @click="form.project_no = ''">自动生成</el-button>
          </div>
        </el-form-item>
        <el-form-item label="项目类型" prop="project_type">
          <el-select v-model="form.project_type" placeholder="请选择">
            <el-option v-for="o in PROJECT_TYPE_OPTIONS" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
      </div>
      <el-form-item label="项目名称" prop="name">
        <el-input
          v-model="form.name"
          maxlength="100"
          show-word-limit
          placeholder="如：绿城桂语 3 期精装房空调工程"
        />
      </el-form-item>

      <!-- ② 甲方信息 -->
      <p class="eng-section-title">② 甲方信息</p>
      <div class="eng-grid">
        <el-form-item label="甲方单位" prop="client_name">
          <el-input v-model="form.client_name" maxlength="150" placeholder="如：绿城置业有限公司" />
        </el-form-item>
        <el-form-item label="甲方联系人" prop="client_contact">
          <el-input v-model="form.client_contact" maxlength="50" placeholder="如：张经理" />
        </el-form-item>
        <el-form-item label="联系电话" prop="client_phone">
          <el-input v-model="form.client_phone" maxlength="20" placeholder="手机或座机，如 0571-88886666" />
        </el-form-item>
        <el-form-item label="项目地址">
          <el-input v-model="form.address" maxlength="255" placeholder="送货地址默认取此值" />
        </el-form-item>
      </div>

      <!-- ③ 商务信息 -->
      <p class="eng-section-title">③ 商务信息</p>
      <div class="eng-grid">
        <el-form-item label="销售负责人" prop="owner_employee_id">
          <el-select v-model="form.owner_employee_id" filterable placeholder="请选择">
            <el-option v-for="e in employees" :key="e.id" :label="e.name" :value="e.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="预计合同额">
          <el-input-number
            v-model="form.estimated_amount"
            :precision="2"
            :controls="false"
            :min="0"
            class="w-full"
            placeholder="跟进阶段预估"
          />
          <p class="eng-hint">跟进阶段的预估金额，不参与签约校验</p>
        </el-form-item>
        <el-form-item label="甲方合同号">
          <el-input v-model="form.contract_no" maxlength="50" placeholder="选填" />
        </el-form-item>
        <el-form-item label="签约日期" :prop="needSignDate ? 'sign_date' : undefined">
          <el-date-picker
            v-model="form.sign_date"
            type="date"
            value-format="YYYY-MM-DD"
            placeholder="已签约必填"
            class="w-full"
          />
        </el-form-item>
      </div>

      <el-form-item label="备注">
        <el-input v-model="form.remark" type="textarea" :rows="3" maxlength="500" show-word-limit />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="flex gap-2">
        <el-button class="flex-1 md:flex-none" @click="handleBeforeClose()">取消</el-button>
        <el-button class="flex-1 md:flex-none" type="primary" :loading="saving" @click="submit">
          {{ isEdit ? '保存' : '创建项目' }}
        </el-button>
      </div>
    </template>
  </el-drawer>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  createEngineeringProject,
  getEngineeringProject,
  updateEngineeringProject
} from '../../api/engineering'
import { PROJECT_TYPE_OPTIONS } from '../../composables/useEngineeringList'

const props = defineProps({
  modelValue: { type: Boolean, required: true },
  project: { type: Object, default: null },
  employees: { type: Array, default: () => [] }
})

const emit = defineEmits(['update:modelValue', 'saved'])

const isMobile = ref(window.innerWidth < 768)
window.addEventListener('resize', () => {
  isMobile.value = window.innerWidth < 768
})

const formRef = ref(null)
const saving = ref(false)
const loadingDetail = ref(false)
const isEdit = computed(() => !!props.project?.id)
const needSignDate = computed(() => Number(props.project?.status || 0) >= 2)

const emptyForm = () => ({
  project_no: '',
  name: '',
  project_type: 1,
  client_name: '',
  client_contact: '',
  client_phone: '',
  address: '',
  owner_employee_id: null,
  estimated_amount: null,
  contract_no: '',
  sign_date: null,
  remark: ''
})

const form = reactive(emptyForm())

// 手机 1[3-9]xxxxxxxxx 或座机 0XX(X)-XXXXXXX(X)（与后端 engPhoneRe 同规则）
const phoneOk = v => /^(1[3-9]\d{9}|0\d{2,3}-?\d{7,8})$/.test(String(v || '').trim())

const rules = {
  name: [{ required: true, message: '请填写项目名称', trigger: 'blur' }],
  project_type: [{ required: true, message: '请选择项目类型', trigger: 'change' }],
  client_name: [{ required: true, message: '请填写甲方单位', trigger: 'blur' }],
  client_contact: [{ required: true, message: '请填写甲方联系人', trigger: 'blur' }],
  client_phone: [
    { required: true, message: '请填写联系电话', trigger: 'blur' },
    {
      validator: (_r, v, cb) => (phoneOk(v) ? cb() : cb(new Error('请输入有效的手机号或座机'))),
      trigger: 'blur'
    }
  ],
  owner_employee_id: [{ required: true, message: '请选择销售负责人', trigger: 'change' }],
  sign_date: [{ required: true, message: '已签约项目必须填写签约日期', trigger: 'change' }]
}

const visible = computed({
  get: () => props.modelValue,
  set: v => emit('update:modelValue', v)
})

const snapshot = ref('')
const takenSnapshot = () => JSON.stringify(form)
const isDirty = () => snapshot.value !== takenSnapshot()

const fill = source => {
  Object.assign(form, emptyForm())
  if (source) {
    Object.assign(form, {
      project_no: source.project_no || '',
      name: source.name || '',
      project_type: source.project_type || 1,
      client_name: source.client_name || '',
      client_contact: source.client_contact || '',
      client_phone: source.client_phone || '',
      address: source.address || '',
      owner_employee_id: source.owner_employee_id || null,
      estimated_amount: source.estimated_amount === null || source.estimated_amount === undefined ? null : Number(source.estimated_amount),
      contract_no: source.contract_no || '',
      sign_date: source.sign_date || null,
      remark: source.remark || ''
    })
  }
  snapshot.value = takenSnapshot()
}

// 打开时：新建重置表单；编辑拉一次详情（列表行不含 address/remark 等字段）
watch(
  () => props.modelValue,
  async open => {
    if (!open) return
    formRef.value?.clearValidate()
    if (!props.project?.id) {
      fill(null)
      return
    }
    fill(props.project)
    loadingDetail.value = true
    try {
      const res = await getEngineeringProject(props.project.id)
      if (res?.data) fill(res.data)
    } catch (e) {
      /* 拉详情失败就用列表行数据，字段会有缺省 */
    } finally {
      loadingDetail.value = false
    }
  }
)

const handleBeforeClose = async done => {
  if (saving.value) return
  if (isDirty()) {
    try {
      await ElMessageBox.confirm('有未保存的修改，确定关闭？', '提示', {
        confirmButtonText: '放弃修改',
        cancelButtonText: '继续编辑',
        type: 'warning'
      })
    } catch (e) {
      return
    }
  }
  if (typeof done === 'function') done()
  else visible.value = false
}

const buildPayload = () => {
  const phone = String(form.client_phone || '').trim()
  const payload = {
    name: form.name.trim(),
    project_type: Number(form.project_type),
    client_name: form.client_name.trim(),
    client_contact: form.client_contact.trim(),
    client_phone: phone,
    address: form.address.trim(),
    owner_employee_id: form.owner_employee_id ? Number(form.owner_employee_id) : null,
    contract_no: form.contract_no.trim(),
    estimated_amount: form.estimated_amount === null ? null : Number(form.estimated_amount),
    sign_date: form.sign_date || null,
    remark: form.remark.trim()
  }
  if (!isEdit.value && form.project_no.trim()) payload.project_no = form.project_no.trim()
  return payload
}

const submit = async () => {
  if (loadingDetail.value) return
  try {
    await formRef.value.validate()
  } catch (e) {
    return
  }
  saving.value = true
  try {
    if (isEdit.value) {
      await updateEngineeringProject(props.project.id, buildPayload())
      ElMessage.success('项目已保存')
      snapshot.value = takenSnapshot()
      emit('saved', { id: props.project.id, created: false })
    } else {
      // 幂等键：双击/网络重试不会建出两个项目
      const payload = { ...buildPayload(), client_request_id: `eng-${Date.now()}-${Math.random().toString(36).slice(2, 8)}` }
      const res = await createEngineeringProject(payload)
      const id = res?.result?.id
      ElMessage.success('项目已创建')
      snapshot.value = takenSnapshot()
      emit('saved', { id, created: true })
    }
  } catch (e) {
    /* 后端 msg 由拦截器统一提示（如"该编号已被占用"） */
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.eng-form {
  --el-font-size-base: 13px;
}
.eng-section-title {
  margin: 4px 0 12px;
  font-size: 12px;
  font-weight: 600;
  color: #94a3b8;
  letter-spacing: 0.04em;
}
.eng-section-title:not(:first-child) {
  margin-top: 20px;
}
.eng-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}
.eng-no-row {
  display: flex;
  align-items: center;
  gap: 4px;
  width: 100%;
}
.eng-hint {
  margin-top: 2px;
  font-size: 11px;
  line-height: 16px;
  color: #94a3b8;
}
@media (max-width: 767px) {
  .eng-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
