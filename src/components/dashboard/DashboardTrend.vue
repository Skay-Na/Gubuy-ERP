<script setup>
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import VChart from 'vue-echarts'

use([CanvasRenderer, BarChart, LineChart, GridComponent, TooltipComponent])

const props = defineProps({ trend: { type: Array, default: () => [] }, period: { type: String, required: true } })
const option = computed(() => ({
  animationDuration: 250,
  color: ['#2563eb', '#e0a22d'],
  tooltip: { trigger: 'axis', confine: true, valueFormatter: value => Number(value || 0).toLocaleString('zh-CN') },
  grid: { left: 12, right: 18, top: 24, bottom: 10, containLabel: true },
  xAxis: { type: 'category', boundaryGap: true, axisTick: { show: false }, axisLine: { lineStyle: { color: '#e2e8f0' } }, axisLabel: { color: '#94a3b8', fontSize: 11 }, data: props.trend.map(item => String(item.date || '').slice(5, 10) || '—') },
  yAxis: [
    { type: 'value', axisLabel: { color: '#94a3b8', fontSize: 11 }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
    { type: 'value', minInterval: 1, axisLabel: { color: '#94a3b8', fontSize: 11 }, splitLine: { show: false } }
  ],
  series: [
    { name: '销售额', type: 'bar', barMaxWidth: 24, itemStyle: { borderRadius: [5, 5, 0, 0] }, data: props.trend.map(item => Number(item.revenue || 0)) },
    { name: '订单数', type: 'line', yAxisIndex: 1, smooth: true, symbolSize: 7, lineStyle: { width: 2 }, data: props.trend.map(item => Number(item.order_count || 0)) }
  ]
}))
</script>

<template>
  <section class="panel trend" aria-labelledby="trend-title">
    <div class="panel-heading">
      <div><span class="eyebrow">SALES TREND</span><h2 id="trend-title">销售趋势</h2></div>
      <div class="legend"><span><i class="sales-dot" />销售额</span><span><i class="orders-dot" />订单数</span></div>
    </div>
    <div v-if="trend.length" class="chart-wrap"><VChart :option="option" autoresize /></div>
    <div v-else class="empty">所选期间暂无销售记录</div>
    <p v-if="period === 'today' && trend.length" class="hint">按日汇总，今日显示一个数据点</p>
  </section>
</template>

<style scoped>
.panel{min-width:0;background:white;border:1px solid #e8edf3;border-radius:18px;padding:22px}
.panel-heading{display:flex;justify-content:space-between;align-items:flex-end;gap:12px}
.eyebrow{font-size:10px;font-weight:800;letter-spacing:.14em;color:#94a3b8}
h2{margin:5px 0 0;color:#172033;font-size:18px;font-weight:750}
.legend{display:flex;gap:16px;color:#64748b;font-size:12px;white-space:nowrap}
.legend span{display:flex;align-items:center;gap:6px}
.legend i{width:8px;height:8px;border-radius:50%;display:inline-block}
.sales-dot{background:#2563eb}.orders-dot{background:#e0a22d}
.chart-wrap{height:264px;margin-top:20px}.chart-wrap>div{height:100%;width:100%}
.empty{height:264px;display:grid;place-items:center;color:#94a3b8;font-size:13px}
.hint{margin:4px 0 0;color:#94a3b8;font-size:11px}
@media(max-width:767px){.panel{padding:18px}.chart-wrap,.empty{height:210px}.legend{gap:9px;font-size:11px}}
</style>
