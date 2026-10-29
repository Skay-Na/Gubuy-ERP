<template>
  <div class="h-[100dvh] w-full bg-[#f6f9fc] flex flex-col md:flex-row overflow-hidden relative font-sans text-slate-800 tracking-tight">
    
    <!-- Mobile Header (auto-hide on scroll down) -->
    <header ref="headerRef"
      class="md:hidden absolute inset-x-0 top-0 flex items-center justify-between px-5 py-3 bg-white/80 backdrop-blur-xl border-b border-slate-200/50 z-50 transition-transform duration-300 ease-out will-change-transform"
      :class="headerVisible ? 'translate-y-0' : '-translate-y-full'"
    >
      <div class="flex items-center gap-3">
        <div class="w-8 h-8 rounded-xl bg-gradient-to-br from-[#635bff] to-[#8b85ff] shadow-md flex items-center justify-center text-white font-bold text-base">
          E
        </div>
        <span class="font-bold text-lg tracking-tight text-slate-900">美的智慧 ERP</span>
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
          <div class="w-10 h-10 rounded-2xl bg-gradient-to-br from-[#635bff] to-[#8b85ff] shadow-lg shadow-[#635bff] flex items-center justify-center text-white font-bold text-xl flex-shrink-0">
            E
          </div>
          <span v-if="!isCollapsed || mobileMenuOpen" class="font-bold text-xl tracking-tighter bg-clip-text text-transparent bg-gradient-to-r from-slate-900 to-slate-600 whitespace-nowrap animate-in fade-in slide-in-from-left-2 duration-300">
            美的智慧 ERP
          </span>
        </div>
        <!-- Close Button for Mobile -->
        <button @click="mobileMenuOpen = false" class="md:hidden p-2 text-slate-400">
          <el-icon :size="24"><Close /></el-icon>
        </button>
      </div>

      <nav class="flex-1 px-3 overflow-y-auto pb-2">
        <div v-if="mobileMenuOpen" class="flex items-center gap-3 px-2 py-4 mb-4 border-b border-slate-100">
          <div class="w-12 h-12 rounded-full bg-gradient-to-tr from-cyan-400 to-blue-500 shadow-sm border-2 border-white flex-shrink-0"></div>
          <div class="overflow-hidden">
            <div class="font-bold text-slate-800 truncate">王冲</div>
            <div class="text-xs text-slate-400 truncate">Store Manager</div>
          </div>
        </div>

        <template v-for="row in navRows" :key="row.type === 'group' ? 'g-' + row.key : row.path">
          <!-- 分组头：图标栏模式下退化为一条细分割线 -->
          <div v-if="row.type === 'group'" class="pt-2 pb-0.5">
            <!-- 移动端抽屉不折叠，组头只作静态标签，避免出现点了没反应的箭头 -->
            <div v-if="mobileMenuOpen" class="pl-3 text-[10px] font-bold uppercase tracking-widest text-slate-400">{{ row.title }}</div>
            <button v-else-if="!isIconRail"
                    @click="toggleGroup(row.key)"
                    class="w-full flex items-center justify-between pl-3 pr-1 text-[10px] font-bold uppercase tracking-widest transition-colors"
                    :class="row.collapsed ? 'text-slate-300 hover:text-slate-500' : 'text-slate-400 hover:text-slate-600'">
              <span>{{ row.title }}</span>
              <el-icon :size="11" class="transition-transform duration-200" :class="row.collapsed ? '-rotate-90' : ''"><ArrowDown /></el-icon>
            </button>
            <div v-else class="flex justify-center py-0.5"><div class="w-6 h-px bg-slate-200"></div></div>
          </div>

          <router-link v-else :to="row.path" custom v-slot="{ isActive, navigate }">
            <div @click="() => { navigate(); mobileMenuOpen = false }"
                 class="flex items-center rounded-xl cursor-pointer transition-all duration-200 group relative"
                 :class="[
                   isActive ? 'bg-gradient-to-r from-[#635bff] to-[#4b44c8] text-white shadow-md shadow-[#635bff]/25' : 'text-slate-500 hover:bg-slate-100/80 hover:text-slate-900',
                   isIconRail ? 'justify-center w-12 h-10 mx-auto px-0' : 'px-3.5 py-2 gap-3'
                 ]">
              <div class="flex-shrink-0 w-6 h-6 flex items-center justify-center">
                <component :is="row.icon" class="w-6 h-6 transition-transform duration-300 group-hover:scale-110" :class="isActive ? 'text-white' : 'text-slate-400 group-hover:text-slate-700'" style="width: 24px; height: 24px" />
              </div>
              <span v-if="!isIconRail" class="font-medium text-[13px] whitespace-nowrap animate-in fade-in slide-in-from-left-2 duration-300">{{ row.title }}</span>
            </div>
          </router-link>
        </template>
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
                <span class="font-semibold text-xs truncate text-slate-800 font-sans tracking-tight">王冲</span>
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
    <main class="min-w-0 flex-1 overflow-hidden flex flex-col relative z-0" style="background:#f6f9fc">
      <div
        ref="scrollContainer"
        :style="{ paddingTop: scrollPaddingTop }"
        class="min-w-0 flex-1 overflow-x-hidden overflow-y-auto px-4 md:px-8 pt-0 md:pt-8 scroll-smooth will-change-scroll pb-[env(safe-area-inset-bottom)] transition-[padding-top] duration-300 ease-out"
      >
        <div class="min-w-0 max-w-7xl mx-auto w-full">
          <router-view />
        </div>
      </div>
    </main>
  </div>
</template>

<script setup>
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import {
  DataLine,
  ShoppingCart,
  Box,
  Memo,
  Fold,
  User,
  SwitchButton,
  Close,
  Setting,
  Money,
  EditPen,
  Clock,
  Wallet,
  Suitcase,
  MagicStick,
  ArrowDown
} from '@element-plus/icons-vue'
import { useRouter } from 'vue-router'
import { updateAdminPassword } from '../api/auth'
import { clearAuth } from '../utils/request'
import { ElMessage } from 'element-plus'

const router = useRouter()
const mobileMenuOpen = ref(false)
const isCollapsed = ref(false)

// Auto-hide mobile header on scroll
// Header is shown only when:
//   1. The user is at the very top of the page, OR
//   2. The user swipes DOWN quickly (scrollTop decreasing fast = going back toward top).
// Slow scrolling in either direction keeps the header hidden so it does not flicker.
// Notes on sign convention:
//   - delta > 0 (scrollTop growing) => finger swiping UP => reading forward => HIDE
//   - delta < 0 (scrollTop shrinking) => finger swiping DOWN => going back to top
//     - If fast enough: SHOW (user is reaching for navigation)
//     - If slow: keep current state
const headerVisible = ref(true)
const scrollContainer = ref(null)
const headerRef = ref(null) // template ref on the mobile <header>, used to measure its real height
const isMobile = ref(typeof window !== 'undefined' && window.innerWidth < 768)
// Mobile header height is measured at runtime via ResizeObserver. The fallback (60px)
// covers the typical py-3 + h-8 icon + border-b case before the observer first fires.
const headerHeight = ref(60)
let headerResizeObserver = null
// Reserve top padding on the scroll container so the absolute header never overlaps content.
// - Mobile + header visible: headerHeight + 4px breathing room (content sits below the bar)
// - Mobile + header hidden :  0                  (content uses the full height)
// - Desktop                :  undefined           (let Tailwind md:pt-8 take over)
const scrollPaddingTop = computed(() => {
  if (!isMobile.value) return undefined
  return headerVisible.value ? `${headerHeight.value + 4}px` : '0px'
})
let lastScrollTop = 0
let lastScrollTime = 0
let showUntil = 0 // timestamp; header stays visible until this point after a fast swipe

// Tunables
const TOP_THRESHOLD = 16 // px - top zone where header is always visible
const FAST_BACK_VELOCITY = 1.0 // px/ms (~1000 px/s) - magnitude needed for a "fast back-to-top" swipe
const SHOW_LINGER_MS = 600 // keep header visible briefly after a fast swipe so it does not flicker

const handleScroll = () => {
  // Only auto-hide on mobile (md breakpoint is 768px)
  if (typeof window !== 'undefined' && window.innerWidth >= 768) {
    headerVisible.value = true
    lastScrollTop = 0
    lastScrollTime = 0
    showUntil = 0
    return
  }
  const el = scrollContainer.value
  if (!el) return

  const now = performance.now()
  const current = el.scrollTop
  const dt = Math.max(1, now - lastScrollTime)
  const delta = current - lastScrollTop
  const velocity = delta / dt // px/ms; positive = swiping UP (reading forward), negative = swiping DOWN (going back)

  if (current <= TOP_THRESHOLD) {
    // At the top: always visible
    headerVisible.value = true
    showUntil = 0
  } else if (velocity < -FAST_BACK_VELOCITY) {
    // Fast DOWN swipe (scrollTop shrinking fast): show, then keep visible during linger window
    headerVisible.value = true
    showUntil = now + SHOW_LINGER_MS
  } else if (now < showUntil) {
    // Still inside the linger window after a recent fast swipe
    headerVisible.value = true
  } else if (velocity > 0) {
    // UP swipe / reading forward: hide
    headerVisible.value = false
  }
  // Else: slow DOWN swipe while not at the top -> keep current state (no flicker)

  lastScrollTop = current
  lastScrollTime = now
}

// Reset header state when viewport crosses the mobile/desktop breakpoint
const handleResize = () => {
  isMobile.value = window.innerWidth < 768
  if (window.innerWidth >= 768) {
    headerVisible.value = true
    lastScrollTop = 0
    lastScrollTime = 0
    showUntil = 0
  }
}

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
    const res = await updateAdminPassword({
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
  // 退出前清空本地登录态，避免返回门户后残留 Token
  clearAuth()
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
  window.addEventListener('resize', handleResize)
  // Attach scroll listener on next tick so the ref is mounted
  nextTick(() => {
    if (scrollContainer.value) {
      scrollContainer.value.addEventListener('scroll', handleScroll, { passive: true })
    }
    // Measure the real header height so the scroll container's padding matches it exactly,
    // and keep measuring if the layout ever changes (font load, rotation, etc.)
    if (headerRef.value) {
      headerHeight.value = headerRef.value.offsetHeight
      headerResizeObserver = new ResizeObserver(entries => {
        for (const entry of entries) {
          // entry.contentRect.height excludes transforms (e.g. translate-y-full),
          // so we always read the bar's natural height.
          headerHeight.value = entry.contentRect.height
        }
      })
      headerResizeObserver.observe(headerRef.value)
    }
  })
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('resize', handleResize)
  if (scrollContainer.value) {
    scrollContainer.value.removeEventListener('scroll', handleScroll)
  }
  if (headerResizeObserver) {
    headerResizeObserver.disconnect()
    headerResizeObserver = null
  }
})

// 菜单按业务域分组：分组做的是「分类」不是「藏东西」，默认只收起最低频的后台管理组
const MENU_GROUPS = [
  {
    key: 'insight',
    title: '经营分析',
    items: [
      { title: 'AI 店长', path: '/admin/ai-manager', icon: MagicStick },
      { title: '看板', path: '/admin/dashboard', icon: DataLine }
    ]
  },
  {
    key: 'business',
    title: '日常经营',
    items: [
      { title: '收银', path: '/admin/pos', icon: ShoppingCart },
      { title: '商品', path: '/admin/products', icon: Box },
      { title: '订单', path: '/admin/orders', icon: Memo },
      { title: '国补', path: '/admin/subsidy', icon: Money }
    ]
  },
  {
    key: 'finance',
    title: '财务与工程',
    items: [
      { title: '记账', path: '/admin/bookkeeping', icon: EditPen },
      { title: '财务', path: '/admin/finance', icon: Wallet },
      { title: '工程', path: '/admin/engineering', icon: Suitcase }
    ]
  },
  {
    key: 'admin',
    title: '后台管理',
    defaultCollapsed: true, // 员工/系统设置使用频率最低，默认收起给菜单留白
    hideOnMobile: true,     // 手机端抽屉空间有限，这类后台功能不在移动端出现
    items: [
      { title: '员工', path: '/admin/employees', icon: User },
      { title: '历史操作', path: '/admin/history', icon: Clock },
      { title: '系统设置', path: '/admin/settings', icon: Setting }
    ]
  }
]

const MENU_COLLAPSE_KEY = 'erp_menu_collapsed_groups'
const DEFAULT_COLLAPSED_GROUPS = MENU_GROUPS.filter(group => group.defaultCollapsed).map(group => group.key)

const loadCollapsedGroups = () => {
  try {
    const raw = localStorage.getItem(MENU_COLLAPSE_KEY)
    if (raw === null) return [...DEFAULT_COLLAPSED_GROUPS]
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return [...DEFAULT_COLLAPSED_GROUPS]
    // 过滤掉已不存在的分组 key，避免历史数据留下脏状态
    return parsed.filter(key => MENU_GROUPS.some(group => group.key === key))
  } catch {
    return [...DEFAULT_COLLAPSED_GROUPS]
  }
}

const collapsedGroups = ref(loadCollapsedGroups())

const saveCollapsedGroups = () => {
  try {
    localStorage.setItem(MENU_COLLAPSE_KEY, JSON.stringify(collapsedGroups.value))
  } catch {
    /* 隐私模式下 localStorage 可能不可写，折叠状态不持久化即可，不影响功能 */
  }
}

// 侧边栏收起成图标条时不再分组，全部平铺（否则只剩组头没有入口）
const isIconRail = computed(() => isCollapsed.value && !mobileMenuOpen.value)

// 一维渲染序列：组头与菜单项混排，模板只做一次 v-for，避免两套重复模板
const navRows = computed(() => {
  const rows = []
  for (const group of MENU_GROUPS) {
    if (mobileMenuOpen.value && group.hideOnMobile) continue
    const collapsed = !isIconRail.value && !mobileMenuOpen.value && collapsedGroups.value.includes(group.key)
    // 图标栏模式下第一条分割线没有意义，只从第二个分组开始画
    if (!isIconRail.value || rows.length > 0) {
      rows.push({ type: 'group', key: group.key, title: group.title, collapsed })
    }
    if (collapsed) continue
    for (const item of group.items) rows.push({ type: 'item', ...item })
  }
  return rows
})

const toggleGroup = (key) => {
  if (isIconRail.value) return
  const index = collapsedGroups.value.indexOf(key)
  if (index >= 0) collapsedGroups.value.splice(index, 1)
  else collapsedGroups.value.push(key)
  saveCollapsedGroups()
}

// 进到某个分组的页面时自动展开该组，避免「人在页面里，菜单里却找不到它」
watch(
  () => router.currentRoute.value.path,
  (path) => {
    const hit = MENU_GROUPS.find(group =>
      group.items.some(item => path === item.path || path.startsWith(item.path + '/'))
    )
    if (!hit || !collapsedGroups.value.includes(hit.key)) return
    collapsedGroups.value = collapsedGroups.value.filter(key => key !== hit.key)
    saveCollapsedGroups()
  },
  { immediate: true }
)
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
