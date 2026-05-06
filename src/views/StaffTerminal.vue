<template>
  <div class="h-screen w-screen bg-slate-100 overflow-hidden relative">
    <!-- 1. 登录锁定界面 -->
    <transition name="el-fade-in-linear">
      <div v-if="!currentEmployee" class="absolute inset-0 z-[100] bg-slate-900 flex items-center justify-center p-6">
        <div class="max-w-4xl w-full">
          <div class="text-center mb-12">
            <h1 class="text-4xl font-black text-white tracking-widest mb-2">WONCHON POS</h1>
            <p class="text-slate-400">请选择您的身份以开始营业</p>
          </div>
          
          <div v-if="!selectedEmployee" class="grid grid-cols-2 md:grid-cols-4 gap-6">
            <div 
              v-for="emp in employees" 
              :key="emp.id"
              @click="selectedEmployee = emp"
              class="bg-slate-800/50 border border-slate-700 p-6 rounded-2xl flex flex-col items-center hover:bg-blue-600 hover:border-blue-400 transition-all cursor-pointer group"
            >
              <div class="w-20 h-20 bg-slate-700 rounded-full flex items-center justify-center mb-4 text-3xl font-bold text-white group-hover:bg-white/20 transition-colors">
                {{ emp.name.charAt(0) }}
              </div>
              <div class="text-lg font-bold text-white">{{ emp.name }}</div>
              <div class="text-xs text-slate-500 mt-1">{{ emp.emp_no }}</div>
            </div>
          </div>

          <div v-else class="max-w-md mx-auto bg-slate-800 border border-slate-700 p-8 rounded-3xl shadow-2xl select-none">
            <div class="flex items-center gap-4 mb-8">
              <el-button circle :icon="Back" @click="selectedEmployee = null" />
              <div class="text-xl font-bold text-white">身份确认：{{ selectedEmployee.name }}</div>
            </div>

            <!-- 动态 PIN 码显示区：支持变长密码 -->
            <div class="flex justify-center items-center gap-2 h-16 mb-8 bg-slate-900/50 rounded-2xl border border-slate-700/50">
              <template v-if="pin.length > 0">
                <div v-for="i in pin.length" :key="i" class="w-4 h-4 rounded-full bg-blue-500 shadow-[0_0_10px_rgba(59,130,246,0.5)] transition-all"></div>
              </template>
              <div v-else class="text-slate-600 font-medium tracking-widest animate-pulse">请输入密码</div>
            </div>

            <!-- 数字键盘：禁用双击缩放 -->
            <div class="grid grid-cols-3 gap-4 touch-none" style="touch-action: manipulation;">
              <button v-for="n in 9" :key="n" 
                @click="inputPin(n)" 
                class="h-16 rounded-2xl bg-slate-700 text-white text-2xl font-bold active:bg-blue-600 active:scale-95 transition-all outline-none"
                style="touch-action: manipulation;"
              >{{ n }}</button>
              
              <button @click="pin = ''" 
                class="h-16 rounded-2xl bg-red-500/10 text-red-500 text-xl font-bold active:bg-red-500 active:text-white transition-all outline-none"
                style="touch-action: manipulation;"
              >重置</button>
              
              <button @click="inputPin(0)" 
                class="h-16 rounded-2xl bg-slate-700 text-white text-2xl font-bold active:bg-blue-600 active:scale-95 transition-all outline-none"
                style="touch-action: manipulation;"
              >0</button>
              
              <button @click="verifyPin" 
                class="h-16 rounded-2xl bg-blue-600 text-white text-xl font-bold active:bg-blue-700 active:scale-95 transition-all outline-none shadow-lg shadow-blue-500/20"
                style="touch-action: manipulation;"
                :loading="verifying"
              >确认</button>
            </div>
          </div>
        </div>
      </div>
    </transition>

    <!-- 2. 全功能主收银界面 (与管理员版完全一致) -->
    <div v-if="currentEmployee" class="relative flex flex-col h-full bg-slate-50">
      <!-- 顶部固定区域：状态栏与搜索框 -->
      <div class="p-4 bg-white shadow-sm z-10 flex-shrink-0">
        <div class="flex justify-between items-center mb-3">
          <div class="flex items-center gap-2">
            <div class="w-1.5 h-5 bg-blue-600 rounded-full"></div>
            <h1 class="text-xl font-bold text-slate-800 tracking-tight">门店收银终端</h1>
            <el-tag type="info" size="small" class="ml-2">{{ currentEmployee.name }}</el-tag>
            
            <!-- Attendance Buttons -->
            <div class="ml-4 flex items-center gap-2">
              <el-button 
                v-if="!todayAttendance?.check_in" 
                type="success" 
                size="small" 
                :loading="attendanceLoading"
                @click="handleCheckIn"
              >上班打卡</el-button>
              <el-button 
                v-else-if="!todayAttendance?.check_out" 
                type="warning" 
                size="small" 
                :loading="attendanceLoading"
                @click="handleCheckOut"
              >下班打卡</el-button>
              <el-tag v-else type="info" size="small">今日打卡已完成</el-tag>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <el-button type="primary" link @click="openOrdersDrawer" :icon="List">我的订单</el-button>
            <el-divider direction="vertical" />
            <el-button type="info" link @click="logout" :icon="Switch">切换账号</el-button>
          </div>
        </div>
        <div class="flex gap-2 mb-3">
          <el-input
            v-model="searchQuery"
            placeholder="搜索商品名称、型号或 SKU..."
            prefix-icon="Search"
            clearable
            size="large"
            class="pos-search-input flex-1"
            @input="fetchProducts"
          />
          <el-button type="primary" size="large" class="search-btn" @click="fetchProducts">搜索</el-button>
        </div>

        <!-- 分类横向导航 -->
        <div class="flex overflow-x-auto no-scrollbar gap-2 pb-1">
          <div 
            class="px-4 py-1.5 rounded-full text-xs font-bold whitespace-nowrap transition-all cursor-pointer"
            :class="selectedCategoryId === '' ? 'bg-blue-600 text-white shadow-md shadow-blue-100' : 'bg-slate-100 text-slate-500'"
            @click="selectCategory('')"
          >
            全部
          </div>
          <div 
            v-for="cat in categories" 
            :key="cat.id"
            class="px-4 py-1.5 rounded-full text-xs font-bold whitespace-nowrap transition-all cursor-pointer"
            :class="selectedCategoryId === cat.id ? 'bg-blue-600 text-white shadow-md shadow-blue-100' : 'bg-slate-100 text-slate-500'"
            @click="selectCategory(cat.id)"
          >
            {{ cat.name }}
          </div>
        </div>
      </div>

      <!-- 中间滚动区域：商品网格 -->
      <div class="flex-1 overflow-y-auto p-4 pb-32">
        <div v-if="productList.length === 0" class="py-10 text-center">
          <el-empty description="暂无商品数据" :image-size="100" />
        </div>
        
        <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 gap-3">
          <div 
            v-for="row in productList" 
            :key="row.id" 
            class="bg-white rounded-xl shadow-sm border border-slate-100 flex flex-col p-3 hover:border-blue-300 transition-all cursor-pointer group"
            @click="addToCart(row)"
          >
            <div class="flex-1 min-w-0">
              <div class="font-bold text-slate-800 text-sm line-clamp-2 leading-tight h-9 mb-1">{{ row.name }}</div>
              <div class="flex flex-wrap gap-1 mb-2">
                <el-tag :type="row.store_stock > 0 ? 'success' : 'danger'" size="small" class="!px-1.5 !h-5 !text-[10px]">店:{{ row.store_stock }}</el-tag>
                <el-tag :type="row.main_stock > 0 ? 'primary' : 'info'" size="small" class="!px-1.5 !h-5 !text-[10px]">仓:{{ row.main_stock }}</el-tag>
              </div>
            </div>
            <div class="flex justify-between items-end mt-auto pt-2 border-t border-slate-50">
              <div class="flex flex-col">
                <span class="text-[10px] text-slate-400 line-through">¥{{ (calculateMinPrice(row) * 1.2).toFixed(0) }}</span>
                <span class="font-bold text-red-600 text-base leading-none">¥{{ calculateMinPrice(row).toFixed(0) }}</span>
              </div>
              <div class="bg-blue-600 text-white p-1.5 rounded-lg">
                <el-icon :size="16"><Plus /></el-icon>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 底部吸底结算条 -->
      <div class="fixed bottom-0 left-0 w-full bg-white shadow-[0_-4px_15px_rgba(0,0,0,0.05)] p-4 z-40 flex justify-between items-center">
        <div class="flex items-center gap-3 cursor-pointer" @click="openDrawer">
          <el-badge :value="cart.length" :hidden="cart.length === 0" type="danger">
            <div class="bg-blue-100 p-2.5 rounded-full text-blue-600">
              <el-icon size="26"><ShoppingCart /></el-icon>
            </div>
          </el-badge>
          <div>
            <div class="text-xs text-slate-500">已选商品</div>
            <div class="font-black text-xl text-slate-800 leading-none">¥{{ originalTotal.toFixed(2) }}</div>
          </div>
        </div>
        <el-button type="primary" size="large" round class="px-10 !h-12 font-bold shadow-lg shadow-blue-200" @click="openDrawer" :disabled="cart.length === 0">
          立即开单结算
        </el-button>
      </div>

      <!-- 结算抽屉 (完全复用管理员版逻辑) -->
      <el-drawer v-model="drawerVisible" direction="rtl" :size="isMobile ? '100%' : '500px'" title="购物车与结算">
        <div class="h-full flex flex-col">
          <div class="flex-1 overflow-y-auto p-4 space-y-4">
            <div v-for="(item, index) in cart" :key="index" class="p-3 rounded-xl border border-slate-100 bg-white shadow-sm">
              <div class="flex justify-between items-start mb-2">
                <div class="font-bold text-slate-800 text-sm truncate pr-2">{{ item.name }}</div>
                <el-button type="danger" link :icon="Delete" @click="removeFromCart(index)" />
              </div>
              <div class="flex justify-between items-center mt-2">
                <div class="text-xs text-slate-600">作为赠品</div>
                <el-switch v-model="item.isGift" size="small" />
              </div>
              <div class="flex justify-between items-center mt-2" :class="{'opacity-40 pointer-events-none': item.isGift}">
                <div class="text-xs text-slate-600">成交单价 (底价:¥{{ item.minPrice.toFixed(2) }})</div>
                <el-input-number v-model="item.price" :min="0" :precision="2" :controls="false" size="small" class="!w-24" :class="{'price-warning': item.price < item.minPrice && !item.isGift}" />
              </div>
              <div class="flex justify-between items-center mt-2">
                <div class="text-xs text-slate-600">购买数量</div>
                <el-input-number v-model="item.quantity" :min="1" size="small" class="!w-24" />
              </div>
            </div>

            <!-- 客户与支付信息 -->
            <div class="bg-blue-50/50 p-4 rounded-xl space-y-3">
              <div class="grid grid-cols-2 gap-2">
                <el-input v-model="customerName" placeholder="客户姓名" />
                <el-input v-model="customerPhone" placeholder="手机号" />
              </div>

              <!-- 模式切换 -->
              <div class="grid grid-cols-2 gap-2">
                <div class="flex justify-between items-center p-2.5 rounded-xl bg-white border border-slate-100 shadow-sm">
                  <span class="text-xs font-bold text-slate-600">国家补贴</span>
                  <el-switch v-model="isSubsidized" size="small" />
                </div>
                <div class="flex justify-between items-center p-2.5 rounded-xl bg-white border border-slate-100 shadow-sm">
                  <span class="text-xs font-bold text-slate-600">支付定金</span>
                  <el-switch v-model="isDepositMode" size="small" />
                </div>
              </div>

              <el-collapse-transition>
                <div v-if="isDepositMode" class="bg-purple-50 p-3 rounded-xl border border-purple-100">
                  <div class="text-[10px] text-purple-400 mb-1 font-bold">预收定金金额</div>
                  <el-input-number v-model="depositAmount" :min="0" :max="totalPayable" :precision="2" class="w-full" :controls="false" placeholder="输入定金金额" />
                </div>
              </el-collapse-transition>

              <div class="flex justify-between items-center pt-2">
                <span class="text-xs text-slate-500">发货方式</span>
                <el-radio-group v-model="deliveryMethod" size="small">
                  <el-radio-button :value="1">自提</el-radio-button>
                  <el-radio-button :value="2">主仓</el-radio-button>
                  <el-radio-button :value="3">云仓</el-radio-button>
                </el-radio-group>
              </div>
              <div class="flex justify-between items-center">
                <span class="text-xs text-slate-500">收款账户</span>
                <el-radio-group v-model="paymentMethod" size="small">
                  <el-radio-button :value="1">支付宝</el-radio-button>
                  <el-radio-button :value="2">微信</el-radio-button>
                </el-radio-group>
              </div>
            </div>
          </div>

          <div class="p-4 bg-white border-t space-y-2">
            <div v-if="isSubsidized" class="flex justify-between text-xs text-orange-600">
              <span>国家补贴减免</span>
              <span>-¥{{ subsidyAmount.toFixed(2) }}</span>
            </div>
            <div class="flex justify-between items-end mb-4">
              <span class="text-sm font-bold text-slate-600">{{ isDepositMode ? '预收定金' : '实收合计' }}</span>
              <span class="text-3xl font-black text-blue-600">¥{{ actualReceipt.toFixed(2) }}</span>
            </div>
            <el-button type="primary" size="large" class="w-full !h-14 !rounded-2xl !font-bold" :loading="loading" @click="handleCheckout">
              确认开单提交
            </el-button>
          </div>
        </div>
      </el-drawer>
    </div>

    <!-- 我的订单跟踪抽屉 -->
    <el-drawer v-model="ordersDrawerVisible" direction="ltr" :size="isMobile ? '100%' : '450px'" title="订单查询与追踪" class="staff-orders-drawer">
      <div v-loading="ordersLoading" class="h-full flex flex-col bg-slate-50">
        <!-- 搜索栏 -->
        <div class="p-4 bg-white border-b border-slate-100 sticky top-0 z-10">
          <el-input 
            v-model="orderSearchKeyword" 
            placeholder="搜索订单号/客户电话 (全局)" 
            clearable
            @input="handleOrderSearchInput"
            class="staff-order-search"
          >
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
        </div>

        <div class="flex-1 overflow-y-auto p-4 space-y-4">
          <div v-for="order in myOrders" :key="order.id" class="bg-white rounded-2xl p-4 shadow-sm border border-slate-100">
            <div class="flex justify-between items-start mb-3">
              <div>
                <div class="text-[10px] text-slate-400 font-mono">{{ order.order_no }}</div>
                <div class="font-bold text-slate-800">{{ order.customer_name || '散客' }}</div>
              </div>
              <el-tag :type="order.payment_status === 2 ? 'success' : 'warning'" size="small">
                {{ order.payment_status === 1 ? '定金' : '已清' }}
              </el-tag>
            </div>

            <!-- 商品摘要 -->
            <div class="mb-3 p-2 bg-blue-50/30 rounded-xl space-y-1">
              <div v-for="item in order.order_items" :key="item.id" class="flex justify-between items-center text-[11px]">
                <span class="text-slate-600 font-medium truncate flex-1 mr-2">{{ item.product?.name }}</span>
                <span class="text-slate-400 flex-shrink-0">x{{ item.quantity }}</span>
              </div>
            </div>
            
            <!-- 关键状态追踪 -->
            <div class="grid grid-cols-2 gap-2 mb-4">
              <div class="p-2 rounded-xl bg-slate-50 border border-slate-100">
                <div class="text-[10px] text-slate-400 mb-1">安装进度</div>
                <div class="text-xs font-bold" :class="order.is_installed ? 'text-green-600' : 'text-slate-500'">
                  {{ order.is_installed ? '✅ 已安装' : '⏳ 待安装' }}
                </div>
              </div>
              <div class="p-2 rounded-xl bg-slate-50 border border-slate-100">
                <div class="text-[10px] text-slate-400 mb-1">国补资料</div>
                <div class="text-xs font-bold" :class="getSubsidyType(order.subsidy_status)">
                  {{ getSubsidyLabel(order.subsidy_status) }}
                </div>
              </div>
            </div>

            <!-- 操作按钮 -->
            <div v-if="order.order_status === 1" class="flex justify-end gap-2 pt-3 border-t border-slate-50">
              <el-button 
                v-if="!order.is_installed" 
                type="primary" 
                size="small" 
                plain
                @click="handleStaffInstall(order)"
              >确认安装</el-button>
              <el-button 
                v-if="order.subsidy_status === 1" 
                type="warning" 
                size="small" 
                plain
                @click="confirmSubsidy(order)"
              >确认已交资料</el-button>
              <el-button type="info" size="small" link @click="showOrderDetail(order)">详情</el-button>
            </div>
          </div>
          <el-empty v-if="myOrders.length === 0" description="暂无历史订单" />
        </div>
      </div>
    </el-drawer>

    <!-- 员工端确认安装弹窗 -->
    <el-dialog v-model="installDialogVisible" title="确认安装完成" width="90%" class="!rounded-2xl max-w-md">
      <div class="space-y-4">
        <el-alert title="确认安装" type="info" :closable="false" show-icon>请选择实际执行安装的人员</el-alert>
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">安装师傅</label>
          <el-select v-model="selectedInstallerId" class="w-full" placeholder="请选择师傅" size="large">
            <el-option v-for="emp in employees" :key="emp.id" :label="emp.name" :value="emp.id" />
          </el-select>
        </div>
      </div>
      <template #footer>
        <div class="flex gap-2">
          <el-button @click="installDialogVisible = false" class="flex-1">取消</el-button>
          <el-button type="primary" @click="confirmStaffInstall" :disabled="!selectedInstallerId" class="flex-1">确认提交</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- 员工端订单详情弹窗 -->
    <el-dialog v-model="detailDialogVisible" title="订单深度详情" width="95%" class="!rounded-2xl max-w-lg" body-class="!p-0">
      <div v-if="currentDetailOrder" class="p-5 space-y-6">
        <!-- 头部信息 -->
        <div class="flex justify-between items-start">
          <div>
            <div class="text-[10px] text-slate-400 font-mono">{{ currentDetailOrder.order_no }}</div>
            <div class="text-xl font-black text-slate-800">{{ currentDetailOrder.customer_name || '散客' }}</div>
            <div class="text-sm text-slate-500">{{ currentDetailOrder.customer_phone || '-' }}</div>
          </div>
          <el-tag :type="currentDetailOrder.payment_status === 2 ? 'success' : 'warning'" effect="dark">
            {{ currentDetailOrder.payment_status === 1 ? '仅付定金' : '已结清全款' }}
          </el-tag>
        </div>

        <!-- 商品明细 -->
        <div class="space-y-3">
          <div class="text-xs font-bold text-slate-400 uppercase tracking-wider">购买清单</div>
          <div class="bg-slate-50 rounded-2xl p-4 space-y-3">
            <div v-for="item in currentDetailOrder.order_items" :key="item.id" class="flex justify-between items-center">
              <div class="flex-1 mr-4">
                <div class="text-sm font-bold text-slate-700">{{ item.product?.name }}</div>
                <div class="text-[10px] text-slate-400">SKU: {{ item.product?.sku || '-' }}</div>
              </div>
              <div class="text-right">
                <div class="text-sm font-black text-slate-800">¥{{ item.unit_price.toFixed(2) }}</div>
                <div class="text-[10px] text-slate-400">x{{ item.quantity }}</div>
              </div>
            </div>
          </div>
        </div>

        <!-- 业务追踪 -->
        <div class="grid grid-cols-2 gap-4">
          <div class="p-3 rounded-2xl border border-slate-100 bg-white">
            <div class="text-[10px] text-slate-400 mb-1">安装进度</div>
            <div class="text-sm font-bold">{{ currentDetailOrder.is_installed ? '✅ 已确认安装' : '⏳ 待确认' }}</div>
            <div v-if="currentDetailOrder.is_installed" class="text-[10px] text-slate-400 mt-1">
              师傅: {{ currentDetailOrder.installer?.name }}<br>
              时间: {{ formatDate(currentDetailOrder.install_time) }}
            </div>
          </div>
          <div class="p-3 rounded-2xl border border-slate-100 bg-white">
            <div class="text-[10px] text-slate-400 mb-1">国补资料</div>
            <div class="text-sm font-bold" :class="getSubsidyType(currentDetailOrder.subsidy_status)">
              {{ getSubsidyLabel(currentDetailOrder.subsidy_status) }}
            </div>
            <div v-if="currentDetailOrder.subsidy_amount > 0" class="text-[10px] text-orange-500 font-bold mt-1">
              补: ¥{{ currentDetailOrder.subsidy_amount.toFixed(2) }}
            </div>
          </div>
        </div>

        <!-- 财务摘要 -->
        <div class="bg-blue-600 rounded-2xl p-4 text-white shadow-lg shadow-blue-200">
          <div class="flex justify-between items-center mb-2">
            <span class="text-xs opacity-70">订单总金额</span>
            <span class="text-lg font-bold">¥{{ currentDetailOrder.actual_pay_amount.toFixed(2) }}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-xs opacity-70">{{ currentDetailOrder.payment_status === 1 ? '已收定金' : '实收总额' }}</span>
            <span class="text-2xl font-black">¥{{ (currentDetailOrder.payment_status === 1 ? currentDetailOrder.deposit_amount : currentDetailOrder.actual_pay_amount).toFixed(2) }}</span>
          </div>
        </div>
      </div>
      <template #footer>
        <el-button @click="detailDialogVisible = false" type="primary" class="w-full !h-12 !rounded-xl">返回列表</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Search, ShoppingCart, Delete, Switch, Plus, Back, List } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'

// 1. 登录锁定逻辑
const employees = ref([])
const currentEmployee = ref(null)
const selectedEmployee = ref(null)
const pin = ref('')
const verifying = ref(false)

// Attendance logic
const todayAttendance = ref(null)
const attendanceLoading = ref(false)

const fetchTodayAttendance = async () => {
  if (!currentEmployee.value) return
  try {
    const today = new Date().toISOString().split('T')[0]
    const res = await axios.get('/api/employees/attendance-logs', {
      params: { employee_id: currentEmployee.value.id, month: today.substring(0, 7) }
    })
    if (res.data.code === 200) {
      todayAttendance.value = res.data.data.find(log => log.date === today) || null
    }
  } catch (error) {
    console.error('Fetch attendance failed:', error)
  }
}

const handleCheckIn = async () => {
  attendanceLoading.value = true
  try {
    const res = await axios.post('/api/employees/check-in', {
      employee_id: currentEmployee.value.id
    })
    if (res.data.code === 200) {
      ElMessage.success('上班打卡成功')
      fetchTodayAttendance()
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '打卡失败')
  } finally {
    attendanceLoading.value = false
  }
}

const handleCheckOut = async () => {
  attendanceLoading.value = true
  try {
    const res = await axios.post('/api/employees/check-out', {
      employee_id: currentEmployee.value.id
    })
    if (res.data.code === 200) {
      ElMessage.success('下班打卡成功')
      fetchTodayAttendance()
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '打卡失败')
  } finally {
    attendanceLoading.value = false
  }
}

// 订单跟踪逻辑
const ordersDrawerVisible = ref(false)
const ordersLoading = ref(false)
const myOrders = ref([])
const orderSearchKeyword = ref('')
const installDialogVisible = ref(false)
const installTargetOrder = ref(null)
const selectedInstallerId = ref(null)
const detailDialogVisible = ref(false)
const currentDetailOrder = ref(null)

const openOrdersDrawer = () => {
  ordersDrawerVisible.value = true
  orderSearchKeyword.value = ''
  fetchMyOrders()
}

const formatDate = (date) => {
  if (!date) return '-'
  return dayjs(date).format('YYYY-MM-DD HH:mm')
}

let orderSearchTimer = null
const handleOrderSearchInput = () => {
  if (orderSearchTimer) clearTimeout(orderSearchTimer)
  orderSearchTimer = setTimeout(() => {
    fetchMyOrders()
  }, 500)
}

const fetchMyOrders = async () => {
  ordersLoading.value = true
  try {
    const params = {}
    if (orderSearchKeyword.value) {
      params.keyword = orderSearchKeyword.value
    }
    
    const res = await axios.get('/api/orders', { params })
    if (res.data.code === 200) {
      // 如果没有关键词，只看自己的；如果有关键词，则是全局搜索
      if (orderSearchKeyword.value) {
        myOrders.value = res.data.data
      } else {
        myOrders.value = res.data.data.filter(o => o.employee_id === currentEmployee.value.id)
      }
    }
  } catch (err) {
    ElMessage.error('获取订单失败')
  } finally {
    ordersLoading.value = false
  }
}

const handleStaffInstall = (order) => {
  installTargetOrder.value = order
  selectedInstallerId.value = null
  installDialogVisible.value = true
}

const confirmStaffInstall = async () => {
  if (!selectedInstallerId.value) return
  try {
    const res = await axios.put(`/api/orders/${installTargetOrder.value.id}/install`, {
      installer_id: selectedInstallerId.value
    })
    if (res.data.code === 200) {
      ElMessage.success('安装确认成功')
      installDialogVisible.value = false
      fetchMyOrders()
    }
  } catch (error) {
    ElMessage.error('操作失败')
  }
}

const getSubsidyLabel = (status) => {
  const map = { 0: '无', 1: '待提交', 2: '已提交', 3: '已回款' }
  return map[status] || '未知'
}

const getSubsidyType = (status) => {
  const map = { 0: 'info', 1: 'danger', 2: 'warning', 3: 'success' }
  return map[status] || 'info'
}

const confirmSubsidy = (order) => {
  ElMessageBox.confirm('确认已在政府系统提交该订单的补贴资料？', '资料提交确认').then(async () => {
    try {
      const res = await axios.put(`/api/orders/${order.id}/subsidy`, { target_status: 2 })
      if (res.data.code === 200) {
        ElMessage.success('状态更新成功')
        fetchMyOrders()
      }
    } catch (err) {
      ElMessage.error('操作失败')
    }
  })
}

const showOrderDetail = (order) => {
  currentDetailOrder.value = order
  detailDialogVisible.value = true
}

const fetchEmployees = async () => {
  const res = await axios.get('/api/employees')
  employees.value = res.data.data
}

const inputPin = (n) => {
  if (pin.value.length < 6) pin.value += n
}

const verifyPin = async () => {
  if (pin.value.length < 4) return
  verifying.value = true
  try {
    const res = await axios.post('/api/employees/verify-pin', {
      employee_id: selectedEmployee.value.id,
      pin_code: pin.value
    })
    if (res.data.code === 200) {
      currentEmployee.value = res.data.data
      // 持久化登录状态
      localStorage.setItem('pos_current_employee', JSON.stringify(currentEmployee.value))
      fetchProducts()
      fetchTodayAttendance()
      ElMessage.success(`欢迎回来，${currentEmployee.value.name}`)
    }
  } catch (err) {
    ElMessage.error('PIN 码错误')
    pin.value = ''
  } finally {
    verifying.value = false
  }
}

const logout = () => {
  currentEmployee.value = null
  selectedEmployee.value = null
  pin.value = ''
  cart.value = []
  localStorage.removeItem('pos_current_employee')
}

// 2. 收银核心逻辑 (复用 Cashier.vue)
const productList = ref([])
const categories = ref([])
const selectedCategoryId = ref('')
const searchQuery = ref('')
const cart = ref([])
const drawerVisible = ref(false)
const loading = ref(false)
const isMobile = ref(window.innerWidth < 768)

const customerName = ref('')
const customerPhone = ref('')
const paymentMethod = ref(1)
const deliveryMethod = ref(1)

const fetchCategories = async () => {
  try {
    const res = await axios.get('/api/categories')
    if (res.data.code === 200) {
      categories.value = res.data.data
    }
  } catch (error) {
    console.error('Fetch categories error:', error)
  }
}

const selectCategory = (id) => {
  selectedCategoryId.value = id
  fetchProducts()
}

const calculateMinPrice = (product) => {
  const cost = Number(product.latest_cost) || 0
  const rate = Number(product.margin_rate) || 0
  return cost / (1 - rate)
}

const fetchProducts = async () => {
  const res = await axios.get('/api/products', {
    params: { 
      keyword: searchQuery.value,
      category_id: selectedCategoryId.value
    }
  })
  productList.value = res.data.data
}

const addToCart = (product) => {
  const minPrice = calculateMinPrice(product)
  const existing = cart.value.find(item => item.id === product.id)
  if (existing) {
    existing.quantity++
    ElMessage.success(`已增加数量`)
  } else {
    cart.value.push({
      id: product.id,
      name: product.name,
      minPrice: minPrice,
      price: Math.ceil(minPrice),
      quantity: 1,
      store_stock: product.store_stock,
      main_stock: product.main_stock,
      isGift: false
    })
    ElMessage.success(`已加入购物车`)
  }
}

const removeFromCart = (index) => cart.value.splice(index, 1)
const openDrawer = () => drawerVisible.value = true

// 结算状态
const isSubsidized = ref(false)
const isDepositMode = ref(false)
const depositAmount = ref(0)

const originalTotal = computed(() => {
  return cart.value.reduce((sum, item) => sum + (item.isGift ? 0 : item.price * item.quantity), 0)
})

const subsidyAmount = computed(() => {
  if (!isSubsidized.value) return 0
  const total = originalTotal.value
  return total <= 10000 ? total * 0.15 : 1500
})

const totalPayable = computed(() => {
  return Math.max(0, originalTotal.value - subsidyAmount.value)
})

const actualReceipt = computed(() => {
  return isDepositMode.value ? depositAmount.value : totalPayable.value
})

const handleCheckout = async () => {
  if (!customerName.value) return ElMessage.warning('请输入客户姓名')
  
  try {
    const type = isDepositMode.value ? '定金' : '全款'
    const amount = actualReceipt.value.toFixed(2)
    await ElMessageBox.confirm(`确认收取客户 ${type} ¥${amount} 并提交订单？`, '结算确认')
    
    loading.value = true
    const payload = {
      items: cart.value.map(i => ({
        product_id: i.id,
        quantity: i.quantity,
        unit_price: i.price,
        is_gift: i.isGift
      })),
      is_subsidy: isSubsidized.value,
      deposit_amount: isDepositMode.value ? depositAmount.value : 0,
      customer_name: customerName.value,
      customer_phone: customerPhone.value,
      employee_id: currentEmployee.value.id,
      payment_method: paymentMethod.value,
      delivery_method: deliveryMethod.value
    }
    const res = await axios.post('/api/orders', payload)
    if (res.data.code === 200) {
      ElMessage.success('开单成功！')
      cart.value = []
      customerName.value = ''
      customerPhone.value = ''
      isSubsidized.value = false
      isDepositMode.value = false
      depositAmount.value = 0
      drawerVisible.value = false
      fetchProducts()
    }
  } catch (err) {
    if (err !== 'cancel') ElMessage.error('开单失败')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  // 恢复登录状态
  const savedEmployee = localStorage.getItem('pos_current_employee')
  if (savedEmployee) {
    try {
      currentEmployee.value = JSON.parse(savedEmployee)
      fetchProducts()
      fetchTodayAttendance()
    } catch (e) {
      localStorage.removeItem('pos_current_employee')
    }
  }

  fetchEmployees()
  fetchCategories()
  window.addEventListener('resize', () => {
    isMobile.value = window.innerWidth < 768
  })
})
</script>

<style scoped>
.pos-search-input :deep(.el-input__wrapper) {
  border-radius: 12px;
  background: #f8fafc;
}
.search-btn {
  border-radius: 12px;
  padding: 0 24px;
}
.price-warning :deep(.el-input__wrapper) {
  box-shadow: 0 0 0 1px #ef4444 inset !important;
}
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
