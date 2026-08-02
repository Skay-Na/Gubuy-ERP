<template>
  <div class="p-6 max-w-7xl mx-auto">
    <!-- Header Section -->
    <div class="mb-8">
      <div class="flex flex-col md:flex-row justify-between items-start md:items-end gap-4 mb-6">
        <div>
          <h1 class="text-2xl md:text-3xl font-black text-slate-900 tracking-tight mb-1 whitespace-nowrap">订单管理与历史</h1>
          <p class="text-sm text-slate-400">查看和管理所有的销售订单及款项流转</p>
        </div>
        <div class="hidden md:block w-full md:max-w-3xl">
          <div class="bg-white border border-slate-100 rounded-2xl p-4 shadow-sm space-y-3">
            <div class="flex items-center gap-3">
              <el-input 
                v-model="searchKeyword" 
                placeholder="搜索订单号、客户、电话、地址、商品、员工、老板客等" 
                clearable 
                size="large"
                class="flex-1"
                @input="handleSearchInput"
              >
                <template #prefix><el-icon><Search /></el-icon></template>
              </el-input>
              <el-button type="primary" size="large" @click="handleExport" class="shadow-sm font-bold">导出明细</el-button>
            </div>
            <div class="grid grid-cols-12 gap-3">
              <el-date-picker
                v-model="dateRange"
                type="daterange"
                range-separator="至"
                start-placeholder="开始日期"
                end-placeholder="结束日期"
                class="!w-full shadow-sm col-span-6"
                @change="fetchOrders"
              />
              <el-select
                v-model="selectedCategoryId"
                clearable
                placeholder="产品类型"
                class="!w-full shadow-sm col-span-3"
                @change="fetchOrders"
              >
                <el-option label="全部分类" value="" />
                <el-option v-for="category in categories" :key="category.id" :label="category.name" :value="category.id" />
              </el-select>
              <el-select
                v-model="selectedOrderStatus"
                clearable
                placeholder="订单状态"
                class="!w-full shadow-sm col-span-3"
                @change="fetchOrders"
              >
                <el-option v-for="option in orderStatusOptions" :key="option.value" :label="option.label" :value="option.value" />
              </el-select>
              <el-select
                v-model="selectedPaymentStatus"
                clearable
                placeholder="支付状态"
                class="!w-full shadow-sm col-span-4"
                @change="fetchOrders"
              >
                <el-option v-for="option in paymentStatusOptions" :key="option.value" :label="option.label" :value="option.value" />
              </el-select>
              <el-select
                v-model="selectedSubsidyStatus"
                clearable
                placeholder="国补状态"
                class="!w-full shadow-sm col-span-4"
                @change="fetchOrders"
              >
                <el-option v-for="option in subsidyStatusOptions" :key="option.value" :label="option.label" :value="option.value" />
              </el-select>
              <el-select
                v-model="selectedFulfillmentStatus"
                clearable
                placeholder="配送与安装"
                class="!w-full shadow-sm col-span-4"
                @change="fetchOrders"
              >
                <el-option v-for="option in fulfillmentStatusOptions" :key="option.value" :label="option.label" :value="option.value" />
              </el-select>
            </div>
          </div>
        </div>

        <div class="md:hidden w-full space-y-3">
          <div class="flex gap-2">
            <el-input
              v-model="searchKeyword"
              placeholder="搜客户/电话/订单"
              clearable
              class="flex-1"
              @input="handleSearchInput"
            >
              <template #prefix><el-icon><Search /></el-icon></template>
            </el-input>
            <el-button class="!rounded-xl" @click="filterDrawerVisible = true">筛选</el-button>
          </div>
          <div class="text-xs text-slate-400">{{ activeMobileTabLabel }} · 共 {{ orders.length }} 单</div>
        </div>
      </div>

      <!-- 快捷状态分类 Tab -->
      <div class="flex items-center gap-2 overflow-x-auto no-scrollbar pb-1">
        <div 
          v-for="tab in currentFilterTabs" 
          :key="tab.value"
          @click="selectTab(tab.value)"
          class="px-5 py-2 rounded-xl text-sm font-bold whitespace-nowrap cursor-pointer transition-all border"
          :class="activeTab === tab.value 
            ? 'bg-blue-600 text-white border-blue-600 shadow-md shadow-blue-100' 
            : 'bg-white text-slate-500 border-slate-100 hover:border-slate-200'"
        >
          {{ tab.label }}
        </div>
      </div>
    </div>

    <!-- Data Table Card -->
    <el-card shadow="never" class="!rounded-2xl border-slate-100 shadow-sm overflow-hidden" body-class="!p-0">
      <!-- PC View -->
      <el-table 
        :data="orders" 
        v-loading="loading" 
        style="width: 100%" 
        class="desktop-only hidden md:block"
        :header-cell-style="{ background: '#f8fafc', color: '#475569', fontWeight: '600', height: '54px' }"
        :row-style="{ height: '64px' }"
        :row-class-name="({row}) => row.order_status === 2 ? 'opacity-50 grayscale' : ''"
      >
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="px-8 py-6 bg-slate-50/50">
              <div class="flex justify-between items-center mb-4">
                <h4 class="text-base font-bold text-slate-800">订单深度明细</h4>
                <div class="flex gap-2">
                  <el-tag v-if="isBossCustomerOrder(row)" type="warning" effect="dark">老板客户</el-tag>
                  <el-tag v-if="row.order_status === 2" type="info" effect="dark">已废弃单据</el-tag>
                  <el-tag v-else type="success" effect="dark">有效业务单据</el-tag>
                </div>
              </div>

              <el-table :data="row.order_items" size="small" border class="shadow-sm rounded-lg overflow-hidden mb-6">
                <el-table-column label="商品信息" min-width="200">
                  <template #default="scope">
                    <div class="flex flex-col">
                      <span class="font-bold text-slate-700">{{ scope.row.product?.name || '未知商品' }}</span>
                      <span class="text-xs text-slate-400">{{ scope.row.product?.specification || '-' }} / {{ scope.row.product?.model || '-' }}</span>
                    </div>
                  </template>
                </el-table-column>
                <el-table-column label="单价" width="100" align="center">
                  <template #default="scope">
                    <span :class="{'line-through text-slate-400': scope.row.is_gift, 'font-bold': !scope.row.is_gift}">¥{{ scope.row.unit_price.toFixed(2) }}</span>
                    <el-tag v-if="scope.row.is_gift" size="small" type="danger" effect="dark" class="ml-1">赠</el-tag>
                  </template>
                </el-table-column>
                <el-table-column prop="quantity" label="数量" width="70" align="center" />
                <el-table-column label="小计" width="90" align="right">
                  <template #default="scope">
                    <span class="font-bold text-slate-700">¥{{ (scope.row.unit_price * scope.row.quantity).toFixed(2) }}</span>
                  </template>
                </el-table-column>
                <el-table-column label="成本核算" min-width="170" align="right">
                  <template #default="scope">
                    <div class="flex flex-col text-[11px] leading-tight">
                      <span class="font-bold text-blue-600">货品成本 ¥{{ getItemProductCost(scope.row).toFixed(2) }}</span>
                      <span class="text-slate-400">单件成本 ¥{{ getItemUnitCost(scope.row).toFixed(2) }}</span>
                      <span class="text-emerald-600">毛利 ¥{{ getItemGrossProfit(scope.row).toFixed(2) }}</span>
                    </div>
                  </template>
                </el-table-column>
                <el-table-column label="配送状态" min-width="160">
                  <template #default="scope">
                    <div v-if="scope.row.is_delivered" class="flex flex-col text-[11px] leading-tight">
                      <span class="text-blue-600 font-bold">🚚 已送货</span>
                      <span class="text-slate-500">送货: {{ scope.row.delivery_person?.name || '-' }} <el-tag v-if="scope.row.delivery_person?.is_outsourced" size="small" type="warning" class="!px-1 !py-0 !h-auto scale-90 origin-left">外包</el-tag></span>
                      <span class="text-slate-400">运费: ¥{{ scope.row.delivery_fee.toFixed(2) }}</span>
                    </div>
                    <span v-else class="text-slate-400">⏳ 待送货</span>
                  </template>
                </el-table-column>
                <el-table-column label="安装状态" min-width="160">
                  <template #default="scope">
                    <div v-if="scope.row.is_installed" class="flex flex-col text-[11px] leading-tight">
                      <span class="text-green-600 font-bold">✅ 已完成安装</span>
                      <span class="text-slate-500">师傅: {{ scope.row.installer?.name || '-' }} <el-tag v-if="scope.row.installer?.is_outsourced" size="small" type="success" class="!px-1 !py-0 !h-auto scale-90 origin-left">外包</el-tag></span>
                      <span class="text-slate-400">费用: ¥{{ scope.row.installation_fee.toFixed(2) }}</span>
                    </div>
                    <span v-else class="text-slate-400">⏳ 待安装</span>
                  </template>
                </el-table-column>
                <el-table-column label="履约操作" width="130" align="center" v-if="row.order_status !== 2">
                  <template #default="scope">
                    <span v-if="row.delivery_method === 1" class="text-slate-400">门店自提</span>
                    <div v-else class="flex flex-col gap-1 items-center">
                      <el-button size="small" type="warning" class="!rounded-md !px-2 !py-1" @click="handleDeliver(scope.row)" v-if="!scope.row.is_delivered">确认送货</el-button>
                      <el-button size="small" type="primary" class="!rounded-md !px-2 !py-1" @click="handleInstall(scope.row)" v-else-if="!scope.row.is_installed">确认安装</el-button>
                      <span v-else class="text-green-600 font-bold">🎉 已完工</span>
                    </div>
                  </template>
                </el-table-column>
              </el-table>
              
              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <!-- 核心财务信息 -->
                <el-descriptions title="核心账务" :column="1" border size="small" class="bg-white p-3 rounded-xl shadow-sm">
                  <el-descriptions-item label="订单总额">
                    <span class="font-bold text-slate-900">¥{{ row.total_amount.toFixed(2) }}</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="客户应付">
                    <span class="font-bold text-slate-900">¥{{ row.actual_pay_amount.toFixed(2) }}</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="实收金额">
                    <span class="text-blue-600 font-bold">¥{{ row.payment_status === 2 ? row.actual_pay_amount.toFixed(2) : row.deposit_amount.toFixed(2) }}</span>
                    <el-tag size="small" class="ml-2" :type="row.payment_status === 2 ? 'success' : 'warning'">
                      {{ row.payment_status === 2 ? '已结清' : '仅付定金' }}
                    </el-tag>
                  </el-descriptions-item>
                  <el-descriptions-item label="支付方式">
                    {{ getPaymentMethodLabel(row.payment_method) }}
                    <span v-if="row.account" class="text-slate-400 text-[10px] ml-1">({{ row.account.name }})</span>
                    <div v-if="row.payment_method === 4" class="text-slate-500 text-[11px] mt-1">{{ getPaymentBreakdownText(row.payment_breakdown) }}</div>
                  </el-descriptions-item>
                  <el-descriptions-item label="开单人员">
                    {{ row.employee?.name || '未知' }} (工号: {{ row.employee?.emp_no || '-' }})
                  </el-descriptions-item>
                  <el-descriptions-item v-if="isBossCustomerOrder(row)" label="业务归属">
                    <el-tag type="warning" effect="dark" size="small">老板客户</el-tag>
                    <span class="ml-2 text-amber-600 font-bold">不计员工业务提成</span>
                  </el-descriptions-item>
                </el-descriptions>

                <!-- 成本核算 -->
                <el-descriptions title="成本核算" :column="1" border size="small" class="bg-white p-3 rounded-xl shadow-sm">
                  <el-descriptions-item label="货品成本">
                    <span class="font-bold text-blue-600">¥{{ getOrderProductCost(row).toFixed(2) }}</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="履约成本">
                    <span class="font-bold text-orange-600">¥{{ getOrderFulfillmentCost(row).toFixed(2) }}</span>
                    <span class="text-slate-400 text-[10px] ml-1">配送+安装</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="其他成本">
                    <span class="font-bold text-orange-600">¥{{ getOrderOtherCost(row).toFixed(2) }}</span>
                    <span class="text-slate-400 text-[10px] ml-1">渠道+换新</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="综合毛利">
                    <span :class="getOrderGrossProfit(row) >= 0 ? 'text-emerald-600' : 'text-red-600'" class="font-black">¥{{ getOrderGrossProfit(row).toFixed(2) }}</span>
                    <span class="text-slate-400 text-[10px] ml-1">毛利率 {{ getOrderGrossMargin(row).toFixed(1) }}%</span>
                  </el-descriptions-item>
                </el-descriptions>

                <!-- 履约与售后 -->
                <el-descriptions title="履约与售后" :column="1" border size="small" class="bg-white p-3 rounded-xl shadow-sm">
                  <el-descriptions-item label="交付方式">
                    <el-tag size="small" effect="plain">{{ getDeliveryMethodLabel(row.delivery_method) }}</el-tag>
                  </el-descriptions-item>
                  <el-descriptions-item label="配送与安装">
                    <span v-if="row.is_installed" class="text-green-600 font-bold">✅ 已完成安装</span>
                    <span v-else-if="row.is_delivered" class="text-blue-600 font-bold">🚚 已送货 / ⏳ 待安装</span>
                    <span v-else class="text-slate-400">⏳ 待送货</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="送货人员" v-if="row.is_delivered">
                    <span>{{ row.delivery_person?.name || '-' }}</span>
                    <el-tag v-if="row.delivery_person?.is_outsourced" size="small" type="warning" effect="plain" class="ml-2 !rounded-md">外包</el-tag>
                  </el-descriptions-item>
                  <el-descriptions-item label="送货费用" v-if="row.is_delivered">
                    <span class="font-bold text-orange-600">¥{{ (row.delivery_fee || 0).toFixed(2) }}</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="安装人员" v-if="row.is_installed">
                    <span>{{ row.installer?.name || '-' }}</span>
                    <el-tag v-if="row.installer?.is_outsourced" size="small" type="success" effect="plain" class="ml-2 !rounded-md">外包</el-tag>
                  </el-descriptions-item>
                  <el-descriptions-item label="安装费用" v-if="row.is_installed">
                    <span class="font-bold text-orange-600">¥{{ (row.installation_fee || 0).toFixed(2) }}</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="安装时间" v-if="row.is_installed">
                    {{ formatDate(row.install_time) }}
                  </el-descriptions-item>

                </el-descriptions>

                <!-- 国补资料 -->
                <el-descriptions title="国补政策" :column="1" border size="small" class="bg-white p-3 rounded-xl shadow-sm">
                  <el-descriptions-item label="是否包含补贴">
                    {{ row.subsidy_amount > 0 ? '是' : '否' }}
                  </el-descriptions-item>
                  <el-descriptions-item label="补贴金额" v-if="row.subsidy_amount > 0">
                    <span class="text-orange-500 font-bold">¥{{ row.subsidy_amount.toFixed(2) }}</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="资料进度" v-if="row.subsidy_amount > 0">
                    <el-tag :type="getSubsidyType(row.subsidy_status)">{{ getSubsidyLabel(row.subsidy_status) }}</el-tag>
                  </el-descriptions-item>
                </el-descriptions>

                <!-- 以旧换新 -->
                <el-descriptions v-if="row.trade_in_amount > 0" title="以旧换新" :column="1" border size="small" class="bg-white p-3 rounded-xl shadow-sm">
                  <el-descriptions-item label="支出金额">
                    <span class="text-emerald-600 font-bold">¥{{ row.trade_in_amount.toFixed(2) }}</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="旧件品类">
                    <el-tag size="small" type="success" effect="plain" class="!rounded-md">{{ row.trade_in_type || '未指定' }}</el-tag>
                  </el-descriptions-item>
                  <el-descriptions-item label="折旧备注">
                    <span class="text-slate-500 font-medium">{{ row.trade_in_remark || '无特殊描述' }}</span>
                  </el-descriptions-item>
                </el-descriptions>

                <!-- 客户与市场 -->
                <el-descriptions title="客户与市场" :column="1" border size="small" class="bg-white p-3 rounded-xl shadow-sm">
                  <el-descriptions-item label="客户名称">{{ row.customer_name || '散客' }}</el-descriptions-item>
                  <el-descriptions-item label="联系电话">{{ row.customer_phone || '-' }}</el-descriptions-item>
                  <el-descriptions-item label="收货地址">{{ row.customer_address || '-' }}</el-descriptions-item>
                  <el-descriptions-item label="推荐渠道">{{ row.referrer_name || '无' }}</el-descriptions-item>
                  <el-descriptions-item v-if="isBossCustomerOrder(row)" label="客户来源">
                    <span class="font-bold text-amber-600">老板自带客户</span>
                  </el-descriptions-item>
                  <el-descriptions-item label="渠道奖励">
                    ¥{{ (row.referral_fee || 0).toFixed(2) }} 
                    <el-tag v-if="row.referral_fee > 0" size="small" :type="row.is_referral_fee_paid ? 'success' : 'info'" class="ml-2">
                      {{ row.is_referral_fee_paid ? '已发放' : '待结算' }}
                    </el-tag>
                    <el-button text type="primary" size="small" class="ml-2" @click="openReferralFeeDialog(row)">调整</el-button>
                  </el-descriptions-item>
                </el-descriptions>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="order_no" label="订单号" min-width="180" />
        <el-table-column label="客户信息" min-width="150">
          <template #default="{ row }">
            <div class="flex flex-col gap-0.5">
              <div class="flex items-center gap-1">
                <span class="font-bold text-slate-700">{{ row.customer_name || '散客' }}</span>
                <el-tag v-if="row.trade_in_amount > 0" size="small" type="success" effect="plain" class="!rounded-md !px-1 shrink-0">♻️ 换新</el-tag>
                <el-tag v-if="isBossCustomerOrder(row)" size="small" type="warning" effect="plain" class="!rounded-md !px-1 shrink-0">老板客</el-tag>
                <el-tag v-if="row.order_type === 2" size="small" type="danger" effect="plain" class="!rounded-md !px-1 shrink-0">商会订单</el-tag>
              </div>
              <span class="text-[10px] text-slate-400 font-mono">{{ row.customer_phone || '-' }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="商品信息" min-width="200">
          <template #default="{ row }">
            <div class="flex flex-col gap-1 my-1">
              <div v-for="item in row.order_items" :key="item.id" class="text-xs leading-tight">
                <span class="font-bold text-slate-700">{{ item.product?.name || '未知商品' }}</span>
                <span class="text-slate-500 ml-1">({{ item.product?.category?.name || getProductCategoryName(item.product?.category_id) }})</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="订单金额" width="120">
          <template #default="{ row }">
            <span class="font-bold text-slate-900">¥{{ row.total_amount.toFixed(2) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="业务提成" width="120">
          <template #default="{ row }">
            <div class="flex items-center gap-1 cursor-pointer group" @click="openCommissionDialog(row)">
              <span class="font-bold text-orange-600">¥{{ (row.commission_amount || 0).toFixed(2) }}</span>
              <el-tag v-if="isBossCustomerOrder(row)" size="small" type="warning" effect="plain" class="!h-5">老板客</el-tag>
              <el-icon class="text-slate-300 group-hover:text-blue-500 transition-colors"><Edit /></el-icon>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="支付状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.order_status === 2" type="info">已取消</el-tag>
            <el-tag v-else :type="row.payment_status === 2 ? 'success' : 'warning'">
              {{ row.payment_status === 1 ? '定金' : '全款' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="配送与安装" width="140" align="center">
          <template #default="{ row }">
            <template v-if="row.order_status !== 2">
              <el-tag v-if="row.is_installed" type="success" effect="plain">已安装</el-tag>
              <el-tag v-else-if="row.is_delivered" type="primary" effect="plain">已送货/待安装</el-tag>
              <el-tag v-else-if="row.order_items && row.order_items.some(it => it.is_delivered || it.is_installed)" type="warning" effect="plain">部分履约</el-tag>
              <el-tag v-else type="info" effect="plain">待送货</el-tag>
            </template>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="国补状态" width="110" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.order_status !== 2 && row.subsidy_status !== 0" :type="getSubsidyType(row.subsidy_status)" effect="light">
              {{ getSubsidyLabel(row.subsidy_status) }}
            </el-tag>
            <span v-else-if="row.subsidy_status === 0" class="text-xs text-slate-300">无补贴</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right" align="center">
          <template #default="{ row }">
            <div v-if="row.order_status === 1" class="flex items-center justify-center gap-2">
              <el-button v-slot v-if="row.payment_status === 1" type="success" size="small" @click="handlePayBalance(row)">结清</el-button>
              <el-dropdown trigger="click">
                <el-button size="small" class="px-2"><el-icon><MoreFilled /></el-icon></el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item @click="openReceiptPreview(row)">打印小票</el-dropdown-item>
                    <el-dropdown-item v-if="row.subsidy_status === 1" @click="handleSubmitSubsidy(row)">提交国补</el-dropdown-item>
                    <el-dropdown-item @click="openCommissionDialog(row)">调整提成</el-dropdown-item>
                    <el-dropdown-item @click="openReferralFeeDialog(row)">调整推荐人提成</el-dropdown-item>
                    <el-dropdown-item v-if="isOrderFulfilled(row)" divided type="danger" @click="handleReversalCancel(row)" class="!text-red-500">红冲重开</el-dropdown-item>
                    <el-dropdown-item v-else divided type="danger" @click="handleCancel(row)" class="!text-red-500">取消订单</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
            <span v-else class="text-xs text-slate-400">-</span>
          </template>
        </el-table-column>
      </el-table>

      <!-- Mobile View -->
      <div class="mobile-only md:hidden p-3 space-y-3 bg-slate-50" v-loading="loading">
        <div
          v-for="row in orders"
          :key="row.id"
          class="bg-white border border-slate-100 rounded-2xl p-4 shadow-sm active:scale-[0.99] transition-transform"
          :class="row.order_status === 2 ? 'opacity-60' : ''"
        >
          <div class="flex justify-between items-start gap-3 mb-3">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 mb-1">
                <div class="font-black text-slate-900 text-base truncate">{{ row.customer_name || '散客' }}</div>
                <el-tag v-if="isBossCustomerOrder(row)" size="small" type="warning" effect="plain" class="!rounded-md">老板客</el-tag>
              </div>
              <div class="text-[10px] text-slate-400 font-mono truncate">{{ row.order_no }}</div>
              <div class="text-xs text-slate-400 mt-1">{{ row.customer_phone || '无电话' }}</div>
            </div>
            <div class="text-right shrink-0">
              <div class="text-slate-950 font-black text-lg leading-none">¥{{ row.total_amount.toFixed(2) }}</div>
              <div class="text-[10px] text-slate-400 mt-1">{{ formatDate(row.created_at).slice(5, 16) }}</div>
            </div>
          </div>

          <div class="flex flex-wrap gap-1.5 mb-3">
            <el-tag v-for="tag in getOrderStatusTags(row)" :key="tag.label" size="small" :type="tag.type" effect="light" class="!rounded-md">
              {{ tag.label }}
            </el-tag>
          </div>

          <button class="w-full text-left bg-slate-50 border border-slate-100 rounded-xl px-3 py-2 mb-3" @click="openOrderDetail(row)">
            <div class="text-xs font-bold text-slate-700 truncate">{{ getOrderProductSummary(row) }}</div>
            <div class="text-[10px] text-slate-400 mt-1">点开查看商品、账务、履约和更多操作</div>
          </button>

          <div v-if="row.order_status === 1" class="grid grid-cols-[1fr_1.4fr] gap-2">
            <el-button size="small" class="!rounded-xl" @click="openOrderDetail(row)">详情</el-button>
            <el-button size="small" :type="getPrimaryAction(row).type" class="!rounded-xl !font-bold" @click="runPrimaryAction(row)">
              {{ getPrimaryAction(row).label }}
            </el-button>
          </div>
          <div v-else class="grid grid-cols-2 gap-2">
            <el-button size="small" class="!rounded-xl" @click="openOrderDetail(row)">详情</el-button>
            <el-button size="small" class="!rounded-xl" @click="openReceiptPreview(row)">打印小票</el-button>
          </div>
        </div>
        <el-empty v-if="!loading && orders.length === 0" description="暂无订单" />
      </div>
    </el-card>

    <!-- 结清尾款弹窗 -->
    <el-dialog v-model="balanceDialogVisible" title="结清尾款" width="500px" destroy-on-close class="!rounded-2xl">
      <template v-if="balanceTargetOrder">
        <el-alert title="结算说明" type="info" :closable="false" show-icon class="mb-4 !rounded-xl">系统将按"退定金 → 收全款"两步完成结算：账户余额先减后增，财务上等价于一次性收取全款（订单净收款 = actual_pay_amount）。</el-alert>
        <el-descriptions :column="1" border class="mb-4">
          <el-descriptions-item label="订单号">{{ balanceTargetOrder.order_no }}</el-descriptions-item>
          <el-descriptions-item label="客户">{{ balanceTargetOrder.customer_name || '散客' }}</el-descriptions-item>
          <el-descriptions-item label="待收全款"><span class="text-green-600 font-bold">¥{{ balanceTargetOrder.actual_pay_amount.toFixed(2) }}</span></el-descriptions-item>
        </el-descriptions>
        <div class="bg-slate-50 p-4 rounded-xl border border-slate-100">
          <label class="block text-sm font-bold text-slate-700 mb-2">确认全款支付方式</label>
          <el-select v-model="finalPaymentMethod" class="w-full">
            <el-option v-for="opt in paymentMethodOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </div>
      </template>
      <template #footer>
        <el-button @click="balanceDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="confirmPayBalance">确认结算</el-button>
      </template>
    </el-dialog>

    <!-- 确认送货弹窗 -->
    <el-dialog v-model="deliveryDialogVisible" title="确认送货完成" width="400px" destroy-on-close class="!rounded-2xl">
      <div class="space-y-4">
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">选择送货人员</label>
          <el-select v-model="selectedDeliveryPersonId" class="w-full" placeholder="请选择送货员" filterable clearable>
            <el-option-group v-for="group in deliveryPersonOptions" :key="group.label" :label="group.label">
              <el-option v-for="emp in group.options" :key="emp.id" :label="emp.name" :value="emp.id" />
            </el-option-group>
          </el-select>
        </div>
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">送货时间</label>
          <input
            v-if="isMobileView"
            v-model="mobileDeliveryTime"
            type="datetime-local"
            inputmode="none"
            aria-label="送货时间"
            class="mobile-native-date-input"
          />
          <el-date-picker
            v-else
            v-model="selectedDeliveryTime"
            type="datetime"
            placeholder="选择送货时间"
            class="!w-full"
            format="YYYY-MM-DD HH:mm"
          />
        </div>
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">实际送货费用 (支出)</label>
          <el-input-number v-model="deliveryFee" :min="0" :precision="2" class="!w-full" controls-position="right" />
        </div>
      </div>
      <template #footer>
        <el-button @click="deliveryDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="confirmDeliver" :disabled="!selectedDeliveryPersonId">确认提交</el-button>
      </template>
    </el-dialog>

    <!-- 确认安装弹窗 -->
    <el-dialog v-model="installDialogVisible" title="确认安装完成" width="400px" destroy-on-close class="!rounded-2xl">
      <div class="space-y-4">
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">选择安装师傅</label>
          <el-select v-model="selectedInstallerId" class="w-full" placeholder="请选择师傅" filterable clearable>
            <el-option-group v-for="group in installerOptions" :key="group.label" :label="group.label">
              <el-option v-for="emp in group.options" :key="emp.id" :label="emp.name" :value="emp.id" />
            </el-option-group>
          </el-select>
        </div>
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">安装时间</label>
          <input
            v-if="isMobileView"
            v-model="mobileInstallTime"
            type="datetime-local"
            inputmode="none"
            aria-label="安装时间"
            class="mobile-native-date-input"
          />
          <el-date-picker
            v-else
            v-model="selectedInstallTime"
            type="datetime"
            placeholder="选择安装时间"
            class="!w-full"
            format="YYYY-MM-DD HH:mm"
          />
        </div>
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">实际安装费用 (支出)</label>
          <el-input-number v-model="installationFee" :min="0" :precision="2" class="!w-full" controls-position="right" />
        </div>
      </div>
      <template #footer>
        <el-button @click="installDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="confirmInstall" :disabled="!selectedInstallerId">确认提交</el-button>
      </template>
    </el-dialog>

    <!-- 调整提成弹窗 -->
    <el-dialog v-if="!isMobileView" v-model="commissionDialogVisible" title="调整业务提成" width="400px" destroy-on-close class="!rounded-2xl">
      <div v-if="commissionTargetOrder" class="space-y-4">
        <el-alert title="提示" type="warning" :closable="false" show-icon class="!rounded-xl">请根据与店长/员工商定的最终结果进行手动调整。</el-alert>
        <div class="bg-slate-50 p-4 rounded-xl border border-slate-100">
          <div class="text-xs text-slate-400 mb-2 uppercase font-bold tracking-wider">订单号: {{ commissionTargetOrder.order_no }}</div>
          <div class="text-sm font-bold text-slate-700 mb-4">当前订单总额: ¥{{ commissionTargetOrder.total_amount.toFixed(2) }}</div>
          
          <label class="block text-sm font-bold text-slate-700 mb-2">最终提成金额 (元)</label>
          <el-input-number v-model="newCommissionAmount" :min="0" :precision="2" class="!w-full" controls-position="right" />
        </div>
      </div>
      <template #footer>
        <el-button @click="commissionDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="confirmUpdateCommission">确认修改</el-button>
      </template>
    </el-dialog>

    <el-drawer v-if="isMobileView" v-model="commissionDialogVisible" title="调整业务提成" direction="btt" size="75%" class="md:hidden">
      <div v-if="commissionTargetOrder" class="h-full flex flex-col">
        <div class="flex-1 space-y-4">
          <div class="rounded-2xl bg-orange-50 border border-orange-100 p-3 text-sm text-orange-700">
            根据最终商定结果填写提成金额，保存后会同步刷新订单列表。
          </div>
          <div class="bg-slate-50 p-4 rounded-2xl border border-slate-100">
            <div class="text-xs text-slate-400 mb-2 font-bold">订单号</div>
            <div class="font-mono text-xs text-slate-600 break-all mb-4">{{ commissionTargetOrder.order_no }}</div>
            <div class="text-xs text-slate-400 mb-1 font-bold">订单总额</div>
            <div class="text-xl font-black text-slate-900 mb-4">¥{{ commissionTargetOrder.total_amount.toFixed(2) }}</div>
            <label class="block text-sm font-bold text-slate-700 mb-2">最终提成金额</label>
            <el-input-number v-model="newCommissionAmount" :min="0" :precision="2" class="!w-full" controls-position="right" />
          </div>
        </div>
        <div class="grid grid-cols-2 gap-2 pt-3 pb-[env(safe-area-inset-bottom)]">
          <el-button @click="commissionDialogVisible = false">取消</el-button>
          <el-button type="primary" @click="confirmUpdateCommission">保存提成</el-button>
        </div>
      </div>
    </el-drawer>

    <el-dialog v-if="!isMobileView" v-model="referralFeeDialogVisible" title="调整推荐人提成" width="400px" destroy-on-close class="!rounded-2xl">
      <div v-if="referralFeeTargetOrder" class="space-y-4">
        <el-alert
          :title="referralFeeTargetOrder.is_referral_fee_paid ? '推荐人提成已发放，不可直接修改' : '保存后会同步刷新订单列表'"
          :type="referralFeeTargetOrder.is_referral_fee_paid ? 'warning' : 'info'"
          :closable="false"
          show-icon
          class="!rounded-xl"
        />
        <div class="bg-slate-50 p-4 rounded-xl border border-slate-100">
          <div class="text-xs text-slate-400 mb-2 uppercase font-bold tracking-wider">订单号: {{ referralFeeTargetOrder.order_no }}</div>
          <div class="text-sm text-slate-600 mb-2">推荐人: {{ referralFeeTargetOrder.referrer_name || '未填写推荐人' }}</div>
          <div class="text-sm font-bold text-slate-700 mb-4">当前订单总额: ¥{{ referralFeeTargetOrder.total_amount.toFixed(2) }}</div>
          <label class="block text-sm font-bold text-slate-700 mb-2">推荐人提成金额 (元)</label>
          <el-input-number v-model="newReferralFee" :min="0" :precision="2" :disabled="referralFeeTargetOrder.is_referral_fee_paid" class="!w-full" controls-position="right" />
        </div>
      </div>
      <template #footer>
        <el-button @click="referralFeeDialogVisible = false">取消</el-button>
        <el-button type="primary" :disabled="referralFeeTargetOrder?.is_referral_fee_paid" @click="confirmUpdateReferralFee">确认修改</el-button>
      </template>
    </el-dialog>

    <el-drawer v-if="isMobileView" v-model="referralFeeDialogVisible" title="调整推荐人提成" direction="btt" size="75%" class="md:hidden">
      <div v-if="referralFeeTargetOrder" class="h-full flex flex-col">
        <div class="flex-1 space-y-4">
          <el-alert
            :title="referralFeeTargetOrder.is_referral_fee_paid ? '推荐人提成已发放，不可直接修改' : '保存后会同步刷新订单列表'"
            :type="referralFeeTargetOrder.is_referral_fee_paid ? 'warning' : 'info'"
            :closable="false"
            show-icon
            class="!rounded-2xl"
          />
          <div class="bg-slate-50 p-4 rounded-2xl border border-slate-100">
            <div class="text-xs text-slate-400 mb-1 font-bold">订单号</div>
            <div class="font-mono text-xs text-slate-600 break-all mb-3">{{ referralFeeTargetOrder.order_no }}</div>
            <div class="text-xs text-slate-400 mb-1 font-bold">推荐人</div>
            <div class="text-sm font-bold text-slate-700 mb-3">{{ referralFeeTargetOrder.referrer_name || '未填写推荐人' }}</div>
            <label class="block text-sm font-bold text-slate-700 mb-2">推荐人提成金额</label>
            <el-input-number v-model="newReferralFee" :min="0" :precision="2" :disabled="referralFeeTargetOrder.is_referral_fee_paid" class="!w-full" controls-position="right" />
          </div>
        </div>
        <div class="grid grid-cols-2 gap-2 pt-3 pb-[env(safe-area-inset-bottom)]">
          <el-button @click="referralFeeDialogVisible = false">取消</el-button>
          <el-button type="primary" :disabled="referralFeeTargetOrder.is_referral_fee_paid" @click="confirmUpdateReferralFee">保存提成</el-button>
        </div>
      </div>
    </el-drawer>

    <el-drawer v-model="filterDrawerVisible" title="筛选订单" direction="btt" size="78%" class="md:hidden">
      <div class="space-y-4">
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">日期范围</label>
          <div class="grid grid-cols-2 gap-2">
            <input
              :value="dateRange[0] ? dayjs(dateRange[0]).format('YYYY-MM-DD') : ''"
              type="date"
              inputmode="none"
              aria-label="开始日期"
              class="mobile-native-date-input"
              @change="updateMobileDateRange(0, $event.target.value)"
            />
            <input
              :value="dateRange[1] ? dayjs(dateRange[1]).format('YYYY-MM-DD') : ''"
              type="date"
              inputmode="none"
              aria-label="结束日期"
              class="mobile-native-date-input"
              @change="updateMobileDateRange(1, $event.target.value)"
            />
          </div>

        </div>
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">产品类型</label>
          <el-select v-model="selectedCategoryId" clearable placeholder="全部分类" class="!w-full">
            <el-option label="全部分类" value="" />
            <el-option v-for="category in categories" :key="category.id" :label="category.name" :value="category.id" />
          </el-select>
        </div>
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">支付状态</label>
          <el-select v-model="selectedPaymentStatus" clearable placeholder="全部支付状态" class="!w-full">
            <el-option v-for="option in paymentStatusOptions" :key="option.value" :label="option.label" :value="option.value" />
          </el-select>
        </div>
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">国补状态</label>
          <el-select v-model="selectedSubsidyStatus" clearable placeholder="全部国补状态" class="!w-full">
            <el-option v-for="option in subsidyStatusOptions" :key="option.value" :label="option.label" :value="option.value" />
          </el-select>
        </div>
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">订单状态</label>
          <el-select v-model="selectedOrderStatus" clearable placeholder="全部订单状态" class="!w-full">
            <el-option v-for="option in orderStatusOptions" :key="option.value" :label="option.label" :value="option.value" />
          </el-select>
        </div>
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">配送与安装</label>
          <el-select v-model="selectedFulfillmentStatus" clearable placeholder="全部配送与安装" class="!w-full">
            <el-option v-for="option in fulfillmentStatusOptions" :key="option.value" :label="option.label" :value="option.value" />
          </el-select>
        </div>
        <div class="grid grid-cols-2 gap-2 pt-2">
          <el-button @click="resetMobileFilters">重置</el-button>
          <el-button type="primary" @click="applyMobileFilters">应用筛选</el-button>
        </div>
        <el-button class="w-full" @click="handleExport">导出明细</el-button>
      </div>
    </el-drawer>

    <el-drawer v-model="orderDetailVisible" direction="btt" size="86%" class="md:hidden" :with-header="false">
      <div v-if="detailOrder" class="h-full flex flex-col">
        <div class="flex items-start justify-between gap-3 pb-3 border-b border-slate-100">
          <div class="min-w-0">
            <div class="font-black text-lg text-slate-900 truncate">{{ detailOrder.customer_name || '散客' }}</div>
            <div class="text-xs text-slate-400 font-mono truncate">{{ detailOrder.order_no }}</div>
            <div class="text-xs text-slate-400 mt-1">{{ detailOrder.customer_phone || '-' }}</div>
          </div>
          <div class="text-right shrink-0">
            <div class="font-black text-xl text-slate-950">¥{{ detailOrder.total_amount.toFixed(2) }}</div>
            <el-button size="small" text @click="orderDetailVisible = false">关闭</el-button>
          </div>
        </div>

        <div class="flex-1 overflow-y-auto py-3 space-y-3">
          <div class="bg-slate-50 rounded-xl p-3">
            <div class="text-xs font-black text-slate-500 mb-2">商品明细</div>
            <div v-for="item in detailOrder.order_items" :key="item.id" class="flex justify-between gap-3 py-2 border-b border-slate-100 last:border-0">
              <div class="min-w-0">
                <div class="font-bold text-sm text-slate-800">{{ item.product?.name || '未知商品' }}</div>
                <div class="text-xs text-slate-400">x{{ item.quantity }} · ¥{{ item.unit_price.toFixed(2) }}</div>
                <div class="text-[10px] text-blue-500 mt-0.5">成本 ¥{{ getItemProductCost(item).toFixed(2) }} · 毛利 ¥{{ getItemGrossProfit(item).toFixed(2) }}</div>
              </div>
              <div class="flex gap-1 items-center shrink-0">
                <el-button v-if="!item.is_delivered && detailOrder.delivery_method !== 1" size="small" type="warning" class="!rounded-md" @click="handleDeliver(item)">送货</el-button>
                <el-button v-if="item.is_delivered && !item.is_installed && detailOrder.delivery_method !== 1" size="small" type="primary" class="!rounded-md" @click="handleInstall(item)">安装</el-button>
                <span v-if="item.is_installed" class="text-xs font-bold text-green-600">已装</span>
                <span v-else-if="item.is_delivered" class="text-xs font-bold text-blue-600">已送</span>
              </div>
            </div>
          </div>

          <div class="grid grid-cols-2 gap-2">
            <div class="bg-white border border-slate-100 rounded-xl p-3">
              <div class="text-xs text-slate-400">客户应付</div>
              <div class="font-black text-slate-900">¥{{ detailOrder.actual_pay_amount.toFixed(2) }}</div>
            </div>
            <div class="bg-white border border-slate-100 rounded-xl p-3">
              <div class="text-xs text-slate-400">支付状态</div>
              <div class="font-black text-slate-900">{{ detailOrder.payment_status === 2 ? '已结清' : '仅付定金' }}</div>
            </div>
            <div class="bg-white border border-slate-100 rounded-xl p-3">
              <div class="text-xs text-slate-400">国补</div>
              <div class="font-black text-slate-900">{{ getSubsidyLabel(detailOrder.subsidy_status) }}</div>
            </div>
            <div class="bg-white border border-slate-100 rounded-xl p-3">
              <div class="text-xs text-slate-400">交付</div>
              <div class="font-black text-slate-900">{{ getDeliveryMethodLabel(detailOrder.delivery_method) }}</div>
            </div>
            <div class="bg-white border border-slate-100 rounded-xl p-3">
              <div class="text-xs text-slate-400">货品成本</div>
              <div class="font-black text-blue-600">¥{{ getOrderProductCost(detailOrder).toFixed(2) }}</div>
            </div>
            <div class="bg-white border border-slate-100 rounded-xl p-3">
              <div class="text-xs text-slate-400">综合毛利</div>
              <div :class="getOrderGrossProfit(detailOrder) >= 0 ? 'text-emerald-600' : 'text-red-600'" class="font-black">¥{{ getOrderGrossProfit(detailOrder).toFixed(2) }}</div>
              <div class="text-[10px] text-slate-400">毛利率 {{ getOrderGrossMargin(detailOrder).toFixed(1) }}%</div>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-2 pt-3 pb-[env(safe-area-inset-bottom)] border-t border-slate-100">
          <el-button @click="openReceiptPreview(detailOrder)">打印小票</el-button>
          <el-button v-if="detailOrder.subsidy_status === 1" type="warning" @click="handleSubmitSubsidy(detailOrder)">提交国补</el-button>
          <el-button v-else @click="openCommissionDialog(detailOrder)">调整提成</el-button>
          <el-button @click="openReferralFeeDialog(detailOrder)">调整推荐人提成</el-button>
          <el-button v-if="detailOrder.order_status === 1 && isOrderFulfilled(detailOrder)" type="danger" plain @click="handleReversalCancel(detailOrder)">红冲重开</el-button>
          <el-button v-else-if="detailOrder.order_status === 1" type="danger" plain @click="handleCancel(detailOrder)">取消订单</el-button>
        </div>
      </div>
    </el-drawer>

    <receipt-preview v-model="receiptVisible" :receipt="currentReceipt" />
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import request from '../utils/request'
import { downloadExcel } from '../utils/export'
import { buildReceiptFromOrder } from '../utils/receipt'
import ReceiptPreview from '../components/ReceiptPreview.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search, InfoFilled, MoreFilled, Edit } from '@element-plus/icons-vue'
import dayjs from 'dayjs'

const orders = ref([])
const allOrders = ref([])
const loading = ref(false)
const searchKeyword = ref('')
const dateRange = ref([])
const activeTab = ref('all')
const categories = ref([])
const selectedCategoryId = ref('')
const selectedPaymentStatus = ref('')
const selectedFulfillmentStatus = ref('')
const selectedSubsidyStatus = ref('')
const selectedOrderStatus = ref('')
const receiptVisible = ref(false)
const currentReceipt = ref(null)
const filterDrawerVisible = ref(false)
const orderDetailVisible = ref(false)
const detailOrder = ref(null)
const viewportWidth = ref(typeof window === 'undefined' ? 1024 : window.innerWidth)
const isMobileView = computed(() => viewportWidth.value < 768)
const handleViewportResize = () => {
  viewportWidth.value = window.innerWidth
}

const filterTabs = [
  { label: '全部订单', value: 'all' },
  { label: '待处理', value: 'todo' },
  { label: '待收尾款', value: 'pending_balance' },
  { label: '待送货', value: 'pending_delivery' },
  { label: '待安装', value: 'pending_install' },
  { label: '待提交国补', value: 'pending_subsidy' },
]

const mobileFilterTabs = [
  { label: '待处理', value: 'todo' },
  { label: '待收款', value: 'pending_balance' },
  { label: '待送货', value: 'pending_delivery' },
  { label: '待安装', value: 'pending_install' },
  { label: '国补', value: 'pending_subsidy' },
  { label: '全部', value: 'all' },
]

const paymentStatusOptions = [
  { label: '全部支付状态', value: '' },
  { label: '仅付定金', value: '1' },
  { label: '已结全款', value: '2' },
]

const subsidyStatusOptions = [
  { label: '全部国补状态', value: '' },
  { label: '无补贴', value: '0' },
  { label: '待提交', value: '1' },
  { label: '已提交', value: '2' },
  { label: '已回款', value: '3' },
]

const orderStatusOptions = [
  { label: '全部订单状态', value: '' },
  { label: '正常', value: '1' },
  { label: '已取消', value: '2' },
]

const fulfillmentStatusOptions = [
  { label: '全部配送与安装', value: '' },
  { label: '待送货', value: 'pending_delivery' },
  { label: '待安装', value: 'pending_install' },
  { label: '已安装', value: 'installed' },
  { label: '部分履约', value: 'partial' },
  { label: '门店自提', value: 'pickup' },
]

const currentFilterTabs = computed(() => isMobileView.value ? mobileFilterTabs : filterTabs)
const activeMobileTabLabel = computed(() => (currentFilterTabs.value.find(tab => tab.value === activeTab.value)?.label || '订单'))
const mobileDeliveryTime = computed({
  get: () => selectedDeliveryTime.value ? dayjs(selectedDeliveryTime.value).format('YYYY-MM-DDTHH:mm') : '',
  set: value => { selectedDeliveryTime.value = value ? new Date(value) : null }
})
const mobileInstallTime = computed({
  get: () => selectedInstallTime.value ? dayjs(selectedInstallTime.value).format('YYYY-MM-DDTHH:mm') : '',
  set: value => { selectedInstallTime.value = value ? new Date(value) : null }
})

const updateMobileDateRange = (index, value) => {
  const range = [dateRange.value?.[0] || '', dateRange.value?.[1] || '']
  range[index] = value
  dateRange.value = range.some(Boolean) ? range : []
}

const selectTab = (val) => {
  activeTab.value = val
  fetchOrders()
}

let searchTimer = null
const handleSearchInput = () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    fetchOrders()
  }, 500)
}

const buildOrderQueryParams = () => {
  const params = {
    keyword: searchKeyword.value,
    category_id: selectedCategoryId.value,
    payment_status: selectedPaymentStatus.value,
    fulfillment_status: selectedFulfillmentStatus.value,
    subsidy_status: selectedSubsidyStatus.value,
    order_status: selectedOrderStatus.value,
  }
  if (dateRange.value?.[0] && dateRange.value?.[1]) {
    params.start_date = dayjs(dateRange.value[0]).startOf('day').toISOString()
    params.end_date = dayjs(dateRange.value[1]).endOf('day').toISOString()
  }
  Object.keys(params).forEach(key => {
    if (params[key] === '' || params[key] === null || params[key] === undefined) delete params[key]
  })
  return params
}

const handleExport = () => {
  const params = buildOrderQueryParams()
  downloadExcel('/api/orders/export', params)
}

const fetchCategories = async () => {
  try {
    const res = await request.get('/api/categories')
    if (res.code === 200) {
      categories.value = res.result || []
    }
  } catch (error) {
    ElMessage.error('获取商品分类失败')
  }
}

const fetchOrders = async () => {
  loading.value = true
  try {
    const params = buildOrderQueryParams()

    const res = await request.get('/api/orders', { params })
    if (res.data.code === 200) {
      let rawData = res.data.data || []
      allOrders.value = rawData
      
      // 前端进行状态聚合筛选
      if (activeTab.value === 'todo') {
        rawData = rawData
          .filter(o => o.order_status === 1 && (
            o.payment_status === 1 ||
            needsDelivery(o) ||
            needsInstall(o) ||
            o.subsidy_status === 1
          ))
          .sort((a, b) => getOrderPriority(a) - getOrderPriority(b))
      } else if (activeTab.value === 'pending_delivery') {
        rawData = rawData.filter(o => o.order_status === 1 && needsDelivery(o))
      } else if (activeTab.value === 'pending_install') {
        rawData = rawData.filter(o => o.order_status === 1 && needsInstall(o))
      } else if (activeTab.value === 'pending_balance') {
        rawData = rawData.filter(o => o.order_status === 1 && o.payment_status === 1)
      } else if (activeTab.value === 'pending_subsidy') {
        rawData = rawData.filter(o => o.order_status === 1 && o.subsidy_status === 1)
      }
      
      orders.value = rawData
    }
  } catch (error) {
    ElMessage.error('获取订单列表失败')
  } finally {
    loading.value = false
  }
}

const formatDate = (date) => dayjs(date).format('YYYY-MM-DD HH:mm')

const needsDelivery = (row) => {
  if (!row || row.order_status !== 1 || row.delivery_method === 1) return false
  return (row.order_items || []).some(item => !item.is_delivered)
}

const needsInstall = (row) => {
  if (!row || row.order_status !== 1 || row.delivery_method === 1) return false
  return (row.order_items || []).some(item => item.is_delivered && !item.is_installed)
}

const getOrderPriority = (row) => {
  if (row.payment_status === 1) return 1
  if (needsDelivery(row)) return 2
  if (needsInstall(row)) return 3
  if (row.subsidy_status === 1) return 4
  return 9
}

const getOrderProductSummary = (row) => {
  const items = row.order_items || []
  if (items.length === 0) return '无商品明细'
  const names = items.slice(0, 2).map(item => item.product?.name || '未知商品').join('、')
  return `商品 ${items.length} 件：${names}${items.length > 2 ? '等' : ''}`
}

const getOrderStatusTags = (row) => {
  if (row.order_status === 2) return [{ label: '已取消', type: 'info' }]
  const tags = []
  tags.push({ label: row.payment_status === 2 ? '全款' : '定金', type: row.payment_status === 2 ? 'success' : 'warning' })
  if (needsDelivery(row)) tags.push({ label: '待送货', type: 'warning' })
  else if (needsInstall(row)) tags.push({ label: '待安装', type: 'primary' })
  else if (row.delivery_method !== 1) tags.push({ label: row.is_installed ? '已安装' : '履约中', type: row.is_installed ? 'success' : 'info' })
  if (row.subsidy_status !== 0) tags.push({ label: `国补${getSubsidyLabel(row.subsidy_status)}`, type: getSubsidyType(row.subsidy_status) })
  if (row.trade_in_amount > 0) tags.push({ label: '以旧换新', type: 'success' })
  return tags
}

const getFirstDeliveryItem = (row) => (row.order_items || []).find(item => !item.is_delivered)
const getFirstInstallItem = (row) => (row.order_items || []).find(item => item.is_delivered && !item.is_installed)

const getPrimaryAction = (row) => {
  if (row.payment_status === 1) return { label: '结尾款', type: 'success', action: 'balance' }
  if (needsDelivery(row)) return { label: '确认送货', type: 'warning', action: 'deliver' }
  if (needsInstall(row)) return { label: '确认安装', type: 'primary', action: 'install' }
  if (row.subsidy_status === 1) return { label: '提交国补', type: 'warning', action: 'subsidy' }
  return { label: '打印小票', type: 'default', action: 'receipt' }
}

const runPrimaryAction = (row) => {
  const action = getPrimaryAction(row).action
  if (action === 'balance') return handlePayBalance(row)
  if (action === 'deliver') return handleDeliver(getFirstDeliveryItem(row))
  if (action === 'install') return handleInstall(getFirstInstallItem(row))
  if (action === 'subsidy') return handleSubmitSubsidy(row)
  return openReceiptPreview(row)
}

const openOrderDetail = (row) => {
  detailOrder.value = row
  orderDetailVisible.value = true
}

const applyMobileFilters = () => {
  filterDrawerVisible.value = false
  fetchOrders()
}

const resetMobileFilters = () => {
  dateRange.value = []
  selectedCategoryId.value = ''
  selectedPaymentStatus.value = ''
  selectedFulfillmentStatus.value = ''
  selectedSubsidyStatus.value = ''
  selectedOrderStatus.value = ''
  filterDrawerVisible.value = false
  fetchOrders()
}

const getProductCategoryName = (categoryId) => {
  if (!categoryId) return '未分类'
  const cat = categories.value.find(c => c.id === categoryId)
  return cat ? cat.name : '未分类'
}

const getSubsidyLabel = (status) => {
  const map = { 0: '无', 1: '待提交', 2: '已提交', 3: '已回款' }
  return map[status] || '未知'
}

const getPaymentMethodLabel = (method) => {
  const map = { 1: '支付宝', 2: '微信', 3: '公户', 4: '混合支付', 5: '现金' }
  return map[method] || '其他'
}

const getPaymentBreakdownText = (breakdown) => {
  if (!breakdown) return ''
  try {
    const items = JSON.parse(breakdown)
    if (!Array.isArray(items)) return ''
    return items
      .map(item => `${getPaymentMethodLabel(item.payment_method)} ¥${Number(item.amount || 0).toFixed(2)}`)
      .join(' / ')
  } catch (error) {
    return ''
  }
}

const getItemUnitCost = (item) => Number(item?.unit_cost ?? item?.product?.avg_cost ?? item?.product?.latest_cost ?? 0) || 0
const getItemProductCost = (item) => getItemUnitCost(item) * Number(item?.quantity || 0)
const getItemRevenue = (item) => Number(item?.unit_price || 0) * Number(item?.quantity || 0)
const getItemGrossProfit = (item) => getItemRevenue(item) - getItemProductCost(item)
const getOrderProductCost = (row) => (row?.order_items || []).reduce((sum, item) => sum + getItemProductCost(item), 0)
const getOrderFulfillmentCost = (row) => (row?.order_items || []).reduce((sum, item) => {
  return sum + Number(item?.delivery_fee || 0) + Number(item?.installation_fee || 0)
}, 0)
const getOrderOtherCost = (row) => Number(row?.referral_fee || 0) + Number(row?.trade_in_amount || 0)
const getOrderTotalCost = (row) => getOrderProductCost(row) + getOrderFulfillmentCost(row) + getOrderOtherCost(row)
const getOrderGrossProfit = (row) => Number(row?.total_amount || 0) - getOrderTotalCost(row)
const getOrderGrossMargin = (row) => {
  const total = Number(row?.total_amount || 0)
  if (total <= 0) return 0
  return (getOrderGrossProfit(row) / total) * 100
}

const getDeliveryMethodLabel = (method) => {
  const map = { 1: '门店自提', 2: '主仓发货', 3: '云仓代发' }
  return map[method] || '其他'
}

const getSubsidyType = (status) => {
  const map = { 0: 'info', 1: 'danger', 2: 'warning', 3: 'success' }
  return map[status] || 'info'
}

const isBossCustomerOrder = (row) => {
  return row?.referrer_name === '老板客户' && Number(row?.commission_amount || 0) === 0
}

const isOrderFulfilled = (row) => {
  if (!row || row.order_status !== 1) return false
  if (row.is_delivered || row.is_installed) return true
  return (row.order_items || []).some(item => item.is_delivered || item.is_installed)
}

const openReceiptPreview = (row) => {
  currentReceipt.value = buildReceiptFromOrder(row)
  if (isMobileView.value) orderDetailVisible.value = false
  receiptVisible.value = true
}

const handleSubmitSubsidy = (row) => {
  ElMessageBox.confirm('确认已向政府系统提交资料？', '提交资料', { type: 'info' }).then(async () => {
    try {
      const res = await request.put(`/api/orders/${row.id}/subsidy`, { target_status: 2 })
      if (res.data.code === 200) { fetchOrders(); ElMessage.success('提交成功') }
    } catch (error) { ElMessage.error('操作失败') }
  })
}

const balanceDialogVisible = ref(false)
const balanceTargetOrder = ref(null)
const finalPaymentMethod = ref(1)
const paymentMethodOptions = [
  { value: 1, label: '支付宝' },
  { value: 2, label: '微信' },
  { value: 3, label: '公户' },
  { value: 5, label: '现金' },
]

const handlePayBalance = (row) => {
  balanceTargetOrder.value = row
  finalPaymentMethod.value = row.payment_method === 4 ? 1 : row.payment_method
  balanceDialogVisible.value = true
}

const confirmPayBalance = async () => {
  if (!balanceTargetOrder.value) return
  try {
    const res = await request.put(`/api/orders/${balanceTargetOrder.value.id}/balance`, {
      final_payment_method: finalPaymentMethod.value
    })
    if (res.data.code === 200) {
      ElMessage.success('结算完成')
      balanceDialogVisible.value = false
      fetchOrders()
    }
  } catch (error) { ElMessage.error(error.response?.data?.msg || '操作失败') }
}

const handleCancel = (row) => {
  ElMessageBox.confirm(`确认取消订单 ${row.order_no}？此操作将退还已收金额并回滚库存！`, '高危操作', { type: 'error' }).then(async () => {
    try {
      const res = await request.put(`/api/orders/${row.id}/cancel`)
      if (res.data.code === 200) { fetchOrders(); ElMessage.success('订单已取消') }
    } catch (error) { ElMessage.error(error.response?.data?.msg || '取消失败') }
  })
}

const handleReversalCancel = (row) => {
  ElMessageBox.confirm(
    `确认对已履约订单 ${row.order_no} 执行红冲重开？系统会反向冲回库存和已收款项，并将原订单标记为已取消。完成后必须重新录入包含全部商品和赠品的正确新单。`,
    '红冲重开确认',
    {
      type: 'error',
      confirmButtonText: '确认红冲',
      cancelButtonText: '再检查一下'
    }
  ).then(async () => {
    try {
      const res = await request.put(`/api/orders/${row.id}/reversal-cancel`)
      if (res.data.code === 200) {
        fetchOrders()
        ElMessage.success(res.data.msg || '订单已红冲')
      }
    } catch (error) {
      ElMessage.error(error.response?.data?.msg || '红冲失败')
    }
  })
}

const deliveryDialogVisible = ref(false)
const deliveryTargetItem = ref(null)
const selectedDeliveryPersonId = ref(null)
const selectedDeliveryTime = ref(null)
const deliveryFee = ref(0)

const installDialogVisible = ref(false)
const installTargetItem = ref(null)
const selectedInstallerId = ref(null)
const selectedInstallTime = ref(null)
const installationFee = ref(0)
const employees = ref([])

const deliveryPersonOptions = computed(() => {
  const outsourced = employees.value.filter(emp => emp.is_outsourced)
  const staff = employees.value.filter(emp => !emp.is_outsourced)
  return [
    { label: '外包合作人员', options: outsourced },
    { label: '店内正式员工', options: staff }
  ]
})

const installerOptions = computed(() => {
  const outsourced = employees.value.filter(emp => emp.is_outsourced)
  const staff = employees.value.filter(emp => !emp.is_outsourced)
  return [
    { label: '外包合作人员', options: outsourced },
    { label: '店内正式员工', options: staff }
  ]
})

const fetchEmployees = async () => {
  try {
    const res = await request.get('/api/employees')
    if (res.data.code === 200) employees.value = res.data.data
  } catch (error) {}
}

const handleDeliver = (item) => {
  deliveryTargetItem.value = item
  selectedDeliveryPersonId.value = null
  selectedDeliveryTime.value = new Date()
  deliveryFee.value = 0
  if (employees.value.length === 0) fetchEmployees()
  deliveryDialogVisible.value = true
}

const confirmDeliver = async () => {
  if (!selectedDeliveryPersonId.value) return
  try {
    const res = await request.put(`/api/order-items/${deliveryTargetItem.value.id}/deliver`, {
      delivery_person_id: selectedDeliveryPersonId.value,
      delivery_fee: deliveryFee.value,
      delivery_time: selectedDeliveryTime.value
    })
    if (res.data.code === 200) {
      ElMessage.success('送货确认成功')
      deliveryDialogVisible.value = false
      fetchOrders()
    }
  } catch (error) { ElMessage.error('操作失败') }
}

const handleInstall = (item) => {
  installTargetItem.value = item
  selectedInstallerId.value = null
  selectedInstallTime.value = new Date()
  installationFee.value = 0
  if (employees.value.length === 0) fetchEmployees()
  installDialogVisible.value = true
}

const confirmInstall = async () => {
  if (!selectedInstallerId.value) return
  try {
    const res = await request.put(`/api/order-items/${installTargetItem.value.id}/install`, {
      installer_id: selectedInstallerId.value,
      install_time: selectedInstallTime.value,
      installation_fee: installationFee.value
    })
    if (res.data.code === 200) {
      ElMessage.success('安装确认成功')
      installDialogVisible.value = false
      fetchOrders()
    }
  } catch (error) { ElMessage.error(error.response?.data?.msg || '操作失败') }
}


const commissionDialogVisible = ref(false)
const commissionTargetOrder = ref(null)
const newCommissionAmount = ref(0)

const openCommissionDialog = (row) => {
  commissionTargetOrder.value = row
  newCommissionAmount.value = row.commission_amount || 0
  if (isMobileView.value) orderDetailVisible.value = false
  commissionDialogVisible.value = true
}

const confirmUpdateCommission = async () => {
  try {
    const res = await request.put(`/api/orders/${commissionTargetOrder.value.id}/commission`, {
      commission_amount: newCommissionAmount.value
    })
    if (res.data.code === 200) {
      ElMessage.success('提成调整成功')
      commissionDialogVisible.value = false
      fetchOrders()
    }
  } catch (error) {
    ElMessage.error('调整失败')
  }
}

const referralFeeDialogVisible = ref(false)
const referralFeeTargetOrder = ref(null)
const newReferralFee = ref(0)

const openReferralFeeDialog = (row) => {
  referralFeeTargetOrder.value = row
  newReferralFee.value = Number(row.referral_fee || 0)
  if (isMobileView.value) orderDetailVisible.value = false
  referralFeeDialogVisible.value = true
}

const confirmUpdateReferralFee = async () => {
  if (referralFeeTargetOrder.value?.is_referral_fee_paid) return
  try {
    const res = await request.put(`/api/orders/${referralFeeTargetOrder.value.id}/referral-fee`, {
      referral_fee: newReferralFee.value
    })
    if (res.data.code === 200) {
      ElMessage.success('推荐人提成调整成功')
      referralFeeDialogVisible.value = false
      fetchOrders()
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '调整失败')
  }
}

onMounted(() => {
  window.addEventListener('resize', handleViewportResize)
  if (isMobileView.value) activeTab.value = 'todo'
  fetchCategories()
  fetchOrders()
})

onUnmounted(() => {
  window.removeEventListener('resize', handleViewportResize)
})
</script>

<style scoped>
:deep(.el-card__body) { padding: 0; }
.mobile-native-date-input {
  width: 100%;
  min-height: 40px;
  padding: 0 12px;
  border: 1px solid #cbd5e1;
  border-radius: 10px;
  background: #fff;
  color: #334155;
  font-size: 16px;
}
</style>
