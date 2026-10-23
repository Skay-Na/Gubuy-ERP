<script setup>
import { computed } from 'vue'

const props = defineProps({
  batch: { type: Object, default: null }
})

const itemsPerPage = 20
const pages = computed(() => {
  const items = props.batch?.items || []
  const result = []
  for (let index = 0; index < items.length; index += itemsPerPage) {
    result.push(items.slice(index, index + itemsPerPage))
  }
  return result
})

const warehouseName = (type) => ({ 2: '门店', 3: '云仓' })[type] || '未知仓库'
</script>

<template>
  <Teleport to="body">
    <div v-if="batch" class="stocktake-print-root">
      <section
        v-for="(pageItems, pageIndex) in pages"
        :key="`${batch.id}-${pageIndex}`"
        class="stocktake-print-page"
        :class="{ 'stocktake-print-page-last': pageIndex === pages.length - 1 }"
      >
        <header class="stocktake-print-header">
          <div>
            <h1>员工实物盘库单（盲盘）</h1>
            <p>{{ batch.batch_no }}　·　{{ warehouseName(batch.warehouse_type) }}　·　{{ batch.scope_label }}</p>
            <p v-if="batch.print_count > 0" class="stocktake-print-reprint">补打第 {{ batch.print_count + 1 }} 次，请作废旧单</p>
          </div>
          <div class="stocktake-print-meta">
            <span>盘点人：{{ batch.operator_name || '____________' }}</span>
            <span>盘点日期：____________</span>
            <span>复核人：____________</span>
          </div>
        </header>

        <table class="stocktake-print-table">
          <thead>
            <tr>
              <th class="stocktake-print-index">序号</th>
              <th class="stocktake-print-code">商品编号</th>
              <th>商品名称 / 型号备注</th>
              <th class="stocktake-print-actual">实盘数量</th>
              <th class="stocktake-print-remark">现场备注</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(item, itemIndex) in pageItems" :key="item.id">
              <td class="stocktake-print-center">{{ pageIndex * itemsPerPage + itemIndex + 1 }}</td>
              <td class="stocktake-print-center">{{ item.product_id }}</td>
              <td>
                <strong>{{ item.product_name }}</strong>
                <small v-if="item.product_remark">{{ item.product_remark }}</small>
              </td>
              <td></td>
              <td></td>
            </tr>
          </tbody>
        </table>

        <footer class="stocktake-print-footer">
          <p>填写 0 表示已盘且无库存；空白表示尚未清点。发现破损、缺件等情况，请记入现场备注；数量差异原因在系统录入时补充。</p>
          <div class="stocktake-print-signatures">
            <span>盘点人签字：________________</span>
            <span>复核人签字：________________</span>
            <span>第 {{ pageIndex + 1 }} / {{ pages.length }} 页</span>
          </div>
        </footer>
      </section>
    </div>
  </Teleport>
</template>

<style>
.stocktake-print-root {
  display: none;
  color: #111;
  font-family: "Noto Sans CJK SC", "Microsoft YaHei", sans-serif;
}

.stocktake-print-page {
  page: stocktakePage;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  width: 190mm;
  min-height: 276mm;
  margin: 0 auto 12px;
  padding: 0;
  background: white;
  color: #111;
  font-size: 11px;
}

.stocktake-print-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 10px;
  border-bottom: 1px solid #111;
  padding-bottom: 8px;
}

.stocktake-print-header h1 {
  margin: 0 0 5px;
  font-size: 20px;
  letter-spacing: 2px;
}

.stocktake-print-header p,
.stocktake-print-footer p {
  margin: 0;
}

.stocktake-print-reprint {
  margin-top: 4px !important;
  font-weight: 700;
}

.stocktake-print-meta {
  display: grid;
  gap: 5px;
  min-width: 125px;
  font-size: 10px;
}

.stocktake-print-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}

.stocktake-print-table th,
.stocktake-print-table td {
  border: 1px solid #333;
  padding: 4px 6px;
  vertical-align: middle;
}

.stocktake-print-table th {
  height: 8mm;
  background: #f2f2f2;
  text-align: center;
  font-weight: 700;
}

.stocktake-print-table td {
  height: 8mm;
}

.stocktake-print-table small {
  display: block;
  margin-top: 2px;
  color: #444;
  font-size: 9px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stocktake-print-index { width: 11mm; }
.stocktake-print-code { width: 20mm; }
.stocktake-print-actual { width: 27mm; }
.stocktake-print-remark { width: 42mm; }
.stocktake-print-center { text-align: center; }

.stocktake-print-footer {
  margin-top: auto;
  padding-top: 8px;
  font-size: 10px;
}

.stocktake-print-signatures {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-top: 13px;
}

@page stocktakePage {
  size: A4 portrait;
  margin: 10mm;
}

@media print {
  body.stocktake-printing * {
    visibility: hidden !important;
  }

  body.stocktake-printing .stocktake-print-root,
  body.stocktake-printing .stocktake-print-root * {
    visibility: visible !important;
  }

  body.stocktake-printing .stocktake-print-root {
    position: absolute !important;
    top: 0 !important;
    left: 0 !important;
    display: block !important;
    width: 100% !important;
    background: white !important;
  }

  .stocktake-print-page {
    width: 100% !important;
    min-height: 276mm !important;
    margin: 0 !important;
    page-break-after: always;
    break-after: page;
  }

  .stocktake-print-page-last {
    page-break-after: auto;
    break-after: auto;
  }

  .stocktake-print-table tr {
    break-inside: avoid;
    page-break-inside: avoid;
  }
}
</style>
