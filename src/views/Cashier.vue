<template>
  <!-- 为底部导航栏和购物条留出双层空间 (导航栏 4rem + 购物条 4rem) -->
  <div class="relative flex flex-col h-full bg-slate-50 pb-32 md:pb-16">
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
          placeholder="搜索商品名称、型号或 SKU..."
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
    <div class="flex-1 overflow-y-auto p-4">
      <div v-if="productList.length === 0" class="py-10 text-center">
        <el-empty description="暂无符合条件的商品" :image-size="100" />
      </div>
      
      <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3">
        <div 
          v-for="row in productList" 
          :key="row.id" 
          class="bg-white rounded-xl shadow-sm border border-slate-100 flex flex-col p-3 hover:border-blue-300 transition-all cursor-pointer group"
          @click="addToCart(row)"
        >
          <!-- Info Section -->
          <div class="flex-1 min-w-0">
            <div class="font-bold text-slate-800 text-sm line-clamp-2 leading-tight h-9 mb-1 group-hover:text-blue-600 transition-colors">
              {{ row.name }}
            </div>
            <div class="flex flex-wrap gap-1 mb-2">
              <el-tag :type="row.store_stock > 0 ? 'success' : 'danger'" size="small" class="!px-1.5 !h-5 !text-[10px]">
                店:{{ row.store_stock }}
              </el-tag>
              <el-tag :type="row.main_stock > 0 ? 'primary' : 'info'" size="small" class="!px-1.5 !h-5 !text-[10px]">
                仓:{{ row.main_stock }}
              </el-tag>
            </div>
          </div>

          <!-- Price & Action Section -->
          <div class="flex justify-between items-end mt-auto pt-2 border-t border-slate-50">
            <div class="flex flex-col">
              <span class="text-[10px] text-slate-400 line-through">¥{{ (calculateMinPrice(row) * 1.2).toFixed(0) }}</span>
              <span class="font-bold text-red-600 text-base leading-none">
                <span class="text-xs">¥</span>{{ calculateMinPrice(row).toFixed(0) }}
              </span>
            </div>
            <div class="bg-blue-600 text-white p-1.5 rounded-lg shadow-sm shadow-blue-200 group-hover:scale-110 transition-transform">
              <el-icon :size="16"><Plus /></el-icon>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 底部吸底结算条 (Lifted for mobile nav) -->
    <div class="fixed bottom-[calc(4rem+env(safe-area-inset-bottom,0px))] md:bottom-0 left-0 w-full bg-white shadow-[0_-4px_6px_-1px_rgba(0,0,0,0.1)] p-3 z-40 flex justify-between items-center">
      <div class="flex items-center gap-3 cursor-pointer" @click="openDrawer">
        <el-badge :value="cart.length" :hidden="cart.length === 0" class="flex" type="danger">
          <div class="bg-blue-100 p-2 rounded-full text-blue-600">
            <el-icon size="24"><ShoppingCart /></el-icon>
          </div>
        </el-badge>
        <div>
          <div class="text-xs text-slate-500">合计</div>
          <div class="font-bold text-lg text-slate-800 leading-none">¥{{ originalTotal.toFixed(2) }}</div>
        </div>
      </div>
      <el-button type="primary" size="large" round class="px-6 shadow-md shadow-blue-200 font-bold" @click="openDrawer" :disabled="cart.length === 0">
        去结算
      </el-button>
    </div>

    <!-- 结算抽屉 -->
    <el-drawer
      v-model="drawerVisible"
      direction="rtl"
      :size="isMobile ? '100%' : '800px'"
      class="pos-drawer"
      :with-header="true"
      title="购物车与结算"
    >
      <div class="h-full bg-slate-50 overflow-hidden flex flex-col">
        <!-- Mobile Tabs -->
        <div v-if="isMobile" class="flex-1 flex flex-col overflow-hidden">
          <el-tabs v-model="drawerActiveTab" class="mobile-pos-tabs h-full flex flex-col">
            <el-tab-pane label="购物车" name="cart" class="h-full overflow-y-auto">
              <div class="p-4 space-y-3">
                <div v-if="cart.length === 0" class="flex flex-col items-center justify-center text-slate-400 py-10">
                  <el-icon size="48" class="mb-2 opacity-20"><Files /></el-icon>
                  <p class="text-sm">购物车是空的</p>
                </div>
                
                <div v-for="(item, index) in cart" :key="index" class="p-3 rounded-xl border border-slate-100 bg-white shadow-sm relative">
                  <div class="flex justify-between items-start mb-2">
                    <div class="font-bold text-slate-800 text-sm truncate pr-2">{{ item.name }}</div>
                    <el-button type="danger" link :icon="Delete" class="!p-0 absolute right-3 top-3" @click="removeFromCart(index)" />
                  </div>
                  
                  <div class="flex justify-between items-center mt-2">
                    <div class="text-xs text-slate-600 font-medium">作为赠品</div>
                    <el-switch v-model="item.isGift" size="small" />
                  </div>
                  
                  <div class="flex justify-between items-center mt-2" :class="{'opacity-40 pointer-events-none': item.isGift}">
                    <div class="text-xs text-slate-600">单价 (底价:¥{{ item.minPrice.toFixed(2) }})</div>
                    <el-input-number v-if="!item.isGift" v-model="item.price" :min="0.01" :precision="2" :controls="false" size="small" class="!w-28 pos-input-number" :class="{ 'price-warning': item.price < item.minPrice }" />
                    <span v-else class="text-red-500 font-bold px-3 py-0.5 bg-red-50 rounded text-xs">¥0.00</span>
                  </div>
                  
                  <div class="flex justify-between items-center mt-2">
                    <div class="text-xs text-slate-600">数量 (店:{{ item.store_stock }})</div>
                    <el-input-number v-model="item.quantity" :min="1" size="small" class="!w-28 pos-input-number" />
                  </div>
                </div>
              </div>
            </el-tab-pane>
            
            <el-tab-pane label="结算信息" name="checkout" class="h-full overflow-y-auto">
              <div class="p-4 space-y-4">
                <!-- 客户信息录入区 -->
                <div class="bg-blue-50/50 p-4 rounded-xl border border-blue-100/50 space-y-3">
                  <div class="flex items-center gap-2 mb-1">
                    <el-icon class="text-blue-600"><User /></el-icon>
                    <span class="text-sm font-bold text-slate-700">客户信息录入</span>
                  </div>
                  <div class="grid grid-cols-2 gap-3">
                    <el-input v-model="customerName" placeholder="姓名" class="pos-customer-input" />
                    <el-input v-model="customerPhone" placeholder="手机号" class="pos-customer-input" />
                  </div>
                  <div class="space-y-3 pt-2 border-t border-blue-100/50">
                    <el-select v-model="employeeId" placeholder="选择销售员工 (可选)" class="w-full" clearable>
                      <el-option v-for="emp in employees" :key="emp.id" :label="emp.name" :value="emp.id" />
                    </el-select>
                    <div class="grid grid-cols-2 gap-3">
                      <el-input v-model="referrerName" placeholder="推荐人" class="pos-customer-input" />
                      <el-input-number v-model="referralFee" :min="0" :precision="2" :controls="false" class="w-full pos-customer-input" placeholder="奖励" />
                    </div>
                  </div>
                </div>

                <!-- 模式切换 -->
                <div class="grid grid-cols-2 gap-2">
                  <div class="flex justify-between items-center p-3 rounded-xl bg-white border border-slate-100 shadow-sm">
                    <span class="text-xs font-bold text-slate-600">国家补贴</span>
                    <el-switch v-model="isSubsidized" size="small" />
                  </div>
                  <div class="flex justify-between items-center p-3 rounded-xl bg-white border border-slate-100 shadow-sm">
                    <span class="text-xs font-bold text-slate-600">支付定金</span>
                    <el-switch v-model="isDepositMode" size="small" />
                  </div>
                </div>

                <el-collapse-transition>
                  <div v-if="isDepositMode" class="bg-purple-50 p-3 rounded-xl border border-purple-100">
                    <el-input-number v-model="depositAmount" :min="0" :max="totalPayable" :precision="2" class="w-full" :controls="false" placeholder="输入定金金额" />
                  </div>
                </el-collapse-transition>

                <div class="bg-white p-3 rounded-xl border border-slate-100 shadow-sm space-y-4">
                  <div>
                    <div class="text-[10px] text-slate-400 mb-2 uppercase font-bold">发货方式</div>
                    <el-radio-group v-model="deliveryMethod" class="w-full flex" size="small">
                      <el-radio-button :value="1" class="flex-1">自提</el-radio-button>
                      <el-radio-button :value="2" class="flex-1">主仓</el-radio-button>
                      <el-radio-button :value="3" class="flex-1">云仓</el-radio-button>
                    </el-radio-group>
                  </div>
                  <div>
                    <div class="text-[10px] text-slate-400 mb-2 uppercase font-bold">收款账户</div>
                    <el-radio-group v-model="paymentMethod" class="w-full flex" size="small">
                      <el-radio-button :value="1" class="flex-1">支付宝</el-radio-button>
                      <el-radio-button :value="2" class="flex-1">微信</el-radio-button>
                      <el-radio-button :value="3" class="flex-1">公户</el-radio-button>
                    </el-radio-group>
                  </div>
                </div>

                <!-- 汇总 -->
                <div class="bg-slate-900 p-4 rounded-2xl text-white shadow-xl">
                  <div class="flex justify-between text-xs opacity-60 mb-1">
                    <span>商品合计</span>
                    <span>¥{{ originalTotal.toFixed(2) }}</span>
                  </div>
                  <div v-if="isSubsidized" class="flex justify-between text-xs text-orange-300 mb-1">
                    <span>国补减免</span>
                    <span>-¥{{ subsidyAmount.toFixed(2) }}</span>
                  </div>
                  <div class="flex justify-between items-end mt-2">
                    <span class="text-sm font-bold">{{ isDepositMode ? '预收定金' : '应付全款' }}</span>
                    <span class="text-2xl font-black">¥{{ actualReceipt.toFixed(2) }}</span>
                  </div>
                </div>

                <el-button type="primary" size="large" class="w-full !h-14 !rounded-2xl !border-none shadow-xl shadow-blue-500/20" @click="handleCheckout" :loading="loading" :disabled="cart.length === 0 || hasPriceViolation || hasStockViolation">
                  确认开单提交
                </el-button>
              </div>
            </el-tab-pane>
          </el-tabs>
        </div>

        <!-- Desktop Side-by-Side -->
        <div v-else class="flex flex-1 overflow-hidden">
          <!-- 左侧：购物车商品列表 -->
          <div class="flex-1 overflow-y-auto p-4 space-y-3">
            <div v-if="cart.length === 0" class="h-full flex flex-col items-center justify-center text-slate-400 py-10">
              <el-icon size="48" class="mb-2 opacity-20"><Files /></el-icon>
              <p class="text-sm">购物车是空的</p>
            </div>
            
            <div v-for="(item, index) in cart" :key="index" class="p-3 rounded-lg border border-slate-100 bg-white shadow-sm relative">
              <div class="flex justify-between items-start mb-2">
                <div class="font-bold text-slate-800 text-sm truncate pr-2">{{ item.name }}</div>
                <el-button type="danger" link :icon="Delete" class="!p-0 absolute right-3 top-3" @click="removeFromCart(index)" />
              </div>
              
              <div class="flex justify-between items-center mt-2">
                <div class="text-xs text-slate-600 font-medium">作为赠品 (不再计入总价)</div>
                <el-switch v-model="item.isGift" size="small" />
              </div>
              
              <div class="flex justify-between items-center mt-2" :class="{'opacity-40 pointer-events-none': item.isGift}">
                <div class="text-xs text-slate-600">单价 (底价: ¥{{ item.minPrice.toFixed(2) }})</div>
                <div class="flex items-center">
                  <el-input-number v-if="!item.isGift" v-model="item.price" :min="0.01" :precision="2" :controls="false" size="small" class="!w-32 pos-input-number" :class="{ 'price-warning': item.price < item.minPrice && !item.isGift }" />
                  <span v-else class="text-red-500 font-bold px-4 py-1 bg-red-50 rounded">¥0.00 (赠品)</span>
                </div>
              </div>
              
              <div class="flex justify-between items-center mt-2">
                <div class="text-xs text-slate-600">数量 (店:{{ item.store_stock }} | 仓:{{ item.main_stock }})</div>
                <el-input-number v-model="item.quantity" :min="1" size="small" class="!w-32 pos-input-number" />
              </div>
            </div>
          </div>

          <!-- 右侧：结算操作区 -->
          <div class="w-[400px] shrink-0 h-full overflow-y-auto p-6 bg-white border-l border-slate-200">
            <div class="space-y-4">
              <!-- 客户信息录入区 -->
              <div class="bg-blue-50/50 p-4 rounded-xl border border-blue-100/50 space-y-3">
                <div class="flex items-center gap-2 mb-1">
                  <el-icon class="text-blue-600"><User /></el-icon>
                  <span class="text-sm font-bold text-slate-700">客户信息录入</span>
                </div>
                <div class="grid grid-cols-2 gap-3 mb-2">
                  <div class="space-y-1">
                    <span class="text-[10px] text-slate-400 font-medium pl-1">客户姓名</span>
                    <el-input v-model="customerName" placeholder="请输入姓名" class="pos-customer-input" />
                  </div>
                  <div class="space-y-1">
                    <span class="text-[10px] text-slate-400 font-medium pl-1">手机号码</span>
                    <el-input v-model="customerPhone" placeholder="请输入手机号" class="pos-customer-input" />
                  </div>
                </div>

                <div class="space-y-3 pt-2 border-t border-blue-100/50">
                  <div class="space-y-1">
                    <span class="text-[10px] text-slate-400 font-medium pl-1">开单员工</span>
                    <el-select v-model="employeeId" placeholder="选择销售员工 (可选)" class="w-full" clearable>
                      <el-option v-for="emp in employees" :key="emp.id" :label="emp.name" :value="emp.id" />
                    </el-select>
                  </div>
                  <div class="grid grid-cols-2 gap-3">
                    <div class="space-y-1">
                      <span class="text-[10px] text-slate-400 font-medium pl-1">推荐人姓名</span>
                      <el-input v-model="referrerName" placeholder="选填" class="pos-customer-input" />
                    </div>
                    <div class="space-y-1">
                      <span class="text-[10px] text-slate-400 font-medium pl-1">推荐人奖励(元)</span>
                      <el-input-number v-model="referralFee" :min="0" :precision="2" :controls="false" class="w-full pos-customer-input" placeholder="奖励金额" />
                    </div>
                  </div>
                </div>
              </div>

              <div class="flex flex-col gap-2">
                <div class="flex justify-between items-center p-2 rounded-lg bg-slate-50 border border-slate-100">
                  <span class="text-sm text-slate-600 flex items-center gap-2"><el-icon color="#3b82f6"><Promotion /></el-icon>参与国补政策</span>
                  <el-switch v-model="isSubsidized" size="small" />
                </div>
                <div class="flex justify-between items-center p-2 rounded-lg bg-slate-50 border border-slate-100">
                  <span class="text-sm text-slate-600 flex items-center gap-2"><el-icon color="#8b5cf6"><Wallet /></el-icon>支付定金模式</span>
                  <el-switch v-model="isDepositMode" size="small" />
                </div>
              </div>

              <el-collapse-transition>
                <div v-if="isDepositMode" class="bg-purple-50 p-2 rounded-lg border border-purple-100">
                  <el-input-number v-model="depositAmount" :min="0" :max="totalPayable" :precision="2" class="!flex-1" :controls="false" placeholder="输入定金金额" />
                </div>
              </el-collapse-transition>

              <div class="bg-slate-50 p-3 rounded-lg border border-slate-100 mb-2">
                <div class="text-xs text-slate-600 mb-2 font-medium">发货/提货方式</div>
                <el-radio-group v-model="deliveryMethod" class="w-full flex mb-4" size="small">
                  <el-radio-button :value="1" class="flex-1">门店自提</el-radio-button>
                  <el-radio-button :value="2" class="flex-1">主仓发货</el-radio-button>
                  <el-radio-button :value="3" class="flex-1">云仓代发</el-radio-button>
                </el-radio-group>
                
                <div class="text-xs text-slate-600 mb-2 font-medium">收款账户</div>
                <el-radio-group v-model="paymentMethod" class="w-full flex" size="small">
                  <el-radio-button :value="1" class="flex-1">支付宝</el-radio-button>
                  <el-radio-button :value="2" class="flex-1">微信</el-radio-button>
                  <el-radio-button :value="3" class="flex-1">公户</el-radio-button>
                </el-radio-group>
              </div>

              <div class="bg-slate-50 p-3 rounded-lg border border-slate-100 space-y-1">
                <div class="flex justify-between text-xs text-slate-500"><span>商品合计</span><span>¥{{ originalTotal.toFixed(2) }}</span></div>
                <div v-if="isSubsidized" class="flex justify-between text-xs text-orange-600"><span>国家补贴减免</span><span>-¥{{ subsidyAmount.toFixed(2) }}</span></div>
                <div class="flex justify-between text-sm text-slate-800 font-bold pt-1 border-t border-slate-200 mt-1">
                  <span>{{ isDepositMode ? '本次预收定金' : '本次应付全款' }}</span>
                  <span class="text-blue-600 text-xl font-mono">¥{{ actualReceipt.toFixed(2) }}</span>
                </div>
              </div>

              <el-button type="primary" size="large" class="w-full !h-12 !text-base !rounded-xl !border-none shadow-md shadow-blue-200 pos-submit-btn" :disabled="cart.length === 0 || hasPriceViolation || hasStockViolation" :loading="loading" @click="handleCheckout">
                <span class="font-bold tracking-widest text-[16px]">确认开单</span>
              </el-button>
            </div>
          </div>
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Search, ShoppingCart, Delete, Promotion, Wallet, User, Warning, WarningFilled, Files, Plus, Fold } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'

const productList = ref([])
const categories = ref([])
const selectedCategoryId = ref('')
const employees = ref([])
const searchQuery = ref('')
const loading = ref(false)
const drawerVisible = ref(false)
const drawerActiveTab = ref('cart')
const isMobile = ref(window.innerWidth < 768)

window.addEventListener('resize', () => {
  isMobile.value = window.innerWidth < 768
})

const fetchCategories = async () => {
  try {
    const res = await axios.get('/api/categories')
    if (res.data.code === 200) {
      categories.value = res.data.data
    }
  } catch (error) {
    console.error('Fetch categories error:', error)
  }
}

const selectCategory = (id) => {
  selectedCategoryId.value = id
  fetchProducts()
}

// 获取商品数据
const fetchProducts = async () => {
  try {
    const response = await axios.get('/api/products', {
      params: { 
        keyword: searchQuery.value,
        category_id: selectedCategoryId.value
      }
    })
    if (response.data.code === 200) {
      productList.value = response.data.data
    } else {
      ElMessage.error(response.data.msg || '获取商品失败')
    }
  } catch (error) {
    console.error('Fetch products error:', error)
    ElMessage.error('无法连接到后端服务')
  }
}

const fetchEmployees = async () => {
  try {
    const response = await axios.get('/api/employees')
    if (response.data.code === 200) {
      employees.value = response.data.data
    }
  } catch (error) {
    console.error('Fetch employees error:', error)
  }
}

onMounted(() => {
  fetchProducts()
  fetchEmployees()
  fetchCategories()
})

const calculateMinPrice = (product) => {
  return (product.latest_cost || 0) / (1 - (product.margin_rate || 0))
}

// 购物车状态
const cart = ref([])

const addToCart = (product) => {
  const minPrice = calculateMinPrice(product)
  const existing = cart.value.find(item => item.id === product.id)
  if (existing) {
    existing.quantity++
    ElMessage.success(`已增加 ${product.name} 的数量`)
  } else {
    cart.value.push({
      id: product.id,
      name: product.name,
      store_stock: product.store_stock || 0,
      main_stock: product.main_stock || 0,
      support_cloud: product.support_cloud,
      minPrice: minPrice,
      price: Math.ceil(minPrice),
      quantity: 1,
      isGift: false
    })
    ElMessage.success(`已添加 ${product.name} 到购物车`)
  }
}

const openDrawer = () => {
  drawerActiveTab.value = 'cart'
  drawerVisible.value = true
}

const removeFromCart = (index) => {
  cart.value.splice(index, 1)
}

const clearCart = () => {
  cart.value = []
}

// 结算状态
const isSubsidized = ref(false)
const isDepositMode = ref(false)
const depositAmount = ref(0)
const customerName = ref('')
const customerPhone = ref('')
const employeeId = ref(null)
const referrerName = ref('')
const referralFee = ref(0)
const paymentMethod = ref(1)
const deliveryMethod = ref(1) // 1-门店自提, 2-主仓发货, 3-云仓代发

const originalTotal = computed(() => {
  return cart.value.reduce((sum, item) => sum + (item.isGift ? 0 : item.price * item.quantity), 0)
})

const subsidyAmount = computed(() => {
  if (!isSubsidized.value) return 0
  const total = originalTotal.value
  if (total <= 10000) {
    return total * 0.15
  } else {
    return 1500
  }
})

const totalPayable = computed(() => {
  return Math.max(0, originalTotal.value - subsidyAmount.value)
})

const actualReceipt = computed(() => {
  return isDepositMode.value ? depositAmount.value : totalPayable.value
})

const hasPriceViolation = computed(() => {
  return cart.value.some(item => !item.isGift && item.price < item.minPrice)
})

const getStockWarning = (item) => {
  if (deliveryMethod.value === 1 && item.quantity > item.store_stock) return `门店库存不足 (仅剩 ${item.store_stock})`
  if (deliveryMethod.value === 2 && item.quantity > item.main_stock) return `主仓库存不足 (仅剩 ${item.main_stock})`
  if (deliveryMethod.value === 3 && !item.support_cloud) return `该商品不支持云仓代发`
  return ''
}

const hasStockViolation = computed(() => {
  return cart.value.some(item => getStockWarning(item) !== '')
})

// 开单请求
const handleCheckout = async () => {
  if (cart.value.length === 0 || hasPriceViolation.value) return
  
  const type = isDepositMode.value ? '定金' : '全款'
  const amount = actualReceipt.value.toFixed(2)
  
  try {
    await ElMessageBox.confirm(
      `确认收取客户 ${type} ¥${amount} 并生成订单吗？`,
      '结算确认',
      {
        confirmButtonText: '确认',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )
    
    loading.value = true
    
    const payload = {
      items: cart.value.map(item => ({
        product_id: item.id,
        quantity: item.quantity,
        unit_price: item.price,
        is_gift: item.isGift
      })),
      is_subsidy: isSubsidized.value,
      deposit_amount: isDepositMode.value ? depositAmount.value : 0,
      customer_name: customerName.value,
      customer_phone: customerPhone.value,
      employee_id: employeeId.value || undefined,
      referrer_name: referrerName.value,
      referral_fee: referralFee.value,
      payment_method: paymentMethod.value,
      delivery_method: deliveryMethod.value
    }
    
    const response = await axios.post('/api/orders', payload)
    
    if (response.data.code === 200) {
      ElMessage.success('开单成功！')
      clearCart()
      isSubsidized.value = false
      isDepositMode.value = false
      depositAmount.value = 0
      customerName.value = ''
      customerPhone.value = ''
      employeeId.value = null
      referrerName.value = ''
      referralFee.value = 0
      paymentMethod.value = 1
      deliveryMethod.value = 1
      drawerVisible.value = false
      fetchProducts()
    } else {
      ElMessage.error(response.data.msg || '开单失败')
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Order submission error:', error)
      const errorMsg = error.response?.data?.msg || '网络错误，请稍后重试'
      ElMessage.error(errorMsg)
    }
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
@media (max-width: 800px) {
  :deep(.pos-drawer) {
    width: 100% !important;
  }
}

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

:deep(.pos-input-number.el-input-number .el-input__wrapper) {
  padding: 0 8px;
  background: #f8fafc;
  border-radius: 8px;
}

.price-warning :deep(.el-input__wrapper) {
  box-shadow: 0 0 0 1px #ef4444 inset !important;
  background-color: #fef2f2 !important;
}

:deep(.pos-customer-input .el-input__wrapper) {
  background-color: #ffffff;
  border-radius: 8px;
  box-shadow: 0 1px 2px 0 rgba(0, 0, 0, 0.05) !important;
}

:deep(.pos-customer-input .el-input__inner) {
  font-size: 13px;
}

.pos-submit-btn {
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%) !important;
  transition: all 0.3s ease;
}

.pos-submit-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 8px 12px -3px rgba(37, 99, 235, 0.2);
}

.pos-submit-btn:active {
  transform: translateY(0);
}



.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;  
  overflow: hidden;
}

/* Mobile Tabs Styling */
:deep(.mobile-pos-tabs) {
  display: flex;
  flex-direction: column;
}
:deep(.mobile-pos-tabs .el-tabs__header) {
  margin: 0;
  background: white;
  padding: 0 10px;
  border-bottom: 1px solid #f1f5f9;
}
:deep(.mobile-pos-tabs .el-tabs__nav-wrap::after) {
  display: none;
}
:deep(.mobile-pos-tabs .el-tabs__item) {
  font-weight: 700;
  font-size: 14px;
  height: 50px;
  line-height: 50px;
}
:deep(.mobile-pos-tabs .el-tabs__content) {
  flex: 1;
}

/* 自定义滚动条 */
::-webkit-scrollbar {
  width: 4px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: #cbd5e1;
  border-radius: 4px;
}
</style>
