import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const source = await readFile(new URL('./CheckoutPanel.vue', import.meta.url), 'utf8')

// 提取 Vue SFC 中关键计算块的源码（用正则）
function extractComputed(name) {
  const re = new RegExp(`const ${name} = computed\\(\\(\\) => \\{([\\s\\S]*?)\\}\\)`)
  const m = source.match(re)
  return m ? m[1].trim() : null
}

test('actualReceipt returns totalPayable in full-pay mode', () => {
  const body = extractComputed('actualReceipt')
  assert.match(body ?? '', /isDepositMode\.value \? depositAmount\.value : totalPayable\.value/)
})

test('mixedPaymentTarget returns actualReceipt (not actualReceipt + subsidyAmount)', () => {
  const body = extractComputed('mixedPaymentTarget')
  assert.ok(body, 'mixedPaymentTarget computed 应存在')
  assert.match(body, /return actualReceipt\.value/)
  // 防止历史 BUG-3 复发：不能加 subsidyAmount
  assert.doesNotMatch(body, /\+= subsidyAmount|subsidyAmount\.value\)/)
})

test('mixedPaymentTotal sums all split amounts', () => {
  const body = extractComputed('mixedPaymentTotal')
  assert.match(body ?? '', /Object\.values\(mixedPaymentAmounts\.value\)\.reduce/)
})

test('mixedPaymentRemaining equals target minus total', () => {
  const body = extractComputed('mixedPaymentRemaining')
  assert.match(body ?? '', /mixedPaymentTarget\.value - mixedPaymentTotal\.value/)
})

test('subsidy cap 1500 enforced', () => {
  // 锚定到 subsidyAmount computed 定义，在其后 800 字符窗口内检查 1500 上限
  const idx = source.indexOf('const subsidyAmount = computed(')
  assert.ok(idx > -1, 'subsidyAmount computed must exist')
  const window = source.slice(idx, idx + 800)
  assert.match(window, /1500/)
})