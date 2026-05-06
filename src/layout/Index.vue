<template>
  <div class="h-[100dvh] w-full bg-slate-50 flex flex-col md:flex-row overflow-hidden font-sans text-slate-800 tracking-tight">
    
    <!-- Mobile Header -->
    <header class="md:hidden flex items-center justify-between px-5 py-3 bg-white/80 backdrop-blur-xl border-b border-slate-200/50 flex-shrink-0 z-50">
      <div class="flex items-center gap-3">
        <div class="w-8 h-8 rounded-xl bg-gradient-to-br from-slate-900 to-slate-700 shadow-md flex items-center justify-center text-white font-bold text-base">
          E
        </div>
        <span class="font-bold text-lg tracking-tight text-slate-900">ERP Pro</span>
      </div>
      <!-- Hamburger Menu -->
      <button @click="mobileMenuOpen = !mobileMenuOpen" class="p-2 text-slate-500 hover:text-slate-900 transition-colors rounded-full hover:bg-slate-100 flex-shrink-0">
        <div class="w-6 h-6 flex-shrink-0 flex items-center justify-center">
          <svg class="w-6 h-6" style="width: 24px; height: 24px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>
          </svg>
        </div>
      </button>
    </header>

    <!-- Mobile Sidebar Drawer Background -->
    <transition name="el-fade-in">
      <div v-if="mobileMenuOpen" class="md:hidden fixed inset-0 z-[60] bg-slate-900/40 backdrop-blur-sm" @click="mobileMenuOpen = false"></div>
    </transition>

    <aside 
      :class="[
        mobileMenuOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0',
        isCollapsed ? 'md:w-20' : 'md:w-56'
      ]" 
      class="fixed md:relative top-0 left-0 w-72 md:w-auto h-full bg-white md:bg-white/80 backdrop-blur-2xl border-r border-slate-200/60 z-[70] md:z-50 transition-all duration-300 ease-[cubic-bezier(0.16,1,0.3,1)] flex flex-col shadow-2xl md:shadow-none"
    >
      <!-- PC/Mobile Logo Section -->
      <div class="px-6 py-8 flex items-center justify-between overflow-hidden">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-2xl bg-gradient-to-br from-slate-900 to-slate-700 shadow-lg shadow-slate-300 flex items-center justify-center text-white font-bold text-xl flex-shrink-0">
            E
          </div>
          <span v-if="!isCollapsed || mobileMenuOpen" class="font-bold text-xl tracking-tighter bg-clip-text text-transparent bg-gradient-to-r from-slate-900 to-slate-600 whitespace-nowrap animate-in fade-in slide-in-from-left-2 duration-300">
            ERP Pro
          </span>
        </div>
        <!-- Close Button for Mobile -->
        <button @click="mobileMenuOpen = false" class="md:hidden p-2 text-slate-400">
          <el-icon :size="24"><Close /></el-icon>
        </button>
      </div>

      <nav class="flex-1 px-4 space-y-1.5 overflow-y-auto">
        <div v-if="mobileMenuOpen" class="flex items-center gap-3 px-2 py-4 mb-4 border-b border-slate-100">
          <div class="w-12 h-12 rounded-full bg-gradient-to-tr from-cyan-400 to-blue-500 shadow-sm border-2 border-white flex-shrink-0"></div>
          <div class="overflow-hidden">
            <div class="font-bold text-slate-800 truncate">Admin User</div>
            <div class="text-xs text-slate-400 truncate">Store Manager</div>
          </div>
        </div>

        <router-link v-for="item in menuItems" :key="item.path" :to="item.path" custom v-slot="{ isActive, navigate }">
          <div @click="() => { navigate(); mobileMenuOpen = false }"
               class="flex items-center rounded-2xl cursor-pointer transition-all duration-200 group relative"
               :class="[
                 isActive ? 'bg-slate-900 text-white shadow-md shadow-slate-900/10' : 'text-slate-500 hover:bg-slate-100/80 hover:text-slate-900',
                 isCollapsed && !mobileMenuOpen ? 'justify-center w-12 h-12 mx-auto px-0' : 'px-4 py-3 gap-3'
               ]">
            <div class="flex-shrink-0 w-6 h-6 flex items-center justify-center">
              <component :is="item.icon" class="w-6 h-6 transition-transform duration-300 group-hover:scale-110" :class="isActive ? 'text-white' : 'text-slate-400 group-hover:text-slate-700'" style="width: 24px; height: 24px" />
            </div>
            <span v-if="!isCollapsed || mobileMenuOpen" class="font-medium text-sm whitespace-nowrap animate-in fade-in slide-in-from-left-2 duration-300">{{ item.title }}</span>
          </div>
        </router-link>
      </nav>

      <!-- Collapse Toggle & Profile (PC Only) -->
      <div class="hidden md:block p-3 mt-auto space-y-2">
        <button 
          @click="isCollapsed = !isCollapsed"
          class="w-full flex items-center justify-center p-2.5 rounded-2xl hover:bg-slate-100 transition-all text-slate-400 hover:text-slate-900 border border-transparent hover:border-slate-200/60"
        >
          <el-icon class="transition-transform duration-500" :class="isCollapsed ? 'rotate-180' : ''"><Fold /></el-icon>
        </button>

        <el-popover
          placement="top-start"
          :width="300"
          trigger="click"
          popper-style="border-radius: 16px; padding: 20px; box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.1);"
        >
          <template #reference>
            <div class="flex items-center rounded-2xl hover:bg-slate-100/80 transition-all cursor-pointer border border-transparent hover:border-slate-200/60 overflow-hidden"
                 :class="isCollapsed ? 'justify-center p-0 h-12 w-12 mx-auto' : 'p-2.5 gap-3'">
              <div class="w-8 h-8 rounded-full bg-gradient-to-tr from-cyan-400 to-blue-500 shadow-sm border-2 border-white flex-shrink-0"></div>
              <div v-if="!isCollapsed" class="flex flex-col overflow-hidden animate-in fade-in slide-in-from-left-2 duration-300">
                <span class="font-semibold text-xs truncate text-slate-800 font-sans tracking-tight">Admin User</span>
                <span class="text-[10px] text-slate-400 truncate">Store Manager</span>
              </div>
            </div>
          </template>
          
          <div class="space-y-4">
            <div class="flex items-center gap-3 mb-2">
              <div class="w-10 h-10 rounded-full bg-gradient-to-tr from-cyan-400 to-blue-500 flex-shrink-0"></div>
              <div>
                <div class="font-bold text-slate-800">管理员设置</div>
                <div class="text-xs text-slate-400">admin@wonchon.com</div>
              </div>
            </div>
            
            <div class="pt-4 border-t border-slate-100">
              <h4 class="text-xs font-bold text-slate-400 uppercase tracking-widest mb-3">修改登录密码</h4>
              <div class="space-y-3">
                <el-input v-model="pwdForm.old" type="password" placeholder="当前密码" show-password size="default" />
                <el-input v-model="pwdForm.new" type="password" placeholder="新密码" show-password size="default" />
                <el-input v-model="pwdForm.confirm" type="password" placeholder="确认新密码" show-password size="default" />
                <el-button type="primary" class="w-full !rounded-xl" :loading="pwdLoading" @click="handleUpdatePwd">
                  保存新密码
                </el-button>
              </div>
            </div>

            <div class="pt-2">
              <el-button type="danger" link class="w-full !justify-start" @click="handleLogout">
                <el-icon class="mr-2"><SwitchButton /></el-icon> 退出系统
              </el-button>
            </div>
          </div>
        </el-popover>
      </div>
    </aside>

    <!-- Main Content Area -->
    <main class="flex-1 h-full overflow-hidden flex flex-col relative pb-[calc(4rem+env(safe-area-inset-bottom,0px))] md:pb-0 z-0 bg-slate-50" style="transform: translateZ(0)">
      <div class="flex-1 overflow-y-auto p-4 md:p-8 scroll-smooth will-change-scroll">
        <div class="max-w-7xl mx-auto w-full">
          <router-view />
        </div>
      </div>
    </main>

    <!-- Mobile Bottom Tab Bar -->
    <nav class="md:hidden fixed bottom-0 left-0 w-full bg-white/90 backdrop-blur-2xl border-t border-slate-200/50 z-50 flex justify-around items-center px-2 pt-2 pb-[env(safe-area-inset-bottom,0.5rem)]">
      <router-link v-for="item in mobileMenuItems" :key="item.path" :to="item.path" custom v-slot="{ isActive, navigate }">
        <div @click="navigate" class="flex flex-col items-center justify-center w-16 h-12 rounded-xl transition-all duration-300"
             :class="isActive ? 'text-slate-900' : 'text-slate-400'">
          <div :class="['p-1.5 rounded-full transition-all duration-300 flex-shrink-0 flex items-center justify-center', isActive ? 'bg-slate-100 -translate-y-1' : '']">
            <div class="w-[22px] h-[22px] flex-shrink-0 flex items-center justify-center">
              <component :is="item.icon" class="w-full h-full" :class="isActive ? 'text-slate-900' : 'text-slate-400'" style="width: 22px; height: 22px" />
            </div>
          </div>
          <span class="text-[10px] font-semibold mt-0.5 tracking-tight" :class="isActive ? 'text-slate-900' : 'text-slate-500'">{{ item.title }}</span>
        </div>
      </router-link>
    </nav>

  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import {
  DataLine,
  ShoppingCart,
  Box,
  DocumentCopy,
  Memo,
  Fold,
  User,
  List,
  SwitchButton,
  Close,
  Setting
} from '@element-plus/icons-vue'
import { useRouter } from 'vue-router'
import axios from 'axios'
import { ElMessage } from 'element-plus'

const router = useRouter()
const mobileMenuOpen = ref(false)
const isCollapsed = ref(false)

const pwdLoading = ref(false)
const pwdForm = ref({ old: '', new: '', confirm: '' })

const handleUpdatePwd = async () => {
  if (!pwdForm.value.old || !pwdForm.value.new) {
    ElMessage.warning('密码不能为空')
    return
  }
  if (pwdForm.value.new !== pwdForm.value.confirm) {
    ElMessage.warning('两次新密码输入不一致')
    return
  }

  pwdLoading.value = true
  try {
    const res = await axios.put('/api/admin/password', {
      old_password: pwdForm.value.old,
      new_password: pwdForm.value.new
    })
    if (res.data.code === 200) {
      ElMessage.success('密码修改成功')
      pwdForm.value = { old: '', new: '', confirm: '' }
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '修改失败')
  } finally {
    pwdLoading.value = false
  }
}

const handleLogout = () => {
  router.push('/')
}

const handleKeydown = (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'b') {
    const tagName = e.target.tagName.toLowerCase()
    if (tagName !== 'input' && tagName !== 'textarea' && tagName !== 'select' && !e.target.isContentEditable) {
      e.preventDefault()
      isCollapsed.value = !isCollapsed.value
    }
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
})

const menuItems = ref([
  { title: '工作台', path: '/admin/dashboard', icon: DataLine },
  { title: '收银', path: '/admin/pos', icon: ShoppingCart },
  { title: '商品', path: '/admin/products', icon: Box },
  { title: '入库', path: '/admin/purchase', icon: DocumentCopy },
  { title: '库存流水', path: '/admin/inventory-logs', icon: List },
  { title: '订单', path: '/admin/orders', icon: Memo },
  { title: '财务', path: '/admin/finance', icon: DataLine },
  { title: '员工', path: '/admin/employees', icon: User },
  { title: '系统设置', path: '/admin/settings', icon: Setting },
])

const mobileMenuItems = ref([
  { title: '台面', path: '/admin/dashboard', icon: DataLine },
  { title: '收银', path: '/admin/pos', icon: ShoppingCart },
  { title: '商品', path: '/admin/products', icon: Box },
  { title: '库存流水', path: '/admin/inventory-logs', icon: List },
  { title: '订单', path: '/admin/orders', icon: Memo },
])
</script>

<style>
/* CSS transition for router view - very smooth like Linear */
.page-fade-enter-active,
.page-fade-leave-active {
  transition: opacity 0.25s ease, transform 0.25s cubic-bezier(0.16, 1, 0.3, 1);
}

.page-fade-enter-from {
  opacity: 0;
  transform: translateY(8px);
}

.page-fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

/* Custom styles for mobile aside shadow */
@media (max-width: 768px) {
  .translate-x-0 {
    box-shadow: 20px 0 50px -10px rgba(0, 0, 0, 0.3);
  }
}
</style>
