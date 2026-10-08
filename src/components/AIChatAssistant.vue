<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AgentEvidence from './agent/AgentEvidence.vue'
import AgentActionCard from './agent/AgentActionCard.vue'
import AgentMemoryPanel from './agent/AgentMemoryPanel.vue'
import AgentFeedback from './agent/AgentFeedback.vue'
import { useAIAgent } from '../composables/useAIAgent'

const route = useRoute()
const router = useRouter()
const agent = useAIAgent()
const open = ref(false)
const memoryOpen = ref(false)
const draft = ref('')
const list = ref(null)
const dismissedBrief = ref(false)
const sheetOffset = ref(0)
const isDraggingSheet = ref(false)
let dragStartY = 0
const suggestions = ['张三买过什么？', '近 30 天利润是多少？', '查询员工上月提成', '有哪些未收尾款？', '帮我记一笔支出']
const sheetStyle = computed(() => ({ transform: `translateY(${sheetOffset.value}px)` }))

onMounted(() => agent.start().catch(() => { agent.error.value = 'Agent 初始化失败，请重新打开页面。' }))
watch(() => agent.messages.value.length, async () => {
  await nextTick()
  if (list.value) list.value.scrollTop = list.value.scrollHeight
})

function sourcesOf(message) {
  try { return JSON.parse(message.sources || '[]').join('、') } catch { return '' }
}

function send(value) {
  const message = (value || draft.value).trim()
  if (!message || !agent.sessionId.value) return
  draft.value = ''
  agent.send(message, route.path)
}

function actionFor(message) {
  return agent.actions.value.find(action => action.id === message.action_id)
}

function navigate(path) {
  router.push(path)
  open.value = false
}

function isMobileSheet() {
  return window.matchMedia('(max-width: 767px)').matches
}

function startSheetDrag(event) {
  if (!isMobileSheet() || (event.target instanceof Element && event.target.closest('button'))) return
  dragStartY = event.clientY
  isDraggingSheet.value = true
  event.currentTarget.setPointerCapture?.(event.pointerId)
}

function moveSheetDrag(event) {
  if (!isDraggingSheet.value) return
  sheetOffset.value = Math.min(180, Math.max(0, event.clientY - dragStartY))
}

function finishSheetDrag() {
  if (!isDraggingSheet.value) return
  isDraggingSheet.value = false
  if (sheetOffset.value >= 96) open.value = false
  sheetOffset.value = 0
}

onUnmounted(() => { isDraggingSheet.value = false })
</script>

<template>
  <div class="ai-assistant" :class="{ open }">
    <button v-if="open" type="button" class="ai-backdrop" aria-label="关闭 AI 助手" @click="open = false"></button>
    <section v-if="open" aria-label="ERP Agent" class="ai-panel mb-3 flex h-[min(78vh,700px)] w-[min(calc(100vw-2rem),440px)] flex-col overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-2xl" :class="{ 'is-empty': !agent.messages.value.length, 'is-dragging': isDraggingSheet }" :style="sheetStyle">
      <div class="ai-sheet-handle" aria-hidden="true"></div>
      <header class="ai-panel-header flex items-center justify-between bg-slate-900 px-5 py-4 text-white" @pointerdown="startSheetDrag" @pointermove="moveSheetDrag" @pointerup="finishSheetDrag" @pointercancel="finishSheetDrag">
        <div><div class="font-semibold">ERP Agent</div><div class="text-xs text-slate-300">查询 · 草稿 · 确认执行 · 待办</div></div>
        <div class="flex gap-2"><button class="rounded-lg px-2 py-1 text-xs hover:bg-white/10" @click="memoryOpen = !memoryOpen">长期记忆</button><button class="ai-new-session rounded-lg px-2 py-1 text-xs hover:bg-white/10" @click="agent.newSession()">新对话</button><button aria-label="关闭 Agent" class="ai-close rounded-lg p-2 hover:bg-white/10" @click="open = false">✕</button></div>
      </header>
      <AgentMemoryPanel v-if="memoryOpen" @close="memoryOpen = false" />
      <div v-if="!memoryOpen" ref="list" class="ai-message-list flex-1 space-y-3 overflow-y-auto bg-slate-50 p-4" aria-live="polite">
        <div v-if="!agent.messages.value.length" class="ai-welcome rounded-2xl border bg-white p-4 text-sm text-slate-700">你好！我可以连续查询业务数据、准备订单或支出草稿，也会提示需要关注的待办。</div>
        <section v-if="!dismissedBrief && agent.brief.value.some(item => item.count > 0)" class="ai-brief rounded-2xl border border-amber-200 bg-amber-50 p-3 text-sm">
          <div class="mb-2 flex justify-between font-semibold text-amber-900"><span>当前待办</span><button class="font-normal" aria-label="收起待办" @click="dismissedBrief = true">收起</button></div>
          <button v-for="item in agent.brief.value.filter(item => item.count > 0)" :key="item.key" class="block py-1 text-left text-amber-800 hover:underline" @click="navigate(item.path)">{{ item.label }} {{ item.count }} 项 →</button>
        </section>
        <div v-for="message in agent.messages.value" :key="message.id" :class="message.role === 'user' ? 'flex justify-end' : ''">
          <div :class="['max-w-[95%] whitespace-pre-wrap break-words rounded-2xl px-4 py-3 text-sm leading-6', message.role === 'user' ? 'bg-slate-900 text-white' : 'border border-slate-200 bg-white text-slate-700']">
            {{ message.content }}
            <button v-if="message.role === 'assistant' && message.task_id" type="button" class="mt-2 block text-xs font-medium text-blue-700 underline" @click="navigate('/admin/ai-manager')">查看持久任务 #{{ message.task_id }} →</button>
            <AgentEvidence :evidence="message.evidence" :status="message.status" />
            <div v-if="!message.evidence && sourcesOf(message)" class="mt-2 border-t pt-2 text-xs text-slate-400">数据来源：{{ sourcesOf(message) }}</div>
            <AgentFeedback v-if="message.role === 'assistant' && message.run_id" :message-id="message.id" :saved="agent.feedbacks.value[message.id]" :submit="agent.submitFeedback" />
          </div>
          <AgentActionCard v-if="actionFor(message)?.status === 'pending'" :key="actionFor(message).id" class="mt-2" :action="actionFor(message)" :submit="agent.confirm" />
          <div v-else-if="actionFor(message)?.status === 'saved'" class="mt-2 rounded-xl bg-green-50 p-2 text-xs text-green-700">已确认保存 · 记录 #{{ actionFor(message).target_id || '已创建' }}</div>
        </div>
        <div v-if="agent.loading.value" class="text-sm text-slate-400">正在理解问题、核对数据并整理结论…</div>
        <div v-if="agent.error.value" class="rounded-xl bg-red-50 p-3 text-sm text-red-600">{{ agent.error.value }}</div>
      </div>
      <div v-if="!memoryOpen && !agent.messages.value.length" class="ai-suggestions flex flex-wrap gap-2 border-t px-4 pt-3"><button v-for="question in suggestions" :key="question" class="rounded-full border border-slate-200 px-3 py-1.5 text-xs text-slate-600" @click="send(question)">{{ question }}</button></div>
      <div v-if="!memoryOpen" class="ai-quick-actions flex gap-3 px-4 pb-2 text-xs text-blue-600"><button @click="navigate('/admin/ai-import?kind=order')">拍照录订单 →</button><button @click="navigate('/admin/ai-import?kind=expense')">截图记支出 →</button></div>
      <div v-if="!memoryOpen" class="flex items-center gap-2 border-t px-4 py-2 text-xs text-slate-600"><label for="agent-thinking">思考等级</label><select id="agent-thinking" v-model="agent.thinking.value" :disabled="agent.loading.value" class="rounded border px-2 py-1"><option value="auto">自动</option><option value="quick">快速查询</option><option value="standard">标准分析</option><option value="deep">深度分析</option></select><span>复杂分析可能需要更长时间</span></div>
      <form v-if="!memoryOpen" class="ai-composer flex items-end gap-2 border-t bg-white p-3" @submit.prevent="send()">
        <textarea v-model="draft" rows="2" maxlength="500" aria-label="向 Agent 下达任务" placeholder="例如：查张三订单和尾款…" class="ai-input max-h-28 min-h-12 flex-1 resize-none rounded-xl border p-3 text-sm outline-none focus:border-blue-500" @keydown.enter.exact.prevent="send()" />
        <button type="submit" :disabled="agent.loading.value || !draft.trim() || !agent.sessionId.value" class="ai-send rounded-xl bg-slate-900 px-4 py-3 text-sm text-white disabled:opacity-40">发送</button>
      </form>
    </section>
    <button v-show="!open" class="ai-launcher" aria-label="打开 Agent" @click="open = true">
      <span aria-hidden="true">{{ open ? '✕' : 'AI' }}</span>
    </button>
  </div>
</template>

<style scoped>
.ai-assistant {
  position: fixed;
  right: 28px;
  bottom: 28px;
  z-index: 90;
}

.ai-backdrop,
.ai-sheet-handle {
  display: none;
}

.ai-launcher {
  display: flex;
  width: 56px;
  height: 56px;
  margin-left: auto;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(255, 255, 255, .7);
  border-radius: 999px;
  color: #fff;
  background: linear-gradient(145deg, #2563eb, #06b6d4);
  box-shadow: 0 14px 30px rgba(37, 99, 235, .28);
  font-size: 18px;
  font-weight: 800;
  transition: transform .2s ease, box-shadow .2s ease;
}

.ai-launcher:hover {
  transform: translateY(-2px);
  box-shadow: 0 18px 34px rgba(37, 99, 235, .34);
}

@media (max-width: 767px) {
  .ai-assistant {
    right: auto;
    bottom: max(16px, env(safe-area-inset-bottom));
    left: 14px;
  }

  .ai-assistant.open {
    inset: 0;
    display: flex;
    align-items: flex-end;
    padding-top: max(12px, env(safe-area-inset-top));
  }

  .ai-backdrop {
    position: absolute;
    inset: 0;
    display: block;
    border: 0;
    background: rgba(15, 23, 42, .38);
    backdrop-filter: blur(3px);
  }

  .ai-panel {
    position: relative;
    width: 100%;
    height: min(90dvh, 820px);
    max-height: calc(100dvh - max(12px, env(safe-area-inset-top)));
    margin: 0;
    border-width: 1px 0 0;
    border-radius: 24px 24px 0 0;
    box-shadow: 0 -18px 50px rgba(15, 23, 42, .2);
    transition: transform .22s ease-out;
  }

  .ai-panel.is-empty {
    height: min(88dvh, 760px);
    max-height: calc(100dvh - max(12px, env(safe-area-inset-top)));
  }

  .ai-panel.is-dragging {
    transition: none;
  }

  .ai-sheet-handle {
    position: absolute;
    top: 8px;
    left: 50%;
    z-index: 2;
    display: block;
    width: 38px;
    height: 4px;
    border-radius: 999px;
    background: rgba(255, 255, 255, .42);
    transform: translateX(-50%);
  }

  .ai-panel-header {
    min-height: 68px;
    padding: 18px 16px 12px;
    touch-action: none;
    cursor: grab;
    user-select: none;
  }

  .ai-panel.is-dragging .ai-panel-header {
    cursor: grabbing;
  }

  .ai-new-session {
    padding: 7px 10px;
    background: rgba(255, 255, 255, .1);
  }

  .ai-close {
    display: grid;
    width: 34px;
    height: 34px;
    place-items: center;
    padding: 0;
    border-radius: 999px;
    background: rgba(255, 255, 255, .12);
    font-size: 18px;
  }

  .ai-message-list {
    min-height: 0;
    padding: 14px;
  }

  .is-empty .ai-message-list {
    flex: 0 1 auto;
    max-height: 310px;
  }

  .ai-welcome {
    padding: 13px 14px;
    border-radius: 16px;
    line-height: 1.65;
  }

  .ai-brief {
    padding: 12px 14px;
    border-radius: 16px;
  }

  .ai-suggestions {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
    padding: 12px 14px 8px;
  }

  .ai-suggestions button {
    min-width: 0;
    padding: 8px 9px;
    overflow: hidden;
    background: #f8fafc;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .ai-quick-actions {
    padding: 4px 14px 10px;
  }

  .ai-composer {
    padding: 10px 14px max(12px, env(safe-area-inset-bottom));
    box-shadow: 0 -8px 24px rgba(15, 23, 42, .05);
  }

  .ai-input {
    min-width: 0;
    min-height: 46px;
    max-height: 92px;
    padding: 11px 12px;
    border-radius: 14px;
  }

  .ai-send {
    min-width: 64px;
    min-height: 46px;
    padding: 0 14px;
    border-radius: 14px;
  }

  .ai-launcher {
    width: 46px;
    height: 46px;
    margin-right: auto;
    margin-left: 0;
    border-width: 2px;
    font-size: 14px;
    box-shadow: 0 10px 24px rgba(37, 99, 235, .26);
  }
}
</style>
