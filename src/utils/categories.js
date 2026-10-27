// 两级分类工具：平铺列表 <-> 树 / 显示路径 / 级联配置
// 后端 GetCategories 返回平铺列表，子分类紧跟其父分类（排序已由后端保证）。

// 组树：一级分类带 children
export const buildCategoryTree = (list = []) => {
  const roots = list.filter(c => !c.parent_id)
  const childrenMap = {}
  list.filter(c => c.parent_id).forEach(c => {
    (childrenMap[c.parent_id] = childrenMap[c.parent_id] || []).push(c)
  })
  return roots.map(root => ({ ...root, children: childrenMap[root.id] || [] }))
}

// 完整显示路径：大分类 / 二级分类
export const categoryFullPath = (flatList = [], categoryId) => {
  const cat = flatList.find(c => c.id === categoryId)
  if (!cat) return '未分类'
  if (!cat.parent_id) return cat.name
  const parent = flatList.find(c => c.id === cat.parent_id)
  return parent ? `${parent.name} / ${cat.name}` : cat.name
}

// el-cascader 配置：允许选中任意层级，值直接是分类 id
export const categoryCascaderProps = {
  checkStrictly: true,
  value: 'id',
  label: 'name',
  children: 'children',
  emitPath: false
}
