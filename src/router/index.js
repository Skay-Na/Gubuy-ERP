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
      {
        path: 'finance',
        name: 'Finance',
        component: () => import('../views/Finance.vue'),
        meta: { title: '资金工作台' }
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
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('erp_token')
  if (to.path !== '/' && to.path !== '/terminal' && !token) {
    next('/')
  } else {
    next()
  }
})

export default router
