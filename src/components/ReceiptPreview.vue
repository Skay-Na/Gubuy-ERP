<template>
  <el-dialog
    v-model="visible"
    title="发货单预览"
    width="900px"
    class="receipt-dialog"
    append-to-body
    destroy-on-close
    :fullscreen="isMobilePreview"
  >
    <div v-if="receipt && visible" class="delivery-order-page" id="receipt-print-area">
      <section
        v-for="copy in receiptCopies"
        :key="copy.key"
        class="delivery-copy"
        :class="`copy-${copy.tone}`"
      >
        <aside class="copy-side">
          <span
            v-for="line in copy.sideLines"
            :key="line"
            class="copy-side-line"
          >{{ line }}</span>
        </aside>

        <div class="delivery-content">
          <header class="delivery-header">
            <h1 class="delivery-title">{{ receipt.storeName }}发货单</h1>
          </header>

          <div class="delivery-meta-row">
            <div class="meta-date">
              <span class="meta-label">日期：</span>
              <span class="meta-value">{{ receipt.createdAt }}</span>
            </div>
            <div class="meta-order-no">
              <span class="meta-label">No.</span>
              <span class="order-no-num">{{ receipt.orderNo }}</span>
            </div>
          </div>

          <div class="delivery-info-row">
            <div class="info-cell">
              <span class="info-label">顾客姓名</span>
              <span class="info-value">{{ receipt.customer.name }}</span>
            </div>
            <div class="info-cell">
              <span class="info-label">联系电话</span>
              <span class="info-value">{{ receipt.customer.phone }}</span>
            </div>
          </div>

          <div class="delivery-info-row single">
            <div class="info-cell wide">
              <span class="info-label">送货地址</span>
              <span class="info-value">{{ receipt.customer.address }}</span>
            </div>
          </div>

          <table class="items-table">
            <colgroup>
              <col class="col-name" />
              <col class="col-qty" />
              <col class="col-unit" />
              <col class="col-price" />
              <col class="col-digit" span="6" />
              <col class="col-note" />
            </colgroup>
            <thead>
              <tr>
                <th rowspan="2" class="cell-name">品名型号</th>
                <th rowspan="2" class="cell-qty">数量</th>
                <th rowspan="2" class="cell-unit">单位</th>
                <th rowspan="2" class="cell-price">单价</th>
                <th colspan="6" class="cell-amount">金额</th>
                <th rowspan="2" class="cell-note">备注</th>
              </tr>
              <tr>
                <th class="cell-digit">十</th>
                <th class="cell-digit">万</th>
                <th class="cell-digit">千</th>
                <th class="cell-digit">百</th>
                <th class="cell-digit">十</th>
                <th class="cell-digit">元</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="item in receipt.items"
                :key="`${copy.key}-${item.key}`"
                class="item-row"
              >
                <td class="cell-name">
                  <div class="item-name">{{ item.name }}</div>
                  <div v-if="item.specText" class="item-spec">{{ item.specText }}</div>
                </td>
                <td class="cell-qty">{{ item.quantity }}</td>
                <td class="cell-unit">{{ item.unit }}</td>
                <td class="cell-price">{{ item.unitPriceText }}</td>
                <td
                  v-for="(digit, idx) in item.lineTotalDigits"
                  :key="`${item.key}-${idx}`"
                  class="cell-digit"
                >{{ digit }}</td>
                <td class="cell-note">{{ item.note }}</td>
              </tr>
              <tr
                v-for="i in emptyRowCount"
                :key="`empty-${copy.key}-${i}`"
                class="item-row empty-row"
              >
                <td class="cell-name">&nbsp;</td>
                <td class="cell-qty">&nbsp;</td>
                <td class="cell-unit">&nbsp;</td>
                <td class="cell-price">&nbsp;</td>
                <td class="cell-digit">&nbsp;</td>
                <td class="cell-digit">&nbsp;</td>
                <td class="cell-digit">&nbsp;</td>
                <td class="cell-digit">&nbsp;</td>
                <td class="cell-digit">&nbsp;</td>
                <td class="cell-digit">&nbsp;</td>
                <td class="cell-note">&nbsp;</td>
              </tr>
            </tbody>
          </table>

          <div class="delivery-total-row">
            <div class="total-amount">
              <span class="total-label">合计（大写）：</span>
              <span class="total-words">{{ receipt.summary.amountInWords }}</span>
              <span class="total-digits">
                <span class="digit-slot">拾</span>
                <span class="digit-slot">万</span>
                <span class="digit-slot">仟</span>
                <span class="digit-slot">佰</span>
                <span class="digit-slot">拾</span>
                <span class="digit-slot">元</span>
              </span>
            </div>
            <div class="total-operator">
              <span class="total-label">经手人：</span>
              <span class="total-value">{{ receipt.employeeName }}</span>
            </div>
          </div>

          <footer class="delivery-footer">
            <span>地址：{{ receipt.storeAddress }}</span>
            <span class="footer-divider">电话：{{ receipt.storePhone }}</span>
          </footer>
        </div>
      </section>
    </div>

    <template #footer>
      <div class="flex justify-between items-center gap-2">
        <span class="footer-tip text-xs text-slate-500">打印版式：A4 竖向，一页上下两联（客户联 / 店铺留存联）</span>
        <div class="flex gap-2">
          <el-button @click="visible = false">关闭</el-button>
          <el-button type="primary" @click="printReceipt">打印发货单</el-button>
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

// 两联：客户联（红） / 店铺留存联（白）
const receiptCopies = [
  { key: 'store',  tone: 'white', sideLines: ['第二联', '存根', '（白）'] },
  { key: 'customer', tone: 'red',  sideLines: ['第一联', '客户', '（红）'] }
]

const receiptItemCount = computed(() => props.receipt?.items?.length || 0)
const minItemRows = 6
const emptyRowCount = computed(() => Math.max(0, minItemRows - receiptItemCount.value))

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
.delivery-order-page {
  width: 820px;
  margin: 0 auto;
  padding: 12px;
  background: #f1f5f9;
  color: #0f172a;
  font-family: "Songti SC", "SimSun", "Microsoft YaHei", Arial, sans-serif;
}

.delivery-copy {
  position: relative;
  display: flex;
  margin-bottom: 14px;
  background: #fff;
  border: 1px solid #94a3b8;
  overflow: hidden;
  box-shadow: 0 6px 18px rgba(15, 23, 42, 0.08);
}

.delivery-copy:last-child {
  margin-bottom: 0;
}

.copy-side {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  gap: 6px;
  padding: 6px 6px;
  width: 32px;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 2px;
  writing-mode: vertical-rl;
  text-orientation: upright;
  border-right: 1px solid #94a3b8;
  background: #f8fafc;
  color: #0f172a;
}

.copy-side-line {
  display: block;
  line-height: 1.3;
}

.copy-red .copy-side {
  background: #fef2f2;
  color: #b91c1c;
}

.copy-white .copy-side {
  background: #f8fafc;
  color: #334155;
}

.delivery-content {
  flex: 1;
  padding: 10px 14px 8px;
}

.delivery-header {
  text-align: center;
  margin-bottom: 4px;
}

.delivery-title {
  font-size: 26px;
  font-weight: 900;
  letter-spacing: 6px;
  margin: 0;
  color: #0f172a;
}

.delivery-meta-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 2px 4px 6px;
  border-bottom: 1px solid #cbd5e1;
  margin-bottom: 6px;
  font-size: 13px;
}

.meta-date,
.meta-order-no {
  display: flex;
  align-items: baseline;
  gap: 4px;
}

.meta-label {
  color: #475569;
}

.meta-value {
  font-weight: 700;
}

.order-no-num {
  color: #dc2626;
  font-weight: 900;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 16px;
  letter-spacing: 1px;
}

.delivery-info-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0;
  border: 1px solid #cbd5e1;
  border-bottom: 0;
}

.delivery-info-row.single {
  grid-template-columns: 1fr;
}

.info-cell {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border-bottom: 1px solid #cbd5e1;
  border-right: 1px solid #cbd5e1;
  min-height: 32px;
}

.info-cell:last-child {
  border-right: 0;
}

.info-cell.wide {
  border-right: 0;
}

.info-label {
  flex-shrink: 0;
  color: #475569;
  font-weight: 700;
}

.info-value {
  font-weight: 700;
  color: #0f172a;
  word-break: break-word;
}

.items-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
  font-size: 12px;
  border: 1px solid #cbd5e1;
}

.items-table th,
.items-table td {
  border: 1px solid #cbd5e1;
  padding: 4px 4px;
  text-align: center;
  vertical-align: middle;
  word-break: break-word;
}

.items-table thead th {
  background: #f1f5f9;
  color: #0f172a;
  font-weight: 800;
  font-size: 12px;
  height: 26px;
}

.col-name  { width: 22%; }
.col-qty   { width: 6%; }
.col-unit  { width: 6%; }
.col-price { width: 8%; }
.col-digit { width: 4.5%; }
.col-note  { width: 13%; }

.cell-name {
  text-align: left;
  padding-left: 6px;
}

.cell-qty,
.cell-unit,
.cell-price {
  text-align: right;
  padding-right: 6px;
  font-variant-numeric: tabular-nums;
}

.cell-digit {
  height: 22px;
  font-variant-numeric: tabular-nums;
  font-weight: 700;
}

.cell-amount {
  letter-spacing: 2px;
}

.item-row .cell-name {
  text-align: left;
}

.item-name {
  font-weight: 700;
  color: #0f172a;
  line-height: 1.2;
}

.item-spec {
  font-size: 11px;
  color: #64748b;
  line-height: 1.2;
  margin-top: 1px;
}

.item-row.empty-row td {
  height: 22px;
  color: transparent;
}

.delivery-total-row {
  display: grid;
  grid-template-columns: 1fr 180px;
  border: 1px solid #cbd5e1;
  border-top: 0;
  font-size: 13px;
}

.total-amount,
.total-operator {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border-right: 1px solid #cbd5e1;
  min-height: 32px;
}

.total-operator {
  border-right: 0;
}

.total-label {
  color: #475569;
  font-weight: 700;
  flex-shrink: 0;
}

.total-words {
  flex: 1;
  font-weight: 900;
  color: #0f172a;
  letter-spacing: 1px;
}

.total-digits {
  display: inline-flex;
  gap: 2px;
  font-weight: 700;
  color: #475569;
}

.digit-slot {
  display: inline-block;
  min-width: 18px;
  padding: 1px 2px;
  border: 1px solid #cbd5e1;
  background: #f8fafc;
  text-align: center;
  font-variant-numeric: tabular-nums;
}

.total-value {
  flex: 1;
  font-weight: 700;
  color: #0f172a;
}

.delivery-footer {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 6px;
  padding: 6px 4px 0;
  font-size: 13px;
  font-weight: 700;
  color: #0f172a;
}

.footer-divider {
  margin-left: 6px;
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
    padding: 10px 8px 84px !important;
    background: #f1f5f9;
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

  .delivery-order-page {
    width: 100%;
    padding: 0;
    background: transparent;
  }

  .delivery-copy {
    border-radius: 0;
    box-shadow: none;
    margin-bottom: 10px;
  }

  .copy-side {
    width: 24px;
    font-size: 10px;
    letter-spacing: 1px;
  }

  .delivery-content {
    padding: 8px;
  }

  .delivery-title {
    font-size: 20px;
    letter-spacing: 3px;
  }

  .delivery-meta-row {
    font-size: 11px;
  }

  .order-no-num {
    font-size: 13px;
  }

  .delivery-info-row {
    grid-template-columns: 1fr;
  }

  .info-cell {
    border-right: 0;
  }

  .delivery-total-row {
    grid-template-columns: 1fr;
  }

  .total-amount {
    border-right: 0;
    border-bottom: 1px solid #cbd5e1;
  }

  .items-table {
    font-size: 10px;
  }

  .items-table th,
  .items-table td {
    padding: 3px 2px;
  }

  .col-name  { width: 24%; }
  .col-qty   { width: 7%; }
  .col-unit  { width: 7%; }
  .col-price { width: 9%; }
  .col-digit { width: 4.5%; }
  .col-note  { width: 12%; }
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
    padding: 8mm 8mm !important;
    background: #fff !important;
    box-shadow: none !important;
    overflow: hidden !important;
  }

  .delivery-order-page {
    width: 210mm;
    height: 297mm;
    padding: 0;
    background: #fff;
  }

  .delivery-copy {
    height: 138mm;
    margin: 0 0 6mm 0;
    border: 0.4mm solid #0f172a;
    box-shadow: none;
    break-inside: avoid;
    page-break-inside: avoid;
  }

  .delivery-copy:last-child {
    margin-bottom: 0;
  }

  .copy-side {
    width: 8mm;
    font-size: 10pt;
    padding: 6mm 0;
    background: #fff;
    color: #0f172a;
  }

  .copy-red .copy-side {
    background: #fff;
    color: #b91c1c;
  }

  .delivery-content {
    padding: 4mm 5mm;
  }

  .delivery-title {
    font-size: 18pt;
    letter-spacing: 4pt;
  }

  .delivery-meta-row,
  .delivery-info-row,
  .delivery-total-row,
  .delivery-footer {
    font-size: 9pt;
  }

  .order-no-num {
    font-size: 12pt;
  }

  .items-table {
    font-size: 8.5pt;
  }

  .items-table th,
  .items-table td {
    padding: 1.5mm 1.5mm;
  }

  .items-table thead th {
    height: 6mm;
    font-size: 9pt;
  }

  .item-row td {
    height: 6mm;
  }

  .digit-slot {
    min-width: 5mm;
    padding: 0.5mm 1mm;
  }

  .delivery-footer {
    font-size: 10pt;
  }
}

@page {
  size: A4 portrait;
  margin: 0;
}
</style>
