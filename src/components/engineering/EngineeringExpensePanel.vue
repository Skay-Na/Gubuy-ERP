<template>
  <!--
    项目详情 · 费用 Tab（SPEC-ENG-001 §5.7，任务卡 T14）
    工程费用独立成账：这里只统计"本项目"的支出，公共费用不摊入（口径见工程开销台账）。
  -->
  <div class="eng-stack">
    <EngineeringExpenseSummary
      :summary="summarySafe"
      :by-category="byCategory"
      :show-year="false"
      :active-category="activeCategory"
      @pick-category="c => (activeCategory = c)"
    >
      <template #actions>
        <el-button
          type="primary"
          size="small"
          :disabled="isVoided"
          :title="isVoided ? '项目已作废，不可新增费用' : ''"
          @click="openCreate"
        >
          录入费用
        </el-button>
      </template>
    </EngineeringExpenseSummary>

    <!-- PC 表格 -->
    <div v-if="!isMobile" class="eng-panel">
      <el-table :data="filtered" stripe class="eng-table">
        <el-table-column label="发生日期" width="100">
          <template #default="{ row }">{{ formatEngDate(row.expense_date) }}</template>
        </el-table-column>
        <el-table-column label="类别" width="110">
          <template #default="{ row }">
            <EngineeringExpenseTag :category="row.category" />
          </template>
        </el-table-column>
        <el-table-column label="金额" width="120" align="right">
          <template #default="{ row }">
            <span class="eng-money eng-money--strong">{{ formatEngMoney(row.amount) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="付款状态" width="88" align="center">
          <template #default="{ row }">
            <span class="eng-pay-tag" :style="payStyle(row.pay_status)">{{ row.pay_status_label || payLabel(row.pay_status) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="付款方式" width="84">
          <template #default="{ row }">
            {{ row.pay_status === 1 ? row.pay_method_label || '—' : '—' }}
          </template>
        </el-table-column>
        <el-table-column label="收款方" width="110">
          <template #default="{ row }">{{ row.payee || '—' }}</template>
        </el-table-column>
        <el-table-column label="票据号" width="100">
          <template #default="{ row }">
            <span class="eng-mono">{{ row.bill_no || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="备注" min-width="140">
          <template #default="{ row }">
            <span class="eng-ellipsis" :title="row.remark">{{ row.remark || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作人" width="90">
          <template #default="{ row }">{{ row.operator_name || '—' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="110" align="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" :disabled="isVoided" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" size="small" :disabled="isVoided" @click="confirmDelete(row)">删除</el-button>
          </template>
        </el-table-column>

        <template #empty>
          <div class="eng-empty">
            <p class="eng-empty-title">还没有录入工程费用</p>
            <p class="eng-empty-sub">安装、运输、辅材这些实际支出记在这里，录完才能看到真实净利</p>
            <el-button v-if="!isVoided" type="primary" size="small" class="mt-3" @click="openCreate">
              录入第一笔费用
            </el-button>
          </div>
        </template>
      </el-table>
      <div class="eng-sum-line">合计 <b>{{ formatEngMoney(summarySafe.total) }}</b></div>
    </div>

    <!-- 移动端卡片 -->
    <div v-else class="eng-mobile-list">
      <div v-for="row in filtered" :key="row.id" class="eng-mobile-card" @click="isVoided ? null : openEdit(row)">
        <div class="eng-mobile-head">
          <EngineeringExpenseTag :category="row.category" />
          <span class="eng-money eng-money--strong">{{ formatEngMoney(row.amount) }}</span>
        </div>
        <p class="eng-mobile-sub">
          {{ formatEngDate(row.expense_date) }} · {{ row.pay_status_label || payLabel(row.pay_status) }} ·
          {{ row.pay_status === 1 ? row.pay_method_label || '—' : '—' }} · {{ row.payee || '—' }}
        </p>
      </div>
      <div v-if="!filtered.length" class="eng-mobile-empty">
        还没有录入工程费用
        <el-button v-if="!isVoided" type="primary" size="small" class="mt-2" @click="openCreate">录入第一笔费用</el-button>
      </div>
    </div>

    <EngineeringExpenseDialog
      v-model="dialogVisible"
      :project-id="projectId"
      :expense="editingExpense"
      @saved="onSaved"
    />
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteEngineeringExpense } from '../../api/engineering'
import EngineeringExpenseSummary from './EngineeringExpenseSummary.vue'
import EngineeringExpenseTag from './EngineeringExpenseTag.vue'
import EngineeringExpenseDialog from './EngineeringExpenseDialog.vue'
import { formatEngDate, formatEngMoney } from '../../utils/engineeringFormat'

const props = defineProps({
  projectId: { type: Number, required: true },
  expenses: { type: Array, default: () => [] },
  expenseSummary: { type: Object, default: null },
  status: { type: Number, default: 1 }
})

const emit = defineEmits(['changed'])

const isMobile = ref(window.innerWidth < 768)
window.addEventListener('resize', () => {
  isMobile.value = window.innerWidth < 768
})

const isVoided = computed(() => Number(props.status) === 9)

const summarySafe = computed(() => ({
  total: Number(props.expenseSummary?.total) || 0,
  paid: Number(props.expenseSummary?.paid) || 0,
  unpaid: Number(props.expenseSummary?.unpaid) || 0,
  year_total: 0,
  count: Number(props.expenseSummary?.count) || 0
}))

const byCategory = computed(() => props.expenseSummary?.by_category || [])

const activeCategory = ref(null)
const filtered = computed(() =>
  activeCategory.value ? props.expenses.filter(e => Number(e.category) === activeCategory.value) : props.expenses
)

const payLabel = status => (Number(status) === 1 ? '已付' : '未付')
const payStyle = status =>
  Number(status) === 1 ? { color: '#15803d', background: '#e8f7ee' } : { color: '#d97706', background: '#fffbeb' }

const dialogVisible = ref(false)
const editingExpense = ref(null)

const openCreate = () => {
  editingExpense.value = null
  dialogVisible.value = true
}

const openEdit = row => {
  editingExpense.value = row
  dialogVisible.value = true
}

const onSaved = () => {
  dialogVisible.value = false
  emit('changed')
}

const confirmDelete = async row => {
  try {
    await ElMessageBox.confirm(
      `删除这笔 ${formatEngMoney(row.amount)} 的${row.category_label || '费用'}？删除后项目净利会重算，操作记录可追溯。`,
      '删除工程费用',
      { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消', confirmButtonClass: 'el-button--danger' }
    )
  } catch (e) {
    return
  }
  try {
    await deleteEngineeringExpense(row.id)
    ElMessage.success('费用已删除')
    emit('changed')
  } catch (e) {
    /* 提示由拦截器处理 */
  }
}
</script>

<style scoped>
.eng-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.eng-panel {
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 14px 16px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.eng-sum-line {
  margin-top: 10px;
  padding-top: 9px;
  border-top: 1px dashed #f1f5f9;
  text-align: right;
  font-size: 12px;
  color: #64748b;
}
.eng-sum-line b {
  color: #0f172a;
  font-size: 13px;
}
.eng-money {
  font-variant-numeric: tabular-nums;
  color: #334155;
}
.eng-money--strong {
  font-weight: 600;
  color: #0f172a;
}
.eng-pay-tag {
  display: inline-flex;
  border-radius: 999px;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  line-height: 18px;
}
.eng-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  color: #64748b;
}
.eng-ellipsis {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #64748b;
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
.eng-mobile-card {
  border-radius: 14px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 12px 14px;
  cursor: pointer;
}
.eng-mobile-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.eng-mobile-sub {
  margin-top: 6px;
  font-size: 11px;
  color: #94a3b8;
}
.eng-mobile-empty {
  padding: 24px;
  text-align: center;
  font-size: 12px;
  color: #94a3b8;
}
</style>
