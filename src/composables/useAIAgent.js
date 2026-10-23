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

  async function send(message, path, attachments = []) {
    const content = message.trim()
    if (loading.value || (!content && !attachments.length)) return
    error.value = ''
    loading.value = true
    const optimistic = {
      id: `local-${Date.now()}`,
      role: 'user',
      content,
      attachments: attachments.map(item => ({
        id: item.id,
        name: item.name,
        relative_path: item.relativePath || '',
        media_type: item.mediaType || '',
        size: item.size
      }))
    }
    messages.value.push(optimistic)
    try {
      let payload = { message: content, path, thinking: thinking.value }
      if (attachments.length) {
        payload = new FormData()
        payload.append('message', content)
        payload.append('path', path)
        payload.append('thinking', thinking.value)
        for (const attachment of attachments) {
          payload.append('attachments', attachment.file, attachment.name)
          payload.append('attachment_paths', attachment.relativePath || '')
        }
      }
      const response = await request.post(`/api/ai/agent/sessions/${sessionId.value}/messages`, payload, { timeout: 270000 })
      if (response.result.user_message) {
        const optimisticIndex = messages.value.indexOf(optimistic)
        if (optimisticIndex >= 0) messages.value.splice(optimisticIndex, 1, response.result.user_message)
      }
      messages.value.push(response.result.message)
      feedbacks.value = { ...feedbacks.value, [response.result.message.id]: null }
      if (response.result.action) actions.value.push(response.result.action)
      if (response.result.memory_updates?.length) ElMessage.success('已归纳长期记忆，可在「长期记忆」中查看和修改')
      return response.result
    } catch (cause) {
      messages.value = messages.value.filter(item => item !== optimistic)
      error.value = cause.response?.data?.msg || 'Agent 暂时无法处理，请重试。'
      return null
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
