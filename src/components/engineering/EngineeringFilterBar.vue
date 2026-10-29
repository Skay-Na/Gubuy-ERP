<template>
  <!--
    工程列表筛选条（SPEC-ENG-001 §4.4）
    设计取舍：父组件把 useEngineeringList 里的 reactive(filters) 直接传进来，
    本组件按字段就地改写 —— 同一对象引用，父层 watch 照样触发。
    这样避免了 9 个字段写 9 组 v-model:xxx 事件，接缝更少、更不容易漏。
    唯一对外事件是 reset（由父层统一清空，保证 URL 同步逻辑只有一处）。
  -->
  <div class="eng-filter-bar">
    <el-input
      :model-value="filters.keyword"
      class="eng-filter-keyword"
      placeholder="搜项目名 / 编号 / 甲方 / 联系人 / 电话"
      clearable
      maxlength="50"
      @update:model-value="v => (filters.keyword = v || '')"
    />

    <el-select
      :model-value="filters.statuses"
      class="eng-filter-status"
      placeholder="状态"
      multiple
      collapse-tags
      collapse-tags-tooltip
      clearable
      @update:model-value="v => (filters.statuses = v || [])"
    >
      <el-option v-for="o in STATUS_OPTIONS" :key="o.value" :label="o.label" :value="o.value" />
    </el-select>

    <el-select
      :model-value="filters.projectType"
      class="eng-filter-type"
      placeholder="项目类型"
      clearable
      @update:model-value="v => (filters.projectType = v ?? null)"
    >
      <el-option v-for="o in PROJECT_TYPE_OPTIONS" :key="o.value" :label="o.label" :value="o.value" />
    </el-select>

    <el-select
      :model-value="filters.ownerId"
      class="eng-filter-owner"
      placeholder="负责人"
      clearable
      filterable
      @update:model-value="v => (filters.ownerId = v ?? null)"
    >
      <el-option v-for="e in employees" :key="e.id" :label="e.name" :value="e.id" />
    </el-select>

    <el-date-picker
      :model-value="filters.signRange"
      class="eng-filter-date"
      type="daterange"
      value-format="YYYY-MM-DD"
      range-separator="至"
      start-placeholder="签约开始"
      end-placeholder="签约结束"
      unlink-panels
      @update:model-value="v => (filters.signRange = v || [])"
    />

    <label class="eng-filter-switch">
      <el-switch :model-value="filters.onlyOverdue" @update:model-value="v => (filters.onlyOverdue = !!v)" />
      <span>只看逾期</span>
    </label>

    <label class="eng-filter-switch">
      <el-switch :model-value="filters.onlyPending" @update:model-value="v => (filters.onlyPending = !!v)" />
      <span>只看待回款</span>
    </label>

    <el-checkbox
      :model-value="filters.includeVoid"
      class="eng-filter-void"
      @update:model-value="v => (filters.includeVoid = !!v)"
    >
      含已作废
    </el-checkbox>

    <el-button text class="eng-filter-reset" @click="emit('reset')">重置</el-button>
  </div>
</template>

<script setup>
import { PROJECT_TYPE_OPTIONS, STATUS_OPTIONS } from '../../composables/useEngineeringList'

defineProps({
  filters: { type: Object, required: true },
  employees: { type: Array, default: () => [] }
})

const emit = defineEmits(['reset'])
</script>

<style scoped>
.eng-filter-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  background: #fff;
  padding: 12px 14px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.eng-filter-keyword {
  flex: 1 1 240px;
  min-width: 240px;
}
.eng-filter-status {
  width: 180px;
  flex: 0 0 auto;
}
.eng-filter-type,
.eng-filter-owner {
  width: 140px;
  flex: 0 0 auto;
}
.eng-filter-date {
  width: 260px;
  flex: 0 0 auto;
}
.eng-filter-switch {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #475569;
  cursor: pointer;
  user-select: none;
}
.eng-filter-void {
  margin-right: 0;
}
.eng-filter-reset {
  margin-left: auto;
  color: #64748b;
}

/* 移动端：抽屉里单列铺满，避免控件互相挤压 */
@media (max-width: 767px) {
  .eng-filter-bar {
    flex-direction: column;
    align-items: stretch;
  }
  .eng-filter-keyword,
  .eng-filter-status,
  .eng-filter-type,
  .eng-filter-owner,
  .eng-filter-date {
    width: 100%;
    min-width: 0;
    flex: 0 0 auto;
  }
  .eng-filter-reset {
    margin-left: 0;
    justify-content: flex-end;
  }
}
</style>
