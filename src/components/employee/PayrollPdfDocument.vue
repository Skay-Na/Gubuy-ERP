<script setup>
import { computed, useTemplateRef } from 'vue'
import dayjs from 'dayjs'

const props = defineProps({
  employee: { type: Object, required: true },
  month: { type: String, required: true }
})

const pageRoot = useTemplateRef('pageRoot')
const money = value => `¥${Number(value || 0).toFixed(2)}`
const orderDate = value => value ? dayjs(value).format('YYYY-MM-DD HH:mm') : '-'
const orderType = value => value === 2 ? '商会平分' : '个人订单'
const productModels = order => (order.product_models || []).join('、') || '未记录商品型号'
const orderRows = computed(() => props.employee.commission_details || [])
const orderPages = computed(() => {
  const firstPageCount = 6
  const continuationCount = 12
  const pages = [orderRows.value.slice(0, firstPageCount)]
  for (let offset = firstPageCount; offset < orderRows.value.length; offset += continuationCount) {
    pages.push(orderRows.value.slice(offset, offset + continuationCount))
  }
  return pages
})

const getPageElements = () => Array.from(pageRoot.value?.querySelectorAll('.pdf-page') || [])
defineExpose({ getPageElements })
</script>

<template>
  <div ref="pageRoot" class="pdf-render-root" aria-hidden="true">
    <section v-for="(orders, pageIndex) in orderPages" :key="pageIndex" class="pdf-page">
      <header class="pdf-header">
        <div>
          <p>EMPLOYEE PAYROLL</p>
          <h1>{{ month }} 薪资明细</h1>
        </div>
        <div class="employee-meta">
          <strong>{{ employee.employee_name }}</strong>
          <span>工号 {{ employee.emp_no || '-' }}</span>
        </div>
      </header>

      <template v-if="pageIndex === 0">
        <div class="net-pay">
          <span>本月实发</span>
          <strong>{{ money(employee.final_salary) }}</strong>
        </div>
        <div class="summary-grid">
          <div><span>基本底薪</span><strong>{{ money(employee.base_salary) }}</strong></div>
          <div><span>个人订单提成</span><strong>{{ money(employee.personal_commission) }}</strong></div>
          <div><span>商会订单平分</span><strong>{{ money(employee.chamber_commission) }}</strong></div>
          <div class="deduction"><span>超假扣款</span><strong>-{{ money(employee.deduction) }}</strong></div>
        </div>
        <div class="attendance-note">
          <strong>考勤说明</strong>
          <span>请假 {{ employee.leave_days || 0 }} 天，其中免费 {{ employee.free_leave_days || 0 }} 天；扣款 {{ employee.deduction_days || 0 }} 天，日薪 {{ money(employee.daily_salary) }}。</span>
        </div>
      </template>

      <div class="orders-heading">
        <h2>提成订单明细</h2>
        <span>共 {{ orderRows.length }} 笔</span>
      </div>
      <table class="orders-table">
        <thead><tr><th>订单号 / 时间</th><th>类型</th><th>订单金额</th><th>订单提成</th><th>本人所得</th></tr></thead>
        <tbody>
          <tr v-for="order in orders" :key="order.order_id">
            <td><strong>{{ order.order_no }}</strong><small>{{ orderDate(order.order_date) }}</small><small>客户：{{ order.customer_name || '散客' }}</small><small>型号：{{ productModels(order) }}</small></td>
            <td>{{ orderType(order.order_type) }}<small v-if="order.order_type === 2">{{ order.participant_count || 0 }} 人平分</small></td>
            <td>{{ money(order.order_amount) }}</td>
            <td>{{ money(order.commission_pool) }}</td>
            <td class="income">{{ money(order.commission_amount) }}</td>
          </tr>
          <tr v-if="!orders.length"><td class="empty" colspan="5">本月没有计入提成的订单</td></tr>
        </tbody>
      </table>

      <footer class="pdf-footer">
        <span>本明细由系统根据薪资结算数据生成</span>
        <span>第 {{ pageIndex + 1 }} / {{ orderPages.length }} 页</span>
      </footer>
    </section>
  </div>
</template>

<style scoped>
.pdf-render-root{position:fixed;left:-10000px;top:0;width:794px;z-index:-1;pointer-events:none;color:#17243d;font-family:"PingFang SC","Microsoft YaHei",Arial,sans-serif}.pdf-page{box-sizing:border-box;width:794px;height:1123px;padding:62px 58px 48px;background:#fff;overflow:hidden;position:relative}.pdf-header{display:flex;align-items:flex-start;justify-content:space-between;padding-bottom:24px;border-bottom:2px solid #153760}.pdf-header p{margin:0 0 8px;color:#55708f;font-size:12px;font-weight:800;letter-spacing:.15em}.pdf-header h1{margin:0;font-size:30px;letter-spacing:-.03em}.employee-meta{display:flex;flex-direction:column;align-items:flex-end;gap:5px}.employee-meta strong{font-size:20px}.employee-meta span{color:#728198;font-size:12px}.net-pay{display:flex;align-items:center;justify-content:space-between;margin:24px 0 14px;padding:22px 24px;border-radius:14px;background:#153760;color:#fff}.net-pay span{font-size:14px}.net-pay strong{font-size:31px}.summary-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:10px}.summary-grid div{display:flex;flex-direction:column;gap:8px;padding:15px;border:1px solid #e2e8f0;border-radius:10px;background:#f8fafc}.summary-grid span{color:#64748b;font-size:11px}.summary-grid strong{font-size:16px}.summary-grid .deduction strong{color:#d55353}.attendance-note{display:flex;gap:18px;margin-top:14px;padding:13px 15px;border-radius:9px;background:#eef5ff;font-size:11px}.attendance-note span{color:#52657e}.orders-heading{display:flex;align-items:center;justify-content:space-between;margin:26px 0 10px}.orders-heading h2{margin:0;font-size:16px}.orders-heading span{color:#728198;font-size:11px}.orders-table{width:100%;border-collapse:collapse;table-layout:fixed;font-size:11px}.orders-table th{padding:10px 8px;background:#f1f5f9;color:#52657e;text-align:left}.orders-table th:first-child{width:30%}.orders-table th:nth-child(2){width:15%}.orders-table th:nth-child(n+3){text-align:right}.orders-table td{padding:11px 8px;border-bottom:1px solid #e7ecf2;vertical-align:top}.orders-table td:nth-child(n+3){text-align:right}.orders-table td strong,.orders-table td small{display:block}.orders-table td small{margin-top:4px;color:#8490a4;font-size:9px}.orders-table .income{color:#1f5d9f;font-weight:800}.orders-table .empty{padding:28px;text-align:center;color:#8490a4}.pdf-footer{position:absolute;left:58px;right:58px;bottom:28px;display:flex;justify-content:space-between;padding-top:12px;border-top:1px solid #e4eaf2;color:#94a3b8;font-size:9px}
</style>
