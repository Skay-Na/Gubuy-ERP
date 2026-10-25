<template>
  <div class="flex flex-col h-full bg-slate-50">
    <!-- 顶部固定区域：状态栏与搜索框 -->
    <div class="p-4 bg-white shadow-sm z-10 flex-shrink-0">
      <div class="flex justify-between items-center mb-3">
        <div class="flex items-center gap-2">
          <div class="w-1.5 h-5 bg-blue-600 rounded-full"></div>
          <h1 class="text-xl font-bold text-slate-800 tracking-tight">收银台</h1>
        </div>
        <div class="text-slate-400 text-xs font-mono tracking-wider">{{ new Date().toLocaleDateString() }}</div>
      </div>
      <div class="flex gap-2 mb-3">
        <el-input
          v-model="searchQuery"
          placeholder="搜索商品名称、型号或备注..."
          prefix-icon="Search"
          clearable
          size="large"
          class="pos-search-input flex-1"
          @input="fetchProducts"
        />
        <el-button 
          type="primary" 
          size="large" 
          class="search-btn shadow-md shadow-blue-200" 
          @click="fetchProducts"
        >
          搜索
        </el-button>
      </div>

      <!-- 分类横向导航 -->
      <div class="flex overflow-x-auto no-scrollbar gap-2 pb-1">
        <div 
          class="px-4 py-1.5 rounded-full text-xs font-bold whitespace-nowrap transition-all cursor-pointer"
          :class="selectedCategoryId === '' ? 'bg-blue-600 text-white shadow-md shadow-blue-100' : 'bg-slate-100 text-slate-500 hover:bg-slate-200'"
          @click="selectCategory('')"
        >
          全部
        </div>
        <div 
          v-for="cat in categories" 
          :key="cat.id"
          class="px-4 py-1.5 rounded-full text-xs font-bold whitespace-nowrap transition-all cursor-pointer"
          :class="selectedCategoryId === cat.id ? 'bg-blue-600 text-white shadow-md shadow-blue-100' : 'bg-slate-100 text-slate-500 hover:bg-slate-200'"
          @click="selectCategory(cat.id)"
        >
          {{ cat.name }}
        </div>
      </div>
    </div>

    <!-- 中间滚动区域：商品网格 -->
    <div class="flex-1 overflow-y-auto p-4 pb-24 md:pb-20">
      <!-- 加载中：骨架屏 -->
      <div v-if="loading" class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3">
        <el-skeleton v-for="i in 10" :key="i" animated>
          <template #template>
            <div class="bg-white rounded-xl p-3 border border-slate-100 flex flex-col gap-3">
              <el-skeleton-item variant="rect" class="h-9 w-full rounded" />
              <div class="flex gap-1">
                <el-skeleton-item variant="text" class="w-10 h-5" />
                <el-skeleton-item variant="text" class="w-10 h-5" />
              </div>
              <div class="mt-auto pt-2 border-t border-slate-50 flex justify-between items-end">
                <div class="flex flex-col gap-1">
                  <el-skeleton-item variant="text" class="w-16 h-6" />
                  <el-skeleton-item variant="text" class="w-10 h-3" />
                </div>
                <el-skeleton-item variant="rect" class="w-8 h-8 rounded-lg" />
              </div>
            </div>
          </template>
        </el-skeleton>
      </div>

      <div v-else-if="productList.length === 0" class="py-10 text-center">
        <el-empty description="暂无符合条件的商品" :image-size="100" />
      </div>
      
      <div v-else class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3">
        <div 
          v-for="row in productList" 
          :key="row.id" 
          v-memo="[row.id, row.store_stock, row.cloud_stock, row.avg_cost, row.latest_cost, row.special_price, isSpecialActive(row)]"
          class="bg-white rounded-xl shadow-sm border border-slate-100 flex flex-col p-3 hover:border-blue-300 transition-all cursor-pointer group relative overflow-hidden"
          :class="isSpecialActive(row) ? 'special-offer-card border-red-200' : ''"
          @click="handleAddToCart(row)"
        >
          <!-- 特价角标 -->
          <div v-if="isSpecialActive(row)" class="absolute top-0 right-0 z-10">
            <div class="bg-red-500 text-white text-[10px] font-black px-2 py-0.5 rounded-bl-lg shadow-sm flex items-center gap-0.5 animate-pulse">
              <el-icon><Lightning /></el-icon>SALE
            </div>
          </div>

          <!-- Info Section -->
          <div class="flex-1 min-w-0">
            <div class="font-bold text-slate-800 text-sm line-clamp-2 leading-tight h-9 mb-1 group-hover:text-blue-600 transition-colors">
              {{ row.name }}
            </div>
            <div class="flex flex-wrap gap-1 mb-2">
            <el-tag v-if="getProductCost(row) <= 0" type="danger" size="small" class="!px-1.5 !h-5 !text-[10px]">成本待维护</el-tag>
              <el-tag :type="row.store_stock > 0 ? 'success' : 'danger'" size="small" class="!px-1.5 !h-5 !text-[10px]">
                店:{{ row.store_stock }}
              </el-tag>
              <el-tag type="info" size="small" class="!px-1.5 !h-5 !text-[10px]">
                云仓不限量
              </el-tag>
            </div>
          </div>

          <!-- Price & Action Section -->
          <div class="flex justify-between items-end mt-auto pt-2 border-t border-slate-50">
            <div class="flex flex-col">
              <div v-if="isSpecialActive(row)" class="flex flex-col">
                <span class="text-[10px] text-slate-400 line-through">¥{{ ((row.avg_cost || row.latest_cost || 0) / (1 - (row.margin_rate || 0))).toFixed(0) }}</span>
                <div class="flex items-center gap-1">
                  <span class="font-black text-red-600 text-lg leading-none">
                    <span class="text-xs">¥</span>{{ row.special_price.toFixed(0) }}
                  </span>
                  <span class="bg-red-50 text-red-500 text-[9px] px-1 rounded font-bold border border-red-100">
                    立省 ¥{{ (((row.avg_cost || row.latest_cost || 0) / (1 - (row.margin_rate || 0))) - row.special_price).toFixed(0) }}
                  </span>
                </div>
              </div>
              <div v-else class="flex flex-col">
                <span v-if="getProductCost(row) > 0" class="font-bold text-slate-800 text-base leading-none">
                  <span class="text-xs">¥</span>{{ calculateMinPrice(row).toFixed(0) }}
                </span>
                <span v-else class="text-xs text-amber-600 font-bold">先维护商品成本</span>
                <span v-if="getProductCost(row) > 0" class="text-[9px] text-slate-400 mt-0.5">成本参考: ¥{{ getProductCost(row).toFixed(1) }}</span>
              </div>
            </div>
            <div class="bg-blue-600 text-white p-1.5 rounded-lg shadow-sm shadow-blue-200 group-hover:scale-110 transition-transform">
              <el-icon :size="16"><Plus /></el-icon>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Plus, Lightning } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { fetchCategories as fetchCategoriesApi, fetchProducts as fetchProductsApi } from '../../api/products'
import { useCartStore } from '../../store/cart'

const cartStore = useCartStore()

const productList = ref([])
const categories = ref([])
const selectedCategoryId = ref('')
const searchQuery = ref('')
const loading = ref(false)

const fetchCategories = async () => {
  try {
    const res = await fetchCategoriesApi()
    if (res.code === 200) {
      categories.value = res.result
    }
  } catch (error) {
    console.error('Fetch categories error:', error)
  }
}

const selectCategory = (id) => {
  selectedCategoryId.value = id
  fetchProducts()
}

const fetchProducts = async () => {
  loading.value = true
  try {
    const response = await fetchProductsApi({
      keyword: searchQuery.value,
      category_id: selectedCategoryId.value
    })
    if (response.code === 200) {
      productList.value = response.result
    } else {
      ElMessage.error(response.msg || '获取商品失败')
    }
  } catch (error) {
    console.error('Fetch products error:', error)
  } finally {
    loading.value = false
  }
}

const isSpecialActive = (product) => {
  if (!product.special_price || product.special_price <= 0) return false
  if (!product.special_price_start || !product.special_price_end) return false
  const now = new Date()
  const start = new Date(product.special_price_start)
  const end = new Date(product.special_price_end)
  return now >= start && now <= end
}

const getProductCost = (product) => Number(product.avg_cost) > 0 ? Number(product.avg_cost) : Number(product.latest_cost) || 0

const calculateMinPrice = (product) => {
  if (isSpecialActive(product)) {
    return product.special_price
  }
  const cost = getProductCost(product)
  const rate = Number(product.margin_rate) || 0
  return cost / (1 - rate)
}

const handleAddToCart = (product) => {
  if (getProductCost(product) <= 0) {
    ElMessage.warning('该商品尚未建立有效成本，请先维护商品成本后再开单')
    return
  }
  const minPrice = calculateMinPrice(product)
  const item = {
    id: product.id,
    name: product.name,
    category_name: product.category?.name || product.Category?.name || '',
    store_stock: product.store_stock || 0,
    cloud_stock: product.cloud_stock || 0,
    minPrice: minPrice,
    price: Math.ceil(minPrice),
    quantity: 1,
    isGift: false
  }
  cartStore.addToCart(item)
  ElMessage.success(`已添加 ${product.name} 到购物车`)
}

onMounted(() => {
  fetchProducts()
  fetchCategories()
})

defineExpose({
  fetchProducts
})
</script>

<style scoped>
:deep(.pos-search-input .el-input__wrapper) {
  box-shadow: 0 1px 3px 0 rgba(0, 0, 0, 0.05) !important;
  background: #f8fafc;
  border-radius: 12px;
  padding-left: 12px;
  border: 1px solid #e2e8f0;
  transition: all 0.3s ease;
}

:deep(.pos-search-input .el-input__wrapper.is-focus) {
  background: #ffffff;
  border-color: #3b82f6;
  box-shadow: 0 0 0 4px rgba(59, 130, 246, 0.1) !important;
}

.search-btn {
  border-radius: 12px;
  padding: 0 24px;
  font-weight: 600;
  border: none;
  background: linear-gradient(135deg, #3b82f6 0%, #2563eb 100%);
}

.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;  
  overflow: hidden;
}

.special-offer-card {
  border-color: #fee2e2 !important;
  background: linear-gradient(135deg, #ffffff 0%, #fffcfc 100%) !important;
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.08) !important;
}

.special-offer-card:hover {
  border-color: #ef4444 !important;
}

.special-offer-card .text-slate-800 {
  color: #991b1b !important;
}
</style>
