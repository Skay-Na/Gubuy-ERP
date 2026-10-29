<template>
  <!--
    回款 Tab（SPEC-ENG-001 §5.5，任务卡 T13）
    上半：付款节点表（支持整表保存 + 快捷比例填充）
    下半：回款记录表（正数绿 / 冲销红，冲销只对正数记录开放）
  -->
  <div class="eng-stack">
    <!-- ===== 付款节点 ===== -->
    <div class="eng-panel">
      <div class="eng-panel-head">
        <div>
          <p class="eng-panel-title">付款节点</p>
          <p class="eng-panel-sub">
            节点合计必须等于合同总额
            <span v-if="!balanced" class="eng-unbalanced">
              · 当前差 {{ formatEngMoney(Math.abs(termsTotal - contractTotal)) }}
            </span>
          </p>
        </div>
        <div class="eng-panel-actions">
          <template v-if="!editing">
            <el-button v-if="canEdit" size="small" @click="startEdit">编辑节点</el-button>
            <el-button v-if="!isVoided" type="primary" size="small" @click="emit('add-receipt', null)">登记回款</el-button>
          </template>
          <template v-else>
            <el-button size="small" @click="cancelEdit">取消</el-button>
            <el-button type="primary" size="small" :loading="saving" @click="save">保存节点</el-button>
          </template>
        </div>
      </div>

      <!-- 快捷填充（仅编辑态） -->
      <div v-if="editing" class="eng-preset-bar">
        <span class="eng-preset-label">快捷填充</span>
        <el-button v-for="p in PRESETS" :key="p.label" size="small" text @click="applyPreset(p)">
          {{ p.label }}
        </el-button>
        <span class="eng-preset-tip">按合同总额换算金额，日期默认签约日 +0/+60/+120 天，可改</span>
      </div>

      <div v-if="editing" class="eng-edit-hint">编辑中，保存后会校验合计是否等于合同总额</div>

      <!-- 移动端卡片 -->
      <div v-if="isMobile && !editing" class="eng-mobile-list">
        <EngineeringMobileRow
          v-for="t in terms"
          :key="t.id"
          :title="t.name"
          :subtitle="`计划 ${formatEngDate(t.planned_date)}`"
          :amount="formatEngMoney(t.planned_amount)"
          :fields="[
            { label: '已收', value: formatEngMoney(t.received_amount) },
            { label: '未收', value: formatEngMoney(t.pending_amount) }
          ]"
        >
          <template #right>
            <span class="eng-term-tag" :style="termTagStyle(t)">{{ t.status_label }}</span>
          </template>
          <div class="eng-mobile-actions">
            <el-button
              v-if="Number(t.pending_amount) > 0 && !isVoided"
              size="small"
              text
              @click="emit('add-receipt', t)"
            >
              登记回款
            </el-button>
          </div>
        </EngineeringMobileRow>
        <p v-if="!terms.length" class="eng-mobile-empty">还没有付款节点，签约前需配置，且合计等于合同总额</p>
      </div>

      <el-table v-else :data="editing ? draft : terms" stripe class="eng-table">
        <el-table-column label="节点名称" min-width="150">
          <template #default="{ row }">
            <el-input v-if="editing" v-model="row.name" maxlength="50" placeholder="如 预付款" />
            <span v-else class="eng-strong">{{ row.name }}</span>
          </template>
        </el-table-column>

        <el-table-column label="计划比例" width="110" align="right">
          <template #default="{ row }">
            <el-input-number v-if="editing" v-model="row.ratio" :min="0" :max="100" :precision="2" :controls="false" class="w-full" />
            <span v-else>{{ row.ratio ? `${row.ratio}%` : '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="计划金额" width="140" align="right">
          <template #default="{ row }">
            <el-input-number v-if="editing" v-model="row.planned_amount" :min="0" :precision="2" :controls="false" class="w-full" />
            <span v-else class="eng-money">{{ formatEngMoney(row.planned_amount) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="计划日期" width="140" align="center">
          <template #default="{ row }">
            <el-date-picker
              v-if="editing"
              v-model="row.planned_date"
              type="date"
              value-format="YYYY-MM-DD"
              class="w-full"
            />
            <span v-else>{{ formatEngDate(row.planned_date) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="已收" width="130" align="right">
          <template #default="{ row }">
            <span class="eng-money" style="color: #15803d">{{ formatEngMoney(row.received_amount ?? 0) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="未收" width="130" align="right">
          <template #default="{ row }">
            <span
              class="eng-money"
              :style="Number(row.overdue_days) > 0 && Number(row.pending_amount) > 0 ? { color: '#dc2626', fontWeight: 600 } : null"
            >
              {{ formatEngMoney(row.pending_amount ?? planPending(row)) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <span class="eng-term-tag" :style="termTagStyle(row)">{{ row.status_label || termStatusLabel(row) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="110" align="right">
          <template #default="{ row, $index }">
            <template v-if="editing">
              <el-button
                link
                type="danger"
                size="small"
                :disabled="Number(row.received_amount) > 0"
                @click="removeRow($index)"
              >
                删除
              </el-button>
            </template>
            <el-button
              v-else-if="Number(row.pending_amount) > 0 && !isVoided"
              link
              type="primary"
              size="small"
              @click="emit('add-receipt', row)"
            >
              登记回款
            </el-button>
            <span v-else class="eng-dim">—</span>
          </template>
        </el-table-column>

        <template #empty>
          <div class="eng-empty">
            <p class="eng-empty-title">还没有付款节点</p>
            <p class="eng-empty-sub">签约前需配置付款节点，且合计等于合同总额</p>
            <el-button v-if="canEdit" type="primary" size="small" class="mt-3" @click="startEdit">添加节点</el-button>
            <el-button v-if="canEdit" size="small" class="mt-3" @click="startEditWithPreset(PRESETS[2])">
              用 50/50 填充
            </el-button>
          </div>
        </template>
      </el-table>

      <div v-if="!isMobile || editing" class="eng-sum-line">
        计划合计 <b>{{ formatEngMoney(currentTermsTotal) }}</b>
        <span class="eng-dot">·</span>合同总额 {{ formatEngMoney(contractTotal) }}
        <span v-if="!currentBalanced" class="eng-diff">
          <span class="eng-dot">·</span>与合同总额差 {{ formatEngMoney(Math.abs(currentTermsTotal - contractTotal)) }}
        </span>
      </div>
    </div>

    <!-- ===== 回款记录 ===== -->
    <div class="eng-panel">
      <div class="eng-panel-head">
        <div>
          <p class="eng-panel-title">回款记录</p>
          <p class="eng-panel-sub">冲销不删原记录，生成一条负数流水，操作可追溯</p>
        </div>
      </div>

      <div v-if="isMobile" class="eng-mobile-list">
        <EngineeringMobileRow
          v-for="r in receipts"
          :key="r.id"
          :title="r.term_name || '未关联节点'"
          :subtitle="`${formatEngDate(r.payment_date)} · ${r.method_label} · ${r.operator_name}`"
          :amount="formatEngMoney(r.amount)"
          :amount-style="Number(r.amount) < 0 ? { color: '#dc2626' } : { color: '#15803d' }"
        >
          <p v-if="r.remark" class="eng-mobile-note">{{ r.remark }}</p>
          <div v-if="!r.is_reversal && Number(r.amount) > 0 && !isVoided" class="eng-mobile-actions">
            <el-button size="small" text @click="emit('reverse-receipt', r)">冲销</el-button>
          </div>
        </EngineeringMobileRow>
        <p v-if="!receipts.length" class="eng-mobile-empty">暂无回款记录，到账后在这里登记</p>
      </div>

      <el-table v-else :data="receipts" stripe class="eng-table">
        <el-table-column label="到账日期" width="120">
          <template #default="{ row }">{{ formatEngDate(row.payment_date) }}</template>
        </el-table-column>
        <el-table-column label="付款节点" min-width="130" prop="term_name" />
        <el-table-column label="金额" width="140" align="right">
          <template #default="{ row }">
            <span class="eng-money" :style="Number(row.amount) < 0 ? { color: '#dc2626', fontWeight: 600 } : { color: '#15803d' }">
              {{ formatEngMoney(row.amount) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="收款方式" width="110" prop="method_label" />
        <el-table-column label="票据号" width="120">
          <template #default="{ row }">
            <span class="eng-mono">{{ row.bill_no || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="备注" min-width="130">
          <template #default="{ row }">
            <span class="eng-ellipsis" :title="row.remark">{{ row.remark || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作人" width="100">
          <template #default="{ row }">{{ row.operator_name || '—' }}</template>
        </el-table-column>
        <el-table-column label="登记时间" width="120">
          <template #default="{ row }">{{ shortTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="80" align="right">
          <template #default="{ row }">
            <el-button
              v-if="!row.is_reversal && Number(row.amount) > 0 && !isVoided"
              link
              type="danger"
              size="small"
              @click="emit('reverse-receipt', row)"
            >
              冲销
            </el-button>
            <span v-else class="eng-dim">—</span>
          </template>
        </el-table-column>
        <template #empty>
          <div class="eng-empty">
            <p class="eng-empty-title">暂无回款记录</p>
            <p class="eng-empty-sub">到账后在这里登记</p>
          </div>
        </template>
      </el-table>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import EngineeringMobileRow from './EngineeringMobileRow.vue'
import { formatEngDate, formatEngMoney } from '../../utils/engineeringFormat'

const props = defineProps({
  terms: { type: Array, default: () => [] },
  receipts: { type: Array, default: () => [] },
  contractTotal: { type: Number, default: 0 },
  signDate: { type: String, default: '' },
  status: { type: Number, default: 1 },
  saving: { type: Boolean, default: false }
})

const emit = defineEmits(['save-terms', 'add-receipt', 'reverse-receipt'])

const isMobile = ref(window.innerWidth < 768)
window.addEventListener('resize', () => {
  isMobile.value = window.innerWidth < 768
})

const isVoided = computed(() => Number(props.status) === 9)
// 验收后金额锁定，节点不可再改
const canEdit = computed(() => [1, 2, 3].includes(Number(props.status)))

const editing = ref(false)
const draft = ref([])

const PRESETS = [
  { label: '30/60/10', ratios: [30, 60, 10], names: ['预付款', '到货款', '质保金'], days: [0, 60, 365] },
  { label: '30/50/20', ratios: [30, 50, 20], names: ['预付款', '发货款', '验收款'], days: [0, 60, 120] },
  { label: '50/50', ratios: [50, 50], names: ['预付款', '尾款'], days: [0, 60] }
]

const termsTotal = computed(() => props.terms.reduce((s, t) => s + (Number(t.planned_amount) || 0), 0))
const balanced = computed(() => Math.abs(termsTotal.value - Number(props.contractTotal)) < 0.01)

const currentTermsTotal = computed(() =>
  editing.value ? draft.value.reduce((s, t) => s + (Number(t.planned_amount) || 0), 0) : termsTotal.value
)
const currentBalanced = computed(() => Math.abs(currentTermsTotal.value - Number(props.contractTotal)) < 0.01)

const planPending = row => Math.max(0, (Number(row.planned_amount) || 0) - (Number(row.received_amount) || 0))

const termStatusLabel = row => {
  const planned = Number(row.planned_amount) || 0
  const received = Number(row.received_amount) || 0
  if (received <= 0) return '未收'
  if (received < planned) return '部分收'
  return '已收'
}

const termTagStyle = row => {
  const label = row.status_label || termStatusLabel(row)
  if (label === '逾期' || (Number(row.overdue_days) > 0 && Number(row.pending_amount) > 0)) {
    return { color: '#dc2626', background: '#fef2f2' }
  }
  if (label === '已收') return { color: '#15803d', background: '#e8f7ee' }
  if (label === '部分收') return { color: '#d97706', background: '#fffbeb' }
  return { color: '#64748b', background: '#f1f5f9' }
}

const shortTime = value => {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '—'
  const pad = n => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

const addDays = (base, days) => {
  const d = base ? new Date(base) : new Date()
  if (Number.isNaN(d.getTime())) return ''
  const t = new Date(d.getTime() + days * 86400000)
  const pad = n => String(n).padStart(2, '0')
  return `${t.getFullYear()}-${pad(t.getMonth() + 1)}-${pad(t.getDate())}`
}

const cloneRows = () =>
  props.terms.map(t => ({
    id: t.id,
    name: t.name,
    ratio: t.ratio === null ? 0 : Number(t.ratio),
    planned_amount: Number(t.planned_amount),
    planned_date: t.planned_date || null,
    received_amount: Number(t.received_amount) || 0
  }))

const startEdit = () => {
  draft.value = cloneRows()
  editing.value = true
}

const applyPreset = preset => {
  const total = Number(props.contractTotal) || 0
  if (total <= 0) {
    ElMessage.warning('合同总额为 0，请先在设备清单里维护商品与单价')
    return
  }
  draft.value = preset.ratios.map((ratio, i) => ({
    id: 0,
    name: preset.names[i],
    ratio,
    planned_amount: Math.round(((total * ratio) / 100) * 100) / 100,
    planned_date: addDays(props.signDate, preset.days[i]),
    received_amount: 0
  }))
}

const startEditWithPreset = preset => {
  startEdit()
  applyPreset(preset)
}

const cancelEdit = () => {
  editing.value = false
  draft.value = []
}

const removeRow = index => {
  draft.value.splice(index, 1)
}

const save = () => {
  if (!draft.value.length) {
    ElMessage.warning('至少保留 1 个付款节点')
    return
  }
  const invalid = draft.value.find(t => !String(t.name || '').trim() || Number(t.planned_amount) <= 0)
  if (invalid) {
    ElMessage.warning('请填写节点名称，且计划金额必须大于 0')
    return
  }
  emit('save-terms', {
    terms: draft.value.map(t => ({
      id: Number(t.id) || 0,
      name: String(t.name).trim(),
      ratio: Number(t.ratio) || 0,
      planned_amount: Number(t.planned_amount),
      planned_date: t.planned_date || null
    }))
  })
}

const finishSave = () => {
  editing.value = false
  draft.value = []
}

defineExpose({ finishSave })
</script>

<style scoped>
.eng-stack {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.eng-panel {
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 16px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.eng-panel-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}
.eng-panel-title {
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
}
.eng-panel-sub {
  margin-top: 2px;
  font-size: 11px;
  color: #94a3b8;
}
.eng-unbalanced {
  color: #dc2626;
}
.eng-panel-actions {
  display: flex;
  gap: 8px;
  flex: 0 0 auto;
}
.eng-preset-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-bottom: 10px;
  border-radius: 10px;
  background: #f8fafc;
  padding: 8px 12px;
}
.eng-preset-label {
  font-size: 11px;
  color: #64748b;
}
.eng-preset-tip {
  margin-left: auto;
  font-size: 11px;
  color: #cbd5e1;
}
.eng-edit-hint {
  margin-bottom: 10px;
  border-radius: 8px;
  background: #eef0ff;
  padding: 8px 12px;
  font-size: 12px;
  color: #635bff;
}
.eng-sum-line {
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px dashed #f1f5f9;
  text-align: right;
  font-size: 12px;
  color: #64748b;
}
.eng-sum-line b {
  color: #0f172a;
}
.eng-diff {
  color: #dc2626;
}
.eng-dot {
  margin: 0 6px;
  color: #cbd5e1;
}
.eng-strong {
  font-weight: 600;
  color: #0f172a;
}
.eng-money {
  font-variant-numeric: tabular-nums;
  color: #334155;
}
.eng-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  color: #64748b;
}
.eng-dim {
  color: #cbd5e1;
}
.eng-ellipsis {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #64748b;
}
.eng-term-tag {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  line-height: 18px;
}
.eng-empty {
  padding: 30px 12px;
  text-align: center;
}
.eng-empty-title {
  font-size: 14px;
  font-weight: 600;
  color: #334155;
}
.eng-empty-sub {
  margin-top: 4px;
  font-size: 12px;
  color: #94a3b8;
}
.eng-mobile-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.eng-mobile-empty {
  padding: 18px;
  text-align: center;
  font-size: 12px;
  color: #94a3b8;
}
.eng-mobile-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
  margin-top: 6px;
}
.eng-mobile-note {
  margin-top: 6px;
  font-size: 11px;
  color: #94a3b8;
}
</style>
