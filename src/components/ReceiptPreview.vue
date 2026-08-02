<template>
  <el-dialog
    v-model="visible"
    title="A4 竖向购物小票预览"
    width="760px"
    class="receipt-dialog"
    append-to-body
    destroy-on-close
    :fullscreen="isMobilePreview"
  >
    <div v-if="receipt && visible" class="receipt-a4-page" id="receipt-print-area">
      <section v-for="copy in receiptCopies" :key="copy.key" class="receipt-copy">
        <div class="receipt-copy-watermark">{{ copy.label }}</div>
        <div class="receipt-copy-content">
          <div class="receipt-header">
            <div>
              <div class="receipt-store">{{ receipt.storeName }}</div>
              <div class="receipt-title">{{ receipt.title }} · {{ copy.label }}</div>
            </div>
            <div class="receipt-order-no">{{ receipt.orderNo }}</div>
          </div>

          <div class="receipt-meta-grid">
            <div><span>时间</span><b>{{ receipt.createdAtText }}</b></div>
            <div><span>客户</span><b>{{ receipt.customer.name }}</b></div>
            <div><span>电话</span><b>{{ receipt.customer.phone }}</b></div>
            <div><span>开单人</span><b>{{ receipt.employeeName }}</b></div>
            <div class="meta-wide"><span>地址</span><b>{{ receipt.customer.address }}</b></div>
          </div>

          <div class="receipt-body-grid">
            <div class="receipt-items">
              <div class="items-head">
                <span>商品</span><span>类型</span><span>数量</span><span>单价</span><span>小计</span>
              </div>
              <div v-for="item in receipt.items" :key="`${copy.key}-${item.key}`" class="items-row">
                <div class="item-name">
                  <b>{{ item.name }}</b>
                  <small v-if="item.specText">{{ item.specText }}</small>
                  <em v-if="item.isGift">赠品</em>
                </div>
                <span class="item-category" style="font-size: 0.9em; align-self: center; color: #64748b;">{{ item.categoryName }}</span>
                <span>x{{ item.quantity }}</span>
                <span>{{ item.unitPriceText }}</span>
                <strong>{{ item.lineTotalText }}</strong>
              </div>
            </div>

            <div class="receipt-summary">
              <div><span>订单金额</span><b>{{ receipt.summary.totalAmountText }}</b></div>
              <div v-if="receipt.summary.subsidyAmount > 0"><span>国补优惠</span><b>-{{ receipt.summary.subsidyAmountText }}</b></div>
              <div v-if="receipt.summary.tradeInAmount > 0"><span>以旧换新支出</span><b>{{ receipt.summary.tradeInAmountText }}</b></div>
              <div v-if="receipt.summary.discountAmount > 0"><span>抹零优惠</span><b>-{{ receipt.summary.discountAmountText }}</b></div>
              <div class="summary-payable"><span>客户应付</span><b>{{ receipt.summary.actualPayAmountText }}</b></div>
              <div v-if="receipt.summary.depositAmount > 0" class="summary-deposit"><span>本次定金</span><b>{{ receipt.summary.depositAmountText }}</b></div>
              <div class="summary-received"><span>本次实收</span><b>{{ receipt.summary.receiptAmountText }}</b></div>
              <div><span>支付方式</span><b>{{ receipt.paymentMethodLabel }}</b></div>
              <div v-if="receipt.paymentBreakdown" class="summary-note">{{ receipt.paymentBreakdown }}</div>
              <div><span>交付方式</span><b>{{ receipt.deliveryMethodLabel }}</b></div>
            </div>
          </div>

          <div class="receipt-footer">
            <span>{{ receipt.footer }}</span>
            <span class="signature">客户签字：________________　店员签字：________________</span>
          </div>
        </div>
      </section>
    </div>

    <template #footer>
      <div class="flex justify-between items-center gap-2">
        <span class="footer-tip text-xs text-slate-500">打印版式：A4 竖向，一页上下两联（客户联 / 店铺留存联）</span>
        <div class="flex gap-2">
          <el-button @click="visible = false">关闭</el-button>
          <el-button type="primary" @click="printReceipt">打印 A4 小票</el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  receipt: { type: Object, default: null }
})

const emit = defineEmits(['update:modelValue'])

const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value)
})

const receiptCopies = [
  { key: 'customer', label: '客户联' },
  { key: 'store', label: '店铺留存联' }
]

const printReceipt = () => {
  window.print()
}

const windowWidth = ref(typeof window === 'undefined' ? 1024 : window.innerWidth)
const isMobilePreview = computed(() => windowWidth.value < 768)
const handleResize = () => {
  windowWidth.value = window.innerWidth
}

onMounted(() => window.addEventListener('resize', handleResize))
onUnmounted(() => window.removeEventListener('resize', handleResize))
</script>

<style scoped>
.receipt-a4-page {
  width: 680px;
  margin: 0 auto;
  padding: 18px;
  background: #f8fafc;
  color: #0f172a;
  font-family: Arial, "PingFang SC", "Microsoft YaHei", sans-serif;
}

.receipt-copy {
  position: relative;
  min-height: 310px;
  margin-bottom: 16px;
  padding: 18px 20px;
  background: #fff;
  border: 1px dashed #94a3b8;
  border-radius: 10px;
  overflow: hidden;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.08);
}

.receipt-copy:last-child {
  margin-bottom: 0;
}

.receipt-copy-watermark {
  position: absolute;
  right: 24px;
  top: 12px;
  color: rgba(37, 99, 235, 0.08);
  font-size: 64px;
  font-weight: 900;
  letter-spacing: 8px;
  pointer-events: none;
}

.receipt-copy-content {
  position: relative;
  z-index: 1;
}

.receipt-header,
.receipt-footer {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 16px;
  border-bottom: 1px solid #cbd5e1;
  padding-bottom: 8px;
  margin-bottom: 10px;
}

.receipt-store {
  font-size: 22px;
  font-weight: 900;
  letter-spacing: 2px;
}

.receipt-title {
  margin-top: 2px;
  font-size: 12px;
  color: #64748b;
  font-weight: 700;
}

.receipt-order-no {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 18px;
  font-weight: 900;
}

.receipt-meta-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 6px 14px;
  font-size: 12px;
  margin-bottom: 10px;
}

.receipt-meta-grid div {
  display: flex;
  justify-content: flex-start;
  gap: 8px;
}

.receipt-summary div {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}

.receipt-meta-grid span,
.receipt-summary span {
  color: #64748b;
  white-space: nowrap;
}

.receipt-meta-grid b {
  text-align: left;
  word-break: break-word;
}

.receipt-summary b {
  text-align: right;
  word-break: break-word;
}

.meta-wide {
  grid-column: span 3;
}

.receipt-body-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 10px;
  align-items: start;
}

.receipt-items {
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  overflow: hidden;
}

.items-head,
.items-row {
  display: grid;
  grid-template-columns: 1.5fr 1fr 48px 72px 82px;
  gap: 8px;
  align-items: start;
  padding: 6px 8px;
  font-size: 12px;
}

.items-head {
  background: #f1f5f9;
  color: #475569;
  font-weight: 900;
}

.items-row {
  border-top: 1px solid #e2e8f0;
}

.items-head span:not(:first-child),
.items-row > span,
.items-row > strong {
  text-align: right;
}

.item-name {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.item-name small {
  color: #64748b;
}

.item-name em {
  width: fit-content;
  padding: 1px 6px;
  border-radius: 999px;
  background: #fee2e2;
  color: #dc2626;
  font-size: 10px;
  font-style: normal;
  font-weight: 900;
}

.receipt-summary {
  padding: 10px 12px;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.7;
}

.summary-payable {
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px dashed #cbd5e1;
  font-size: 14px;
  font-weight: 900;
}

.summary-deposit {
  color: #ea580c;
  font-weight: 900;
}

.summary-received {
  color: #1d4ed8;
  font-size: 16px;
  font-weight: 900;
}

.summary-note {
  display: block !important;
  color: #64748b;
  font-size: 11px;
  word-break: break-word;
}

.receipt-footer {
  align-items: center;
  margin: 10px 0 0;
  padding: 8px 0 0;
  border-top: 1px solid #cbd5e1;
  border-bottom: none;
  font-size: 11px;
  color: #475569;
}

.signature {
  color: #0f172a;
  font-weight: 700;
}

@media (max-width: 767px) {
  :global(.receipt-dialog.el-dialog.is-fullscreen) {
    display: flex;
    flex-direction: column;
    margin: 0 !important;
    max-height: 100dvh;
    overflow: hidden;
  }

  :global(.receipt-dialog .el-dialog__body) {
    flex: 1;
    overflow-y: auto;
    padding: 10px 10px 84px !important;
    background: #f8fafc;
  }

  :global(.receipt-dialog .el-dialog__footer) {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 10;
    padding: 10px 12px calc(10px + env(safe-area-inset-bottom)) !important;
    background: rgba(255, 255, 255, 0.96);
    border-top: 1px solid #e2e8f0;
    box-shadow: 0 -8px 24px rgba(15, 23, 42, 0.08);
  }

  :global(.receipt-dialog .el-dialog__footer .flex) {
    align-items: stretch;
  }

  :global(.receipt-dialog .el-dialog__footer .footer-tip) {
    display: none;
  }

  :global(.receipt-dialog .el-dialog__footer .flex > div) {
    display: grid;
    grid-template-columns: 1fr 1.4fr;
    width: 100%;
    gap: 8px;
  }

  .receipt-a4-page {
    width: 100%;
    padding: 0;
    background: transparent;
  }

  .receipt-copy {
    min-height: auto;
    margin-bottom: 12px;
    padding: 12px;
    border-radius: 14px;
  }

  .receipt-copy-watermark {
    font-size: 34px;
    letter-spacing: 2px;
  }

  .receipt-header,
  .receipt-footer {
    align-items: flex-start;
    flex-direction: column;
    gap: 6px;
  }

  .receipt-store { font-size: 18px; }
  .receipt-order-no { font-size: 12px; }

  .receipt-meta-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 6px 10px;
  }

  .meta-wide {
    grid-column: span 2;
  }

  .items-head,
  .items-row {
    grid-template-columns: 1.2fr 1fr 32px 48px 56px;
    gap: 4px;
    padding: 6px;
    font-size: 10px;
  }

  .receipt-summary {
    font-size: 11px;
  }

  .signature {
    width: 100%;
  }
}

@media print {
  :global(body *) {
    visibility: hidden !important;
  }

  :global(#app) {
    display: none !important;
  }

  :global(#receipt-print-area),
  :global(#receipt-print-area *) {
    visibility: visible !important;
  }

  :global(#receipt-print-area) {
    position: fixed !important;
    left: 0 !important;
    top: 0 !important;
    width: 210mm !important;
    height: 297mm !important;
    margin: 0 !important;
    padding: 10mm 12mm !important;
    background: #fff !important;
    box-shadow: none !important;
    overflow: hidden !important;
  }

  .receipt-a4-page {
    width: 210mm;
    height: 297mm;
    padding: 10mm 12mm;
    background: #fff;
  }

  .receipt-copy {
    height: 130mm;
    min-height: 130mm;
    margin: 0 0 10mm 0;
    padding: 6mm;
    border: 0.35mm dashed #334155;
    border-radius: 0;
    box-shadow: none;
    break-inside: avoid;
    page-break-inside: avoid;
  }

  .receipt-copy:last-child {
    margin-bottom: 0;
  }

  .receipt-copy-watermark {
    color: rgba(15, 23, 42, 0.06);
  }

  .receipt-store { font-size: 18pt; }
  .receipt-title, .receipt-meta-grid, .receipt-summary, .items-head, .items-row { font-size: 9pt; }
  .receipt-order-no { font-size: 15pt; }
  .summary-received { font-size: 13pt; }
  .receipt-footer { font-size: 8pt; }
}

@page {
  size: A4 portrait;
  margin: 0;
}
</style>
