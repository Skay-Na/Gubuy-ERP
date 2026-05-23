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
      <!-- Modern Compact Header -->
      <div class="bg-white border-b border-slate-100 px-4 md:px-6 py-2.5 md:py-4 flex items-center justify-between shadow-sm sticky top-0 z-50">
        <div class="flex items-center gap-3 md:gap-6">
          <div class="flex items-center gap-2 md:gap-3">
            <div class="bg-blue-600 p-1.5 md:p-2 rounded-lg md:rounded-xl shadow-lg shadow-blue-200 text-white flex items-center justify-center">
              <el-icon size="18" class="md:hidden"><Shop /></el-icon>
              <el-icon size="24" class="hidden md:block"><Shop /></el-icon>
            </div>
            <div>
              <h1 class="text-base md:text-xl font-black text-slate-800 tracking-tighter leading-tight">收银终端</h1>
              <div class="flex items-center gap-1.5">
                <span class="inline-block w-1.5 h-1.5 rounded-full bg-green-500"></span>
                <span class="text-[9px] md:text-[10px] font-bold text-slate-400 uppercase tracking-widest">
                  {{ currentEmployee.name }} 
                  <span class="ml-1 pl-1 border-l border-slate-200 text-slate-500 font-mono">{{ currentTime.time.substring(0, 5) }}</span>
                </span>
              </div>
            </div>
          </div>

          <!-- Empty spacer for desktop -->
          <div class="hidden md:block"></div>
        </div>

        <div class="flex items-center gap-4 md:gap-8">
          <!-- Digital Clock (Hidden on Mobile) -->
          <div class="hidden lg:flex flex-col items-end border-r border-slate-100 pr-8">
            <div class="text-2xl font-black text-slate-800 tabular-nums leading-none tracking-tighter">{{ currentTime.time }}</div>
            <div class="text-[10px] font-bold text-slate-400 uppercase tracking-widest mt-1">{{ currentTime.date }} {{ currentTime.weekday }}</div>
          </div>

          <div class="flex items-center gap-3 md:gap-6">
            <!-- Attendance Buttons for Mobile/Desktop -->
            <div class="flex items-center">
              <el-button 
                v-if="!todayAttendance?.check_in" 
                type="success" 
                size="small"
                class="!rounded-lg !px-2 md:!px-4 !h-8" 
                @click="handleCheckIn"
                :loading="attendanceLoading"
              >
                <el-icon class="md:mr-1"><Calendar /></el-icon>
                <span class="text-xs md:text-sm font-bold">上班</span>
              </el-button>
              <el-button 
                v-else-if="!todayAttendance?.check_out" 
                type="warning" 
                size="small"
                class="!rounded-lg !px-2 md:!px-4 !h-8" 
                @click="handleCheckOut"
                :loading="attendanceLoading"
              >
                <el-icon class="md:mr-1"><Calendar /></el-icon>
                <span class="text-xs md:text-sm font-bold">下班</span>
              </el-button>
              <div v-else class="hidden md:flex items-center gap-1 text-green-500 text-[10px] font-bold">
                <el-icon><Calendar /></el-icon>已打卡
              </div>
            </div>

            <div class="flex items-center gap-1 cursor-pointer text-slate-600 hover:text-blue-600 transition-colors" @click="openOrdersDrawer">
              <el-icon size="18" class="md:size-[20px]"><List /></el-icon>
              <span class="text-xs md:text-sm font-bold">订单</span>
            </div>
            <div class="flex items-center gap-1 cursor-pointer text-slate-400 hover:text-red-500 transition-colors" @click="logout">
              <el-icon size="18" class="md:size-[20px]"><SwitchButton /></el-icon>
              <span class="text-xs md:text-sm font-bold">退出</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Compact Search & Filter Bar -->
      <div class="bg-white border-b border-slate-50 px-4 md:px-6 py-2 md:py-3 shadow-sm flex flex-col gap-2">
        <div class="flex gap-2 w-full items-center">
          <el-input
            v-model="searchQuery"
            placeholder="搜型号、名称、备注..."
            class="terminal-search-input flex-1"
            clearable
            @input="handleSearch"
          >
            <template #prefix>
              <el-icon size="16" class="text-blue-500"><Search /></el-icon>
            </template>
          </el-input>
          <div 
            class="flex items-center gap-1.5 px-3 py-2 rounded-xl border transition-all cursor-pointer select-none"
            :class="onlySpecial ? 'bg-red-50 border-red-200 text-red-600' : 'bg-slate-50 border-slate-100 text-slate-400'"
            @click="toggleSpecial"
          >
            <el-icon size="16" :class="{'animate-pulse': onlySpecial}"><Lightning /></el-icon>
            <span class="text-[10px] font-black whitespace-nowrap">特价</span>
            <el-switch v-model="onlySpecial" size="small" @click.stop @change="handleSearch" class="scale-75 -mr-1" />
          </div>
        </div>
      </div>

      <!-- Category Scrollable Bar -->
      <div class="bg-slate-50/50 px-4 md:px-6 py-2 overflow-hidden border-b border-slate-100 flex items-center gap-2">
        <div class="flex-1 overflow-hidden relative">
          <div class="flex items-center gap-2 overflow-x-auto no-scrollbar pr-10">
            <div 
              class="terminal-chip-sm"
              :class="{ active: selectedCategoryId === '' }"
              @click="selectCategory('')"
            >
              全部
            </div>
            <div 
              v-for="cat in categories" 
              :key="cat.id"
              class="terminal-chip-sm"
              :class="{ active: selectedCategoryId === cat.id }"
              @click="selectCategory(cat.id)"
            >
              {{ cat.name }}
            </div>
          </div>
        </div>

        <div 
          @click="categoryDialogVisible = true"
          class="flex-shrink-0 w-8 h-8 rounded-lg bg-white border border-slate-200 flex items-center justify-center text-slate-500 hover:bg-blue-600 hover:text-white cursor-pointer shadow-sm"
        >
          <el-icon size="16"><Menu /></el-icon>
        </div>
      </div>

      <!-- 中间滚动区域：商品网格 -->
      <div class="flex-1 overflow-y-auto p-4 pb-32">
        <!-- 加载中：骨架屏 -->
        <div v-if="loading" class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 gap-3">
          <el-skeleton v-for="i in 12" :key="i" animated>
            <template #template>
              <div class="bg-white rounded-xl p-3 border border-slate-100 flex flex-col gap-3">
                <el-skeleton-item variant="rect" class="h-9 w-full rounded" />
                <div class="flex gap-1">
                  <el-skeleton-item variant="text" class="w-10 h-5" />
                  <el-skeleton-item variant="text" class="w-10 h-5" />
                </div>
                <div class="mt-auto pt-2 border-t border-slate-50 flex justify-between items-end">
                  <div class="flex flex-col gap-1">
                    <el-skeleton-item variant="text" class="w-16 h-6" />
                    <el-skeleton-item variant="text" class="w-10 h-3" />
                  </div>
                  <el-skeleton-item variant="rect" class="w-8 h-8 rounded-lg" />
                </div>
              </div>
            </template>
          </el-skeleton>
        </div>

        <div v-else-if="productList.length === 0" class="py-10 text-center">
          <el-empty description="暂无商品数据" :image-size="100" />
        </div>
        
        <div v-else class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 gap-3">
          <div 
            v-for="row in productList" 
            :key="row.id" 
            v-memo="[row.id, row.store_stock, row.main_stock, row.cloud_stock, row.special_price, isSpecialActive(row)]"
            class="bg-white rounded-xl shadow-sm border border-slate-100 flex flex-col p-3 hover:border-blue-300 transition-all cursor-pointer group relative overflow-hidden"
            :class="isSpecialActive(row) ? 'special-offer-card border-red-200' : ''"
            @click="addToCart(row)"
          >
            <!-- 特价角标 -->
            <div v-if="isSpecialActive(row)" class="absolute top-0 right-0 z-10">
              <div class="bg-red-500 text-white text-[10px] font-black px-2 py-0.5 rounded-bl-lg shadow-sm flex items-center gap-0.5 animate-pulse">
                <el-icon><Lightning /></el-icon>SALE
              </div>
            </div>
            <div class="flex-1 min-w-0">
              <div class="font-bold text-slate-800 text-sm line-clamp-2 leading-tight h-9 mb-1">{{ row.name }}</div>
              <div class="flex flex-wrap gap-1 mb-2">
                <el-tag :type="row.store_stock > 0 ? 'success' : 'danger'" size="small" class="!px-1.5 !h-5 !text-[10px]">店:{{ row.store_stock }}</el-tag>
                <el-tag :type="row.main_stock > 0 ? 'primary' : 'info'" size="small" class="!px-1.5 !h-5 !text-[10px]">仓:{{ row.main_stock }}</el-tag>
                <el-tag v-if="row.support_cloud" :type="row.cloud_stock > 0 ? 'warning' : 'info'" size="small" class="!px-1.5 !h-5 !text-[10px]">云:{{ row.cloud_stock }}</el-tag>
              </div>
            </div>
            <div class="flex justify-between items-end mt-auto pt-2 border-t border-slate-50">
              <div class="flex flex-col">
                <div v-if="isSpecialActive(row)" class="flex flex-col">
                  <span class="text-[10px] text-slate-400 line-through">¥{{ ((row.avg_cost || row.latest_cost || 0) / (1 - (row.margin_rate || 0))).toFixed(0) }}</span>
                  <div class="flex items-center gap-1">
                    <span class="font-black text-red-600 text-lg leading-none">
                      <span class="text-xs">¥</span>{{ row.special_price.toFixed(0) }}
                    </span>
                    <span class="bg-red-50 text-red-500 text-[9px] px-1 rounded font-bold border border-red-100">
                      立省 ¥{{ (((row.avg_cost || row.latest_cost || 0) / (1 - (row.margin_rate || 0))) - row.special_price).toFixed(0) }}
                    </span>
                  </div>
                </div>
                <div v-else class="flex flex-col">
                  <span class="font-black text-slate-800 text-lg leading-none">¥{{ calculateMinPrice(row).toFixed(0) }}</span>
                </div>
              </div>
              <div class="bg-blue-600 text-white p-1.5 rounded-lg">
                <el-icon :size="16"><Plus /></el-icon>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 底部吸底结算条：增加安全区域适配 -->
      <div class="fixed bottom-0 left-0 w-full bg-white/90 backdrop-blur-md shadow-[0_-4px_20px_rgba(0,0,0,0.1)] px-4 pt-2 pb-[calc(8px+env(safe-area-inset-bottom,0px))] z-40 flex justify-between items-center border-t border-slate-100">
        <div class="flex items-center gap-3 cursor-pointer py-1" @click="openDrawer">
          <el-badge :value="cart.length" :hidden="cart.length === 0" type="danger">
            <div class="bg-blue-600 p-2 rounded-xl text-white shadow-lg shadow-blue-200">
              <el-icon size="22" class="block md:hidden"><ShoppingCart /></el-icon>
              <el-icon size="26" class="hidden md:block"><ShoppingCart /></el-icon>
            </div>
          </el-badge>
          <div class="leading-none">
            <div class="text-[10px] text-slate-400 mb-1 font-bold uppercase tracking-wider">合计金额</div>
            <div class="font-black text-xl text-slate-800 tracking-tighter">¥{{ originalTotal.toFixed(2) }}</div>
          </div>
        </div>
        <el-button type="primary" size="large" round class="!px-6 md:!px-10 !h-11 md:!h-12 font-black shadow-lg shadow-blue-200 active:scale-95 transition-all" @click="openDrawer" :disabled="cart.length === 0">
          立即结算
        </el-button>
      </div>

      <!-- 结算抽屉 (完全复用管理员版逻辑) -->
      <el-drawer v-model="drawerVisible" direction="rtl" :size="isMobile ? '100%' : '500px'" :with-header="false">
        <div class="h-full flex flex-col">
          <!-- 增强头部：增加明确的关闭按钮，适配移动端习惯 -->
          <div class="flex items-center justify-between px-4 py-3 border-b border-slate-100 bg-white sticky top-0 z-10">
            <div class="flex items-center gap-2 cursor-pointer p-1 -ml-1" @click="drawerVisible = false">
              <el-icon size="20" class="text-slate-900"><Back /></el-icon>
              <span class="text-base font-black text-slate-900 tracking-tight">购物车与结算</span>
            </div>
            <div class="p-2 -mr-2 cursor-pointer flex items-center justify-center" @click="drawerVisible = false">
              <el-icon size="22" class="text-slate-400 hover:text-slate-600"><Close /></el-icon>
            </div>
          </div>

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
              <el-input v-model="customerAddress" placeholder="请输入详细收货地址" />

              <!-- 模式切换 -->
              <div class="grid grid-cols-2 gap-2">
                <div 
                  class="flex justify-between items-center p-2.5 rounded-xl bg-white border border-slate-100 shadow-sm cursor-pointer hover:bg-slate-50 transition-all"
                  @click="isSubsidized = !isSubsidized"
                >
                  <span class="text-xs font-bold text-slate-600">国家补贴</span>
                  <el-switch v-model="isSubsidized" size="small" @click.stop />
                </div>
                <div 
                  class="flex justify-between items-center p-2.5 rounded-xl bg-white border border-slate-100 shadow-sm cursor-pointer hover:bg-slate-50 transition-all"
                  @click="isTradeIn = !isTradeIn"
                >
                  <span class="text-xs font-bold text-slate-600">以旧换新</span>
                  <el-switch v-model="isTradeIn" size="small" @click.stop />
                </div>
                <div 
                  class="col-span-2 flex justify-between items-center p-2.5 rounded-xl bg-white border border-slate-100 shadow-sm cursor-pointer hover:bg-slate-50 transition-all"
                  @click="isDepositMode = !isDepositMode"
                >
                  <span class="text-xs font-bold text-slate-600">支付定金模式</span>
                  <el-switch v-model="isDepositMode" size="small" @click.stop />
                </div>
              </div>

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
                    <span class="text-[10px] text-slate-400 font-medium pl-1">旧件备注</span>
                    <el-input 
                      v-model="tradeInRemark" 
                      placeholder="品牌/成色/是否通电等" 
                      class="pos-customer-input" 
                    />
                  </div>
                </div>
              </el-collapse-transition>

              <el-collapse-transition>
                <div v-if="isDepositMode" class="bg-purple-50 p-3 rounded-xl border border-purple-100">
                  <div class="text-[10px] text-purple-400 mb-1 font-bold">预收定金金额</div>
                  <el-input-number v-model="depositAmount" :min="0" :max="totalPayable" :precision="2" class="w-full" :controls="false" placeholder="输入定金金额" />
                </div>
              </el-collapse-transition>

              <div class="space-y-4 pt-2">
                <div>
                  <div class="text-[10px] text-slate-400 mb-2 uppercase font-bold tracking-wider">发货/提货方式</div>
                  <div class="grid grid-cols-3 gap-2">
                    <div 
                      class="flex flex-col items-center justify-center p-2.5 rounded-xl border-2 transition-all cursor-pointer bg-white"
                      :class="deliveryMethod === 1 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200'"
                      @click="deliveryMethod = 1"
                    >
                      <el-icon size="18" class="mb-1"><Shop /></el-icon>
                      <span class="text-[11px] font-bold">门店自提</span>
                    </div>
                    <div 
                      class="flex flex-col items-center justify-center p-2.5 rounded-xl border-2 transition-all cursor-pointer bg-white"
                      :class="deliveryMethod === 2 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200'"
                      @click="deliveryMethod = 2"
                    >
                      <el-icon size="18" class="mb-1"><Box /></el-icon>
                      <span class="text-[11px] font-bold">主仓发货</span>
                    </div>
                    <div 
                      class="flex flex-col items-center justify-center p-2.5 rounded-xl border-2 transition-all cursor-pointer bg-white"
                      :class="deliveryMethod === 3 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200'"
                      @click="deliveryMethod = 3"
                    >
                      <el-icon size="18" class="mb-1"><Cloudy /></el-icon>
                      <span class="text-[11px] font-bold">云仓代发</span>
                    </div>
                  </div>
                </div>
                
                <div>
                  <div class="text-[10px] text-slate-400 mb-2 uppercase font-bold tracking-wider">收款账户</div>
                  <div class="grid grid-cols-3 gap-2">
                    <div 
                      class="flex flex-col items-center justify-center p-2.5 rounded-xl border-2 transition-all cursor-pointer bg-white"
                      :class="[
                        paymentMethod === 1 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200',
                        isSubsidized ? 'opacity-40 cursor-not-allowed grayscale' : ''
                      ]"
                      @click="!isSubsidized && (paymentMethod = 1)"
                    >
                      <el-icon size="18" class="mb-1"><Wallet /></el-icon>
                      <span class="text-[11px] font-bold">支付宝</span>
                    </div>
                    <div 
                      class="flex flex-col items-center justify-center p-2.5 rounded-xl border-2 transition-all cursor-pointer bg-white"
                      :class="[
                        paymentMethod === 2 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200',
                        isSubsidized ? 'opacity-40 cursor-not-allowed grayscale' : ''
                      ]"
                      @click="!isSubsidized && (paymentMethod = 2)"
                    >
                      <el-icon size="18" class="mb-1"><CreditCard /></el-icon>
                      <span class="text-[11px] font-bold">微信支付</span>
                    </div>
                    <div 
                      class="flex flex-col items-center justify-center p-2.5 rounded-xl border-2 transition-all cursor-pointer bg-white"
                      :class="paymentMethod === 3 ? 'border-blue-500 bg-blue-50/30 text-blue-600 shadow-sm' : 'border-slate-100 text-slate-500 hover:border-slate-200'"
                      @click="paymentMethod = 3"
                    >
                      <el-icon size="18" class="mb-1"><OfficeBuilding /></el-icon>
                      <span class="text-[11px] font-bold">对公转账</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="p-4 pb-[calc(16px+env(safe-area-inset-bottom,0px))] bg-white border-t space-y-2 shadow-[0_-10px_20px_-10px_rgba(0,0,0,0.05)] z-20">
            <div v-if="isSubsidized" class="flex justify-between text-xs text-orange-600">
              <span>国家补贴减免</span>
              <span>-¥{{ subsidyAmount.toFixed(2) }}</span>
            </div>
            <div v-if="isTradeIn" class="flex justify-between text-xs text-emerald-600">
              <span>以旧换新折抵</span>
              <span>-¥{{ (tradeInAmount || 0).toFixed(2) }}</span>
            </div>
            <div class="flex justify-between items-end mb-4">
              <span class="text-sm font-bold text-slate-600">{{ isDepositMode ? '预收定金' : '实收合计' }}</span>
              <span class="text-3xl font-black text-blue-600">¥{{ actualReceipt.toFixed(2) }}</span>
            </div>
            <div class="flex flex-col gap-3">
              <el-button type="primary" size="large" class="w-full !h-14 !rounded-2xl !font-bold shadow-lg shadow-blue-100" :loading="loading" @click="handleCheckout">
                确认开单提交
              </el-button>
              <el-button size="large" class="w-full !h-10 !rounded-xl !border-none !bg-slate-50 !text-slate-400" @click="drawerVisible = false">
                取消并继续购物
              </el-button>
            </div>
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
                <div class="flex items-center gap-1.5">
                  <div class="font-bold text-slate-800">{{ order.customer_name || '散客' }}</div>
                  <el-tag v-if="order.trade_in_amount > 0" size="small" type="success" effect="plain" class="!rounded-md !px-1 text-[9px] shrink-0">♻️ 换新</el-tag>
                </div>
              </div>
              <el-tag :type="order.payment_status === 2 ? 'success' : 'warning'" size="small">
                {{ order.payment_status === 1 ? '定金' : '已清' }}
              </el-tag>
            </div>

            <!-- 商品履约明细 -->
            <div class="mb-3 p-3 bg-blue-50/20 rounded-2xl space-y-3">
              <div v-for="item in order.order_items" :key="item.id" class="flex flex-col text-[11px] border-b border-slate-100/50 last:border-0 pb-2 last:pb-0">
                <div class="flex justify-between items-center mb-1">
                  <span class="text-slate-700 font-bold truncate flex-1 mr-2">{{ item.product?.name }}</span>
                  <span class="text-slate-400 font-semibold flex-shrink-0">x{{ item.quantity }}</span>
                </div>
                <!-- 配送与安装状态及动作 -->
                <div class="flex justify-between items-center mt-1 bg-white/50 p-1.5 rounded-lg border border-slate-100/50">
                  <div class="flex gap-2">
                    <span :class="item.is_delivered ? 'text-blue-600' : 'text-orange-500'">
                      {{ item.is_delivered ? '🚚已送' : '📦待送' }}
                    </span>
                    <span :class="item.is_installed ? 'text-green-600' : 'text-slate-400'">
                      {{ item.is_installed ? '✅已装' : '⏳待装' }}
                    </span>
                  </div>
                  <!-- 快捷操作按钮 -->
                  <div v-if="order.order_status === 1 && order.delivery_method !== 1">
                    <el-button 
                      v-if="!item.is_delivered" 
                      type="warning" 
                      size="small" 
                      class="!h-5 !px-2 !py-0 !text-[10px] !rounded-md"
                      @click="handleStaffDeliver(item)"
                    >确认送货</el-button>
                    <el-button 
                      v-else-if="!item.is_installed" 
                      type="primary" 
                      size="small" 
                      class="!h-5 !px-2 !py-0 !text-[10px] !rounded-md"
                      @click="handleStaffInstall(item)"
                    >确认安装</el-button>
                  </div>
                </div>
              </div>
            </div>
            
            <!-- 关键状态追踪 -->
            <div class="grid grid-cols-2 gap-2 mb-4">
              <div class="p-2 rounded-xl bg-slate-50 border border-slate-100">
                <div class="text-[10px] text-slate-400 mb-1">安装进度</div>
                <div class="text-xs font-bold" :class="order.is_installed ? 'text-green-600' : (order.is_delivered ? 'text-blue-600' : 'text-orange-500')">
                  {{ order.is_installed ? '✅ 已安装' : (order.is_delivered ? '🚚 已送货' : (order.order_items && order.order_items.some(it => it.is_delivered || it.is_installed) ? '⏳ 部分履约' : '📦 待送货')) }}
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

    <!-- 移动端专用：全屏品类导航 (Native Mobile Style) -->
    <transition name="el-zoom-in-bottom">
      <div v-if="isMobile && categoryDialogVisible" class="fixed inset-0 z-[2000] bg-white flex flex-col overflow-hidden w-screen h-screen">
        <!-- Mobile Header -->
        <div class="px-4 py-4 border-b border-slate-100 flex items-center justify-between shrink-0">
          <div class="flex items-center gap-2">
            <div class="w-1.5 h-5 bg-blue-600 rounded-full"></div>
            <span class="text-xl font-black text-slate-800">全部分类</span>
          </div>
          <el-icon size="24" class="text-slate-400" @click="categoryDialogVisible = false"><Close /></el-icon>
        </div>

        <!-- Mobile Search Bar -->
        <div class="px-4 py-3 bg-slate-50">
          <el-input
            v-model="categorySearchQuery"
            placeholder="搜索品类名称..."
            size="large"
            class="mobile-cat-search"
            clearable
          >
            <template #prefix>
              <el-icon class="text-blue-500"><Search /></el-icon>
            </template>
          </el-input>
        </div>

        <!-- Mobile Waterfall Grid -->
        <div class="flex-1 overflow-y-auto px-4 py-6">
          <div class="grid grid-cols-3 gap-y-8 gap-x-4">
            <!-- 全部商品 -->
            <div 
              class="mobile-cat-item"
              :class="{ active: selectedCategoryId === '' }"
              @click="selectCategoryAndClose('')"
            >
              <div class="item-icon bg-blue-600 text-white shadow-lg shadow-blue-100">
                <el-icon size="24"><Shop /></el-icon>
              </div>
              <div class="item-text">全部商品</div>
            </div>

            <!-- 品类列表 -->
            <div 
              v-for="cat in filteredCategories" 
              :key="cat.id"
              class="mobile-cat-item"
              :class="{ active: selectedCategoryId === cat.id }"
              @click="selectCategoryAndClose(cat.id)"
            >
              <div class="item-icon bg-slate-100 text-slate-500">
                <span class="text-xl font-black">{{ cat.name.charAt(0) }}</span>
              </div>
              <div class="item-text">{{ cat.name }}</div>
            </div>
          </div>
          
          <el-empty v-if="filteredCategories.length === 0" description="未找到相关分类" />
          <div class="h-20"></div> <!-- Padding for bottom area -->
        </div>
      </div>
    </transition>

    <!-- 桌面端专用：分类导航弹窗 (Desktop Only) -->
    <el-dialog
      v-if="!isMobile"
      v-model="categoryDialogVisible"
      title="全部分类导航"
      width="800px"
      class="!rounded-3xl"
      destroy-on-close
    >
      <div class="flex flex-col gap-6">
        <el-input
          v-model="categorySearchQuery"
          placeholder="搜索品类名称..."
          size="large"
          clearable
        >
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>

        <div class="grid grid-cols-5 gap-6 max-h-[500px] overflow-y-auto pr-2">
          <div 
            class="cat-icon-card"
            :class="{ active: selectedCategoryId === '' }"
            @click="selectCategoryAndClose('')"
          >
            <div class="icon-wrapper bg-blue-600 text-white">
              <el-icon size="24"><Shop /></el-icon>
            </div>
            <div class="cat-label">全部商品</div>
          </div>

          <div 
            v-for="cat in filteredCategories" 
            :key="cat.id"
            class="cat-icon-card"
            :class="{ active: selectedCategoryId === cat.id }"
            @click="selectCategoryAndClose(cat.id)"
          >
            <div class="icon-wrapper bg-slate-100 text-slate-500">
              <span class="text-xl font-black">{{ cat.name.charAt(0) }}</span>
            </div>
            <div class="cat-label">{{ cat.name }}</div>
          </div>
        </div>
      </div>
    </el-dialog>

    <!-- 员工端确认送货弹窗 -->
    <el-dialog v-model="deliveryDialogVisible" title="确认送货完成" width="90%" class="!rounded-2xl max-w-md">
      <div class="space-y-4">
        <el-alert title="确认送货" type="warning" :closable="false" show-icon>请确认实际执行送货的人人员</el-alert>
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">送货人员</label>
          <el-select v-model="selectedDeliveryPersonId" class="w-full" placeholder="请选择送货员" size="large" filterable clearable>
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
            placeholder="选择时间"
            size="large"
            class="!w-full"
            format="YYYY-MM-DD HH:mm"
          />
        </div>
      </div>
      <template #footer>
        <div class="flex gap-2">
          <el-button @click="deliveryDialogVisible = false" class="flex-1">取消</el-button>
          <el-button type="primary" @click="confirmStaffDeliver" :disabled="!selectedDeliveryPersonId" class="flex-1">确认提交</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- 员工端确认安装弹窗 -->
    <el-dialog v-model="installDialogVisible" title="确认安装完成" width="90%" class="!rounded-2xl max-w-md">
      <div class="space-y-4">
        <el-alert title="确认安装" type="info" :closable="false" show-icon>请确认实际执行安装的人员</el-alert>
        <div>
          <label class="block text-sm font-bold text-slate-700 mb-2">安装师傅</label>
          <el-select v-model="selectedInstallerId" class="w-full" placeholder="请选择师傅" size="large" filterable clearable>
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
            placeholder="选择时间"
            size="large"
            class="!w-full"
            format="YYYY-MM-DD HH:mm"
          />
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
                <div class="text-[10px] text-slate-400">备注: {{ item.product?.remark || '-' }}</div>
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
            <div class="text-[10px] text-slate-400 mb-1">配送与安装</div>
            <div class="text-sm font-bold">
              {{ currentDetailOrder.is_installed ? '✅ 已确认安装' : (currentDetailOrder.is_delivered ? '🚚 已送货/待安装' : '⏳ 待送货') }}
            </div>
            <div v-if="currentDetailOrder.is_delivered || currentDetailOrder.is_installed" class="text-[10px] text-slate-400 mt-1 space-y-1">
              <div v-if="currentDetailOrder.is_delivered">送货: {{ currentDetailOrder.delivery_person?.name || '-' }} <span v-if="currentDetailOrder.delivery_person?.is_outsourced" class="text-[8px] bg-amber-50 text-amber-600 px-1 rounded">外包</span></div>
              <div v-if="currentDetailOrder.is_installed">安装: {{ currentDetailOrder.installer?.name || '-' }} <span v-if="currentDetailOrder.installer?.is_outsourced" class="text-[8px] bg-green-50 text-green-600 px-1 rounded">外包</span></div>
              <div v-if="currentDetailOrder.is_installed">时间: {{ formatDate(currentDetailOrder.install_time) }}</div>
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

        <!-- 以旧换新 -->
        <div v-if="currentDetailOrder.trade_in_amount > 0" class="p-4 bg-emerald-50/50 rounded-2xl border border-emerald-100/50 space-y-2">
          <div class="flex justify-between items-center">
            <span class="text-xs font-bold text-emerald-800 flex items-center gap-1">
              <el-icon><Refresh /></el-icon>以旧换新折抵
            </span>
            <span class="text-sm font-bold text-emerald-700">-¥{{ currentDetailOrder.trade_in_amount.toFixed(2) }}</span>
          </div>
          <div class="grid grid-cols-2 gap-2 text-[11px] text-slate-600">
            <div><span class="text-slate-400">旧件品类:</span> {{ currentDetailOrder.trade_in_type || '-' }}</div>
            <div><span class="text-slate-400">旧件备注:</span> {{ currentDetailOrder.trade_in_remark || '-' }}</div>
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
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { Search, ShoppingCart, Delete, Switch, Plus, Back, List, SwitchButton, Shop, Box, Cloudy, CreditCard, OfficeBuilding, Wallet, Close, Lightning, Menu, Calendar, Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'
import dayjs from 'dayjs'

// --- 响应式识别逻辑 ---
const isMobile = ref(false)
const checkMobile = () => {
  // 识别逻辑：屏幕宽度 < 1024px 或者 具备触控功能
  isMobile.value = window.innerWidth < 1024 || (('ontouchstart' in window) || (navigator.maxTouchPoints > 0))
}

// 实时时钟逻辑
const now = ref(dayjs())
const timer = ref(null)

const currentTime = computed(() => {
  const weekdayMap = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
  return {
    time: now.value.format('HH:mm:ss'),
    date: now.value.format('YYYY年MM月DD日'),
    weekday: weekdayMap[now.value.day()]
  }
})

onMounted(() => {
  timer.value = setInterval(() => {
    now.value = dayjs()
  }, 1000)
})

onUnmounted(() => {
  if (timer.value) clearInterval(timer.value)
})

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
const deliveryDialogVisible = ref(false)
const deliveryTargetItem = ref(null)
const selectedDeliveryTime = ref(null)
const installDialogVisible = ref(false)
const installTargetItem = ref(null)
const selectedInstallerId = ref(null)
const selectedDeliveryPersonId = ref(null)
const selectedInstallTime = ref(null)
const detailDialogVisible = ref(false)
const currentDetailOrder = ref(null)

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

const handleStaffDeliver = (item) => {
  deliveryTargetItem.value = item
  selectedDeliveryPersonId.value = null
  selectedDeliveryTime.value = new Date()
  if (employees.value.length === 0) fetchEmployees()
  deliveryDialogVisible.value = true
}

const confirmStaffDeliver = async () => {
  if (!selectedDeliveryPersonId.value) return
  try {
    const res = await axios.put(`/api/order-items/${deliveryTargetItem.value.id}/deliver`, {
      delivery_person_id: selectedDeliveryPersonId.value,
      delivery_time: selectedDeliveryTime.value,
      delivery_fee: 0
    })
    if (res.data.code === 200) {
      ElMessage.success('送货确认成功')
      deliveryDialogVisible.value = false
      fetchMyOrders()
    }
  } catch (error) {
    ElMessage.error('操作失败')
  }
}

const handleStaffInstall = (item) => {
  installTargetItem.value = item
  selectedInstallerId.value = null
  selectedInstallTime.value = new Date()
  if (employees.value.length === 0) fetchEmployees()
  installDialogVisible.value = true
}

const confirmStaffInstall = async () => {
  if (!selectedInstallerId.value) return
  try {
    const res = await axios.put(`/api/order-items/${installTargetItem.value.id}/install`, {
      installer_id: selectedInstallerId.value,
      install_time: selectedInstallTime.value,
      installation_fee: 0
    })
    if (res.data.code === 200) {
      ElMessage.success('安装确认成功')
      installDialogVisible.value = false
      fetchMyOrders()
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '操作失败')
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
const onlySpecial = ref(false)
const categoryDialogVisible = ref(false)
const categorySearchQuery = ref('')
const cart = ref([])
const drawerVisible = ref(false)
const loading = ref(false)
const filteredCategories = computed(() => {
  if (!categorySearchQuery.value) return categories.value
  return categories.value.filter(cat => 
    cat.name.toLowerCase().includes(categorySearchQuery.value.toLowerCase())
  )
})

const customerName = ref('')
const customerPhone = ref('')
const customerAddress = ref('')
const paymentMethod = ref(1)
const deliveryMethod = ref(2)

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

const selectCategoryAndClose = (id) => {
  selectCategory(id)
  categoryDialogVisible.value = false
}

const isSpecialActive = (product) => {
  if (!product.special_price || product.special_price <= 0) return false
  if (!product.special_price_start || !product.special_price_end) return false
  const now = new Date()
  const start = new Date(product.special_price_start)
  const end = new Date(product.special_price_end)
  return now >= start && now <= end
}

const calculateMinPrice = (product) => {
  if (isSpecialActive(product)) {
    return product.special_price
  }
  const cost = Number(product.avg_cost || product.latest_cost) || 0
  const rate = Number(product.margin_rate) || 0
  return cost / (1 - rate)
}

const fetchProducts = async () => {
  loading.value = true
  try {
    const res = await axios.get('/api/products', {
      params: { 
        keyword: searchQuery.value,
        category_id: selectedCategoryId.value,
        only_special: onlySpecial.value ? 1 : 0
      }
    })
    if (res.data.code === 200) {
      productList.value = res.data.data
    }
  } catch (error) {
    console.error('Fetch products error:', error)
  } finally {
    loading.value = false
  }
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
const isTradeIn = ref(false)
const tradeInAmount = ref(0)
const tradeInType = ref('')
const tradeInRemark = ref('')
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
  const discount = isTradeIn.value ? (tradeInAmount.value || 0) : 0
  return Math.max(0, originalTotal.value - subsidyAmount.value - discount)
})

// 自动计算默认定金：50% 并取整 (如 4329 -> 2000)
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

// 强制国补订单使用公户
watch(isSubsidized, (newVal) => {
  if (newVal) {
    paymentMethod.value = 3
  }
})

const actualReceipt = computed(() => {
  return isDepositMode.value ? depositAmount.value : totalPayable.value
})

// 处理物理返回键/手势返回：拦截并关闭结算抽屉
let isInternalBack = false
const handlePopState = () => {
  if (drawerVisible.value) {
    isInternalBack = true
    drawerVisible.value = false
  }
}

watch(drawerVisible, (newVal) => {
  if (newVal) {
    window.history.pushState({ drawer: 'checkout' }, '')
  } else {
    if (!isInternalBack && window.history.state?.drawer === 'checkout') {
      window.history.back()
    }
    isInternalBack = false
  }
})

onMounted(() => {
  window.addEventListener('popstate', handlePopState)
})

onUnmounted(() => {
  window.removeEventListener('popstate', handlePopState)
})

const handleCheckout = async () => {
  if (!customerName.value) return ElMessage.warning('请输入客户姓名')
  if (!customerPhone.value) return ElMessage.warning('请输入客户手机号')
  if (!customerAddress.value) return ElMessage.warning('请输入客户收货地址')
  
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
      customer_address: customerAddress.value,
      employee_id: currentEmployee.value.id,
      payment_method: paymentMethod.value,
      delivery_method: deliveryMethod.value,
      trade_in_amount: isTradeIn.value ? tradeInAmount.value : 0,
      trade_in_type: isTradeIn.value ? tradeInType.value : '',
      trade_in_remark: isTradeIn.value ? tradeInRemark.value : ''
    }
    const res = await axios.post('/api/orders', payload)
    if (res.data.code === 200) {
      ElMessage.success('开单成功！')
      cart.value = []
      customerName.value = ''
      customerPhone.value = ''
      customerAddress.value = ''
      isSubsidized.value = false
      isTradeIn.value = false
      tradeInAmount.value = 0
      tradeInType.value = ''
      tradeInRemark.value = ''
      isDepositMode.value = false
      depositAmount.value = 0
      deliveryMethod.value = 2
      drawerVisible.value = false
      fetchProducts()
    }
  } catch (err) {
    if (err !== 'cancel') {
      const errorMsg = err.response?.data?.msg || '开单失败，请检查网络'
      ElMessage.error(errorMsg)
    }
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  // 响应式监听
  checkMobile()
  window.addEventListener('resize', checkMobile)
  window.addEventListener('popstate', handlePopState)

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
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
  window.removeEventListener('popstate', handlePopState)
})
</script>

<style scoped>
.no-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}

/* Enhanced POS UI Styles */
.terminal-search-input :deep(.el-input__wrapper) {
  border-radius: 12px;
  box-shadow: 0 4px 12px rgba(0,0,0,0.02) inset;
  background-color: #f8fafc;
  border: 1px solid #e2e8f0;
  padding: 4px 12px;
  height: 40px;
  transition: all 0.3s;
}

@media (min-width: 768px) {
  .terminal-search-input :deep(.el-input__wrapper) {
    height: 48px;
    border-radius: 16px;
    padding: 8px 16px;
  }
}

.terminal-search-input :deep(.el-input__wrapper.is-focus) {
  background-color: #fff;
  border-color: #3b82f6;
}

.terminal-chip-sm {
  padding: 6px 14px;
  border-radius: 10px;
  background-color: #ffffff;
  color: #64748b;
  font-size: 12px;
  font-weight: 800;
  cursor: pointer;
  transition: all 0.2s;
  border: 1px solid #f1f5f9;
  white-space: nowrap;
}

.terminal-chip-sm.active {
  background-color: #3b82f6;
  color: #ffffff;
  border-color: #3b82f6;
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.2);
}

.terminal-chip {
  padding: 8px 20px;
  border-radius: 14px;
  background-color: #ffffff;
  color: #64748b;
  font-size: 13px;
  font-weight: 800;
  cursor: pointer;
  transition: all 0.2s;
  border: 1px solid #f1f5f9;
  white-space: nowrap;
  display: flex;
  align-items: center;
  gap: 4px;
}

.terminal-chip:hover {
  background-color: #f8fafc;
  color: #1e293b;
  border-color: #e2e8f0;
}

.terminal-chip.active {
  background-color: #3b82f6;
  color: #ffffff;
  border-color: #3b82f6;
  box-shadow: 0 4px 14px rgba(59, 130, 246, 0.3);
}

/* Category Grid Dialog: Strict Vertical Waterfall */
.category-grid-dialog :deep(.el-dialog__body) {
  padding: 0 !important;
  overflow-x: hidden !important;
}

.category-dialog-search-compact :deep(.el-input__wrapper) {
  border-radius: 12px;
  background-color: #f8fafc;
  border: 1px solid #e2e8f0;
  box-shadow: none;
  height: 44px;
}

/* Mobile Native Category Overlay: Force Viewport Fit */
.fixed.inset-0 {
  width: 100vw !important;
  max-width: 100vw !important;
  height: 100vh !important;
  left: 0 !important;
  top: 0 !important;
  right: 0 !important;
  bottom: 0 !important;
}

/* 桌面弹窗保险：防止在未识别为 Mobile 时溢出 */
:deep(.el-dialog) {
  max-width: 95vw !important;
}

.mobile-cat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  cursor: pointer;
}

.mobile-cat-item .item-icon {
  width: 72px;
  height: 72px;
  border-radius: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
  background-color: #f8fafc;
  border: 1px solid #f1f5f9;
}

.mobile-cat-item .item-text {
  font-size: 13px;
  font-weight: 700;
  color: #475569;
  text-align: center;
  line-height: 1.2;
}

.mobile-cat-item.active .item-icon {
  background-color: #3b82f6;
  color: #ffffff;
  border-color: #3b82f6;
  box-shadow: 0 10px 20px rgba(59, 130, 246, 0.2);
  transform: scale(1.05);
}

.mobile-cat-item.active .item-text {
  color: #1e40af;
}

.mobile-cat-search :deep(.el-input__wrapper) {
  border-radius: 12px;
  background-color: #ffffff;
  box-shadow: 0 2px 8px rgba(0,0,0,0.04);
  border: none;
  padding: 12px 16px;
}

/* Desktop Styles */
.cat-icon-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  transition: all 0.2s;
  padding: 4px;
}

.cat-icon-card .icon-wrapper {
  width: 48px;
  height: 48px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
  position: relative;
}

.cat-icon-card .cat-label {
  font-size: 11px;
  font-weight: 700;
  color: #64748b;
  text-align: center;
  width: 100%;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Active State: Minimalist but Clear */
.cat-icon-card.active .icon-wrapper {
  background-color: #3b82f6;
  color: #ffffff;
  box-shadow: 0 8px 16px rgba(59, 130, 246, 0.3);
  transform: scale(1.05);
}

.cat-icon-card.active .cat-label {
  color: #3b82f6;
}

.cat-icon-card.active .icon-wrapper::after {
  content: '';
  position: absolute;
  bottom: -4px;
  width: 12px;
  height: 3px;
  background-color: #3b82f6;
  border-radius: 2px;
}

.price-warning :deep(.el-input__wrapper) {
  box-shadow: 0 0 0 1px #ef4444 inset !important;
}

.special-offer-card {
  border-color: #fee2e2 !important;
  background: linear-gradient(135deg, #ffffff 0%, #fffcfc 100%) !important;
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.08) !important;
}

.special-offer-card:hover {
  border-color: #ef4444 !important;
}

.special-offer-card .text-slate-800 {
  color: #991b1b !important;
}
</style>
