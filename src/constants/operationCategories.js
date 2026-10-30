// 历史操作分类定义：key 与后端 operationCategory() 返回值保持一致
export const OPERATION_CATEGORIES = [
  { key: 'order', label: '订单' },
  { key: 'product', label: '商品库存' },
  { key: 'inbound', label: '采购入库' },
  { key: 'stocktake', label: '盘点' },
  { key: 'finance', label: '财务资金' },
  { key: 'employee', label: '员工' },
  { key: 'engineering', label: '工程' },
  { key: 'system', label: '系统' },
  { key: 'other', label: '其他' }
]

const CATEGORY_LABELS = Object.fromEntries(OPERATION_CATEGORIES.map((item) => [item.key, item.label]))

export function operationCategoryLabel(key) {
  return CATEGORY_LABELS[key] || '其他'
}
