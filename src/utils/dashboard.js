export const formatCurrency = (value) => `¥${Number(value || 0).toLocaleString('zh-CN', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2
})}`

export const stockTotal = (product = {}) => Number(product.main_stock || 0) + Number(product.store_stock || 0) + Number(product.cloud_stock || 0)

const needsDelivery = (order = {}) => {
  if (Number(order.delivery_method) === 1) return false
  const items = order.order_items || []
  return items.length > 0 ? items.some(item => !item.is_delivered) : !order.is_delivered
}

const needsInstall = (order = {}) => {
  if (Number(order.delivery_method) === 1) return false
  const items = order.order_items || []
  return items.length > 0
    ? items.some(item => item.is_delivered && !item.is_installed)
    : Boolean(order.is_delivered && !order.is_installed)
}

export function buildDashboardTodos({ summary = {}, pendingOrders = [], pendingInbounds = [], products = [], orders = [] } = {}) {
  const activeOrders = orders.filter(order => Number(order.order_status) === 1)
  const pendingDelivery = activeOrders.filter(needsDelivery)
  const pendingInstall = activeOrders.filter(needsInstall)
  const lowStock = products.filter(product => stockTotal(product) < 5)
  const pendingSubsidyOrders = activeOrders.filter(order => [1, 2].includes(Number(order.subsidy_status)))

  const items = [
    {
      key: 'pendingBalance',
      title: '尾款待催收',
      count: Number(summary.pending_balance_count || pendingOrders.length || 0),
      amount: Number(summary.pending_balance || 0),
      level: 'high',
      route: '/admin/orders',
      action: '去催收'
    },
    {
      key: 'pendingInbound',
      title: '员工入库待审核',
      count: pendingInbounds.length,
      amount: 0,
      level: 'high',
      route: '/admin/products/inbound',
      action: '去审核'
    },
    {
      key: 'pendingDelivery',
      title: '待安排送货',
      count: pendingDelivery.length,
      amount: 0,
      level: 'medium',
      route: '/admin/orders',
      action: '去处理'
    },
    {
      key: 'pendingInstall',
      title: '待确认安装',
      count: pendingInstall.length,
      amount: 0,
      level: 'medium',
      route: '/admin/orders',
      action: '去处理'
    },
    {
      key: 'pendingSubsidy',
      title: '国补待提交/回款',
      count: pendingSubsidyOrders.length,
      amount: Number(summary.pending_subsidy || 0),
      level: Number(summary.pending_subsidy || 0) > 0 ? 'high' : 'low',
      route: '/admin/subsidy',
      action: '去国补'
    },
    {
      key: 'lowStock',
      title: '低库存商品',
      count: lowStock.length,
      amount: 0,
      level: lowStock.length > 0 ? 'medium' : 'low',
      route: '/admin/products',
      action: '去补货'
    }
  ]

  return {
    pendingBalance: items[0],
    pendingInbound: items[1],
    pendingDelivery: items[2],
    pendingInstall: items[3],
    pendingSubsidy: items[4],
    lowStock: items[5],
    items,
    totalCount: items.reduce((sum, item) => sum + item.count, 0)
  }
}

export function buildProductInsights({ topProducts = [], products = [] } = {}) {
  const productsByID = new Map(products.map(product => [Number(product.id), product]))
  const productsByName = new Map(products.map(product => [product.name, product]))
  const hotProducts = topProducts.slice(0, 8).map(item => {
    const product = productsByID.get(Number(item.product_id)) || productsByName.get(item.name) || {}
    return {
      ...item,
      stockTotal: stockTotal(product),
      main_stock: product.main_stock || 0,
      store_stock: product.store_stock || 0,
      cloud_stock: product.cloud_stock || 0
    }
  })

  return {
    hotProducts,
    hotLowStock: hotProducts.filter(item => item.stockTotal < 5).sort((a, b) => a.stockTotal - b.stockTotal),
    lowStock: products
      .map(product => ({ ...product, stockTotal: stockTotal(product) }))
      .filter(product => product.stockTotal < 5)
      .sort((a, b) => a.stockTotal - b.stockTotal)
      .slice(0, 10)
  }
}
