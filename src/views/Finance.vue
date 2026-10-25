<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Management, Plus, Switch, TrendCharts } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import FinanceWorkspace from '../components/finance/FinanceWorkspace.vue'
import { useFinanceWorkspace } from '../composables/useFinanceWorkspace'
import request from '../utils/request'

const period = ref('month')
const accountDialogVisible = ref(false)
const transferDialogVisible = ref(false)
const calibrateDialogVisible = ref(false)
const submitting = ref(false)
const accountFormRef = ref()
const transferFormRef = ref()
const { summary, loading, totalBalance, netCashFlow, attentionCount, loadWorkspace } = useFinanceWorkspace()

// ---------- 真实净利润 ----------
const currentMonth = () => {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}
const realProfitMonth = ref(currentMonth())
const realProfit = ref(null)
const realProfitLoading = ref(false)
const realProfitDrawerVisible = ref(false)

const fetchRealProfit = async () => {
  realProfitLoading.value = true
  try {
    const res = await request.get('/api/finance/real-profit', { params: { month: realProfitMonth.value } })
    if (res.data.code === 200) {
      realProfit.value = res.data.data
    }
  } catch {
    realProfit.value = null
  } finally {
    realProfitLoading.value = false
  }
}

const realProfitTone = computed(() => {
  const value = Number(realProfit.value?.real_profit || 0)
  if (value > 0) return 'profit-positive'
  if (value < 0) return 'profit-negative'
  return 'profit-zero'
})
const money = value => `¥${Number(value || 0).toLocaleString({ minimumFractionDigits: 2, maximumFractionDigits: 2 })}`

const periodLabel = computed(() => ({ today: '今日', month: '本月', all: '累计' })[period.value] || '本期')
const accounts = computed(() => summary.value.accounts || [])
const accountForm = reactive({ name: '', balance: 0 })
const transferForm = reactive({ from_account_id: null, to_account_id: null, amount: 0, remark: '' })
const calibrateForm = reactive({ accountId: null, accountName: '', currentBalance: 0, realBalance: 0, remark: '' })
const calibrateDiff = computed(() => calibrateForm.realBalance - calibrateForm.currentBalance)

async function refreshAll() { try { await loadWorkspace(period.value) } catch { ElMessage.error('财务数据加载失败，请稍后重试') } }
function openCalibrate(account) { Object.assign(calibrateForm, { accountId: account.id, accountName: account.name, currentBalance: Number(account.balance || 0), realBalance: Number(account.balance || 0), remark: '' }); calibrateDialogVisible.value = true }
async function createAccount() {
  if (!accountFormRef.value || !await accountFormRef.value.validate().catch(() => false)) return
  submitting.value = true
  try { const response = await request.post('/api/accounts', accountForm); if (response.data.code !== 200) return ElMessage.error(response.data.msg || '创建失败'); ElMessage.success('账户已创建'); accountDialogVisible.value = false; Object.assign(accountForm, { name: '', balance: 0 }); await refreshAll() } catch (error) { ElMessage.error(error.response?.data?.msg || '创建失败') } finally { submitting.value = false }
}
async function transferFunds() {
  if (!transferFormRef.value || !await transferFormRef.value.validate().catch(() => false)) return
  if (transferForm.from_account_id === transferForm.to_account_id) return ElMessage.warning('转出和转入账户不能相同')
  submitting.value = true
  try { const response = await request.post('/api/accounts/transfer', transferForm); if (response.data.code !== 200) return ElMessage.error(response.data.msg || '划拨失败'); ElMessage.success('资金划拨成功'); transferDialogVisible.value = false; Object.assign(transferForm, { from_account_id: null, to_account_id: null, amount: 0, remark: '' }); await refreshAll() } catch (error) { ElMessage.error(error.response?.data?.msg || '划拨失败') } finally { submitting.value = false }
}
async function calibrateAccount() {
  if (!calibrateDiff.value) return
  submitting.value = true
  try { const response = await request.put(`/api/accounts/${calibrateForm.accountId}/calibrate`, { real_balance: calibrateForm.realBalance, remark: calibrateForm.remark }); if (response.data.code !== 200) return ElMessage.error(response.data.msg || '校准失败'); ElMessage.success('账户余额已校准，并保留审计记录'); calibrateDialogVisible.value = false; await refreshAll() } catch (error) { ElMessage.error(error.response?.data?.msg || '校准失败') } finally { submitting.value = false }
}
onMounted(() => { refreshAll(); fetchRealProfit() })
</script>

<template>
  <main class="finance-page">
    <header class="finance-header"><div><div class="page-kicker">FINANCE WORKSPACE</div><h1>资金工作台</h1><p>聚焦账户余额、待回资金和资金调拨；日常收支请前往“记账”。</p></div><div class="header-actions"><el-radio-group v-model="period" size="small" @change="refreshAll"><el-radio-button value="today">今日</el-radio-button><el-radio-button value="month">本月</el-radio-button><el-radio-button value="all">累计</el-radio-button></el-radio-group><el-button type="primary" @click="$router.push('/admin/bookkeeping')">前往记账</el-button><el-dropdown trigger="click"><el-button :icon="Plus">账户操作</el-button><template #dropdown><el-dropdown-menu><el-dropdown-item :icon="Switch" @click="transferDialogVisible = true">资金划拨</el-dropdown-item><el-dropdown-item :icon="Management" @click="accountDialogVisible = true">新增资金账户</el-dropdown-item></el-dropdown-menu></template></el-dropdown></div></header>
    <!-- 真实净利润 -->
    <section class="real-profit-card" v-loading="realProfitLoading">
      <div class="real-profit-main">
        <div class="real-profit-head">
          <div class="page-kicker">REAL PROFIT / 真实净利润</div>
          <el-date-picker v-model="realProfitMonth" type="month" format="YYYY-MM" value-format="YYYY-MM" :clearable="false" size="small" style="width: 130px" @change="fetchRealProfit" />
        </div>
        <div class="real-profit-value" :class="realProfitTone">{{ money(realProfit?.real_profit) }}</div>
        <div class="real-profit-lines">
          <span>毛利 {{ money(realProfit?.gross_profit) }}</span>
          <span>经营开销 {{ money(realProfit?.expense_total) }}</span>
          <span>工资 {{ money(realProfit?.payroll_total) }}</span>
        </div>
        <div v-if="realProfit?.uncosted_order_item_count > 0" class="real-profit-warn">* 本月有 {{ realProfit.uncosted_order_item_count }} 个商品未录成本，毛利可能偏高</div>
      </div>
      <div class="real-profit-side">
        <el-button :icon="TrendCharts" @click="realProfitDrawerVisible = true">费用分类明细</el-button>
      </div>
    </section>

    <FinanceWorkspace :summary="summary" :total-balance="totalBalance" :net-cash-flow="netCashFlow" :attention-count="attentionCount" :period-label="periodLabel" :loading="loading" @open-orders="$router.push('/admin/orders')" @open-subsidy="$router.push('/admin/subsidy')" @open-bookkeeping="$router.push('/admin/bookkeeping')" @open-calibrate="openCalibrate" />

    <el-drawer v-model="realProfitDrawerVisible" :title="`${realProfitMonth} 费用分类明细（经营开销账）`" size="420px">
      <el-table :data="realProfit?.breakdown || []" size="small" style="width: 100%" empty-text="本月经营开销账暂无记录">
        <el-table-column label="分类" min-width="120">
          <template #default="{ row }">
            {{ row.category }}<el-tag v-if="row.is_payroll" size="small" type="warning" effect="plain" style="margin-left: 6px">工资</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="金额" align="right">
          <template #default="{ row }">{{ money(row.amount) }}</template>
        </el-table-column>
      </el-table>
      <div class="drawer-total">合计：毛利 {{ money(realProfit?.gross_profit) }} − 开销 {{ money(realProfit?.expense_total) }} − 工资 {{ money(realProfit?.payroll_total) }} = <b :class="realProfitTone">{{ money(realProfit?.real_profit) }}</b></div>
    </el-drawer>

    <el-dialog v-model="accountDialogVisible" title="新增资金账户" width="440px" destroy-on-close><el-form ref="accountFormRef" :model="accountForm" label-width="92px"><el-form-item label="账户名称" prop="name" :rules="[{ required: true, message: '请输入账户名称' }]"><el-input v-model="accountForm.name" placeholder="例如：支付宝、对公账户、备用金" /></el-form-item><el-form-item label="初始余额"><el-input-number v-model="accountForm.balance" :min="0" :precision="2" class="full-width" /></el-form-item></el-form><template #footer><el-button @click="accountDialogVisible = false">取消</el-button><el-button type="primary" :loading="submitting" @click="createAccount">创建账户</el-button></template></el-dialog>
    <el-dialog v-model="transferDialogVisible" title="账户资金划拨" width="500px" destroy-on-close><el-alert title="划拨只移动账户间资金，不计入收入或支出。" type="info" :closable="false" show-icon class="dialog-alert" /><el-form ref="transferFormRef" :model="transferForm" label-width="92px"><el-form-item label="转出账户" prop="from_account_id" :rules="[{ required: true, message: '请选择转出账户' }]"><el-select v-model="transferForm.from_account_id" class="full-width"><el-option v-for="account in accounts" :key="account.id" :label="`${account.name}（${account.balance.toFixed(2)}）`" :value="account.id" /></el-select></el-form-item><el-form-item label="转入账户" prop="to_account_id" :rules="[{ required: true, message: '请选择转入账户' }]"><el-select v-model="transferForm.to_account_id" class="full-width"><el-option v-for="account in accounts" :key="account.id" :label="account.name" :value="account.id" /></el-select></el-form-item><el-form-item label="划拨金额" prop="amount" :rules="[{ required: true, message: '请输入划拨金额' }]"><el-input-number v-model="transferForm.amount" :min="0.01" :precision="2" class="full-width" /></el-form-item><el-form-item label="备注"><el-input v-model="transferForm.remark" placeholder="例如：补充备用金" /></el-form-item></el-form><template #footer><el-button @click="transferDialogVisible = false">取消</el-button><el-button type="primary" :loading="submitting" @click="transferFunds">确认划拨</el-button></template></el-dialog>
    <el-dialog v-model="calibrateDialogVisible" :title="`核对余额：${calibrateForm.accountName}`" width="460px" destroy-on-close><el-alert title="校准会将账面余额对齐为实际余额，并自动保留一条校准审计记录。" type="warning" :closable="false" show-icon class="dialog-alert" /><el-form :model="calibrateForm" label-position="top"><el-form-item label="当前账面余额"><strong class="current-balance">¥{{ calibrateForm.currentBalance.toFixed(2) }}</strong></el-form-item><el-form-item label="实际余额"><el-input-number v-model="calibrateForm.realBalance" :min="0" :precision="2" class="full-width" /><small v-if="calibrateDiff" :class="calibrateDiff > 0 ? 'diff-positive' : 'diff-negative'">差异 {{ calibrateDiff > 0 ? '+' : '' }}¥{{ calibrateDiff.toFixed(2) }}</small></el-form-item><el-form-item label="备注"><el-input v-model="calibrateForm.remark" placeholder="例如：月末对账" /></el-form-item></el-form><template #footer><el-button @click="calibrateDialogVisible = false">取消</el-button><el-button type="warning" :loading="submitting" :disabled="!calibrateDiff" @click="calibrateAccount">确认校准</el-button></template></el-dialog>
  </main>
</template>

<style scoped>
.real-profit-card { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 20px 24px; background: #fff; border: 1px solid #e4eaf2; border-radius: 16px; }.real-profit-head { display: flex; align-items: center; gap: 12px; margin-bottom: 8px; }.real-profit-value { font-size: 32px; font-weight: 900; letter-spacing: -.04em; }.profit-positive { color: #059669; }.profit-negative { color: #e11d48; }.profit-zero { color: #475569; }.real-profit-lines { display: flex; flex-wrap: wrap; gap: 14px; margin-top: 8px; color: #64748b; font-size: 12.5px; }.real-profit-warn { margin-top: 6px; color: #d97706; font-size: 11.5px; }.real-profit-side { flex: none; }.drawer-total { margin-top: 14px; padding-top: 12px; border-top: 1px solid #f1f5f9; color: #475569; font-size: 13px; }.drawer-total b { font-size: 15px; }
.finance-page { max-width: 1320px; min-height: 100%; margin: 0 auto; padding: 28px; display: grid; gap: 20px; background: #f8fafc; }.finance-header { display: flex; align-items: center; justify-content: space-between; gap: 20px; }.page-kicker { color: #2563eb; font-size: 11px; font-weight: 800; letter-spacing: .14em; }.finance-header h1 { margin: 4px 0; color: #0f172a; font-size: 28px; letter-spacing: -.04em; }.finance-header p { margin: 0; color: #64748b; font-size: 13px; }.header-actions { display: flex; align-items: center; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }.full-width { width: 100%; }.native-date-input { width: 100%; height: 32px; padding: 0 10px; border: 1px solid #dcdfe6; border-radius: 4px; }.dialog-alert { margin-bottom: 18px; }.current-balance { color: #334155; font-size: 22px; }.diff-positive { display: block; margin-top: 7px; color: #059669; }.diff-negative { display: block; margin-top: 7px; color: #e11d48; } @media (max-width: 760px) { .finance-page { padding: 16px 12px; gap: 14px; }.finance-header { align-items: flex-start; flex-direction: column; gap: 12px; }.header-actions { display: grid; width: 100%; grid-template-columns: 1fr auto; justify-content: stretch; }.header-actions :deep(.el-radio-group) { grid-column: 1 / -1; display: grid; grid-template-columns: repeat(3, 1fr); width: 100%; }.header-actions :deep(.el-radio-button), .header-actions :deep(.el-radio-button__inner) { width: 100%; }.finance-header h1 { font-size: 24px; }.finance-header p { font-size: 12px; }.header-actions :deep(.el-button) { min-height: 38px; }.header-actions :deep(.el-dropdown) { width: 100%; }.header-actions :deep(.el-dropdown .el-button) { width: 100%; } :deep(.el-dialog) { width: calc(100vw - 24px) !important; margin-top: 8vh !important; }.native-date-input { min-height: 40px; font-size: 16px; }.real-profit-card { flex-direction: column; align-items: stretch; } }
</style>
