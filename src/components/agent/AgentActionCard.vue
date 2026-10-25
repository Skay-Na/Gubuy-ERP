<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { fetchAccounts } from '../../api/finance'
import { fetchProducts } from '../../api/products'

const props = defineProps({ action: { type: Object, required: true }, submit: { type: Function, required: true } })
const form = reactive(JSON.parse(props.action.payload))
const accounts = ref([])
const products = ref([])
const busy = ref(false)
const error = ref('')
const valid = computed(() => props.action.kind === 'expense'
  ? Number(form.amount) > 0 && form.category && Number(form.account_id) > 0
  : form.customer_name && Number(form.account_id) > 0 && Number(form.payment_method) > 0 && Number(form.delivery_method) > 0 && form.items?.length > 0 && form.items.every(item => Number(item.product_id) > 0 && Number(item.quantity) > 0 && Number(item.unit_price) > 0))

onMounted(async () => {
  try {
    const accountResponse = await fetchAccounts()
    accounts.value = accountResponse.result || []
    if (props.action.kind === 'order') {
      const productResponse = await fetchProducts({ all: 1 })
      products.value = productResponse.result || []
      for (const item of form.items) {
        const matches = products.value.filter(product => product.name === item.name)
        if (matches.length === 1) item.product_id = matches[0].id
      }
    }
  } catch { error.value = '基础数据加载失败，请刷新后重试。' }
})

async function confirm() {
  if (!valid.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    const payload = props.action.kind === 'order'
      ? { customer_name: form.customer_name.trim(), customer_phone: form.customer_phone || '', items: form.items.map(item => ({ product_id: Number(item.product_id), quantity: Number(item.quantity), unit_price: Number(item.unit_price) })), payment_method: Number(form.payment_method), delivery_method: Number(form.delivery_method), account_id: Number(form.account_id), deposit_amount: form.payment_mode === 'deposit' ? Number(form.deposit_amount) : 0 }
      : { ...form, entry_type: 2, amount: Number(form.amount), account_id: Number(form.account_id), occurred_at: form.occurred_at ? `${form.occurred_at} 12:00:00` : '' }
    await props.submit(props.action, payload)
  } catch (cause) {
    error.value = cause.response?.data?.msg || '保存失败，请核对草稿后重试。'
  } finally { busy.value = false }
}
</script>

<template>
  <section class="rounded-2xl border border-blue-200 bg-blue-50 p-3 text-sm text-slate-700">
    <div class="mb-2 font-semibold">{{ action.kind === 'order' ? '订单草稿' : '支出草稿' }} · 核对后保存</div>
    <div class="grid gap-2">
      <template v-if="action.kind === 'expense'">
        <input v-model="form.category" maxlength="50" placeholder="支出分类" class="rounded-lg border p-2" />
        <input v-model.number="form.amount" type="number" min="0.01" step="0.01" placeholder="金额" class="rounded-lg border p-2" />
        <input v-model="form.counterparty" maxlength="100" placeholder="收款方" class="rounded-lg border p-2" />
        <input v-model="form.remark" maxlength="255" placeholder="备注" class="rounded-lg border p-2" />
        <input v-model="form.occurred_at" type="date" class="rounded-lg border p-2" />
      </template>
      <template v-else>
        <input v-model="form.customer_name" placeholder="客户姓名" class="rounded-lg border p-2" />
        <input v-model="form.customer_phone" placeholder="客户电话" class="rounded-lg border p-2" />
        <div v-for="(item, index) in form.items" :key="index" class="grid gap-1 rounded-xl border border-blue-200 bg-white p-2">
          <div class="text-xs text-slate-500">识别商品：{{ item.name || '未填写' }}</div>
          <select v-model.number="item.product_id" class="rounded-lg border p-2"><option :value="0">选择实际商品</option><option v-for="product in products" :key="product.id" :value="product.id">{{ product.name }} (#{{ product.id }})</option></select>
          <div class="flex gap-2"><input v-model.number="item.quantity" type="number" min="1" placeholder="数量" class="w-1/2 rounded-lg border p-2" /><input v-model.number="item.unit_price" type="number" min="0.01" step="0.01" placeholder="单价" class="w-1/2 rounded-lg border p-2" /></div>
        </div>
        <select v-model.number="form.delivery_method" class="rounded-lg border p-2"><option :value="0">选择发货方式</option><option :value="1">门店自提</option><option :value="4">云仓发货</option></select>
        <select v-model.number="form.payment_method" class="rounded-lg border p-2"><option :value="0">选择支付方式</option><option :value="1">支付宝</option><option :value="2">微信</option><option :value="3">公户</option><option :value="5">现金</option></select>
        <select v-model="form.payment_mode" class="rounded-lg border p-2"><option value="full">全款</option><option value="deposit">定金</option></select>
        <input v-if="form.payment_mode === 'deposit'" v-model.number="form.deposit_amount" type="number" min="0.01" step="0.01" placeholder="定金金额" class="rounded-lg border p-2" />
      </template>
      <select v-model.number="form.account_id" class="rounded-lg border p-2"><option :value="0">选择资金账户</option><option v-for="account in accounts" :key="account.id" :value="account.id">{{ account.name }}</option></select>
      <div v-if="error" class="text-red-600">{{ error }}</div>
      <button :disabled="!valid || busy" class="rounded-xl bg-blue-600 px-3 py-2 font-medium text-white disabled:opacity-40" @click="confirm">{{ busy ? '保存中…' : '确认并保存' }}</button>
    </div>
  </section>
</template>
