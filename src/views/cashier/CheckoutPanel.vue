<template>
  <div class="h-full bg-slate-50 flex flex-col overflow-hidden">
    <!-- Desktop Layout -->
    <div v-if="!isMobile" class="flex flex-1 overflow-hidden flex-row">
      <!-- 购物车列表区域 -->
      <div class="flex-1 overflow-y-auto p-4 space-y-3" :class="isMobile ? 'max-h-[40vh] border-b border-slate-200' : ''">
        <div v-if="cartStore.cart.length === 0" class="h-full flex flex-col items-center justify-center text-slate-400 py-10">
          <el-icon size="48" class="mb-2 opacity-20"><Files /></el-icon>
          <p class="text-sm">购物车是空的</p>
        </div>
        
        <div v-for="(item, index) in cartStore.cart" :key="item.id" class="p-3 rounded-xl border border-slate-100 bg-white shadow-sm relative">
          <div class="flex justify-between items-start mb-2">
            <div class="font-bold text-slate-800 text-sm truncate pr-8">{{ item.name }}</div>
            <el-button type="danger" link :icon="Delete" class="!p-0 absolute right-3 top-3" @click="cartStore.removeFromCart(index)" />
          </div>
          
          <div class="flex justify-between items-center mt-2">
            <div class="text-xs text-slate-600 font-medium">作为赠品 (不再计入总价)</div>
            <el-switch v-model="item.isGift" size="small" />
          </div>
          
          <div class="flex justify-between items-center mt-2" :class="{'opacity-40 pointer-events-none': item.isGift}">
            <div class="text-xs text-slate-600">单价 (底价: ¥{{ item.minPrice.toFixed(2) }})</div>
            <div class="flex items-center">
              <el-input-number v-if="!item.isGift" v-model="item.price" :min="0.01" :precision="2" :controls="false" size="small" class="!w-32 pos-input-number" :class="{ 'price-warning': item.price < item.minPrice }" />
              <span v-else class="text-red-500 font-bold px-4 py-1 bg-red-50 rounded">¥0.00 (赠品)</span>
            </div>
          </div>
          
          <div class="flex justify-between items-center mt-2">
            <div class="text-xs text-slate-600">实物库存 (门店:{{ item.store_stock == null ? '盘点中' : item.store_stock }} | 云仓:{{ item.cloud_stock == null ? '盘点中' : item.cloud_stock || 0 }})</div>
            <el-input-number v-model="item.quantity" :min="1" size="small" class="!w-32 pos-input-number" />
          </div>
          
          <!-- 单品库存警告 -->
          <div v-if="getStockWarning(item)" class="text-red-500 text-[10px] mt-1.5 flex items-center gap-1 font-bold">
            <el-icon><Warning /></el-icon>{{ getStockWarning(item) }}
          </div>
        </div>
      </div>

      
      <!-- 右侧/下部 结算配置区域 -->
      <div class="overflow-y-auto p-4 md:p-6 bg-white border-l border-slate-200" :class="isMobile ? 'flex-1' : 'w-[400px] shrink-0 h-full'">
        <div class="space-y-4">
          <!-- 客户信息录入区 -->
          <div class="bg-blue-50/50 p-4 rounded-xl border border-blue-100/50 space-y-3">
            <div class="flex items-center gap-2 mb-1">
              <el-icon class="text-blue-600"><User /></el-icon>
              <span class="text-sm font-bold text-slate-700">客户信息录入</span>
            </div>
            <div class="grid grid-cols-2 gap-3 mb-2">
              <div class="space-y-1">
                <span class="text-[10px] text-slate-400 font-medium pl-1">客户姓名</span>
                <el-input v-model="customerName" placeholder="请输入姓名" class="pos-customer-input" />
              </div>
              <div class="space-y-1">
                <span class="text-[10px] text-slate-400 font-medium pl-1">手机号码</span>
                <el-input v-model="customerPhone" type="tel" placeholder="请输入手机号" class="pos-customer-input" />
              </div>
            </div>
            <div class="space-y-1 mb-2">
              <span class="text-[10px] text-slate-400 font-medium pl-1">收货地址</span>
              <el-input v-model="customerAddress" placeholder="请输入客户详细地址" class="pos-customer-input" />
            </div>

            <div class="space-y-3 pt-2 border-t border-blue-100/50">
              <div class="space-y-1">
                <span class="text-[10px] text-slate-400 font-medium pl-1">开单员工</span>
                <el-select v-model="employeeId" placeholder="选择销售员工 (可选)" class="w-full" clearable>
                  <el-option v-for="emp in employees" :key="emp.id" :label="emp.name" :value="emp.id" />
                </el-select>
              </div>
              <div class="space-y-1">
                <span class="text-[10px] text-slate-400 font-medium pl-1">推荐人/渠道</span>
                <el-input v-model="referrerName" placeholder="选填" class="pos-customer-input" />
                <span class="text-[10px] text-slate-400">渠道奖励实际支出请在专业记账中登记。</span>
              </div>
              <div class="mt-3 space-y-2">
                <div class="text-[10px] text-slate-400 font-medium pl-1 uppercase tracking-wider">订单类型</div>
                <div class="grid grid-cols-2 gap-2">
                  <div 
                    class="flex items-center justify-center gap-2 p-2.5 rounded-xl border-2 transition-all cursor-pointer bg-white"
                    :class="orderType === 1 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200'"
                    @click="orderType = 1"
                  >
                    <el-icon size="16"><Document /></el-icon>
                    <span class="text-sm font-bold">普通订单</span>
                  </div>
                  <div 
                    class="flex items-center justify-center gap-2 p-2.5 rounded-xl border-2 transition-all cursor-pointer bg-white"
                    :class="orderType === 2 ? 'border-amber-500 bg-amber-50/30 text-amber-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200'"
                    @click="orderType = 2"
                  >
                    <el-icon size="16"><Star /></el-icon>
                    <span class="text-sm font-bold">商会订单</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- 选项开关 -->
          <div class="flex flex-col gap-2">
            <div 
              class="flex justify-between items-center p-3 rounded-xl bg-slate-50 border border-slate-100 cursor-pointer hover:bg-slate-100 transition-all"
              @click="isSubsidized = !isSubsidized"
            >
              <span class="text-sm font-bold text-slate-700 flex items-center gap-2">
                <el-icon :size="18" color="#3b82f6"><Promotion /></el-icon>参与国补政策
              </span>
              <el-switch v-model="isSubsidized" size="small" @click.stop />
            </div>
            <div 
              class="flex justify-between items-center p-3 rounded-xl bg-slate-50 border border-slate-100 cursor-pointer hover:bg-slate-100 transition-all"
              @click="isTradeIn = !isTradeIn"
            >
              <span class="text-sm font-bold text-slate-700 flex items-center gap-2">
                <el-icon :size="18" color="#10b981"><Refresh /></el-icon>以旧换新抵扣
              </span>
              <el-switch v-model="isTradeIn" size="small" @click.stop />
            </div>
            <div 
              class="flex justify-between items-center p-3 rounded-xl bg-slate-50 border border-slate-100 cursor-pointer hover:bg-slate-100 transition-all"
              @click="isDepositMode = !isDepositMode"
            >
              <span class="text-sm font-bold text-slate-700 flex items-center gap-2">
                <el-icon :size="18" color="#8b5cf6"><Wallet /></el-icon>支付定金模式
              </span>
              <el-switch v-model="isDepositMode" size="small" @click.stop />
            </div>
          </div>

          <!-- 以旧换新子面板 -->
          <el-collapse-transition>
            <div v-if="isTradeIn" class="bg-emerald-50 p-3 rounded-xl border border-emerald-100 space-y-3">
              <div class="text-xs font-bold text-emerald-700 mb-1 flex items-center gap-1">
                <el-icon><Refresh /></el-icon>以旧换新折抵设置
              </div>
              <div class="grid grid-cols-2 gap-2">
                <div>
                  <span class="text-[10px] text-slate-400 font-medium pl-1">折抵金额(元)</span>
                  <el-input-number 
                    v-model="tradeInAmount" 
                    :min="0" 
                    :precision="2" 
                    class="w-full pos-customer-input" 
                    :controls="false" 
                    placeholder="抵扣金额" 
                  />
                </div>
                <div>
                  <span class="text-[10px] text-slate-400 font-medium pl-1">旧件品类</span>
                  <el-select 
                    v-model="tradeInType" 
                    placeholder="选择/输入品类" 
                    class="w-full"
                    filterable 
                    allow-create 
                    default-first-option
                  >
                    <el-option label="旧电视" value="旧电视" />
                    <el-option label="旧冰箱" value="旧冰箱" />
                    <el-option label="旧洗衣机" value="旧洗衣机" />
                    <el-option label="旧空调" value="旧空调" />
                    <el-option label="旧热水器" value="旧热水器" />
                  </el-select>
                </div>
              </div>
              <div>
                <span class="text-[10px] text-slate-400 font-medium pl-1">实际付款账户</span>
                <el-select v-model="tradeInAccountId" placeholder="请选择付款账户" class="w-full" clearable>
                  <el-option v-for="account in accounts" :key="account.id" :label="`${account.name}（账面 ¥${Number(account.balance || 0).toFixed(2)}）`" :value="account.id" />
                </el-select>
              </div>
              <div>
                <span class="text-[10px] text-slate-400 font-medium pl-1">旧件备注</span>
                <el-input 
                  v-model="tradeInRemark" 
                  placeholder="品牌/成色/是否通电等" 
                  class="pos-customer-input" 
                />
              </div>
            </div>
          </el-collapse-transition>

          <!-- 定金输入面板 -->
          <el-collapse-transition>
            <div v-if="isDepositMode" class="bg-purple-50 p-3 rounded-xl border border-purple-100">
              <div class="text-xs font-bold text-purple-700 mb-1.5">输入预付定金金额 (元)</div>
              <el-input-number v-model="depositAmount" :min="0.01" :max="totalPayable" :precision="2" class="w-full" :controls="false" placeholder="输入定金金额" />
            </div>
          </el-collapse-transition>

          <!-- 发货与支付设置 -->
          <div class="bg-white p-3 rounded-xl border border-slate-100 shadow-sm space-y-4">
            <div>
              <div class="text-[10px] text-slate-400 mb-2 uppercase font-bold tracking-wider">发货/提货方式</div>
              <div class="grid grid-cols-2 gap-2">
                <div 
                  class="flex flex-col items-center justify-center p-3 rounded-xl border-2 transition-all cursor-pointer bg-white"
                  :class="deliveryMethod === 1 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200'"
                  @click="deliveryMethod = 1"
                >
                  <el-icon size="20" class="mb-1"><Shop /></el-icon>
                  <span class="text-xs font-bold">门店自提</span>
                </div>
                <div 
                  class="flex flex-col items-center justify-center p-3 rounded-xl border-2 transition-all cursor-pointer bg-white"
                  :class="deliveryMethod === 4 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200'"
                  @click="deliveryMethod = 4"
                >
                  <el-icon size="20" class="mb-1"><Cloudy /></el-icon>
                  <span class="text-xs font-bold">云仓发货</span>
                  <span class="mt-0.5 text-[9px] opacity-70">实物优先，不限量代发</span>
                </div>
              </div>
            </div>
            
            <div>
              <div class="text-[10px] text-slate-400 mb-2 uppercase font-bold tracking-wider">收款账户</div>
              <div class="grid grid-cols-5 gap-2">
                <div
                  class="flex flex-col items-center justify-center p-3 rounded-xl border-2 transition-all cursor-pointer bg-white"
                  :class="[
                    paymentMethod === 1 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200',
                    isSubsidized ? 'opacity-40 cursor-not-allowed grayscale' : ''
                  ]"
                  @click="!isSubsidized && (paymentMethod = 1)"
                >
                  <el-icon size="20" class="mb-1"><Wallet /></el-icon>
                  <span class="text-xs font-bold">支付宝</span>
                </div>
                <div
                  class="flex flex-col items-center justify-center p-3 rounded-xl border-2 transition-all cursor-pointer bg-white"
                  :class="[
                    paymentMethod === 2 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200',
                    isSubsidized ? 'opacity-40 cursor-not-allowed grayscale' : ''
                  ]"
                  @click="!isSubsidized && (paymentMethod = 2)"
                >
                  <el-icon size="20" class="mb-1"><CreditCard /></el-icon>
                  <span class="text-xs font-bold">微信支付</span>
                </div>
                <div
                  class="flex flex-col items-center justify-center p-3 rounded-xl border-2 transition-all cursor-pointer bg-white"
                  :class="paymentMethod === 3 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200'"
                  @click="paymentMethod = 3"
                >
                  <el-icon size="20" class="mb-1"><OfficeBuilding /></el-icon>
                  <span class="text-xs font-bold">对公转账</span>
                </div>
                <div
                  class="flex flex-col items-center justify-center p-3 rounded-xl border-2 transition-all cursor-pointer bg-white"
                  :class="paymentMethod === 4 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200'"
                  @click="paymentMethod = 4"
                >
                  <el-icon size="20" class="mb-1"><CreditCard /></el-icon>
                  <span class="text-xs font-bold">混合支付</span>
                </div>
                <div
                  class="flex flex-col items-center justify-center p-3 rounded-xl border-2 transition-all cursor-pointer bg-white"
                  :class="paymentMethod === 5 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200'"
                  @click="paymentMethod = 5"
                >
                  <el-icon size="20" class="mb-1"><Money /></el-icon>
                  <span class="text-xs font-bold">现金</span>
                </div>
              </div>
              <el-collapse-transition>
                <div v-if="paymentMethod === 4" class="mt-3 p-3 rounded-xl border border-blue-100 bg-blue-50/50 space-y-2">
                  <div class="flex justify-between items-center text-xs font-bold">
                    <span class="text-blue-700">拆分收款金额</span>
                    <span :class="mixedPaymentRemaining === 0 ? 'text-emerald-600' : 'text-red-500'">
                      差额 ¥{{ mixedPaymentRemaining.toFixed(2) }}
                      <el-button v-if="mixedPaymentRemaining > 0 && mixedPaymentRemaining <= 10" size="small" type="primary" link @click="handleWipeZero">抹零</el-button>
                    </span>
                  </div>
                  <div class="grid grid-cols-2 gap-2">
                    <div>
                      <span class="text-[10px] text-slate-400 font-medium pl-1">支付宝</span>
                      <el-input-number v-model="mixedPaymentAmounts[1]" :min="0" :precision="2" :controls="false" class="w-full pos-customer-input" />
                    </div>
                    <div>
                      <span class="text-[10px] text-slate-400 font-medium pl-1">微信</span>
                      <el-input-number v-model="mixedPaymentAmounts[2]" :min="0" :precision="2" :controls="false" class="w-full pos-customer-input" />
                    </div>
                    <div>
                      <span class="text-[10px] text-slate-400 font-medium pl-1">公户</span>
                      <el-input-number v-model="mixedPaymentAmounts[3]" :min="0" :precision="2" :controls="false" class="w-full pos-customer-input" />
                    </div>
                    <div>
                      <span class="text-[10px] text-slate-400 font-medium pl-1">现金</span>
                      <el-input-number v-model="mixedPaymentAmounts[5]" :min="0" :precision="2" :controls="false" class="w-full pos-customer-input" />
                    </div>
                  </div>
                  <div v-if="isSubsidized" class="mt-1 text-[10px] text-orange-500 bg-orange-50 p-1.5 rounded-md flex items-start gap-1">
                    <el-icon class="mt-0.5"><WarningFilled /></el-icon>
                    <span>国补混付提示：请将客户在POS机上刷的<b>原始金额(含国补)</b>直接填入公户，系统将据此精准计算待回款。</span>
                  </div>
                </div>
              </el-collapse-transition>
            </div>
          </div>

          <!-- 收金汇总结算 -->
          <div class="bg-slate-900 p-4 rounded-2xl text-white shadow-xl space-y-1">
            <div class="flex justify-between text-xs opacity-60"><span>订单金额</span><span>¥{{ originalTotal.toFixed(2) }}</span></div>
            <div v-if="isSubsidized" class="flex justify-between text-xs text-orange-300"><span>国补优惠 (15%)</span><span>-¥{{ subsidyAmount.toFixed(2) }}</span></div>
            <div v-if="isTradeIn" class="flex justify-between text-xs text-emerald-300"><span>以旧换新折抵</span><span>-¥{{ (tradeInAmount || 0).toFixed(2) }}</span></div>
            <div class="flex justify-between items-end mt-2 pt-1 border-t border-slate-800">
              <span class="text-sm font-bold opacity-80">{{ isDepositMode ? '本次预收定金' : '本次应付全款' }}</span>
              <div class="flex flex-col items-end">
                <span v-if="wipeZeroAmount > 0" class="text-xs text-emerald-400">抹零优惠 -¥{{ wipeZeroAmount.toFixed(2) }}</span>
                <span class="text-2xl font-black text-blue-400 font-mono">¥{{ actualReceipt.toFixed(2) }}</span>
              </div>
            </div>
          </div>

          <div class="flex flex-col gap-2 pt-2">
            <el-button type="primary" size="large" class="w-full !h-12 !text-base !rounded-xl !border-none shadow-md shadow-blue-200 pos-submit-btn" :disabled="cartStore.cart.length === 0 || hasStockViolation" :loading="submitLoading" @click="handleCheckout">
              <span class="font-bold tracking-widest text-[16px]">确认开单</span>
            </el-button>
          </div>
        </div>
      </div>

    </div>
    
    <!-- Mobile Layout -->
    <div v-else class="flex flex-col flex-1 overflow-hidden relative bg-slate-100">
      
      <!-- 主滚动区域 -->
      <div class="flex-1 overflow-y-auto p-3 space-y-4 pb-[180px]">
        
        <!-- 购物车区域 -->
        <div class="space-y-3">
          <div v-if="cartStore.cart.length === 0" class="h-32 flex flex-col items-center justify-center text-slate-400 bg-white rounded-xl border border-slate-100 shadow-sm">
            <el-icon size="32" class="mb-2 opacity-20"><Files /></el-icon>
            <p class="text-xs">购物车是空的</p>
          </div>
          
          <div v-for="(item, index) in cartStore.cart" :key="item.id" class="p-3 rounded-xl border border-slate-100 bg-white shadow-sm relative">
            <div class="flex justify-between items-start mb-2">
              <div class="font-bold text-slate-800 text-sm truncate pr-8">{{ item.name }}</div>
              <el-button type="danger" link :icon="Delete" class="!p-0 absolute right-3 top-3" @click="cartStore.removeFromCart(index)" />
            </div>
            
            <div class="flex justify-between items-center mt-2">
              <div class="text-xs text-slate-600 font-medium">作为赠品 (不计入总价)</div>
              <el-switch v-model="item.isGift" size="small" />
            </div>
            
            <div class="flex justify-between items-center mt-2" :class="{'opacity-40 pointer-events-none': item.isGift}">
              <div class="text-xs text-slate-600">单价 (底价: ¥{{ item.minPrice.toFixed(2) }})</div>
              <div class="flex items-center">
                <el-input-number v-if="!item.isGift" v-model="item.price" :min="0.01" :precision="2" :controls="false" size="small" class="!w-24 pos-input-number" :class="{ 'price-warning': item.price < item.minPrice }" />
                <span v-else class="text-red-500 font-bold px-2 py-1 bg-red-50 rounded text-xs">¥0.00 (赠品)</span>
              </div>
            </div>
            
            <div class="flex justify-between items-center mt-2">
              <div class="text-xs text-slate-600">实物库存 (门店:{{ item.store_stock == null ? '盘点中' : item.store_stock }} | 云仓:{{ item.cloud_stock == null ? '盘点中' : item.cloud_stock || 0 }})</div>
              <el-input-number v-model="item.quantity" :min="1" size="small" class="!w-28 pos-input-number" />
            </div>
            
            <div v-if="getStockWarning(item)" class="text-red-500 text-[10px] mt-1.5 flex items-center gap-1 font-bold">
              <el-icon><Warning /></el-icon>{{ getStockWarning(item) }}
            </div>
          </div>
        </div>

        <!-- 客户与订单信息 -->
        <div class="bg-white p-3.5 rounded-xl border border-slate-100 shadow-sm space-y-4">
          <div class="flex items-center gap-2 mb-1">
            <el-icon class="text-blue-600"><User /></el-icon>
            <span class="text-sm font-bold text-slate-700">客户信息录入</span>
          </div>
          
          <div class="space-y-3">
            <div class="space-y-1">
              <span class="text-xs text-slate-500 font-medium pl-1">客户姓名</span>
              <el-input v-model="customerName" placeholder="请输入姓名" class="pos-customer-input" size="large" />
            </div>
            <div class="space-y-1">
              <span class="text-xs text-slate-500 font-medium pl-1">手机号码</span>
              <el-input v-model="customerPhone" type="tel" placeholder="请输入手机号" class="pos-customer-input" size="large" />
            </div>
            <div class="space-y-1">
              <span class="text-xs text-slate-500 font-medium pl-1">收货地址</span>
              <el-input v-model="customerAddress" placeholder="请输入客户详细地址" class="pos-customer-input" size="large" />
            </div>
            <div class="space-y-1">
              <span class="text-xs text-slate-500 font-medium pl-1">开单员工</span>
              <el-select v-model="employeeId" placeholder="选择销售员工 (可选)" class="w-full" clearable size="large">
                <el-option v-for="emp in employees" :key="emp.id" :label="emp.name" :value="emp.id" />
              </el-select>
            </div>
            <div class="space-y-1">
              <span class="text-xs text-slate-500 font-medium pl-1">推荐人/渠道</span>
              <el-input v-model="referrerName" placeholder="选填" class="pos-customer-input" size="large" />
              <span class="text-[10px] text-slate-400">渠道奖励实际支出请在专业记账中登记。</span>
            </div>
          </div>
        </div>

        <!-- 发货方式 -->
        <div class="bg-white p-3.5 rounded-xl border border-slate-100 shadow-sm space-y-3">
          <div class="text-xs text-slate-400 uppercase font-bold tracking-wider">发货/提货方式</div>
          <div class="grid grid-cols-2 gap-2">
            <div class="flex flex-col items-center justify-center py-3 rounded-xl border-2 transition-all cursor-pointer"
                 :class="deliveryMethod === 1 ? 'border-blue-500 bg-blue-50/30 text-blue-600' : 'border-slate-100 bg-slate-50 text-slate-500'"
                 @click="deliveryMethod = 1">
              <el-icon size="20" class="mb-1"><Shop /></el-icon>
              <span class="text-[11px] font-bold">门店自提</span>
            </div>
            <div class="flex flex-col items-center justify-center py-3 rounded-xl border-2 transition-all cursor-pointer"
                 :class="deliveryMethod === 4 ? 'border-blue-500 bg-blue-50/30 text-blue-600' : 'border-slate-100 bg-slate-50 text-slate-500'"
                 @click="deliveryMethod = 4">
              <el-icon size="20" class="mb-1"><Cloudy /></el-icon>
              <span class="text-[11px] font-bold">云仓发货</span>
              <span class="mt-0.5 text-[9px] opacity-70">实物优先，不限量代发</span>
            </div>
          </div>
        </div>

        <!-- 收款账户 -->
        <div class="bg-white p-3.5 rounded-xl border border-slate-100 shadow-sm space-y-3">
          <div class="text-xs text-slate-400 uppercase font-bold tracking-wider">收款账户</div>
          <div class="grid grid-cols-3 gap-2">
            <div class="flex flex-col items-center justify-center py-3 rounded-xl border-2 transition-all cursor-pointer"
                 :class="[paymentMethod === 1 ? 'border-blue-500 bg-blue-50/30 text-blue-600' : 'border-slate-100 bg-slate-50 text-slate-500', isSubsidized ? 'opacity-40 grayscale' : '']"
                 @click="!isSubsidized && (paymentMethod = 1)">
              <el-icon size="20" class="mb-1"><Wallet /></el-icon>
              <span class="text-[11px] font-bold">支付宝</span>
            </div>
            <div class="flex flex-col items-center justify-center py-3 rounded-xl border-2 transition-all cursor-pointer"
                 :class="[paymentMethod === 2 ? 'border-blue-500 bg-blue-50/30 text-blue-600' : 'border-slate-100 bg-slate-50 text-slate-500', isSubsidized ? 'opacity-40 grayscale' : '']"
                 @click="!isSubsidized && (paymentMethod = 2)">
              <el-icon size="20" class="mb-1"><CreditCard /></el-icon>
              <span class="text-[11px] font-bold">微信支付</span>
            </div>
            <div class="flex flex-col items-center justify-center py-3 rounded-xl border-2 transition-all cursor-pointer"
                 :class="paymentMethod === 3 ? 'border-blue-500 bg-blue-50/30 text-blue-600' : 'border-slate-100 bg-slate-50 text-slate-500'"
                 @click="paymentMethod = 3">
              <el-icon size="20" class="mb-1"><OfficeBuilding /></el-icon>
              <span class="text-[11px] font-bold">对公转账</span>
            </div>
            <div class="flex flex-col items-center justify-center py-3 rounded-xl border-2 transition-all cursor-pointer"
                 :class="paymentMethod === 4 ? 'border-blue-500 bg-blue-50/30 text-blue-600' : 'border-slate-100 bg-slate-50 text-slate-500'"
                 @click="paymentMethod = 4">
              <el-icon size="20" class="mb-1"><CreditCard /></el-icon>
              <span class="text-[11px] font-bold">混合支付</span>
            </div>
            <div class="flex flex-col items-center justify-center py-3 rounded-xl border-2 transition-all cursor-pointer"
                 :class="paymentMethod === 5 ? 'border-blue-500 bg-blue-50/30 text-blue-600' : 'border-slate-100 bg-slate-50 text-slate-500'"
                 @click="paymentMethod = 5">
              <el-icon size="20" class="mb-1"><Money /></el-icon>
              <span class="text-[11px] font-bold">现金</span>
            </div>
          </div>
          
          <div v-if="paymentMethod === 4" class="mt-2 p-3 rounded-xl border border-blue-100 bg-blue-50/50 space-y-2">
            <div class="flex justify-between items-center text-xs font-bold">
              <span class="text-blue-700">拆分收款金额</span>
              <span :class="mixedPaymentRemaining === 0 ? 'text-emerald-600' : 'text-red-500'">
                差额 ¥{{ mixedPaymentRemaining.toFixed(2) }}
                <el-button v-if="mixedPaymentRemaining > 0 && mixedPaymentRemaining <= 10" size="small" type="primary" link @click="handleWipeZero">抹零</el-button>
              </span>
            </div>
            <div class="grid grid-cols-2 gap-2">
              <div>
                <span class="text-[10px] text-slate-400 font-medium pl-1">支付宝</span>
                <el-input-number v-model="mixedPaymentAmounts[1]" :min="0" :precision="2" :controls="false" class="w-full pos-customer-input" />
              </div>
              <div>
                <span class="text-[10px] text-slate-400 font-medium pl-1">微信</span>
                <el-input-number v-model="mixedPaymentAmounts[2]" :min="0" :precision="2" :controls="false" class="w-full pos-customer-input" />
              </div>
              <div>
                <span class="text-[10px] text-slate-400 font-medium pl-1">公户</span>
                <el-input-number v-model="mixedPaymentAmounts[3]" :min="0" :precision="2" :controls="false" class="w-full pos-customer-input" />
              </div>
              <div>
                <span class="text-[10px] text-slate-400 font-medium pl-1">现金</span>
                <el-input-number v-model="mixedPaymentAmounts[5]" :min="0" :precision="2" :controls="false" class="w-full pos-customer-input" />
              </div>
            </div>
          </div>
        </div>

        <!-- 营销选项: 以旧换新 / 定金模式 -->
        <div class="space-y-3">
          <div class="flex flex-col gap-3">
            <div class="flex justify-between items-center p-3.5 rounded-xl bg-white border border-slate-100 shadow-sm cursor-pointer" @click="isTradeIn = !isTradeIn">
              <span class="text-sm font-bold text-slate-700 flex items-center gap-2">
                <el-icon :size="18" color="#10b981"><Refresh /></el-icon>以旧换新抵扣
              </span>
              <el-switch v-model="isTradeIn" size="small" @click.stop />
            </div>
            <div v-if="isTradeIn" class="bg-emerald-50 p-3 rounded-xl border border-emerald-100 space-y-3 -mt-1">
              <div class="grid grid-cols-2 gap-2">
                <div>
                  <span class="text-[10px] text-slate-400 font-medium pl-1">折抵金额(元)</span>
                  <el-input-number v-model="tradeInAmount" :min="0" :precision="2" class="w-full pos-customer-input" :controls="false" placeholder="抵扣金额" />
                </div>
                <div>
                  <span class="text-[10px] text-slate-400 font-medium pl-1">旧件品类</span>
                  <el-select v-model="tradeInType" placeholder="选择/输入" class="w-full" filterable allow-create default-first-option>
                    <el-option label="旧电视" value="旧电视" />
                    <el-option label="旧冰箱" value="旧冰箱" />
                    <el-option label="旧洗衣机" value="旧洗衣机" />
                    <el-option label="旧空调" value="旧空调" />
                    <el-option label="旧热水器" value="旧热水器" />
                  </el-select>
                </div>
              </div>
              <div>
                <span class="text-[10px] text-slate-400 font-medium pl-1">实际付款账户</span>
                <el-select v-model="tradeInAccountId" placeholder="请选择付款账户" class="w-full" clearable>
                  <el-option v-for="account in accounts" :key="account.id" :label="`${account.name}（账面 ¥${Number(account.balance || 0).toFixed(2)}）`" :value="account.id" />
                </el-select>
              </div>
              <div>
                <span class="text-[10px] text-slate-400 font-medium pl-1">旧件备注</span>
                <el-input v-model="tradeInRemark" placeholder="品牌/成色/是否通电等" class="pos-customer-input" />
              </div>
            </div>

            <div class="flex justify-between items-center p-3.5 rounded-xl bg-white border border-slate-100 shadow-sm cursor-pointer" @click="isDepositMode = !isDepositMode">
              <span class="text-sm font-bold text-slate-700 flex items-center gap-2">
                <el-icon :size="18" color="#8b5cf6"><Wallet /></el-icon>支付定金模式
              </span>
              <el-switch v-model="isDepositMode" size="small" @click.stop />
            </div>
            <div v-if="isDepositMode" class="bg-purple-50 p-3 rounded-xl border border-purple-100 -mt-1">
              <div class="text-xs font-bold text-purple-700 mb-1.5">输入预付定金金额 (元)</div>
              <el-input-number v-model="depositAmount" :min="0.01" :max="totalPayable" :precision="2" class="w-full" :controls="false" placeholder="输入定金金额" />
            </div>
          </div>
        </div>

      </div>

      <!-- 底部操作与结算悬浮栏 -->
      <div class="absolute bottom-0 left-0 right-0 bg-white border-t border-slate-200 shadow-[0_-10px_20px_rgba(0,0,0,0.03)] z-10 flex flex-col pb-safe">
        <!-- 快捷操作区 (横向滚动) -->
        <div class="p-2.5 bg-slate-50/50 border-b border-slate-100 flex gap-2 overflow-x-auto no-scrollbar">
          <div class="shrink-0 flex items-center gap-1.5 px-3 py-1.5 rounded-lg border text-xs font-bold transition-colors cursor-pointer"
               :class="orderType === 1 ? 'bg-blue-50 border-blue-200 text-blue-600' : 'bg-white border-slate-200 text-slate-500'"
               @click="orderType = 1">
            <el-icon><Document /></el-icon>普通订单
          </div>
          
          <div class="shrink-0 flex items-center gap-1.5 px-3 py-1.5 rounded-lg border text-xs font-bold transition-colors cursor-pointer"
               :class="orderType === 2 ? 'bg-amber-50 border-amber-200 text-amber-600' : 'bg-white border-slate-200 text-slate-500'"
               @click="orderType = 2">
            <el-icon><Star /></el-icon>商会订单
          </div>
          
          <div class="shrink-0 flex items-center gap-1.5 px-3 py-1.5 rounded-lg border text-xs font-bold transition-colors cursor-pointer"
               :class="isSubsidized ? 'bg-orange-50 border-orange-200 text-orange-600' : 'bg-white border-slate-200 text-slate-500'"
               @click="isSubsidized = !isSubsidized">
            <el-icon><Promotion /></el-icon>参与国补
          </div>
        </div>
        
        <!-- 结算核心区 -->
        <div class="p-4 flex items-center justify-between bg-white">
           <div class="flex flex-col">
             <div class="text-[10px] text-slate-500 flex gap-2 font-medium">
               <span>合计: ¥{{ originalTotal.toFixed(2) }}</span>
               <span v-if="isSubsidized" class="text-orange-500">-¥{{ subsidyAmount.toFixed(2) }}</span>
               <span v-if="isTradeIn" class="text-emerald-500">-¥{{ (tradeInAmount || 0).toFixed(2) }}</span>
             </div>
             <div class="text-xl font-black text-blue-600 font-mono flex items-center gap-1 mt-0.5">
               <span class="text-[10px] text-slate-800 tracking-wider">{{ isDepositMode ? '预收定金' : '应付全款' }}</span>
               ¥{{ actualReceipt.toFixed(2) }}
             </div>
           </div>
           <el-button type="primary" class="!rounded-xl !h-11 !px-6 pos-submit-btn" :disabled="cartStore.cart.length === 0 || hasStockViolation" :loading="submitLoading" @click="handleCheckout">
             <span class="font-bold text-sm tracking-widest">确认开单</span>
           </el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { Delete, Promotion, Wallet, User, Warning, Files, Shop, Cloudy, CreditCard, OfficeBuilding, Refresh, Document, Star , Close, Setting, Money } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import request from '../../utils/request'
import { buildReceiptFromCheckout } from '../../utils/receipt'
import { useCartStore } from '../../store/cart'

const props = defineProps({
  isMobile: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['checkout-success'])

const cartStore = useCartStore()

const employees = ref([])
const accounts = ref([])
const submitLoading = ref(false)

const mobileDrawerVisible = ref(false)
const mobileDrawer = ref('') // 'customer' or 'settings'

const openMobileDrawer = (type) => {
  mobileDrawer.value = type
  mobileDrawerVisible.value = true
}


// 结算状态
const isSubsidized = ref(false)
const isTradeIn = ref(false)
const tradeInAmount = ref(0)
const tradeInAccountId = ref(null)
const tradeInType = ref('')
const tradeInRemark = ref('')
const isDepositMode = ref(false)
const depositAmount = ref(0)

const customerName = ref('')
const customerPhone = ref('')
const customerAddress = ref('')
const employeeId = ref(null)
const referrerName = ref('')
const orderType = ref(1)
const paymentMethod = ref(1)
const mixedPaymentAmounts = ref({ 1: 0, 2: 0, 3: 0, 5: 0 })
const deliveryMethod = ref(4) // 1-门店自提, 4-优先从云仓发货
const wipeZeroAmount = ref(0) // 手动抹零金额

const originalTotal = computed(() => {
  return cartStore.cart.reduce((sum, item) => sum + (item.isGift ? 0 : item.price * item.quantity), 0)
})

const subsidyAmount = computed(() => {
  if (!isSubsidized.value) return 0
  let base = isTradeIn.value ? Math.max(0, originalTotal.value - (tradeInAmount.value || 0)) : originalTotal.value
  
  if (paymentMethod.value === 4) {
    base = mixedPaymentAmounts.value[3] || 0
  }

  const amount = base * 0.15
  return amount > 1500 ? 1500 : Number(amount.toFixed(2))
})

const totalPayable = computed(() => {
  const discount = isTradeIn.value ? (tradeInAmount.value || 0) : 0
  return Math.max(0, originalTotal.value - subsidyAmount.value - discount - wipeZeroAmount.value)
})

const actualReceipt = computed(() => {
  return isDepositMode.value ? depositAmount.value : totalPayable.value
})

const mixedPaymentTotal = computed(() => {
  return Object.values(mixedPaymentAmounts.value).reduce((sum, amount) => sum + (Number(amount) || 0), 0)
})

const mixedPaymentTarget = computed(() => {
  // 国补混付中的公户填写 POS 原始刷卡额（含国补）；后端会从公户净到账中扣除国补。
  return actualReceipt.value + (isSubsidized.value && !isDepositMode.value ? subsidyAmount.value : 0)
})

const mixedPaymentRemaining = computed(() => {
  return Number((mixedPaymentTarget.value - mixedPaymentTotal.value).toFixed(2))
})

const handleWipeZero = () => {
  const diff = mixedPaymentRemaining.value
  ElMessageBox.confirm(`确定要抹去差额 ${diff.toFixed(2)} 元吗？`, '抹零确认', {
    confirmButtonText: '确定抹零',
    cancelButtonText: '取消',
    type: 'warning'
  }).then(() => {
    wipeZeroAmount.value += diff
    ElMessage.success(`已抹零 ${diff.toFixed(2)} 元`)
  }).catch(() => {})
}

const hasPriceViolation = computed(() => {
  return cartStore.cart.some(item => !item.isGift && item.price < item.minPrice)
})

const getStockWarning = (item) => {
  if (deliveryMethod.value === 1 && item.store_stock != null && item.quantity > item.store_stock) return `门店库存不足 (仅剩 ${item.store_stock})`
  return ''
}

const hasStockViolation = computed(() => {
  return cartStore.cart.some(item => getStockWarning(item) !== '')
})

// 自动计算默认定金：50% 并取整
watch([isDepositMode, totalPayable], ([newMode, newTotal]) => {
  if (newMode) {
    const half = newTotal * 0.5
    if (half >= 1000) {
      depositAmount.value = Math.floor(half / 500) * 500
    } else if (half >= 100) {
      depositAmount.value = Math.floor(half / 100) * 100
    } else {
      depositAmount.value = Math.floor(half)
    }
  }
})

// 国补订单不能单独走支付宝/微信，可走公户或混合支付
watch(isSubsidized, (newVal) => {
  if (newVal && [1, 2].includes(paymentMethod.value)) {
    paymentMethod.value = 3
  }
})

watch(paymentMethod, (newVal) => {
  if (newVal === 4 && mixedPaymentTotal.value === 0) {
    mixedPaymentAmounts.value = { 1: Number(mixedPaymentTarget.value.toFixed(2)), 2: 0, 3: 0, 5: 0 }
  }
})

watch(mixedPaymentTarget, (newTotal) => {
  if (paymentMethod.value === 4 && mixedPaymentTotal.value === 0) {
    mixedPaymentAmounts.value = { 1: Number(newTotal.toFixed(2)), 2: 0, 3: 0, 5: 0 }
  }
})

const fetchEmployees = async () => {
  try {
    const response = await request.get('/api/employees')
    if (response.code === 200) {
      employees.value = (response.result || []).filter(employee => !employee.is_outsourced)
    }
  } catch (error) {
    console.error('Fetch employees error:', error)
  }
}

const fetchAccounts = async () => {
  try {
    const response = await request.get('/api/accounts')
    if (response.code === 200) {
      accounts.value = response.result
    }
  } catch (error) {
    console.error('Fetch accounts error:', error)
  }
}

const buildPaymentSplits = () => {
  if (paymentMethod.value !== 4) return undefined
  return [1, 2, 3, 5]
    .map(method => ({
      payment_method: method,
      amount: Number((mixedPaymentAmounts.value[method] || 0).toFixed(2))
    }))
    .filter(split => split.amount > 0)
}

const getPaymentBreakdownText = (splits) => {
  if (!splits || splits.length === 0) return ''
  const labels = { 1: '支付宝', 2: '微信', 3: '公户', 5: '现金' }
  return splits.map(split => `${labels[split.payment_method] || '其他'} ¥${Number(split.amount || 0).toFixed(2)}`).join(' / ')
}

const createReceiptSnapshot = (orderNo, paymentSplits) => {
  const employee = employees.value.find(emp => emp.id === employeeId.value)
  return buildReceiptFromCheckout({
    orderNo,
    createdAt: new Date(),
    customer: {
      name: customerName.value,
      phone: customerPhone.value,
      address: customerAddress.value
    },
    items: cartStore.cart.map(item => ({ ...item })),
    totals: {
      totalAmount: originalTotal.value,
      subsidyAmount: subsidyAmount.value,
      tradeInAmount: isTradeIn.value ? tradeInAmount.value : 0,
      discountAmount: wipeZeroAmount.value,
      actualPayAmount: totalPayable.value,
      receiptAmount: actualReceipt.value,
      depositAmount: isDepositMode.value ? depositAmount.value : 0
    },
    paymentMethod: paymentMethod.value,
    deliveryMethod: deliveryMethod.value,
    employeeName: employee?.name || '',
    referrerName: referrerName.value,
    paymentBreakdown: getPaymentBreakdownText(paymentSplits)
  })
}

const handleCheckout = async () => {
  if (!customerName.value) return ElMessage.warning('请输入客户姓名')
  if (!customerPhone.value) return ElMessage.warning('请输入客户手机号')
  if (!customerAddress.value) return ElMessage.warning('请输入客户收货地址')
  if (isTradeIn.value && tradeInAmount.value > 0 && !tradeInAccountId.value) return ElMessage.warning('请选择以旧换新付款账户')

  if (hasPriceViolation.value) {
    await ElMessageBox.confirm(
      '部分商品成交价低于成本底价，确定要强制开单吗？',
      '价格授权确认',
      {
        confirmButtonText: '强制开单',
        cancelButtonText: '取消',
        type: 'error',
      }
    )
  }

  const type = isDepositMode.value ? '定金' : '全款'
  const amount = actualReceipt.value.toFixed(2)
  const paymentSplits = buildPaymentSplits()
  if (paymentMethod.value === 4) {
    if (!paymentSplits.length) return ElMessage.warning('请输入混合支付金额')
    if (Math.abs(mixedPaymentRemaining.value) > 0.01) return ElMessage.warning('混合支付金额合计必须等于本次应收')
  }
  
  try {
    await ElMessageBox.confirm(
      `确认收取客户 ${type} ¥${amount} 并生成订单吗？`,
      '结算确认',
      {
        confirmButtonText: '确认',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )
    
    submitLoading.value = true

    // 收款账户不再由前端按账户名称文本匹配猜测，统一由后端根据支付方式
    // （payment_method / payment_splits）解析出规范收款账户，避免同名/错位账户记错账。

    const payload = {
      items: cartStore.cart.map(item => ({
        product_id: item.id,
        quantity: item.quantity,
        unit_price: item.price,
        is_gift: item.isGift
      })),
      is_subsidy: isSubsidized.value,
      deposit_amount: isDepositMode.value ? depositAmount.value : 0,
      customer_name: customerName.value,
      customer_phone: customerPhone.value,
      customer_address: customerAddress.value,
      employee_id: employeeId.value || undefined,
      referrer_name: referrerName.value,
      payment_method: paymentMethod.value,
      payment_splits: paymentSplits,
      delivery_method: deliveryMethod.value,
      bypass_price_check: hasPriceViolation.value,
      trade_in_amount: isTradeIn.value ? tradeInAmount.value : 0,
      trade_in_account_id: isTradeIn.value ? tradeInAccountId.value : undefined,
      trade_in_type: isTradeIn.value ? tradeInType.value : '',
      trade_in_remark: isTradeIn.value ? tradeInRemark.value : '',
      discount_amount: wipeZeroAmount.value,
      order_type: orderType.value
    }
    
    const response = await request.post('/api/orders', payload)
    
    if (response.code === 200) {
      const receipt = createReceiptSnapshot(response.data?.order_no || response.order_no || response.result?.order_no, paymentSplits)
      ElMessage.success('开单成功！')
      cartStore.clearCart()
      isSubsidized.value = false
      isTradeIn.value = false
      tradeInAmount.value = 0
      tradeInAccountId.value = null
      tradeInType.value = ''
      tradeInRemark.value = ''
      isDepositMode.value = false
      depositAmount.value = 0
      customerName.value = ''
      customerPhone.value = ''
      customerAddress.value = ''
      wipeZeroAmount.value = 0
      employeeId.value = null
      referrerName.value = ''
      orderType.value = 1
      paymentMethod.value = 1
      mixedPaymentAmounts.value = { 1: 0, 2: 0, 3: 0, 5: 0 }
      deliveryMethod.value = 4
      
      emit('checkout-success', receipt)
    } else {
      ElMessage.error(response.msg || '开单失败')
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Order submission error:', error)
      const errorMsg = error.response?.data?.msg || error.message || '提交失败，请重试'
      ElMessage.error(errorMsg)
    }
  } finally {
    submitLoading.value = false
  }
}

onMounted(() => {
  fetchEmployees()
  fetchAccounts()
})
</script>

<style scoped>
:deep(.pos-input-number.el-input-number .el-input__wrapper) {
  padding: 0 8px;
  background: #f8fafc;
  border-radius: 8px;
}

.price-warning :deep(.el-input__wrapper) {
  box-shadow: 0 0 0 1px #ef4444 inset !important;
  background-color: #fef2f2 !important;
}

:deep(.pos-customer-input .el-input__wrapper) {
  background-color: #ffffff;
  border-radius: 8px;
  box-shadow: 0 1px 2px 0 rgba(0, 0, 0, 0.05) !important;
}

:deep(.pos-customer-input .el-input__inner) {
  font-size: 13px;
}

.pos-submit-btn {
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%) !important;
  transition: all 0.3s ease;
}

.pos-submit-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 8px 12px -3px rgba(37, 99, 235, 0.2);
}

.pos-submit-btn:active {
  transform: translateY(0);
}
</style>
