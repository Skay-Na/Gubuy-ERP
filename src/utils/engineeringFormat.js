// 工程模块专用格式化（SPEC-ENG-001 §3.1 金额口径的唯一实现处）
//
// 与全局 src/utils/format.js 的分工：
//   - formatMoney  → 千分位 + 固定 2 位，无货币符号（订单等老页面沿用）
//   - 本文件        → 带 ¥ 前缀 + 负数把负号提到 ¥ 前面 + 卡片整数不带小数
// 工程模块内的金额一律走这里，避免同屏出现 "¥1,286,400" 与 "1286400.00" 两种写法。

const toNum = value => {
  const num = Number(value)
  return Number.isFinite(num) ? num : null
}

/**
 * 卡片大数字：¥ + 千分位，整数不显示小数（§3.1）
 * 1286400 -> "¥1,286,400"；1286400.5 -> "¥1,286,400.50"；null -> "—"
 */
export const formatCardMoney = value => {
  const num = toNum(value)
  if (num === null) return '—'
  const abs = Math.abs(num)
  const digits = Number.isInteger(abs) ? 0 : 2
  const text = abs.toLocaleString('zh-CN', { minimumFractionDigits: digits, maximumFractionDigits: digits })
  return `${num < 0 ? '-' : ''}¥${text}`
}

/**
 * 表格 / 明细金额：¥ + 千分位 + 2 位；负数写成 -¥5,000.00（§3.1）
 * null 显示 "—"（不显示 0，避免"没数据"被看成"零元"）
 */
export const formatEngMoney = value => {
  const num = toNum(value)
  if (num === null) return '—'
  const abs = Math.abs(num)
  const text = abs.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  return `${num < 0 ? '-' : ''}¥${text}`
}

/**
 * 百分比：最多 2 位小数，去掉尾随 0（§3.1 "比例/百分比 number，最多 2 位小数"）
 * 14.33 -> "14.33%"；12.5 -> "12.5%"；20 -> "20%"；null -> "—"
 */
export const formatPercent = value => {
  const num = toNum(value)
  if (num === null) return '—'
  const rounded = Math.round(num * 100) / 100
  return `${rounded}%`
}

/**
 * 台数/件数类整数：空值显示 "—"
 */
export const formatQty = value => {
  const num = toNum(value)
  if (num === null) return '—'
  return String(Math.round(num))
}

/**
 * 业务日期显示：后端已是 YYYY-MM-DD（契约 §1.3），空值显示 "—"
 * 不做 slice，只在为空时兜底
 */
export const formatEngDate = value => (value ? String(value) : '—')
