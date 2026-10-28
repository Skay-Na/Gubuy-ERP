<script setup>
import { computed, nextTick, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import AgentEvidence from './agent/AgentEvidence.vue'
import AgentActionCard from './agent/AgentActionCard.vue'
import AgentAttachmentPicker from './agent/AgentAttachmentPicker.vue'
import AgentMemoryPanel from './agent/AgentMemoryPanel.vue'
import AgentFeedback from './agent/AgentFeedback.vue'
import { useAIAgent } from '../composables/useAIAgent'
import { appendAgentAttachments, formatAgentAttachmentSize } from '../utils/agentAttachments'

const route = useRoute()
const router = useRouter()
const agent = useAIAgent()
const memoryOpen = ref(false)
const draft = ref('')
const queuedAttachments = shallowRef([])
const list = ref(null)
const dismissedBrief = ref(false)
const effortOpen = ref(false)
const thinkingLevels = [
  { value: 'auto', label: '自动', description: '根据问题自动选择思考深度' },
  { value: 'quick', label: '快速', description: '适合简单查询和明确问题' },
  { value: 'standard', label: '标准', description: '适合常规分析和多步问题' },
  { value: 'deep', label: '深度', description: '适合复杂分析和综合判断' }
]
const thinkingLevelIndex = computed(() => Math.max(0, thinkingLevels.findIndex(level => level.value === agent.thinking.value)))
const currentThinkingLevel = computed(() => thinkingLevels[thinkingLevelIndex.value])
const needsAIKeyRecovery = computed(() => agent.error.value.includes('无法解密 AI 密钥'))

onMounted(() => agent.start().catch(() => { agent.error.value = 'Agent 初始化失败，请重新打开页面。' }))
watch(() => agent.messages.value.length, async () => {
  await nextTick()
  if (list.value) list.value.scrollTop = list.value.scrollHeight
})

function sourcesOf(message) {
  try { return JSON.parse(message.sources || '[]').join('、') } catch { return '' }
}

async function send() {
  const message = draft.value.trim()
  const attachments = queuedAttachments.value
  if ((!message && !attachments.length) || !agent.sessionId.value) return
  const result = await agent.send(message, route.path, attachments)
  if (result) {
    draft.value = ''
    queuedAttachments.value = []
  }
}

function addAttachments(files) {
  const result = appendAgentAttachments(queuedAttachments.value, files)
  queuedAttachments.value = result.attachments
  const skippedCount = Object.values(result.skipped).reduce((sum, count) => sum + count, 0)
  if (!skippedCount) return
  const reasons = []
  if (result.skipped.unsupported) reasons.push(`${result.skipped.unsupported} 个格式不支持`)
  if (result.skipped.oversized) reasons.push(`${result.skipped.oversized} 个文件过大或为空`)
  if (result.skipped.count) reasons.push('每条消息最多 20 个文件')
  if (result.skipped.totalSize) reasons.push('附件合计不能超过 40 MiB')
  if (result.skipped.duplicate) reasons.push('重复文件已略过')
  ElMessage.warning(`部分文件未添加：${reasons.join('；')}`)
}

function removeAttachment(id) {
  queuedAttachments.value = queuedAttachments.value.filter(item => item.id !== id)
}

function actionFor(message) {
  return agent.actions.value.find(action => action.id === message.action_id)
}

function navigate(path) {
  router.push(path)
}

function setThinkingLevel(index) {
  agent.thinking.value = thinkingLevels[Number(index)]?.value || 'auto'
}

</script>

<template>
  <section aria-label="ERP Agent" class="ai-panel flex min-h-[680px] w-full flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
      <header class="ai-panel-header flex items-center justify-between bg-slate-900 px-5 py-4 text-white">
        <div><div class="font-semibold">ERP Agent</div><div class="text-xs text-slate-300">查询 · 草稿 · 确认执行 · 待办</div></div>
        <div class="flex gap-2"><button class="rounded-lg px-2 py-1 text-xs hover:bg-white/10" @click="memoryOpen = !memoryOpen">长期记忆</button><button class="ai-new-session rounded-lg px-2 py-1 text-xs hover:bg-white/10" @click="agent.newSession()">新对话</button></div>
      </header>
      <AgentMemoryPanel v-if="memoryOpen" @close="memoryOpen = false" />
      <div v-if="!memoryOpen" ref="list" class="ai-message-list flex-1 space-y-3 overflow-y-auto bg-slate-50 p-4" aria-live="polite">
        <div v-if="!agent.messages.value.length" class="ai-welcome rounded-2xl border bg-white p-4 text-sm text-slate-700">你好，直接告诉我你想了解或办理的事情即可。我会先判断需求，信息不足时继续询问；涉及订单或支出时，会先整理完整内容供你核对，确认后再提交。</div>
        <section v-if="!dismissedBrief && agent.brief.value.some(item => item.count > 0)" class="ai-brief rounded-2xl border border-amber-200 bg-amber-50 p-3 text-sm">
          <div class="mb-2 flex justify-between font-semibold text-amber-900"><span>当前待办</span><button class="font-normal" aria-label="收起待办" @click="dismissedBrief = true">收起</button></div>
          <button v-for="item in agent.brief.value.filter(item => item.count > 0)" :key="item.key" class="block py-1 text-left text-amber-800 hover:underline" @click="navigate(item.path)">{{ item.label }} {{ item.count }} 项 →</button>
        </section>
        <div v-for="message in agent.messages.value" :key="message.id" :class="message.role === 'user' ? 'flex justify-end' : ''">
          <div :class="['max-w-[95%] whitespace-pre-wrap break-words rounded-2xl px-4 py-3 text-sm leading-6', message.role === 'user' ? 'bg-slate-900 text-white' : 'border border-slate-200 bg-white text-slate-700']">
            <div v-if="message.content">{{ message.content }}</div>
            <div v-if="message.attachments?.length" class="agent-message-attachments">
              <div v-for="attachment in message.attachments" :key="attachment.id || attachment.name" class="agent-message-attachment">
                <span class="agent-message-attachment-icon" aria-hidden="true">↗</span>
                <span class="min-w-0 truncate" :title="attachment.relative_path || attachment.name">{{ attachment.relative_path || attachment.name }}</span>
                <span class="shrink-0 text-xs opacity-70">{{ formatAgentAttachmentSize(attachment.size) }}</span>
                <span v-if="attachment.truncated" class="shrink-0 text-xs opacity-70">内容已截取</span>
              </div>
            </div>
            <AgentEvidence :evidence="message.evidence" :status="message.status" />
            <div v-if="!message.evidence && sourcesOf(message)" class="mt-2 border-t pt-2 text-xs text-slate-400">数据来源：{{ sourcesOf(message) }}</div>
            <AgentFeedback v-if="message.role === 'assistant' && message.run_id" :message-id="message.id" :saved="agent.feedbacks.value[message.id]" :submit="agent.submitFeedback" />
          </div>
          <AgentActionCard v-if="actionFor(message)?.status === 'pending'" :key="actionFor(message).id" class="mt-2" :action="actionFor(message)" :submit="agent.confirm" />
          <div v-else-if="actionFor(message)?.status === 'saved'" class="mt-2 rounded-xl bg-green-50 p-2 text-xs text-green-700">已确认保存 · 记录 #{{ actionFor(message).target_id || '已创建' }}</div>
        </div>
        <div v-if="agent.loading.value" class="text-sm text-slate-400">{{ agent.loadingTip.value || '正在理解问题、核对数据并整理结论…' }}</div>
        <div v-if="agent.error.value" class="rounded-xl bg-red-50 p-3 text-sm text-red-600">
          <p>{{ agent.error.value }}</p>
          <button v-if="needsAIKeyRecovery" type="button" class="mt-2 font-medium underline" @click="navigate('/admin/settings')">前往 AI 设置重新录入 API Key →</button>
        </div>
      </div>
      <div v-if="!memoryOpen" class="ai-effort-toolbar border-t px-4 py-2">
        <div class="ai-effort-anchor">
          <div v-if="effortOpen" class="ai-effort-popover" role="group" aria-label="调整 Agent 思考等级">
            <div class="ai-effort-popover-header">
              <svg class="ai-effort-bolt" viewBox="0 0 24 24" aria-hidden="true"><path d="M13 2 4 14h7l-1 8 10-13h-7l1-7Z" /></svg>
              <div class="ai-effort-current">
                <strong>{{ currentThinkingLevel.label }}</strong>
                <span>AI 店长 · 思考等级</span>
              </div>
              <button type="button" class="ai-effort-reset" title="恢复自动" aria-label="恢复自动思考等级" :disabled="agent.loading.value" @click="setThinkingLevel(0)">
                <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 12a9 9 0 1 0 2.64-6.36L3 8.28M3 3v5.28h5.28" /></svg>
              </button>
            </div>
            <div class="ai-effort-slider-wrap" :style="{ '--effort-progress': `${thinkingLevelIndex / (thinkingLevels.length - 1) * 100}%` }">
              <div class="ai-effort-ticks" aria-hidden="true"><i v-for="level in thinkingLevels" :key="level.value"></i></div>
              <input
                id="agent-thinking"
                class="ai-effort-slider"
                type="range"
                min="0"
                :max="thinkingLevels.length - 1"
                step="1"
                :value="thinkingLevelIndex"
                :disabled="agent.loading.value"
                :aria-valuetext="`${currentThinkingLevel.label}：${currentThinkingLevel.description}`"
                aria-label="思考等级"
                @input="setThinkingLevel($event.target.value)"
              />
            </div>
            <div class="ai-effort-labels"><span v-for="level in thinkingLevels" :key="level.value">{{ level.label }}</span></div>
            <p class="ai-effort-description">{{ currentThinkingLevel.description }}</p>
          </div>
          <button type="button" class="ai-effort-trigger" :aria-expanded="effortOpen" aria-controls="agent-thinking" :disabled="agent.loading.value" @click="effortOpen = !effortOpen">
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M13 2 4 14h7l-1 8 10-13h-7l1-7Z" /></svg>
            <span>思考等级</span><strong>{{ currentThinkingLevel.label }}</strong>
            <svg class="ai-effort-chevron" viewBox="0 0 24 24" aria-hidden="true"><path d="m7 10 5 5 5-5" /></svg>
          </button>
        </div>
      </div>
      <form v-if="!memoryOpen" class="ai-composer border-t bg-white p-3" @submit.prevent="send()">
        <div v-if="queuedAttachments.length" class="ai-queued-attachments" aria-label="待发送附件">
          <div v-for="attachment in queuedAttachments" :key="attachment.id" class="ai-queued-attachment" :title="attachment.relativePath || attachment.name">
            <span class="ai-queued-attachment-icon" aria-hidden="true">↗</span>
            <span class="min-w-0 flex-1 truncate">{{ attachment.relativePath || attachment.name }}</span>
            <span class="shrink-0 text-xs text-slate-400">{{ formatAgentAttachmentSize(attachment.size) }}</span>
            <button type="button" :disabled="agent.loading.value" :aria-label="`移除附件 ${attachment.name}`" @click="removeAttachment(attachment.id)">×</button>
          </div>
        </div>
        <div class="ai-composer-row flex items-end gap-2">
          <AgentAttachmentPicker :disabled="agent.loading.value || !agent.sessionId.value" @select="addAttachments" />
          <textarea v-model="draft" rows="2" maxlength="500" aria-label="向 Agent 发送消息" placeholder="说说你想查询或办理的事情…" class="ai-input max-h-28 min-h-12 flex-1 resize-none rounded-xl border p-3 text-sm outline-none focus:border-blue-500" @keydown.enter.exact.prevent="send()" />
          <button type="submit" :disabled="agent.loading.value || (!draft.trim() && !queuedAttachments.length) || !agent.sessionId.value" class="ai-send rounded-xl bg-slate-900 px-4 py-3 text-sm text-white disabled:opacity-40">发送</button>
        </div>
      </form>
  </section>
</template>

<style scoped>
.ai-panel { height: min(76vh, 820px); }
.ai-panel-header { min-height: 74px; }
.ai-new-session { padding: 7px 10px; background: rgba(255, 255, 255, .1); }
.agent-message-attachments { display: grid; gap: 5px; margin-top: 8px; }
.agent-message-attachment { display: flex; min-width: 0; align-items: center; gap: 7px; border-radius: 9px; background: rgba(148, 163, 184, .12); padding: 6px 8px; }
.agent-message-attachment-icon, .ai-queued-attachment-icon { display: grid; width: 20px; height: 20px; flex: 0 0 auto; place-items: center; border-radius: 6px; background: rgba(59, 130, 246, .12); color: #7a70ff; font-size: 13px; }
.ai-queued-attachments { display: flex; flex-wrap: wrap; gap: 7px; padding: 0 0 9px 50px; }
.ai-queued-attachment { display: flex; max-width: min(100%, 360px); min-width: 0; align-items: center; gap: 7px; padding: 5px 7px; border: 1px solid #e2e8f0; border-radius: 11px; background: #f8fafc; color: #334155; font-size: 12px; }
.ai-queued-attachment button { display: grid; width: 20px; height: 20px; flex: 0 0 auto; place-items: center; border: 0; border-radius: 50%; background: transparent; color: #64748b; font-size: 17px; line-height: 1; }
.ai-queued-attachment button:hover { background: #e2e8f0; color: #0f172a; }
.ai-queued-attachment button:disabled { opacity: .4; }
.ai-composer-row .ai-input { min-width: 0; }
.ai-effort-toolbar { position: relative; z-index: 3; }
.ai-effort-anchor { position: relative; width: fit-content; }
.ai-effort-trigger { display: inline-flex; align-items: center; gap: 7px; border: 0; border-radius: 10px; padding: 7px 10px; background: #f1f5f9; color: #475569; font-size: 13px; }
.ai-effort-trigger:disabled { opacity: .55; }
.ai-effort-trigger > svg:first-child { width: 16px; height: 16px; fill: none; stroke: #64748b; stroke-linecap: round; stroke-linejoin: round; stroke-width: 1.8; }
.ai-effort-trigger strong { color: #635bff; font-weight: 600; }
.ai-effort-chevron { width: 15px; height: 15px; fill: none; stroke: currentColor; stroke-linecap: round; stroke-linejoin: round; stroke-width: 1.8; }
.ai-effort-popover { position: absolute; bottom: calc(100% + 12px); left: 0; z-index: 10; width: min(340px, calc(100vw - 48px)); padding: 18px 20px 14px; border: 1px solid #e2e8f0; border-radius: 22px; background: #fff; box-shadow: 0 18px 42px rgba(15, 23, 42, .16); }
.ai-effort-popover-header { display: grid; grid-template-columns: 24px 1fr 24px; align-items: center; gap: 8px; }
.ai-effort-bolt { width: 22px; height: 22px; fill: none; stroke: #94a3b8; stroke-linecap: round; stroke-linejoin: round; stroke-width: 1.8; }
.ai-effort-current { display: flex; flex-direction: column; align-items: center; line-height: 1.35; }
.ai-effort-current strong { color: #7a70ff; font-size: 20px; font-weight: 500; }
.ai-effort-current span { color: #64748b; font-size: 13px; }
.ai-effort-reset { display: grid; width: 26px; height: 26px; place-items: center; padding: 2px; border: 0; border-radius: 50%; background: transparent; color: #94a3b8; }
.ai-effort-reset:hover { background: #f1f5f9; color: #475569; }
.ai-effort-reset:disabled { opacity: .5; }
.ai-effort-reset svg { width: 22px; height: 22px; fill: none; stroke: currentColor; stroke-linecap: round; stroke-linejoin: round; stroke-width: 1.8; }
.ai-effort-slider-wrap { position: relative; height: 34px; margin: 18px 0 8px; }
.ai-effort-slider { position: relative; z-index: 1; width: 100%; height: 34px; margin: 0; appearance: none; background: transparent; cursor: pointer; }
.ai-effort-slider::-webkit-slider-runnable-track { height: 30px; border-radius: 999px; background: linear-gradient(to right, #7a70ff 0 var(--effort-progress), #e2e8f0 var(--effort-progress) 100%); }
.ai-effort-slider::-moz-range-track { height: 30px; border-radius: 999px; background: #e2e8f0; }
.ai-effort-slider::-moz-range-progress { height: 30px; border-radius: 999px; background: #7a70ff; }
.ai-effort-slider::-webkit-slider-thumb { width: 38px; height: 38px; margin-top: -4px; appearance: none; border: 2px solid #e2e8f0; border-radius: 50%; background: #fff; box-shadow: 0 1px 5px rgba(15, 23, 42, .16); }
.ai-effort-slider::-moz-range-thumb { width: 34px; height: 34px; border: 2px solid #e2e8f0; border-radius: 50%; background: #fff; box-shadow: 0 1px 5px rgba(15, 23, 42, .16); }
.ai-effort-slider:focus-visible { outline: none; }
.ai-effort-slider:focus-visible::-webkit-slider-thumb { outline: 3px solid #dcdaff; }
.ai-effort-ticks { position: absolute; z-index: 2; inset: 0 17px; display: flex; align-items: center; justify-content: space-between; pointer-events: none; }
.ai-effort-ticks i { width: 6px; height: 6px; border-radius: 50%; background: rgba(255, 255, 255, .45); }
.ai-effort-labels { display: flex; justify-content: space-between; color: #64748b; font-size: 11px; }
.ai-effort-description { margin: 10px 0 0; text-align: center; color: #94a3b8; font-size: 11px; }
@media (max-width: 767px) {
  .ai-panel { min-height: 620px; height: 75dvh; }
  .ai-effort-popover { width: min(320px, calc(100vw - 40px)); }
}
</style>
