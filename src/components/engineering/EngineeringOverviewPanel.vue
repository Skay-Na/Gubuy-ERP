<template>
  <!--
    概览 Tab（SPEC-ENG-001 §5.3，任务卡 T12）
    金额构成 / 双进度 / 盈利卡 / 关键日期 / 备注 / 待办条
    盈利卡是老板最先看的一张，口径必须写清：设备成本是录入时快照，工程费用不含公共费用。
  -->
  <div class="eng-overview">
    <!-- 待办条 -->
    <div v-if="overdueTermCount > 0" class="eng-todo eng-todo--danger">
      <span>⚠ {{ overdueTermCount }} 个付款节点已逾期，合计 {{ formatEngMoney(overdueAmount) }}</span>
      <el-button link type="danger" @click="emit('goto-tab', 'payment')">去回款 Tab 处理</el-button>
    </div>
    <div v-else-if="canAccept" class="eng-todo eng-todo--info">
      <span>设备已全部发货，可以确认验收</span>
      <el-button link type="primary" @click="emit('accept')">去确认</el-button>
    </div>
    <div v-else-if="isSettled" class="eng-todo eng-todo--success">
      <span>项目已结清，金额字段已锁定</span>
    </div>

    <div class="eng-overview-grid">
      <!-- 左列 -->
      <div class="eng-col-main">
        <!-- 金额构成 -->
        <section class="eng-card">
          <p class="eng-card-title">金额构成</p>
          <div class="eng-kv-list">
            <div class="eng-kv">
              <span class="eng-kv-label">设备总额</span>
              <span class="eng-kv-value">
                {{ formatEngMoney(project.items_total) }}
                <span class="eng-kv-note">{{ itemCount }} 行 / {{ project.ordered_qty || 0 }} 台</span>
              </span>
            </div>
            <div class="eng-kv">
              <span class="eng-kv-label">安装费</span>
              <span class="eng-kv-value">{{ formatEngMoney(project.install_fee) }}<span class="eng-kv-note">手工录入</span></span>
            </div>
            <div class="eng-kv">
              <span class="eng-kv-label">运输费</span>
              <span class="eng-kv-value">{{ formatEngMoney(project.freight_fee) }}<span class="eng-kv-note">手工录入</span></span>
            </div>
            <div class="eng-kv">
              <span class="eng-kv-label">其他费用</span>
              <span class="eng-kv-value">{{ formatEngMoney(project.other_fee) }}</span>
            </div>
            <div class="eng-kv">
              <span class="eng-kv-label">折让</span>
              <span
                class="eng-kv-value"
                :style="Number(project.discount_amount) > 0 ? { color: '#dc2626' } : null"
              >
                {{ Number(project.discount_amount) > 0 ? '-' : '' }}{{ formatEngMoney(project.discount_amount) }}
              </span>
            </div>
            <div class="eng-kv eng-kv--total">
              <span class="eng-kv-label">合同总额</span>
              <span class="eng-kv-value eng-kv-value--strong">{{ formatEngMoney(project.contract_total) }}</span>
            </div>
          </div>
        </section>

        <!-- 双进度 -->
        <section class="eng-card">
          <p class="eng-card-title">进度</p>
          <div class="eng-progress-block">
            <div class="eng-progress-head">
              <span class="eng-progress-label">回款进度</span>
              <span class="eng-progress-nums">
                {{ formatEngMoney(project.received_amount) }} / {{ formatEngMoney(project.contract_total) }}
              </span>
            </div>
            <el-progress
              :percentage="Math.min(100, Math.max(0, Number(project.received_ratio) || 0))"
              :stroke-width="8"
              :show-text="false"
              :color="progressColor(project.received_ratio, overdueTermCount > 0)"
            />
          </div>
          <div class="eng-progress-block">
            <div class="eng-progress-head">
              <span class="eng-progress-label">发货进度</span>
              <span class="eng-progress-nums">{{ project.shipped_qty || 0 }} / {{ project.ordered_qty || 0 }} 台</span>
            </div>
            <el-progress
              :percentage="Math.min(100, Math.max(0, Number(project.shipped_ratio) || 0))"
              :stroke-width="8"
              :show-text="false"
              :color="progressColor(project.shipped_ratio, false)"
            />
          </div>
        </section>
      </div>

      <!-- 右列 -->
      <div class="eng-col-side">
        <!-- 盈利卡 -->
        <section class="eng-card">
          <p class="eng-card-title">盈利</p>
          <div class="eng-kv-list">
            <div class="eng-kv">
              <span class="eng-kv-label">合同总额</span>
              <span class="eng-kv-value">{{ formatEngMoney(project.contract_total) }}</span>
            </div>
            <div class="eng-kv">
              <span class="eng-kv-label">− 设备成本</span>
              <span class="eng-kv-value" style="color: #475569">-{{ formatEngMoney(project.cost_total) }}</span>
            </div>
            <div class="eng-kv">
              <span class="eng-kv-label">− 工程费用</span>
              <span class="eng-kv-value">
                <button
                  v-if="Number(project.expense_total) > 0"
                  type="button"
                  class="eng-link-btn"
                  @click="emit('goto-tab', 'expense')"
                >
                  -{{ formatEngMoney(project.expense_total) }}
                </button>
                <button v-else type="button" class="eng-link-btn eng-link-btn--warn" @click="emit('goto-tab', 'expense')">
                  未录入
                </button>
              </span>
            </div>
            <div class="eng-kv eng-kv--total">
              <span class="eng-kv-label">= 项目净利</span>
              <span
                class="eng-kv-value eng-kv-value--strong"
                :style="Number(project.net_profit) < 0 ? { color: '#dc2626' } : { color: '#635bff' }"
              >
                {{ formatEngMoney(project.net_profit) }}
              </span>
            </div>
            <div class="eng-kv eng-kv--plain">
              <span class="eng-kv-label">净利率</span>
              <span class="eng-kv-value" :style="marginStyle">
                {{ formatPercent(project.net_margin_rate) }}
                <span v-if="marginHint" class="eng-margin-hint">{{ marginHint }}</span>
              </span>
            </div>
          </div>

          <div v-if="Number(project.expense_total) <= 0" class="eng-card-warn">
            工程费用尚未录入，净利可能虚高
            <el-button link type="warning" @click="emit('goto-tab', 'expense')">去录入</el-button>
          </div>

          <p class="eng-card-foot">设备成本按录入时快照；工程费用为本项目实际支出，不含公共费用（见工程开销台账）</p>
        </section>

        <!-- 关键日期 -->
        <section class="eng-card">
          <p class="eng-card-title">关键日期</p>
          <div class="eng-kv-list">
            <div class="eng-kv">
              <span class="eng-kv-label">签约日期</span>
              <span class="eng-kv-value">{{ formatEngDate(project.sign_date) }}</span>
            </div>
            <div class="eng-kv">
              <span class="eng-kv-label">预计验收</span>
              <span class="eng-kv-value">—</span>
            </div>
            <div class="eng-kv">
              <span class="eng-kv-label">实际验收</span>
              <span class="eng-kv-value">{{ formatEngDate(project.accept_date) }}</span>
            </div>
            <div class="eng-kv">
              <span class="eng-kv-label">结清时间</span>
              <span class="eng-kv-value">{{ formatEngDate(project.settled_at) }}</span>
            </div>
          </div>
        </section>

        <!-- 备注 -->
        <section class="eng-card">
          <p class="eng-card-title">备注</p>
          <p v-if="!project.remark" class="eng-note-empty">—</p>
          <template v-else>
            <p class="eng-note" :class="expanded ? '' : 'eng-note--clamp'">{{ project.remark }}</p>
            <el-button link type="primary" size="small" @click="expanded = !expanded">
              {{ expanded ? '收起' : '展开' }}
            </el-button>
          </template>
        </section>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { formatEngDate, formatEngMoney, formatPercent } from '../../utils/engineeringFormat'

const props = defineProps({
  project: { type: Object, required: true }
})

const emit = defineEmits(['goto-tab', 'accept'])

const expanded = ref(false)

const itemCount = computed(() => (props.project.items || []).length)
const overdueTermCount = computed(() => Number(props.project.overdue_term_count) || 0)
const overdueAmount = computed(() => Number(props.project.overdue_amount) || 0)
const isSettled = computed(() => Number(props.project.status) === 5)

// 全部发完且还在供货中 → 可以验收
const canAccept = computed(() => {
  const p = props.project
  const ordered = Number(p.ordered_qty) || 0
  return Number(p.status) === 3 && ordered > 0 && Number(p.shipped_qty) >= ordered
})

const marginRate = computed(() => Number(props.project.net_margin_rate) || 0)
const marginStyle = computed(() => {
  if (marginRate.value < 0) return { color: '#dc2626', fontWeight: 700 }
  if (marginRate.value < 10) return { color: '#d97706', fontWeight: 700 }
  return null
})
const marginHint = computed(() => {
  if (marginRate.value < 0) return '负净利'
  if (marginRate.value < 10) return '净利率偏低'
  return ''
})

const progressColor = (ratio, hasOverdue) => {
  if (Number(ratio) >= 100) return '#15803d'
  if (hasOverdue) return '#dc2626'
  return '#635BFF'
}
</script>

<style scoped>
.eng-overview {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.eng-overview-grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
  gap: 12px;
}
.eng-col-main,
.eng-col-side {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}
.eng-card {
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 14px 16px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.eng-card-title {
  font-size: 12px;
  font-weight: 600;
  color: #94a3b8;
  letter-spacing: 0.03em;
  margin-bottom: 10px;
}
.eng-kv-list {
  display: flex;
  flex-direction: column;
}
.eng-kv {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  padding: 5px 0;
}
.eng-kv-label {
  font-size: 12px;
  color: #94a3b8;
  flex: 0 0 auto;
}
.eng-kv-value {
  font-size: 14px;
  color: #0f172a;
  font-variant-numeric: tabular-nums;
  text-align: right;
}
.eng-kv-note {
  margin-left: 6px;
  font-size: 11px;
  color: #cbd5e1;
  font-weight: 400;
}
.eng-kv--total {
  margin-top: 4px;
  padding-top: 9px;
  border-top: 1px solid #f1f5f9;
}
.eng-kv--total .eng-kv-value--strong {
  font-size: 16px;
  font-weight: 900;
}
.eng-kv--plain {
  padding-top: 2px;
}
.eng-link-btn {
  border: none;
  background: none;
  padding: 0;
  font-size: 14px;
  color: #0f172a;
  cursor: pointer;
  font-variant-numeric: tabular-nums;
}
.eng-link-btn:hover {
  color: #635bff;
  text-decoration: underline;
}
.eng-link-btn--warn {
  color: #d97706;
  font-size: 13px;
}
.eng-margin-hint {
  margin-left: 6px;
  font-size: 11px;
  font-weight: 600;
}
.eng-progress-block {
  margin-bottom: 12px;
}
.eng-progress-block:last-child {
  margin-bottom: 0;
}
.eng-progress-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: 6px;
}
.eng-progress-label {
  font-size: 12px;
  color: #64748b;
}
.eng-progress-nums {
  font-size: 12px;
  color: #334155;
  font-variant-numeric: tabular-nums;
}
.eng-card-warn {
  margin-top: 10px;
  border-radius: 8px;
  background: #fffbeb;
  padding: 8px 10px;
  font-size: 11px;
  color: #b45309;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.eng-card-foot {
  margin-top: 10px;
  font-size: 11px;
  line-height: 16px;
  color: #cbd5e1;
}
.eng-note {
  font-size: 12px;
  line-height: 19px;
  color: #475569;
  white-space: pre-wrap;
}
.eng-note--clamp {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.eng-note-empty {
  font-size: 12px;
  color: #cbd5e1;
}
.eng-todo {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  border-radius: 12px;
  padding: 10px 14px;
  font-size: 12px;
}
.eng-todo--danger {
  background: #fef2f2;
  color: #b91c1c;
}
.eng-todo--info {
  background: #eff6ff;
  color: #1d4ed8;
}
.eng-todo--success {
  background: #e8f7ee;
  color: #15803d;
}
@media (max-width: 767px) {
  .eng-overview-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
