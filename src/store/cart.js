import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

export const useCartStore = defineStore('cart', () => {
  const cart = ref([])

  // 从 localStorage 初始化购物车数据
  const savedCart = localStorage.getItem('erp_cart')
  if (savedCart) {
    try {
      cart.value = JSON.parse(savedCart)
    } catch (e) {
      console.error('Failed to parse erp_cart from localStorage', e)
      cart.value = []
    }
  }

  // 监听并实时同步至 localStorage 实现刷新保持
  watch(
    cart,
    (newVal) => {
      localStorage.setItem('erp_cart', JSON.stringify(newVal))
    },
    { deep: true }
  )

  const addToCart = (item) => {
    const existing = cart.value.find(i => i.id === item.id)
    if (existing) {
      existing.quantity += item.quantity || 1
    } else {
      cart.value.push({ ...item })
    }
  }

  const removeFromCart = (index) => {
    cart.value.splice(index, 1)
  }

  const clearCart = () => {
    cart.value = []
  }

  return {
    cart,
    addToCart,
    removeFromCart,
    clearCart
  }
})
