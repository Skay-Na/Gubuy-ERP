// 订单与订单项相关 API
import request from '../utils/request'

// 查询订单列表
export const fetchOrders = params => request.get('/api/orders', { params })

// 创建订单（含行项与支付信息）
export const createOrder = data => request.post('/api/orders', data)

// 确认补差价（尾款）支付
export const confirmPayBalance = (orderId, data) => request.put(`/api/orders/${orderId}/balance`, data)

// 确认国补到账（target_status: 2）
export const confirmSubsidyReceived = orderId => request.put(`/api/orders/${orderId}/subsidy`, { target_status: 2 })

// 取消订单
export const cancelOrder = orderId => request.put(`/api/orders/${orderId}/cancel`)

// 反结账取消订单
export const reversalCancelOrder = orderId => request.put(`/api/orders/${orderId}/reversal-cancel`)

// 设置订单提成
export const setOrderCommission = (orderId, data) => request.put(`/api/orders/${orderId}/commission`, data)

// 设置订单转介绍人
export const setOrderReferrer = (orderId, data) => request.put(`/api/orders/${orderId}/referrer`, data)

// 订单项发货确认
export const deliverOrderItem = (itemId, data) => request.put(`/api/order-items/${itemId}/deliver`, data)

// 修改订单项运费
export const updateDeliveryFee = (itemId, data) => request.put(`/api/order-items/${itemId}/delivery-fee`, data)

// 订单项安装确认
export const installOrderItem = (itemId, data) => request.put(`/api/order-items/${itemId}/install`, data)

// 修改订单项安装费
export const updateInstallationFee = (itemId, data) => request.put(`/api/order-items/${itemId}/installation-fee`, data)
