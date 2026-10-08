function compact(value) {
  return String(value || '').toUpperCase().replace(/[^0-9A-Z\u4E00-\u9FFF]/g, '')
}

function modelTokens(value) {
  return String(value || '').toUpperCase().match(/[A-Z0-9-]*\d[A-Z0-9-]*/g) || []
}

function similarity(left, right) {
  if (!left || !right) return 0
  const rows = Array.from({ length: left.length + 1 }, (_, index) => index)
  for (let column = 1; column <= right.length; column += 1) {
    let diagonal = rows[0]
    rows[0] = column
    for (let row = 1; row <= left.length; row += 1) {
      const previous = rows[row]
      rows[row] = Math.min(rows[row] + 1, rows[row - 1] + 1, diagonal + (left[row - 1] === right[column - 1] ? 0 : 1))
      diagonal = previous
    }
  }
  return 1 - rows[left.length] / Math.max(left.length, right.length)
}

export function findBestProductMatch(recognizedName, products = []) {
  const source = compact(recognizedName)
  if (!source) return null
  const sourceModels = modelTokens(recognizedName)
  const ranked = products.map(product => {
    const target = compact(product.name)
    let score = similarity(source, target)
    if (source === target) score = 1
    else if (target.includes(source) || source.includes(target)) score = Math.max(score, 0.9 * Math.min(source.length, target.length) / Math.max(source.length, target.length) + 0.1)

    const targetModels = modelTokens(product.name)
    for (const sourceModel of sourceModels) {
      for (const targetModel of targetModels) {
        score = Math.max(score, similarity(compact(sourceModel), compact(targetModel)) * 0.95)
        const shorterModel = Math.min(sourceModel.length, targetModel.length)
        if (shorterModel >= 4 && (targetModel.includes(sourceModel) || sourceModel.includes(targetModel))) {
          score = Math.max(score, 0.82)
        }
      }
    }
    return { product, score }
  }).sort((left, right) => right.score - left.score)

  const best = ranked[0]
  const runnerUp = ranked[1]
  if (!best || best.score < 0.68 || (runnerUp && best.score - runnerUp.score < 0.08 && best.score < 0.9)) return null
  return { product: best.product, score: best.score }
}

export function findEmployeeMatch(recognizedName, employees = []) {
  const source = compact(recognizedName)
  if (!source) return null
  return employees.find(employee => compact(employee.name) === source)
    || employees.find(employee => source.length >= 2 && (compact(employee.name).includes(source) || source.includes(compact(employee.name))))
    || null
}

export function normalizeAIOrderFields(fields = {}, products = [], employees = []) {
  const amount = Number(fields.amount) > 0 ? Number(fields.amount) : null
  const items = (fields.items || []).map(item => {
    const quantity = Number(item.quantity) > 0 ? Number(item.quantity) : 1
    const match = findBestProductMatch(item.name, products)
    return {
      name: item.name || '',
      product_id: match?.product.id || null,
      match_score: match ? Number(match.score.toFixed(2)) : null,
      quantity,
      unit_price: Number(item.unit_price) > 0 ? Number(item.unit_price) : null
    }
  })

  // 手写单经常只填写合计栏。单商品时合计可以无歧义地反推成交单价。
  if (amount && items.length === 1 && !items[0].unit_price) {
    items[0].unit_price = Number((amount / items[0].quantity).toFixed(2))
  }

  const paymentChannel = String(fields.payment_channel || '')
  return {
    customer_name: fields.customer_name || '',
    customer_phone: fields.customer_phone || '',
    customer_address: fields.customer_address || '',
    employee_name: fields.employee_name || '',
    employee_id: findEmployeeMatch(fields.employee_name, employees)?.id || null,
    amount,
    payment_channel: paymentChannel,
    is_subsidy: Boolean(fields.is_subsidy) || paymentChannel.includes('云闪付'),
    items
  }
}
