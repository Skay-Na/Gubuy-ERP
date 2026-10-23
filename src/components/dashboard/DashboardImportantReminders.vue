<script setup>
import { CircleCheckFilled, Right, WarningFilled } from '@element-plus/icons-vue'

const props = defineProps({
  zeroCostSaleCount: { type: Number, default: 0 }
})

const emit = defineEmits(['open-cost-repair'])
</script>

<template>
  <section class="important-reminders" aria-labelledby="important-reminders-title">
    <header class="reminders-heading">
      <div>
        <span class="reminders-eyebrow">FOLLOW UP</span>
        <h2 id="important-reminders-title">重要事项提醒</h2>
      </div>
      <span v-if="props.zeroCostSaleCount > 0" class="reminders-count">
        {{ props.zeroCostSaleCount }} 项待处理
      </span>
    </header>

    <button
      v-if="props.zeroCostSaleCount > 0"
      type="button"
      class="reminder-card"
      aria-label="打开历史成本修复，核对零成本售出订单"
      @click="emit('open-cost-repair')"
    >
      <span class="reminder-icon" aria-hidden="true"><el-icon><WarningFilled /></el-icon></span>
      <span class="reminder-copy">
        <strong>零成本售出提醒</strong>
        <span>有 {{ props.zeroCostSaleCount }} 条有效订单明细成本为 0 或负数，毛利统计暂不完整。请核对凭证后逐笔补录。</span>
      </span>
      <span class="reminder-action">去核对 <el-icon><Right /></el-icon></span>
    </button>

    <div v-else class="reminders-empty" role="status">
      <el-icon aria-hidden="true"><CircleCheckFilled /></el-icon>
      暂无待处理的零成本售出事项
    </div>
  </section>
</template>

<style scoped>
.important-reminders{margin:0 0 16px;padding:18px 20px;background:#fff;border:1px solid #e8edf3;border-radius:17px}
.reminders-heading{display:flex;justify-content:space-between;align-items:center;gap:12px;margin-bottom:13px}
.reminders-eyebrow{font-size:10px;font-weight:800;letter-spacing:.14em;color:#94a3b8}
.reminders-heading h2{margin:4px 0 0;color:#172033;font-size:18px;font-weight:750}
.reminders-count{padding:5px 9px;border-radius:999px;background:#fff2e8;color:#c66a17;font-size:11px;font-weight:700;white-space:nowrap}
.reminder-card{width:100%;display:flex;align-items:center;gap:13px;padding:14px;text-align:left;background:#fffaf5;border:1px solid #f4dec7;border-radius:12px;color:#344054;cursor:pointer;transition:border-color .15s,background .15s}
.reminder-card:hover{background:#fff7ed;border-color:#e8bd91}
.reminder-icon{width:36px;height:36px;display:grid;place-items:center;flex:none;border-radius:10px;background:#ffedd5;color:#d97706;font-size:18px}
.reminder-copy{display:flex;flex:1;flex-direction:column;gap:4px;min-width:0}
.reminder-copy strong{font-size:13px;color:#7c3f0c}
.reminder-copy span{color:#7b6b5b;font-size:12px;line-height:1.5}
.reminder-action{display:flex;align-items:center;gap:5px;flex:none;color:#b65b12;font-size:12px;font-weight:700;white-space:nowrap}
.reminders-empty{display:flex;align-items:center;gap:8px;padding:14px;border:1px solid #e6efe9;border-radius:12px;background:#f8fcf9;color:#6d8274;font-size:12px}
.reminders-empty .el-icon{color:#39a56d;font-size:16px}
@media(max-width:767px){.important-reminders{padding:15px;margin-bottom:12px}.reminder-card{align-items:flex-start;gap:10px;padding:12px}.reminder-icon{width:32px;height:32px}.reminder-copy span{font-size:11px}.reminder-action{padding-top:8px;font-size:11px}}
</style>
