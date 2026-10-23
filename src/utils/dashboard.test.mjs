import test from 'node:test'
import assert from 'node:assert/strict'
import { formatCurrency } from './dashboard.js'

test('formatCurrency keeps compact Chinese-friendly currency', () => {
  assert.equal(formatCurrency(1234.5), '¥1,234.50')
  assert.equal(formatCurrency(null), '¥0.00')
})
