import { createRouter, createWebHistory } from 'vue-router'
import Layout from '../layout/Index.vue'

const routes = [
  {
    path: '/',
    name: 'Portal',
    component: () => import('../views/Portal.vue'),
    meta: { title: '系统入口' }
  },
  {
    path: '/admin',
    component: Layout,
    redirect: '/admin/ai-manager',
    children: [
      {
        path: 'ai-manager',
        name: 'AIManager',
        component: () => import('../views/AIManager.vue'),
        meta: { title: 'AI 店长' }
      },
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('../views/dashboard/Index.vue'),
        meta: { title: '看板' }
      },
      {
        path: 'pos',
        name: 'POS',
        component: () => import('../views/Cashier.vue'),
        meta: { title: '收银工作台' }
      },
      {
        path: 'products',
        component: () => import('../views/ProductWorkspace.vue'),
        meta: { title: '商品管理' },
        children: [
          {
            path: '',
            name: 'ProductManage',
            component: () => import('../views/ProductManage.vue'),
            meta: { title: '商品库管理' }
          },
          {
            path: 'inbound',
            name: 'Purchase',
            component: () => import('../views/Inbound.vue'),
            meta: { title: '采购入库' }
          },
          {
            path: 'inventory-logs',
            name: 'InventoryLog',
            component: () => import('../views/InventoryLog.vue'),
            meta: { title: '库存流水审计' }
          }
        ]
      },
      {
        path: 'purchase',
        redirect: '/admin/products/inbound'
      },
      {
        path: 'inventory-logs',
        redirect: '/admin/products/inventory-logs'
      },
      {
        path: 'employees',
        name: 'EmployeeManage',
        component: () => import('../views/EmployeeManage.vue'),
        meta: { title: '员工管理' }
      },
      {
        path: 'orders',
        name: 'OrderManage',
        component: () => import('../views/OrderManage.vue'),
        meta: { title: '订单管理' }
      },
      // 工程模块：仅管理端后台，与零售订单/国补链路完全独立。
      // 路由顺序注意：'engineering/expenses' 必须排在 'engineering/:id' 之前，
      // 否则 'expenses' 会被当成 :id 匹配进详情页。
      {
        path: 'engineering',
        name: 'EngineeringList',
        component: () => import('../views/engineering/Index.vue'),
        meta: { title: '工程项目' }
      },
      {
        path: 'engineering/expenses',
        name: 'EngineeringExpenses',
        component: () => import('../views/engineering/Expenses.vue'),
        meta: { title: '工程开销' }
      },
      {
        path: 'engineering/:id',
        name: 'EngineeringDetail',
        component: () => import('../views/engineering/Detail.vue'),
        meta: { title: '工程详情' }
      },
      {
        path: 'finance',
        name: 'Finance',
        component: () => import('../views/Finance.vue'),
        meta: { title: '资金工作台' }
      },
      {
        path: 'history',
        name: 'OperationHistory',
        component: () => import('../views/OperationHistory.vue'),
        meta: { title: '历史操作' }
      },
      {
        path: 'bookkeeping',
        name: 'Bookkeeping',
        component: () => import('../views/Bookkeeping.vue'),
        meta: { title: '专业记账' }
      },
      {
        path: 'ai-import',
        name: 'AIImport',
        component: () => import('../views/AIImport.vue'),
        meta: { title: 'AI 图片导入' }
      },
      {
        path: 'subsidy',
        name: 'SubsidyManage',
        component: () => import('../views/SubsidyManage.vue'),
        meta: { title: '国补回款管理' }
      },
      {
        path: 'settings',
        name: 'SystemSettings',
        component: () => import('../views/SystemSettings.vue'),
        meta: { title: '系统设置' }
      }
    ]
  },
  {
    path: '/terminal',
    name: 'StaffTerminal',
    component: () => import('../views/StaffTerminal.vue'),
    meta: { title: '员工收银终端' }
  },
  {
    path: '/terminal/inventory',
    name: 'StaffInventory',
    component: () => import('../views/StaffInventory.vue'),
    meta: { title: '员工库管' }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to, from, next) => {
  // 支持 ?token= 直达：存入登录态后剥掉 URL 中的 token（本地演示/多端快速登录用）
  const queryToken = typeof to.query.token === 'string' ? to.query.token : ''
  if (queryToken) {
    localStorage.setItem('erp_token', queryToken)
    const query = { ...to.query }
    delete query.token
    next({ path: to.path, query, replace: true })
    return
  }
  const token = localStorage.getItem('erp_token')
  if (to.path !== '/' && !to.path.startsWith('/terminal') && !token) {
    next('/')
  } else {
    next()
  }
})

// 统一页面标题："页面名 - 美的智慧家ERP"（取路由 meta.title）
router.afterEach(to => {
  document.title = to.meta?.title ? `${to.meta.title} - 美的智慧家ERP` : '美的智慧家ERP'
})

export default router
