<template>
  <div class="relative flex flex-col h-full bg-slate-50 pb-20 md:pb-16">
    <!-- 主体：商品网格展示 -->
    <product-grid ref="productGridRef" class="flex-1" />

    <!-- 底部吸底结算条 -->
    <div class="fixed bottom-0 left-0 w-full bg-white shadow-[0_-4px_15px_rgba(0,0,0,0.05)] px-4 pt-2 pb-[env(safe-area-inset-bottom,8px)] z-40 flex justify-between items-center transition-all duration-300">
      <div class="flex items-center gap-3 cursor-pointer" @click="openDrawer">
        <el-badge :value="cartStore.cart.length" :hidden="cartStore.cart.length === 0" class="flex" type="danger">
          <div class="bg-indigo-100 p-2.5 rounded-full text-indigo-600">
            <el-icon size="26"><ShoppingCart /></el-icon>
          </div>
        </el-badge>
        <div>
          <div class="text-xs text-slate-500">合计金额</div>
          <div class="font-black text-xl text-slate-800 leading-none">¥{{ originalTotal.toFixed(2) }}</div>
        </div>
      </div>
      <el-button type="primary" size="large" round class="px-10 !h-12 shadow-lg shadow-indigo-200 font-bold" @click="openDrawer" :disabled="cartStore.cart.length === 0">
        去结算
      </el-button>
    </div>

    <!-- 结算抽屉 -->
    <el-drawer
      v-model="drawerVisible"
      direction="rtl"
      :size="isMobile ? '100%' : '800px'"
      class="pos-drawer"
      append-to-body
      :show-close="false"
    >
      <template #header>
        <div class="flex items-center justify-between px-4 py-3 border-b border-slate-100 bg-white">
          <div class="flex items-center gap-2 cursor-pointer p-1 -ml-1" @click="drawerVisible = false">
            <el-icon size="20" class="text-slate-900"><Back /></el-icon>
            <span class="text-base font-black text-slate-900 tracking-tight">购物车与结算</span>
          </div>
          <div class="p-2 -mr-2 cursor-pointer flex items-center justify-center" @click="drawerVisible = false">
            <el-icon size="22" class="text-slate-400 hover:text-slate-600"><Close /></el-icon>
          </div>
        </div>
      </template>

      <!-- 结算核心面板 -->
      <div class="h-full overflow-hidden">
        <checkout-panel 
          :is-mobile="isMobile"
          @checkout-success="handleCheckoutSuccess"
        />
      </div>
    </el-drawer>

    <receipt-preview v-model="receiptVisible" :receipt="latestReceipt" />
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { ShoppingCart, Back, Close } from '@element-plus/icons-vue'
import ProductGrid from './cashier/ProductGrid.vue'
import CheckoutPanel from './cashier/CheckoutPanel.vue'
import ReceiptPreview from '../components/ReceiptPreview.vue'
import { useCartStore } from '../store/cart'

const cartStore = useCartStore()
const productGridRef = ref(null)

const drawerVisible = ref(false)
const receiptVisible = ref(false)
const latestReceipt = ref(null)
const isMobile = ref(window.innerWidth < 768)

const handleResize = () => {
  isMobile.value = window.innerWidth < 768
}

onMounted(() => {
  window.addEventListener('resize', handleResize)
  window.addEventListener('popstate', handlePopState)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  window.removeEventListener('popstate', handlePopState)
})

const originalTotal = computed(() => {
  return cartStore.cart.reduce((sum, item) => sum + (item.isGift ? 0 : item.price * item.quantity), 0)
})

const openDrawer = () => {
  drawerVisible.value = true
}

const handleCheckoutSuccess = (receipt) => {
  drawerVisible.value = false
  if (receipt) {
    latestReceipt.value = receipt
    receiptVisible.value = true
  }
  // 刷新商品列表以同步最新库存
  if (productGridRef.value) {
    productGridRef.value.fetchProducts()
  }
}

// 适配返回手势
let isInternalBack = false
const handlePopState = () => {
  if (drawerVisible.value) {
    isInternalBack = true
    drawerVisible.value = false
  }
}

watch(drawerVisible, (newVal) => {
  if (newVal) {
    window.history.pushState({ drawer: 'checkout' }, '')
  } else {
    if (!isInternalBack && window.history.state?.drawer === 'checkout') {
      window.history.back()
    }
    isInternalBack = false
  }
})
</script>

<style>
@media (max-width: 800px) {
  .pos-drawer {
    width: 100% !important;
  }
}

.pos-drawer .el-drawer__header {
  margin-bottom: 0 !important;
  padding: 0 !important;
}
.pos-drawer .el-drawer__body {
  padding: 0 !important;
}
</style>

<style scoped>
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
