<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import request from '../utils/request'
import { normalizeAIOrderFields } from '../utils/aiOrderImport'
import AIImportRecentList from '../components/ai-import/AIImportRecentList.vue'

const route = useRoute()
const router = useRouter()
const kind = ref(route.query.kind === 'order' ? 'order' : 'expense')
const file = ref(null)
const description = ref('')
const imageURL = ref('')
const draftID = ref(null)
const warnings = ref([])
const products = ref([])
const accounts = ref([])
const employees = ref([])
const orders = ref([])
const recentImports = ref([])
const searchingOrders = ref(false)
const working = ref(false)
const deletingID = ref(null)
const saved = ref(false)
const targetID = ref(null)
const storageKey = () => `erp_ai_import_${kind.value}_draft`
const order = reactive({ customer_name: '', customer_phone: '', customer_address: '', employee_name: '', employee_id: null, amount: null, payment_channel: '', is_subsidy: false, items: [], delivery_method: null, payment_method: null, account_id: null, payment_mode: null, deposit_amount: null })
const expense = reactive({ entry_type: 2, category: '', amount: null, account_id: null, order_id: null, counterparty: '', related_no: '', remark: '', occurred_at: '' })

const total = computed(() => order.items.reduce((sum, item) => sum + Number(item.quantity || 0) * Number(item.unit_price || 0), 0))
const amountMismatch = computed(() => Number(order.amount) > 0 && Math.abs(Number(order.amount) - total.value) > 0.01)
const fileName = computed(() => file.value?.name || '支持 JPG、PNG、WebP，最大 8MB')
const linkedOrder = computed(() => orders.value.find(item => item.id === expense.order_id))
const linkedField = computed(() => ({ '渠道奖励': '渠道奖励', '员工提成': '员工提成', '安装费': '安装费用', '运输配送': '送货费用' })[expense.category])
const channelRewardNeedsOrder = computed(() => String(expense.category || '').trim() === '渠道奖励')
const canSave = computed(() => {
  if (!draftID.value || saved.value || working.value) return false
  if (kind.value === 'expense') return Number(expense.amount) > 0 && expense.category && expense.account_id && (!channelRewardNeedsOrder.value || expense.order_id)
  return order.customer_name && order.delivery_method && order.payment_method && order.account_id && order.payment_mode &&
    (order.payment_mode === 'full' || Number(order.deposit_amount) > 0) && order.items.length > 0 &&
    order.items.every(item => item.product_id && Number(item.quantity) > 0 && Number(item.unit_price) > 0)
})

function setImage(blob) {
  if (imageURL.value) URL.revokeObjectURL(imageURL.value)
  imageURL.value = URL.createObjectURL(blob)
}

function clearDraft() {
  file.value = null
  draftID.value = null
  saved.value = false
  targetID.value = null
  warnings.value = []
  if (imageURL.value) URL.revokeObjectURL(imageURL.value)
  imageURL.value = ''
  localStorage.removeItem(storageKey())
}

function chooseFile(event) {
  clearDraft()
  file.value = event.target.files?.[0] || null
  if (file.value) setImage(file.value)
}

async function loadRecent() {
  const response = await request.get('/api/ai/imports', { params: { kind: kind.value } })
  recentImports.value = response.data.data || []
}

async function switchKind() {
  clearDraft()
  file.value = null
  await loadRecent()
}

async function deleteImport(item) {
  const recordLabel = item.status === 'saved' ? '导入记录' : '草稿'
  try {
    await ElMessageBox.confirm(
      `确定删除${recordLabel} #${item.id} 及其原图吗？${item.status === 'saved' ? ' 已生成的订单或账目不会被删除。' : ''}`,
      `删除${recordLabel}`,
      { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  deletingID.value = item.id
  try {
    await request.delete(`/api/ai/imports/${item.id}`)
    recentImports.value = recentImports.value.filter(record => record.id !== item.id)
    if (draftID.value === item.id) clearDraft()
    ElMessage.success(`${recordLabel}及原图已删除`)
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '草稿删除失败')
  } finally {
    deletingID.value = null
  }
}

function applyFields(fields) {
  warnings.value = fields.warnings || []
  if (kind.value === 'order') {
    Object.assign(order, normalizeAIOrderFields(fields, products.value, employees.value))
    if (order.is_subsidy) {
      order.payment_method = 3
      order.account_id = accounts.value.find(account => String(account.name || '').includes('公户'))?.id || null
    }
  } else {
    expense.amount = fields.amount || null
    expense.category = fields.category || ''
    expense.counterparty = fields.counterparty || ''
    expense.occurred_at = fields.occurred_at || ''
    expense.remark = fields.remark || ''
  }
}

function updateRecognizedAmount(value) {
  order.amount = Number(value) > 0 ? Number(value) : null
  if (order.amount && order.items.length === 1) {
    const quantity = Number(order.items[0].quantity) || 1
    order.items[0].unit_price = Number((order.amount / quantity).toFixed(2))
  }
}

async function analyze() {
  if (!file.value) return ElMessage.warning('请先选择图片')
  working.value = true
  try {
    const data = new FormData()
    data.append('kind', kind.value)
    data.append('image', file.value)
    data.append('description', description.value)
    const response = await request.post('/api/ai/imports', data, { timeout: 120000 })
    draftID.value = response.data.data.id
    applyFields(response.data.data.fields)
    localStorage.setItem(storageKey(), String(draftID.value))
    await loadRecent().catch(() => {})
    ElMessage.success('已生成草稿，请逐项核对后保存')
  } catch (error) {
    if (error.response?.status === 409 && error.response.data?.data?.id) {
      await restore(error.response.data.data.id)
    } else {
      ElMessage.error(error.response?.data?.msg || '图片识别失败')
    }
  } finally { working.value = false }
}

async function restore(id) {
  const response = await request.get(`/api/ai/imports/${id}`)
  kind.value = response.data.data.kind
  draftID.value = response.data.data.id
  saved.value = response.data.data.status === 'saved'
  targetID.value = response.data.data.target_id
  applyFields(response.data.data.fields)
  const image = await request.get(`/api/ai/imports/${id}/image`, { responseType: 'blob' })
  setImage(image.data)
  localStorage.setItem(storageKey(), String(id))
  await loadRecent().catch(() => {})
}

async function searchOrders(keyword) {
  searchingOrders.value = true
  try {
    const response = await request.get('/api/bookkeeping/orders', { params: { q: keyword } })
    orders.value = response.data.data || []
  } finally { searchingOrders.value = false }
}

function selectOrder(id) {
  const selected = orders.value.find(item => item.id === id)
  expense.order_id = id || null
  expense.related_no = selected?.order_no || ''
  if (selected?.customer_name) expense.counterparty = selected.customer_name
}

async function save() {
  if (!canSave.value) return
  working.value = true
  try {
    if (kind.value === 'order') {
      const payload = {
        ai_import_id: draftID.value,
        customer_name: order.customer_name.trim(), customer_phone: order.customer_phone.trim(), customer_address: order.customer_address.trim(),
        items: order.items.map(item => ({ product_id: item.product_id, quantity: Number(item.quantity), unit_price: Number(item.unit_price) })),
        delivery_method: order.delivery_method, payment_method: order.payment_method, account_id: order.account_id, is_subsidy: order.is_subsidy,
        employee_id: order.employee_id || undefined,
        deposit_amount: order.payment_mode === 'full' ? 0 : Number(order.deposit_amount)
      }
      const response = await request.post('/api/orders', payload)
      targetID.value = response.data.data.order.id
    } else {
      const payload = {
        ...expense, ai_import_id: draftID.value, amount: Number(expense.amount),
        order_id: expense.order_id || undefined, occurred_at: expense.occurred_at ? `${expense.occurred_at} 12:00:00` : ''
      }
      const response = await request.post('/api/bookkeeping/entries', payload)
      targetID.value = response.data.data.entry.id
    }
    saved.value = true
    await loadRecent().catch(() => {})
    ElMessage.success(kind.value === 'order' ? '订单已保存，原图已关联' : '支出已入账，原图已关联')
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '保存失败，请检查字段')
  } finally { working.value = false }
}

onMounted(async () => {
  try {
    const [productResponse, accountResponse, employeeResponse] = await Promise.all([request.get('/api/products', { params: { all: 1 } }), request.get('/api/accounts'), request.get('/api/employees')])
    products.value = productResponse.data.data || []
    accounts.value = accountResponse.data.data || []
    employees.value = (employeeResponse.data.data || []).filter(employee => !employee.is_outsourced)
    await searchOrders('')
    await loadRecent()
    const id = route.query.draft || localStorage.getItem(storageKey())
    if (id) await restore(id)
  } catch { ElMessage.warning('部分基础数据或草稿加载失败') }
})
onBeforeUnmount(() => { if (imageURL.value) URL.revokeObjectURL(imageURL.value) })
</script>

<template>
  <main class="ai-import-page">
    <header class="ai-header"><div><span class="eyebrow">SMART IMPORT</span><h1>AI 图片导入</h1><p>上传凭证，智能提取并匹配系统数据，核对后再保存。</p></div><el-button round @click="router.back()">返回</el-button></header>
    <section class="import-panel">
      <el-radio-group v-model="kind" class="kind-switch" :disabled="Boolean(draftID)" @change="switchKind"><el-radio-button value="order">手写订单</el-radio-button><el-radio-button value="expense">支出截图</el-radio-button></el-radio-group>
      <div v-if="!draftID" class="upload-area">
        <label class="upload-label"><span class="upload-symbol">＋</span><strong>{{ file ? '已选择图片' : '拍照或选择图片' }}</strong><small>{{ fileName }}</small><input type="file" accept="image/jpeg,image/png,image/webp" capture="environment" @change="chooseFile" /></label>
        <el-input v-model="description" type="textarea" :rows="2" :placeholder="kind === 'expense' ? '补充描述，例如：昨天给张师傅的安装费' : '可补充模糊的型号、客户或经手人（选填）'" />
        <div class="privacy-note">图片仅用于本次业务识别，原图会随订单或账目留档。</div>
        <el-button type="primary" size="large" class="analyze-button" :loading="working" :disabled="!file" @click="analyze">开始识别</el-button>
      </div>
      <div v-else class="draft-bar"><span>草稿 #{{ draftID }} · {{ saved ? '已保存' : '待核对' }}</span><el-button @click="clearDraft">导入另一张</el-button></div>
    </section>
    <AIImportRecentList :items="recentImports" :deleting-id="deletingID" @select="restore" @delete="deleteImport" />
    <div v-if="imageURL" class="review-grid">
      <el-card class="image-card"><template #header>原图</template><img :src="imageURL" alt="待核对的原始图片" /></el-card>
      <el-card v-if="draftID" class="review-card">
        <template #header>电子{{ kind === 'order' ? '订单' : '支出' }}预览 · 请核对每个字段</template>
        <el-alert v-if="warnings.length" type="warning" :closable="false" :title="warnings.join('；')" show-icon />
        <template v-if="kind === 'order'">
          <section class="customer-preview">
            <div class="customer-preview-title">客户信息录入</div>
            <div class="field-grid"><label>客户姓名<el-input v-model="order.customer_name" placeholder="请输入姓名" /></label><label>手机号码<el-input v-model="order.customer_phone" type="tel" placeholder="请输入手机号" /></label></div>
            <label>收货地址<el-input v-model="order.customer_address" placeholder="请输入客户详细地址" /></label>
            <label>开单员工（经手人）<el-select v-model="order.employee_id" filterable clearable placeholder="请选择经手人"><el-option v-for="employee in employees" :key="employee.id" :label="`${employee.name} · ${employee.emp_no}`" :value="employee.id" /></el-select><small v-if="order.employee_name" class="recognized-hint">图片识别：{{ order.employee_name }}</small></label>
          </section>
          <el-alert v-if="order.is_subsidy" type="success" :closable="false" show-icon :title="`已识别${order.payment_channel || '云闪付'}，此订单已确定为国补订单`" />
          <div v-for="(item, index) in order.items" :key="index" class="item-row">
            <div class="item-heading"><span>识别内容：{{ item.name || '未识别商品名称' }}</span><b v-if="item.match_score">已自动匹配 {{ Math.round(item.match_score * 100) }}%</b></div>
            <el-select v-model="item.product_id" filterable placeholder="明确选择系统商品" class="full-width"><el-option v-for="product in products" :key="product.id" :label="`${product.name} (#${product.id})`" :value="product.id" /></el-select>
            <div class="field-grid"><label>数量<el-input-number v-model="item.quantity" :min="1" /></label><label>单价<el-input-number v-model="item.unit_price" :min="0" :precision="2" /></label></div>
            <el-button text type="danger" @click="order.items.splice(index, 1)">删除此项</el-button>
          </div>
          <el-button @click="order.items.push({ name: '', product_id: null, quantity: 1, unit_price: null })">添加商品</el-button>
          <div class="amount-summary"><span>图片识别金额</span><el-input-number :model-value="order.amount" :min="0" :precision="2" :controls="false" @change="updateRecognizedAmount" /><strong>商品合计 ¥{{ total.toFixed(2) }}</strong></div>
          <el-alert v-if="amountMismatch" type="warning" :closable="false" show-icon title="图片识别金额与商品合计不一致，请核对每项成交单价" />
          <div class="field-grid"><label>发货方式<el-select v-model="order.delivery_method" placeholder="请选择"><el-option label="门店自提" :value="1" /><el-option label="云仓发货" :value="4" /></el-select></label><label>收款方式<el-select v-model="order.payment_method" placeholder="请选择"><el-option label="支付宝" :value="1" /><el-option label="微信" :value="2" /><el-option label="公户" :value="3" /><el-option label="现金" :value="5" /></el-select></label></div>
          <div class="field-grid"><label>收款账户<el-select v-model="order.account_id" placeholder="请选择"><el-option v-for="account in accounts" :key="account.id" :label="account.name" :value="account.id" /></el-select></label><label>收款情况<el-select v-model="order.payment_mode" placeholder="请确认"><el-option label="已收全款" value="full" /><el-option label="只收定金" value="deposit" /></el-select></label></div>
          <small>请核对收款方式与账户是否一致。</small>
          <label v-if="order.payment_mode === 'deposit'">已收定金<el-input-number v-model="order.deposit_amount" :min="0.01" :precision="2" /></label>
          <el-alert type="warning" :closable="false" :title="order.is_subsidy ? '此单将按国补订单保存；请重点核对成交金额与公户账户。' : '此入口会按上述收款情况更新资金与库存；以旧换新或混合支付订单请使用原开单流程。'" />
        </template>
        <template v-else>
          <div class="field-grid"><label>支出金额<el-input-number v-model="expense.amount" :min="0" :precision="2" /></label><label>发生日期<el-date-picker v-model="expense.occurred_at" type="date" value-format="YYYY-MM-DD" placeholder="未识别则留空" /></label></div>
          <div class="field-grid"><label>分类<el-input v-model="expense.category" /></label><label>付款账户<el-select v-model="expense.account_id" placeholder="必须选择"><el-option v-for="account in accounts" :key="account.id" :label="account.name" :value="account.id" /></el-select></label></div>
          <label>往来对象<el-input v-model="expense.counterparty" /></label>
          <label>关联订单（{{ channelRewardNeedsOrder ? '渠道奖励必填' : '选填' }}）<el-select :model-value="expense.order_id" filterable remote clearable :remote-method="searchOrders" :loading="searchingOrders" :placeholder="channelRewardNeedsOrder ? '选择本次奖励对应订单' : '不关联订单也可保存'" @change="selectOrder"><el-option v-for="option in orders" :key="option.id" :label="`${option.order_no} · ${option.customer_name}`" :value="option.id" /></el-select></label>
          <el-alert v-if="linkedOrder && channelRewardNeedsOrder" type="info" :closable="false" :title="`保存后将订单 ${linkedOrder.order_no} 的渠道奖励记为实际支出 ¥${Number(expense.amount || 0).toFixed(2)}，并标记已支出`" />
          <el-alert v-else-if="linkedOrder && linkedField" type="info" :closable="false" :title="`保存后将增加订单 ${linkedOrder.order_no} 的${linkedField} ¥${Number(expense.amount || 0).toFixed(2)}`" />
          <el-alert v-else-if="channelRewardNeedsOrder" type="warning" :closable="false" title="渠道奖励支出必须关联对应订单" />
          <label>备注<el-input v-model="expense.remark" type="textarea" /></label>
        </template>
        <el-button v-if="!saved" type="primary" size="large" :disabled="!canSave" :loading="working" @click="save">确认并保存</el-button>
        <el-alert v-else type="success" :closable="false" :title="`已保存，记录编号 #${targetID}；原图保存在草稿 #${draftID}`" />
      </el-card>
    </div>
  </main>
</template>

<style scoped>
.ai-import-page{width:100%;max-width:1120px;min-width:0;margin:auto;padding:28px;display:grid;grid-template-columns:minmax(0,1fr);gap:18px;box-sizing:border-box}.ai-import-page>*{min-width:0;max-width:100%}.ai-header{display:flex;justify-content:space-between;align-items:center;min-width:0;padding:4px}.ai-header>div{min-width:0}.ai-header h1{margin:3px 0 4px;color:#0f172a;font-size:30px;letter-spacing:-.04em}.ai-header p{margin:0;color:#64748b}.eyebrow{color:#2563eb;font-size:11px;font-weight:800;letter-spacing:.18em}.import-panel{width:100%;min-width:0;padding:22px;border:1px solid #e2e8f0;border-radius:20px;background:#fff;box-shadow:0 16px 40px rgba(15,23,42,.07);box-sizing:border-box;overflow:hidden}.kind-switch{max-width:100%;margin-bottom:18px}.upload-area{min-width:0;display:grid;grid-template-columns:minmax(0,1fr);gap:14px}.upload-label{min-width:0;display:grid;min-height:160px;place-items:center;align-content:center;gap:7px;border:1.5px dashed #93c5fd;border-radius:16px;background:linear-gradient(145deg,#eff6ff,#f8fbff);cursor:pointer;text-align:center;color:#1d4ed8;transition:.2s}.upload-label:hover{border-color:#3b82f6;transform:translateY(-1px)}.upload-label input{position:absolute;width:1px;height:1px;opacity:0}.upload-label strong{font-size:17px}.upload-label small{max-width:90%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#64748b}.upload-symbol{display:grid;width:42px;height:42px;place-items:center;border-radius:14px;background:#2563eb;color:#fff;font-size:26px;box-shadow:0 8px 18px rgba(37,99,235,.25)}.privacy-note{padding-left:2px;color:#94a3b8;font-size:12px;overflow-wrap:anywhere}.analyze-button{width:100%;border-radius:12px}.draft-bar{display:flex;min-width:0;justify-content:space-between;align-items:center;gap:10px}.review-grid{min-width:0;display:grid;grid-template-columns:minmax(0,.85fr) minmax(0,1.25fr);gap:18px;align-items:start}.image-card,.review-card{min-width:0;max-width:100%;overflow:hidden;border-radius:18px}.image-card img{display:block;width:100%;max-width:100%;height:auto;margin:auto;border-radius:10px;object-fit:contain}.review-card :deep(.el-card__body){min-width:0;display:grid;grid-template-columns:minmax(0,1fr);gap:15px}.review-card :deep(.el-input),.review-card :deep(.el-input-number),.review-card :deep(.el-select),.review-card :deep(.el-date-editor){width:100%;min-width:0;max-width:100%}.review-card label{min-width:0;display:grid;gap:6px;font-size:13px;color:#475569}.field-grid{min-width:0;display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:12px}.item-row{min-width:0;padding:14px;border:1px solid #e2e8f0;border-radius:14px;background:#fff;display:grid;grid-template-columns:minmax(0,1fr);gap:10px}.item-heading{min-width:0;display:flex;align-items:center;justify-content:space-between;gap:10px;color:#64748b;font-size:12px;overflow-wrap:anywhere}.item-heading b{flex:none;padding:4px 8px;border-radius:999px;background:#ecfdf5;color:#059669;font-size:11px}.full-width{width:100%;min-width:0}.customer-preview{min-width:0;display:grid;grid-template-columns:minmax(0,1fr);gap:12px;padding:16px;border:1px solid rgba(191,219,254,.7);border-radius:14px;background:rgba(239,246,255,.65)}.customer-preview-title{font-size:14px;font-weight:800;color:#334155}.recognized-hint{color:#2563eb}.amount-summary{min-width:0;display:grid;grid-template-columns:auto minmax(120px,1fr) auto;gap:12px;align-items:center;padding:12px;border-radius:12px;background:#f8fafc;color:#475569}.amount-summary strong{color:#0f172a}@media(max-width:760px){.ai-import-page{padding:14px;gap:14px}.ai-header{align-items:flex-start}.ai-header h1{font-size:24px}.ai-header p{max-width:240px;font-size:13px;white-space:normal}.import-panel{padding:14px;border-radius:16px}.kind-switch{display:flex;width:100%}.kind-switch :deep(.el-radio-button){min-width:0;flex:1}.kind-switch :deep(.el-radio-button__inner){width:100%;padding-inline:8px}.upload-label{min-height:145px}.review-grid,.field-grid{grid-template-columns:minmax(0,1fr)}.image-card img{max-height:50vh}.amount-summary{grid-template-columns:minmax(0,1fr)}.amount-summary strong{text-align:right}.item-heading{align-items:flex-start;flex-direction:column}.review-card :deep(.el-alert__title){white-space:normal;overflow-wrap:anywhere}}
</style>
