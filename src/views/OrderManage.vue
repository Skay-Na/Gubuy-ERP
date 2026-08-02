<template>
  <div class="p-6 max-w-7xl mx-auto">
    <!-- Header Section -->
    <div class="mb-8">
      <div class="flex flex-col md:flex-row justify-between items-start md:items-end gap-4 mb-6">
        <div>
          <h1 class="text-2xl md:text-3xl font-black text-slate-900 tracking-tight mb-1 whitespace-nowrap">订单管理与历史</h1>
          <p class="text-sm text-slate-400">查看和管理所有的销售订单及款项流转</p>
        </div>
        <div class="flex flex-wrap items-center gap-3 w-full md:w-auto">
          <el-date-picker
            v-model="dateRange"
            type="daterange"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            class="!w-full md:!w-72 shadow-sm"
            @change="fetchOrders"
          />
          <div class="flex w-full md:w-auto gap-2">
            <el-input 
              v-model="searchKeyword" 
              placeholder="搜索订单/客户/电话" 
              clearable 
              class="flex-1 md:!w-64"
              @input="handleSearchInput"
            >
              <template #prefix><el-icon><Search /></el-icon></template>
            </el-input>
          </div>
        </div>
      </div>

      <!-- 快捷状态分类 Tab -->
      <div class="flex items-center gap-2 overflow-x-auto no-scrollbar pb-1">
        <div 
          v-for="tab in filterTabs" 
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
                  <el-descriptions-item label="结算金额">
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
                  </el-descriptions-item>
                  <el-descriptions-item label="开单人员">
                    {{ row.employee?.name || '未知' }} (工号: {{ row.employee?.emp_no || '-' }})
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
                  <el-descriptions-item label="折抵金额">
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
                  <el-descriptions-item label="渠道奖励">
                    ¥{{ (row.referral_fee || 0).toFixed(2) }} 
                    <el-tag v-if="row.referral_fee > 0" size="small" :type="row.is_referral_fee_paid ? 'success' : 'info'" class="ml-2">
                      {{ row.is_referral_fee_paid ? '已发放' : '待结算' }}
                    </el-tag>
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
              </div>
              <span class="text-[10px] text-slate-400 font-mono">{{ row.customer_phone || '-' }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="订单金额" width="120">
          <template #default="{ row }">
            <span class="font-bold text-slate-900">¥{{ row.actual_pay_amount.toFixed(2) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="业务提成" width="120">
          <template #default="{ row }">
            <div class="flex items-center gap-1 cursor-pointer group" @click="openCommissionDialog(row)">
              <span class="font-bold text-orange-600">¥{{ (row.commission_amount || 0).toFixed(2) }}</span>
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
                    <el-dropdown-item v-if="row.subsidy_status === 1" @click="handleSubmitSubsidy(row)">提交国补</el-dropdown-item>
                    <el-dropdown-item v-if="row.subsidy_status === 2" @click="handleVerify(row)">国补核销</el-dropdown-item>
                    <el-dropdown-item @click="openCommissionDialog(row)">调整提成</el-dropdown-item>
                    <el-dropdown-item divided type="danger" @click="handleCancel(row)" class="!text-red-500">取消订单</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
            <span v-else class="text-xs text-slate-400">-</span>
          </template>
        </el-table-column>
      </el-table>

      <!-- Mobile View -->
      <div class="mobile-only md:hidden p-4 space-y-4" v-loading="loading">
        <div v-for="row in orders" :key="row.id" class="bg-white border border-slate-100 rounded-2xl p-4 shadow-sm" :class="row.order_status === 2 ? 'opacity-60' : ''">
          <div class="flex justify-between items-start mb-3">
            <div class="flex-1 min-w-0 pr-2">
              <div class="text-[10px] text-slate-400 font-mono mb-1">{{ row.order_no }}</div>
              <div class="font-bold text-slate-800 text-base leading-tight">{{ row.customer_name || '散客' }}</div>
            </div>
            <div class="text-right flex-shrink-0">
              <div class="text-slate-900 font-black text-lg">¥{{ row.actual_pay_amount.toFixed(2) }}</div>
            </div>
          </div>

          <div class="flex flex-wrap gap-2 mb-4">
             <el-tag size="small" :type="row.order_status === 2 ? 'info' : (row.payment_status === 2 ? 'success' : 'warning')">
               {{ row.order_status === 2 ? '已取消' : (row.payment_status === 1 ? '定金' : '全款') }}
             </el-tag>
             <el-tag size="small" v-if="row.order_status !== 2" :type="getSubsidyType(row.subsidy_status)">国补:{{ getSubsidyLabel(row.subsidy_status) }}</el-tag>
             <el-tag size="small" v-if="row.order_status !== 2" :type="row.is_installed ? 'success' : (row.is_delivered ? 'primary' : 'warning')">{{ row.is_installed ? '已安装' : (row.is_delivered ? '已送货，待安装' : '待送货') }}</el-tag>
          </div>

          <!-- 商品及履约进度清单 (移动端专属) -->
          <div class="space-y-3 border-t border-b border-slate-100 py-3 my-3">
             <div v-for="item in row.order_items" :key="item.id" class="flex justify-between items-center text-xs">
                <div class="flex-1 pr-2">
                   <div class="font-bold text-slate-700">{{ item.product?.name }}</div>
                   <div class="text-[10px] text-slate-400">数量: {{ item.quantity }} | 单价: ¥{{ item.unit_price.toFixed(2) }}</div>
                </div>
                <div class="flex gap-2 items-center">
                   <!-- 送货 -->
                   <el-button v-if="!item.is_delivered && row.delivery_method !== 1" size="small" type="warning" class="!px-2 !py-0.5 !h-6 !text-[10px] !rounded-md" @click="handleDeliver(item)">送货</el-button>
                   <span v-else-if="item.is_delivered && !item.is_installed" class="text-blue-600 text-[10px] font-bold">🚚已送</span>
                   
                   <!-- 安装 -->
                   <el-button v-if="item.is_delivered && !item.is_installed && row.delivery_method !== 1" size="small" type="primary" class="!px-2 !py-0.5 !h-6 !text-[10px] !rounded-md" @click="handleInstall(item)">安装</el-button>
                   <span v-else-if="item.is_installed" class="text-green-600 text-[10px] font-bold">✅已装</span>
                </div>
             </div>
          </div>

          <div v-if="row.order_status === 1" class="flex justify-end gap-2 pt-1">
            <el-button v-if="row.payment_status === 1" size="small" type="success" @click="handlePayBalance(row)" class="!rounded-lg">结尾款</el-button>
            <el-dropdown trigger="click">
              <el-button size="small" class="!rounded-lg px-2"><el-icon><MoreFilled /></el-icon></el-button>
              <template #dropdown>
                <el-dropdown-menu>
                   <el-dropdown-item v-if="row.subsidy_status === 1" @click="handleSubmitSubsidy(row)">提交国补</el-dropdown-item>
                   <el-dropdown-item v-if="row.subsidy_status === 2" @click="handleVerify(row)">国补核销</el-dropdown-item>
                   <el-dropdown-item divided type="danger" @click="handleCancel(row)" class="!text-red-500">取消订单</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
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
          <el-date-picker
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
          <el-date-picker
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
    <el-dialog v-model="commissionDialogVisible" title="调整业务提成" width="400px" destroy-on-close class="!rounded-2xl">
      <div v-if="commissionTargetOrder" class="space-y-4">
        <el-alert title="提示" type="warning" :closable="false" show-icon class="!rounded-xl">请根据与店长/员工商定的最终结果进行手动调整。</el-alert>
        <div class="bg-slate-50 p-4 rounded-xl border border-slate-100">
          <div class="text-xs text-slate-400 mb-2 uppercase font-bold tracking-wider">订单号: {{ commissionTargetOrder.order_no }}</div>
          <div class="text-sm font-bold text-slate-700 mb-4">当前订单实收: ¥{{ commissionTargetOrder.actual_pay_amount.toFixed(2) }}</div>
          
          <label class="block text-sm font-bold text-slate-700 mb-2">最终提成金额 (元)</label>
          <el-input-number v-model="newCommissionAmount" :min="0" :precision="2" class="!w-full" controls-position="right" />
        </div>
      </div>
      <template #footer>
        <el-button @click="commissionDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="confirmUpdateCommission">确认修改</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search, InfoFilled, MoreFilled, Edit } from '@element-plus/icons-vue'
import dayjs from 'dayjs'

const orders = ref([])
const loading = ref(false)
const searchKeyword = ref('')
const dateRange = ref([])
const activeTab = ref('all')

const filterTabs = [
  { label: '全部订单', value: 'all' },
  { label: '待确认安装', value: 'pending_install' },
  { label: '待催收尾款', value: 'pending_balance' },
  { label: '待提交国补', value: 'pending_subsidy' },
  { label: '已结清全款', value: 'settled' },
  { label: '已取消', value: 'cancelled' },
]

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

const fetchOrders = async () => {
  loading.value = true
  try {
    const params = { 
      keyword: searchKeyword.value,
    }
    
    if (dateRange.value && dateRange.value.length === 2) {
      params.start_date = dayjs(dateRange.value[0]).startOf('day').toISOString()
      params.end_date = dayjs(dateRange.value[1]).endOf('day').toISOString()
    }

    const res = await axios.get('/api/orders', { params })
    if (res.data.code === 200) {
      let rawData = res.data.data
      
      // 前端进行状态聚合筛选
      if (activeTab.value === 'pending_install') {
        rawData = rawData.filter(o => o.order_status === 1 && !o.is_installed)
      } else if (activeTab.value === 'pending_balance') {
        rawData = rawData.filter(o => o.order_status === 1 && o.payment_status === 1 && o.is_installed)
      } else if (activeTab.value === 'pending_subsidy') {
        rawData = rawData.filter(o => o.order_status === 1 && o.subsidy_status === 1)
      } else if (activeTab.value === 'settled') {
        rawData = rawData.filter(o => o.order_status === 1 && o.payment_status === 2)
      } else if (activeTab.value === 'cancelled') {
        rawData = rawData.filter(o => o.order_status === 2)
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

const getSubsidyLabel = (status) => {
  const map = { 0: '无', 1: '待提交', 2: '已提交', 3: '已回款' }
  return map[status] || '未知'
}

const getPaymentMethodLabel = (method) => {
  const map = { 1: '支付宝', 2: '微信', 3: '公户' }
  return map[method] || '其他'
}

const getDeliveryMethodLabel = (method) => {
  const map = { 1: '门店自提', 2: '主仓发货', 3: '云仓代发' }
  return map[method] || '其他'
}

const getSubsidyType = (status) => {
  const map = { 0: 'info', 1: 'danger', 2: 'warning', 3: 'success' }
  return map[status] || 'info'
}

const handleSubmitSubsidy = (row) => {
  ElMessageBox.confirm('确认已向政府系统提交资料？', '提交资料', { type: 'info' }).then(async () => {
    try {
      const res = await axios.put(`/api/orders/${row.id}/subsidy`, { target_status: 2 })
      if (res.data.code === 200) { fetchOrders(); ElMessage.success('提交成功') }
    } catch (error) { ElMessage.error('操作失败') }
  })
}

const handleVerify = (row) => {
  ElMessageBox.confirm('确认国补资金已打入对公账户？', '确认核销', { type: 'warning' }).then(async () => {
    try {
      const res = await axios.put(`/api/orders/${row.id}/subsidy`, { target_status: 3 })
      if (res.data.code === 200) { fetchOrders(); ElMessage.success('核销成功') }
    } catch (error) { ElMessage.error('核销失败') }
  })
}

const balanceDialogVisible = ref(false)
const balanceTargetOrder = ref(null)
const finalPaymentMethod = ref(1)
const paymentMethodOptions = [
  { value: 1, label: '支付宝' },
  { value: 2, label: '微信' },
  { value: 3, label: '公户' },
]

const handlePayBalance = (row) => {
  balanceTargetOrder.value = row
  finalPaymentMethod.value = row.payment_method
  balanceDialogVisible.value = true
}

const confirmPayBalance = async () => {
  if (!balanceTargetOrder.value) return
  try {
    const res = await axios.put(`/api/orders/${balanceTargetOrder.value.id}/balance`, {
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
      const res = await axios.put(`/api/orders/${row.id}/cancel`)
      if (res.data.code === 200) { fetchOrders(); ElMessage.success('订单已取消') }
    } catch (error) { ElMessage.error('取消失败') }
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
    const res = await axios.get('/api/employees')
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
    const res = await axios.put(`/api/order-items/${deliveryTargetItem.value.id}/deliver`, {
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
    const res = await axios.put(`/api/order-items/${installTargetItem.value.id}/install`, {
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
  commissionDialogVisible.value = true
}

const confirmUpdateCommission = async () => {
  try {
    const res = await axios.put(`/api/orders/${commissionTargetOrder.value.id}/commission`, {
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

onMounted(() => fetchOrders())
</script>

<style scoped>
:deep(.el-card__body) { padding: 0; }
</style>
