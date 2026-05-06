<template>
  <div class="p-6 max-w-7xl mx-auto">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row justify-between items-start md:items-center mb-8 gap-4">
      <div>
        <h1 class="text-2xl md:text-3xl font-black text-slate-900 tracking-tight mb-1">员工管理</h1>
        <p class="text-sm text-slate-400">管理员工信息及提成比例</p>
      </div>
      <div class="flex gap-2 w-full md:w-auto">
        <el-button type="success" plain class="flex-1 md:flex-none !rounded-xl" @click="openReportDrawer()">
          薪资核算
        </el-button>
        <el-button type="primary" class="flex-1 md:flex-none !rounded-xl shadow-lg shadow-blue-500/20" @click="openDialog()">
          新增员工
        </el-button>
      </div>
    </div>

    <!-- Data Table Card -->
    <el-card shadow="never" class="!rounded-2xl border-slate-100 shadow-sm overflow-hidden" body-class="!p-0">
      <!-- PC View -->
      <el-table 
        :data="employees" 
        v-loading="loading" 
        style="width: 100%" 
        class="desktop-only hidden md:block"
        :header-cell-style="{ background: '#f8fafc', color: '#475569', fontWeight: '600', height: '54px' }"
        :row-style="{ height: '64px' }"
      >
        <el-table-column prop="emp_no" label="工号" width="100" />
        <el-table-column prop="name" label="姓名" min-width="100" />
        <el-table-column prop="pin_code" label="收银PIN码" width="100">
          <template #default="{ row }">
            <span class="font-mono text-blue-600 font-bold">{{ row.pin_code || '未设置' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="phone" label="联系电话" min-width="150" />
        <el-table-column prop="base_salary" label="底薪 (元)" min-width="120" />
        <el-table-column label="提成比例" min-width="150">
          <template #default="{ row }">
            {{ (row.commission_rate * 100).toFixed(2) }}%
          </template>
        </el-table-column>
        <el-table-column label="入职时间" min-width="120">
          <template #default="{ row }">
            {{ formatEntryDate(row.entry_date) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" link @click="openDialog(row)">编辑</el-button>
            <el-button type="info" link @click="viewAttendance(row)">考勤</el-button>
            <el-button type="danger" link @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- Mobile View -->
      <div class="mobile-only md:hidden p-4 space-y-3">
        <div v-for="row in employees" :key="row.id" class="p-4 bg-slate-50 rounded-2xl flex justify-between items-center">
          <div class="flex-1 min-w-0 pr-4">
            <div class="flex items-center gap-2 mb-1">
              <span class="font-bold text-slate-800">{{ row.name }}</span>
              <el-tag size="small" effect="plain" class="!text-[10px]">#{{ row.emp_no }}</el-tag>
            </div>
            <div class="text-[10px] text-slate-400">
              PIN: <span class="text-blue-600 font-bold">{{ row.pin_code || '-' }}</span> | 底薪: ¥{{ row.base_salary }}
            </div>
          </div>
          <div class="flex gap-2">
            <el-button type="primary" plain size="small" @click="openDialog(row)" class="!rounded-lg">编辑</el-button>
          </div>
        </div>
        <el-empty v-if="employees.length === 0" description="暂无员工" />
      </div>
    </el-card>

    <!-- Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑员工' : '新增员工'"
      width="400px"
      destroy-on-close
    >
      <el-form :model="form" label-width="100px" class="mt-4">
        <el-form-item label="工号" required>
          <el-input v-model="form.emp_no" placeholder="请输入工号" />
        </el-form-item>
        <el-form-item label="姓名" required>
          <el-input v-model="form.name" placeholder="请输入姓名" />
        </el-form-item>
        <el-form-item label="收银PIN码">
          <el-input v-model="form.pin_code" placeholder="4-6位数字，用于终端登录" maxlength="6" />
        </el-form-item>
        <el-form-item label="电话">
          <el-input v-model="form.phone" placeholder="请输入联系电话" />
        </el-form-item>
        <el-form-item label="底薪 (元)">
          <el-input-number v-model="form.base_salary" :min="0" :precision="2" :step="100" class="w-full" />
        </el-form-item>
        <el-form-item label="提成比例">
          <div class="flex items-center gap-2">
            <el-input-number v-model="form.commission_rate_percent" :min="0" :max="100" :precision="2" :step="1" :controls="false" />
            <span>%</span>
          </div>
        </el-form-item>
        <el-form-item label="入职时间">
          <el-date-picker
            v-model="form.entry_date"
            type="date"
            placeholder="选择入职时间"
            format="YYYY-MM-DD"
            value-format="YYYY-MM-DD"
            style="width: 100%"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="handleSave" :loading="saving">
            保存
          </el-button>
        </span>
      </template>
    </el-dialog>

    <el-drawer v-model="drawerVisible" title="薪资核算表" size="80%">
      <div class="p-4">
        <div class="mb-4">
          <el-date-picker 
            v-model="reportMonth" 
            type="month" 
            format="YYYY-MM" 
            value-format="YYYY-MM" 
            @change="fetchReport" 
            placeholder="选择月份"
          />
        </div>
        <el-table :data="reportData" style="width: 100%" border v-loading="reportLoading">
          <el-table-column prop="employee_name" label="姓名" width="120" />
          <el-table-column prop="base_salary" label="底薪 (元)" width="120" />
          <el-table-column prop="total_sales" label="当月销售额" width="120">
            <template #default="{ row }">{{ row.total_sales.toFixed(2) }}</template>
          </el-table-column>
          <el-table-column prop="total_commission" label="当月提成" width="120">
            <template #default="{ row }">{{ row.total_commission.toFixed(2) }}</template>
          </el-table-column>
          <el-table-column label="请假天数" width="160">
            <template #default="{ row }">
              <el-input-number 
                v-model="row.leave_days" 
                :min="0" 
                :max="31" 
                size="small" 
                @change="updateLeaveDays(row)" 
              />
            </template>
          </el-table-column>
          <el-table-column prop="deduction" label="超假扣款" width="120">
            <template #default="{ row }">
              <span class="text-red-500 font-medium">-{{ row.deduction.toFixed(2) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="final_salary" label="本月实发" min-width="150" fixed="right">
            <template #default="{ row }">
              <span class="text-blue-600 font-bold text-lg">{{ row.final_salary.toFixed(2) }}</span>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-drawer>

    <!-- Attendance Logs Dialog -->
    <el-dialog v-model="attendanceDialogVisible" title="员工考勤明细" width="600px" class="!rounded-2xl">
      <div v-if="selectedEmpForAttendance" class="space-y-4">
        <div class="flex justify-between items-center mb-4">
          <div class="text-lg font-bold">{{ selectedEmpForAttendance.name }} 的打卡记录</div>
          <el-date-picker
            v-model="attendanceMonth"
            type="month"
            format="YYYY-MM"
            value-format="YYYY-MM"
            size="small"
            @change="fetchAttendanceLogs"
          />
        </div>

        <el-table :data="attendanceLogs" border v-loading="logsLoading" size="small">
          <el-table-column prop="date" label="日期" width="120" />
          <el-table-column label="上班打卡" width="150">
            <template #default="{ row }">
              {{ row.check_in ? formatTime(row.check_in) : '-' }}
            </template>
          </el-table-column>
          <el-table-column label="下班打卡" width="150">
            <template #default="{ row }">
              {{ row.check_out ? formatTime(row.check_out) : '-' }}
            </template>
          </el-table-column>
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="getStatusType(row.status)" size="small">
                {{ getStatusLabel(row.status) }}
              </el-tag>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'
import dayjs from 'dayjs'

const employees = ref([])
const loading = ref(false)

const dialogVisible = ref(false)
const isEdit = ref(false)
const saving = ref(false)
const form = ref({
  id: null,
  emp_no: '',
  name: '',
  phone: '',
  base_salary: 0,
  commission_rate_percent: 0,
  pin_code: '',
  entry_date: null,
})

const drawerVisible = ref(false)
const reportMonth = ref(dayjs().format('YYYY-MM'))
const reportData = ref([])
const reportLoading = ref(false)

// Attendance Logs Logic
const attendanceDialogVisible = ref(false)
const selectedEmpForAttendance = ref(null)
const attendanceMonth = ref(dayjs().format('YYYY-MM'))
const attendanceLogs = ref([])
const logsLoading = ref(false)

const viewAttendance = (row) => {
  selectedEmpForAttendance.value = row
  attendanceMonth.value = dayjs().format('YYYY-MM')
  attendanceDialogVisible.value = true
  fetchAttendanceLogs()
}

const fetchAttendanceLogs = async () => {
  if (!selectedEmpForAttendance.value) return
  logsLoading.value = true
  try {
    const res = await axios.get('/api/employees/attendance-logs', {
      params: {
        employee_id: selectedEmpForAttendance.value.id,
        month: attendanceMonth.value
      }
    })
    if (res.data.code === 200) {
      attendanceLogs.value = res.data.data
    }
  } catch (error) {
    ElMessage.error('获取考勤记录失败')
  } finally {
    logsLoading.value = false
  }
}

const formatTime = (timeStr) => {
  if (!timeStr) return '-'
  return dayjs(timeStr).format('HH:mm:ss')
}

const getStatusLabel = (status) => {
  const map = { 1: '正常', 2: '迟到', 3: '早退', 4: '缺勤' }
  return map[status] || '未知'
}

const getStatusType = (status) => {
  const map = { 1: 'success', 2: 'warning', 3: 'warning', 4: 'danger' }
  return map[status] || 'info'
}

const openReportDrawer = () => {
  drawerVisible.value = true
  fetchReport()
}

const fetchReport = async () => {
  reportLoading.value = true
  try {
    const res = await axios.get(`/api/employees/commission-report?month=${reportMonth.value}`)
    if (res.data.code === 200) {
      reportData.value = res.data.data
    }
  } catch (error) {
    ElMessage.error('获取薪资报表失败')
  } finally {
    reportLoading.value = false
  }
}

const updateLeaveDays = async (row) => {
  try {
    await axios.post('/api/employees/attendance', {
      employee_id: row.employee_id,
      month: reportMonth.value,
      leave_days: row.leave_days
    })
    fetchReport()
  } catch (error) {
    ElMessage.error('更新考勤失败')
  }
}

const fetchEmployees = async () => {
  loading.value = true
  try {
    const res = await axios.get('/api/employees')
    if (res.data.code === 200) {
      employees.value = res.data.data
    }
  } catch (error) {
    ElMessage.error('获取员工列表失败')
  } finally {
    loading.value = false
  }
}

const formatDate = (date) => {
  return dayjs(date).format('YYYY-MM-DD HH:mm')
}

const formatEntryDate = (date) => {
  if (!date) return '-'
  return dayjs(date).format('YYYY-MM-DD')
}

const openDialog = (row) => {
  if (row) {
    isEdit.value = true
    form.value = {
      id: row.id,
      emp_no: row.emp_no,
      name: row.name,
      phone: row.phone,
      base_salary: row.base_salary,
      commission_rate_percent: row.commission_rate * 100,
      pin_code: row.pin_code || '',
      entry_date: row.entry_date ? dayjs(row.entry_date).format('YYYY-MM-DD') : null,
    }
  } else {
    isEdit.value = false
    form.value = {
      id: null,
      emp_no: '',
      name: '',
      phone: '',
      base_salary: 0,
      commission_rate_percent: 0,
      pin_code: '',
      entry_date: null,
    }
  }
  dialogVisible.value = true
}

const handleSave = async () => {
  if (!form.value.emp_no || !form.value.name) {
    ElMessage.warning('工号和姓名不能为空')
    return
  }

  saving.value = true
  try {
    let entryDateIso = null
    if (form.value.entry_date) {
      entryDateIso = dayjs(form.value.entry_date, 'YYYY-MM-DD').toISOString()
    }

    const payload = {
      emp_no: form.value.emp_no,
      name: form.value.name,
      phone: form.value.phone,
      base_salary: form.value.base_salary || 0,
      commission_rate: form.value.commission_rate_percent / 100,
      pin_code: form.value.pin_code,
      entry_date: entryDateIso
    }

    if (isEdit.value) {
      await axios.put(`/api/employees/${form.value.id}`, payload)
      ElMessage.success('更新成功')
    } else {
      await axios.post('/api/employees', payload)
      ElMessage.success('创建成功')
    }
    
    dialogVisible.value = false
    fetchEmployees()
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '保存失败')
  } finally {
    saving.value = false
  }
}

const handleDelete = (row) => {
  ElMessageBox.confirm(
    `确认删除员工 ${row.name} 吗？`,
    '警告',
    {
      confirmButtonText: '确认',
      cancelButtonText: '取消',
      type: 'warning',
    }
  ).then(async () => {
    try {
      await axios.delete(`/api/employees/${row.id}`)
      ElMessage.success('删除成功')
      fetchEmployees()
    } catch (error) {
      ElMessage.error('删除失败')
    }
  })
}

onMounted(() => {
  fetchEmployees()
})
</script>

<style scoped>
:deep(.el-card__body) {
  padding: 0;
}
.custom-table {
  --el-table-border-color: #f1f5f9;
  --el-table-header-bg-color: #f8fafc;
  --el-table-row-hover-bg-color: #f8fafc;
}
:deep(.custom-table .el-table__inner-wrapper::before) {
  display: none;
}
</style>
