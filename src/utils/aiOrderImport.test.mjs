import assert from 'node:assert/strict'
import test from 'node:test'

import { findBestProductMatch, normalizeAIOrderFields } from './aiOrderImport.js'

test('single item uses recognized total as its unit price', () => {
  const result = normalizeAIOrderFields({
    amount: 4705,
    items: [{ name: 'TCL 85K585', quantity: 1, unit_price: 0 }]
  })

  assert.equal(result.amount, 4705)
  assert.equal(result.items[0].unit_price, 4705)
})

test('single item divides recognized total by quantity', () => {
  const result = normalizeAIOrderFields({
    amount: 4705,
    items: [{ name: '测试商品', quantity: 5 }]
  })

  assert.equal(result.items[0].unit_price, 941)
})

test('cloud quickpass marks order as subsidized and keeps product match', () => {
  const result = normalizeAIOrderFields({
    payment_channel: '云闪付',
    items: [{ name: 'TCL 85K585', quantity: 1, unit_price: 4705 }]
  }, [{ id: 8, name: 'TCL 85K585' }])

  assert.equal(result.is_subsidy, true)
  assert.equal(result.items[0].product_id, 8)
})

test('multiple missing item prices are not guessed from one total', () => {
  const result = normalizeAIOrderFields({
    amount: 4705,
    items: [{ name: '商品一', quantity: 1 }, { name: '商品二', quantity: 1 }]
  })

  assert.equal(result.items[0].unit_price, null)
  assert.equal(result.items[1].unit_price, null)
})

test('abbreviated handwritten model matches a longer database product name', () => {
  const products = [
    { id: 1, name: '美的空调 KFR-35GW' },
    { id: 2, name: 'TCL电视 85K585 85英寸' }
  ]

  const match = findBestProductMatch('TCL 85K585', products)
  assert.equal(match.product.id, 2)
  assert.ok(match.score >= 0.9)
})

test('partial but unique model can match while ambiguous results stay unselected', () => {
  const unique = findBestProductMatch('K585', [
    { id: 1, name: 'TCL电视 85K585 85英寸' },
    { id: 2, name: '美的冰箱 BCD-508' }
  ])
  assert.equal(unique.product.id, 1)

  const ambiguous = findBestProductMatch('K585', [
    { id: 1, name: 'TCL电视 85K585 85英寸' },
    { id: 2, name: 'TCL电视 75K585 75英寸' }
  ])
  assert.equal(ambiguous, null)
})

test('recognized handler is mapped to order employee', () => {
  const result = normalizeAIOrderFields({ employee_name: '张师傅', items: [] }, [], [
    { id: 3, name: '张师傅' },
    { id: 4, name: '李师傅' }
  ])

  assert.equal(result.employee_id, 3)
  assert.equal(result.employee_name, '张师傅')
})
