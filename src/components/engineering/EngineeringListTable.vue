<template>
  <!--
    工程列表 PC 表格（SPEC-ENG-001 §4.5）
    逾期三重信号：行首 3px 红条（row-style）+ 待回款红字带"逾期 N 天" + 状态区红点/红字
  -->
  <div class="eng-table-wrap">
    <el-table
      v-loading="loading"
      :data="list"
      stripe
      :row-style="rowStyle"
      row-key="id"
      :default-sort="defaultSort"
      class="eng-table"
      @row-click="row => emit('row-click', row)"
      @sort-change="e => emit('sort-change', e)"
    >
      <!-- 项目编号 -->
      <el-table-column label="项目编号" width="112">
        <template #default="{ row }">
          <span class="eng-no" :title="row.project_no" @click.stop="copyNo(row.project_no)">{{ row.project_no }}</span>
        </template>
      </el-table-column>

      <!-- 项目名称 -->
      <el-table-column label="项目名称" min-width="160">
        <template #default="{ row }">
          <div class="eng-name" @click.stop="emit('view', row)">
            <span class="eng-name-main">{{ row.name }}</span>
            <span class="eng-name-sub">{{ row.project_type_label || '—' }}</span>
          </div>
        </template>
      </el-table-column>

      <!-- 甲方单位 -->
      <el-table-column label="甲方单位" width="112">
        <template #default="{ row }">
          <div class="eng-cell-stack">
            <span class="eng-cell-strong">{{ row.client_name || '—' }}</span>
            <span class="eng-cell-sub">
              {{ [row.client_contact, row.client_phone].filter(Boolean).join(' · ') || '—' }}
            </span>
          </div>
        </template>
      </el-table-column>

      <!-- 负责人 -->
      <el-table-column label="负责人" width="58">
        <template #default="{ row }">
          <span class="eng-cell-strong">{{ row.owner_name || '—' }}</span>
        </template>
      </el-table-column>

      <!-- 合同总额 -->
      <el-table-column label="合同总额" width="100" align="right" prop="contract_total" sortable="custom">
        <template #default="{ row }">
          <div class="eng-cell-stack eng-cell-stack--right">
            <span class="eng-money">
              {{ contractMoney(row) }}
              <span v-if="isEstimated(row)" class="eng-estimated">(预估)</span>
            </span>
            <span class="eng-cell-sub" :style="marginStyle(row)">{{ marginText(row) }}</span>
          </div>
        </template>
      </el-table-column>

      <!-- 已回款 -->
      <el-table-column label="已回款" width="88" align="right">
        <template #default="{ row }">
          <span class="eng-money eng-money--received">{{ formatEngMoney(row.received_amount) }}</span>
        </template>
      </el-table-column>

      <!-- 待回款 -->
      <el-table-column label="待回款" width="88" align="right" prop="pending_amount" sortable="custom">
        <template #default="{ row }">
          <div class="eng-cell-stack eng-cell-stack--right">
            <span class="eng-money" :style="Number(row.pending_amount) > 0 ? { color: '#dc2626' } : null">
              {{ formatEngMoney(row.pending_amount) }}
            </span>
            <span v-if="Number(row.overdue_days) > 0" class="eng-overdue-text">逾期 {{ row.overdue_days }} 天</span>
          </div>
        </template>
      </el-table-column>

      <!-- 回款进度 -->
      <el-table-column label="回款进度" width="92">
        <template #default="{ row }">
          <div class="eng-progress">
            <el-progress
              :percentage="Math.min(100, Math.max(0, Number(row.received_ratio) || 0))"
              :stroke-width="6"
              :show-text="false"
              :color="progressColor(row)"
            />
            <span class="eng-progress-text">{{ Number(row.received_ratio) || 0 }}%</span>
          </div>
        </template>
      </el-table-column>

      <!-- 状态 -->
      <el-table-column label="状态" width="68" align="center">
        <template #default="{ row }">
          <EngineeringStatusTag :status="row.status" />
        </template>
      </el-table-column>

      <!-- 下次回款（列窄，用 4 字标题避免表头折行） -->
      <el-table-column label="下次回款" width="88" align="center" prop="next_due_date" sortable="custom">
        <template #default="{ row }">
          <span :style="Number(row.overdue_days) > 0 ? { color: '#dc2626', fontWeight: 600 } : null">
            {{ formatEngDate(row.next_due_date) }}
          </span>
        </template>
      </el-table-column>

      <!-- 操作 -->
      <el-table-column label="操作" width="88" align="right" fixed="right">
        <template #default="{ row }">
          <div class="eng-actions" @click.stop>
            <el-button link type="primary" size="small" @click="emit('view', row)">查看</el-button>
            <el-dropdown trigger="click" @command="cmd => emit(cmd, row)">
              <span class="eng-more">
                更多
                <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
                  <path d="M2 4l3 3 3-3" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" />
                </svg>
              </span>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="edit" :disabled="row.status !== 1">编辑</el-dropdown-item>
                  <el-dropdown-item command="void" :disabled="row.status === 9">作废</el-dropdown-item>
                  <el-dropdown-item
                    command="delete"
                    :disabled="row.status !== 1"
                    divided
                  >
                    删除
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </template>
      </el-table-column>

      <!-- 空态：区分"一条都没有"与"筛选没结果"（§3.6） -->
      <template #empty>
        <div class="eng-empty">
          <p class="eng-empty-title">{{ hasActiveFilter ? '没有符合条件的项目' : '还没有工程项目' }}</p>
          <p class="eng-empty-sub">
            {{ hasActiveFilter ? '试试放宽筛选条件' : '工程机批量供货的合同、清单、回款都在这里管' }}
          </p>
          <el-button v-if="hasActiveFilter" class="eng-empty-btn" @click="emit('reset')">清空筛选</el-button>
          <el-button v-else type="primary" class="eng-empty-btn" @click="emit('create')">新建第一个工程</el-button>
        </div>
      </template>
    </el-table>
  </div>
</template>

<script setup>
import { ElMessage } from 'element-plus'
import EngineeringStatusTag from './EngineeringStatusTag.vue'
import { formatEngDate, formatEngMoney } from '../../utils/engineeringFormat'

const props = defineProps({
  list: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  hasActiveFilter: { type: Boolean, default: false },
  sortBy: { type: String, default: 'updated_at' },
  order: { type: String, default: 'desc' }
})

const emit = defineEmits(['row-click', 'view', 'edit', 'void', 'delete', 'sort-change', 'reset', 'create'])

// el-table 的表头箭头状态要跟 URL 里的排序保持一致（从详情返回时也正确）
const defaultSort = {
  prop: props.sortBy === 'updated_at' ? null : props.sortBy,
  order: props.order === 'asc' ? 'ascending' : 'descending'
}

// 逾期行：左侧 3px 红条（§3.3 信号 2）
const rowStyle = ({ row }) => (Number(row.overdue_days) > 0 ? { boxShadow: 'inset 3px 0 #dc2626' } : null)

// 状态 1 且无合同额 → 展示预估额并标注（§4.5）
const isEstimated = row => Number(row.status) === 1 && Number(row.contract_total) <= 0
const contractMoney = row => formatEngMoney(isEstimated(row) ? row.estimated_amount : row.contract_total)

// 合同额下方净利率：未签约（状态 1）显示 — （预估本身没有成本口径）
const marginText = row => {
  if (Number(row.status) === 1) return '净利率 —'
  return `净利率 ${Number(row.net_margin_rate).toFixed(2)}%`
}
const marginStyle = row => (Number(row.status) === 1 ? null : Number(row.net_margin_rate) < 0 ? { color: '#dc2626' } : null)

// 进度条颜色（§3.4）：100% 绿 / 有逾期红 / 其余品牌紫
const progressColor = row => {
  if (Number(row.received_ratio) >= 100) return '#15803d'
  if (Number(row.overdue_days) > 0) return '#dc2626'
  return '#635BFF'
}

const copyNo = async projectNo => {
  try {
    await navigator.clipboard.writeText(projectNo)
    ElMessage.success(`已复制 ${projectNo}`)
  } catch (e) {
    ElMessage.warning('复制失败，请手动选中复制')
  }
}
</script>

<style scoped>
.eng-table-wrap {
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  background: #fff;
  overflow: hidden;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.eng-table {
  --el-table-border-color: #f1f5f9;
  --el-table-header-bg-color: #f8fafc;
  --el-table-row-hover-bg-color: #f8faff;
  font-size: 13px;
}
/* Element Plus 单元格左右各 12px padding，11 列会白白吃掉 260px，
   工程表格列多，统一收到 8px，给数据列留出可读宽度 */
.eng-table :deep(.el-table__cell .cell) {
  padding-left: 6px;
  padding-right: 6px;
}
.eng-table :deep(th.el-table__cell) {
  font-size: 12px;
  font-weight: 600;
  color: #64748b;
}
.eng-no {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  white-space: nowrap;
  color: #475569;
  cursor: pointer;
  border-bottom: 1px dashed transparent;
}
.eng-no:hover {
  color: #635bff;
  border-bottom-color: #c7c2ff;
}
.eng-name {
  cursor: pointer;
}
.eng-name-main {
  display: block;
  font-weight: 600;
  color: #0f172a;
  line-height: 20px;
}
.eng-name-main:hover {
  color: #635bff;
}
.eng-name-sub {
  display: block;
  font-size: 11px;
  color: #94a3b8;
  line-height: 16px;
}
.eng-cell-stack {
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.eng-cell-stack--right {
  align-items: flex-end;
}
.eng-cell-strong {
  color: #0f172a;
  line-height: 20px;
}
.eng-cell-sub {
  font-size: 11px;
  color: #94a3b8;
  line-height: 16px;
}
.eng-money {
  color: #0f172a;
  font-variant-numeric: tabular-nums;
  line-height: 20px;
}
.eng-money--received {
  color: #15803d;
}
.eng-estimated {
  font-size: 11px;
  color: #94a3b8;
  font-weight: 400;
}
.eng-overdue-text {
  font-size: 11px;
  font-weight: 600;
  color: #dc2626;
  line-height: 16px;
}
.eng-progress {
  display: flex;
  align-items: center;
  gap: 6px;
}
.eng-progress :deep(.el-progress) {
  flex: 1;
  min-width: 44px;
}
.eng-progress-text {
  font-size: 11px;
  color: #64748b;
  font-variant-numeric: tabular-nums;
}
.eng-actions {
  display: inline-flex;
  align-items: center;
  gap: 0;
  white-space: nowrap;
}
.eng-actions :deep(.el-button) {
  font-size: 12px;
  padding: 0 3px;
}
.eng-more {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 12px;
  color: #64748b;
  cursor: pointer;
  padding: 0 4px;
}
.eng-more:hover {
  color: #635bff;
}
.eng-empty {
  padding: 40px 12px;
  text-align: center;
}
.eng-empty-title {
  font-size: 14px;
  font-weight: 600;
  color: #334155;
}
.eng-empty-sub {
  margin-top: 6px;
  font-size: 12px;
  color: #94a3b8;
}
.eng-empty-btn {
  margin-top: 14px;
}
</style>
