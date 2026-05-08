<template>
  <div class="p-6 min-h-screen bg-gray-50">
    <div class="max-w-7xl mx-auto space-y-6">
      <!-- 顶部标题 -->
      <!-- 顶部标题 -->
      <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h1 class="text-2xl md:text-3xl font-black text-slate-900 tracking-tight">采购与入库</h1>
          <p class="text-sm text-slate-500 mt-1">记录商品进货流水，动态更新库存与成本</p>
        </div>
        <el-tag type="success" effect="dark" class="!px-4 !py-2 !rounded-xl hidden md:inline-flex">仓库管理系统</el-tag>
      </div>

      <el-row :gutter="24" class="!mx-0">
        <!-- 左侧：录入表单 -->
        <el-col :xs="24" :sm="24" :md="8" :lg="8" class="!px-0 md:!px-3 mb-6 md:mb-0">
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
                      <span class="text-gray-400 text-[10px] ml-2">SKU: {{ item.sku }}</span>
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
                <div class="grid grid-cols-3 gap-3 w-full">
                  <div 
                    v-for="item in [
                      { label: '主仓', value: 1, icon: 'OfficeBuilding' },
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

              <el-form-item label="付款账户" prop="account_id">
                <el-select v-model="form.account_id" placeholder="选择付款账户" class="w-full">
                  <el-option v-for="acc in accounts" :key="acc.id" :label="acc.name" :value="acc.id" />
                </el-select>
              </el-form-item>

              <div class="mt-8 p-4 bg-blue-50/50 rounded-2xl border border-blue-100 mb-6">
                <div class="flex justify-between items-center">
                  <span class="text-sm text-blue-700 font-medium">预计总支出:</span>
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
        <el-col :xs="24" :sm="24" :md="16" :lg="16" class="!px-0 md:!px-3">
          <div class="bg-white p-6 rounded-2xl shadow-sm border border-slate-100">
            <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center mb-6 gap-4">
              <h3 class="text-lg font-bold text-slate-700">实时库存大盘</h3>
              <el-input
                v-model="searchKeyword"
                placeholder="搜索名称或 SKU..."
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
              <el-table-column prop="sku" label="SKU" width="120" />
              <el-table-column label="库存详情" width="120" align="center">
                <template #default="scope">
                  <div class="flex flex-col gap-1 text-[10px]">
                    <div class="flex justify-between border-b border-slate-50">
                      <span>主:</span> <span class="font-bold">{{ scope.row.main_stock }}</span>
                    </div>
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
                    SKU: {{ row.sku }} | 主:{{ row.main_stock }} 店:{{ row.store_stock }}
                  </div>
                </div>
                <el-button type="primary" plain size="small" @click="preFillForm(row)" class="!rounded-lg">填入</el-button>
              </div>
              <el-empty v-if="allProducts.length === 0" description="未找到商品" />
            </div>
          </div>
        </el-col>
      </el-row>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Download, Search, OfficeBuilding, Shop, Cloudy } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import axios from 'axios'

const allProducts = ref([])
const accounts = ref([])
const searchKeyword = ref('')
const submitting = ref(false)
const formRef = ref(null)

const form = ref({
  product_id: null,
  quantity: 1,
  unit_cost: 0,
  warehouse_type: 1,
  account_id: null
})

const rules = {
  product_id: [{ required: true, message: '请选择入库商品', trigger: 'change' }],
  quantity: [{ required: true, message: '请输入入库数量', trigger: 'blur' }],
  unit_cost: [{ required: true, message: '请输入进货单价', trigger: 'blur' }],
  warehouse_type: [{ required: true, message: '请选择入库仓库', trigger: 'change' }],
  account_id: [{ required: true, message: '请选择付款账户', trigger: 'change' }]
}

const totalCost = computed(() => {
  return (form.value.quantity || 0) * (form.value.unit_cost || 0)
})

const fetchProducts = async () => {
  try {
    // 调用现有接口获取所有商品（不限库存）
    const response = await axios.get('/api/products', {
      params: { keyword: searchKeyword.value }
    })
    if (response.data.code === 200) {
      allProducts.value = response.data.data
    }
  } catch (error) {
    console.error('Fetch products error:', error)
    ElMessage.error('获取商品列表失败')
  }
}

const fetchAccounts = async () => {
  try {
    const response = await axios.get('/api/accounts')
    if (response.data.code === 200) {
      accounts.value = response.data.data
    }
  } catch (error) {
    console.error('Fetch accounts error:', error)
  }
}

const submitInbound = async () => {
  if (!formRef.value) return
  
  await formRef.value.validate(async (valid) => {
    if (valid) {
      submitting.value = true
      try {
        const response = await axios.post('/api/inbounds', {
          product_id: form.value.product_id,
          quantity: form.value.quantity,
          unit_cost: form.value.unit_cost,
          warehouse_type: form.value.warehouse_type,
          account_id: form.value.account_id
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
    unit_cost: 0,
    warehouse_type: 1,
    account_id: null
  }
  if (formRef.value) formRef.value.resetFields()
}

const preFillForm = (product) => {
  form.value.product_id = product.id
  form.value.unit_cost = product.latest_cost
  ElMessage.info(`已填入 ${product.name} 的信息`)
}

onMounted(() => {
  fetchProducts()
  fetchAccounts()
})
</script>

<style scoped>
/* 针对 el-input-number 的宽度优化 */
:deep(.el-input-number .el-input__wrapper) {
  padding-left: 5px;
  padding-right: 5px;
}
</style>
