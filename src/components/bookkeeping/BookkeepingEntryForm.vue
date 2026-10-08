<script setup>
import { computed, onMounted, reactive, watch } from 'vue'
import { Check, Clock, Link, Wallet } from '@element-plus/icons-vue'
import { useBookkeepingOrderSearch } from '../../composables/useBookkeepingOrderSearch'

const props = defineProps({
  accounts: { type: Array, default: () => [] },
  submitting: Boolean,
  resetToken: { type: Number, default: 0 },
  mobile: Boolean
})

const emit = defineEmits(['submit'])

const incomeCategories = ['销售外收入', '安装服务', '运费收入', '补贴返还', '利息收入', '其他收入']
const expenseCategories = ['渠道奖励', '员工提成', '安装费', '日常采购', '运输配送', '水电物业', '员工报销', '营销推广', '房租', '其他支出']
const orderLinkedCategories = {
  '渠道奖励': '订单渠道奖励',
  '员工提成': '订单员工提成',
  '安装费': '订单安装费',
  '运输配送': '订单送货费用'
}
const form = reactive({
  entry_type: 2,
  category: '日常采购',
  amount: '',
  account_id: null,
  order_id: null,
  counterparty: '',
  related_no: '',
  remark: '',
  occurred_at: ''
})

const {
  orderOptions,
  orderSearchLoading,
  searchOrders,
  searchCustomers
} = useBookkeepingOrderSearch()

const categories = computed(() => form.entry_type === 1 ? incomeCategories : expenseCategories)
const selectedAccount = computed(() => props.accounts.find(account => account.id === form.account_id))
const canSubmit = computed(() =>
  Boolean(form.account_id && form.category.trim() && Number(form.amount) > 0 && !props.submitting)
)
const selectedOrder = computed(() => orderOptions.value.find(order => order.id === form.order_id))
const linkedOrderField = computed(() => orderLinkedCategories[form.category] || '')

watch(
  () => props.accounts,
  (accounts) => {
    if (!accounts.length || form.account_id) return
    form.account_id = accounts.find(account => account.is_default_expense)?.id || accounts[0].id
  },
  { immediate: true }
)

watch(
  () => form.entry_type,
  (type) => {
    form.category = type === 1 ? incomeCategories[0] : expenseCategories[0]
  }
)

watch(
  () => props.resetToken,
  () => {
    form.amount = ''
    form.order_id = null
    form.counterparty = ''
    form.related_no = ''
    form.remark = ''
    form.occurred_at = ''
  }
)

function selectOrder(orderID) {
  if (!orderID) {
    form.order_id = null
    form.related_no = ''
    return
  }
  const order = orderOptions.value.find(item => item.id === orderID)
  if (!order) return
  form.order_id = order.id
  form.related_no = order.order_no
  form.counterparty = order.customer_name || form.counterparty
}

function selectCustomer(suggestion) {
  form.counterparty = suggestion.value
}

function formatOccurredAt(value) {
  if (!value) return ''
  return `${value.replace('T', ' ')}:00`
}

function submit() {
  if (!canSubmit.value) return
  emit('submit', {
    entry_type: form.entry_type,
    category: form.category.trim(),
    amount: Number(form.amount),
    account_id: form.account_id,
    order_id: form.order_id || undefined,
    counterparty: form.counterparty.trim(),
    related_no: form.related_no.trim(),
    remark: form.remark.trim(),
    occurred_at: formatOccurredAt(form.occurred_at)
  })
}

onMounted(() => searchOrders(''))
</script>

<template>
  <section class="entry-form" :class="{ mobile }">
    <div class="entry-type-switch" role="group" aria-label="收支类型">
      <button type="button" :class="{ active: form.entry_type === 2 }" @click="form.entry_type = 2">支出</button>
      <button type="button" :class="{ active: form.entry_type === 1 }" @click="form.entry_type = 1">收入</button>
    </div>

    <label class="amount-field">
      <span>{{ form.entry_type === 1 ? '收入金额' : '支出金额' }}</span>
      <div class="amount-control">
        <b>¥</b>
        <input v-model="form.amount" type="number" min="0.01" step="0.01" inputmode="decimal" placeholder="0.00" @keyup.enter="submit" />
      </div>
    </label>

    <div class="category-field">
      <span class="field-label">分类</span>
      <div class="category-list">
        <button v-for="category in categories" :key="category" type="button" :class="{ active: form.category === category }" @click="form.category = category">
          {{ category }}
        </button>
      </div>
      <el-input v-model="form.category" maxlength="50" placeholder="或输入自定义分类" />
    </div>

    <div class="entry-grid">
      <label class="form-field">
        <span>资金账户</span>
        <el-select v-model="form.account_id" placeholder="选择账户" class="full-width">
          <el-option v-for="account in accounts" :key="account.id" :label="account.name" :value="account.id" />
        </el-select>
        <small v-if="selectedAccount"><el-icon><Wallet /></el-icon> 当前余额 ¥{{ Number(selectedAccount.balance || 0).toFixed(2) }}</small>
      </label>
      <label class="form-field">
        <span>往来客户 <i>选填 · 支持姓名、电话、拼音搜索</i></span>
        <el-autocomplete
          v-model="form.counterparty"
          :fetch-suggestions="searchCustomers"
          clearable
          maxlength="100"
          placeholder="选填，可搜索已有客户或直接输入"
          class="full-width"
          @select="selectCustomer"
        >
          <template #default="{ item }">
            <div class="customer-option"><b>{{ item.value }}</b><small>{{ item.phone || '无电话' }} · {{ item.orderNo }}</small></div>
          </template>
        </el-autocomplete>
      </label>
      <label class="form-field">
        <span>关联订单 <i>选填 · 支持订单号、客户、电话、拼音搜索</i></span>
        <el-select
          v-model="form.order_id"
          filterable
          remote
          clearable
          remote-show-suffix
          :remote-method="searchOrders"
          :loading="orderSearchLoading"
          placeholder="选填，可搜索并关联系统订单"
          class="full-width"
          @visible-change="visible => visible && searchOrders('')"
          @change="selectOrder"
        >
          <el-option v-for="order in orderOptions" :key="order.id" :label="`${order.order_no} · ${order.customer_name || '散客'}`" :value="order.id">
            <div class="order-option"><div><b>{{ order.order_no }}</b><small>{{ order.customer_name || '散客' }} · {{ order.customer_phone || '无电话' }}</small></div><strong>¥{{ Number(order.actual_pay_amount || 0).toFixed(2) }}</strong></div>
          </el-option>
        </el-select>
        <small v-if="selectedOrder"><el-icon><Link /></el-icon> 已关联 {{ selectedOrder.order_no }}</small>
      </label>
      <label class="form-field">
        <span>发生时间 <i>默认现在</i></span>
        <div class="native-input-wrap"><el-icon><Clock /></el-icon><input v-model="form.occurred_at" type="datetime-local" /></div>
      </label>
    </div>

    <div v-if="linkedOrderField" class="order-link-hint" :class="{ active: selectedOrder }">
      <el-icon><Link /></el-icon>
      <span v-if="selectedOrder">保存后将同步增加 {{ selectedOrder.order_no }} 的{{ linkedOrderField }}。</span>
      <span v-else>选择关联订单后，这笔金额会同步增加到{{ linkedOrderField }}。</span>
    </div>

    <label class="form-field remark-field">
      <span>备注 <i>选填</i></span>
      <el-input v-model="form.remark" type="textarea" :rows="mobile ? 2 : 3" maxlength="255" show-word-limit placeholder="补充用途、经手情况或核对说明" />
    </label>

    <button type="button" class="submit-button" :class="form.entry_type === 1 ? 'income' : 'expense'" :disabled="!canSubmit" @click="submit">
      <el-icon><Check /></el-icon>
      {{ submitting ? '正在入账…' : `确认记${form.entry_type === 1 ? '收入' : '支出'}` }}
    </button>
  </section>
</template>

<style scoped>
.entry-form { display: grid; width: 100%; min-width: 0; gap: 16px; }
.entry-type-switch { display: grid; grid-template-columns: repeat(2, 1fr); padding: 4px; border-radius: 14px; background: #f1f5f9; }
.entry-type-switch button { min-height: 42px; border: 0; border-radius: 11px; background: transparent; color: #64748b; font-size: 14px; font-weight: 900; cursor: pointer; }
.entry-type-switch button.active { color: #0f172a; background: #fff; box-shadow: 0 3px 10px rgba(15, 23, 42, .09); }
.amount-field > span, .field-label, .form-field > span { display: block; margin-bottom: 7px; color: #475569; font-size: 12px; font-style: normal; font-weight: 800; }
.form-field i { color: #94a3b8; font-size: 10px; font-style: normal; font-weight: 500; }
.amount-control { display: flex; align-items: baseline; gap: 10px; padding: 6px 0 12px; border-bottom: 2px solid #cbd5e1; }
.amount-control:focus-within { border-color: #2563eb; }
.amount-control b { color: #64748b; font-size: 28px; }
.amount-control input { min-width: 0; width: 100%; border: 0; outline: 0; background: transparent; color: #0f172a; font-size: 42px; font-weight: 900; letter-spacing: -.04em; }
.amount-control input::-webkit-inner-spin-button { appearance: none; }
.category-field { min-width: 0; overflow: hidden; }
.category-list { display: flex; max-width: 100%; flex-wrap: wrap; gap: 7px; margin-bottom: 9px; }
.category-list button { padding: 7px 11px; border: 1px solid #dbe3ee; border-radius: 999px; background: #fff; color: #475569; font-size: 12px; font-weight: 700; cursor: pointer; }
.category-list button.active { border-color: #2563eb; color: #fff; background: #2563eb; }
.entry-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 13px; }
.form-field { min-width: 0; }.full-width { width: 100%; }.form-field small { display: flex; align-items: center; gap: 4px; margin-top: 6px; color: #64748b; font-size: 10px; }
.customer-option { display: grid; line-height: 1.35; }.customer-option b { color: #334155; }.customer-option small { color: #94a3b8; font-size: 10px; }.order-option { display: flex; align-items: center; justify-content: space-between; gap: 16px; }.order-option > div { display: grid; line-height: 1.35; }.order-option b { color: #334155; }.order-option small { color: #94a3b8; font-size: 10px; }.order-option strong { color: #0f172a; font-size: 12px; }
.order-link-hint { display: flex; align-items: center; gap: 7px; padding: 10px 12px; border: 1px solid #fde68a; border-radius: 11px; color: #92400e; background: #fffbeb; font-size: 11px; }.order-link-hint.active { border-color: #bfdbfe; color: #1d4ed8; background: #eff6ff; }
.native-input-wrap { display: flex; min-height: 32px; align-items: center; gap: 7px; padding: 0 10px; border: 1px solid #dcdfe6; border-radius: 4px; background: #fff; color: #a8abb2; }
.native-input-wrap input { min-width: 0; width: 100%; border: 0; outline: 0; background: transparent; color: #606266; font: inherit; }
.submit-button { display: inline-flex; min-height: 48px; align-items: center; justify-content: center; gap: 8px; border: 0; border-radius: 13px; color: #fff; font-size: 14px; font-weight: 900; cursor: pointer; }
.submit-button.expense { background: #e11d48; box-shadow: 0 10px 20px rgba(225, 29, 72, .2); }.submit-button.income { background: #059669; box-shadow: 0 10px 20px rgba(5, 150, 105, .2); }.submit-button:disabled { cursor: not-allowed; opacity: .45; box-shadow: none; }
@media (max-width: 560px) { .entry-form { gap: 14px; }.entry-grid { grid-template-columns: minmax(0, 1fr); gap: 11px; }.amount-control input { font-size: clamp(32px, 11vw, 38px); }.category-list { flex-wrap: nowrap; padding: 0 2px 5px; overflow-x: auto; overscroll-behavior-inline: contain; scrollbar-width: none; }.category-list::-webkit-scrollbar { display: none; }.category-list button { flex: 0 0 auto; }.form-field > span { max-width: 100%; overflow-wrap: anywhere; }.native-input-wrap { min-height: 40px; }.submit-button { min-height: 52px; } }
</style>
