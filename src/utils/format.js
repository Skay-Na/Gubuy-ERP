// 统一格式化工具（T4 stretch）：金额、日期时间展示口径全局一致
// 金额：千分位 + 固定 2 位小数；日期：YYYY-MM-DD；日期时间：YYYY-MM-DD HH:mm

const pad = n => String(n).padStart(2, '0')

/**
 * 金额格式化：千分位 + 2 位小数（null/undefined/非数值归零）
 * @param {number|string} value
 * @returns {string} 例如 1234.5 -> "1,234.50"
 */
export const formatMoney = value => {
  const num = Number(value)
  if (!Number.isFinite(num)) return '0.00'
  return num.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

/**
 * 日期格式化：YYYY-MM-DD（空值返回空串）
 * @param {string|number|Date} value
 * @returns {string}
 */
export const formatDate = value => {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/**
 * 日期时间格式化：YYYY-MM-DD HH:mm（空值返回空串）
 * @param {string|number|Date} value
 * @returns {string}
 */
export const formatDateTime = value => {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  return `${formatDate(d)} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
