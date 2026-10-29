<template>
  <div class="product-manage-container">
    <!-- Desktop Header -->
    <div class="hidden md:flex flex-row items-end justify-between mb-8 gap-4">
      <div class="title-wrapper">
        <h1 class="text-2xl md:text-3xl font-black text-slate-900 tracking-tight mb-1">商品库管理</h1>
        <p class="text-sm text-slate-400">维护商品基础档案，设定编码与毛利率</p>
      </div>
      <div class="flex flex-wrap gap-3">
        <el-dropdown trigger="click" @command="handleDesktopMenuCommand">
          <el-button plain class="!rounded-xl h-10 !border-slate-200 hover:!border-indigo-400">
            <el-icon class="mr-1 text-slate-500"><MoreFilled /></el-icon>更多操作<el-icon class="el-icon--right ml-1 text-slate-400"><arrow-down /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="recalculate">
                <el-icon class="text-orange-500"><MagicStick /></el-icon>重构历史成本
              </el-dropdown-item>
              <el-dropdown-item command="category">
                <el-icon><Setting /></el-icon>分类管理
              </el-dropdown-item>
              <el-dropdown-item command="export">
                <el-icon><Download /></el-icon>导出清单
              </el-dropdown-item>
              <el-dropdown-item command="stockFilter" divided>
                <el-icon><Box /></el-icon>{{ onlyInStock ? '取消仅看有库存' : '只显示有库存' }}
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button type="primary" class="!rounded-xl h-10 shadow-md shadow-indigo-100" @click="openAddDialog">
          <el-icon class="mr-1"><Plus /></el-icon>新增商品
        </el-button>
      </div>
    </div>

    <!-- Mobile Header & Filters (Sticky) -->
    <div class="md:hidden sticky top-0 z-10 bg-[#f8fafc] pb-2 pt-2 -mx-4 px-4 mb-4">
      <div class="flex items-center gap-2 mb-3">
        <el-input
          v-model="searchQuery"
          placeholder="搜索商品名称、备注..."
          class="custom-search-input flex-1"
          clearable
          @input="handleSearch"
        >
          <template #prefix><el-icon class="text-blue-500"><Search /></el-icon></template>
        </el-input>
        <el-dropdown trigger="click" @command="handleMobileMenuCommand">
          <div class="w-9 h-9 bg-white rounded-xl flex items-center justify-center shadow-sm border border-slate-200 text-slate-600 active:bg-slate-50">
            <el-icon size="18"><MoreFilled /></el-icon>
          </div>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="recalculate">
                <el-icon class="text-orange-500"><MagicStick /></el-icon>重构历史成本
              </el-dropdown-item>
              <el-dropdown-item command="stockFilter">
                <el-icon><Box /></el-icon>{{ onlyInStock ? '取消仅看有库存' : '只显示有库存' }}
              </el-dropdown-item>
              <el-dropdown-item command="category">
                <el-icon><Setting /></el-icon>分类管理
              </el-dropdown-item>
              <el-dropdown-item command="export">
                <el-icon><Download /></el-icon>导出清单
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
      
      <!-- Mobile Filters: 分类级联 + 特价开关 -->
      <div class="flex items-center gap-2 mt-2">
        <el-cascader
          v-model="selectedCategoryId"
          :options="categoryOptions"
          :props="categoryCascaderProps"
          placeholder="全部分类"
          clearable
          class="flex-1 mobile-category-select"
          @change="selectCategory"
        />
        <div class="flex items-center gap-1 px-3 py-2 rounded-xl border text-xs font-bold transition-all shrink-0"
          :class="onlySpecial ? 'bg-red-50 border-red-200 text-red-600' : 'bg-white border-slate-200 text-slate-500'"
          @click="toggleSpecial"
        >
          <el-icon><Lightning /></el-icon> 特价
        </div>
      </div>
    </div>

    <!-- Desktop Compact Filter Bar -->
    <div class="hidden md:flex bg-white rounded-2xl px-4 py-3 shadow-sm border border-slate-100 mb-6 gap-4 items-center">
        <el-cascader
          v-model="selectedCategoryId"
          :options="categoryOptions"
          :props="categoryCascaderProps"
          placeholder="全部分类（可选大分类或二级）"
          clearable
          filterable
          class="!w-64 category-select"
          @change="selectCategory"
        />
      <div class="w-80">
        <el-input
          v-model="searchQuery"
          placeholder="输入商品名称、备注关键词..."
          class="custom-search-input"
          clearable
          @input="handleSearch"
        >
          <template #prefix>
            <el-icon size="16" class="text-indigo-400"><Search /></el-icon>
          </template>
        </el-input>
      </div>

      <div
        class="flex items-center gap-2 px-4 py-2 rounded-xl border transition-all cursor-pointer select-none ml-auto"
        :class="onlySpecial ? 'bg-red-50 border-red-200 text-red-600 shadow-sm' : 'bg-slate-50 border-slate-100 text-slate-500 hover:border-slate-200'"
        @click="toggleSpecial"
      >
        <el-icon :class="{'animate-pulse': onlySpecial}"><Lightning /></el-icon>
        <span class="text-sm font-bold">仅看特价促销</span>
      </div>
    </div>

    <!-- Data Table Card -->  <!-- Data Table Card -->
    <el-card class="!rounded-2xl border-slate-100 shadow-sm overflow-hidden" shadow="never" body-class="!p-0">
      <!-- PC View -->
      <el-table 
        v-loading="loading"
        :data="paginatedProductList" 
        style="width: 100%" 
        border
        stripe
        highlight-current-row
        class="desktop-only hidden xl:block"
      >
        <el-table-column label="商品档案" min-width="250">
          <template #default="{ row }">
            <div class="compact-cell">
              <strong class="compact-primary" :title="row.name">{{ row.name }}</strong>
              <span class="compact-meta">#{{ row.id }} · {{ categoryLabel(row.category_id) }} · 预设毛利 {{ (row.margin_rate * 100).toFixed(0) }}%</span>
              <span v-if="row.remark || commissionBadge(row)" class="compact-meta" :title="row.remark || ''">{{ row.remark || '' }}<span v-if="commissionBadge(row)" class="text-amber-600">{{ row.remark ? ' · ' : '' }}{{ commissionBadge(row) }}</span></span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="latest_cost" label="成本 / 售价" width="170" align="right">
          <template #default="{ row }">
            <div class="flex flex-col items-end gap-0.5">
              <div class="flex items-center gap-1">
                <span class="text-[10px] text-slate-400">平均:</span>
                <span v-if="Number(row.avg_cost) > 0" class="font-mono text-xs font-bold text-blue-600">¥{{ formatMoney(row.avg_cost) }}</span>
                <span v-else class="font-mono text-xs font-bold text-amber-600">待核</span>
              </div>
              <div class="flex items-center gap-1">
                <span class="text-[10px] text-slate-400">最新:</span>
                <span class="font-mono text-xs text-slate-500">¥{{ formatMoney(row.latest_cost) }}</span>
              </div>
              <div class="mt-1 pt-1 border-t border-slate-50 w-full text-right">
                <span class="font-mono font-bold" :class="isSpecialActive(row) ? 'text-red-500' : 'text-slate-800'">
                  ¥{{ formatMoney(isSpecialActive(row) ? row.special_price : (row.avg_cost || row.latest_cost) / (1 - row.margin_rate)) }}
                </span>
                <el-tag v-if="isSpecialActive(row)" size="small" type="danger" effect="dark" class="ml-1 !scale-75 !px-1">特价</el-tag>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="库存分布" min-width="190">
          <template #default="{ row }">
            <div class="grid grid-cols-2 gap-x-4 gap-y-1 text-[11px]">
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
        <el-table-column label="操作" width="125" align="center">
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
      <div class="xl:hidden p-2 grid grid-cols-1 md:grid-cols-2 gap-3" v-loading="loading">
        <div 
          v-for="row in paginatedProductList" 
          :key="row.id" 
          class="bg-white rounded-[16px] p-3 shadow-[0_2px_8px_rgba(0,0,0,0.04)] border border-slate-100 relative overflow-hidden"
          @click="handleMobileActions(row)"
        >
          <!-- special tag -->
          <div v-if="isSpecialActive(row)" class="absolute top-0 right-0 bg-red-500 text-white text-[10px] px-2 py-0.5 rounded-bl-lg font-bold">特价</div>
          
          <div class="flex justify-between items-start mb-2">
            <div class="flex-1 min-w-0 pr-3">
              <div class="font-bold text-slate-800 text-[15px] leading-tight mb-1 truncate">{{ row.name }}</div>
              <div class="flex items-center gap-1.5 flex-wrap">
                <span v-if="row.remark" class="text-[10px] px-1.5 py-0.5 rounded bg-slate-100 text-slate-500 font-mono">{{ row.remark }}</span>
                <span class="text-[10px] text-slate-400">预设毛利:{{ (row.margin_rate * 100).toFixed(0) }}%</span>
                <span v-if="commissionBadge(row)" class="text-[10px] px-1.5 py-0.5 rounded bg-amber-50 text-amber-600 font-bold">{{ commissionBadge(row) }}</span>
              </div>
            </div>
            <div class="text-right flex-shrink-0 mt-0.5">
              <div class="text-green-600 font-black text-base leading-none">¥{{ formatMoney(row.latest_cost) }}</div>
              <div class="text-[9px] text-slate-400 mt-1">进价</div>
            </div>
          </div>

          <!-- Compact Inventory Badges -->
          <div class="flex gap-1.5 mt-2 flex-wrap">
             <div v-if="row.store_stock > 0" class="flex items-center gap-1 px-1.5 py-0.5 rounded-md bg-orange-50 text-orange-600 text-[11px]">
               <span class="opacity-70">门店</span><span class="font-bold">{{ row.store_stock }}</span>
             </div>
             <div v-if="row.sample_stock > 0" class="flex items-center gap-1 px-1.5 py-0.5 rounded-md bg-purple-50 text-purple-600 text-[11px]">
               <span class="opacity-70">样机</span><span class="font-bold">{{ row.sample_stock }}</span>
             </div>
             <div v-if="row.cloud_stock > 0" class="flex items-center gap-1 px-1.5 py-0.5 rounded-md bg-cyan-50 text-cyan-600 text-[11px]">
               <span class="opacity-70">云仓</span><span class="font-bold">{{ row.cloud_stock }}</span>
             </div>
             <div v-if="!row.store_stock && !row.sample_stock && !row.cloud_stock" class="text-[11px] text-slate-300">
               无可用库存
             </div>
          </div>
        </div>
        
        <el-empty v-if="!loading && productList.length === 0" description="暂无商品" />
        
        <div v-if="productList.length > 0 && productList.length < totalProducts" class="text-center pt-2 pb-6 md:col-span-2">
          <el-button round size="small" plain @click="loadMore" :loading="loading" class="!text-slate-500">
            加载更多 ({{ productList.length }}/{{ totalProducts }})
          </el-button>
        </div>
        <div v-else-if="productList.length > 0" class="text-center pt-2 pb-6 text-xs text-slate-400 md:col-span-2">
          已加载全部
        </div>
      </div>

      <!-- Enhanced Pagination (Desktop Only) -->
      <div class="hidden xl:flex justify-end p-4 border-t border-slate-100 bg-slate-50/30">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 15, 20, 50]"
          layout="total, sizes, prev, pager, next"
          :total="totalProducts"
          background
          class="custom-pagination"
          @current-change="fetchProducts"
          @size-change="handleSizeChange"
        />
      </div>
    </el-card>

    <!-- Mobile FAB -->
    <div class="md:hidden fixed bottom-6 right-6 z-50">
      <el-button type="primary" circle size="large" class="!w-12 !h-12 shadow-lg shadow-blue-300" @click="openAddDialog">
        <el-icon size="24"><Plus /></el-icon>
      </el-button>
    </div>

    <!-- Mobile Action Sheet -->
    <el-drawer
      v-model="actionSheetVisible"
      direction="btt"
      :with-header="false"
      size="auto"
      class="mobile-action-sheet rounded-t-2xl"
    >
      <div v-if="activeActionRow" class="p-4">
        <div class="text-center pb-3 border-b border-slate-100 mb-2">
          <div class="font-bold text-slate-800 text-base truncate">{{ activeActionRow.name }}</div>
          <div class="text-xs text-slate-400 mt-1">请选择操作</div>
        </div>
        <div class="grid grid-cols-4 gap-4 py-4">
          <div class="flex flex-col items-center gap-2" @click="handleMobileCommand('edit')">
            <div class="w-12 h-12 rounded-full bg-blue-50 text-blue-500 flex items-center justify-center"><el-icon size="24"><Edit /></el-icon></div>
            <span class="text-xs text-slate-600 font-medium">编辑</span>
          </div>
          <div class="flex flex-col items-center gap-2" @click="handleMobileCommand('transfer')">
            <div class="w-12 h-12 rounded-full bg-orange-50 text-orange-500 flex items-center justify-center"><el-icon size="24"><MagicStick /></el-icon></div>
            <span class="text-xs text-slate-600 font-medium">调拨</span>
          </div>
          <div class="flex flex-col items-center gap-2" @click="handleMobileCommand('stocktake')">
            <div class="w-12 h-12 rounded-full bg-cyan-50 text-cyan-500 flex items-center justify-center"><el-icon size="24"><Setting /></el-icon></div>
            <span class="text-xs text-slate-600 font-medium">盘库</span>
          </div>
          <div class="flex flex-col items-center gap-2" @click="handleMobileCommand('logs')">
            <div class="w-12 h-12 rounded-full bg-purple-50 text-purple-500 flex items-center justify-center"><el-icon size="24"><Search /></el-icon></div>
            <span class="text-xs text-slate-600 font-medium">流水</span>
          </div>
        </div>
        <el-button class="w-full mt-2 !rounded-xl !h-11" @click="actionSheetVisible = false">取消</el-button>
      </div>
    </el-drawer>

    <!-- Product Form -->
    <!-- PC View -->
    <el-dialog
      v-if="!isMobile"
      v-model="dialogVisible"
      :title="isEdit ? '编辑商品' : '新增商品'"
      width="500px"
      destroy-on-close
      class="custom-dialog"
    >
      <el-form ref="productFormRef" :model="productForm" :rules="formRules" label-position="top" class="product-form">
        <div class="form-section">
          <div class="form-section-title"><el-icon class="text-indigo-500"><Files /></el-icon>基本信息</div>
          <el-form-item label="商品名称" prop="name">
            <el-input v-model="productForm.name" placeholder="请输入商品完整名称" />
          </el-form-item>
          <el-form-item label="商品备注 (特征说明)" prop="remark">
            <el-input v-model="productForm.remark" placeholder="请输入商品特征或备注" />
          </el-form-item>
          <el-form-item label="商品分类" prop="category_id">
            <el-cascader
              v-model="productForm.category_id"
              :options="categoryOptions"
              :props="categoryCascaderProps"
              placeholder="选择大分类 / 二级分类"
              class="w-full"
              filterable
            />
          </el-form-item>
        </div>

        <div class="form-section">
          <div class="form-section-title"><el-icon class="text-indigo-500"><Coin /></el-icon>价格与提成</div>
          <el-form-item label="固定毛利率 (小数，如 0.15 代表 15%)" prop="margin_rate">
            <el-input-number v-model="productForm.margin_rate" :precision="2" :step="0.01" :min="0" :max="1" style="width: 100%" />
            <div class="input-tip">预览价格：¥{{ productForm.margin_rate < 1 ? formatMoney((productForm.avg_cost || 0) / (1 - productForm.margin_rate)) : '0.00' }}</div>
          </el-form-item>
          <el-form-item label="销售提成">
            <el-radio-group v-model="commissionOption">
              <el-radio value="normal">正常提成</el-radio>
              <el-radio value="none">无提成</el-radio>
              <el-radio value="custom">固定提成（每台）</el-radio>
            </el-radio-group>
            <template v-if="commissionOption === 'custom'">
              <el-form-item label="每台固定提成（元/台）" prop="commission_fixed_amount" class="w-full mt-2">
                <el-input-number v-model="productForm.commission_fixed_amount" :precision="2" :step="1" :min="0.01" style="width: 100%" />
                <div class="input-tip">每卖一台计固定提成，不受订单折扣影响；商会订单不使用商品固定提成。</div>
              </el-form-item>
            </template>
            <div v-else-if="commissionOption === 'none'" class="input-tip">开启后，新订单中该商品的成交金额不参与员工及商会订单提成。</div>
          </el-form-item>
        </div>

        <div v-if="isEdit" class="form-section bg-blue-50/50 border-blue-100">
          <div class="form-section-title text-blue-600"><el-icon><InfoFilled /></el-icon>成本校准</div>
          <el-form-item label="平均成本" prop="avg_cost">
            <el-input-number v-model="productForm.avg_cost" :precision="2" :min="0" style="width: 100%" />
            <p class="mt-1 text-xs text-slate-400">手动调整会记录成本基准；最新进价仍以实际入库流水为准。</p>
          </el-form-item>
        </div>

        <div class="form-section bg-orange-50/50 border-orange-100">
          <div class="form-section-title text-orange-600"><el-icon><Calendar /></el-icon>限时特价设置</div>
          <el-form-item label="特价金额" prop="special_price">
            <el-input-number v-model="productForm.special_price" :precision="2" :min="0" style="width: 100%" />
          </el-form-item>
          <el-form-item label="特价有效时间范围" prop="special_range">
            <el-date-picker v-model="productForm.special_range" type="datetimerange" range-separator="至" start-placeholder="开始时间" end-placeholder="结束时间" class="!w-full" value-format="YYYY-MM-DDTHH:mm:ss" />
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" :loading="submitting" @click="submitProduct">{{ isEdit ? '确认更新' : '确认创建' }}</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- Mobile View -->
    <el-drawer
      v-else
      v-model="dialogVisible"
      direction="btt"
      size="90%"
      :with-header="false"
      destroy-on-close
      class="mobile-action-sheet rounded-t-2xl flex flex-col"
    >
      <div class="px-4 py-3 border-b border-slate-100 sticky top-0 bg-white z-10 flex justify-between items-center">
        <div class="font-bold text-slate-800 text-lg">{{ isEdit ? '编辑商品' : '新增商品' }}</div>
        <el-button link @click="dialogVisible = false" class="!text-slate-400">关闭</el-button>
      </div>
      <div class="p-4 overflow-y-auto flex-1">
        <el-form ref="productFormRef" :model="productForm" :rules="formRules" label-position="top">
          <el-form-item label="商品名称" prop="name">
            <el-input v-model="productForm.name" size="large" placeholder="请输入商品完整名称" />
          </el-form-item>
          <el-form-item label="商品备注" prop="remark">
            <el-input v-model="productForm.remark" size="large" placeholder="请输入商品特征或备注" />
          </el-form-item>
          <el-form-item label="商品分类" prop="category_id">
            <el-cascader
              v-model="productForm.category_id"
              :options="categoryOptions"
              :props="categoryCascaderProps"
              size="large"
              placeholder="选择大分类 / 二级分类"
              class="w-full"
            />
          </el-form-item>
          <el-form-item label="固定毛利率" prop="margin_rate">
            <el-input-number v-model="productForm.margin_rate" size="large" :precision="2" :step="0.01" :min="0" :max="1" style="width: 100%" />
            <div class="mobile-price-preview">
              <span class="text-slate-400">预览价格</span>
              <span class="font-mono font-black text-blue-600">¥{{ previewPrice }}</span>
            </div>
          </el-form-item>
          <el-form-item label="销售提成">
            <el-radio-group v-model="commissionOption" class="flex flex-wrap">
              <el-radio value="normal">正常提成</el-radio>
              <el-radio value="none">无提成</el-radio>
              <el-radio value="custom">固定提成（每台）</el-radio>
            </el-radio-group>
            <template v-if="commissionOption === 'custom'">
              <el-form-item label="每台固定提成（元/台）" prop="commission_fixed_amount">
                <el-input-number v-model="productForm.commission_fixed_amount" size="large" :precision="2" :step="1" :min="0.01" style="width: 100%" />
                <div class="input-tip">每卖一台计固定提成，不受订单折扣影响；商会订单不使用商品固定提成。</div>
              </el-form-item>
            </template>
            <div v-else-if="commissionOption === 'none'" class="input-tip">开启后，新订单中该商品的成交金额不参与提成。</div>
          </el-form-item>

          <div v-if="isEdit" class="mt-4 p-4 bg-blue-50/50 rounded-2xl border border-blue-100">
            <div class="text-xs font-bold text-blue-600 mb-2">成本校准</div>
            <el-form-item label="平均成本" prop="avg_cost" class="mb-0">
              <el-input-number v-model="productForm.avg_cost" size="large" :precision="2" :min="0" style="width: 100%" />
              <p class="mt-1 text-xs text-slate-400">手动调整会记录成本基准；最新进价仍以实际入库流水为准。</p>
            </el-form-item>
          </div>

          <div class="mt-4 p-4 bg-orange-50/50 rounded-2xl border border-orange-100">
            <div class="text-xs font-bold text-orange-600 mb-2">限时特价设置</div>
            <el-form-item label="特价金额" prop="special_price">
              <el-input-number v-model="productForm.special_price" size="large" :precision="2" :min="0" style="width: 100%" />
            </el-form-item>
            <el-form-item label="特价时间范围">
              <div class="flex gap-2 w-full">
                <input type="datetime-local" inputmode="none" :value="mobileSpecialStart" @change="mobileSpecialStart = $event.target.value; syncMobileDates()" class="flex-1 h-10 px-2 border border-slate-300 rounded-lg text-sm bg-white text-slate-700 outline-none focus:border-blue-500" />
                <input type="datetime-local" inputmode="none" :value="mobileSpecialEnd" @change="mobileSpecialEnd = $event.target.value; syncMobileDates()" class="flex-1 h-10 px-2 border border-slate-300 rounded-lg text-sm bg-white text-slate-700 outline-none focus:border-blue-500" />
              </div>
            </el-form-item>
          </div>
        </el-form>
      </div>
      <div class="p-4 border-t border-slate-100 bg-white sticky bottom-0 z-10 safe-area-pb">
        <el-button type="primary" size="large" class="w-full !rounded-xl" :loading="submitting" @click="submitProduct">{{ isEdit ? '确认更新' : '确认创建' }}</el-button>
      </div>
    </el-drawer>

    <!-- 库存调拨 -->
    <el-dialog v-if="!isMobile" v-model="transferVisible" title="库存调拨" width="400px" destroy-on-close class="custom-dialog">
      <el-form :model="transferForm" label-position="top">
        <el-form-item label="调拨动作">
          <el-select v-model="transferForm.action" class="w-full">
            <el-option label="门店可用 调至 样机" value="store_to_sample" />
            <el-option label="样机 撤回至 门店可用" value="sample_to_store" />
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

    <el-drawer v-else v-model="transferVisible" direction="btt" size="auto" :with-header="false" destroy-on-close class="mobile-action-sheet rounded-t-2xl">
      <div class="px-4 py-3 border-b border-slate-100 flex justify-between items-center">
        <div class="font-bold text-slate-800 text-lg">库存调拨</div>
        <el-button link @click="transferVisible = false" class="!text-slate-400">关闭</el-button>
      </div>
      <div class="p-4 safe-area-pb">
        <div class="font-bold text-sm mb-2 text-slate-700">请选择调拨路线</div>
        <div class="grid grid-cols-1 gap-2 mb-4">
          <div v-for="(label, val) in {'store_to_sample': '门店可用 调至 样机', 'sample_to_store': '样机 撤回至 门店可用'}" :key="val"
               @click="transferForm.action = val"
               class="border rounded-xl p-3 text-center transition-colors font-bold text-sm cursor-pointer"
               :class="transferForm.action === val ? 'border-blue-500 bg-blue-50 text-blue-600' : 'border-slate-200 text-slate-600'">
            {{ label }}
          </div>
        </div>
        <div class="font-bold text-sm mb-2 text-slate-700">调拨数量</div>
        <el-input-number v-model="transferForm.quantity" size="large" :min="1" class="w-full mb-4 !text-lg" />
        <el-button type="primary" size="large" class="w-full !rounded-xl" :loading="transferring" @click="submitTransfer">确认调拨</el-button>
      </div>
    </el-drawer>

    <!-- 盘点清查 -->
    <el-dialog v-if="!isMobile" v-model="stocktakeVisible" title="盘点清查" width="440px" destroy-on-close class="custom-dialog">
      <el-form :model="stocktakeForm" label-position="top">
        <el-form-item label="盘点仓库">
            <el-radio-group v-model="stocktakeForm.warehouse_type" @change="syncStocktakeExpectedStock">
            <el-radio :value="2">门店</el-radio>
            <el-radio :value="3">云仓</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="实际清点数量">
          <el-input-number v-model="stocktakeForm.actual_stock" :min="0" class="w-full" />
        </el-form-item>
        <el-form-item label="盘点性质">
          <el-radio-group v-model="stocktakeForm.stocktake_type">
            <el-radio :value="1"><span class="font-medium">正常损耗</span></el-radio>
            <el-radio :value="2"><span class="font-medium">业务调整</span></el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="备注说明">
          <el-input v-model="stocktakeForm.remark" placeholder="数量与账面不一致时必填差异原因" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="stocktakeVisible = false">取消</el-button>
          <el-button type="primary" :loading="stocktaking" @click="submitStocktake">确认盘点</el-button>
        </div>
      </template>
    </el-dialog>

    <el-drawer v-else v-model="stocktakeVisible" direction="btt" size="auto" :with-header="false" destroy-on-close class="mobile-action-sheet rounded-t-2xl">
      <div class="px-4 py-3 border-b border-slate-100 flex justify-between items-center">
        <div class="font-bold text-slate-800 text-lg">盘点清查</div>
        <el-button link @click="stocktakeVisible = false" class="!text-slate-400">关闭</el-button>
      </div>
      <div class="p-4 safe-area-pb space-y-4">
        <div>
          <div class="font-bold text-sm mb-2 text-slate-700">盘点仓库</div>
          <el-radio-group v-model="stocktakeForm.warehouse_type" class="w-full flex gap-2" size="large" @change="syncStocktakeExpectedStock">
            <el-radio-button :value="2" class="flex-1 text-center">门店</el-radio-button>
            <el-radio-button :value="3" class="flex-1 text-center">云仓</el-radio-button>
          </el-radio-group>
        </div>
        <div>
          <div class="font-bold text-sm mb-2 text-slate-700">实际清点数量</div>
          <el-input-number v-model="stocktakeForm.actual_stock" size="large" :min="0" class="w-full !text-lg !font-bold" />
        </div>
        <div>
          <div class="font-bold text-sm mb-2 text-slate-700">盘点性质</div>
          <div class="space-y-2">
            <div @click="stocktakeForm.stocktake_type = 1" class="border rounded-xl p-3 transition-colors cursor-pointer" :class="stocktakeForm.stocktake_type === 1 ? 'border-blue-500 bg-blue-50' : 'border-slate-200 bg-white'">
              <div class="font-bold text-sm" :class="stocktakeForm.stocktake_type === 1 ? 'text-blue-700' : 'text-slate-700'">正常损耗</div>
              <div class="text-xs mt-1" :class="stocktakeForm.stocktake_type === 1 ? 'text-blue-500' : 'text-slate-500'">差额计入财务亏损</div>
            </div>
            <div @click="stocktakeForm.stocktake_type = 2" class="border rounded-xl p-3 transition-colors cursor-pointer" :class="stocktakeForm.stocktake_type === 2 ? 'border-blue-500 bg-blue-50' : 'border-slate-200 bg-white'">
              <div class="font-bold text-sm" :class="stocktakeForm.stocktake_type === 2 ? 'text-blue-700' : 'text-slate-700'">业务调整</div>
              <div class="text-xs mt-1" :class="stocktakeForm.stocktake_type === 2 ? 'text-blue-500' : 'text-slate-500'">仅修正库存数量</div>
            </div>
          </div>
        </div>
        <div>
          <div class="font-bold text-sm mb-2 text-slate-700">备注说明</div>
          <el-input v-model="stocktakeForm.remark" size="large" placeholder="数量与账面不一致时必填差异原因" />
        </div>
        <el-button type="primary" size="large" class="w-full !rounded-xl mt-2" :loading="stocktaking" @click="submitStocktake">确认盘点</el-button>
      </div>
    </el-drawer>

        <!-- 库存流水 -->
    <el-dialog v-if="!isMobile" v-model="logsVisible" title="库存操作流水" width="800px" destroy-on-close class="custom-dialog">
      <div class="mb-4 flex items-center justify-between">
        <el-radio-group v-model="logWarehouseType" size="small" @change="fetchLogs">
          <el-radio-button value="">全部仓库</el-radio-button>
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
            {{ formatDateTime(row.created_at) }}
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
        <el-table-column label="单价/进价" width="100" align="right">
          <template #default="{ row }">
            <span v-if="Number(row.unit_cost) > 0" class="font-mono text-xs text-slate-600">¥{{ formatMoney(row.unit_cost) }}</span>
            <span v-else class="text-xs font-bold text-amber-600">成本待核</span>
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

    <el-drawer v-else v-model="logsVisible" direction="btt" size="90%" :with-header="false" destroy-on-close class="mobile-action-sheet rounded-t-2xl flex flex-col">
      <div class="px-4 py-3 border-b border-slate-100 flex justify-between items-center sticky top-0 bg-white z-10">
        <div class="font-bold text-slate-800 text-lg">操作流水</div>
        <el-button link @click="logsVisible = false" class="!text-slate-400">关闭</el-button>
      </div>
      <div class="p-4 flex-1 overflow-y-auto bg-slate-50">
        <div class="mb-3 overflow-x-auto hide-scrollbar pb-1">
          <div class="mb-4 flex items-center justify-between">
        <el-radio-group class="flex whitespace-nowrap" v-model="logWarehouseType" size="small" @change="fetchLogs">
          <el-radio-button value="">全部仓库</el-radio-button>
          <el-radio-button :value="1">主仓</el-radio-button>
          <el-radio-button :value="2">门店</el-radio-button>
          <el-radio-button :value="3">云仓</el-radio-button>
          <el-radio-button :value="4">样机</el-radio-button>
        </el-radio-group>
      </div>
        </div>
        <div v-if="currentLogs.length === 0" class="py-10 text-center text-slate-400">暂无流水记录</div>
        <div v-else class="space-y-3 mt-2">
          <div v-for="row in currentLogs" :key="row.id" class="bg-white rounded-xl p-3 border border-slate-100 shadow-sm relative">
            <div class="flex justify-between items-center mb-2">
              <span class="text-xs font-medium text-slate-500">{{ formatDateTime(row.created_at) }}</span>
              <el-tag size="small" :type="getLogTypeTag(row.log_type).type" effect="dark" class="!rounded-md">{{ getLogTypeTag(row.log_type).label }}</el-tag>
            </div>
            <div class="flex justify-between items-center bg-slate-50 rounded-lg p-2.5 mb-2">
              <div class="text-xs text-slate-500">
                <div class="mb-1 font-bold text-slate-700">{{ getWarehouseName(row.warehouse_type) }}</div>
                <div>{{ row.before_qty }} <span class="mx-1 text-slate-300">&rarr;</span> <span class="font-bold text-slate-700">{{ row.after_qty }}</span></div>
              </div>
              <div class="text-right">
                <div class="font-bold text-lg" :class="row.change_qty > 0 ? 'text-blue-600' : 'text-red-500'">
                  {{ row.change_qty > 0 ? '+' : '' }}{{ row.change_qty }}
                </div>
                <div v-if="Number(row.unit_cost) > 0" class="font-mono text-[10px] text-slate-400 mt-1">¥{{ formatMoney(row.unit_cost) }}</div>
                <div v-else class="text-[10px] text-amber-600 font-bold mt-1">成本待核</div>
              </div>
            </div>
            <div v-if="row.remark || row.related_no" class="text-[11px] text-slate-500 mt-2 border-t border-slate-100 pt-2">
              <div v-if="row.related_no" class="truncate"><span class="text-slate-400">单号:</span> {{ row.related_no }}</div>
              <div v-if="row.remark" class="truncate mt-0.5"><span class="text-slate-400">备注:</span> {{ row.remark }}</div>
            </div>
          </div>
        </div>

        
      </div>
    </el-drawer>

        <!-- 分类管理 -->
    <el-dialog v-if="!isMobile" v-model="categoryDialogVisible" title="分类管理" width="560px" destroy-on-close class="custom-dialog">
      <div class="mb-3 p-3 rounded-xl bg-slate-50 border border-slate-100">
        <div class="text-xs font-bold text-slate-500 mb-2">新增分类</div>
        <div class="flex gap-2">
          <el-select v-model="newCategoryParentId" class="!w-40" placeholder="作为一级分类">
            <el-option label="作为一级大分类" value="" />
            <el-option v-for="root in categoryTree" :key="root.id" :label="`挂在「${root.name}」下`" :value="root.id" />
          </el-select>
          <el-input v-model="newCategoryName" placeholder="新分类名称" class="flex-1" @keyup.enter="handleAddCategory" />
          <el-button type="primary" @click="handleAddCategory">添加</el-button>
        </div>
      </div>
      <el-table :data="categoryTree" row-key="id" border default-expand-all :tree-props="{ children: 'children' }" size="small" height="320">
        <el-table-column prop="name" label="分类名称" min-width="200">
          <template #default="{ row }">
            <el-input v-if="row.isEditing" v-model="row.editName" size="small" />
            <span v-else :class="{ 'font-bold': !row.parent_id }">{{ row.name }}<el-tag v-if="row.parent_id" size="small" type="info" class="ml-1.5">二级</el-tag></span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="170" align="center">
          <template #default="{ row }">
            <template v-if="row.isEditing">
              <el-button link type="primary" @click="saveCategoryEdit(row)">保存</el-button>
              <el-button link @click="row.isEditing = false">取消</el-button>
            </template>
            <template v-else>
              <el-button v-if="!row.parent_id" link type="success" @click="newCategoryParentId = row.id; newCategoryName = ''; ">加子分类</el-button>
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

    <el-drawer v-else v-model="categoryDialogVisible" direction="btt" size="90%" :with-header="false" destroy-on-close class="mobile-action-sheet rounded-t-2xl flex flex-col">
      <div class="px-4 py-3 border-b border-slate-100 flex justify-between items-center sticky top-0 bg-white z-10">
        <div class="font-bold text-slate-800 text-lg">分类管理</div>
        <el-button link @click="categoryDialogVisible = false" class="!text-slate-400">关闭</el-button>
      </div>
      <div class="p-4 flex-1 overflow-y-auto safe-area-pb bg-slate-50">
        <div class="mb-4 p-3 rounded-xl bg-white border border-slate-100">
          <div class="text-xs font-bold text-slate-500 mb-2">新增分类</div>
          <el-select v-model="newCategoryParentId" class="w-full mb-2" placeholder="作为一级分类">
            <el-option label="作为一级大分类" value="" />
            <el-option v-for="root in categoryTree" :key="root.id" :label="`挂在「${root.name}」下`" :value="root.id" />
          </el-select>
          <div class="flex gap-2">
            <el-input v-model="newCategoryName" placeholder="新分类名称" class="flex-1 h-10" />
            <el-button type="primary" class="h-10 px-6 !rounded-lg" @click="handleAddCategory">添加</el-button>
          </div>
        </div>
        <div v-if="categoryTree.length === 0" class="py-10 text-center text-slate-400">暂无分类</div>
        <div v-else class="space-y-3 mt-2">
          <div v-for="root in categoryTree" :key="root.id" class="bg-white rounded-xl border border-slate-100 shadow-sm overflow-hidden">
            <div class="p-3 flex items-center justify-between bg-slate-50/60">
              <template v-if="root.isEditing">
                <div class="flex-1 flex gap-2 pr-2">
                  <el-input v-model="root.editName" size="small" class="flex-1" />
                  <el-button type="primary" size="small" @click="saveCategoryEdit(root)">保存</el-button>
                  <el-button size="small" @click="root.isEditing = false">取消</el-button>
                </div>
              </template>
              <template v-else>
                <div class="font-bold text-slate-800 text-sm">{{ root.name }}</div>
                <div class="flex gap-1">
                  <el-button link type="success" size="small" @click="newCategoryParentId = root.id; newCategoryName = ''">+子分类</el-button>
                  <el-button link type="primary" size="small" @click="startCategoryEdit(root)">编辑</el-button>
                  <el-popconfirm title="确定删除吗？" @confirm="handleDeleteCategory(root.id)">
                    <template #reference>
                      <el-button link type="danger" size="small">删除</el-button>
                    </template>
                  </el-popconfirm>
                </div>
              </template>
            </div>
            <div v-if="root.children?.length" class="divide-y divide-slate-50">
              <div v-for="child in root.children" :key="child.id" class="px-3 py-2.5 flex items-center justify-between pl-7">
                <template v-if="child.isEditing">
                  <div class="flex-1 flex gap-2 pr-2">
                    <el-input v-model="child.editName" size="small" class="flex-1" />
                    <el-button type="primary" size="small" @click="saveCategoryEdit(child)">保存</el-button>
                    <el-button size="small" @click="child.isEditing = false">取消</el-button>
                  </div>
                </template>
                <template v-else>
                  <div class="text-sm text-slate-600 flex items-center gap-1.5"><span class="text-slate-300">└</span>{{ child.name }}</div>
                  <div class="flex gap-1">
                    <el-button link type="primary" size="small" @click="startCategoryEdit(child)">编辑</el-button>
                    <el-popconfirm title="确定删除吗？" @confirm="handleDeleteCategory(child.id)">
                      <template #reference>
                        <el-button link type="danger" size="small">删除</el-button>
                      </template>
                    </el-popconfirm>
                  </div>
                </template>
              </div>
            </div>
            <div v-else class="px-7 pb-2.5 text-[11px] text-slate-300">暂无子分类</div>
          </div>
        </div>


      </div>
    </el-drawer>

  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, reactive, computed } from 'vue'
import { Plus, Search, ArrowDown, Calendar, Lightning, Setting, MagicStick, InfoFilled, Download, MoreFilled, Edit, Box, Menu, Files, Coin } from '@element-plus/icons-vue'
import {
  fetchCategories as fetchCategoriesApi,
  fetchProducts as fetchProductsApi,
  createCategory,
  updateCategory,
  deleteCategory,
  createProduct,
  updateProduct,
  fetchProductLogs,
  transferProduct,
  stocktakeProduct,
  recalculateProductCosts
} from '../api/products'
import { formatMoney, formatDateTime } from '../utils/format'
import { downloadExcel } from '../utils/export'
import { buildCategoryTree, categoryFullPath, categoryCascaderProps } from '../utils/categories'
import { ElMessage, ElMessageBox } from 'element-plus'

const windowWidth = ref(window.innerWidth)
const isMobile = computed(() => windowWidth.value <= 768)

const handleResize = () => { windowWidth.value = window.innerWidth }
onMounted(() => { window.addEventListener('resize', handleResize) })
onUnmounted(() => { window.removeEventListener('resize', handleResize) })

const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const productList = ref([])
const categories = ref([])
const selectedCategoryId = ref('')

// 两级分类：树（级联/管理用）与完整路径显示
const categoryTree = computed(() => buildCategoryTree(categories.value))
const categoryLabel = (id) => categoryFullPath(categories.value, id)
// 级联选项：无子分类的一级不给空 children，避免出现可展开的空面板
const categoryOptions = computed(() => categoryTree.value.map(root => ({
  ...root,
  children: root.children.length ? root.children : undefined
})))
const onlySpecial = ref(false)
const onlyInStock = ref(false)
const searchQuery = ref('')
const productFormRef = ref(null)

const mobileSpecialStart = ref('')
const mobileSpecialEnd = ref('')

const formatToBackend = (val) => {
  if (!val) return ''
  return val.length === 16 ? val + ':00' : val
}

const syncMobileDates = () => {
  if (mobileSpecialStart.value && mobileSpecialEnd.value) {
    productForm.special_range = [
      formatToBackend(mobileSpecialStart.value), 
      formatToBackend(mobileSpecialEnd.value)
    ]
  } else {
    productForm.special_range = []
  }
}

// Action Sheet & Mobile Logic
const actionSheetVisible = ref(false)
const activeActionRow = ref(null)

const handleMobileActions = (row) => {
  activeActionRow.value = row
  actionSheetVisible.value = true
}

const handleMobileCommand = (cmd) => {
  if (!activeActionRow.value) return
  if (cmd === 'edit') {
    handleEdit(activeActionRow.value)
  } else {
    handleCommand(cmd, activeActionRow.value)
  }
  actionSheetVisible.value = false
}

const handleMobileMenuCommand = (cmd) => {
  if (cmd === 'recalculate') handleRecalculateCosts()
  if (cmd === 'stockFilter') toggleInStock()
  if (cmd === 'category') openCategoryManage()
  if (cmd === 'export') handleExport()
}

const handleDesktopMenuCommand = (cmd) => {
  if (cmd === 'recalculate') handleRecalculateCosts()
  if (cmd === 'stockFilter') toggleInStock()
  if (cmd === 'category') openCategoryManage()
  if (cmd === 'export') handleExport()
}

const loadMore = () => {
  if (productList.value.length >= totalProducts.value) return
  pageSize.value += 15
  fetchProducts()
}

// Pagination Setup
const currentPage = ref(1)
const pageSize = ref(15)
const totalProducts = ref(0)
const paginatedProductList = computed(() => {
  return productList.value
})

const isEdit = ref(false)
const currentEditId = ref(null)

const productForm = reactive({
  name: '',
  remark: '',
  category_id: null,
  margin_rate: 0.15,
  no_commission: false,
  commission_fixed_amount: 0,
  special_price: 0,
  special_range: [],
  avg_cost: 0
})

// 销售提成三选一：normal=跟随员工比例 / none=无提成 / custom=每台固定提成
const commissionOption = ref('normal')

const previewPrice = computed(() => {
  if (productForm.margin_rate >= 1) return '0.00'
  return ((productForm.avg_cost || 0) / (1 - productForm.margin_rate)).toFixed(2)
})

const formRules = {
  name: [{ required: true, message: '请输入商品名称', trigger: 'blur' }],
  remark: [{ required: false, message: '请输入备注', trigger: 'blur' }],
  category_id: [{ required: true, message: '请选择商品分类', trigger: 'change' }],
  margin_rate: [{ required: true, message: '请设置毛利率', trigger: 'blur' }],
  commission_fixed_amount: [{
    validator: (rule, value, callback) => {
      if (commissionOption.value === 'custom' && !(value > 0)) {
        callback(new Error('请设置每台固定提成金额'))
      } else {
        callback()
      }
    },
    trigger: 'blur'
  }]
}

const fetchCategories = async () => {
  try {
    const res = await fetchCategoriesApi()
    if (res.data.code === 200) {
      categories.value = res.data.data.map(c => ({ ...c, isEditing: false, editName: c.name }))
    }
  } catch (error) {
    console.error('Fetch categories error:', error)
  }
}

const handleExport = () => {
  downloadExcel('/api/products/export', {
    keyword: searchQuery.value,
    category_id: selectedCategoryId.value,
    only_special: onlySpecial.value ? 1 : 0,
    all: 1
  })
}

const fetchProducts = async () => {
  loading.value = true
  try {
    const res = await fetchProductsApi({
      keyword: searchQuery.value,
      category_id: selectedCategoryId.value,
      only_special: onlySpecial.value ? 1 : 0,
      stock_filter: onlyInStock.value ? 'available' : '',
      all: 1,
      page: currentPage.value,
      page_size: pageSize.value
    })
    if (res.data.code === 200) {
      productList.value = res.data.data
      totalProducts.value = res.data.total || 0
    }
  } catch (error) {
    ElMessage.error('获取商品列表失败')
  } finally {
    loading.value = false
  }
}

const handleSizeChange = (val) => {
  pageSize.value = val
  currentPage.value = 1
  fetchProducts()
}

// 分类管理相关（两级：大分类 / 二级分类）
const categoryDialogVisible = ref(false)
const newCategoryName = ref('')
const newCategoryParentId = ref('') // 空=作为一级大分类
const openCategoryManage = () => {
  newCategoryParentId.value = ''
  categoryDialogVisible.value = true
}

const handleAddCategory = async () => {
  if (!newCategoryName.value) return
  try {
    const res = await createCategory({
      name: newCategoryName.value,
      parent_id: newCategoryParentId.value || null
    })
    if (res.data.code === 200) {
      ElMessage.success('添加成功')
      newCategoryName.value = ''
      fetchCategories()
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '添加失败')
  }
}

const startCategoryEdit = (row) => {
  row.isEditing = true
  row.editName = row.name
}

const saveCategoryEdit = async (row) => {
  try {
    // parent_id 必须显式回传，否则后端会把二级分类提升为一级
    const res = await updateCategory(row.id, { name: row.editName, parent_id: row.parent_id || null })
    if (res.data.code === 200) {
      ElMessage.success('修改成功')
      fetchCategories()
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '修改失败')
  }
}

const handleDeleteCategory = async (id) => {
  try {
    const res = await deleteCategory(id)
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
  currentPage.value = 1
  fetchProducts()
}

const handleRecalculateCosts = async () => {
  try {
    await ElMessageBox.confirm(
      '此操作将按成本基准重放采购、销售、盘点和撤销流水。遇到零成本入库或库存数量不一致时，该商品会跳过并说明原因，避免写入不可信成本。是否继续？',
      '重算商品成本',
      {
        confirmButtonText: '立即重构',
        cancelButtonText: '取消',
        type: 'warning',
        icon: MagicStick
      }
    )
    
    loading.value = true
    const res = await recalculateProductCosts()
    if (res.data.code === 200) {
      ElMessage.success(res.data.msg)
      fetchProducts()
    }
  } catch (error) {
    if (error !== 'cancel') {
      const responseData = error.response?.data
      const failed = responseData?.data?.failed || []
      const failedSummary = failed.slice(0, 3).map(item => `商品#${item.product_id}: ${item.error}`).join('；')
      const message = [responseData?.msg || error.message, failedSummary].filter(Boolean).join('；')
      ElMessage.warning(message || '成本重算失败')
      fetchProducts()
    }
  } finally {
    loading.value = false
  }
}

const selectCategory = (id) => {
  // cascader 清空时回调 null，统一归一为 ''（全部分类）
  selectedCategoryId.value = id ?? ''
  currentPage.value = 1
  fetchProducts()
}

const toggleSpecial = () => {
  onlySpecial.value = !onlySpecial.value
  currentPage.value = 1
  fetchProducts()
}

const toggleInStock = () => {
  onlyInStock.value = !onlyInStock.value
  currentPage.value = 1
  fetchProducts()
}

const openAddDialog = () => {
  isEdit.value = false
  currentEditId.value = null
  productForm.name = ''
  productForm.remark = ''
  productForm.category_id = null
  productForm.margin_rate = 0.15
  productForm.no_commission = false
  productForm.commission_fixed_amount = 0
  commissionOption.value = 'normal'
  productForm.special_price = 0
  productForm.special_range = []
  productForm.avg_cost = 0
  dialogVisible.value = true
}

const handleEdit = (row) => {
  isEdit.value = true
  currentEditId.value = row.id
  productForm.name = row.name
  productForm.remark = row.remark
  productForm.category_id = row.category_id
  productForm.margin_rate = row.margin_rate
  productForm.avg_cost = Number(row.avg_cost) || 0
  productForm.no_commission = row.no_commission || false
  // 老数据无该字段时回退 0（0=跟随员工比例），防止编辑一次丢配置
  productForm.commission_fixed_amount = row.commission_fixed_amount || 0
  commissionOption.value = row.no_commission ? 'none' : (productForm.commission_fixed_amount > 0 ? 'custom' : 'normal')
  productForm.special_price = row.special_price || 0
  if (row.special_price_start && row.special_price_end) {
    productForm.special_range = [row.special_price_start, row.special_price_end]
    mobileSpecialStart.value = row.special_price_start ? row.special_price_start.substring(0, 16) : ''
    mobileSpecialEnd.value = row.special_price_end ? row.special_price_end.substring(0, 16) : ''
  } else {
    productForm.special_range = []
    mobileSpecialStart.value = ''
    mobileSpecialEnd.value = ''
  }
  dialogVisible.value = true
}

// 调拨与盘库状态
const transferVisible = ref(false)
const transferring = ref(false)
const transferForm = reactive({
  action: 'store_to_sample',
  quantity: 1,
  productId: null
})

const stocktakeVisible = ref(false)
const stocktaking = ref(false)
const stocktakeForm = reactive({
  warehouse_type: 3,
  actual_stock: 0,
  expected_stock: 0,
  stocktake_type: 1,
  remark: '',
  productId: null
})

const handleCommand = (command, row) => {
  if (command === 'transfer') {
    transferForm.productId = row.id
    transferForm.quantity = 1
    transferForm.action = 'store_to_sample'
    transferVisible.value = true
  } else if (command === 'stocktake') {
    stocktakeForm.productId = row.id
    stocktakeForm.warehouse_type = 3
    stocktakeForm.actual_stock = row.cloud_stock || 0
    stocktakeForm.expected_stock = row.cloud_stock || 0
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

const syncStocktakeExpectedStock = () => {
  const product = productList.value.find(item => item.id === stocktakeForm.productId)
  if (!product) return
  const stock = Number(stocktakeForm.warehouse_type === 2 ? product.store_stock : product.cloud_stock) || 0
  stocktakeForm.expected_stock = stock
  stocktakeForm.actual_stock = stock
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
    const res = await fetchProductLogs(currentLogProductId.value, {
      warehouse_type: logWarehouseType.value
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
    const res = await transferProduct(transferForm.productId, {
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
  if (Number(stocktakeForm.actual_stock) !== Number(stocktakeForm.expected_stock) && !stocktakeForm.remark.trim()) {
    ElMessage.warning('盘点数量有差异时请填写差异原因')
    return
  }
  stocktaking.value = true
  try {
    const res = await stocktakeProduct(stocktakeForm.productId, {
      warehouse_type: stocktakeForm.warehouse_type,
      actual_stock: stocktakeForm.actual_stock,
      expected_stock: stocktakeForm.expected_stock,
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
        // 三选一状态落回后端字段：固定提成与无提成互斥，非固定提成时金额归零
        if (commissionOption.value === 'custom') {
          productForm.no_commission = false
        } else {
          productForm.no_commission = commissionOption.value === 'none'
          productForm.commission_fixed_amount = 0
        }
        const submitData = { ...productForm }
        if (productForm.special_range && productForm.special_range.length === 2) {
          submitData.special_price_start = productForm.special_range[0]
          submitData.special_price_end = productForm.special_range[1]
        } else {
          submitData.special_price_start = null
          submitData.special_price_end = null
        }
        delete submitData.special_range

        let res
        if (isEdit.value) {
          res = await updateProduct(currentEditId.value, submitData)
        } else {
          res = await createProduct(submitData)
        }
        
        if (res.data.code === 200) {
          ElMessage.success(isEdit.value ? '商品更新成功' : '商品创建成功')
          dialogVisible.value = false
          fetchProducts()
        } else {
          ElMessage.error(res.data.msg || '操作失败')
        }
      } catch (error) {
        ElMessage.error(error.response?.data?.msg || '服务器响应错误')
      } finally {
        submitting.value = false
      }
    }
  })
}

// 列表角标文案：无提成 / 提成¥N/台，两者互斥
const commissionBadge = (row) => {
  if (row.no_commission) return '无提成'
  const fixed = Number(row.commission_fixed_amount) || 0
  if (fixed > 0) return `提成¥${fixed}/台`
  return ''
}

const isSpecialActive = (product) => {
  if (!product.special_price || product.special_price <= 0) return false
  if (!product.special_price_start || !product.special_price_end) return false
  const now = new Date()
  const start = new Date(product.special_price_start)
  const end = new Date(product.special_price_end)
  return now >= start && now <= end
}

onMounted(() => {
  fetchCategories()
  fetchProducts()
})
</script>

<style scoped>
.compact-cell { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
.compact-primary { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #1e293b; font-size: 13px; }
.compact-meta { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #94a3b8; font-size: 11px; }
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

.form-section {
  padding: 14px 16px 4px;
  border-radius: 14px;
  border: 1px solid #f1f5f9;
  background: #fafbfc;
  margin-bottom: 14px;
}

.form-section.bg-blue-50\/50 {
  background: rgba(239, 246, 255, 0.5);
}
.form-section.bg-orange-50\/50 {
  background: rgba(255, 247, 237, 0.5);
}

.form-section-title {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  font-weight: 700;
  color: #475569;
  margin-bottom: 12px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.input-tip {
  font-size: 12px;
  color: #94a3b8;
  margin-top: 4px;
}

.mobile-price-preview {
  width: 100%;
  margin-top: 10px;
  padding: 10px 12px;
  border-radius: 14px;
  background: linear-gradient(135deg, #eef0ff 0%, #ffffff 100%);
  border: 1px solid #e8e9ff;
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
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

/* New Modern UI Styles */
.custom-search-input :deep(.el-input__wrapper) {
  border-radius: 14px;
  box-shadow: 0 2px 6px rgba(0,0,0,0.02) inset;
  background-color: #f8fafc;
  border: 1px solid #e2e8f0;
  padding: 4px 12px;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.custom-search-input :deep(.el-input__wrapper.is-focus) {
  background-color: #fff;
  border-color: #635bff;
  box-shadow: 0 0 0 4px rgba(79, 70, 229, 0.1);
}

/* 分类下拉 */
.category-select :deep(.el-select__wrapper),
.mobile-category-select :deep(.el-select__wrapper) {
  border-radius: 12px;
  background-color: #f8fafc;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.03) inset, 0 0 0 1px #e2e8f0 inset;
  font-weight: 600;
}
.category-select :deep(.el-select__wrapper.is-focused),
.mobile-category-select :deep(.el-select__wrapper.is-focused) {
  background-color: #fff;
  box-shadow: 0 0 0 1px #635bff inset, 0 0 0 4px rgba(79, 70, 229, 0.1);
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

/* Premium Pagination Styles */
.custom-pagination :deep(.el-pager li) {
  border-radius: 8px;
  font-weight: 700;
  border: 1px solid #f1f5f9;
  background-color: #ffffff;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  color: #64748b;
  font-family: 'Inter', system-ui;
}
.custom-pagination :deep(.el-pager li:hover) {
  color: #635bff;
  border-color: #635bff;
}
.custom-pagination :deep(.el-pager li.is-active) {
  background-color: #635bff !important;
  color: #ffffff !important;
  border-color: #635bff;
  box-shadow: 0 4px 10px rgba(79, 70, 229, 0.2);
}
.custom-pagination :deep(.btn-prev),
.custom-pagination :deep(.btn-next) {
  border-radius: 8px;
  border: 1px solid #f1f5f9;
  background-color: #ffffff !important;
  color: #64748b;
}
.custom-pagination :deep(.btn-prev:disabled),
.custom-pagination :deep(.btn-next:disabled) {
  border-color: #f1f5f9;
  color: #cbd5e1;
}

/* Mobile Enhancements */
.hide-scrollbar::-webkit-scrollbar {
  display: none;
}
.hide-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}

:deep(.mobile-action-sheet) {
  border-radius: 20px 20px 0 0 !important;
}

@media (max-width: 768px) {
  .product-manage-container {
    padding: 12px;
  }
  :deep(.responsive-dialog) {
    display: flex;
    align-items: flex-end;
    justify-content: center;
  }
  :deep(.responsive-dialog .el-dialog) {
    width: 100% !important;
    border-radius: 20px 20px 0 0 !important;
    margin: 0 !important;
    max-height: 90vh;
    display: flex;
    flex-direction: column;
  }
  :deep(.responsive-dialog .el-dialog__body) {
    overflow-y: auto;
    padding: 16px;
    padding-bottom: env(safe-area-inset-bottom, 24px);
  }
  :deep(.responsive-dialog .el-dialog__footer) {
    padding-bottom: calc(16px + env(safe-area-inset-bottom, 16px));
    position: sticky;
    bottom: 0;
    background: #ffffff;
    z-index: 10;
    border-top: 1px solid #f1f5f9;
  }
  :deep(.responsive-dialog .el-dialog__header) {
    position: sticky;
    top: 0;
    background: #ffffff;
    z-index: 10;
  }
  :deep(.responsive-dialog .el-dialog__body) {
    padding-bottom: 30px;
  }
}
</style>
