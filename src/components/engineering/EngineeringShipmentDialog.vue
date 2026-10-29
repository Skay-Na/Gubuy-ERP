<template>
  <!--
    工程出库弹窗（SPEC-ENG-001 §8，任务卡 T15）
    上半选待发行，下半填出库信息。
    库存不足时后端会拦下（400），本弹窗捕获这条 msg 后显示红色提示条 +
    「允许负库存出库」勾选框，用户确认后带 allow_negative=true 重提 —— 与设计稿一致，
    且不需要前端自己算库存（库存字段来自商品，工程清单行里没有）。
  -->
  <el-dialog v-model="visible" title="工程出库" :width="isMobile ? '96%' : '720px'" :close-on-click-modal="false">
    <div class="eng-ship-body">
      <!-- 待发行 -->
      <div class="eng-ship-lines">
        <div class="eng-ship-line" v-for="line in draft" :key="line.item_id">
          <el-checkbox v-model="line.checked" :disabled="line.remain <= 0" @change="v => onCheck(line, v)" />
          <div class="eng-ship-line-main">
            <p class="eng-ship-line-name">{{ line.product_name }}</p>
            <p class="eng-ship-line-sub">订购 {{ line.quantity }} · 已发 {{ line.shipped_qty }} · 待发 {{ line.remain }}</p>
          </div>
          <div class="eng-ship-line-qty">
            <el-input-number
              v-model="line.qty"
              :min="0"
              :max="line.remain"
              :precision="0"
              :disabled="!line.checked"
              :controls="false"
              class="w-full"
            />
            <span class="eng-ship-unit">台</span>
          </div>
        </div>
        <p v-if="!draft.length" class="eng-ship-empty">该项目的设备已全部出库，无需再发货</p>
      </div>

      <!-- 出库信息 -->
      <el-form label-position="top" class="eng-ship-form">
        <div class="eng-grid">
          <el-form-item label="发货仓">
            <el-radio-group v-model="warehouse">
              <el-radio-button :value="2">门店出库</el-radio-button>
              <el-radio-button :value="3">云仓代发</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="出库日期">
            <el-date-picker
              v-model="shipDate"
              type="date"
              value-format="YYYY-MM-DD"
              :disabled-date="date => date.getTime() > Date.now()"
              class="w-full"
            />
          </el-form-item>
        </div>
        <div class="eng-grid">
          <el-form-item label="物流单号">
            <el-input v-model="trackingNo" maxlength="100" placeholder="选填" />
          </el-form-item>
          <el-form-item label="备注">
            <el-input v-model="remark" maxlength="200" placeholder="选填" />
          </el-form-item>
        </div>
      </el-form>

      <!-- 提示条 -->
      <div v-if="stockWarning" class="eng-ship-warn">
        <p>{{ stockWarning }}</p>
        <el-checkbox v-model="allowNegative">我确认允许负库存出库，事后补货</el-checkbox>
      </div>
      <div v-else class="eng-ship-tip">
        本次出库 {{ totalQty }} 台，涉及 {{ checkedCount }} 个商品
      </div>
    </div>

    <template #footer>
      <div class="flex gap-2">
        <el-button class="flex-1 md:flex-none" @click="visible = false">取消</el-button>
        <el-button
          class="flex-1 md:flex-none"
          type="primary"
          :loading="saving"
          :disabled="!canSubmit"
          @click="submit"
        >
          确认出库
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'

const props = defineProps({
  modelValue: { type: Boolean, required: true },
  items: { type: Array, default: () => [] },
  saving: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue', 'confirm'])

const isMobile = ref(window.innerWidth < 768)
window.addEventListener('resize', () => {
  isMobile.value = window.innerWidth < 768
})

const today = () => {
  const d = new Date()
  const pad = n => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

const draft = ref([])
const warehouse = ref(2)
const shipDate = ref(today())
const trackingNo = ref('')
const remark = ref('')
const stockWarning = ref('')
const allowNegative = ref(false)

const visible = computed({
  get: () => props.modelValue,
  set: v => emit('update:modelValue', v)
})

const onCheck = (line, checked) => {
  line.qty = checked ? line.remain : 0
}

const checkedCount = computed(() => draft.value.filter(l => l.checked && l.qty > 0).length)
const totalQty = computed(() => draft.value.reduce((sum, l) => sum + (l.checked ? Number(l.qty) || 0 : 0), 0))
const canSubmit = computed(() => totalQty.value > 0 && (!stockWarning.value || allowNegative.value))

watch(
  () => props.modelValue,
  open => {
    if (!open) return
    stockWarning.value = ''
    allowNegative.value = false
    shipDate.value = today()
    trackingNo.value = ''
    remark.value = ''
    // 默认勾选所有"未发完"行并填满待发数（§8）
    draft.value = props.items
      .map(it => {
        const remain = Math.max(0, (Number(it.quantity) || 0) - (Number(it.shipped_qty) || 0))
        return {
          item_id: it.id,
          product_name: it.product_name,
          quantity: Number(it.quantity) || 0,
          shipped_qty: Number(it.shipped_qty) || 0,
          remain,
          checked: remain > 0,
          qty: remain
        }
      })
      .filter(l => l.remain > 0)
  }
)

const submit = () => {
  if (!canSubmit.value) return
  emit('confirm', {
    warehouse_type: Number(warehouse.value),
    ship_date: shipDate.value,
    tracking_no: (trackingNo.value || '').trim(),
    remark: (remark.value || '').trim(),
    allow_negative: !!allowNegative.value,
    lines: draft.value
      .filter(l => l.checked && Number(l.qty) > 0)
      .map(l => ({ item_id: l.item_id, quantity: Number(l.qty) }))
  })
}

// 父层在出库失败时把"库存不足"的 msg 回传，这里切成确认态（§8 红色提示条）
const showStockWarning = message => {
  stockWarning.value = message || '库存不足，如需继续请确认负库存出库'
  allowNegative.value = false
}
const close = () => {
  visible.value = false
}

defineExpose({ close, showStockWarning })
</script>

<style scoped>
.eng-ship-body {
  font-size: 13px;
}
.eng-ship-lines {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 320px;
  overflow-y: auto;
  margin-bottom: 14px;
}
.eng-ship-line {
  display: flex;
  align-items: center;
  gap: 10px;
  border-radius: 10px;
  border: 1px solid #f1f5f9;
  padding: 10px 12px;
}
.eng-ship-line-main {
  flex: 1;
  min-width: 0;
}
.eng-ship-line-name {
  font-weight: 600;
  color: #0f172a;
  line-height: 19px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.eng-ship-line-sub {
  font-size: 11px;
  color: #94a3b8;
  line-height: 16px;
}
.eng-ship-line-qty {
  display: flex;
  align-items: center;
  gap: 4px;
  width: 110px;
  flex: 0 0 auto;
}
.eng-ship-unit {
  font-size: 11px;
  color: #94a3b8;
}
.eng-ship-empty {
  padding: 24px;
  text-align: center;
  font-size: 12px;
  color: #94a3b8;
}
.eng-ship-form {
  --el-font-size-base: 13px;
}
.eng-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 14px;
}
.eng-ship-tip {
  border-radius: 8px;
  background: #f8fafc;
  padding: 8px 12px;
  font-size: 12px;
  color: #64748b;
}
.eng-ship-warn {
  border-radius: 8px;
  border: 1px solid #fecaca;
  background: #fef2f2;
  padding: 10px 12px;
  font-size: 12px;
  color: #b91c1c;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
@media (max-width: 767px) {
  .eng-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
