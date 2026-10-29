<template>
  <!--
    移动端通用信息卡（SPEC-ENG-001 §10，任务卡 T12）
    清单 / 回款 / 出库 / 费用 四个 Tab 在移动端都改用卡片流，共用这一个壳：
    第一行主信息 + 右侧金额（或状态），第二行次要信息，第三行起是 label/value 组。
    PC 端不使用本组件。
  -->
  <div class="eng-mrow" :class="clickable ? 'eng-mrow--clickable' : ''" @click="clickable && emit('click')">
    <div class="eng-mrow-head">
      <div class="eng-mrow-titles">
        <p class="eng-mrow-title">{{ title }}</p>
        <p v-if="subtitle" class="eng-mrow-sub">{{ subtitle }}</p>
      </div>
      <div class="eng-mrow-right">
        <span v-if="amount" class="eng-mrow-amount" :style="amountStyle">{{ amount }}</span>
        <slot name="right" />
      </div>
    </div>

    <div v-if="fields.length" class="eng-mrow-fields">
      <span v-for="f in fields" :key="f.label" class="eng-mrow-field">
        <span class="eng-mrow-label">{{ f.label }}</span>
        <span class="eng-mrow-value">{{ f.value }}</span>
      </span>
    </div>

    <slot />
  </div>
</template>

<script setup>
defineProps({
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' },
  amount: { type: String, default: '' },
  amountStyle: { type: Object, default: null },
  // [{ label, value }]
  fields: { type: Array, default: () => [] },
  clickable: { type: Boolean, default: false }
})

const emit = defineEmits(['click'])
</script>

<style scoped>
.eng-mrow {
  border-radius: 14px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 12px 14px;
}
.eng-mrow--clickable {
  cursor: pointer;
}
.eng-mrow-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}
.eng-mrow-titles {
  min-width: 0;
  flex: 1;
}
.eng-mrow-title {
  font-size: 14px;
  font-weight: 600;
  color: #0f172a;
  line-height: 20px;
  word-break: break-all;
}
.eng-mrow-sub {
  margin-top: 2px;
  font-size: 11px;
  color: #94a3b8;
  line-height: 16px;
}
.eng-mrow-right {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
  flex: 0 0 auto;
}
.eng-mrow-amount {
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
  font-variant-numeric: tabular-nums;
}
.eng-mrow-fields {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 14px;
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed #f1f5f9;
}
.eng-mrow-field {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
}
.eng-mrow-label {
  font-size: 11px;
  color: #94a3b8;
}
.eng-mrow-value {
  font-size: 12px;
  color: #334155;
  font-variant-numeric: tabular-nums;
}
</style>
