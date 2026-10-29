<template>
  <!--
    工程列表移动端卡片流（SPEC-ENG-001 §4.6，< 768px 时替代表格）
    目标：375px 宽度下无横向滚动；整卡可点进详情
  -->
  <div class="eng-cards">
    <div
      v-for="row in list"
      :key="row.id"
      class="eng-card"
      :class="Number(row.overdue_days) > 0 ? 'eng-card--overdue' : ''"
      @click="emit('card-click', row)"
    >
      <!-- 第一行：名称 + 状态 -->
      <div class="eng-card-head">
        <p class="eng-card-name">{{ row.name }}</p>
        <EngineeringStatusTag :status="row.status" />
      </div>

      <!-- 第二行：甲方 · 联系人 电话（脱敏） -->
      <p class="eng-card-client">
        {{ [row.client_name, row.client_contact].filter(Boolean).join(' · ') || '—' }}
        <span v-if="row.client_phone">{{ maskPhone(row.client_phone) }}</span>
      </p>

      <!-- 第三行：合同 / 已回 -->
      <div class="eng-card-row">
        <span class="eng-card-kv">
          <span class="eng-card-label">合同</span>
          <span class="eng-card-value">{{ cardContractMoney(row) }}</span>
          <span v-if="isEstimated(row)" class="eng-card-est">预估</span>
        </span>
        <span class="eng-card-kv">
          <span class="eng-card-label">已回</span>
          <span class="eng-card-value eng-card-value--received">{{ formatCardMoney(row.received_amount) }}</span>
        </span>
      </div>

      <!-- 第四行：待回 / 逾期 -->
      <div class="eng-card-row">
        <span class="eng-card-kv">
          <span class="eng-card-label">待回</span>
          <span
            class="eng-card-value eng-card-value--strong"
            :style="Number(row.pending_amount) > 0 ? { color: '#dc2626' } : null"
          >
            {{ formatCardMoney(row.pending_amount) }}
          </span>
        </span>
        <span v-if="Number(row.overdue_days) > 0" class="eng-card-overdue">逾期 {{ row.overdue_days }} 天</span>
      </div>

      <!-- 第五行：净利 · 净利率 -->
      <p class="eng-card-profit">
        <template v-if="Number(row.status) === 1">净利率 —</template>
        <template v-else>
          净利 {{ formatCardMoney(row.net_profit) }} ·
          <span :style="Number(row.net_margin_rate) < 0 ? { color: '#dc2626' } : null">
            净利率 {{ formatPercent(row.net_margin_rate) }}
          </span>
        </template>
      </p>

      <!-- 第六行：进度条 -->
      <div class="eng-card-progress">
        <el-progress
          :percentage="Math.min(100, Math.max(0, Number(row.received_ratio) || 0))"
          :stroke-width="6"
          :show-text="false"
          :color="progressColor(row)"
        />
        <span class="eng-card-progress-text">{{ Number(row.received_ratio) || 0 }}%</span>
      </div>
    </div>

    <!-- 空态 -->
    <div v-if="firstLoaded && !list.length" class="eng-card-empty">
      <p class="eng-card-empty-title">{{ hasActiveFilter ? '没有符合条件的项目' : '还没有工程项目' }}</p>
      <p class="eng-card-empty-sub">
        {{ hasActiveFilter ? '试试放宽筛选条件' : '工程机批量供货的合同、清单、回款都在这里管' }}
      </p>
      <el-button v-if="hasActiveFilter" size="small" @click="emit('reset')">清空筛选</el-button>
      <el-button v-else size="small" type="primary" @click="emit('create')">新建第一个工程</el-button>
    </div>

    <!-- 加载更多 -->
    <div v-if="hasMore" class="eng-card-more">
      <el-button size="small" :loading="loadingMore" @click="emit('load-more')">加载更多</el-button>
    </div>
    <p v-else-if="list.length" class="eng-card-end">已显示全部 {{ list.length }} 个项目</p>
  </div>
</template>

<script setup>
import EngineeringStatusTag from './EngineeringStatusTag.vue'
import { formatCardMoney, formatPercent } from '../../utils/engineeringFormat'

defineProps({
  list: { type: Array, default: () => [] },
  hasMore: { type: Boolean, default: false },
  loadingMore: { type: Boolean, default: false },
  firstLoaded: { type: Boolean, default: false },
  hasActiveFilter: { type: Boolean, default: false }
})

const emit = defineEmits(['card-click', 'load-more', 'reset', 'create'])

const isEstimated = row => Number(row.status) === 1 && Number(row.contract_total) <= 0
const cardContractMoney = row => formatCardMoney(isEstimated(row) ? row.estimated_amount : row.contract_total)

// 手机号脱敏：13800008000 -> 138****8000（列表页不裸奔完整号码）
const maskPhone = phone => {
  const text = String(phone)
  if (text.length < 7) return text
  return `${text.slice(0, 3)}****${text.slice(-4)}`
}

const progressColor = row => {
  if (Number(row.received_ratio) >= 100) return '#15803d'
  if (Number(row.overdue_days) > 0) return '#dc2626'
  return '#635BFF'
}
</script>

<style scoped>
.eng-cards {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.eng-card {
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 14px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
  cursor: pointer;
}
.eng-card--overdue {
  box-shadow: inset 3px 0 #dc2626, 0 1px 2px rgba(15, 23, 42, 0.04);
}
.eng-card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
}
.eng-card-name {
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
  line-height: 21px;
  flex: 1;
  min-width: 0;
}
.eng-card-client {
  margin-top: 4px;
  font-size: 12px;
  color: #94a3b8;
  line-height: 17px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.eng-card-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-top: 8px;
}
.eng-card-kv {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  min-width: 0;
}
.eng-card-label {
  font-size: 12px;
  color: #94a3b8;
}
.eng-card-value {
  font-size: 13px;
  color: #0f172a;
  font-variant-numeric: tabular-nums;
}
.eng-card-value--received {
  color: #15803d;
}
.eng-card-value--strong {
  font-weight: 600;
}
.eng-card-est {
  font-size: 10px;
  color: #94a3b8;
}
.eng-card-overdue {
  font-size: 11px;
  font-weight: 600;
  color: #dc2626;
}
.eng-card-profit {
  margin-top: 6px;
  font-size: 12px;
  color: #64748b;
  font-variant-numeric: tabular-nums;
}
.eng-card-progress {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
}
.eng-card-progress :deep(.el-progress) {
  flex: 1;
  min-width: 0;
}
.eng-card-progress-text {
  font-size: 11px;
  color: #64748b;
  font-variant-numeric: tabular-nums;
}
.eng-card-empty {
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 36px 16px;
  text-align: center;
}
.eng-card-empty-title {
  font-size: 14px;
  font-weight: 600;
  color: #334155;
}
.eng-card-empty-sub {
  margin: 6px 0 14px;
  font-size: 12px;
  color: #94a3b8;
  line-height: 18px;
}
.eng-card-more {
  padding: 6px 0 2px;
  text-align: center;
}
.eng-card-end {
  padding: 6px 0 2px;
  text-align: center;
  font-size: 11px;
  color: #cbd5e1;
}
</style>
