<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts'
import { aggregateTrend, resolveGranularity } from '../../utils/trendAggregate'

const props = defineProps({
  trend: { type: Array, default: () => [] },
  title: { type: String, default: '销售趋势' }
})

const chartEl = ref(null)
let chart = null

// 聚合粒度随数据点数自适应（≤62 天/63~250 周/>250 月），图例文案同步切换
const granularity = computed(() => resolveGranularity(props.trend))
const granularityLabel = computed(() => ({ day: '日', week: '周', month: '月' }[granularity.value] || '日'))

const compactMoney = value => {
  const num = Number(value || 0)
  if (Math.abs(num) >= 10000) return `${(num / 10000).toFixed(num % 10000 === 0 ? 0 : 1)}万`
  return num.toLocaleString('zh-CN', { maximumFractionDigits: 0 })
}

/** x 轴刻度文案：日/周显示 MM-DD，月显示 YY年M月（带年份，跨年柱可区分） */
function axisLabel(date) {
  const text = String(date || '')
  if (granularity.value === 'month') return `${text.slice(2, 4)}年${Number(text.slice(5, 7))}月`
  return text.slice(5)
}

/** tooltip 标题：注明当前聚合粒度（周汇总以周一日期为起点，月汇总带完整年份） */
function tooltipTitle(item) {
  const text = String(item?.date || '')
  if (granularity.value === 'week') return `周汇总（${text} 起）`
  if (granularity.value === 'month') return `${text.slice(0, 4)}年${Number(text.slice(5, 7))}月汇总`
  return text
}

function render() {
  if (!chart) return
  const aggregated = aggregateTrend(props.trend || [])
  // 容器从 v-show 隐藏变为可见（首次数据到达）后，先同步一次尺寸再渲染，避免画布 0x0
  if (aggregated.length) chart.resize()
  chart.setOption({
    grid: { left: 6, right: 10, top: 26, bottom: 2, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow', shadowStyle: { color: '#4f46e50d' } },
      backgroundColor: '#172033',
      borderWidth: 0,
      padding: [8, 12],
      textStyle: { color: '#e2e8f0', fontSize: 11 },
      formatter: params => {
        const point = params?.[0]
        const item = aggregated[point?.dataIndex] || {}
        return `<b>${tooltipTitle(item)}</b><br/>销售额 ¥${Number(item.revenue || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}<br/>${item.order_count || 0} 单`
      }
    },
    xAxis: {
      type: 'category',
      data: aggregated.map(item => axisLabel(item.date)),
      axisLine: { lineStyle: { color: '#e1e7ef' } },
      axisTick: { show: false },
      axisLabel: { color: '#8390a2', fontSize: 10, interval: 'auto', hideOverlap: true }
    },
    yAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: '#eef2f7' } },
      axisLabel: { color: '#8390a2', fontSize: 10, formatter: compactMoney }
    },
    series: [{
      name: '销售额',
      type: 'bar',
      data: aggregated.map(item => Number(item.revenue || 0)),
      barMaxWidth: 24,
      itemStyle: { color: '#6366f1', borderRadius: [4, 4, 0, 0] },
      emphasis: { itemStyle: { color: '#4f46e5' } }
    }]
  }, true)
}

function handleResize() { chart?.resize() }

onMounted(() => {
  if (!chartEl.value) return
  chart = echarts.init(chartEl.value)
  render()
  window.addEventListener('resize', handleResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  chart?.dispose()
  chart = null
})

// flush: 'post' 保证回调执行时 v-show 的 DOM 更新已完成，resize 能取到真实宽高
watch(() => props.trend, render, { deep: true, flush: 'post' })
</script>

<template>
  <section class="trend-panel" aria-labelledby="trend-title">
    <div class="panel-heading">
      <div>
        <span class="eyebrow">TREND</span>
        <h2 id="trend-title">{{ title }}</h2>
      </div>
      <span class="trend-legend"><i></i>{{ granularityLabel }}销售额</span>
    </div>
    <div v-show="(trend || []).length" ref="chartEl" class="trend-chart" role="img" aria-label="销售额趋势图"></div>
    <p v-if="!(trend || []).length" class="empty">所选期间暂无销售数据</p>
  </section>
</template>

<style scoped>
.trend-panel{min-width:0;margin-bottom:16px;padding:22px;background:#fff;border:1px solid #e8edf3;border-radius:18px}
.panel-heading{display:flex;justify-content:space-between;align-items:flex-end;gap:12px}
.eyebrow{font-size:10px;font-weight:800;letter-spacing:.14em;color:#94a3b8}
h2{margin:5px 0 0;color:#172033;font-size:18px;font-weight:750}
.trend-legend{display:flex;align-items:center;gap:6px;color:#66758a;font-size:11px}
.trend-legend i{width:10px;height:10px;border-radius:3px;background:#6366f1}
.trend-chart{width:100%;height:260px;margin-top:16px}
.empty{margin:40px 0 22px;color:#94a3b8;font-size:13px;text-align:center}
@media(max-width:767px){.trend-panel{padding:15px;margin-bottom:12px;border-radius:14px}.trend-chart{height:180px;margin-top:12px}}
</style>
