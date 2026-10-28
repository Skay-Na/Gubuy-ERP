/**
 * trendAggregate 稀疏数据补齐（densifyTrend）与聚合回归测试。
 * 背景：后端 sales_trend 只返回有销量的日期，直接画图会导致柱子挤在左侧、
 * 时间轴比例失真。densifyTrend 必须补齐连续日历且不丢任何金额。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { aggregateTrend, densifyTrend, resolveGranularity } from './trendAggregate.js'

test('densifyTrend 补齐缺失日期并延伸到今天，缺天补 0', () => {
  const sparse = [
    { date: '2026-10-01', revenue: 6636, order_count: 2 },
    { date: '2026-10-05', revenue: 9000, order_count: 1 },
    { date: '2026-10-24', revenue: 3500, order_count: 1 }
  ]
  const dense = densifyTrend(sparse)
  assert.equal(dense[0].date, '2026-10-01')
  // 区间右端至少到数据最后一天（今天在测试机上 ≥ 2026-10-28，运行时动态判定）
  assert.ok(dense.length >= 24)
  // 断档日 10-02 必须存在且为 0
  const day2 = dense.find(p => p.date === '2026-10-02')
  assert.ok(day2, '缺失日期 10-02 应被补出')
  assert.equal(day2.revenue, 0)
  assert.equal(day2.order_count, 0)
  // 有单日的数值不被覆盖
  assert.equal(dense.find(p => p.date === '2026-10-05').revenue, 9000)
})

test('aggregateTrend 补齐后金额守恒（稀疏输入 → 聚合输出总额一致）', () => {
  const sparse = [
    { date: '2026-10-01', revenue: 6636, order_count: 2 },
    { date: '2026-10-19', revenue: 7400, order_count: 2 },
    { date: '2026-10-24', revenue: 3500, order_count: 1 }
  ]
  const agg = aggregateTrend(sparse)
  const sum = agg.reduce((s, p) => s + p.revenue, 0)
  assert.equal(sum, 17536)
  assert.equal(agg.reduce((s, p) => s + p.order_count, 0), 5)
})

test('补齐后大跨度数据自动切换聚合粒度', () => {
  // 一年内 300 个互不重复的有单日（2025-01-01 ~ 2025-12-25）→ 应按月聚合而非逐日
  const sparse = []
  for (let i = 0; i < 300; i++) {
    const month = Math.floor(i / 25) + 1
    const day = (i % 25) + 1
    sparse.push({ date: `2025-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`, revenue: 100, order_count: 1 })
  }
  const dense = densifyTrend(sparse)
  assert.equal(resolveGranularity(dense), 'month')
  const agg = aggregateTrend(sparse)
  // 区间会延伸到今天，因此桶数 = 2025-01 起 ~ 今天的月份数（≥ 12），且金额守恒
  assert.ok(agg.length >= 12, `月桶数应 ≥ 12，实际 ${agg.length}`)
  assert.equal(agg.reduce((s, p) => s + p.revenue, 0), 30000)
})

test('空数据与全非法日期安全兜底', () => {
  assert.deepEqual(densifyTrend([]), [])
  assert.deepEqual(aggregateTrend([]), [])
  assert.deepEqual(aggregateTrend([{ date: 'not-a-date', revenue: 1, order_count: 1 }]).length, 1)
})
