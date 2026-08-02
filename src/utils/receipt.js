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
  3: '云仓代发'
}[Number(method)] || '其他')

const formatDateTime = (value) => {
  const date = value ? new Date(value) : new Date()
  if (Number.isNaN(date.getTime())) return '-'
  return dateTimeFormatter.format(date).replaceAll('/', '-')
}

const normalizeItems = (items = []) => items.map((item, index) => {
  const product = item.product || {}
  const name = item.name || product.name || '未知商品'
  const specification = item.specification || product.specification || ''
  const model = item.model || product.model || ''
  const categoryName = item.category_name || product.category?.name || product.Category?.name || '-'
  const quantity = Number(item.quantity || 0)
  const unitPrice = Number(item.price ?? item.unit_price ?? 0)
  const isGift = Boolean(item.isGift ?? item.is_gift)
  const lineTotal = isGift ? 0 : unitPrice * quantity
  return {
    key: item.id || item.product_id || `${name}-${index}`,
    name,
    specification,
    model,
    categoryName,
    specText: [specification, model].filter(Boolean).join(' / '),
    quantity,
    unitPrice,
    unitPriceText: isGift ? '赠品' : money(unitPrice),
    isGift,
    lineTotal,
    lineTotalText: isGift ? '赠品' : money(lineTotal)
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
  paymentBreakdown = ''
}) {
  const depositAmount = Number(totals.depositAmount || 0)
  const actualPayAmount = Number(totals.actualPayAmount || 0)
  const receiptAmount = Number(totals.receiptAmount ?? (depositAmount > 0 ? depositAmount : actualPayAmount))

  return {
    title: '购物小票',
    storeName: '美的智慧家',
    orderNo: orderNo || '待生成',
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
      depositAmountText: money(depositAmount)
    },
    paymentMethodLabel: paymentMethodLabel(paymentMethod),
    deliveryMethodLabel: deliveryMethodLabel(deliveryMethod),
    paymentBreakdown,
    footer: '感谢惠顾，请妥善保管小票'
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
