export const AGENT_ATTACHMENT_LIMITS = Object.freeze({
  maxFiles: 20,
  maxFileBytes: 8 * 1024 * 1024,
  maxTotalBytes: 40 * 1024 * 1024
})

export const AGENT_ATTACHMENT_EXTENSIONS = Object.freeze([
  '.jpg', '.jpeg', '.png', '.webp', '.pdf', '.docx', '.xlsx', '.csv', '.txt', '.md'
])

let attachmentSequence = 0

function extensionOf(name) {
  const separator = name.lastIndexOf('.')
  return separator < 0 ? '' : name.slice(separator).toLowerCase()
}

function pathOf(file) {
  return (file.webkitRelativePath || file.name).replaceAll('\\', '/')
}

function attachmentKey(file) {
  return `${pathOf(file)}\u0000${file.size}\u0000${file.lastModified}`
}

export function appendAgentAttachments(current, files) {
  const attachments = [...current]
  const skipped = { unsupported: 0, oversized: 0, count: 0, totalSize: 0, duplicate: 0 }
  const keys = new Set(current.map(item => attachmentKey(item.file)))
  let totalBytes = current.reduce((sum, item) => sum + item.size, 0)

  for (const file of files) {
    if (!AGENT_ATTACHMENT_EXTENSIONS.includes(extensionOf(file.name))) {
      skipped.unsupported++
      continue
    }
    if (!file.size || file.size > AGENT_ATTACHMENT_LIMITS.maxFileBytes) {
      skipped.oversized++
      continue
    }
    const key = attachmentKey(file)
    if (keys.has(key)) {
      skipped.duplicate++
      continue
    }
    if (attachments.length >= AGENT_ATTACHMENT_LIMITS.maxFiles) {
      skipped.count++
      continue
    }
    if (totalBytes + file.size > AGENT_ATTACHMENT_LIMITS.maxTotalBytes) {
      skipped.totalSize++
      continue
    }

    keys.add(key)
    totalBytes += file.size
    attachments.push({
      id: `agent-file-${Date.now()}-${++attachmentSequence}`,
      file,
      name: file.name,
      relativePath: file.webkitRelativePath || '',
      size: file.size,
      mediaType: file.type || ''
    })
  }

  return { attachments, skipped }
}

export function formatAgentAttachmentSize(size) {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}
