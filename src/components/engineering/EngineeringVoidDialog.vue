<template>
  <!--
    状态危险操作弹窗（SPEC-ENG-001 §9，任务卡 T15）
    mode=void   作废：红色警示 + 原因 ≥5 字
    mode=revert 状态回退：仅允许退一步，原因必填（写入操作历史）
  -->
  <el-dialog
    v-model="visible"
    :title="isRevert ? '回退项目状态' : '作废工程'"
    width="480px"
    :close-on-click-modal="false"
    :before-close="handleBeforeClose"
  >
    <div class="eng-void-body">
      <p v-if="!isRevert" class="eng-warn">
        作废后不计入任何统计；已出库库存不会自动退回，需人工补录入库。
      </p>
      <p v-else class="eng-warn eng-warn--soft">
        回退后状态变为「{{ prevStatusLabel }}」，合同金额仍锁定，需重新走签约/供货流程。
      </p>

      <div class="eng-target">
        <span class="eng-target-no">{{ project?.project_no }}</span>
        <span class="eng-target-name">{{ project?.name }}</span>
        <EngineeringStatusTag v-if="project?.status" :status="project.status" />
      </div>

      <el-form label-position="top">
        <el-form-item :label="isRevert ? '回退原因' : '作废原因'" required>
          <el-input
            v-model="reason"
            type="textarea"
            :rows="3"
            maxlength="200"
            show-word-limit
            :placeholder="isRevert ? '如：甲方要求重新签约，金额需调整' : '如：甲方取消订单，已口头确认'"
          />
          <p v-if="reasonTooShort" class="eng-err">原因至少 5 个字</p>
        </el-form-item>
      </el-form>
    </div>

    <template #footer>
      <div class="flex gap-2">
        <el-button class="flex-1 md:flex-none" @click="visible = false">取消</el-button>
        <el-button
          class="flex-1 md:flex-none"
          :type="isRevert ? 'primary' : 'danger'"
          :loading="saving"
          :disabled="reasonTooShort"
          @click="submit"
        >
          {{ isRevert ? '确认回退' : '确认作废' }}
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { updateEngineeringStatus } from '../../api/engineering'
import EngineeringStatusTag from './EngineeringStatusTag.vue'

const props = defineProps({
  modelValue: { type: Boolean, required: true },
  project: { type: Object, default: null },
  mode: { type: String, default: 'void' } // 'void' | 'revert'
})

const emit = defineEmits(['update:modelValue', 'saved'])

const isRevert = computed(() => props.mode === 'revert')

// 回退只允许退一步：3 供货中 → 2 已签约；2 已签约 → 1 跟进中
const prevStatusLabel = computed(() => (Number(props.project?.status) === 3 ? '已签约' : '跟进中'))

const reason = ref('')
const saving = ref(false)

const visible = computed({
  get: () => props.modelValue,
  set: v => emit('update:modelValue', v)
})

const reasonTooShort = computed(() => reason.value.trim().length > 0 && reason.value.trim().length < 5)
const handleBeforeClose = done => {
  if (saving.value) return
  if (typeof done === 'function') done()
  else visible.value = false
}

watch(
  () => props.modelValue,
  open => {
    if (open) reason.value = ''
  }
)

const submit = async () => {
  const text = reason.value.trim()
  if (text.length < 5) return
  saving.value = true
  try {
    await updateEngineeringStatus(props.project.id, { action: isRevert.value ? 'revert' : 'void', reason: text })
    ElMessage.success(isRevert.value ? '状态已回退' : '项目已作废')
    visible.value = false
    emit('saved')
  } catch (e) {
    /* 提示由拦截器统一处理 */
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.eng-void-body {
  font-size: 13px;
}
.eng-warn {
  border-radius: 10px;
  border: 1px solid #fecaca;
  background: #fef2f2;
  padding: 10px 12px;
  font-size: 12px;
  line-height: 18px;
  color: #b91c1c;
}
.eng-warn--soft {
  border-color: #e2e8f0;
  background: #f8fafc;
  color: #475569;
}
.eng-target {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin: 14px 0;
}
.eng-target-no {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  color: #64748b;
}
.eng-target-name {
  font-weight: 600;
  color: #0f172a;
}
.eng-err {
  margin-top: 4px;
  font-size: 11px;
  color: #dc2626;
}
</style>
