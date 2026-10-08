<script setup>
import { ref, watch } from 'vue'

const props = defineProps({
  messageId: { type: Number, required: true },
  saved: { type: Object, default: null },
  submit: { type: Function, required: true },
})

const category = ref('understanding')
const comment = ref('')
const sending = ref(false)
const failed = ref(false)
const correcting = ref(false)

watch(() => props.saved, () => {
  correcting.value = false
  failed.value = false
}, { deep: true })

async function send(rating) {
  sending.value = true
  failed.value = false
  try {
    await props.submit(props.messageId, {
      rating,
      category: rating === 'unhelpful' ? category.value : '',
      comment: rating === 'unhelpful' ? comment.value.trim() : '',
    })
    correcting.value = false
    comment.value = ''
  } catch {
    failed.value = true
  } finally {
    sending.value = false
  }
}
</script>

<template>
  <div class="mt-2 border-t border-slate-100 pt-2 text-xs text-slate-400">
    <template v-if="saved && !correcting">
      <span>{{ saved.rating === 'helpful' ? '已标记有帮助' : '已反馈，感谢纠正' }}</span>
      <button class="ml-2 underline" @click="correcting = true">修改</button>
    </template>
    <template v-else-if="correcting || !saved">
      <div class="flex items-center gap-3">
        <span>这个回答有帮助吗？</span>
        <button :disabled="sending" class="text-blue-600 hover:underline disabled:opacity-50" @click="send('helpful')">有帮助</button>
        <button :disabled="sending" class="text-blue-600 hover:underline disabled:opacity-50" @click="correcting = true">有问题</button>
      </div>
      <div v-if="correcting" class="mt-2 space-y-2">
        <select v-model="category" aria-label="问题类型" class="rounded border px-2 py-1 text-slate-600">
          <option value="understanding">理解有误</option>
          <option value="calculation">数字或口径有误</option>
          <option value="evidence">依据不足</option>
          <option value="forecast">预测不合理</option>
          <option value="failure">运行失败</option>
          <option value="other">其他</option>
        </select>
        <input v-model="comment" maxlength="500" aria-label="补充说明" placeholder="补充说明（选填）" class="w-full rounded border px-2 py-1 text-slate-600" />
        <div class="flex gap-3">
          <button :disabled="sending" class="text-blue-600 hover:underline disabled:opacity-50" @click="send('unhelpful')">提交反馈</button>
          <button class="hover:underline" @click="correcting = false">取消</button>
        </div>
      </div>
      <span v-if="failed" class="mt-1 block text-red-500">反馈未保存，请重试。</span>
    </template>
  </div>
</template>
