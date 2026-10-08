import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const routerSource = await readFile(new URL('../router/index.js', import.meta.url), 'utf8')
const layoutSource = await readFile(new URL('../layout/Index.vue', import.meta.url), 'utf8')
const workspaceSource = await readFile(new URL('./ProductWorkspace.vue', import.meta.url), 'utf8')

test('inventory tools are grouped below the product section', () => {
  assert.match(routerSource, /path: 'products',[\s\S]*?path: 'inbound',[\s\S]*?path: 'inventory-logs'/)
  assert.match(workspaceSource, /商品档案/)
  assert.match(workspaceSource, /采购入库/)
  assert.match(workspaceSource, /库存流水/)
})

test('legacy inventory routes redirect and no longer occupy sidebar items', () => {
  assert.match(routerSource, /path: 'purchase',[\s\S]*?redirect: '\/admin\/products\/inbound'/)
  assert.match(routerSource, /path: 'inventory-logs',[\s\S]*?redirect: '\/admin\/products\/inventory-logs'/)
  assert.doesNotMatch(layoutSource, /title: '入库'/)
  assert.doesNotMatch(layoutSource, /title: '库存流水'/)
})
