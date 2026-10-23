<template>
  <div class="min-h-screen space-y-4 pb-8 md:space-y-6">
    <header class="flex flex-col gap-3 rounded-2xl bg-white p-4 shadow-sm sm:flex-row sm:items-center sm:justify-between md:p-6">
      <div class="flex items-center gap-3">
        <el-button circle :icon="ArrowLeft" aria-label="返回收银终端" @click="router.push('/terminal')" />
        <div>
          <h1 class="text-xl font-black text-slate-900 md:text-2xl">库管</h1>
          <p class="mt-1 text-xs text-slate-400">通过纸质盲盘单清点门店与云仓实物库存</p>
        </div>
      </div>
      <div v-if="currentEmployee" class="flex items-center gap-2 self-end rounded-xl bg-blue-50 px-3 py-2 text-sm font-bold text-blue-700 sm:self-auto">
        <el-icon><User /></el-icon>{{ currentEmployee.name }}
      </div>
    </header>

    <el-tabs v-model="activeTab" class="rounded-2xl bg-white px-4 shadow-sm md:px-6" @tab-change="handleTabChange">
      <el-tab-pane label="商品盘库" name="stocktake">
        <div class="mb-3 flex flex-wrap justify-end gap-2">
          <StocktakeBatchManager
            v-if="currentEmployee"
            :warehouse-type="stocktakeWarehouse"
            :category-id="productFilters.category_id"
            :keyword="productKeyword"
            @inventory-updated="refreshAfterBatchStocktake"
            @blind-mode-change="handleBlindModeChange"
          />
          <el-button v-if="!blindStockMode" :disabled="products.length === 0" type="primary" plain @click="openBatchStocktake">批量盘点本页</el-button>
        </div>
        <div v-if="blindStockMode" class="mb-3 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-800">
          盲盘进行中，账面库存、库存流水和旧盘点入口已隐藏，请按纸质盘库单清点并录入。
        </div>
        <div class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5">
          <el-input
            v-model="productKeyword"
            clearable
            placeholder="搜索商品名称或型号"
            class="w-full lg:col-span-2"
            @input="handleProductSearch"
          >
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-select v-model="productFilters.category_id" clearable placeholder="全部分类" @change="handleProductFilter">
            <el-option v-for="category in categories" :key="category.id" :label="category.name" :value="category.id" />
          </el-select>
          <el-select v-if="!blindStockMode" v-model="productFilters.stock_filter" clearable placeholder="全部库存状态" @change="handleProductFilter">
            <el-option label="门店有库存" value="store" />
            <el-option label="云仓有库存" value="cloud" />
            <el-option label="门店和云仓均无库存" value="none" />
          </el-select>
          <el-select v-model="stocktakeWarehouse" aria-label="盘点仓库" placeholder="盘点仓库">
            <el-option label="盘点门店" :value="2" />
            <el-option label="盘点云仓" :value="3" />
          </el-select>
        </div>

        <el-table v-loading="productsLoading" :data="products" class="hidden md:block" stripe>
          <el-table-column label="商品" min-width="220">
            <template #default="{ row }">
              <div class="font-bold text-slate-800">{{ row.name }}</div>
              <div class="mt-1 text-xs text-slate-400">{{ row.remark || '无型号备注' }}</div>
            </template>
          </el-table-column>
          <el-table-column v-if="!blindStockMode" label="门店库存" width="120" align="center">
            <template #default="{ row }">{{ row.store_stock || 0 }}</template>
          </el-table-column>
          <el-table-column v-if="!blindStockMode" label="云仓实物库存" width="140" align="center">
            <template #default="{ row }">{{ row.cloud_stock || 0 }}</template>
          </el-table-column>
          <el-table-column label="操作" min-width="220" align="center">
            <template #default="{ row }">
              <div class="flex flex-wrap justify-center gap-1">
                <el-button v-if="!blindStockMode" type="primary" plain size="small" @click="openStocktake(row)">盘点</el-button>
                <el-button v-if="!blindStockMode" plain size="small" @click="openProductLogs(row)">流水</el-button>
              </div>
            </template>
          </el-table-column>
        </el-table>

        <div v-loading="productsLoading" class="grid grid-cols-1 gap-3 md:hidden">
          <article v-for="product in products" :key="product.id" class="rounded-xl border border-slate-100 p-4">
            <div class="mb-3">
              <div class="font-bold text-slate-800">{{ product.name }}</div>
              <div class="mt-1 text-xs text-slate-400">{{ product.remark || '无型号备注' }}</div>
            </div>
            <div v-if="!blindStockMode" class="mb-3 grid grid-cols-2 gap-2 text-center">
              <div class="rounded-lg bg-slate-50 p-2">
                <div class="text-[11px] text-slate-400">门店</div>
                <div class="font-black text-slate-700">{{ product.store_stock || 0 }}</div>
              </div>
              <div class="rounded-lg bg-slate-50 p-2">
                <div class="text-[11px] text-slate-400">云仓实物</div>
                <div class="font-black text-slate-700">{{ product.cloud_stock || 0 }}</div>
              </div>
            </div>
            <div v-if="!blindStockMode" class="grid grid-cols-2 gap-2">
              <el-button type="primary" @click="openStocktake(product)">盘点</el-button>
              <el-button @click="openProductLogs(product)">流水</el-button>
            </div>
          </article>
          <el-empty v-if="!productsLoading && products.length === 0" description="没有找到商品" />
        </div>

        <el-pagination
          v-if="productTotal > productPageSize"
          class="mt-4 justify-end"
          background
          layout="prev, pager, next"
          :current-page="productPage"
          :page-size="productPageSize"
          :total="productTotal"
          @current-change="changeProductPage"
        />
      </el-tab-pane>

      <el-tab-pane label="历史申请" name="applications">
        <el-empty v-if="blindStockMode" description="盲盘期间暂不显示可能包含库存信息的申请明细" />
        <template v-else>
        <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div class="text-sm text-slate-500">此处保留历史入库申请和盘点审核记录；当前库存数量统一通过盘点调整，记录请查看“库存流水”</div>
          <div class="flex items-center gap-2">
            <el-select v-model="applicationStatus" clearable placeholder="全部状态" class="!w-32" @change="refreshApplications">
              <el-option label="待审核" value="pending" />
              <el-option label="已通过" value="approved" />
              <el-option label="已拒绝" value="rejected" />
              <el-option label="需重新盘点" value="recount_required" />
            </el-select>
            <el-button :loading="applicationsLoading || stocktakeRequestsLoading" @click="refreshApplications">刷新</el-button>
          </div>
        </div>
        <el-table v-loading="applicationsLoading" :data="applications" class="hidden md:block" stripe>
          <el-table-column label="商品" min-width="200">
            <template #default="{ row }">
              <div class="font-bold text-slate-800">{{ row.product?.name || '商品已删除' }}</div>
              <div class="mt-1 text-xs text-slate-400">{{ row.product?.remark || '' }}</div>
            </template>
          </el-table-column>
          <el-table-column label="仓库 / 数量" width="150" align="center">
            <template #default="{ row }">{{ warehouseName(row.warehouse_type) }} · {{ row.quantity }}</template>
          </el-table-column>
          <el-table-column label="提交时间" width="170">
            <template #default="{ row }">{{ formatDate(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="状态" width="110" align="center">
            <template #default="{ row }"><el-tag :type="applicationStatusTag(row.status).type">{{ applicationStatusTag(row.status).label }}</el-tag></template>
          </el-table-column>
          <el-table-column label="备注 / 审核说明" min-width="220">
            <template #default="{ row }">
              <div>{{ row.remark || '无备注' }}</div>
              <div v-if="row.rejection_reason" class="mt-1 text-xs text-red-600">拒绝原因：{{ row.rejection_reason }}</div>
            </template>
          </el-table-column>
        </el-table>
        <div v-loading="applicationsLoading" class="grid grid-cols-1 gap-3 md:hidden">
          <article v-for="row in applications" :key="row.id" class="rounded-xl border border-slate-100 p-4">
            <div class="flex items-start justify-between gap-3">
              <div class="font-bold text-slate-800">{{ row.product?.name || '商品已删除' }}</div>
              <el-tag size="small" :type="applicationStatusTag(row.status).type">{{ applicationStatusTag(row.status).label }}</el-tag>
            </div>
            <div class="mt-2 text-sm text-slate-600">{{ warehouseName(row.warehouse_type) }} · 申请 {{ row.quantity }}</div>
            <div class="mt-1 text-xs text-slate-400">{{ formatDate(row.created_at) }}</div>
            <div class="mt-3 text-xs text-slate-500">{{ row.remark || '无备注' }}</div>
            <div v-if="row.rejection_reason" class="mt-2 text-xs text-red-600">拒绝原因：{{ row.rejection_reason }}</div>
          </article>
          <el-empty v-if="!applicationsLoading && applications.length === 0" description="暂无入库申请" />
        </div>
        <div class="mb-3 mt-8 flex items-center justify-between gap-3">
          <h2 class="text-base font-bold text-slate-800">盘点差异审核</h2>
          <span class="text-xs text-slate-400">历史盘点申请记录；新盘点直接调整库存并留存流水</span>
        </div>
        <el-table v-loading="stocktakeRequestsLoading" :data="stocktakeRequests" class="hidden md:block" stripe>
          <el-table-column label="商品 / 仓库" min-width="200">
            <template #default="{ row }"><div class="font-bold text-slate-800">{{ row.product?.name || '商品已删除' }}</div><div class="mt-1 text-xs text-slate-400">{{ warehouseName(row.warehouse_type) }}</div></template>
          </el-table-column>
          <el-table-column label="账面 → 实盘" width="140" align="center"><template #default="{ row }">{{ row.expected_stock }} → {{ row.actual_stock }}</template></el-table-column>
          <el-table-column label="差异" width="90" align="center"><template #default="{ row }"><span :class="row.difference < 0 ? 'text-red-600' : 'text-blue-600'" class="font-bold">{{ row.difference > 0 ? '+' : '' }}{{ row.difference }}</span></template></el-table-column>
          <el-table-column label="状态" width="100" align="center"><template #default="{ row }"><el-tag :type="applicationStatusTag(row.status).type">{{ applicationStatusTag(row.status).label }}</el-tag></template></el-table-column>
          <el-table-column label="盘点原因 / 审核说明" min-width="220"><template #default="{ row }"><div>{{ row.remark }}</div><div v-if="row.review_remark" class="mt-1 text-xs text-red-600">审核说明：{{ row.review_remark }}</div></template></el-table-column>
          <el-table-column label="提交时间" width="170"><template #default="{ row }">{{ formatDate(row.created_at) }}</template></el-table-column>
        </el-table>
        <el-empty v-if="!stocktakeRequestsLoading && stocktakeRequests.length === 0" description="暂无盘点审核记录" class="hidden md:block" />
        <div v-loading="stocktakeRequestsLoading" class="grid grid-cols-1 gap-3 md:hidden">
          <article v-for="row in stocktakeRequests" :key="row.id" class="rounded-xl border border-slate-100 p-4">
            <div class="flex items-start justify-between gap-3"><div class="font-bold text-slate-800">{{ row.product?.name || '商品已删除' }}</div><el-tag size="small" :type="applicationStatusTag(row.status).type">{{ applicationStatusTag(row.status).label }}</el-tag></div>
            <div class="mt-2 text-sm text-slate-600">{{ warehouseName(row.warehouse_type) }} · {{ row.expected_stock }} → {{ row.actual_stock }}</div>
            <div class="mt-2 text-xs text-slate-500">{{ row.remark }}</div>
            <div v-if="row.review_remark" class="mt-2 text-xs text-red-600">审核说明：{{ row.review_remark }}</div>
            <div class="mt-2 text-xs text-slate-400">{{ formatDate(row.created_at) }}</div>
          </article>
          <el-empty v-if="!stocktakeRequestsLoading && stocktakeRequests.length === 0" description="暂无盘点审核记录" />
        </div>
        </template>
      </el-tab-pane>

      <el-tab-pane v-if="!blindStockMode" label="库存流水" name="logs">
        <div class="mb-4 grid grid-cols-1 gap-2 sm:grid-cols-3">
          <el-input v-model="logFilters.keyword" clearable placeholder="搜索商品、备注或单号" @input="handleLogSearch">
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-select v-model="logFilters.log_type" clearable placeholder="全部变动类型" @change="resetAndFetchLogs">
            <el-option label="采购入库" value="purchase" />
            <el-option label="销售出库" value="sale" />
            <el-option label="库存调拨" value="transfer" />
            <el-option label="盘点调整" value="stocktake" />
            <el-option label="取消退库" value="cancel" />
          </el-select>
          <el-select v-model="logFilters.warehouse_type" clearable placeholder="全部仓库" @change="resetAndFetchLogs">
            <el-option label="门店" :value="2" />
            <el-option label="云仓" :value="3" />
            <el-option label="样机" :value="4" />
          </el-select>
        </div>

        <el-table v-loading="logsLoading" :data="logs" class="hidden md:block" stripe>
          <el-table-column label="商品 / 时间" min-width="210">
            <template #default="{ row }">
              <div class="font-bold text-slate-800">{{ row.product?.name || '未知商品' }}</div>
              <div class="mt-1 text-xs text-slate-400">{{ formatDate(row.created_at) }}</div>
            </template>
          </el-table-column>
          <el-table-column label="操作人" width="110">
            <template #default="{ row }">{{ row.operator_name || '历史记录' }}</template>
          </el-table-column>
          <el-table-column label="变动" width="120">
            <template #default="{ row }">
              <el-tag size="small" :type="logType(row.log_type).type" effect="plain">{{ logType(row.log_type).label }}</el-tag>
              <div class="mt-1 text-xs text-slate-400">{{ warehouseName(row.warehouse_type) }}</div>
            </template>
          </el-table-column>
          <el-table-column label="数量 / 库存" width="140" align="right">
            <template #default="{ row }">
              <div :class="row.change_qty > 0 ? 'text-blue-600' : 'text-red-500'" class="font-bold">
                {{ row.change_qty > 0 ? '+' : '' }}{{ row.change_qty }}
              </div>
              <div class="text-xs text-slate-400">{{ row.before_qty }} → {{ row.after_qty }}</div>
            </template>
          </el-table-column>
          <el-table-column label="备注 / 单号" min-width="180">
            <template #default="{ row }">
              <div class="truncate text-xs text-slate-600">{{ row.remark || '无备注' }}</div>
              <div v-if="row.related_no" class="mt-1 truncate font-mono text-[11px] text-slate-400">{{ row.related_no }}</div>
            </template>
          </el-table-column>
        </el-table>

        <div v-loading="logsLoading" class="grid grid-cols-1 gap-3 md:hidden">
          <article v-for="row in logs" :key="row.id" class="rounded-xl border border-slate-100 p-4">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="truncate font-bold text-slate-800">{{ row.product?.name || '未知商品' }}</div>
                <div class="mt-1 text-xs text-slate-400">{{ formatDate(row.created_at) }}</div>
              </div>
              <div class="text-right">
                <div :class="row.change_qty > 0 ? 'text-blue-600' : 'text-red-500'" class="font-black">
                  {{ row.change_qty > 0 ? '+' : '' }}{{ row.change_qty }}
                </div>
                <div class="text-[11px] text-slate-400">{{ row.before_qty }} → {{ row.after_qty }}</div>
              </div>
            </div>
            <div class="mt-3 flex flex-wrap items-center gap-2">
              <el-tag size="small" :type="logType(row.log_type).type" effect="plain">{{ logType(row.log_type).label }}</el-tag>
              <span class="text-xs text-slate-500">{{ warehouseName(row.warehouse_type) }}</span>
              <span class="ml-auto text-xs font-bold text-slate-600">{{ row.operator_name || '历史记录' }}</span>
            </div>
            <div class="mt-3 rounded-lg bg-slate-50 p-3 text-xs text-slate-500">{{ row.remark || '无备注' }}</div>
          </article>
          <el-empty v-if="!logsLoading && logs.length === 0" description="暂无库存流水" />
        </div>

        <el-pagination
          v-if="logTotal > logPageSize"
          class="mt-4 justify-end"
          background
          layout="prev, pager, next"
          :current-page="logPage"
          :page-size="logPageSize"
          :total="logTotal"
          @current-change="changeLogPage"
        />
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="stocktakeVisible" title="确认盘点" width="min(440px, calc(100vw - 24px))" destroy-on-close>
      <div v-if="selectedProduct" class="space-y-4">
        <div class="rounded-xl bg-slate-50 p-3">
          <div class="font-bold text-slate-800">{{ selectedProduct.name }}</div>
          <div class="mt-1 text-xs text-slate-500">
            {{ stocktakeWarehouse === 2 ? '门店' : '云仓实物' }}账面库存：{{ expectedStock }}
          </div>
        </div>
        <el-form label-position="top">
          <el-form-item label="实盘数量">
            <el-input-number v-model="actualStock" :min="0" :precision="0" size="large" class="!w-full" />
          </el-form-item>
          <el-form-item label="盘点性质">
            <el-radio-group v-model="stocktakeType">
              <el-radio :value="1">正常损耗</el-radio>
              <el-radio :value="2">业务调整</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="差异原因" :required="Number(actualStock) !== expectedStock">
            <el-input v-model="stocktakeRemark" maxlength="200" show-word-limit placeholder="数量与账面不一致时必填，例如：月末实物盘点" />
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <el-button @click="stocktakeVisible = false">取消</el-button>
        <el-button type="primary" :loading="stocktakeSubmitting" @click="submitStocktake">确认盘点</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="batchVisible" title="批量盘点本页商品" width="min(760px, calc(100vw - 24px))" destroy-on-close>
      <p class="mb-3 text-sm text-slate-500">请逐项填写实盘数量（包括账实相符的商品）；已填写 {{ batchProgress }} / {{ products.length }} 件。存在差异时需填写原因。</p>
      <el-table :data="products" max-height="50vh" stripe>
        <el-table-column label="商品" min-width="180">
          <template #default="{ row }"><div class="font-semibold">{{ row.name }}</div><div class="text-xs text-slate-400">{{ row.remark || '' }}</div></template>
        </el-table-column>
        <el-table-column label="账面" width="90" align="center"><template #default="{ row }">{{ stockFor(row, stocktakeWarehouse) }}</template></el-table-column>
        <el-table-column label="实盘数量" width="155" align="center">
          <template #default="{ row }"><el-input-number v-model="batchActuals[row.id]" :min="0" :precision="0" controls-position="right" /></template>
        </el-table-column>
      </el-table>
      <el-form class="mt-4" label-position="top">
        <el-form-item label="差异原因" :required="batchHasDifference"><el-input v-model="batchRemark" maxlength="200" show-word-limit placeholder="存在数量差异时必填，例如：月末实物盘点" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="batchVisible = false">取消</el-button>
        <el-button type="primary" :loading="batchSubmitting" @click="submitBatchStocktake">提交差异盘点</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="productLogsVisible" :title="`${productLogProduct?.name || '商品'} · 库存流水`" width="min(820px, calc(100vw - 24px))" destroy-on-close>
      <el-table v-loading="productLogsLoading" :data="productLogs" stripe>
        <el-table-column label="时间" min-width="155"><template #default="{ row }">{{ formatDate(row.created_at) }}</template></el-table-column>
        <el-table-column label="仓库" width="90"><template #default="{ row }">{{ warehouseName(row.warehouse_type) }}</template></el-table-column>
        <el-table-column label="操作人" width="100"><template #default="{ row }">{{ row.operator_name || '历史记录' }}</template></el-table-column>
        <el-table-column label="变化" width="100" align="right"><template #default="{ row }">{{ row.change_qty > 0 ? '+' : '' }}{{ row.change_qty }}</template></el-table-column>
        <el-table-column label="库存" width="115" align="center"><template #default="{ row }">{{ row.before_qty }} → {{ row.after_qty }}</template></el-table-column>
        <el-table-column label="备注" min-width="180"><template #default="{ row }">{{ row.remark || '无备注' }}</template></el-table-column>
      </el-table>
      <el-pagination v-if="productLogTotal > productLogPageSize" class="mt-4 justify-end" background layout="prev, pager, next, total" :current-page="productLogPage" :page-size="productLogPageSize" :total="productLogTotal" @current-change="changeProductLogPage" />
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft, Search, User } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import request from '../utils/request'
import StocktakeBatchManager from '../components/inventory/StocktakeBatchManager.vue'

const router = useRouter()
const currentEmployee = ref(null)
const activeTab = ref('stocktake')
const blindStockMode = ref(true)
const products = ref([])
const categories = ref([])
const productsLoading = ref(false)
const productKeyword = ref('')
const productPage = ref(1)
const productPageSize = 30
const productTotal = ref(0)
const productFilters = reactive({ category_id: '', stock_filter: '' })
const stocktakeWarehouse = ref(2)
const selectedProduct = ref(null)
const expectedStock = ref(0)
const stocktakeVisible = ref(false)
const stocktakeSubmitting = ref(false)
const actualStock = ref(0)
const stocktakeType = ref(1)
const stocktakeRemark = ref('')
const logs = ref([])
const logsLoading = ref(false)
const logPage = ref(1)
const logPageSize = 30
const logTotal = ref(0)
const logFilters = reactive({ keyword: '', log_type: '', warehouse_type: '' })
const applications = ref([])
const applicationsLoading = ref(false)
const applicationStatus = ref('')
const stocktakeRequests = ref([])
const stocktakeRequestsLoading = ref(false)
const batchVisible = ref(false)
const batchSubmitting = ref(false)
const batchActuals = reactive({})
const batchRemark = ref('')
const batchProgress = computed(() => products.value.filter((product) => batchActuals[product.id] !== undefined && batchActuals[product.id] !== null && batchActuals[product.id] !== '' && Number.isInteger(Number(batchActuals[product.id]))).length)
const batchHasDifference = computed(() => products.value.some((product) => {
  const actual = batchActuals[product.id]
  return actual !== undefined && actual !== null && actual !== '' && Number(actual) !== stockFor(product, stocktakeWarehouse.value)
}))
const productLogsVisible = ref(false)
const productLogsLoading = ref(false)
const productLogs = ref([])
const productLogProduct = ref(null)
const productLogPage = ref(1)
const productLogPageSize = 10
const productLogTotal = ref(0)

const hideProductStocks = (product) => {
  const safeProduct = { ...(product || {}) }
  delete safeProduct.main_stock
  delete safeProduct.store_stock
  delete safeProduct.sample_stock
  delete safeProduct.cloud_stock
  return safeProduct
}

const stockFor = (product, warehouseType) => Number(warehouseType === 2 ? product?.store_stock : product?.cloud_stock) || 0

const selectedWarehouseStock = computed(() => {
  if (!selectedProduct.value) return 0
  return Number(stocktakeWarehouse.value === 2 ? selectedProduct.value.store_stock : selectedProduct.value.cloud_stock) || 0
})

const readCurrentEmployee = () => {
  try {
    return JSON.parse(localStorage.getItem('pos_current_employee') || 'null')
  } catch {
    return null
  }
}

const fetchProducts = async () => {
  productsLoading.value = true
  try {
    const res = await request.get('/api/products', {
      params: {
        all: 1,
        keyword: productKeyword.value,
        category_id: productFilters.category_id,
        stock_filter: productFilters.stock_filter,
        page: productPage.value,
        page_size: productPageSize
      }
    })
    if (res.data.code === 200) {
      if (res.data.blind_stocktake_mode) handleBlindModeChange(true)
      const fetchedProducts = res.data.data || []
      products.value = blindStockMode.value ? fetchedProducts.map(hideProductStocks) : fetchedProducts
      productTotal.value = Number(res.data.total) || 0
    }
  } catch {
    ElMessage.error('获取商品库存失败')
  } finally {
    productsLoading.value = false
  }
}

const refreshAfterBatchStocktake = async () => {
  logPage.value = 1
  await Promise.all([fetchProducts(), blindStockMode.value ? Promise.resolve() : fetchLogs()])
}

const handleBlindModeChange = (enabled) => {
  const wasBlind = blindStockMode.value
  blindStockMode.value = Boolean(enabled)
  if (blindStockMode.value) {
    productFilters.stock_filter = ''
    stocktakeVisible.value = false
    batchVisible.value = false
    productLogsVisible.value = false
    stocktakeRequests.value = []
    applications.value = []
    logs.value = []
    productLogs.value = []
    logTotal.value = 0
    products.value = products.value.map(hideProductStocks)
    if (selectedProduct.value) selectedProduct.value = hideProductStocks(selectedProduct.value)
    if (activeTab.value === 'logs') activeTab.value = 'stocktake'
    return
  }
  if (wasBlind) {
    fetchProducts()
    if (activeTab.value === 'applications') refreshApplications()
    if (activeTab.value === 'logs') fetchLogs()
  }
}

const fetchCategories = async () => {
  try {
    const res = await request.get('/api/categories')
    if (res.data.code === 200) categories.value = res.data.data || []
  } catch {
    ElMessage.error('获取商品分类失败')
  }
}

const fetchLogs = async () => {
  if (blindStockMode.value) {
    logs.value = []
    logTotal.value = 0
    return
  }
  logsLoading.value = true
  try {
    const res = await request.get('/api/inventory/logs', {
      params: { ...logFilters, page: logPage.value, page_size: logPageSize }
    })
    if (res.data.code === 200) {
      logs.value = res.data.data || []
      logTotal.value = Number(res.data.total) || 0
    }
  } catch {
    ElMessage.error('获取库存流水失败')
  } finally {
    logsLoading.value = false
  }
}

const fetchMyApplications = async () => {
  if (blindStockMode.value) {
    applications.value = []
    return
  }
  applicationsLoading.value = true
  try {
    const res = await request.get('/api/pending-inbounds/mine', { params: { status: applicationStatus.value || undefined } })
    if (res.data.code === 200) applications.value = res.data.data || []
  } catch {
    ElMessage.error('获取入库申请失败')
  } finally {
    applicationsLoading.value = false
  }
}

const fetchMyStocktakeRequests = async () => {
  if (blindStockMode.value) {
    stocktakeRequests.value = []
    return
  }
  stocktakeRequestsLoading.value = true
  try {
    const res = await request.get('/api/pending-stocktakes/mine', { params: { status: applicationStatus.value || undefined } })
    if (res.data.code === 200) stocktakeRequests.value = res.data.data || []
  } catch {
    ElMessage.error('获取盘点审核进度失败')
  } finally {
    stocktakeRequestsLoading.value = false
  }
}

const refreshApplications = () => Promise.all([fetchMyApplications(), fetchMyStocktakeRequests()])

const handleTabChange = (tabName) => {
  if (blindStockMode.value) return
  if (tabName === 'applications') refreshApplications()
  if (tabName === 'logs') fetchLogs()
}

let productSearchTimer
const handleProductSearch = () => {
  clearTimeout(productSearchTimer)
  productSearchTimer = setTimeout(() => {
    productPage.value = 1
    fetchProducts()
  }, 300)
}

const handleProductFilter = () => {
  productPage.value = 1
  fetchProducts()
}

let logSearchTimer
const handleLogSearch = () => {
  clearTimeout(logSearchTimer)
  logSearchTimer = setTimeout(() => {
    logPage.value = 1
    fetchLogs()
  }, 300)
}

const resetAndFetchLogs = () => {
  logPage.value = 1
  fetchLogs()
}

const changeProductPage = (page) => {
  productPage.value = page
  fetchProducts()
}

const changeLogPage = (page) => {
  logPage.value = page
  fetchLogs()
}

const openStocktake = (product) => {
  if (blindStockMode.value) return
  selectedProduct.value = product
  actualStock.value = selectedWarehouseStock.value
  expectedStock.value = selectedWarehouseStock.value
  stocktakeType.value = 1
  stocktakeRemark.value = ''
  stocktakeVisible.value = true
}

const submitStocktake = async () => {
  if (blindStockMode.value) return
  if (!selectedProduct.value || stocktakeSubmitting.value) return
  const nextStock = Number(actualStock.value)
  if (!Number.isInteger(nextStock) || nextStock < 0) {
    ElMessage.warning('实盘数量必须是大于等于 0 的整数')
    return
  }

  const warehouseLabel = stocktakeWarehouse.value === 2 ? '门店' : '云仓'
  const difference = nextStock - expectedStock.value
  if (difference !== 0 && !stocktakeRemark.value.trim()) {
    ElMessage.warning('盘点数量有差异时请填写差异原因')
    return
  }
  try {
    await ElMessageBox.confirm(
      `确定将「${selectedProduct.value.name}」的${warehouseLabel}实物库存从 ${expectedStock.value} 调整为 ${nextStock}？`,
      '确认库存盘点',
      { confirmButtonText: '确认盘点', cancelButtonText: '返回修改', type: difference < 0 ? 'warning' : 'info' }
    )
  } catch {
    return
  }

  stocktakeSubmitting.value = true
  try {
    const res = await request.post(`/api/products/${selectedProduct.value.id}/stocktake`, {
      warehouse_type: stocktakeWarehouse.value,
      actual_stock: nextStock,
      expected_stock: expectedStock.value,
      stocktake_type: stocktakeType.value,
      remark: stocktakeRemark.value.trim()
    })
    if (res.data.code !== 200) {
      ElMessage.error(res.data.msg || '盘点失败')
      return
    } else {
      ElMessage.success(`盘点完成，库存差异 ${res.data.difference || 0}`)
    }
    stocktakeVisible.value = false
    logPage.value = 1
    await Promise.all([fetchProducts(), fetchLogs()])
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '盘点失败')
  } finally {
    stocktakeSubmitting.value = false
  }
}

const openBatchStocktake = () => {
  if (blindStockMode.value) return
  Object.keys(batchActuals).forEach((key) => delete batchActuals[key])
  products.value.forEach((product) => { batchActuals[product.id] = undefined })
  batchRemark.value = ''
  batchVisible.value = true
}

const submitBatchStocktake = async () => {
  if (blindStockMode.value) return
  if (batchSubmitting.value) return
  const uncounted = products.value.filter((product) => batchActuals[product.id] === undefined || batchActuals[product.id] === null || batchActuals[product.id] === '' || !Number.isInteger(Number(batchActuals[product.id])))
  if (uncounted.length > 0) {
    ElMessage.warning(`还有 ${uncounted.length} 件商品未填写实盘数量`)
    return
  }
  if (batchHasDifference.value && !batchRemark.value.trim()) {
    ElMessage.warning('请填写本次盘点的差异原因')
    return
  }
  batchSubmitting.value = true
  let accepted = 0
  const failed = []
  for (const product of products.value) {
    try {
      const res = await request.post(`/api/products/${product.id}/stocktake`, {
        warehouse_type: stocktakeWarehouse.value,
        actual_stock: Number(batchActuals[product.id]),
        expected_stock: stockFor(product, stocktakeWarehouse.value),
        stocktake_type: 1,
        remark: batchRemark.value.trim()
      })
      if (res.data.code === 200) accepted += 1
      else failed.push(`${product.name}：${res.data.msg || '提交失败'}`)
    } catch (error) {
      failed.push(`${product.name}：${error.response?.data?.msg || '提交失败'}`)
    }
  }
  batchSubmitting.value = false
  await Promise.all([fetchProducts(), fetchLogs()])
  if (failed.length === 0) {
    ElMessage.success(`${accepted} 件商品已完成盘点，库存变动已即时记录`)
    batchVisible.value = false
  } else {
    ElMessageBox.alert(`已提交 ${accepted} 件，失败 ${failed.length} 件：\n${failed.join('\n')}`, '批量盘点结果', { type: 'warning' })
  }
}

const openProductLogs = (product) => {
  if (blindStockMode.value) return
  productLogProduct.value = product
  productLogPage.value = 1
  productLogsVisible.value = true
  fetchProductLogs()
}

const fetchProductLogs = async () => {
  if (blindStockMode.value || !productLogProduct.value) return
  productLogsLoading.value = true
  try {
    const res = await request.get(`/api/products/${productLogProduct.value.id}/logs`, {
      params: { page: productLogPage.value, page_size: productLogPageSize }
    })
    if (res.data.code === 200) {
      productLogs.value = res.data.data || []
      productLogTotal.value = Number(res.data.total) || 0
    }
  } catch {
    ElMessage.error('获取商品库存流水失败')
  } finally {
    productLogsLoading.value = false
  }
}

const changeProductLogPage = (page) => {
  productLogPage.value = page
  fetchProductLogs()
}

const formatDate = (value) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '-'
const warehouseName = (type) => ({ 1: '主仓', 2: '门店', 3: '云仓', 4: '样机' })[type] || '未知仓库'
const applicationStatusTag = (status) => ({
  pending: { label: '待审核', type: 'warning' },
  approved: { label: '已通过', type: 'success' },
  rejected: { label: '已拒绝', type: 'danger' },
  recount_required: { label: '需重新盘点', type: 'warning' }
})[status] || { label: status || '未知', type: 'info' }
const logType = (type) => ({
  purchase: { label: '采购入库', type: 'success' },
  sale: { label: '销售出库', type: 'primary' },
  transfer: { label: '库存调拨', type: 'warning' },
  stocktake: { label: '盘点调整', type: 'info' },
  cancel: { label: '取消退库', type: 'danger' }
})[type] || { label: type || '其他', type: 'info' }

onMounted(() => {
  currentEmployee.value = readCurrentEmployee()
  if (!localStorage.getItem('pos_token') || !currentEmployee.value) {
    router.replace('/terminal')
    return
  }
  fetchProducts()
  fetchCategories()
  fetchLogs()
  refreshApplications()
})

onUnmounted(() => {
  clearTimeout(productSearchTimer)
  clearTimeout(logSearchTimer)
})
</script>
