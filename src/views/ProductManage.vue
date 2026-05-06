<template>
  <div class="product-manage-container">
    <!-- Header Area -->
    <div class="flex flex-col md:flex-row md:items-end justify-between mb-8 gap-4">
      <div class="title-wrapper">
        <h1 class="text-2xl md:text-3xl font-black text-slate-900 tracking-tight mb-1">商品库管理</h1>
        <p class="text-sm text-slate-400">维护商品基础档案，设定编码与毛利率</p>
      </div>
      <div class="flex flex-col sm:flex-row gap-3">
        <el-select
          v-model="selectedCategoryId"
          placeholder="全部分类"
          class="!w-full sm:!w-40"
          clearable
          @change="handleSearch"
        >
          <el-option v-for="cat in categories" :key="cat.id" :label="cat.name" :value="cat.id" />
        </el-select>
        <el-input
          v-model="searchQuery"
          placeholder="搜索名称或 SKU"
          class="!w-full sm:!w-48"
          clearable
          @input="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        <el-button type="primary" class="!rounded-xl h-10 shadow-sm" @click="openAddDialog">
          <el-icon class="mr-1"><Plus /></el-icon>新增商品
        </el-button>
        <el-button plain class="!rounded-xl h-10" @click="openCategoryManage">
          分类管理
        </el-button>
      </div>
    </div>

    <!-- Data Table Card -->
    <el-card class="!rounded-2xl border-slate-100 shadow-sm overflow-hidden" shadow="never" body-class="!p-0">
      <!-- PC View -->
      <el-table 
        v-loading="loading"
        :data="productList" 
        style="width: 100%" 
        border
        stripe
        highlight-current-row
        class="desktop-only hidden md:block"
      >
        <el-table-column prop="id" label="ID" width="80" align="center" />
        <el-table-column prop="name" label="商品名称" min-width="200" show-overflow-tooltip />
        <el-table-column prop="sku" label="SKU 编码" width="150" align="center">
          <template #default="{ row }">
            <el-tag size="small" effect="plain" class="!rounded-lg">{{ row.sku }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="分类" width="120" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.category" size="small" type="info">{{ row.category.name }}</el-tag>
            <span v-else class="text-xs text-slate-300">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="margin_rate" label="利润率" width="100" align="center">
          <template #default="{ row }">
            <span class="font-bold text-slate-700">{{ (row.margin_rate * 100).toFixed(0) }}%</span>
          </template>
        </el-table-column>
        <el-table-column prop="latest_cost" label="最新进价" width="120" align="right">
          <template #default="{ row }">
            <span class="font-mono font-bold text-green-600">¥{{ row.latest_cost.toFixed(2) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="库存详情" min-width="220">
          <template #default="{ row }">
            <div class="grid grid-cols-2 gap-x-4 gap-y-1 text-[11px]">
              <div class="flex justify-between border-b border-slate-50 pb-0.5">
                <span class="text-slate-400">主仓</span>
                <span class="font-bold text-slate-700">{{ row.main_stock }}</span>
              </div>
              <div class="flex justify-between border-b border-slate-50 pb-0.5">
                <span class="text-slate-400">门店</span>
                <span class="font-bold" :class="row.store_stock > 10 ? 'text-green-600' : 'text-orange-500'">{{ row.store_stock }}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-slate-400">样机</span>
                <span class="text-slate-600">{{ row.sample_stock }}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-slate-400">云仓</span>
                <span class="font-bold text-blue-600">{{ row.cloud_stock || 0 }}</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" align="center" fixed="right">
          <template #default="{ row }">
            <div class="flex items-center justify-center gap-1">
              <el-button link type="primary" @click="handleEdit(row)">编辑</el-button>
              <el-divider direction="vertical" />
              <el-dropdown trigger="click" @command="(cmd) => handleCommand(cmd, row)">
                <el-button link type="primary">
                  更多<el-icon class="el-icon--right"><arrow-down /></el-icon>
                </el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item command="transfer">库存调拨</el-dropdown-item>
                    <el-dropdown-item command="stocktake">盘库清查</el-dropdown-item>
                    <el-dropdown-item command="logs">变动流水</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <!-- Mobile View -->
      <div class="mobile-only md:hidden p-4 space-y-4" v-loading="loading">
        <div v-for="row in productList" :key="row.id" class="bg-white border border-slate-100 rounded-2xl p-4 shadow-sm">
          <div class="flex justify-between items-start mb-3">
            <div class="flex-1 min-w-0 pr-2">
              <div class="font-bold text-slate-800 text-base leading-tight mb-1">{{ row.name }}</div>
              <div class="flex items-center gap-2">
                <el-tag size="small" effect="plain" class="!rounded-lg !px-2 font-mono">{{ row.sku }}</el-tag>
                <span class="text-[11px] text-slate-400">利润率: {{ (row.margin_rate * 100).toFixed(0) }}%</span>
              </div>
            </div>
            <div class="text-right flex-shrink-0">
              <div class="text-green-600 font-black text-lg">¥{{ row.latest_cost.toFixed(2) }}</div>
              <div class="text-[10px] text-slate-400">最新进价</div>
            </div>
          </div>

          <div class="bg-slate-50 rounded-xl p-3 grid grid-cols-4 gap-2 mb-4">
            <div class="text-center">
              <div class="text-[10px] text-slate-400 mb-0.5">主仓</div>
              <div class="font-bold text-sm text-slate-700">{{ row.main_stock }}</div>
            </div>
            <div class="text-center border-l border-slate-200">
              <div class="text-[10px] text-slate-400 mb-0.5">门店</div>
              <div class="font-bold text-sm" :class="row.store_stock > 0 ? 'text-orange-500' : 'text-red-500'">{{ row.store_stock }}</div>
            </div>
            <div class="text-center border-l border-slate-200">
              <div class="text-[10px] text-slate-400 mb-0.5">样机</div>
              <div class="font-bold text-sm text-slate-600">{{ row.sample_stock }}</div>
            </div>
            <div class="text-center border-l border-slate-200">
              <div class="text-[10px] text-slate-400 mb-0.5">云仓</div>
              <div class="font-bold text-sm text-blue-600">{{ row.cloud_stock || 0 }}</div>
            </div>
          </div>

          <div class="flex justify-end gap-2 pt-3 border-t border-slate-100">
            <el-button size="small" plain @click="handleEdit(row)" class="!rounded-lg">编辑</el-button>
            <el-dropdown trigger="click" @command="(cmd) => handleCommand(cmd, row)">
              <el-button size="small" type="primary" plain class="!rounded-lg">
                更多操作<el-icon class="ml-1"><arrow-down /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="transfer">调拨</el-dropdown-item>
                  <el-dropdown-item command="stocktake">盘库</el-dropdown-item>
                  <el-dropdown-item command="logs">流水</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
        <el-empty v-if="!loading && productList.length === 0" description="暂无商品" />
      </div>
    </el-card>

    <!-- Product Form Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑商品' : '新增商品'"
      width="500px"
      destroy-on-close
      class="custom-dialog"
    >
      <el-form
        ref="productFormRef"
        :model="productForm"
        :rules="formRules"
        label-position="top"
        class="product-form"
      >
        <el-form-item label="商品名称" prop="name">
          <el-input v-model="productForm.name" placeholder="请输入商品完整名称" />
        </el-form-item>
        <el-form-item label="SKU 编码" prop="sku">
          <el-input v-model="productForm.sku" placeholder="请输入唯一商品编码 (条码)" />
        </el-form-item>
        <el-form-item label="商品分类" prop="category_id">
          <el-select v-model="productForm.category_id" placeholder="请选择分类" class="w-full">
            <el-option v-for="cat in categories" :key="cat.id" :label="cat.name" :value="cat.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="固定毛利率 (小数，如 0.15 代表 15%)" prop="margin_rate">
          <el-input-number 
            v-model="productForm.margin_rate" 
            :precision="2" 
            :step="0.01" 
            :min="0" 
            :max="1"
            style="width: 100%"
          />
          <div class="input-tip">预览销售利润：{{ (productForm.margin_rate * 100).toFixed(0) }}%</div>
        </el-form-item>
        <el-form-item label="云仓代发" prop="support_cloud">
          <el-switch v-model="productForm.support_cloud" active-text="支持" inactive-text="不支持" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" :loading="submitting" @click="submitProduct">
            {{ isEdit ? '确认更新' : '确认创建' }}
          </el-button>
        </div>
      </template>
    </el-dialog>

    <!-- 库存调拨 Dialog -->
    <el-dialog v-model="transferVisible" title="库存调拨" width="400px" destroy-on-close class="custom-dialog">
      <el-form :model="transferForm" label-position="top">
        <el-form-item label="调拨动作">
          <el-select v-model="transferForm.action" class="w-full">
            <el-option label="主仓 调至 门店可用" value="main_to_store" />
            <el-option label="门店可用 调至 样机" value="store_to_sample" />
            <el-option label="样机 撤回至 门店可用" value="sample_to_store" />
            <el-option label="主仓 调至 云仓" value="main_to_cloud" />
            <el-option label="云仓 调至 主仓" value="cloud_to_main" />
          </el-select>
        </el-form-item>
        <el-form-item label="调拨数量">
          <el-input-number v-model="transferForm.quantity" :min="1" class="w-full" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="transferVisible = false">取消</el-button>
          <el-button type="primary" :loading="transferring" @click="submitTransfer">确认调拨</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- 盘点 Dialog -->
    <el-dialog v-model="stocktakeVisible" title="盘点清查" width="440px" destroy-on-close class="custom-dialog">
      <el-form :model="stocktakeForm" label-position="top">
        <el-form-item label="盘点仓库">
          <el-radio-group v-model="stocktakeForm.warehouse_type">
            <el-radio :value="1">主仓</el-radio>
            <el-radio :value="2">门店</el-radio>
            <el-radio :value="3">云仓</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="实际清点数量">
          <el-input-number v-model="stocktakeForm.actual_stock" :min="0" class="w-full" />
        </el-form-item>
        <el-form-item label="盘点性质">
          <el-radio-group v-model="stocktakeForm.stocktake_type">
            <el-radio :value="1">
              <span class="font-medium">正常损耗</span>
              <span class="text-xs text-slate-400 ml-1">（差额计入财务亏损）</span>
            </el-radio>
            <el-radio :value="2">
              <span class="font-medium">业务调整</span>
              <span class="text-xs text-slate-400 ml-1">（其它门店销售/移库，仅修正库存）</span>
            </el-radio>
          </el-radio-group>
          <div v-if="stocktakeForm.stocktake_type === 2" class="mt-2 p-2 bg-blue-50 rounded-lg text-xs text-blue-600">
            💡 选择此选项：系统只更新库存数量，不生成任何财务亏损记录
          </div>
        </el-form-item>
        <el-form-item label="备注说明">
          <el-input v-model="stocktakeForm.remark" placeholder="如：B门店卖出2台/调货至总仓等" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="stocktakeVisible = false">取消</el-button>
          <el-button type="primary" :loading="stocktaking" @click="submitStocktake">确认盘点</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- 库存流水 Dialog -->
    <el-dialog v-model="logsVisible" title="库存操作流水" width="800px" destroy-on-close class="custom-dialog">
      <div class="mb-4 flex items-center justify-between">
        <el-radio-group v-model="logWarehouseType" size="small" @change="fetchLogs">
          <el-radio-button value="">全部仓库</el-radio-button>
          <el-radio-button :value="1">主仓</el-radio-button>
          <el-radio-button :value="2">门店</el-radio-button>
          <el-radio-button :value="3">云仓</el-radio-button>
          <el-radio-button :value="4">样机</el-radio-button>
        </el-radio-group>
      </div>
      <el-table 
        v-loading="loadingLogs"
        :data="currentLogs" 
        style="width: 100%" 
        border
        stripe
        height="400"
      >
        <el-table-column prop="created_at" label="时间" width="160">
          <template #default="{ row }">
            {{ new Date(row.created_at).toLocaleString('zh-CN', { hour12: false }) }}
          </template>
        </el-table-column>
        <el-table-column label="仓库" width="80" align="center">
          <template #default="{ row }">
            <el-tag size="small" type="info" effect="plain">{{ getWarehouseName(row.warehouse_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="90" align="center">
          <template #default="{ row }">
            <el-tag size="small" :type="getLogTypeTag(row.log_type).type" effect="dark">
              {{ getLogTypeTag(row.log_type).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="库存变动" width="100" align="center">
          <template #default="{ row }">
            <span class="font-bold" :class="row.change_qty > 0 ? 'text-blue-600' : 'text-red-500'">
              {{ row.change_qty > 0 ? '+' : '' }}{{ row.change_qty }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="库存快照" width="120" align="center">
          <template #default="{ row }">
            <span class="text-xs text-slate-500">{{ row.before_qty }} &rarr; <span class="font-bold text-slate-700">{{ row.after_qty }}</span></span>
          </template>
        </el-table-column>
        <el-table-column prop="related_no" label="关联单号" width="120" show-overflow-tooltip />
        <el-table-column prop="remark" label="备注" min-width="150" show-overflow-tooltip />
      </el-table>
    </el-dialog>

    <!-- 分类管理 Dialog -->
    <el-dialog v-model="categoryDialogVisible" title="分类管理" width="500px" destroy-on-close class="custom-dialog">
      <div class="mb-4 flex gap-2">
        <el-input v-model="newCategoryName" placeholder="新分类名称" class="flex-1" />
        <el-button type="primary" @click="handleAddCategory">添加</el-button>
      </div>
      <el-table :data="categories" border stripe size="small" height="300">
        <el-table-column prop="name" label="分类名称">
          <template #default="{ row }">
            <el-input v-if="row.isEditing" v-model="row.editName" size="small" />
            <span v-else>{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" align="center">
          <template #default="{ row }">
            <template v-if="row.isEditing">
              <el-button link type="primary" @click="saveCategoryEdit(row)">保存</el-button>
              <el-button link @click="row.isEditing = false">取消</el-button>
            </template>
            <template v-else>
              <el-button link type="primary" @click="startCategoryEdit(row)">编辑</el-button>
              <el-popconfirm title="确定删除吗？" @confirm="handleDeleteCategory(row.id)">
                <template #reference>
                  <el-button link type="danger">删除</el-button>
                </template>
              </el-popconfirm>
            </template>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

  </div>
</template>

<script setup>
import { ref, onMounted, reactive } from 'vue'
import { Plus, Search, ArrowDown } from '@element-plus/icons-vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'

const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const productList = ref([])
const categories = ref([])
const selectedCategoryId = ref('')
const searchQuery = ref('')
const productFormRef = ref(null)

const isEdit = ref(false)
const currentEditId = ref(null)

const productForm = reactive({
  name: '',
  sku: '',
  category_id: null,
  margin_rate: 0.15,
  support_cloud: false
})

const formRules = {
  name: [{ required: true, message: '请输入商品名称', trigger: 'blur' }],
  sku: [{ required: true, message: '请输入SKU编码', trigger: 'blur' }],
  category_id: [{ required: true, message: '请选择商品分类', trigger: 'change' }],
  margin_rate: [{ required: true, message: '请设置毛利率', trigger: 'blur' }]
}

const fetchCategories = async () => {
  try {
    const res = await axios.get('/api/categories')
    if (res.data.code === 200) {
      categories.value = res.data.data.map(c => ({ ...c, isEditing: false, editName: c.name }))
    }
  } catch (error) {
    console.error('Fetch categories error:', error)
  }
}

const fetchProducts = async () => {
  loading.value = true
  try {
    const res = await axios.get('/api/products', {
      params: { 
        keyword: searchQuery.value,
        category_id: selectedCategoryId.value,
        all: 1
      }
    })
    if (res.data.code === 200) {
      productList.value = res.data.data
    }
  } catch (error) {
    ElMessage.error('获取商品列表失败')
  } finally {
    loading.value = false
  }
}

// 分类管理相关
const categoryDialogVisible = ref(false)
const newCategoryName = ref('')
const openCategoryManage = () => categoryDialogVisible.value = true

const handleAddCategory = async () => {
  if (!newCategoryName.value) return
  try {
    const res = await axios.post('/api/categories', { name: newCategoryName.value })
    if (res.data.code === 200) {
      ElMessage.success('添加成功')
      newCategoryName.value = ''
      fetchCategories()
    }
  } catch (error) {
    ElMessage.error('添加失败')
  }
}

const startCategoryEdit = (row) => {
  row.isEditing = true
  row.editName = row.name
}

const saveCategoryEdit = async (row) => {
  try {
    const res = await axios.put(`/api/categories/${row.id}`, { name: row.editName })
    if (res.data.code === 200) {
      ElMessage.success('修改成功')
      fetchCategories()
    }
  } catch (error) {
    ElMessage.error('修改失败')
  }
}

const handleDeleteCategory = async (id) => {
  try {
    const res = await axios.delete(`/api/categories/${id}`)
    if (res.data.code === 200) {
      ElMessage.success('删除成功')
      fetchCategories()
    } else {
      ElMessage.error(res.data.msg)
    }
  } catch (error) {
    ElMessage.error('删除失败')
  }
}

const handleSearch = () => {
  fetchProducts()
}

const openAddDialog = () => {
  isEdit.value = false
  currentEditId.value = null
  productForm.name = ''
  productForm.sku = ''
  productForm.category_id = null
  productForm.margin_rate = 0.15
  productForm.support_cloud = false
  dialogVisible.value = true
}

const handleEdit = (row) => {
  isEdit.value = true
  currentEditId.value = row.id
  productForm.name = row.name
  productForm.sku = row.sku
  productForm.category_id = row.category_id
  productForm.margin_rate = row.margin_rate
  productForm.support_cloud = row.support_cloud || false
  dialogVisible.value = true
}

// 调拨与盘库状态
const transferVisible = ref(false)
const transferring = ref(false)
const transferForm = reactive({
  action: 'main_to_store',
  quantity: 1,
  productId: null
})

const stocktakeVisible = ref(false)
const stocktaking = ref(false)
const stocktakeForm = reactive({
  warehouse_type: 1,
  actual_stock: 0,
  stocktake_type: 1,
  remark: '',
  productId: null
})

const handleCommand = (command, row) => {
  if (command === 'transfer') {
    transferForm.productId = row.id
    transferForm.quantity = 1
    transferForm.action = 'main_to_store'
    transferVisible.value = true
  } else if (command === 'stocktake') {
    stocktakeForm.productId = row.id
    stocktakeForm.warehouse_type = 1
    stocktakeForm.actual_stock = row.main_stock
    stocktakeForm.stocktake_type = 1
    stocktakeForm.remark = ''
    stocktakeVisible.value = true
  } else if (command === 'logs') {
    currentLogProductId.value = row.id
    logWarehouseType.value = ''
    logsVisible.value = true
    fetchLogs()
  }
}

// 流水相关
const logsVisible = ref(false)
const loadingLogs = ref(false)
const currentLogs = ref([])
const currentLogProductId = ref(null)
const logWarehouseType = ref('')

const fetchLogs = async () => {
  if (!currentLogProductId.value) return
  loadingLogs.value = true
  try {
    const res = await axios.get(`/api/products/${currentLogProductId.value}/logs`, {
      params: {
        warehouse_type: logWarehouseType.value
      }
    })
    if (res.data.code === 200) {
      currentLogs.value = res.data.data
    } else {
      ElMessage.error(res.data.msg || '获取流水失败')
    }
  } catch (error) {
    ElMessage.error('获取流水失败')
  } finally {
    loadingLogs.value = false
  }
}

const getWarehouseName = (type) => {
  const map = { 1: '主仓', 2: '门店', 3: '云仓', 4: '样机' }
  return map[type] || '未知'
}

const getLogTypeTag = (type) => {
  const map = {
    'purchase': { label: '入库', type: 'success' },
    'sale': { label: '销售', type: 'primary' },
    'transfer': { label: '调拨', type: 'warning' },
    'stocktake': { label: '盘点', type: 'info' },
    'cancel': { label: '退库', type: 'danger' }
  }
  return map[type] || { label: type, type: 'info' }
}

const submitTransfer = async () => {
  if (transferForm.quantity <= 0) return
  transferring.value = true
  try {
    const res = await axios.put(`/api/products/${transferForm.productId}/transfer`, {
      action: transferForm.action,
      quantity: transferForm.quantity
    })
    if (res.data.code === 200) {
      ElMessage.success('调拨成功')
      transferVisible.value = false
      fetchProducts()
    } else {
      ElMessage.error(res.data.msg || '调拨失败')
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '网络异常')
  } finally {
    transferring.value = false
  }
}

const submitStocktake = async () => {
  stocktaking.value = true
  try {
    const res = await axios.post(`/api/products/${stocktakeForm.productId}/stocktake`, {
      warehouse_type: stocktakeForm.warehouse_type,
      actual_stock: stocktakeForm.actual_stock,
      stocktake_type: stocktakeForm.stocktake_type,
      remark: stocktakeForm.remark
    })
    if (res.data.code === 200) {
      const typeLabel = stocktakeForm.stocktake_type === 2 ? '（业务调整，未计财务亏损）' : ''
      ElMessage.success(`盘点完成，差异数量：${res.data.difference} ${typeLabel}`)
      stocktakeVisible.value = false
      fetchProducts()
    } else {
      ElMessage.error(res.data.msg || '盘点失败')
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '网络异常')
  } finally {
    stocktaking.value = false
  }
}

const submitProduct = async () => {
  if (!productFormRef.value) return
  
  await productFormRef.value.validate(async (valid) => {
    if (valid) {
      submitting.value = true
      try {
        let res
        if (isEdit.value) {
          res = await axios.put(`/api/products/${currentEditId.value}`, productForm)
        } else {
          res = await axios.post('/api/products', productForm)
        }
        
        if (res.data.code === 200) {
          ElMessage.success(isEdit.value ? '商品更新成功' : '商品创建成功')
          dialogVisible.value = false
          fetchProducts()
        } else {
          ElMessage.error(res.data.msg || '操作失败')
        }
      } catch (error) {
        ElMessage.error('服务器响应错误')
      } finally {
        submitting.value = false
      }
    }
  })
}

onMounted(() => {
  fetchCategories()
  fetchProducts()
})
</script>

<style scoped>
.product-manage-container {
  padding: 24px;
  background-color: #f8fafc;
  min-height: calc(100vh - 64px);
}

.header-section {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  margin-bottom: 24px;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: #1e293b;
  margin: 0 0 4px 0;
}

.page-subtitle {
  font-size: 14px;
  color: #64748b;
  margin: 0;
}

.action-wrapper {
  display: flex;
  gap: 12px;
}

.search-input {
  width: 280px;
}

.table-card {
  border-radius: 12px;
  border: 1px solid #e2e8f0;
}

.margin-text {
  font-weight: 500;
  color: #0f172a;
}

.price-text {
  font-family: 'Inter', system-ui;
  font-weight: 600;
  color: #10b981;
}

.custom-dialog :deep(.el-dialog) {
  border-radius: 16px;
  overflow: hidden;
}

.custom-dialog :deep(.el-dialog__header) {
  margin-right: 0;
  padding: 20px 24px;
  border-bottom: 1px solid #f1f5f9;
}

.custom-dialog :deep(.el-dialog__title) {
  font-weight: 600;
}

.product-form {
  padding: 0 4px;
}

.input-tip {
  font-size: 12px;
  color: #94a3b8;
  margin-top: 4px;
}

.dialog-footer {
  padding-top: 12px;
}

:deep(.el-table) {
  --el-table-header-bg-color: #f8fafc;
  --el-table-header-text-color: #475569;
}

:deep(.el-table th.el-table__cell) {
  font-weight: 600;
}
</style>
