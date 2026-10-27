/**
 * 销售趋势自适应聚合（纯函数，供 DashboardTrendChart 与单元测试使用）。
 *
 * 后端接口按天返回 sales_trend：[{ date, revenue: Number, order_count: Number }]。
 * date 兼容两种格式：'YYYY-MM-DD' 纯日期，或 'YYYY-MM-DDTHH:mm:ss+08:00' 完整 ISO 时间戳
 * （MySQL DATE 经 GORM/parseTime 连接扫入 string 后会带时间与时区），入口统一截取前 10 位规整。
 * 当点数过多时逐日柱会在小画布中糊成一片，因此按点数自适应聚合：
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

/**
 * 自适应聚合趋势数据：输出与输入同结构、按时间升序的数组。
 * @param {Array<{date: string, revenue: number, order_count: number}>} trend 趋势原始数据
 * @returns {Array<{date: string, revenue: number, order_count: number}>} 聚合后的趋势数据
 */
export function aggregateTrend(trend) {
  if (!Array.isArray(trend) || !trend.length) return []
  const items = trend.map(normalizeItem).filter(item => item.date)
  if (!items.length) return []

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
