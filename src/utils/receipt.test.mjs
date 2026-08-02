import test from 'node:test'
import assert from 'node:assert/strict'
import { buildReceiptFromCheckout, buildReceiptFromOrder, paymentMethodLabel, parsePaymentBreakdown } from './receipt.js'

test('buildReceiptFromCheckout formats checkout snapshot with discounts and gifts', () => {
  const receipt = buildReceiptFromCheckout({
    orderNo: '202606290001',
    createdAt: new Date('2026-06-29T10:00:00+08:00'),
    customer: { name: '张三', phone: '13800000000', address: '测试地址' },
    items: [
      { name: '冰箱', specification: '双门', model: 'A1', quantity: 1, price: 3000, isGift: false },
      { name: '赠品杯', quantity: 2, price: 20, isGift: true }
    ],
    totals: {
      totalAmount: 3000,
      subsidyAmount: 450,
      tradeInAmount: 100,
      discountAmount: 5,
      actualPayAmount: 2545,
      receiptAmount: 2545,
      depositAmount: 0
    },
    paymentMethod: 1,
    deliveryMethod: 2,
    employeeName: '李四',
    referrerName: '朋友'
  })

  assert.equal(receipt.orderNo, '202606290001')
  assert.equal(receipt.customer.name, '张三')
  assert.equal(receipt.items.length, 2)
  assert.equal(receipt.items[0].lineTotalText, '¥3000.00')
  assert.equal(receipt.items[1].lineTotalText, '赠品')
  assert.equal(receipt.summary.actualPayAmountText, '¥2545.00')
  assert.equal(receipt.paymentMethodLabel, '支付宝')
  assert.equal(receipt.deliveryMethodLabel, '主仓发货')
})

test('buildReceiptFromOrder supports existing order data for reprint', () => {
  const receipt = buildReceiptFromOrder({
    order_no: '202606290002',
    created_at: '2026-06-29T11:00:00+08:00',
    customer_name: '王五',
    customer_phone: '13900000000',
    customer_address: '复打地址',
    total_amount: 1200,
    subsidy_amount: 0,
    trade_in_amount: 0,
    discount_amount: 0,
    actual_pay_amount: 500,
    deposit_amount: 500,
    payment_status: 1,
    payment_method: 2,
    delivery_method: 1,
    order_items: [
      { quantity: 1, unit_price: 1200, is_gift: false, product: { name: '电视', specification: '55寸', model: 'T55' } }
    ]
  })

  assert.equal(receipt.orderNo, '202606290002')
  assert.equal(receipt.summary.receiptAmountText, '¥500.00')
  assert.equal(receipt.summary.depositAmountText, '¥500.00')
  assert.equal(receipt.paymentMethodLabel, '微信')
  assert.equal(receipt.deliveryMethodLabel, '门店自提')
})

test('paymentMethodLabel maps cash code 5 to 现金', () => {
  assert.equal(paymentMethodLabel(5), '现金')
  assert.equal(paymentMethodLabel('5'), '现金')
})

test('parsePaymentBreakdown renders mixed payment including cash', () => {
  const breakdown = JSON.stringify([
    { payment_method: 1, account_id: 1, amount: 100.00 },
    { payment_method: 5, account_id: 4, amount: 50.00 }
  ])
  const text = parsePaymentBreakdown(breakdown)
  assert.equal(text, '支付宝 ¥100.00 / 现金 ¥50.00')
})
