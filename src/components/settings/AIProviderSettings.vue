<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { fetchAIProviders, saveAIProvider, deleteAIProvider, activateAIProvider } from '../../api/system'

const providers = ref([])
const selected = ref('deepseek')
const active = ref('env')
const envConfigured = ref(false)
const envModel = ref('')
const encryptionReady = ref(false)
const loading = ref(false)
const saving = ref(false)
const initialized = ref(false)
const form = reactive({ api_key: '', model: '', region: 'cn' })
const current = computed(() => providers.value.find(item => item.provider === selected.value))
const hasRegions = computed(() => selected.value === 'minimax' || selected.value === 'qwen')

function syncForm() {
  form.api_key = ''
  form.model = current.value?.model || current.value?.default_model || ''
  form.region = current.value?.region || 'cn'
}

watch(selected, syncForm)

async function load() {
  loading.value = true
  try {
    const response = await fetchAIProviders()
    const data = response.data.data
    providers.value = data.providers || []
    active.value = data.active
    envConfigured.value = data.env_configured
    envModel.value = data.env_model || ''
    encryptionReady.value = data.encryption_ready
    if (!initialized.value && data.active !== 'env') selected.value = data.active
    initialized.value = true
    syncForm()
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || 'AI 配置加载失败')
  } finally { loading.value = false }
}

async function activate(provider) {
  try {
    await activateAIProvider(provider)
    await load()
    ElMessage.success('已切换拍照识别服务')
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '切换 AI 服务失败')
  }
}

async function save(activateAfterSave) {
  if (!form.model.trim()) return ElMessage.warning('请填写模型名称')
  saving.value = true
  try {
    await saveAIProvider(selected.value, {
      api_key: form.api_key.trim(), model: form.model.trim(), region: form.region
    })
    form.api_key = ''
    if (activateAfterSave) await activateAIProvider(selected.value)
    await load()
    ElMessage.success(activateAfterSave ? '配置已保存并启用' : '配置已保存')
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || 'AI 配置保存失败')
  } finally { saving.value = false }
}

async function removeKey() {
  try {
    await ElMessageBox.confirm(`删除 ${current.value?.name} 的密钥？`, '删除 API Key', { type: 'warning' })
  } catch { return }
  try {
    await deleteAIProvider(selected.value)
    await load()
    ElMessage.success('密钥已删除')
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '删除失败')
  }
}

onMounted(load)
</script>

<template>
  <el-card shadow="never" class="!rounded-[2rem] border-slate-200/60" v-loading="loading">
    <h3 class="text-lg font-bold mb-2">AI 图片识别</h3>
    <p class="text-sm text-slate-500 mb-5">为拍照订单和截图记账配置服务商。密钥只保存于服务端，离开此页后不会再次显示。</p>
    <el-alert v-if="!encryptionReady" type="warning" :closable="false" title="请先在服务端设置 AI_CONFIG_SECRET（至少 24 位），或配置安全的 JWT_SECRET，再保存 API Key。" class="mb-4" />
    <div class="provider-tabs">
      <button v-for="provider in providers" :key="provider.provider" type="button" :class="{ chosen: selected === provider.provider }" @click="selected = provider.provider">
        {{ provider.name }} <small>{{ provider.has_key ? (provider.active ? '使用中' : '已配置') : '未配置' }}</small>
      </button>
    </div>
    <div v-if="current" class="provider-form">
      <el-alert v-if="current.has_key && !current.key_usable" type="warning" :closable="false" title="已保存的 Key 无法用当前服务端加密密钥解密；请重新输入 API Key，并点击“保存配置”或“保存并启用”。若仍保留原加密密钥，也可以恢复原值以继续使用旧 Key。" />
      <label>API Key <span>{{ current.has_key ? (current.key_usable ? '已配置；留空则保持原 Key' : '旧 Key 无法解密；请重新输入') : '必填' }}</span>
        <el-input v-model="form.api_key" type="password" show-password autocomplete="new-password" placeholder="在此输入服务商 API Key" />
      </label>
      <label>图片识别模型 <span>需要支持图片输入；可修改为账号可用的模型</span><el-input v-model="form.model" maxlength="100" /></label>
      <label v-if="hasRegions">服务区域
        <el-select v-model="form.region" class="w-full">
          <el-option label="中国大陆" value="cn" /><el-option :label="selected === 'qwen' ? '新加坡' : '国际站'" value="global" />
        </el-select>
      </label>
      <div class="provider-actions">
        <el-button :loading="saving" :disabled="!encryptionReady" @click="save(false)">保存配置</el-button>
        <el-button type="primary" :loading="saving" :disabled="!encryptionReady" @click="save(true)">保存并启用</el-button>
        <el-button v-if="current.has_key && !current.active" type="success" plain :disabled="!current.key_usable" @click="activate(selected)">设为当前服务</el-button>
        <el-button v-if="current.has_key" type="danger" text @click="removeKey">删除 Key</el-button>
      </div>
    </div>
    <div v-if="envConfigured" class="env-fallback">
      <span>环境变量配置：{{ envModel }} {{ active === 'env' ? '· 当前使用' : '' }}</span>
      <el-button v-if="active !== 'env'" text type="primary" @click="activate('env')">切回环境变量</el-button>
    </div>
    <p v-else-if="active === 'env'" class="env-fallback">尚未启用服务商；请保存 Key 后点击“保存并启用”。</p>
  </el-card>
</template>

<style scoped>
.provider-tabs{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px;margin-bottom:18px}.provider-tabs button{border:1px solid #dbe2ea;border-radius:12px;background:#f8fafc;padding:11px 8px;text-align:left;font-weight:700;cursor:pointer}.provider-tabs button.chosen{border-color:#7a70ff;background:#eef0ff;color:#4b44c8}.provider-tabs small{display:block;color:#64748b;font-size:11px;font-weight:400}.provider-form{display:grid;gap:14px}.provider-form label{display:grid;gap:6px;font-size:13px;font-weight:600;color:#334155}.provider-form label span{font-size:11px;font-weight:400;color:#64748b}.provider-actions{display:flex;flex-wrap:wrap;gap:7px}.env-fallback{display:flex;align-items:center;justify-content:space-between;margin-top:18px;padding:12px;border-radius:10px;background:#f8fafc;color:#64748b;font-size:12px}@media(max-width:650px){.provider-tabs{grid-template-columns:repeat(2,minmax(0,1fr))}}
</style>
