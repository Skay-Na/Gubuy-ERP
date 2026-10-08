import test from 'node:test'
import assert from 'node:assert/strict'
import { buildDashboardTodos, buildProductInsights, formatCurrency } from './dashboard.js'

test('buildDashboardTodos summarizes pending operational actions', () => {
  const todos = buildDashboardTodos({
    summary: { pending_balance: 8200, pending_balance_count: 3, pending_subsidy: 1500 },
    pendingOrders: [{ id: 1 }, { id: 2 }, { id: 3 }],
    pendingInbounds: [{ id: 101 }, { id: 102 }],
    products: [
      { id: 1, name: '热销冰箱', main_stock: 1, store_stock: 0, cloud_stock: 0 },
      { id: 2, name: '安全电视', main_stock: 10, store_stock: 2, cloud_stock: 0 }
    ],
    orders: [
      { id: 1, order_status: 1, subsidy_status: 1, is_delivered: false, is_installed: false, delivery_method: 2, order_items: [{ is_delivered: false, is_installed: false }] },
      { id: 2, order_status: 1, subsidy_status: 2, is_delivered: true, is_installed: false, delivery_method: 2, order_items: [{ is_delivered: true, is_installed: false }] },
      { id: 3, order_status: 2, is_delivered: false, is_installed: false, delivery_method: 2 }
    ]
  })

  assert.equal(todos.pendingBalance.count, 3)
  assert.equal(todos.pendingBalance.amount, 8200)
  assert.equal(todos.pendingInbound.count, 2)
  assert.equal(todos.pendingInbound.route, '/admin/products/inbound')
  assert.equal(todos.pendingInbound.level, 'high')
  assert.equal(todos.pendingDelivery.count, 1)
  assert.equal(todos.pendingInstall.count, 1)
  assert.equal(todos.pendingSubsidy.count, 2)
  assert.equal(todos.lowStock.count, 1)
  assert.equal(todos.totalCount, 10)
})

test('buildProductInsights combines hot products with low stock priority', () => {
  const insights = buildProductInsights({
    topProducts: [
      { product_id: 1, name: '热销冰箱', quantity: 8, revenue: 12000 },
      { product_id: 2, name: '普通电视', quantity: 2, revenue: 4000 }
    ],
    products: [
      { id: 1, name: '热销冰箱', main_stock: 1, store_stock: 1, cloud_stock: 0 },
      { id: 2, name: '普通电视', main_stock: 8, store_stock: 2, cloud_stock: 0 },
      { id: 3, name: '低库存但没卖', main_stock: 1, store_stock: 0, cloud_stock: 0 }
    ]
  })

  assert.equal(insights.hotProducts.length, 2)
  assert.equal(insights.hotLowStock[0].name, '热销冰箱')
  assert.equal(insights.lowStock[0].stockTotal, 1)
})

test('buildProductInsights uses product id when names are duplicated', () => {
  const insights = buildProductInsights({
    topProducts: [{ product_id: 1, name: '同名商品', quantity: 3, revenue: 3000 }],
    products: [
      { id: 1, name: '同名商品', main_stock: 9, store_stock: 0, cloud_stock: 0 },
      { id: 2, name: '同名商品', main_stock: 1, store_stock: 0, cloud_stock: 0 }
    ]
  })

  assert.equal(insights.hotProducts[0].stockTotal, 9)
  assert.equal(insights.hotLowStock.length, 0)
})

test('formatCurrency keeps compact Chinese-friendly currency', () => {
  assert.equal(formatCurrency(1234.5), '¥1,234.50')
  assert.equal(formatCurrency(null), '¥0.00')
})
