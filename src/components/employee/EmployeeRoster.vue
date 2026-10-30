<script setup>
import dayjs from 'dayjs'

defineProps({
  rows: { type: Array, default: () => [] },
  type: { type: String, default: 'employees' },
  loading: { type: Boolean, default: false }
})
const emit = defineEmits(['edit', 'attendance', 'remove', 'offboard'])
const money = value => `¥${Number(value || 0).toFixed(2)}`
const date = value => value ? dayjs(value).format('YYYY-MM-DD') : '未填写'
</script>

<template>
  <div class="roster" v-loading="loading">
    <div v-if="rows.length" class="roster-head">
      <span>人员</span><span>{{ type === 'employees' ? '薪资设置' : '服务信息' }}</span><span>联系方式</span><span>操作</span>
    </div>
    <article v-for="row in rows" :key="row.id" class="person-row">
      <div class="identity">
        <span class="avatar" :class="{ green: type !== 'employees' }">{{ row.name?.slice(0, 1) }}</span>
        <div><strong>{{ row.name }}</strong><small>{{ type === 'employees' ? `#${row.emp_no} · 入职 ${date(row.entry_date)}` : `安装/送货师傅 · 登记 ${date(row.created_at)}` }}</small></div>
      </div>
      <div class="pay-setting" v-if="type === 'employees'"><strong>{{ money(row.base_salary) }}</strong><small>底薪 · 提成 {{ (Number(row.commission_rate || 0) * 100).toFixed(2) }}%</small></div>
      <div class="pay-setting" v-else><strong>安装/送货</strong><small>按订单记录结算</small></div>
      <div class="contact"><strong>{{ row.phone || '未填写电话' }}</strong><small v-if="type === 'employees'">收银 PIN：{{ row.pin_code || '未设置' }}</small></div>
      <div class="actions">
        <el-tag v-if="type === 'employees' && row.employment_status === 'offboarding'" size="small" type="warning">交接中</el-tag>
        <el-tag v-else-if="type === 'employees' && row.employment_status === 'departed'" size="small" type="info">已离职</el-tag>
        <el-button size="small" type="primary" plain @click="emit('edit', row)">编辑</el-button>
        <el-button v-if="type === 'employees'" size="small" type="warning" plain @click="emit('offboard', row)">{{ row.employment_status === 'departed' ? '交接记录' : row.employment_status === 'offboarding' ? '交接进度' : '办理离职' }}</el-button>
        <el-button v-if="type === 'employees'" size="small" @click="emit('attendance', row)">考勤</el-button>
        <el-button size="small" text type="danger" @click="emit('remove', row)">删除</el-button>
      </div>
    </article>
    <el-empty v-if="!loading && !rows.length" description="没有匹配的人员" />
  </div>
</template>

<style scoped>
.roster{min-height:200px;background:white;border:1px solid #e4eaf2;border-radius:16px;overflow:hidden}.roster-head,.person-row{display:grid;grid-template-columns:minmax(220px,1.35fr) minmax(170px,1fr) minmax(155px,.9fr) minmax(220px,.9fr);align-items:center;gap:16px;padding:15px 22px}.roster-head{background:#f8fafd;color:#8490a4;font-size:11px;font-weight:800;letter-spacing:.04em}.person-row{border-top:1px solid #edf1f5}.identity{display:flex;align-items:center;gap:12px;min-width:0}.identity div,.pay-setting,.contact{display:flex;flex-direction:column;gap:5px;min-width:0}.identity strong,.pay-setting strong,.contact strong{font-size:14px;color:#213047}.identity small,.pay-setting small,.contact small{font-size:11px;color:#8793a5;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.avatar{display:grid;place-items:center;flex:none;width:42px;height:42px;border-radius:12px;background:#e9f1ff;color:#2e65a3;font-weight:800}.avatar.green{background:#e9f7f1;color:#248260}.actions{display:flex;justify-content:flex-end;gap:3px}.actions :deep(.el-button+.el-button){margin-left:0}@media(max-width:900px){.roster-head{display:none}.person-row{grid-template-columns:1fr 1fr;gap:16px;padding:18px}.identity{grid-column:span 2}.actions{grid-column:span 2;justify-content:flex-start}.contact{align-items:flex-end}}@media(max-width:480px){.person-row{gap:14px}.contact strong{font-size:12px}.actions :deep(.el-button){flex:1}}
</style>
