import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const source = await readFile(new URL('./StaffTerminal.vue', import.meta.url), 'utf8')

function extractComputed(name) {
  const re = new RegExp(`const ${name} = computed\\(\\(\\) => \\{([\\s\\S]*?)\\}\\)`)
  const match = source.match(re)
  return match ? match[1].trim() : null
}

test('subsidized full-payment mixed split includes the POS gross subsidy amount', () => {
  const body = extractComputed('mixedPaymentTarget')

  assert.ok(body, 'mixedPaymentTarget computed 应存在')
  assert.match(body, /isSubsidized\.value && !isDepositMode\.value/)
  assert.match(body, /actualReceipt\.value[\s\S]*subsidyAmount\.value/)
})

test('subsidized mixed-payment guidance distinguishes gross POS charge from net arrival', () => {
  assert.match(source, /原始金额\(含国补\)/)
  assert.match(source, /公户实际到账金额会扣除国补/)
})
