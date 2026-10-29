<template>
  <!--
    设备清单表（SPEC-ENG-001 §5.4，任务卡 T13）
    两种模式：查看 / 编辑。编辑是整表一次性提交（不做逐行保存），
    保存失败保留输入内容，取消则全部回滚。
  -->
  <div class="eng-panel">
    <div class="eng-panel-head">
      <div>
        <p class="eng-panel-title">设备清单</p>
        <p class="eng-panel-sub">清单合计构成合同总额的基础；已出库的行不可更换商品</p>
      </div>
      <div class="eng-panel-actions">
        <template v-if="!editing">
          <el-button v-if="canEdit" type="primary" plain size="small" @click="startEdit">编辑清单</el-button>
        </template>
        <template v-else>
          <el-button size="small" @click="cancelEdit">取消</el-button>
          <el-button type="primary" size="small" :loading="saving" @click="save">保存清单</el-button>
        </template>
      </div>
    </div>

    <div v-if="editing" class="eng-edit-hint">编辑中，保存后会重算合同总额与毛利</div>

    <!-- 移动端：卡片流 -->
    <div v-if="isMobile && !editing" class="eng-mobile-list">
      <EngineeringMobileRow
        v-for="row in items"
        :key="row.id"
        :title="row.product_name"
        :subtitle="row.product_sku || '—'"
        :amount="formatEngMoney(row.amount)"
        :fields="[
          { label: '数量', value: `${row.quantity} 台` },
          { label: '单价', value: formatEngMoney(row.unit_price) },
          { label: '成本', value: formatEngMoney(row.unit_cost) },
          { label: '已发', value: `${row.shipped_qty}/${row.quantity}` }
        ]"
      />
      <p v-if="!items.length" class="eng-mobile-empty">清单为空，添加商品后才能签约</p>
      <div class="eng-mobile-total">
        设备总额 {{ formatEngMoney(aggregate.itemsTotal) }} · 成本 {{ formatEngMoney(aggregate.costTotal) }} ·
        毛利 {{ formatEngMoney(aggregate.grossProfit) }} · 毛利率 {{ formatPercent(aggregate.marginRate) }}
      </div>
    </div>

    <!-- PC：表格 -->
    <el-table v-else :data="editing ? draft : items" stripe class="eng-table" :max-height="editing ? 460 : undefined">
      <el-table-column label="商品" min-width="188">
        <template #default="{ row, $index }">
          <template v-if="editing">
            <el-select
              v-model="row.product_id"
              filterable
              remote
              :remote-method="searchProducts"
              :loading="searching"
              :disabled="row.shipped_qty > 0"
              placeholder="搜索商品名称 / SKU"
              class="w-full"
              @change="v => onPickProduct(row, v)"
            >
              <el-option v-for="p in productOptions" :key="p.id" :label="p.name" :value="p.id">
                <span>{{ p.name }}</span>
                <span class="eng-opt-sub">库存 {{ p.main_stock ?? 0 }}</span>
              </el-option>
            </el-select>
            <span v-if="row.shipped_qty > 0" class="eng-locked">已出库 {{ row.shipped_qty }} 台，不可换商品</span>
          </template>
          <div v-else class="eng-cell-stack">
            <span class="eng-cell-strong">{{ row.product_name }}</span>
            <span class="eng-cell-sub">{{ row.product_sku || '—' }}</span>
          </div>
        </template>
      </el-table-column>

      <el-table-column label="数量" width="84" align="right">
        <template #default="{ row }">
          <el-input-number
            v-if="editing"
            v-model="row.quantity"
            :min="Math.max(1, row.shipped_qty)"
            :controls="false"
            :precision="0"
            class="w-full"
          />
          <span v-else>{{ row.quantity }}</span>
        </template>
      </el-table-column>

      <el-table-column label="工程单价" width="120" align="right">
        <template #default="{ row }">
          <el-input-number v-if="editing" v-model="row.unit_price" :min="0.01" :precision="2" :controls="false" class="w-full" />
          <span v-else class="eng-money">{{ formatEngMoney(row.unit_price) }}</span>
        </template>
      </el-table-column>

      <el-table-column label="成本单价" width="120" align="right">
        <template #default="{ row }">
          <el-input-number v-if="editing" v-model="row.unit_cost" :min="0" :precision="2" :controls="false" class="w-full" />
          <div v-else class="eng-cell-stack eng-cell-stack--right">
            <span class="eng-money">{{ formatEngMoney(row.unit_cost) }}</span>
            <span v-if="row.cost_missing" class="eng-warn-text">成本缺失</span>
          </div>
        </template>
      </el-table-column>

      <el-table-column label="行金额" width="130" align="right">
        <template #default="{ row }">
          <span class="eng-money eng-money--strong">{{ formatEngMoney(rowAmount(row)) }}</span>
        </template>
      </el-table-column>

      <el-table-column label="行毛利" width="114" align="right">
        <template #default="{ row }">
          <span
            class="eng-money"
            :style="rowGross(row) < 0 ? { color: '#dc2626' } : null"
          >{{ formatEngMoney(rowGross(row)) }}</span>
        </template>
      </el-table-column>

      <el-table-column label="毛利率" width="90" align="right">
        <template #default="{ row }">
          <span :style="rowMargin(row) < 10 ? { color: '#d97706' } : null">{{ formatPercent(rowMargin(row)) }}</span>
        </template>
      </el-table-column>

      <el-table-column label="已发货" width="90" align="center">
        <template #default="{ row }">
          <span :style="row.shipped_qty >= row.quantity ? { color: '#15803d', fontWeight: 600 } : null">
            {{ row.shipped_qty }}/{{ row.quantity }}
          </span>
        </template>
      </el-table-column>

      <el-table-column label="备注" width="130">
        <template #default="{ row }">
          <el-input v-if="editing" v-model="row.remark" maxlength="200" placeholder="选填" />
          <span v-else class="eng-ellipsis" :title="row.remark">{{ row.remark || '—' }}</span>
        </template>
      </el-table-column>

      <el-table-column v-if="editing" label="操作" width="70" align="center">
        <template #default="{ $index }">
          <el-button
            link
            type="danger"
            size="small"
            :disabled="draft[$index].shipped_qty > 0"
            @click="removeRow($index)"
          >
            删除
          </el-button>
        </template>
      </el-table-column>

      <template #empty>
        <div class="eng-empty">
          <p class="eng-empty-title">清单为空</p>
          <p class="eng-empty-sub">添加商品后才能签约</p>
          <el-button v-if="editing" type="primary" size="small" class="mt-3" @click="addRow">添加商品</el-button>
        </div>
      </template>
    </el-table>

    <div v-if="editing" class="eng-add-row">
      <el-button size="small" @click="addRow">+ 添加一行</el-button>
    </div>

    <!-- 汇总条 -->
    <div v-if="!isMobile || editing" class="eng-sum-line">
      设备总额 <b>{{ formatEngMoney(aggregate.itemsTotal) }}</b>
      <span class="eng-dot">·</span>成本 {{ formatEngMoney(aggregate.costTotal) }}
      <span class="eng-dot">·</span>毛利 {{ formatEngMoney(aggregate.grossProfit) }}
      <span class="eng-dot">·</span>毛利率 {{ formatPercent(aggregate.marginRate) }}
      <span class="eng-dot">·</span>{{ items.length }} 行 / {{ aggregate.totalQty }} 台
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { fetchProducts } from '../../api/products'
import EngineeringMobileRow from './EngineeringMobileRow.vue'
import { formatEngMoney, formatPercent } from '../../utils/engineeringFormat'

const props = defineProps({
  items: { type: Array, default: () => [] },
  status: { type: Number, default: 1 },
  saving: { type: Boolean, default: false }
})

const emit = defineEmits(['save'])

const isMobile = ref(window.innerWidth < 768)
window.addEventListener('resize', () => {
  isMobile.value = window.innerWidth < 768
})

// 状态 1/2/3 允许改清单；已验收、已结清、已作废不可改（金额已锁定）
const canEdit = computed(() => [1, 2, 3].includes(Number(props.status)))

const editing = ref(false)
const draft = ref([])
const productOptions = ref([])
const searching = ref(false)

const cloneRows = () =>
  props.items.map(it => ({
    id: it.id,
    product_id: it.product_id,
    product_name: it.product_name,
    quantity: it.quantity,
    unit_price: Number(it.unit_price),
    unit_cost: Number(it.unit_cost),
    shipped_qty: it.shipped_qty,
    remark: it.remark || ''
  }))

const startEdit = () => {
  draft.value = cloneRows()
  productOptions.value = props.items.map(it => ({ id: it.product_id, name: it.product_name, main_stock: null }))
  editing.value = true
}

const cancelEdit = () => {
  draft.value = []
  editing.value = false
}

const addRow = () => {
  draft.value.push({ id: 0, product_id: null, product_name: '', quantity: 1, unit_price: 0, unit_cost: 0, shipped_qty: 0, remark: '' })
}

const removeRow = index => {
  draft.value.splice(index, 1)
}

const searchProducts = async keyword => {
  if (!keyword) return
  searching.value = true
  try {
    const res = await fetchProducts({ keyword, page: 1, page_size: 20 })
    const raw = res?.result
    const list = Array.isArray(raw) ? raw : raw?.list || []
    productOptions.value = list
  } catch (e) {
    productOptions.value = []
  } finally {
    searching.value = false
  }
}

// 选商品后把成本单价带出来（最新进货成本），仍可手工改
const onPickProduct = (row, productId) => {
  const p = productOptions.value.find(x => x.id === productId)
  if (!p) return
  row.product_name = p.name
  if (!row.unit_cost) row.unit_cost = Number(p.latest_cost) || 0
  if (!row.unit_price && Number(p.latest_cost)) row.unit_price = Number(p.latest_cost)
}

const rowAmount = row => (Number(row.quantity) || 0) * (Number(row.unit_price) || 0)
const rowGross = row => rowAmount(row) - (Number(row.quantity) || 0) * (Number(row.unit_cost) || 0)
const rowMargin = row => {
  const amount = rowAmount(row)
  if (!amount) return 0
  return Math.round((rowGross(row) / amount) * 10000) / 100
}

const aggregate = computed(() => {
  const source = editing.value ? draft.value : props.items
  let itemsTotal = 0
  let costTotal = 0
  let totalQty = 0
  source.forEach(it => {
    const qty = Number(it.quantity) || 0
    itemsTotal += qty * (Number(it.unit_price) || 0)
    costTotal += qty * (Number(it.unit_cost) || 0)
    totalQty += qty
  })
  const grossProfit = itemsTotal - costTotal
  return {
    itemsTotal,
    costTotal,
    grossProfit,
    totalQty,
    marginRate: itemsTotal > 0 ? Math.round((grossProfit / itemsTotal) * 10000) / 100 : 0
  }
})

const save = () => {
  if (!draft.value.length) {
    ElMessage.warning('清单至少保留 1 行')
    return
  }
  const invalid = draft.value.find(r => !r.product_id || Number(r.quantity) < 1 || Number(r.unit_price) <= 0)
  if (invalid) {
    ElMessage.warning('请检查商品、数量（≥1）与工程单价（>0）')
    return
  }
  emit('save', {
    items: draft.value.map(r => ({
      id: Number(r.id) || 0,
      product_id: Number(r.product_id),
      quantity: Number(r.quantity),
      unit_price: Number(r.unit_price),
      unit_cost: Number(r.unit_cost) || 0,
      remark: (r.remark || '').trim()
    }))
  })
}

// 父层保存成功后调用，退出编辑态
const finishSave = () => {
  editing.value = false
  draft.value = []
}

defineExpose({ finishSave })
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
.eng-panel-actions {
  display: flex;
  gap: 8px;
  flex: 0 0 auto;
}
.eng-edit-hint {
  margin-bottom: 10px;
  border-radius: 8px;
  background: #eef0ff;
  padding: 8px 12px;
  font-size: 12px;
  color: #635bff;
}
.eng-add-row {
  margin-top: 10px;
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
.eng-dot {
  margin: 0 6px;
  color: #cbd5e1;
}
.eng-cell-stack {
  display: flex;
  flex-direction: column;
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
  font-variant-numeric: tabular-nums;
  color: #334155;
}
.eng-money--strong {
  font-weight: 600;
  color: #0f172a;
}
.eng-warn-text {
  font-size: 11px;
  color: #d97706;
}
.eng-locked {
  display: block;
  font-size: 11px;
  color: #94a3b8;
  margin-top: 2px;
}
.eng-ellipsis {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #64748b;
}
.eng-opt-sub {
  float: right;
  margin-left: 10px;
  font-size: 11px;
  color: #94a3b8;
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
.eng-mobile-total {
  border-top: 1px dashed #f1f5f9;
  padding-top: 10px;
  font-size: 11px;
  color: #64748b;
  line-height: 17px;
}
</style>
