import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('historical cost repair supports per-order and stock-cost fixes', async () => {
  const source = await readFile(new URL('./SystemSettings.vue', import.meta.url), 'utf8')

  // 商品级：修复库存成本；订单级：修复此订单（逐笔核对，不做批量改写）
  assert.match(source, /修复库存成本/)
  assert.match(source, /修复此订单/)
  assert.match(source, /不会批量改写历史订单/)
})
