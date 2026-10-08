<script setup>
import { computed, reactive, watch } from 'vue'
import { CircleCheck, EditPen, Wallet } from '@element-plus/icons-vue'

const props = defineProps({
  accounts: { type: Array, default: () => [] },
  submitting: { type: Boolean, default: false },
  resetToken: { type: Number, default: 0 }
})

const emit = defineEmits(['submit'])

const form = reactive({
  expense_type: '日常采购',
  amount: undefined,
  account_id: null,
  remark: ''
})

const expenseTypes = ['日常采购', '运输配送', '水电物业', '员工报销', '营销推广', '其他支出']

const accountOptions = computed(() => props.accounts || [])
const selectedAccount = computed(() => accountOptions.value.find(account => account.id === form.account_id))

watch(
  accountOptions,
  (accounts) => {
    if (!accounts.length || form.account_id) return
    form.account_id = accounts.find(account => account.is_default_expense)?.id || accounts[0].id
  },
  { immediate: true }
)

const selectExpenseType = (type) => {
  form.expense_type = type
}

const submit = () => {
  const amount = Number(form.amount)
  if (!form.expense_type.trim() || !Number.isFinite(amount) || amount <= 0) return

  emit('submit', {
    expense_type: form.expense_type.trim(),
    amount,
    account_id: form.account_id || undefined,
    remark: form.remark.trim()
  })
}

const reset = () => {
  form.amount = undefined
  form.remark = ''
}

watch(() => props.resetToken, reset)
</script>

<template>
  <section class="quick-expense" aria-labelledby="quick-expense-title">
    <div class="quick-expense-heading">
      <div class="quick-expense-icon"><el-icon><EditPen /></el-icon></div>
      <div>
        <div class="quick-expense-eyebrow">快捷记账</div>
        <h2 id="quick-expense-title">记一笔日常支出</h2>
        <p>选分类、填金额，即可完成入账。</p>
      </div>
    </div>

    <div class="quick-expense-body">
      <div class="quick-expense-field type-field">
        <label>支出分类</label>
        <div class="expense-type-list">
          <button
            v-for="type in expenseTypes"
            :key="type"
            type="button"
            class="expense-type"
            :class="{ active: form.expense_type === type }"
            @click="selectExpenseType(type)"
          >
            {{ type }}
          </button>
        </div>
        <el-input v-model="form.expense_type" maxlength="50" placeholder="或输入自定义分类" class="custom-expense-type" />
      </div>

      <div class="quick-expense-field amount-field">
        <label for="quick-expense-amount">金额</label>
        <el-input-number
          id="quick-expense-amount"
          v-model="form.amount"
          :min="0.01"
          :precision="2"
          :step="100"
          controls-position="right"
          placeholder="0.00"
          class="amount-input"
          @keyup.enter="submit"
        />
      </div>

      <div class="quick-expense-field account-field">
        <label>支出账户</label>
        <el-select v-model="form.account_id" placeholder="自动使用默认账户" class="account-select">
          <el-option v-for="account in accountOptions" :key="account.id" :label="account.name" :value="account.id">
            <div class="account-option">
              <span>{{ account.name }}</span>
              <small>¥{{ Number(account.balance || 0).toFixed(2) }}</small>
            </div>
          </el-option>
        </el-select>
        <small v-if="selectedAccount" class="account-hint"><el-icon><Wallet /></el-icon> 余额 ¥{{ Number(selectedAccount.balance || 0).toFixed(2) }}</small>
      </div>

      <div class="quick-expense-field remark-field">
        <label for="quick-expense-remark">备注 <span>选填</span></label>
        <el-input id="quick-expense-remark" v-model="form.remark" maxlength="255" placeholder="例如：门店耗材采购" @keyup.enter="submit" />
      </div>

      <button
        type="button"
        class="quick-expense-submit"
        :disabled="submitting || !form.expense_type.trim() || !Number(form.amount)"
        @click="submit"
      >
        <el-icon><CircleCheck /></el-icon>
        {{ submitting ? '正在保存…' : '确认记账' }}
      </button>
    </div>
  </section>
</template>

<style scoped>
.quick-expense {
  max-width: 1280px;
  margin: 0 auto 14px;
  padding: clamp(15px, 1.6vw, 22px);
  border: 1px solid #bfdbfe;
  border-radius: 22px;
  background: linear-gradient(115deg, #eff6ff, #ffffff 46%, #f0fdfa);
  box-shadow: 0 14px 32px rgba(15, 23, 42, 0.06);
}

.quick-expense-heading { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
.quick-expense-icon { display: grid; width: 42px; height: 42px; place-items: center; border-radius: 15px; background: #2563eb; color: #fff; font-size: 21px; }
.quick-expense-eyebrow { color: #2563eb; font-size: 11px; font-weight: 900; letter-spacing: .12em; }
.quick-expense-heading h2 { margin: 1px 0 2px; color: #0f172a; font-size: 18px; font-weight: 900; }
.quick-expense-heading p { margin: 0; color: #64748b; font-size: 12px; }
.quick-expense-body { display: grid; grid-template-columns: minmax(260px, 1.55fr) minmax(130px, .55fr) minmax(160px, .8fr) minmax(180px, 1fr) auto; align-items: end; gap: 12px; }
.quick-expense-field { min-width: 0; }
.quick-expense-field label { display: block; margin: 0 0 7px; color: #475569; font-size: 12px; font-weight: 800; }
.quick-expense-field label span { color: #94a3b8; font-weight: 500; }
.expense-type-list { display: flex; flex-wrap: wrap; gap: 6px; }
.expense-type { padding: 6px 9px; border: 1px solid #cbd5e1; border-radius: 999px; background: #fff; color: #475569; font-size: 12px; font-weight: 700; cursor: pointer; transition: all .15s ease; }
.expense-type:hover, .expense-type.active { border-color: #2563eb; background: #2563eb; color: #fff; }
.custom-expense-type { margin-top: 7px; }
.amount-input, .account-select { width: 100%; }
.account-hint { display: flex; align-items: center; gap: 4px; margin-top: 7px; color: #64748b; font-size: 11px; }
.account-option { display: flex; justify-content: space-between; gap: 20px; }
.account-option small { color: #94a3b8; }
.quick-expense-submit { display: inline-flex; min-height: 40px; align-items: center; justify-content: center; gap: 7px; padding: 0 18px; border: 0; border-radius: 10px; background: #2563eb; color: #fff; font-weight: 800; white-space: nowrap; cursor: pointer; box-shadow: 0 8px 18px rgba(37, 99, 235, .24); }
.quick-expense-submit:disabled { cursor: not-allowed; opacity: .58; box-shadow: none; }

@media (max-width: 980px) {
  .quick-expense-body { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .quick-expense-submit { min-height: 44px; }
}

@media (max-width: 620px) {
  .quick-expense { border-radius: 18px; }
  .quick-expense-body { grid-template-columns: 1fr; gap: 13px; }
  .quick-expense-submit { width: 100%; }
}
</style>
