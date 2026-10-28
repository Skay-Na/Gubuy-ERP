/**
 * 看板明细钻取列配置与公共格式化（纯数据模块，供 DashboardDrillDownDrawer 消费）。
 *
 * 与后端 GET /api/finance/breakdown/:type 的 type 枚举一一对应。
 * 每列可声明 mobileVisible：手机端（≤767px）抽屉全屏时仅保留核心三列（客户姓名/录单日期/金额）。
 */

/** 金额格式化：¥12,345.67 */
export const formatMoney = value => `¥${Number(value || 0).toLocaleString('zh-CN', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2
})}`

/** 日期格式化：截取 YYYY-MM-DD（后端已 DATE_FORMAT 纯日期串，兜底截断 ISO 时间戳） */
export const formatDate = value => String(value || '').slice(0, 10)

/** 流水方向：1-收入，2-支出 */
export const formatFlowType = value => (Number(value) === 1 ? '收入' : '支出')

/**
 * 期间文案：用于抽屉/利润过程区标题。
 * @param {{period?: string, start_date?: string, end_date?: string}} params
 * @returns {string}
 */
export function formatPeriodText(params = {}) {
  const labels = { today: '今日', month: '本月', last_month: '上月', year: '本年', all: '累计' }
  if (params.period === 'custom' && params.start_date && params.end_date) {
    return `${params.start_date} ~ ${params.end_date}`
  }
  return labels[params.period] || '本期'
}

/**
 * 组装带当前期间的钻取查询参数。
 * @param {string} period 当前看板期间
 * @param {string[]} customDates 自定义日期 [start, end]
 * @param {object} extra 类型专属参数（employee_id/account_id/only_uncosted 等）
 */
export function buildPeriodParams(period, customDates = [], extra = {}) {
  const params = { period, ...extra }
  if (period === 'custom' && customDates[0] && customDates[1]) {
    params.start_date = customDates[0]
    params.end_date = customDates[1]
  }
  return params
}

// —— 通用列片段 ——————————————————————————————————————————————

/** 订单类明细通用列：客户姓名 / 录单日期 / 订单号 / 金额（手机端保留前三列核心 + 金额） */
const ORDER_COLUMNS = [
  { prop: 'customer_name', label: '客户姓名', mobileVisible: true },
  { prop: 'created_at', label: '录单日期', formatter: formatDate, mobileVisible: true },
  { prop: 'order_no', label: '订单号', mobileVisible: false },
  { prop: 'amount', label: '金额', formatter: formatMoney, align: 'right', mobileVisible: true }
]

/** 费用类明细（金额列名替换为对应费用） */
const feeColumns = amountLabel => [
  { prop: 'customer_name', label: '客户姓名', mobileVisible: true },
  { prop: 'created_at', label: '录单日期', formatter: formatDate, mobileVisible: true },
  { prop: 'order_no', label: '订单号', mobileVisible: false },
  { prop: 'amount', label: amountLabel, formatter: formatMoney, align: 'right', mobileVisible: true }
]

// —— 类型元信息 ——————————————————————————————————————————————

/**
 * 明细类型元信息表：title 为默认标题，amountLabel 为底部合计行文案，columns 为列配置。
 * type 'net_profit' 为净利润公式汇总视图，由抽屉内部组装，不走明细接口。
 */
export const BREAKDOWN_TYPES = {
  revenue: { title: '销售额明细', amountLabel: '销售额合计', columns: ORDER_COLUMNS },
  cost: {
    title: '货品成本明细',
    amountLabel: '成本合计',
    columns: [
      { prop: 'customer_name', label: '客户姓名', mobileVisible: true },
      { prop: 'created_at', label: '录单日期', formatter: formatDate, mobileVisible: true },
      { prop: 'order_no', label: '订单号', mobileVisible: false },
      { prop: 'uncosted_count', label: '待核算件数', align: 'right', mobileVisible: false },
      { prop: 'amount', label: '成本小计', formatter: formatMoney, align: 'right', mobileVisible: true }
    ]
  },
  delivery: { title: '配送费明细', amountLabel: '配送费合计', columns: feeColumns('配送费') },
  installation: { title: '安装费明细', amountLabel: '安装费合计', columns: feeColumns('安装费') },
  referral: { title: '介绍人提成明细', amountLabel: '介绍人提成合计', columns: feeColumns('介绍人提成') },
  commission: { title: '业务提成明细', amountLabel: '业务提成合计', columns: feeColumns('业务提成') },
  trade_in: { title: '以旧换新折抵明细', amountLabel: '以旧换新合计', columns: feeColumns('以旧换新') },
  operating_expense: {
    title: '经营开销明细',
    amountLabel: '开销合计',
    columns: [
      { prop: 'expense_type', label: '费用类型', mobileVisible: true },
      { prop: 'created_at', label: '日期', formatter: formatDate, mobileVisible: true },
      { prop: 'remark', label: '备注', mobileVisible: false },
      { prop: 'amount', label: '金额', formatter: formatMoney, align: 'right', mobileVisible: true }
    ]
  },
  pending_balance: {
    title: '待收尾款明细',
    amountLabel: '待收合计',
    columns: [
      { prop: 'customer_name', label: '客户姓名', mobileVisible: true },
      { prop: 'created_at', label: '录单日期', formatter: formatDate, mobileVisible: true },
      { prop: 'order_no', label: '订单号', mobileVisible: false },
      { prop: 'amount', label: '待收尾款', formatter: formatMoney, align: 'right', mobileVisible: true }
    ]
  },
  pending_subsidy: {
    title: '国补待回款明细',
    amountLabel: '待回款合计',
    columns: [
      { prop: 'customer_name', label: '客户姓名', mobileVisible: true },
      { prop: 'created_at', label: '录单日期', formatter: formatDate, mobileVisible: true },
      { prop: 'order_no', label: '订单号', mobileVisible: false },
      { prop: 'amount', label: '待回款', formatter: formatMoney, align: 'right', mobileVisible: true }
    ]
  },
  employee_orders: { title: '员工订单明细', amountLabel: '销售额合计', columns: ORDER_COLUMNS },
  account_logs: {
    title: '账户流水明细',
    amountLabel: '流水合计',
    columns: [
      { prop: 'created_at', label: '日期', formatter: formatDate, mobileVisible: true },
      { prop: 'flow_type', label: '方向', formatter: formatFlowType, mobileVisible: true },
      { prop: 'category', label: '分类', mobileVisible: false },
      { prop: 'counterparty', label: '往来对象', mobileVisible: false },
      { prop: 'related_no', label: '关联单号', mobileVisible: false },
      { prop: 'balance_after', label: '交易后余额', formatter: formatMoney, mobileVisible: false },
      { prop: 'amount', label: '金额', formatter: formatMoney, align: 'right', mobileVisible: true }
    ]
  },
  trend_orders: { title: '趋势区间订单明细', amountLabel: '销售额合计', columns: ORDER_COLUMNS }
}

/** 净利润公式视图类型常量（抽屉内部视图，非后端明细类型） */
export const NET_PROFIT_VIEW_TYPE = 'net_profit'

/**
 * 获取类型元信息。
 * @param {string} type 明细类型
 * @returns {{title: string, amountLabel: string, columns: Array}|null}
 */
export function getBreakdownMeta(type) {
  return BREAKDOWN_TYPES[type] || null
}
