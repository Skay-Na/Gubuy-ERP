// 商品与分类相关 API
import request from '../utils/request'

// 查询商品列表（all=1 表示不过滤在售状态）
export const fetchProducts = params => request.get('/api/products', { params })

// 新建商品
export const createProduct = data => request.post('/api/products', data)

// 更新商品
export const updateProduct = (productId, data) => request.put(`/api/products/${productId}`, data)

// 查询单个商品的库存变动日志
export const fetchProductLogs = (productId, params) => request.get(`/api/products/${productId}/logs`, { params })

// 商品调拨
export const transferProduct = (productId, data) => request.put(`/api/products/${productId}/transfer`, data)

// 商品盘点
export const stocktakeProduct = (productId, data) => request.post(`/api/products/${productId}/stocktake`, data)

// 重算商品成本
export const recalculateProductCosts = () => request.post('/api/products/recalculate-costs')

// 查询分类列表
export const fetchCategories = params => request.get('/api/categories', { params })

// 新建分类
export const createCategory = data => request.post('/api/categories', data)

// 更新分类
export const updateCategory = (categoryId, data) => request.put(`/api/categories/${categoryId}`, data)

// 删除分类
export const deleteCategory = categoryId => request.delete(`/api/categories/${categoryId}`)
