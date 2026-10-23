const money = (value) => `¥${Number(value || 0).toFixed(2)}`

const dateTimeFormatter = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false
})

export const paymentMethodLabel = (method) => ({
  1: '支付宝',
  2: '微信',
  3: '公户',
  4: '混合支付',
  5: '现金'
}[Number(method)] || '其他')

export const parsePaymentBreakdown = (breakdown) => {
  if (!breakdown) return ''
  try {
    const items = typeof breakdown === 'string' ? JSON.parse(breakdown) : breakdown
    if (!Array.isArray(items)) return ''
    return items
      .map(item => `${paymentMethodLabel(item.payment_method)} ¥${Number(item.amount || 0).toFixed(2)}`)
      .join(' / ')
  } catch (error) {
    return ''
  }
}

export const deliveryMethodLabel = (method) => ({
  1: '门店自提',
  2: '主仓发货',
  3: '云仓代发',
  4: '云仓发货'
}[Number(method)] || '其他')

const formatDateTime = (value) => {
  const date = value ? new Date(value) : new Date()
  if (Number.isNaN(date.getTime())) return '-'
  return dateTimeFormatter.format(date).replaceAll('/', '-')
}

// 店铺信息 - 默认值参考历史使用的发货单样式，可在系统设置中覆盖
export const storeConfig = {
  name: '芮城美的智慧家',
  address: '县人民医院北200米路东',
  phone: '15333599478'
}

export const resolveStoreInfo = (override) => ({
  name: override?.name?.trim() || storeConfig.name,
  address: override?.address?.trim() || storeConfig.address,
  phone: override?.phone?.trim() || storeConfig.phone
})

// 数字转中文大写金额（仅支持元）
const CN_DIGITS = ['零', '壹', '贰', '叁', '肆', '伍', '陆', '柒', '捌', '玖']
const CN_INTEGER_UNITS = ['', '拾', '佰', '仟']
const CN_BIG_UNITS = ['', '万', '亿', '万亿']

// 0–9999 的固定 4 位中文表达：保留 0 占位，便于跨 chunk 拼装
const chunkToChinese = (chunk) => {
  if (chunk === 0) return ''
  const digits = String(chunk).padStart(4, '0')
  let result = ''
  let pendingZero = false
  for (let i = 0; i < 4; i++) {
    const d = parseInt(digits[i], 10)
    const unitPos = 3 - i
    if (d === 0) {
      pendingZero = true
    } else {
      if (pendingZero && result) result += '零'
      result += CN_DIGITS[d] + CN_INTEGER_UNITS[unitPos]
      pendingZero = false
    }
  }
  return result
}

const integerToChinese = (n) => {
  if (n === 0) return ''
  const str = String(n)
  const len = str.length
  // 按 4 位一组从右往左切，最高位的 chunk 可能不足 4 位
  const chunks = []
  let pos = len
  while (pos > 0) {
    const start = Math.max(0, pos - 4)
    chunks.unshift(parseInt(str.substring(start, pos), 10))
    pos = start
  }
  // chunks[0] = HIGHEST, chunks[length-1] = LOWEST

  let result = ''
  let lastNonZeroIndex = -1

  for (let i = chunks.length - 1; i >= 0; i--) {
    const chunk = chunks[i]
    const bigUnitIndex = chunks.length - 1 - i
    if (chunk <= 0) continue

    const cnStr = chunkToChinese(chunk)
    const hasMissingHighDigits = String(chunk).length < 4

    // 跨 chunk 的「零」补位：
    // 1. 上一非零 chunk 与当前 chunk 之间隔着 0 chunk → 需要 零
    // 2. 当前 chunk 自身高位缺失（< 1000）且对应大单位存在 → 需要 零
    if (lastNonZeroIndex >= 0) {
      if (i - lastNonZeroIndex > 1) {
        result = '零' + result
      } else if (bigUnitIndex > 0 && hasMissingHighDigits) {
        result = '零' + result
      }
    }

    if (bigUnitIndex > 0) {
      result = CN_BIG_UNITS[bigUnitIndex] + result
    }
    result = cnStr + result
    lastNonZeroIndex = i
  }

  return result
}

export const numberToChineseCapital = (value) => {
  const amount = Number(value || 0)
  if (Number.isNaN(amount)) return ''
  if (amount === 0) return '零元整'
  const sign = amount < 0 ? '负' : ''
  const positive = Math.abs(amount)
  const integerPart = Math.floor(positive)
  const cents = Math.round((positive - integerPart) * 100)
  let result = sign
  if (integerPart > 0) {
    result += integerToChinese(integerPart) + '元'
  }
  if (cents === 0) {
    if (integerPart > 0) result += '整'
  } else {
    const jiao = Math.floor(cents / 10)
    const fen = cents % 10
    if (jiao > 0) result += CN_DIGITS[jiao] + '角'
    else if (integerPart > 0) result += '零'
    if (fen > 0) result += CN_DIGITS[fen] + '分'
  }
  return result
}

// 把金额拆成 6 个数字位（十万/万/千/百/十/元），保留 '0' 占位
export const amountToDigitCells = (value, cells = 6) => {
  const integer = Math.floor(Number(value || 0))
  return String(integer).padStart(cells, '0').slice(-cells).split('')
}

// 单行金额显示用：跳过 0 占位（保留视觉留空，便于手写填空）
export const amountToFilledCells = (value, cells = 6) => {
  const integer = Math.floor(Number(value || 0))
  return String(integer).padStart(cells, '0').slice(-cells)
    .split('').map((d) => (d === '0' ? '' : d))
}

const DEFAULT_UNIT = '台'

const normalizeItems = (items = []) => items.map((item, index) => {
  const product = item.product || {}
  const name = item.name || product.name || '未知商品'
  const specification = item.specification || product.specification || ''
  const model = item.model || product.model || ''
  const categoryName = item.category_name || product.category?.name || product.Category?.name || '-'
  const quantity = Number(item.quantity || 0)
  const unit = item.unit || product.unit || DEFAULT_UNIT
  const unitPrice = Number(item.price ?? item.unit_price ?? 0)
  const isGift = Boolean(item.isGift ?? item.is_gift)
  const lineTotal = isGift ? 0 : unitPrice * quantity
  const note = item.note || item.remark || item.notes || ''
  return {
    key: item.id || item.product_id || `${name}-${index}`,
    name,
    specification,
    model,
    categoryName,
    unit,
    quantity,
    note,
    unitPrice,
    unitPriceText: isGift ? '赠品' : money(unitPrice),
    isGift,
    lineTotal,
    lineTotalText: isGift ? '赠品' : money(lineTotal),
    lineTotalDigits: isGift ? amountToFilledCells(0) : amountToFilledCells(lineTotal)
  }
})

export function buildReceiptFromCheckout({
  orderNo,
  createdAt,
  customer = {},
  items = [],
  totals = {},
  paymentMethod,
  deliveryMethod,
  employeeName = '',
  referrerName = '',
  paymentBreakdown = '',
  store = null
}) {
  const depositAmount = Number(totals.depositAmount || 0)
  const actualPayAmount = Number(totals.actualPayAmount || 0)
  const receiptAmount = Number(totals.receiptAmount ?? (depositAmount > 0 ? depositAmount : actualPayAmount))

  const storeInfo = resolveStoreInfo(store)
  const totalForDisplay = depositAmount > 0 ? depositAmount : actualPayAmount

  return {
    title: '发货单',
    storeName: storeInfo.name,
    storeAddress: storeInfo.address,
    storePhone: storeInfo.phone,
    orderNo: orderNo || '待生成',
    createdAt: formatDateTime(createdAt),
    createdAtText: formatDateTime(createdAt),
    customer: {
      name: customer.name || '散客',
      phone: customer.phone || '-',
      address: customer.address || '-'
    },
    employeeName: employeeName || '-',
    referrerName: referrerName || '无',
    items: normalizeItems(items),
    summary: {
      totalAmount: Number(totals.totalAmount || 0),
      subsidyAmount: Number(totals.subsidyAmount || 0),
      tradeInAmount: Number(totals.tradeInAmount || 0),
      discountAmount: Number(totals.discountAmount || 0),
      actualPayAmount,
      receiptAmount,
      depositAmount,
      totalAmountText: money(totals.totalAmount),
      subsidyAmountText: money(totals.subsidyAmount),
      tradeInAmountText: money(totals.tradeInAmount),
      discountAmountText: money(totals.discountAmount),
      actualPayAmountText: money(actualPayAmount),
      receiptAmountText: money(receiptAmount),
      depositAmountText: money(depositAmount),
      amountInWords: numberToChineseCapital(totalForDisplay),
      amountDigits: amountToDigitCells(totalForDisplay)
    },
    paymentMethodLabel: paymentMethodLabel(paymentMethod),
    deliveryMethodLabel: deliveryMethodLabel(deliveryMethod),
    paymentBreakdown,
    footer: `地址：${storeInfo.address}　电话：${storeInfo.phone}`
  }
}

export function buildReceiptFromOrder(order = {}) {
  const receiptAmount = Number(order.payment_status) === 1
    ? Number(order.deposit_amount || 0)
    : Number(order.actual_pay_amount || 0)

  return buildReceiptFromCheckout({
    orderNo: order.order_no,
    createdAt: order.created_at || order.CreatedAt || order.updated_at,
    customer: {
      name: order.customer_name,
      phone: order.customer_phone,
      address: order.customer_address
    },
    items: order.order_items || order.OrderItems || [],
    totals: {
      totalAmount: order.total_amount,
      subsidyAmount: order.subsidy_amount,
      tradeInAmount: order.trade_in_amount,
      discountAmount: order.discount_amount,
      actualPayAmount: order.actual_pay_amount,
      receiptAmount,
      depositAmount: order.deposit_amount
    },
    paymentMethod: order.payment_method,
    deliveryMethod: order.delivery_method,
    employeeName: order.employee?.name || order.Employee?.name || '',
    referrerName: order.referrer_name,
    paymentBreakdown: parsePaymentBreakdown(order.payment_breakdown) || ''
  })
}
