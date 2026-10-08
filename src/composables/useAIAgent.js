import { ref, shallowRef } from 'vue'
import request from '../utils/request'
import { ElMessage } from 'element-plus'

const sessionKey = 'erp_ai_agent_session'

export function useAIAgent() {
  const sessionId = shallowRef(null)
  const messages = ref([])
  const actions = ref([])
  const feedbacks = ref({})
  const brief = ref([])
  const loading = shallowRef(false)
  const error = shallowRef('')
  const thinking = shallowRef('auto')

  async function start() {
    let id = localStorage.getItem(sessionKey)
    if (id) {
      try {
        const response = await request.get(`/api/ai/agent/sessions/${id}`)
        sessionId.value = id
        messages.value = response.result.messages || []
        actions.value = response.result.actions || []
        feedbacks.value = response.result.feedbacks || {}
      } catch {
        localStorage.removeItem(sessionKey)
        id = null
      }
    }
    if (!id) {
      const response = await request.post('/api/ai/agent/sessions')
      sessionId.value = response.result.id
      localStorage.setItem(sessionKey, String(sessionId.value))
      messages.value = []
      actions.value = []
      feedbacks.value = {}
    }
    await refreshBrief()
  }

  async function refreshBrief() {
    try {
      const response = await request.get('/api/ai/agent/brief')
      brief.value = response.result || []
    } catch {
      brief.value = []
    }
  }

  async function send(message, path) {
    if (loading.value || !message.trim()) return
    error.value = ''
    loading.value = true
    const optimistic = { id: `local-${Date.now()}`, role: 'user', content: message.trim() }
    messages.value.push(optimistic)
    try {
      const response = await request.post(`/api/ai/agent/sessions/${sessionId.value}/messages`, { message: message.trim(), path, thinking: thinking.value }, { timeout: 270000 })
      messages.value.push(response.result.message)
      feedbacks.value = { ...feedbacks.value, [response.result.message.id]: null }
      if (response.result.action) actions.value.push(response.result.action)
      if (response.result.memory_updates?.length) ElMessage.success('已归纳长期记忆，可在「长期记忆」中查看和修改')
    } catch (cause) {
      messages.value = messages.value.filter(item => item !== optimistic)
      error.value = cause.response?.data?.msg || 'Agent 暂时无法处理，请重试。'
    } finally {
      loading.value = false
    }
  }

  async function submitFeedback(messageId, feedback) {
    await request.post(`/api/ai/agent/messages/${messageId}/feedback`, feedback)
    feedbacks.value = { ...feedbacks.value, [messageId]: { ...feedback } }
  }

  async function confirm(action, payload) {
    const endpoint = action.kind === 'order' ? '/api/orders' : `/api/ai/agent/actions/${action.id}/confirm`
    const body = action.kind === 'order' ? { ...payload, agent_action_id: action.id } : payload
    const response = await request.post(endpoint, body, { timeout: 60000 })
    const target = action.kind === 'order' ? response.result?.order?.id : response.result?.id
    const saved = actions.value.find(item => item.id === action.id)
    if (saved) { saved.status = 'saved'; saved.target_id = target }
    messages.value.push({ id: `saved-${action.id}`, role: 'assistant', content: `${action.kind === 'order' ? '订单' : '支出'}已保存${target ? `，记录编号 #${target}` : ''}。`, sources: '[]' })
    await refreshBrief()
    return response
  }

  async function newSession() {
    if (loading.value) return
    localStorage.removeItem(sessionKey)
    sessionId.value = null
    await start()
  }

  return { sessionId, messages, actions, feedbacks, brief, loading, error, thinking, start, send, submitFeedback, confirm, newSession, refreshBrief }
}
