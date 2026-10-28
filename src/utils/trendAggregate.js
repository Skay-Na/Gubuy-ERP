/**
 * 销售趋势自适应聚合（纯函数，供 DashboardTrendChart 与单元测试使用）。
 *
 * 后端接口按天返回 sales_trend：[{ date, revenue: Number, order_count: Number }]。
 * date 兼容两种格式：'YYYY-MM-DD' 纯日期，或 'YYYY-MM-DDTHH:mm:ss+08:00' 完整 ISO 时间戳
 * （MySQL DATE 经 GORM/parseTime 连接扫入 string 后会带时间与时区），入口统一截取前 10 位规整。
 * 当点数过多时逐日柱会在小画布中糊成一片，因此按点数自适应聚合（点数按补齐连续日历后的天数计）：
 * - 点数 ≤ DAY_MAX_POINTS(62)  ：按日展示（原样返回，仅做数值规整）
 * - 点数 ≤ WEEK_MAX_POINTS(250)：按 ISO 周聚合（周一为周起点），date 取该周周一 'YYYY-MM-DD'
 * - 点数 > WEEK_MAX_POINTS(250)：按自然月聚合，date 取 'YYYY-MM'
 * 聚合时 revenue / order_count 累加，输出与输入同结构、按时间升序。
 */

export const DAY_MAX_POINTS = 62
export const WEEK_MAX_POINTS = 250

const DAY_MS = 24 * 60 * 60 * 1000

/** 解析 'YYYY-MM-DD' 为 UTC 时间戳；非法日期返回 null */
function parseDay(dateStr) {
  const match = /^(\d{4})-(\d{1,2})-(\d{1,2})/.exec(String(dateStr || ''))
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  if (month < 1 || month > 12 || day < 1 || day > 31) return null
  return Date.UTC(year, month - 1, day)
}

/** UTC 时间戳 → 'YYYY-MM-DD' */
function formatDay(ms) {
  const d = new Date(ms)
  const month = String(d.getUTCMonth() + 1).padStart(2, '0')
  const day = String(d.getUTCDate()).padStart(2, '0')
  return `${d.getUTCFullYear()}-${month}-${day}`
}

/** 所在 ISO 周的周一（UTC 零点）时间戳 */
function mondayOf(ms) {
  const d = new Date(ms)
  const weekday = (d.getUTCDay() + 6) % 7 // 周一=0 … 周日=6
  return ms - weekday * DAY_MS
}

/** 规整单条数据：字符串/数值兜底，保证输出结构与输入一致 */
function normalizeItem(item) {
  return {
    // 后端可能返回 'YYYY-MM-DD' 或带时间时区的 ISO 串（'YYYY-MM-DDTHH:mm:ss±HH:mm'），统一截取日期部分
    date: String(item?.date || '').slice(0, 10),
    revenue: Number(item?.revenue || 0),
    order_count: Number(item?.order_count || 0)
  }
}

/**
 * 计算聚合粒度。
 * @param {Array<{date: string, revenue: number, order_count: number}>} trend 趋势原始数据
 * @returns {'day'|'week'|'month'} 聚合粒度
 */
export function resolveGranularity(trend) {
  const count = Array.isArray(trend) ? trend.length : 0
  if (count <= DAY_MAX_POINTS) return 'day'
  if (count <= WEEK_MAX_POINTS) return 'week'
  return 'month'
}

/** 补零两位数字 */
const pad2 = n => String(n).padStart(2, '0')

/** 解析 'YYYY-MM' 纯月份键（月聚合桶键）为 (年, 0-based月)；非法返回 null */
function parseMonthKey(dateStr) {
  const match = /^(\d{4})-(\d{1,2})$/.exec(String(dateStr || ''))
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  if (month < 1 || month > 12) return null
  return { year, month: month - 1 }
}

/**
 * 计算趋势柱覆盖的日期区间（供柱点击钻取明细使用）。
 * - 日：当天
 * - 周：该周周一起 7 天
 * - 月：该月 1 日 ~ 月末（date 兼容 'YYYY-MM-DD' 与月聚合桶键 'YYYY-MM'）
 * 传入 bounds（{start, end} 纯日期串）时将区间 clamp 到当前统计期间边界；
 * clamp 后为空区间（start > end）返回 null。
 * @param {{date: string}} item 聚合后的趋势点
 * @param {'day'|'week'|'month'} granularity 聚合粒度
 * @param {{start?: string, end?: string}|null} [bounds] 当前期间边界（可选）
 * @returns {{start_date: string, end_date: string}|null}
 */
export function barDateRange(item, granularity, bounds = null) {
  let start
  let end
  if (granularity === 'month') {
    // 月聚合桶键为 'YYYY-MM'（aggregateTrend slice(0,7) 生成），无"日"部分，须先按月份键解析；
    // 兼容历史 'YYYY-MM-DD' 输入（parseDay 失败时回退月份键，双保险）。
    const monthKey = parseMonthKey(item?.date)
    const ms = monthKey ? null : parseDay(item?.date)
    if (!monthKey && ms === null) return null
    const { year, month } = monthKey || (() => {
      const d = new Date(ms)
      return { year: d.getUTCFullYear(), month: d.getUTCMonth() }
    })()
    start = `${year}-${pad2(month + 1)}-01`
    end = formatDay(Date.UTC(year, month + 1, 0)) // 月末：次月 0 日（自动适配大小月/闰年）
  } else {
    const ms = parseDay(item?.date)
    if (ms === null) return null
    if (granularity === 'week') {
      start = formatDay(ms)
      end = formatDay(ms + 6 * DAY_MS)
    } else {
      start = formatDay(ms)
      end = formatDay(ms)
    }
  }
  if (bounds?.start && start < bounds.start) start = bounds.start
  if (bounds?.end && end > bounds.end) end = bounds.end
  if (start > end) return null
  return { start_date: start, end_date: end }
}

/**
 * 补齐连续日历：后端 sales_trend 只返回有销量的日期（稀疏点），
 * 直接画会导致柱子挤在一起、时间轴比例失真（有单的日子相邻、断档天消失）。
 * 这里把区间 [最早日期, max(最晚日期, 今天)] 内的每一天都补出来，
 * 缺失的天 revenue/order_count 记 0，使柱子按真实时间等距分布。
 * @param {Array<{date: string, revenue: number, order_count: number}>} trend 规整后的稀疏趋势数据
 * @returns {Array<{date: string, revenue: number, order_count: number}>} 连续日历趋势数据
 */
export function densifyTrend(items) {
  if (!Array.isArray(items) || !items.length) return []
  let minMs = null
  let maxMs = null
  for (const item of items) {
    const ms = parseDay(item.date)
    if (ms === null) continue
    if (minMs === null || ms < minMs) minMs = ms
    if (maxMs === null || ms > maxMs) maxMs = ms
  }
  if (minMs === null || maxMs === null) return items
  // 区间右端延伸到今天（本地时区），让"今天之前没单的日子"也留出轨道
  const now = new Date()
  const todayMs = parseDay(`${now.getFullYear()}-${pad2(now.getMonth() + 1)}-${pad2(now.getDate())}`)
  if (todayMs !== null && todayMs > maxMs) maxMs = todayMs
  const totalDays = Math.round((maxMs - minMs) / DAY_MS) + 1
  if (totalDays <= 0) return items
  const byDate = new Map(items.map(item => [item.date, item]))
  const out = []
  for (let i = 0; i < totalDays; i++) {
    const date = formatDay(minMs + i * DAY_MS)
    const hit = byDate.get(date)
    out.push(hit || { date, revenue: 0, order_count: 0 })
  }
  return out
}

/**
 * 自适应聚合趋势数据：输出与输入同结构、按时间升序的数组。
 * 先补齐连续日历再判粒度/聚合，保证柱子按真实时间等距分布。
 * @param {Array<{date: string, revenue: number, order_count: number}>} trend 趋势原始数据
 * @returns {Array<{date: string, revenue: number, order_count: number}>} 聚合后的趋势数据
 */
export function aggregateTrend(trend) {
  if (!Array.isArray(trend) || !trend.length) return []
  const raw = trend.map(normalizeItem).filter(item => item.date)
  if (!raw.length) return []
  const items = densifyTrend(raw)

  if (resolveGranularity(items) === 'day') {
    return items.sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0))
  }

  const granularity = resolveGranularity(items)
  const buckets = new Map()
  for (const item of items) {
    const ms = parseDay(item.date)
    // 非法日期无法归入周/月桶，退化为独立桶，避免静默丢数
    const key = ms === null
      ? item.date
      : granularity === 'week'
        ? formatDay(mondayOf(ms))
        : item.date.slice(0, 7)
    const bucket = buckets.get(key)
    if (bucket) {
      bucket.revenue += item.revenue
      bucket.order_count += item.order_count
    } else {
      buckets.set(key, { date: key, revenue: item.revenue, order_count: item.order_count })
    }
  }
  // 输入乱序时 Map 按插入序输出，这里统一按周期键升序
  return [...buckets.values()].sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0))
}
