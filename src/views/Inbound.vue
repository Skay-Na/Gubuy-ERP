<template>
  <div class="min-h-screen bg-gray-50 p-3 md:p-6">
    <div class="max-w-7xl mx-auto space-y-6">
      <!-- 移动端：审核和库存查询是两个平级工作台，避免桌面表格压缩后低效。 -->
      <section class="md:hidden mb-4">
        <div class="mb-3 flex items-center justify-between">
          <div>
            <h2 class="text-lg font-black text-slate-900">入库工作台</h2>
            <p class="text-xs text-slate-500">先处理待审核，再查询库存</p>
          </div>
          <el-button plain size="small" class="!h-9 !rounded-lg !font-bold" @click="mobileManualEntryOpen = !mobileManualEntryOpen">
            {{ mobileManualEntryOpen ? '收起录入' : '手工入库' }}
          </el-button>
        </div>

        <div class="grid grid-cols-2 rounded-xl bg-slate-100 p-1">
          <button class="mobile-workbench-tab" :class="mobilePanel === 'pending' ? 'is-active' : ''" @click="mobilePanel = 'pending'">
            待审核 <b v-if="pendingInbounds.length" class="ml-1 rounded-full bg-amber-500 px-1.5 py-0.5 text-[10px] text-white">{{ pendingInbounds.length }}</b>
          </button>
          <button class="mobile-workbench-tab" :class="mobilePanel === 'stock' ? 'is-active' : ''" @click="mobilePanel = 'stock'">
            实时库存 <span class="ml-1 text-[10px] text-slate-400">{{ allProducts.length }}</span>
          </button>
        </div>

        <div v-if="mobilePanel === 'pending'" class="mt-3 space-y-2">
          <article v-for="row in pendingInbounds" :key="row.id" class="rounded-xl border border-slate-200 bg-white p-3 shadow-sm">
            <div class="grid grid-cols-[minmax(0,1fr)_auto] gap-3">
              <div class="min-w-0">
                <h3 class="break-all text-sm font-black leading-5 text-slate-900">{{ row.product?.name || '商品已删除' }}</h3>
                <p class="mt-1 truncate text-[11px] text-slate-500">{{ row.product?.remark || '无商品备注' }}</p>
              </div>
              <div class="text-right"><b class="block text-lg leading-5 text-blue-700">+{{ row.quantity }}</b><span class="text-[10px] text-slate-400">{{ warehouseName(row.warehouse_type) }}</span></div>
            </div>
            <div class="mt-2 flex items-center justify-between gap-2 text-[11px] text-slate-500"><span class="truncate">{{ row.submitter_name || '—' }} · {{ row.remark || '无备注' }}</span><span class="shrink-0">{{ formatDate(row.created_at) }}</span></div>
            <div class="mt-3 grid grid-cols-[72px_1fr] gap-2">
              <el-button size="small" plain type="danger" class="!m-0 !h-9 !rounded-lg" @click="rejectPending(row)">拒绝</el-button>
              <el-button size="small" type="primary" class="!m-0 !h-9 !rounded-lg !font-bold" @click="openApproveDialog(row)">填写成本并入库</el-button>
            </div>
          </article>
          <el-empty v-if="!pendingInbounds.length" description="暂无待审核申请" :image-size="62" class="rounded-xl bg-white py-2 shadow-sm" />
        </div>

        <div v-else class="mt-3 rounded-xl border border-slate-200 bg-white p-3 shadow-sm">
          <el-input v-model="mobileStockKeyword" clearable placeholder="搜索型号或备注" />
          <div class="mt-2 max-h-[55vh] space-y-1 overflow-y-auto">
            <button v-for="row in mobileStockRows" :key="row.id" class="mobile-stock-row" @click="preFillForm(row); mobileManualEntryOpen = true">
              <span class="min-w-0 text-left"><b class="block break-all text-sm leading-5 text-slate-800">{{ row.name }}</b><small>{{ row.remark || '无备注' }}</small></span>
              <span class="flex shrink-0 gap-1 text-[10px]"><i>店 {{ row.store_stock }}</i><i>云 {{ row.cloud_stock || 0 }}</i></span>
            </button>
            <el-empty v-if="!mobileStockRows.length" description="没有匹配的商品" :image-size="62" />
          </div>
        </div>
      </section>

      <el-row :gutter="24" class="!mx-0">
        <!-- 左侧：录入表单 -->
        <el-col v-show="!isMobile || mobileManualEntryOpen" :xs="24" :sm="24" :md="8" :lg="8" class="!px-0 md:!px-3 mb-6 md:mb-0">
          <div class="bg-white p-6 rounded-2xl shadow-sm border border-slate-100 sticky top-6">
            <h3 class="text-lg font-bold text-slate-700 mb-6 flex items-center gap-2">
              <el-icon color="#409eff"><Download /></el-icon>
              新增入库登记
            </h3>
         
            <el-form :model="form" label-position="top" ref="formRef" :rules="rules">
              <el-form-item label="选择商品" prop="product_id">
                <el-select
                  v-model="form.product_id"
                  placeholder="搜索并选择商品"
                  class="w-full"
                  filterable
                >
                  <el-option
                    v-for="item in allProducts"
                    :key="item.id"
                    :label="item.name"
                    :value="item.id"
                  >
                    <div class="flex justify-between items-center">
                      <span class="text-sm">{{ item.name }}</span>
                      <span class="text-gray-400 text-[10px] ml-2">备注: {{ item.remark || '-' }}</span>
                    </div>
                  </el-option>
                </el-select>
              </el-form-item>

              <div class="grid grid-cols-2 gap-4">
                <el-form-item label="入库数量" prop="quantity">
                  <el-input-number
                    v-model="form.quantity"
                    :min="1"
                    class="!w-full"
                    controls-position="right"
                  />
                </el-form-item>

                <el-form-item label="进货单价" prop="unit_cost">
                  <el-input-number
                    v-model="form.unit_cost"
                    :min="0"
                    :precision="2"
                    class="!w-full"
                    controls-position="right"
                    placeholder="0.00"
                  />
                </el-form-item>
              </div>

              <el-form-item label="入库仓库" prop="warehouse_type">
                <div class="grid grid-cols-2 gap-3 w-full">
                  <div
                    v-for="item in [
                      { label: '门店', value: 2, icon: 'Shop' },
                      { label: '云仓', value: 3, icon: 'Cloudy' }
                    ]"
                    :key="item.value"
                    @click="form.warehouse_type = item.value"
                    class="flex flex-col items-center justify-center p-3 rounded-xl border-2 transition-all cursor-pointer group"
                    :class="form.warehouse_type === item.value
                      ? 'border-blue-600 bg-blue-50 text-blue-600 shadow-sm'
                      : 'border-slate-100 text-slate-400 hover:border-blue-200 hover:bg-slate-50'"
                  >
                    <el-icon size="20" class="mb-1.5 transition-transform group-hover:scale-110">
                      <component :is="item.icon" />
                    </el-icon>
                    <span class="text-xs font-bold">{{ item.label }}</span>
                  </div>
                </div>
              </el-form-item>

              <el-form-item label="成本规则">
                <div class="w-full rounded-xl border border-blue-100 bg-blue-50/60 p-3 text-sm text-blue-800">
                  入库只记录库存和成本，不扣资金账户；商品售出后按订单成本快照计算利润。
                </div>
              </el-form-item>

              <div class="mt-8 p-4 bg-blue-50/50 rounded-2xl border border-blue-100 mb-6">
                <div class="flex justify-between items-center">
                  <span class="text-sm text-blue-700 font-medium">本批库存成本</span>
                  <span class="text-xl font-bold text-blue-700">¥ {{ totalCost.toFixed(2) }}</span>
                </div>
              </div>

              <el-button
                type="primary"
                class="w-full !h-12 !text-lg !rounded-2xl shadow-lg shadow-blue-500/20 font-bold"
                :loading="submitting"
                @click="submitInbound"
              >
                确认入库
              </el-button>
            </el-form>
          </div>
        </el-col>

        <!-- 右侧：商品大盘 -->
        <el-col :xs="24" :sm="24" :md="16" :lg="16" class="!px-0 md:!px-3 hidden md:block">
          <div class="bg-white p-6 rounded-2xl shadow-sm border border-slate-100">
            <el-tabs v-model="activeTab" class="!mb-0">
              <el-tab-pane :label="`实时库存大盘（${allProducts.length}）`" name="stock">
                <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center mb-6 gap-4">
              <h3 class="text-lg font-bold text-slate-700">实时库存大盘</h3>
              <el-input
                v-model="searchKeyword"
                placeholder="搜索名称或备注..."
                prefix-icon="Search"
                class="!w-full sm:!w-64"
                clearable
                @input="fetchProducts"
              />
            </div>

            <!-- PC View -->
            <el-table
              :data="allProducts"
              stripe
              style="width: 100%"
              header-cell-class-name="bg-slate-50 text-slate-600 font-bold"
              class="desktop-only hidden md:block"
            >
              <el-table-column prop="name" label="商品名称" min-width="150" />
              <el-table-column label="商品备注" width="120">
                <template #default="{ row }">
                  {{ row.remark || '-' }}
                </template>
              </el-table-column>
              <el-table-column label="库存详情" width="120" align="center">
                <template #default="scope">
                  <div class="flex flex-col gap-1 text-[10px]">
                    <div class="flex justify-between border-b border-slate-50">
                      <span>店:</span> <span class="font-bold">{{ scope.row.store_stock }}</span>
                    </div>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="最新进价" width="120" align="right">
                <template #default="scope">
                  <span class="font-mono font-bold text-blue-600">¥{{ (scope.row.latest_cost || 0).toFixed(2) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="100" align="center">
                <template #default="scope">
                  <el-button type="primary" link @click="preFillForm(scope.row)">快速填入</el-button>
                </template>
              </el-table-column>
            </el-table>

            <!-- Mobile View -->
            <div class="mobile-only md:hidden space-y-3">
              <div v-for="row in allProducts" :key="row.id" class="p-3 bg-slate-50 rounded-xl flex justify-between items-center">
                <div class="flex-1 min-w-0 mr-4">
                  <div class="font-bold text-slate-800 text-sm truncate">{{ row.name }}</div>
                  <div class="text-[10px] text-slate-400 mt-1">
                    备注: {{ row.remark || '-' }} | 店:{{ row.store_stock }} 云:{{ row.cloud_stock || 0 }}
                  </div>
                </div>
                <el-button type="primary" plain size="small" @click="preFillForm(row)" class="!rounded-lg">填入</el-button>
              </div>
              <el-empty v-if="allProducts.length === 0" description="未找到商品" />
            </div>
              </el-tab-pane>
           
              <el-tab-pane :label="`待审核入库${pendingInbounds.length ? `（${pendingInbounds.length}）` : ''}`" name="pending">
                <div v-if="pendingInbounds.length" class="mb-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm font-bold text-amber-800">
                  有 {{ pendingInbounds.length }} 条员工入库申请等待你确认成本；通过后只更新库存，不扣资金账户。
                </div>
                <el-empty v-if="!pendingInbounds.length" description="暂无待审核入库申请" :image-size="80" />
                <el-table v-else :data="pendingInbounds" stripe style="width: 100%" header-cell-class-name="bg-slate-50 text-slate-600 font-bold">
                  <el-table-column prop="product.name" label="商品名称" min-width="150" />
                  <el-table-column label="仓库" width="80">
                    <template #default="scope">
                      {{ ['未知', '主仓', '门店', '云仓'][scope.row.warehouse_type] || scope.row.warehouse_type }}
                    </template>
                  </el-table-column>
                  <el-table-column prop="quantity" label="数量" width="80" align="center" />
                  <el-table-column prop="submitter_name" label="提交人" width="100" />
                  <el-table-column prop="remark" label="备注" min-width="120" />
                  <el-table-column prop="created_at" label="日期" width="160">
                    <template #default="scope">
                      {{ new Date(scope.row.created_at).toLocaleString() }}
                    </template>
                  </el-table-column>
                  <el-table-column label="操作" width="120" align="center" fixed="right">
                    <template #default="scope">
                      <el-button type="success" link @click="openApproveDialog(scope.row)">通过</el-button>
                      <el-button type="danger" link @click="rejectPending(scope.row)">拒绝</el-button>
                    </template>
                  </el-table-column>
                </el-table>
              </el-tab-pane>
            </el-tabs>
          </div>
        </el-col>
      </el-row>

      <el-dialog v-model="approveDialogVisible" title="确认入库申请" width="400px" class="approval-dialog">
        <el-form :model="approveForm" :rules="approveRules" ref="approveFormRef" label-position="top">
          <el-form-item label="入库单价" prop="unit_cost">
            <el-input-number v-model="approveForm.unit_cost" :min="0" :precision="2" class="!w-full" controls-position="right" />
          </el-form-item>
          <el-alert title="这里只登记库存成本，不会扣除任何资金账户。商品售出后，订单成本快照用于利润核算。" type="info" :closable="false" show-icon />
        </el-form>
        <template #footer>
          <div class="dialog-footer">
            <el-button @click="approveDialogVisible = false">取消</el-button>
            <el-button type="primary" :loading="approveSubmitting" @click="submitApprove">确认入库</el-button>
          </div>
        </template>
      </el-dialog>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { Download, Search, Shop, Cloudy } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { fetchProducts as fetchProductsApi } from '../api/products'
import {
  createInbound,
  fetchPendingInbounds as fetchPendingInboundsApi,
  approvePendingInbound,
  rejectPendingInbound
} from '../api/inventory'

const allProducts = ref([])
const searchKeyword = ref('')
const submitting = ref(false)
const formRef = ref(null)
const isMobile = ref(typeof window !== 'undefined' && window.innerWidth < 768)
const mobileManualEntryOpen = ref(false)
const mobilePanel = ref('pending')
const mobileStockKeyword = ref('')

const activeTab = ref('stock')
const pendingInbounds = ref([])
const approveDialogVisible = ref(false)
const approveFormRef = ref(null)
const currentPendingRow = ref(null)
const approveSubmitting = ref(false)

const approveForm = ref({ unit_cost: 0 })

const approveRules = {
  unit_cost: [{ required: true, message: '请输入入库单价', trigger: 'blur' }]
}

const form = ref({
  product_id: null,
  quantity: 1,
  unit_cost: undefined,
  warehouse_type: 3
})

const rules = {
  product_id: [{ required: true, message: '请选择入库商品', trigger: 'change' }],
  quantity: [{ required: true, message: '请输入入库数量', trigger: 'blur' }],
  unit_cost: [{ required: true, message: '请输入进货单价', trigger: 'blur' }],
  warehouse_type: [{ required: true, message: '请选择入库仓库', trigger: 'change' }]
}

const totalCost = computed(() => {
  return (form.value.quantity || 0) * (form.value.unit_cost || 0)
})
const mobileStockRows = computed(() => {
  const keyword = mobileStockKeyword.value.trim().toLowerCase()
  if (!keyword) return allProducts.value
  return allProducts.value.filter(row => `${row.name || ''} ${row.remark || ''}`.toLowerCase().includes(keyword))
})

const warehouseName = (warehouseType) => ({ 1: '主仓', 2: '门店', 3: '云仓' }[warehouseType] || '未知仓库')
const formatDate = (value) => value ? new Date(value).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '—'
const syncViewport = () => {
  isMobile.value = window.innerWidth < 768
  if (!isMobile.value) mobileManualEntryOpen.value = false
}

const fetchProducts = async () => {
  try {
    const response = await fetchProductsApi({
      // 入库页必须能查到零库存商品，与员工增库时的商品范围保持一致。
      keyword: searchKeyword.value, all: 1
    })
    if (response.data.code === 200) {
      allProducts.value = response.data.data
    }
  } catch (error) {
    console.error('Fetch products error:', error)
    ElMessage.error('获取商品列表失败')
  }
}

const submitInbound = async () => {
  if (!formRef.value) return

  await formRef.value.validate(async (valid) => {
    if (valid) {
      submitting.value = true
      try {
        const response = await createInbound({
          product_id: form.value.product_id,
          quantity: form.value.quantity,
          unit_cost: form.value.unit_cost,
          warehouse_type: form.value.warehouse_type
        })
     
        if (response.data.code === 200) {
          ElMessage.success('商品入库成功！')
          resetForm()
          fetchProducts() // 重新查询刷新列表
        } else {
          ElMessage.error(response.data.msg || '入库失败')
        }
      } catch (error) {
        console.error('Inbound submission error:', error)
        ElMessage.error(error.response?.data?.msg || '网络提交异常')
      } finally {
        submitting.value = false
      }
    }
  })
}

const resetForm = () => {
  form.value = {
    product_id: null,
    quantity: 1,
    unit_cost: undefined,
    warehouse_type: 3
  }
  if (formRef.value) formRef.value.resetFields()
}

const preFillForm = (product) => {
  form.value.product_id = product.id
  form.value.unit_cost = product.latest_cost
  ElMessage.info(`已填入 ${product.name} 的信息`)
}

const fetchPendingInbounds = async () => {
  try {
    const response = await fetchPendingInboundsApi()
    if (response.data.code === 200) {
      pendingInbounds.value = response.data.data || []
      if (pendingInbounds.value.length > 0) {
        activeTab.value = 'pending'
        mobilePanel.value = 'pending'
      }
    }
  } catch (error) {
    console.error('Fetch pending inbounds error:', error)
  }
}

const openApproveDialog = (row) => {
  currentPendingRow.value = row
  approveForm.value.unit_cost = row.product?.latest_cost || 0
  approveDialogVisible.value = true
}

const submitApprove = async () => {
  if (!approveFormRef.value) return
  await approveFormRef.value.validate(async (valid) => {
    if (valid) {
      approveSubmitting.value = true
      try {
        const response = await approvePendingInbound(currentPendingRow.value.id, {
          unit_cost: approveForm.value.unit_cost
        })
        if (response.data.code === 200) {
          ElMessage.success('申请已通过并入库')
          approveDialogVisible.value = false
          fetchPendingInbounds()
          fetchProducts()
        } else {
          ElMessage.error(response.data.msg || '操作失败')
        }
      } catch (error) {
        ElMessage.error(error.response?.data?.msg || '操作异常')
      } finally {
        approveSubmitting.value = false
      }
    }
  })
}

const rejectPending = async (row) => {
  try {
    const { value: reason } = await ElMessageBox.prompt('请填写拒绝原因，员工可在库管页查看。', '拒绝入库申请', {
      confirmButtonText: '确认拒绝',
      cancelButtonText: '取消',
      inputPlaceholder: '例如：当前库存充足，请先核对门店库存',
      inputPattern: /\S+/,
      inputErrorMessage: '拒绝原因不能为空',
      inputType: 'textarea',
      inputValidator: (value) => (value || '').trim().length <= 255 || '拒绝原因最多 255 个字'
    })
    const response = await rejectPendingInbound(row.id, { reason: reason.trim() })
    if (response.data.code === 200) {
      ElMessage.success('已拒绝申请')
      fetchPendingInbounds()
    } else {
      ElMessage.error(response.data.msg || '操作失败')
    }
  } catch (e) {
    // cancelled
  }
}

onMounted(() => {
  window.addEventListener('resize', syncViewport)
  fetchProducts()
  fetchPendingInbounds()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', syncViewport)
})
</script>

<style scoped>
/* 针对 el-input-number 的宽度优化 */
:deep(.el-input-number .el-input__wrapper) {
  padding-left: 5px;
  padding-right: 5px;
}

.mobile-workbench-tab {
  min-height: 36px;
  border-radius: 9px;
  color: #64748b;
  font-size: 13px;
  font-weight: 700;
}

.mobile-workbench-tab.is-active {
  background: #fff;
  box-shadow: 0 1px 3px rgb(15 23 42 / 12%);
  color: #0f172a;
}

.mobile-stock-row {
  display: grid;
  width: 100%;
  grid-template-columns: minmax(0, 1fr) max-content;
  align-items: center;
  gap: 10px;
  border-bottom: 1px solid #f1f5f9;
  padding: 10px 2px;
}

.mobile-stock-row:last-child {
  border-bottom: 0;
}

.mobile-stock-row small {
  display: block;
  margin-top: 2px;
  color: #94a3b8;
  font-size: 10px;
}

.mobile-stock-row i {
  border-radius: 5px;
  background: #f1f5f9;
  padding: 3px 4px;
  color: #475569;
  font-style: normal;
  white-space: nowrap;
}

@media (max-width: 767px) {
  :deep(.approval-dialog) {
    width: calc(100% - 24px) !important;
    margin: 12vh auto 0 !important;
    border-radius: 20px;
  }

  :deep(.approval-dialog .el-dialog__body) {
    padding: 18px 20px 8px;
  }

  :deep(.approval-dialog .el-dialog__footer) {
    padding: 12px 20px 20px;
  }

  :deep(.approval-dialog .el-button) {
    min-height: 44px;
  }
}
</style>
