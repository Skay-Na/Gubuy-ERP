<script setup>
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import request from '../../utils/request'
import { normalizeAIOrderFields } from '../../utils/aiOrderImport'
import AIImportRecentList from '../ai-import/AIImportRecentList.vue'

const visible = defineModel('visible', { type: Boolean, default: false })
const props = defineProps({ employee: { type: Object, required: true } })
const emit = defineEmits(['apply'])
const file = ref(null)
const description = ref('')
const imageURL = ref('')
const draftID = ref(null)
const draftStatus = ref('')
const warnings = ref([])
const recent = ref([])
const products = ref([])
const working = ref(false)
const deletingID = ref(null)
const form = reactive({ customer_name: '', customer_phone: '', customer_address: '', amount: null, payment_channel: '', is_subsidy: false, items: [] })
const total = computed(() => form.items.reduce((sum, item) => sum + Number(item.quantity || 0) * Number(item.unit_price || 0), 0))
const fileName = computed(() => file.value?.name || '支持 JPG、PNG、WebP，最大 8MB')
const amountMismatch = computed(() => Number(form.amount) > 0 && Math.abs(Number(form.amount) - total.value) > 0.01)
const canApply = computed(() => draftID.value && draftStatus.value === 'pending' && form.customer_name.trim() && form.items.length > 0 && form.items.every(item => item.product_id && Number(item.quantity) > 0 && Number(item.unit_price) > 0))

function setImage(blob) {
  if (imageURL.value) URL.revokeObjectURL(imageURL.value)
  imageURL.value = URL.createObjectURL(blob)
}

function resetDraft() {
  file.value = null
  draftID.value = null
  draftStatus.value = ''
  warnings.value = []
  form.customer_name = ''
  form.customer_phone = ''
  form.customer_address = ''
  form.employee_name = ''
  form.employee_id = null
  form.amount = null
  form.payment_channel = ''
  form.is_subsidy = false
  form.items = []
  if (imageURL.value) URL.revokeObjectURL(imageURL.value)
  imageURL.value = ''
}

function chooseFile(event) {
  resetDraft()
  file.value = event.target.files?.[0] || null
  if (file.value) setImage(file.value)
}

function applyFields(fields) {
  warnings.value = fields.warnings || []
  Object.assign(form, normalizeAIOrderFields(fields, products.value, [props.employee]))
}

async function loadRecent() {
  const response = await request.get('/api/ai/imports', { params: { kind: 'order' } })
  recent.value = response.result || []
  const current = recent.value.find(item => item.id === draftID.value)
  if (current) draftStatus.value = current.status
}

async function restore(id) {
  working.value = true
  try {
    const [draftResponse, imageResponse] = await Promise.all([
      request.get(`/api/ai/imports/${id}`),
      request.get(`/api/ai/imports/${id}/image`, { responseType: 'blob' })
    ])
    resetDraft()
    draftID.value = draftResponse.result.id
    draftStatus.value = draftResponse.result.status
    applyFields(draftResponse.result.fields)
    setImage(imageResponse.data)
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '草稿恢复失败')
  } finally { working.value = false }
}

async function analyze() {
  if (!file.value || working.value) return
  working.value = true
  try {
    const data = new FormData()
    data.append('kind', 'order')
    data.append('image', file.value)
    data.append('description', description.value)
    const response = await request.post('/api/ai/imports', data, { timeout: 120000 })
    draftID.value = response.result.id
    draftStatus.value = 'pending'
    applyFields(response.result.fields)
    await loadRecent()
    ElMessage.success('识别完成，请核对后放入收银台')
  } catch (error) {
    if (error.response?.status === 409 && error.response.data?.data?.id) {
      await restore(error.response.data.data.id)
    } else {
      ElMessage.error(error.response?.data?.msg || '图片识别失败')
    }
  } finally { working.value = false }
}

async function deleteDraft(item) {
  const recordLabel = item.status === 'saved' ? '导入记录' : '草稿'
  try {
    await ElMessageBox.confirm(
      `确定删除${recordLabel} #${item.id} 及其原图吗？${item.status === 'saved' ? ' 已生成的订单不会被删除。' : ''}`,
      `删除${recordLabel}`,
      { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' }
    )
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error('删除操作未完成')
    return
  }

  deletingID.value = item.id
  try {
    await request.delete(`/api/ai/imports/${item.id}`)
    recent.value = recent.value.filter(record => record.id !== item.id)
    if (draftID.value === item.id) resetDraft()
    ElMessage.success(`${recordLabel}及原图已删除`)
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '草稿删除失败')
  } finally {
    deletingID.value = null
  }
}

function updateRecognizedAmount(value) {
  form.amount = Number(value) > 0 ? Number(value) : null
  if (form.amount && form.items.length === 1) {
    const quantity = Number(form.items[0].quantity) || 1
    form.items[0].unit_price = Number((form.amount / quantity).toFixed(2))
  }
}

function applyToCheckout() {
  if (!canApply.value) return
  emit('apply', { id: draftID.value, is_subsidy: form.is_subsidy, customer: { name: form.customer_name.trim(), phone: form.customer_phone.trim(), address: form.customer_address.trim() }, items: form.items.map(item => ({ product: products.value.find(product => product.id === Number(item.product_id)), quantity: Number(item.quantity), unit_price: Number(item.unit_price) })) })
}

watch(visible, async isOpen => {
  if (!isOpen) return
  try {
    const [productResponse] = await Promise.all([request.get('/api/products', { params: { all: 1 } }), loadRecent()])
    products.value = productResponse.result || []
  } catch { ElMessage.warning('商品或草稿加载失败，请重试') }
})
onBeforeUnmount(() => { if (imageURL.value) URL.revokeObjectURL(imageURL.value) })
</script>

<template>
  <el-dialog v-model="visible" title="拍照录入订单" width="min(96vw, 1080px)" :close-on-click-modal="false" class="staff-photo-order">
    <div class="photo-order-page">
      <section class="photo-import-panel">
        <div class="photo-panel-heading">
          <div>
            <span class="photo-eyebrow">SMART IMPORT</span>
            <h2>拍照录入订单</h2>
            <p>上传手写订单，智能提取并匹配系统数据，核对后放入收银台。</p>
          </div>
          <span class="handler-pill">经手人 · {{ props.employee.name }}</span>
        </div>

        <div class="photo-upload-card">
          <label class="photo-drop-zone">
            <input type="file" accept="image/jpeg,image/png,image/webp" capture="environment" @change="chooseFile" />
            <span class="upload-symbol">＋</span>
            <strong>拍照或选择图片</strong>
            <small>{{ fileName }}</small>
          </label>
          <el-input v-model="description" type="textarea" :rows="2" placeholder="可补充模糊的型号、客户或经手人（选填）" maxlength="500" />
          <p class="photo-privacy-note">图片仅用于本次业务识别，原图会随订单草稿留档。</p>
          <el-button type="primary" class="photo-analyze-button" :loading="working" :disabled="!file" @click="analyze">开始识别</el-button>
        </div>
      </section>

      <AIImportRecentList :items="recent" :deleting-id="deletingID" @select="restore" @delete="deleteDraft" />

      <div v-if="imageURL" class="photo-review-grid">
        <section class="photo-original-card">
          <div class="photo-card-title"><span>原图</span><small v-if="draftID">草稿 #{{ draftID }}</small></div>
          <img :src="imageURL" alt="待核对的订单原图" />
        </section>

        <section v-if="draftID" class="photo-review-card">
          <div class="photo-card-title"><div><span>订单预览</span><small>请核对每个字段后放入收银台</small></div><b :class="{ saved: draftStatus === 'saved' }">{{ draftStatus === 'saved' ? '已保存' : '待核对' }}</b></div>
          <div v-for="warning in warnings" :key="warning" class="photo-warning">{{ warning }}</div>

          <div class="customer-preview">
            <div class="customer-preview-title">客户信息录入</div>
            <label>客户姓名<el-input v-model="form.customer_name" placeholder="请输入姓名" /></label>
            <label>手机号码<el-input v-model="form.customer_phone" type="tel" placeholder="请输入手机号" /></label>
            <label class="customer-wide">收货地址<el-input v-model="form.customer_address" placeholder="请输入客户详细地址" /></label>
            <label class="customer-wide">开单员工（经手人）<span class="handler-value">{{ props.employee.name }}</span></label>
            <p v-if="form.employee_name && form.employee_name !== props.employee.name" class="employee-recognition">图片识别：{{ form.employee_name }}，以当前登录员工为准</p>
          </div>

          <el-alert v-if="form.is_subsidy" type="success" :closable="false" show-icon :title="`已识别${form.payment_channel || '云闪付'}，此订单已确定为国补订单`" />

          <div class="product-list">
            <div v-for="(item, index) in form.items" :key="index" class="product-card">
              <div class="product-heading"><span>识别内容：{{ item.name || '未识别' }}</span><b v-if="item.match_score">已自动匹配 {{ Math.round(item.match_score * 100) }}%</b></div>
              <el-select v-model="item.product_id" filterable placeholder="请选择系统中的商品"><el-option v-for="product in products" :key="product.id" :label="`${product.name} (#${product.id})`" :value="product.id" /></el-select>
              <div class="product-fields"><label>数量<el-input-number v-model="item.quantity" :min="1" :precision="0" /></label><label>单价<el-input-number v-model="item.unit_price" :min="0.01" :precision="2" /></label></div>
            </div>
          </div>

          <div class="amount-summary"><label>图片识别金额<el-input-number :model-value="form.amount" :min="0" :precision="2" @update:model-value="updateRecognizedAmount" /></label><div><span>商品合计</span><strong>¥{{ total.toFixed(2) }}</strong></div></div>
          <el-alert v-if="amountMismatch" type="warning" :closable="false" show-icon title="图片金额与商品合计不一致，请核对数量、单价或识别金额。" />
          <p class="checkout-note">放入收银台后可选择发货和收款方式，确认结算后才会保存订单。</p>
          <el-button type="primary" class="photo-checkout-button" :disabled="!canApply" @click="applyToCheckout">放入收银台核对结算</el-button>
        </section>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped>
.photo-order-page{display:grid;gap:18px;min-width:0;max-width:100%;box-sizing:border-box}.photo-import-panel,.photo-original-card,.photo-review-card{min-width:0;max-width:100%;box-sizing:border-box;border:1px solid #e2e8f0;border-radius:20px;background:#fff;box-shadow:0 12px 32px rgba(15,23,42,.06)}.photo-import-panel{padding:22px}.photo-panel-heading{display:flex;min-width:0;gap:16px;align-items:flex-start;justify-content:space-between;margin-bottom:18px}.photo-panel-heading>div{min-width:0}.photo-eyebrow{display:block;margin-bottom:8px;color:#2563eb;font-size:12px;font-weight:800;letter-spacing:.2em}.photo-panel-heading h2{margin:0;color:#0f172a;font-size:25px;line-height:1.2}.photo-panel-heading p{max-width:620px;margin:8px 0 0;color:#64748b;line-height:1.65}.handler-pill{flex:0 0 auto;padding:7px 10px;border-radius:999px;background:#eff6ff;color:#2563eb;font-size:12px;font-weight:700;white-space:nowrap}.photo-upload-card{display:grid;gap:14px}.photo-drop-zone{display:grid;min-height:166px;place-items:center;align-content:center;gap:5px;box-sizing:border-box;border:2px dashed #60a5fa;border-radius:16px;background:linear-gradient(135deg,#f8fbff,#eff6ff);color:#1d4ed8;cursor:pointer;text-align:center}.photo-drop-zone input{position:absolute;width:1px;height:1px;opacity:0}.upload-symbol{display:grid;width:52px;height:52px;place-items:center;border-radius:17px;background:#2563eb;box-shadow:0 8px 16px rgba(37,99,235,.2);color:#fff;font-size:40px;font-weight:300;line-height:1}.photo-drop-zone strong{font-size:19px}.photo-drop-zone small{max-width:85%;overflow:hidden;color:#64748b;text-overflow:ellipsis;white-space:nowrap}.photo-privacy-note,.checkout-note{margin:0;color:#94a3b8;font-size:12px;line-height:1.55}.photo-analyze-button,.photo-checkout-button{height:44px;font-size:16px}.photo-review-grid{display:grid;grid-template-columns:minmax(0,.82fr) minmax(0,1.18fr);gap:18px;align-items:start;min-width:0;max-width:100%}.photo-original-card{overflow:hidden}.photo-card-title{display:flex;min-width:0;align-items:center;justify-content:space-between;gap:10px;padding:16px 18px;border-bottom:1px solid #eef2f7}.photo-card-title>div{display:grid;gap:3px}.photo-card-title span{color:#0f172a;font-size:17px;font-weight:800}.photo-card-title small{color:#94a3b8;font-size:12px}.photo-card-title>b{padding:5px 8px;border-radius:999px;background:#fff7ed;color:#ea580c;font-size:11px;white-space:nowrap}.photo-card-title>b.saved{background:#ecfdf5;color:#059669}.photo-original-card img{display:block;width:100%;max-height:660px;object-fit:contain;background:#f8fafc}.photo-review-card{display:grid;gap:15px;padding-bottom:18px;overflow:hidden}.photo-review-card>.photo-card-title{margin-bottom:0}.photo-warning{margin:0 18px;padding:10px 12px;border-radius:10px;background:#fffbeb;color:#b45309;font-size:13px}.customer-preview{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;margin:0 18px;padding:16px;border:1px solid #bfdbfe;border-radius:14px;background:#f0f7ff}.customer-preview-title,.customer-wide{grid-column:1/-1}.customer-preview-title{color:#1e3a5f;font-weight:800}.customer-preview label,.amount-summary label,.product-fields label{display:grid;min-width:0;gap:6px;color:#475569;font-size:13px;font-weight:600}.customer-wide{min-width:0}.handler-value{display:block;min-height:32px;padding:8px 11px;box-sizing:border-box;border:1px solid #dbeafe;border-radius:8px;background:#fff;color:#1e3a5f;font-weight:700}.employee-recognition{grid-column:1/-1;margin:0;color:#2563eb;font-size:12px}.product-list{display:grid;gap:10px;margin:0 18px}.product-card{display:grid;gap:10px;padding:13px;border:1px solid #e2e8f0;border-radius:13px}.product-heading{display:flex;min-width:0;align-items:center;justify-content:space-between;gap:8px;color:#64748b;font-size:12px}.product-heading span{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.product-heading b{flex:0 0 auto;padding:5px 8px;border-radius:999px;background:#ecfdf5;color:#059669;font-size:11px}.product-fields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}.amount-summary{display:flex;align-items:end;justify-content:space-between;gap:14px;margin:0 18px;padding:14px;border-radius:13px;background:#f8fafc}.amount-summary>div{display:grid;gap:4px;text-align:right;color:#64748b;font-size:13px}.amount-summary strong{color:#0f172a;font-size:20px}.photo-review-card>:deep(.el-alert){margin:0 18px;width:auto}.photo-checkout-button{margin:0 18px}.photo-review-card>.checkout-note{margin:0 18px}:deep(.el-input-number),:deep(.el-select){width:100%;min-width:0}:deep(.el-dialog__body){overflow-x:hidden}@media(max-width:720px){.photo-import-panel{padding:16px}.photo-panel-heading{display:grid;gap:12px}.handler-pill{justify-self:start}.photo-panel-heading h2{font-size:22px}.photo-review-grid{grid-template-columns:minmax(0,1fr)}.photo-original-card img{max-height:440px}.customer-preview{grid-template-columns:minmax(0,1fr);margin:0 13px;padding:13px}.customer-preview-title,.customer-wide{grid-column:auto}.employee-recognition{grid-column:auto}.product-list,.photo-warning,.amount-summary,.photo-review-card>:deep(.el-alert),.photo-checkout-button,.photo-review-card>.checkout-note{margin-left:13px;margin-right:13px}.amount-summary{display:grid;align-items:stretch}.amount-summary>div{text-align:left}.photo-card-title{padding:14px}.photo-drop-zone{min-height:150px}.photo-review-card{gap:13px}}
</style>
