import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const orderSource = await readFile(new URL('./OrderManage.vue', import.meta.url), 'utf8')
const payrollSource = await readFile(new URL('../components/employee/PayrollWorkspace.vue', import.meta.url), 'utf8')

test('order history filters by one complete month', () => {
  assert.match(orderSource, /v-model="selectedMonth"[\s\S]*?type="month"/)
  assert.doesNotMatch(orderSource, /type="daterange"/)
  assert.match(orderSource, /startOf\('month'\)\.toISOString\(\)/)
  assert.match(orderSource, /endOf\('month'\)\.toISOString\(\)/)
})

test('payroll summary separates personal and chamber commissions', () => {
  assert.match(payrollSource, /total\.personalCommission[\s\S]*?row\.personal_commission/)
  assert.match(payrollSource, /total\.chamberCommission[\s\S]*?row\.chamber_commission/)
  assert.match(payrollSource, />个人提成合计</)
  assert.match(payrollSource, />商会提成合计</)
})
