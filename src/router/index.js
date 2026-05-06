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
    redirect: '/admin/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('../views/dashboard/Index.vue'),
        meta: { title: '工作台' }
      },
      {
        path: 'pos',
        name: 'POS',
        component: () => import('../views/Cashier.vue'),
        meta: { title: '收银工作台' }
      },
      {
        path: 'purchase',
        name: 'Purchase',
        component: () => import('../views/Inbound.vue'),
        meta: { title: '采购入库' }
      },
      {
        path: 'products',
        name: 'ProductManage',
        component: () => import('../views/ProductManage.vue'),
        meta: { title: '商品库管理' }
      },
      {
        path: 'inventory-logs',
        name: 'InventoryLog',
        component: () => import('../views/InventoryLog.vue'),
        meta: { title: '库存流水审计' }
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
        meta: { title: '财务总览' }
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

export default router
