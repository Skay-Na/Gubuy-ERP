import test from 'node:test'
import assert from 'node:assert/strict'
import { appendAgentAttachments, formatAgentAttachmentSize } from './agentAttachments.js'

function file(name, size = 10, overrides = {}) {
  return { name, size, lastModified: 1, type: '', ...overrides }
}

test('appendAgentAttachments keeps supported files and folder-relative paths', () => {
  const result = appendAgentAttachments([], [
    file('orders.csv', 120, { webkitRelativePath: '本周/orders.csv' }),
    file('instructions.exe')
  ])

  assert.equal(result.attachments.length, 1)
  assert.equal(result.attachments[0].relativePath, '本周/orders.csv')
  assert.equal(result.skipped.unsupported, 1)
})

test('appendAgentAttachments enforces count, per-file, total-size, and duplicate limits', () => {
  const currentFile = file('existing.txt', 1)
  const current = [{ id: 'existing', file: currentFile, name: currentFile.name, size: currentFile.size }]
  const files = [
    file('existing.txt', 1),
    file('empty.txt', 0),
    file('large.pdf', 8 * 1024 * 1024 + 1)
  ]
  const result = appendAgentAttachments(current, files)

  assert.equal(result.attachments.length, 1)
  assert.equal(result.skipped.duplicate, 1)
  assert.equal(result.skipped.oversized, 2)
  assert.equal(result.skipped.totalSize, 0)

  const fullSize = Array.from({ length: 5 }, (_, index) => ({
    id: String(index), file: file(`${index}.csv`, 8 * 1024 * 1024), name: `${index}.csv`, size: 8 * 1024 * 1024
  }))
  const totalCapped = appendAgentAttachments(fullSize, [file('extra.txt', 1)])
  assert.equal(totalCapped.skipped.totalSize, 1)

  const full = Array.from({ length: 20 }, (_, index) => ({
    id: String(index), file: file(`${index}.txt`), name: `${index}.txt`, size: 1
  }))
  const capped = appendAgentAttachments(full, [file('twenty-first.txt')])
  assert.equal(capped.attachments.length, 20)
  assert.equal(capped.skipped.count, 1)
})

test('formatAgentAttachmentSize uses readable byte units', () => {
  assert.equal(formatAgentAttachmentSize(512), '512 B')
  assert.equal(formatAgentAttachmentSize(1536), '1.5 KB')
  assert.equal(formatAgentAttachmentSize(2 * 1024 * 1024), '2.0 MB')
})
