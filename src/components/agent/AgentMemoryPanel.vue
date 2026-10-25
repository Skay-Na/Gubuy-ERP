<script setup>
import { onMounted, ref } from 'vue'
import {
  fetchAgentMemories,
  createAgentMemory,
  updateAgentMemory,
  deleteAgentMemory
} from '../../api/agent'

const emit = defineEmits(['close'])
const memories = ref([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const editingId = ref(null)
const kind = ref('preference')
const content = ref('')

const labels = { profile: '用户资料', preference: '回答偏好', context: '长期背景' }

async function load() {
  loading.value = true
  error.value = ''
  try {
    const response = await fetchAgentMemories()
    memories.value = response.result || []
  } catch {
    error.value = '读取长期记忆失败。'
  } finally {
    loading.value = false
  }
}

function edit(item) {
  editingId.value = item.id
  kind.value = item.kind
  content.value = item.content
}

function clearForm() {
  editingId.value = null
  kind.value = 'preference'
  content.value = ''
}

async function save() {
  if (!content.value.trim() || saving.value) return
  saving.value = true
  error.value = ''
  try {
    const body = { kind: kind.value, content: content.value.trim() }
    if (editingId.value) await updateAgentMemory(editingId.value, body)
    else await createAgentMemory(body)
    clearForm()
    await load()
  } catch (cause) {
    error.value = cause.response?.data?.msg || '保存失败。'
  } finally {
    saving.value = false
  }
}

async function remove(item) {
  error.value = ''
  try {
    await deleteAgentMemory(item.id)
    if (editingId.value === item.id) clearForm()
    await load()
  } catch (cause) {
    error.value = cause.response?.data?.msg || '删除失败。'
  }
}

onMounted(load)
</script>

<template>
  <section class="flex min-h-0 flex-1 flex-col bg-slate-50 text-sm text-slate-700" aria-label="长期记忆">
    <div class="flex items-center justify-between border-b bg-white px-4 py-3">
      <div><strong>长期记忆</strong><p class="text-xs text-slate-500">用于理解你的偏好；业务数字仍实时查询。</p></div>
      <button type="button" class="rounded border px-2 py-1" @click="emit('close')">返回对话</button>
    </div>
    <div class="min-h-0 flex-1 overflow-y-auto p-4">
      <p v-if="error" role="alert" class="mb-3 text-red-600">{{ error }}</p>
      <p v-if="loading">正在读取…</p>
      <p v-else-if="!memories.length" class="text-slate-500">暂无长期记忆。也可以在对话中说“记住……”。</p>
      <ul v-else class="space-y-2">
        <li v-for="item in memories" :key="item.id" class="rounded-xl border bg-white p-3">
          <div class="text-xs text-slate-500">{{ labels[item.kind] || item.kind }}<span v-if="item.source === 'inferred'"> · 对话归纳</span></div>
          <p class="my-1 whitespace-pre-wrap break-words">{{ item.content }}</p>
          <p v-if="item.evidence" class="mb-1 text-xs text-slate-500">依据：{{ item.evidence }}</p>
          <div class="flex gap-3 text-xs text-blue-700">
            <button type="button" @click="edit(item)">编辑</button>
            <button type="button" class="text-red-600" @click="remove(item)">删除</button>
          </div>
        </li>
      </ul>
    </div>
    <form class="space-y-2 border-t bg-white p-4" @submit.prevent="save">
      <div class="flex items-center justify-between"><strong>{{ editingId ? '编辑记忆' : '添加记忆' }}</strong><button v-if="editingId" type="button" class="text-xs text-blue-700" @click="clearForm">取消编辑</button></div>
      <select v-model="kind" aria-label="记忆类型" class="w-full rounded border p-2"><option value="profile">用户资料</option><option value="preference">回答偏好</option><option value="context">长期背景</option></select>
      <textarea v-model="content" aria-label="记忆内容" maxlength="240" rows="2" class="w-full rounded border p-2" placeholder="例如：回答销售问题时先给结论和同比变化" />
      <button type="submit" :disabled="saving || !content.trim()" class="rounded bg-slate-900 px-3 py-2 text-white disabled:opacity-40">{{ saving ? '保存中…' : '保存记忆' }}</button>
    </form>
  </section>
</template>
