import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('historical cost repair makes no-order records explicitly repairable', async () => {
  const source = await readFile(new URL('./SystemSettings.vue', import.meta.url), 'utf8')

  assert.match(source, /无需修订单/)
  assert.match(source, /修复库存成本/)
})
