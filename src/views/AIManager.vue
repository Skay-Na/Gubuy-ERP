<script setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import request from '../utils/request'

const signals = ref([])
const tasks = ref([])
const drafts = ref([])
const goals = ref([])
const activities = ref([])
const stepsByTask = ref({})
const loading = ref(false)
const scanning = ref(false)
const savingGoal = ref(false)
const reviewingDraft = ref(0)
const cancellingTask = ref(0)
const error = ref('')
let refreshTimer
const reviewNotes = reactive({})
const reviewQuantities = reactive({})
const goal = reactive({
  goal_type: 'reduce_stockout',
  target_metric: 'stockout_signals',
  target_value: 0,
  deadline: '',
  constraints: '',
  priority: 2
})

const openSignals = computed(() => signals.value.filter(item => item.status === 'open'))
const activeDrafts = computed(() => drafts.value.filter(item => item.status === 'suggested'))
const draftByTask = computed(() => new Map(drafts.value.map(item => [item.task_id, item])))
const activityByTask = computed(() => {
  const grouped = new Map()
  for (const item of activities.value) {
    if (!grouped.has(item.task_id)) grouped.set(item.task_id, [])
    grouped.get(item.task_id).push(item)
  }
  return grouped
})

function metricsOf(signal) {
  try { return JSON.parse(signal.metrics || '{}') } catch { return {} }
}

function dateOf(value) {
  return value ? new Date(value).toLocaleString('zh-CN') : '—'
}

function taskJSON(value) {
  try { return JSON.parse(value || '{}') } catch { return {} }
}

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    const paths = ['signals', 'tasks', 'drafts', 'goals', 'activities']
    const results = await Promise.all(paths.map(path => request.get('/api/ai/manager/' + path)))
    ;[signals.value, tasks.value, drafts.value, goals.value, activities.value] = results.map(result => result.data.data || [])
    const workflowTasks = tasks.value.filter(item => ['replenishment', 'replenishment_analysis'].includes(item.kind)).slice(0, 30)
    const stepResults = await Promise.all(workflowTasks.map(async item => {
      try {
        const response = await request.get(`/api/ai/manager/tasks/${item.id}/steps`)
        return [item.id, response.data.data || []]
      } catch {
        return [item.id, []]
      }
    }))
    stepsByTask.value = Object.fromEntries(stepResults)
  } catch {
    error.value = '经营工作台数据读取失败，请稍后重试。'
  } finally {
    loading.value = false
  }
}

async function cancelTask(task) {
  cancellingTask.value = task.id
  error.value = ''
  try {
    await request.post(`/api/ai/manager/tasks/${task.id}/cancel`)
    await refresh()
  } catch (cause) {
    error.value = cause?.response?.data?.msg || '任务取消失败，可能已经结束。'
    await refresh()
  } finally {
    cancellingTask.value = 0
  }
}

async function scan() {
  scanning.value = true
  error.value = ''
  try {
    await request.post('/api/ai/manager/scan')
    await refresh()
  } catch {
    error.value = '规则扫描未完成，请检查服务状态。'
  } finally {
    scanning.value = false
  }
}

async function saveGoal() {
  savingGoal.value = true
  error.value = ''
  try {
    await request.post('/api/ai/manager/goals', { ...goal })
    await refresh()
  } catch {
    error.value = '目标保存失败，请检查日期与数值。'
  } finally {
    savingGoal.value = false
  }
}

async function reviewDraft(draft, decision) {
  reviewingDraft.value = draft.id
  error.value = ''
  try {
    await request.post(`/api/ai/manager/drafts/${draft.id}/review`, {
      decision,
      quantity: Number(reviewQuantities[draft.id] || draft.quantity),
      note: reviewNotes[draft.id] || ''
    })
    await refresh()
  } catch (cause) {
    const message = cause?.response?.data?.msg || '草案审核失败，可能已处理或库存发生变化。'
    await refresh()
    error.value = message
  } finally {
    reviewingDraft.value = 0
  }
}

onMounted(() => {
  refresh()
  refreshTimer = window.setInterval(() => {
    if (tasks.value.some(item => ['CREATED', 'PLANNING', 'RUNNING'].includes(item.status))) refresh()
  }, 3000)
})
onUnmounted(() => window.clearInterval(refreshTimer))
</script>

<template>
  <div class="mx-auto max-w-7xl space-y-6 p-4 pb-20 md:p-8">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-sm font-medium text-blue-700">AI 店长 · 经营工作台</p>
        <h1 class="mt-1 text-3xl font-bold text-slate-900">值得关注的经营变化</h1>
        <p class="mt-2 text-sm text-slate-500">规则扫描真实业务数据，生成可核对的信号与内部建议。右下角可继续向 Agent 提问。</p>
      </div>
      <div class="flex gap-2">
        <button type="button" class="rounded-xl border bg-white px-4 py-2 text-sm" :disabled="loading" @click="refresh">刷新</button>
        <button type="button" class="rounded-xl bg-slate-900 px-4 py-2 text-sm text-white disabled:opacity-50" :disabled="scanning" @click="scan">{{ scanning ? '扫描中…' : '执行规则扫描' }}</button>
      </div>
    </header>

    <p v-if="error" role="alert" class="rounded-xl bg-red-50 p-3 text-sm text-red-700">{{ error }}</p>

    <div class="grid gap-4 sm:grid-cols-3">
      <div class="rounded-2xl border bg-white p-5"><p class="text-sm text-slate-500">待关注信号</p><p class="mt-2 text-3xl font-bold">{{ openSignals.length }}</p></div>
      <div class="rounded-2xl border bg-white p-5"><p class="text-sm text-slate-500">待核对补货草案</p><p class="mt-2 text-3xl font-bold">{{ activeDrafts.length }}</p></div>
      <div class="rounded-2xl border bg-white p-5"><p class="text-sm text-slate-500">持久任务</p><p class="mt-2 text-3xl font-bold">{{ tasks.length }}</p></div>
    </div>

    <section class="rounded-2xl border bg-white p-5">
      <h2 class="text-lg font-semibold">主动发现</h2>
      <p class="mt-1 text-xs text-slate-500">缺货规则：近 30 天有效订单至少售出 3 件，当前可售库存低于 7 天销量估计。库存不含样机。</p>
      <p v-if="!openSignals.length" class="mt-5 text-sm text-slate-500">暂无符合规则的未处理信号。数据不足时不会判定为异常。</p>
      <div v-for="signal in openSignals" :key="signal.id" class="mt-4 rounded-xl border border-amber-200 bg-amber-50 p-4">
        <div class="flex flex-wrap justify-between gap-2"><strong>商品 #{{ signal.product_id }} · 潜在缺货</strong><span class="text-xs text-slate-500">{{ dateOf(signal.detected_at) }}</span></div>
        <p class="mt-2 text-sm">可售库存 {{ metricsOf(signal).available }} 件 · 近 30 天销量 {{ metricsOf(signal).sales_30_days }} 件 · 触发阈值 {{ metricsOf(signal).threshold }} 件</p>
        <p class="mt-1 text-xs text-slate-600">{{ signal.evidence }}</p>
      </div>
    </section>

    <section class="rounded-2xl border bg-white p-5">
      <h2 class="text-lg font-semibold">任务与补货建议</h2>
      <p v-if="!tasks.length" class="mt-5 text-sm text-slate-500">暂无任务。执行规则扫描后，符合条件的信号会生成持久任务。</p>
      <div v-for="task in tasks" :key="task.id" class="mt-4 rounded-xl border p-4">
        <div class="flex justify-between gap-3"><strong>任务 #{{ task.id }} · {{ task.kind }}</strong><span class="text-xs text-slate-500">{{ task.status }}</span></div>
        <p class="mt-1 text-xs text-slate-500">步骤：{{ task.step }} · 尝试 {{ task.attempts }} 次 · {{ dateOf(task.updated_at) }}</p>
        <button v-if="['CREATED', 'PLANNING', 'RUNNING'].includes(task.status)" type="button" class="mt-2 rounded border border-red-300 px-3 py-1 text-xs text-red-700 disabled:opacity-50" :disabled="cancellingTask === task.id" @click="cancelTask(task)">{{ cancellingTask === task.id ? '取消中…' : '取消任务' }}</button>
        <div v-if="stepsByTask[task.id]?.length" class="mt-3 space-y-2">
          <div v-for="step in stepsByTask[task.id]" :key="step.id" class="rounded-lg bg-slate-50 p-3 text-xs">
            <div class="flex flex-wrap justify-between gap-2"><strong>{{ step.sequence }}. {{ step.tool_name }}</strong><span>{{ step.status }} · 重试 {{ step.retry_count }}</span></div>
            <p class="mt-1 text-slate-600">{{ step.input_summary }}</p>
            <p v-if="step.result_summary" class="mt-1 break-all text-slate-700">{{ step.result_summary }}</p>
            <p v-if="step.error_message" class="mt-1 text-red-700">{{ step.error_message }}</p>
          </div>
        </div>
        <p v-if="task.error_code" class="mt-2 text-xs text-red-700">失败原因：{{ task.error_code }}</p>
        <div v-if="task.kind === 'chat_request'" class="mt-3 rounded-lg bg-slate-50 p-3 text-sm">
          <p class="font-medium">聊天任务 · {{ taskJSON(task.input).topic || '一般查询' }} / {{ taskJSON(task.input).operation || 'analysis' }}</p>
          <p v-if="taskJSON(task.input).goal" class="mt-1">{{ taskJSON(task.input).goal }}</p>
          <p v-if="taskJSON(task.output).answer" class="mt-2 text-slate-600">{{ taskJSON(task.output).answer }}</p>
        </div>
        <div v-if="draftByTask.get(task.id)" class="mt-3 rounded-lg bg-blue-50 p-3 text-sm">
          <p class="font-medium">内部草案：商品 #{{ draftByTask.get(task.id).product_id }}，建议补货 {{ draftByTask.get(task.id).quantity }} 件 · {{ draftByTask.get(task.id).status === 'suggested' ? '待审核' : draftByTask.get(task.id).status }}</p>
          <p class="mt-1 text-xs">当前库存快照 {{ draftByTask.get(task.id).stock_snapshot }} 件；近 30 天销量 {{ draftByTask.get(task.id).sales_30_days }} 件；成本快照 {{ draftByTask.get(task.id).cost_snapshot || '未知' }}</p>
          <p class="mt-2 text-xs text-blue-900">{{ draftByTask.get(task.id).data_note }}</p>
          <div v-if="draftByTask.get(task.id).status === 'suggested'" class="mt-3 grid gap-2 md:grid-cols-[160px_1fr_auto_auto]">
            <label class="text-xs">审核数量
              <input v-model.number="reviewQuantities[draftByTask.get(task.id).id]" type="number" min="1" max="100000" :placeholder="String(draftByTask.get(task.id).quantity)" class="mt-1 w-full rounded border bg-white p-2" />
            </label>
            <label class="text-xs">审核备注
              <input v-model="reviewNotes[draftByTask.get(task.id).id]" maxlength="500" class="mt-1 w-full rounded border bg-white p-2" placeholder="可选" />
            </label>
            <button type="button" class="self-end rounded bg-emerald-700 px-3 py-2 text-xs text-white disabled:opacity-50" :disabled="reviewingDraft === draftByTask.get(task.id).id" @click="reviewDraft(draftByTask.get(task.id), 'approve')">确认内部计划</button>
            <button type="button" class="self-end rounded border border-red-300 px-3 py-2 text-xs text-red-700 disabled:opacity-50" :disabled="reviewingDraft === draftByTask.get(task.id).id" @click="reviewDraft(draftByTask.get(task.id), 'reject')">拒绝</button>
          </div>
          <p v-else-if="draftByTask.get(task.id).reviewed_by" class="mt-2 text-xs text-slate-600">审核人 {{ draftByTask.get(task.id).reviewed_by }} · {{ draftByTask.get(task.id).review_note || '无备注' }}</p>
        </div>
        <p v-for="activity in activityByTask.get(task.id) || []" :key="activity.id" class="mt-2 text-xs text-slate-600">{{ dateOf(activity.created_at) }} · {{ activity.detail }}</p>
      </div>
      <p class="mt-4 rounded-lg bg-slate-50 p-3 text-xs text-slate-600">草案没有提交采购订单、联系供应商或支付货款。正式采购需要先补齐供应商、预算与审批流程。</p>
    </section>

    <section class="rounded-2xl border bg-white p-5">
      <h2 class="text-lg font-semibold">经营目标</h2>
      <p class="mt-1 text-xs text-slate-500">目标只用于跟踪经营意图，不授予 Agent 修改业务数据的权限。</p>
      <form class="mt-4 grid gap-3 md:grid-cols-5" @submit.prevent="saveGoal">
        <div class="rounded-lg border bg-slate-50 p-2 text-sm">减少缺货风险</div>
        <input v-model.number="goal.target_value" type="number" min="0" class="rounded-lg border p-2 text-sm" aria-label="目标信号数" placeholder="目标信号数" />
        <input v-model="goal.deadline" type="date" required class="rounded-lg border p-2 text-sm" aria-label="目标日期" />
        <input v-model="goal.constraints" maxlength="500" class="rounded-lg border p-2 text-sm" aria-label="经营约束" placeholder="约束说明（可选）" />
        <button type="submit" class="rounded-lg bg-blue-700 px-4 py-2 text-sm text-white disabled:opacity-50" :disabled="savingGoal">{{ savingGoal ? '保存中…' : '保存目标' }}</button>
      </form>
      <div v-for="item in goals" :key="item.id" class="mt-3 flex flex-wrap justify-between gap-2 rounded-lg bg-slate-50 p-3 text-sm">
        <span>减少缺货风险 · 目标信号数 {{ item.target_value }} · 当前未处理 {{ openSignals.length }}</span>
        <span class="text-slate-500">截止 {{ dateOf(item.deadline) }} · {{ item.status }}</span>
      </div>
    </section>
  </div>
</template>
