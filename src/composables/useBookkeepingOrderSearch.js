import { readonly, shallowRef } from 'vue'
import { searchBookkeepingOrders } from '../api/finance'

export function useBookkeepingOrderSearch() {
  const orderOptions = shallowRef([])
  const orderSearchLoading = shallowRef(false)

  async function fetchOrders(keyword = '') {
    const response = await searchBookkeepingOrders({ q: keyword.trim() })
    return response.data.code === 200 ? response.data.data || [] : []
  }

  async function searchOrders(keyword = '') {
    orderSearchLoading.value = true
    try {
      orderOptions.value = await fetchOrders(keyword)
      return orderOptions.value
    } finally {
      orderSearchLoading.value = false
    }
  }

  async function searchCustomers(keyword, callback) {
    try {
      const orders = await fetchOrders(keyword)
      const seen = new Set()
      const suggestions = []
      for (const order of orders) {
        const customerName = order.customer_name?.trim()
        if (!customerName || seen.has(customerName)) continue
        seen.add(customerName)
        suggestions.push({
          value: customerName,
          phone: order.customer_phone,
          orderNo: order.order_no
        })
      }
      callback(suggestions)
    } catch {
      callback([])
    }
  }

  return {
    orderOptions: readonly(orderOptions),
    orderSearchLoading: readonly(orderSearchLoading),
    searchOrders,
    searchCustomers
  }
}
