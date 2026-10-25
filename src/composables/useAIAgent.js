import { ref, shallowRef } from 'vue'
import request from '../utils/request'
import { ElMessage } from 'element-plus'

const sessionKey = 'erp_ai_agent_session'

const TOOL_LABELS = {
  query_product_inventory: '查询商品库存',
  query_sales_summary: '查询销量汇总',
  query_sales_trend: '查询销量趋势',
  query_inventory_sales: '库存销量对照',
  query_unpaid_orders: '查询未结清订单',
  create_order_draft: '准备订单草稿',
  create_expense_draft: '准备支出草稿',
  save_long_term_memory: '保存长期记忆'
}

export function useAIAgent() {
  const sessionId = shallowRef(null)
  const messages = ref([])
  const actions = ref([])
  const feedbacks = ref({})
  const brief = ref([])
  const loading = shallowRef(false)
  const loadingTip = shallowRef('')
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

  function authHeader() {
    const token = localStorage.getItem('erp_token')
    return token ? { Authorization: `Bearer ${token}` } : {}
  }

  /**
   * 流式发送消息：SSE 逐段接收回复。
   * 服务端事件：status / delta（累计文本）/ tool / done / error
   */
  async function send(message, path, attachments = []) {
    const content = message.trim()
    if (loading.value || (!content && !attachments.length)) return null
    error.value = ''
    loading.value = true
    loadingTip.value = '正在理解问题…'
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
    const placeholder = { id: `stream-${Date.now()}`, role: 'assistant', content: '', sources: '[]', streaming: true }
    let placeholderPushed = false
    const pushPlaceholder = () => {
      if (!placeholderPushed) {
        messages.value.push(placeholder)
        placeholderPushed = true
      }
    }
    try {
      let payload
      if (attachments.length) {
        payload = new FormData()
        payload.append('message', content)
        payload.append('thinking', thinking.value)
        payload.append('path', path || '')
        for (const attachment of attachments) {
          payload.append('attachments', attachment.file, attachment.name)
        }
      } else {
        payload = JSON.stringify({ message: content, thinking: thinking.value, path: path || '' })
      }
      const response = await fetch(`/api/ai/agent/sessions/${sessionId.value}/messages`, {
        method: 'POST',
        headers: {
          ...authHeader(),
          ...(attachments.length ? {} : { 'Content-Type': 'application/json' })
        },
        body: payload
      })
      if (!response.ok || !response.body) {
        let msg = `Agent 服务异常（HTTP ${response.status}）`
        try {
          const body = await response.json()
          if (body?.msg) msg = body.msg
        } catch { /* 保持默认提示 */ }
        throw new Error(msg)
      }
      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      let finalResult = null
      let streamError = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        let boundary = buffer.indexOf('\n\n')
        while (boundary >= 0) {
          const chunk = buffer.slice(0, boundary)
          buffer = buffer.slice(boundary + 2)
          boundary = buffer.indexOf('\n\n')
          let eventName = 'message'
          const dataLines = []
          for (const line of chunk.split('\n')) {
            if (line.startsWith('event: ')) eventName = line.slice(7).trim()
            else if (line.startsWith('data: ')) dataLines.push(line.slice(6))
          }
          if (!dataLines.length) continue
          let data
          try { data = JSON.parse(dataLines.join('\n')) } catch { continue }
          if (eventName === 'delta') {
            pushPlaceholder()
            placeholder.content = data.text || ''
            loadingTip.value = '正在整理结论…'
          } else if (eventName === 'tool') {
            const label = TOOL_LABELS[data.name] || data.name
            loadingTip.value = data.state === 'end'
              ? `已完成：${label}`
              : `正在${label}…`
          } else if (eventName === 'done') {
            finalResult = data
          } else if (eventName === 'error') {
            streamError = data.msg || 'Agent 暂时无法处理，请重试。'
          }
        }
      }
      if (streamError) throw new Error(streamError)
      if (!finalResult) throw new Error('Agent 服务中断，请重试。')
      // 用服务端落库记录替换本地占位
      const optimisticIndex = messages.value.indexOf(optimistic)
      if (optimisticIndex >= 0) messages.value.splice(optimisticIndex, 1, finalResult.user_message)
      pushPlaceholder()
      const placeholderIndex = messages.value.indexOf(placeholder)
      if (placeholderIndex >= 0) messages.value.splice(placeholderIndex, 1, finalResult.message)
      feedbacks.value = { ...feedbacks.value, [finalResult.message.id]: null }
      if (finalResult.action?.id) actions.value.push(finalResult.action)
      return finalResult
    } catch (cause) {
      messages.value = messages.value.filter(item => item !== optimistic && item !== placeholder)
      error.value = cause.response?.data?.msg || cause.message || 'Agent 暂时无法处理，请重试。'
      return null
    } finally {
      loading.value = false
      loadingTip.value = ''
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

  return { sessionId, messages, actions, feedbacks, brief, loading, loadingTip, error, thinking, start, send, submitFeedback, confirm, newSession, refreshBrief }
}
