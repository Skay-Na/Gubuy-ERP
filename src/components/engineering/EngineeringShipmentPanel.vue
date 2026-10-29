<template>
  <!--
    出库 Tab（SPEC-ENG-001 §5.6，任务卡 T12）
    数据来自详情接口的 shipments[]，一行 = 一次出库的一个商品。
  -->
  <div class="eng-panel">
    <div class="eng-panel-head">
      <div>
        <p class="eng-panel-title">出库记录</p>
        <p class="eng-panel-sub">
          已出库 {{ shipSummary.total }} 台 / 共 {{ project?.ordered_qty || 0 }} 台
          <span v-if="shipSummary.store">· 门店 {{ shipSummary.store }} 台</span>
          <span v-if="shipSummary.cloud">· 云仓 {{ shipSummary.cloud }} 台</span>
        </p>
      </div>
      <div class="eng-panel-actions">
        <el-button v-if="canShip" type="primary" size="small" @click="emit('add')">新增出库</el-button>
      </div>
    </div>

    <!-- 移动端 -->
    <div v-if="isMobile" class="eng-mobile-list">
      <EngineeringMobileRow
        v-for="row in shipments"
        :key="row.id"
        :title="row.product_name"
        :subtitle="`${shortTime(row.created_at)} · ${row.warehouse_label} · ${row.operator_name || '—'}`"
        :amount="`${row.quantity} 台`"
        :fields="[{ label: '物流单号', value: row.tracking_no || '—' }]"
      >
        <p v-if="row.remark" class="eng-mobile-note">{{ row.remark }}</p>
      </EngineeringMobileRow>
      <p v-if="!shipments.length" class="eng-mobile-empty">尚未出库，按清单行发货，出库会扣门店库存</p>
    </div>

    <!-- PC -->
    <el-table v-else :data="shipments" stripe class="eng-table">
      <el-table-column label="出库时间" width="150">
        <template #default="{ row }">{{ shortTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="商品" min-width="200" prop="product_name" />
      <el-table-column label="数量" width="90" align="right">
        <template #default="{ row }">{{ row.quantity }} 台</template>
      </el-table-column>
      <el-table-column label="发货仓" width="100" align="center">
        <template #default="{ row }">
          <span class="eng-wh-tag">{{ row.warehouse_label }}</span>
        </template>
      </el-table-column>
      <el-table-column label="出库日期" width="120">
        <template #default="{ row }">{{ formatEngDate(row.ship_date) }}</template>
      </el-table-column>
      <el-table-column label="物流单号" width="142">
        <template #default="{ row }">
          <span class="eng-mono">{{ row.tracking_no || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作人" width="100">
        <template #default="{ row }">{{ row.operator_name || '—' }}</template>
      </el-table-column>
      <el-table-column label="备注" min-width="150">
        <template #default="{ row }">
          <span class="eng-ellipsis" :title="row.remark">{{ row.remark || '—' }}</span>
        </template>
      </el-table-column>
      <template #empty>
        <div class="eng-empty">
          <p class="eng-empty-title">尚未出库</p>
          <p class="eng-empty-sub">按清单行发货，出库会扣门店库存</p>
          <el-button v-if="canShip" type="primary" size="small" class="mt-3" @click="emit('add')">新增出库</el-button>
        </div>
      </template>
    </el-table>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import EngineeringMobileRow from './EngineeringMobileRow.vue'
import { formatEngDate } from '../../utils/engineeringFormat'

const props = defineProps({
  shipments: { type: Array, default: () => [] },
  project: { type: Object, default: null },
  canShip: { type: Boolean, default: false }
})

const emit = defineEmits(['add'])

const isMobile = ref(window.innerWidth < 768)
window.addEventListener('resize', () => {
  isMobile.value = window.innerWidth < 768
})

const shipSummary = computed(() => {
  let total = 0
  let store = 0
  let cloud = 0
  props.shipments.forEach(s => {
    total += Number(s.quantity) || 0
    if (Number(s.warehouse_type) === 2) store += Number(s.quantity) || 0
    if (Number(s.warehouse_type) === 3) cloud += Number(s.quantity) || 0
  })
  return { total, store, cloud }
})

const shortTime = value => {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '—'
  const pad = n => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
</script>

<style scoped>
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
.eng-wh-tag {
  display: inline-flex;
  border-radius: 999px;
  background: #f1f5f9;
  padding: 2px 8px;
  font-size: 11px;
  color: #475569;
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
.eng-mobile-empty {
  padding: 20px;
  text-align: center;
  font-size: 12px;
  color: #94a3b8;
}
.eng-mobile-note {
  margin-top: 6px;
  font-size: 11px;
  color: #94a3b8;
}
</style>
