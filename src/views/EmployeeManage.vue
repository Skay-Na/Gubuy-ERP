<template>
  <div class="p-6 max-w-7xl mx-auto">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row justify-between items-start md:items-center mb-6 gap-4">
      <div>
        <h1 class="text-2xl md:text-3xl font-black text-slate-900 tracking-tight mb-1">员工与外包管理</h1>
        <p class="text-sm text-slate-400">管理店内正式员工信息及外包服务商（安装/送货）</p>
      </div>
      <div class="flex gap-2 w-full md:w-auto">
        <el-button type="info" plain class="flex-1 md:flex-none !rounded-xl" @click="handleExport">
          <el-icon class="mr-1"><Download /></el-icon>导出清单
        </el-button>
        <el-button v-if="activeTab === 'employees'" type="success" plain class="flex-1 md:flex-none !rounded-xl" @click="openReportDrawer()">
          薪资核算
        </el-button>
        <el-button type="primary" class="flex-1 md:flex-none !rounded-xl shadow-lg shadow-blue-500/20" @click="openDialog()">
          {{ activeTab === 'employees' ? '新增员工' : '新增外包人员' }}
        </el-button>
      </div>
    </div>

    <!-- Tabs Toggle -->
    <div class="mb-6">
      <el-radio-group v-model="activeTab" size="large" class="!rounded-xl overflow-hidden border border-slate-100">
        <el-radio-button label="employees">正式员工</el-radio-button>
        <el-radio-button label="outsourced">外包合作人员</el-radio-button>
      </el-radio-group>
    </div>

    <!-- Data Table Card -->
    <el-card shadow="never" class="!rounded-2xl border-slate-100 shadow-sm overflow-hidden" body-class="!p-0">
      <!-- PC View -->
      <el-table 
        :data="filteredEmployees" 
        v-loading="loading" 
        style="width: 100%" 
        class="desktop-only hidden md:block"
        :header-cell-style="{ background: '#f8fafc', color: '#475569', fontWeight: '600', height: '54px' }"
        :row-style="{ height: '64px' }"
      >
        <el-table-column v-if="activeTab === 'employees'" prop="emp_no" label="工号" width="100" />
        <el-table-column v-else label="身份" width="120">
          <template #default>
            <el-tag type="success" effect="dark" size="small" class="!rounded-md">
              外包师傅
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="姓名" min-width="100" />
        <el-table-column v-if="activeTab === 'employees'" prop="pin_code" label="收银PIN码" width="100">
          <template #default="{ row }">
            <span class="font-mono text-blue-600 font-bold">{{ row.pin_code || '未设置' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="phone" label="联系电话" min-width="150" />
        <el-table-column v-if="activeTab === 'employees'" prop="base_salary" label="底薪 (元)" min-width="120" />
        <el-table-column v-if="activeTab === 'employees'" label="提成比例" min-width="150">
          <template #default="{ row }">
            {{ (row.commission_rate * 100).toFixed(2) }}%
          </template>
        </el-table-column>
        <el-table-column v-if="activeTab === 'employees'" label="入职时间" min-width="120">
          <template #default="{ row }">
            {{ formatEntryDate(row.entry_date) }}
          </template>
        </el-table-column>
        <el-table-column v-else label="登记时间" min-width="150">
          <template #default="{ row }">
            {{ formatEntryDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" link @click="openDialog(row)">编辑</el-button>
            <el-button v-if="activeTab === 'employees'" type="info" link @click="viewAttendance(row)">考勤</el-button>
            <el-button type="danger" link @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- Mobile View -->
      <div class="mobile-only md:hidden p-4 space-y-4">
        <div v-for="row in filteredEmployees" :key="row.id" class="bg-white border border-slate-100 rounded-2xl p-5 shadow-sm active:scale-[0.98] transition-transform">
          <div class="flex justify-between items-start mb-4">
            <div class="flex items-center gap-3">
              <div class="w-12 h-12 rounded-full flex items-center justify-center font-black text-xl"
                :class="activeTab === 'employees' ? 'bg-blue-50 text-blue-600' : 'bg-green-50 text-green-600'">
                {{ row.name.charAt(0) }}
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <span class="font-bold text-slate-800 text-lg">{{ row.name }}</span>
                  <el-tag v-if="activeTab === 'employees'" size="small" type="info" effect="plain" class="!rounded-md">#{{ row.emp_no }}</el-tag>
                  <el-tag v-else size="small" type="success" effect="dark" class="!rounded-md">外包师傅</el-tag>
                </div>
                <div class="text-xs text-slate-400 mt-0.5 flex items-center gap-1">
                  <el-icon size="10"><Phone /></el-icon> {{ row.phone || '无电话记录' }}
                </div>
              </div>
            </div>
            <div v-if="activeTab === 'employees'" class="text-right">
              <div class="text-[10px] text-slate-400 uppercase tracking-widest mb-1">终端 PIN</div>
              <div class="font-mono bg-blue-50 text-blue-700 px-2 py-0.5 rounded text-sm font-bold border border-blue-100">
                {{ row.pin_code || '未设' }}
              </div>
            </div>
          </div>

          <div v-if="activeTab === 'employees'" class="grid grid-cols-2 gap-3 mb-4 p-3 bg-slate-50 rounded-xl">
            <div>
              <div class="text-[10px] text-slate-400 mb-0.5">基础底薪</div>
              <div class="text-sm font-bold text-slate-700">¥{{ row.base_salary.toFixed(0) }}</div>
            </div>
            <div>
              <div class="text-[10px] text-slate-400 mb-0.5">销售提成</div>
              <div class="text-sm font-bold text-orange-600">{{ (row.commission_rate * 100).toFixed(1) }}%</div>
            </div>
          </div>

          <div class="flex gap-2">
            <el-button type="primary" class="flex-1 !rounded-xl !h-10" @click="openDialog(row)">
              <el-icon class="mr-1"><Edit /></el-icon>编辑
            </el-button>
            <el-button v-if="activeTab === 'employees'" plain class="flex-1 !rounded-xl !h-10" @click="viewAttendance(row)">
              <el-icon class="mr-1"><Calendar /></el-icon>考勤
            </el-button>
            <el-button type="danger" plain class="px-3 !rounded-xl !h-10" @click="handleDelete(row)">
              <el-icon><Delete /></el-icon>
            </el-button>
          </div>
        </div>
        <el-empty v-if="!loading && filteredEmployees.length === 0" description="暂无记录" />
      </div>
    </el-card>

    <!-- Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? (form.is_outsourced ? '编辑外包人员' : '编辑员工') : (form.is_outsourced ? '新增外包人员' : '新增员工')"
      :width="isMobile ? '90%' : '400px'"
      destroy-on-close
      class="!rounded-2xl"
    >
      <el-form :model="form" label-width="100px" class="mt-4">
        <!-- 仅普通员工显示工号，外包人员直接自动分配隐藏 -->
        <el-form-item v-if="!form.is_outsourced" label="工号" required>
          <el-input v-model="form.emp_no" placeholder="请输入工号" />
        </el-form-item>
        <el-form-item label="姓名" required>
          <el-input v-model="form.name" placeholder="请输入姓名" />
        </el-form-item>
        <el-form-item label="电话" required>
          <el-input v-model="form.phone" placeholder="请输入联系电话" />
        </el-form-item>
        

        <!-- 普通员工独有配置 -->
        <template v-if="!form.is_outsourced">
          <el-form-item label="收银PIN码">
            <el-input v-model="form.pin_code" placeholder="4-6位数字，用于终端登录" maxlength="6" />
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
            <input
              v-if="isMobile"
              v-model="form.entry_date"
              type="date"
              inputmode="none"
              aria-label="入职时间"
              class="mobile-native-date-input"
            />
            <el-date-picker
              v-else
              v-model="form.entry_date"
              type="date"
              placeholder="选择入职时间"
              format="YYYY-MM-DD"
              value-format="YYYY-MM-DD"
              style="width: 100%"
            />
          </el-form-item>
        </template>
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

    <el-drawer v-model="drawerVisible" title="薪资核算表" :size="payrollDrawerSize" class="payroll-drawer">
      <div class="payroll-panel">
        <div class="payroll-toolbar">
          <input
            v-if="isMobile"
            v-model="reportMonth"
            type="month"
            inputmode="none"
            aria-label="核算月份"
            class="mobile-native-date-input mobile-native-month-input"
            @change="fetchReport"
          />
          <el-date-picker 
            v-else
            v-model="reportMonth" 
            type="month" 
            format="YYYY-MM" 
            value-format="YYYY-MM" 
            @change="fetchReport" 
            placeholder="选择月份"
          />
          <div class="payroll-rules">
            规则：9:00 上班 / 17:00 下班；未上班打卡默认计入请假，可由管理员在表内直接调整；每月 4 天内不扣款。
          </div>
        </div>

        <div class="payroll-summary">
          <div class="payroll-summary-card">
            <span>应发合计</span>
            <strong>{{ formatMoney(payrollSummary.finalSalary) }}</strong>
          </div>
          <div class="payroll-summary-card">
            <span>提成合计</span>
            <strong>{{ formatMoney(payrollSummary.commission) }}</strong>
          </div>
          <div class="payroll-summary-card danger">
            <span>扣款合计</span>
            <strong>{{ formatMoney(payrollSummary.deduction) }}</strong>
          </div>
          <div class="payroll-summary-card">
            <span>核算人数</span>
            <strong>{{ reportData.length }} 人</strong>
          </div>
        </div>

        <el-table
          :data="reportData"
          style="width: 100%"
          border
          stripe
          size="small"
          height="calc(100vh - 260px)"
          v-loading="reportLoading"
          class="payroll-table"
          :header-cell-style="{ background: '#f8fafc', color: '#475569', fontWeight: 700 }"
        >
          <el-table-column prop="employee_name" label="员工" width="140" fixed="left">
            <template #default="{ row }">
              <div class="payroll-employee">
                <strong>{{ row.employee_name }}</strong>
                <span>#{{ row.emp_no }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="base_salary" label="底薪" width="110" align="right">
            <template #default="{ row }">{{ formatMoney(row.base_salary) }}</template>
          </el-table-column>
          <el-table-column prop="total_sales" label="月销售额" width="120" align="right">
            <template #default="{ row }">{{ formatMoney(row.total_sales) }}</template>
          </el-table-column>
          <el-table-column prop="total_commission" label="提成快照" width="120" align="right">
            <template #default="{ row }">{{ formatMoney(row.total_commission) }}</template>
          </el-table-column>
          <el-table-column label="请假天数（可调）" width="210" align="center">
            <template #default="{ row }">
              <div class="leave-editor">
                <el-input-number
                  v-model="row.manual_leave_days"
                  :min="0"
                  :max="31"
                  size="small"
                  :controls="false"
                  @change="updateLeaveDays(row)"
                />
                <div class="leave-actions">
                  <el-button size="small" text @click="setLeaveDays(row, Math.max(0, row.manual_leave_days - 1))">-1</el-button>
                  <el-button size="small" text @click="setLeaveDays(row, row.absent_days || 0)">按打卡 {{ row.absent_days || 0 }}天</el-button>
                  <el-button size="small" text @click="setLeaveDays(row, row.manual_leave_days + 1)">+1</el-button>
                </div>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="扣款依据" width="150">
            <template #default="{ row }">
              <div class="deduction-meta">
                <span>请假 {{ row.leave_days || 0 }} 天</span>
                <span>扣款 {{ row.deduction_days || 0 }} 天</span>
                <span class="hint">打卡估算 {{ row.absent_days || 0 }} 天</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="deduction" label="超假扣款" width="120" align="right">
            <template #default="{ row }">
              <span class="text-red-500 font-medium">-{{ formatMoney(row.deduction) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="final_salary" label="本月实发" width="150" fixed="right" align="right">
            <template #default="{ row }">
              <span class="text-blue-600 font-bold text-lg">{{ formatMoney(row.final_salary) }}</span>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-drawer>

    <!-- Attendance Logs Dialog -->
    <el-dialog v-model="attendanceDialogVisible" title="员工考勤明细" :width="isMobile ? '95%' : '600px'" class="!rounded-2xl">
      <div v-if="selectedEmpForAttendance" class="space-y-4">
        <div class="flex justify-between items-center mb-4">
          <div class="text-lg font-bold">{{ selectedEmpForAttendance.name }} 的打卡记录</div>
          <input
            v-if="isMobile"
            v-model="attendanceMonth"
            type="month"
            inputmode="none"
            aria-label="考勤月份"
            class="mobile-native-date-input mobile-native-month-input"
            @change="fetchAttendanceLogs"
          />
          <el-date-picker
            v-else
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
import { ref, onMounted, onUnmounted, computed } from 'vue'
import request from '../utils/request'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Phone, Edit, Calendar, Delete, Download } from '@element-plus/icons-vue'
import { downloadExcel } from '../utils/export'
import dayjs from 'dayjs'

const employees = ref([])
const loading = ref(false)
const windowWidth = ref(window.innerWidth)
const isMobile = computed(() => windowWidth.value < 768)
const payrollDrawerSize = computed(() => isMobile.value ? '100%' : 'calc(100vw - 14rem)')

const handleResize = () => {
  windowWidth.value = window.innerWidth
}

const handleExport = () => {
  downloadExcel('/api/employees/export')
}

const activeTab = ref('employees')
const filteredEmployees = computed(() => {
  if (activeTab.value === 'employees') {
    return employees.value.filter(e => !e.is_outsourced)
  } else {
    return employees.value.filter(e => e.is_outsourced)
  }
})

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
  is_outsourced: false,
  outsourced_type: 'install'
})

const drawerVisible = ref(false)
const reportMonth = ref(dayjs().format('YYYY-MM'))
const reportData = ref([])
const reportLoading = ref(false)

const formatMoney = (value) => Number(value || 0).toFixed(2)
const payrollSummary = computed(() => {
  return reportData.value.reduce((summary, row) => {
    summary.finalSalary += Number(row.final_salary || 0)
    summary.commission += Number(row.total_commission || 0)
    summary.deduction += Number(row.deduction || 0)
    return summary
  }, { finalSalary: 0, commission: 0, deduction: 0 })
})

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
    const res = await request.get('/api/employees/attendance-logs', {
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
    const res = await request.get(`/api/employees/commission-report?month=${reportMonth.value}`)
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
    await request.post('/api/employees/attendance', {
      employee_id: row.employee_id,
      month: reportMonth.value,
      leave_days: row.manual_leave_days || 0
    })
    fetchReport()
  } catch (error) {
    ElMessage.error('更新考勤失败')
  }
}

const setLeaveDays = (row, days) => {
  row.manual_leave_days = Math.max(0, Math.min(31, Number(days) || 0))
  updateLeaveDays(row)
}

const fetchEmployees = async () => {
  loading.value = true
  try {
    const res = await request.get('/api/employees')
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
      is_outsourced: row.is_outsourced || false,
      outsourced_type: row.outsourced_type || 'install'
    }
  } else {
    isEdit.value = false
    form.value = {
      id: null,
      emp_no: activeTab.value === 'outsourced' ? 'WB' + Date.now() : '',
      name: '',
      phone: '',
      base_salary: 0,
      commission_rate_percent: 0,
      pin_code: '',
      entry_date: null,
      is_outsourced: activeTab.value === 'outsourced',
      outsourced_type: 'install'
    }
  }
  dialogVisible.value = true
}

const handleSave = async () => {
  if (form.value.is_outsourced) {
    if (!form.value.emp_no) {
      form.value.emp_no = 'WB' + Date.now()
    }
  }

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
      base_salary: form.value.is_outsourced ? 0 : (form.value.base_salary || 0),
      commission_rate: form.value.is_outsourced ? 0 : (form.value.commission_rate_percent / 100),
      pin_code: form.value.is_outsourced ? '' : form.value.pin_code,
      entry_date: form.value.is_outsourced ? null : entryDateIso,
      is_outsourced: form.value.is_outsourced,
      outsourced_type: form.value.is_outsourced ? form.value.outsourced_type : ''
    }

    if (isEdit.value) {
      await request.put(`/api/employees/${form.value.id}`, payload)
      ElMessage.success('更新成功')
    } else {
      await request.post('/api/employees', payload)
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
      await request.delete(`/api/employees/${row.id}`)
      ElMessage.success('删除成功')
      fetchEmployees()
    } catch (error) {
      ElMessage.error('删除失败')
    }
  })
}

onMounted(() => {
  fetchEmployees()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
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

.payroll-panel {
  padding: 16px;
  background: #f8fafc;
  min-height: 100%;
}
.payroll-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.payroll-rules {
  flex: 1;
  color: #64748b;
  font-size: 12px;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  padding: 8px 12px;
}
.payroll-summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 12px;
}
.payroll-summary-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 14px;
  padding: 12px 14px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  box-shadow: 0 8px 22px rgba(15, 23, 42, 0.04);
}
.payroll-summary-card span {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}
.payroll-summary-card strong {
  color: #0f172a;
  font-size: 18px;
  font-weight: 900;
}
.payroll-summary-card.danger strong {
  color: #ef4444;
}
.payroll-table {
  border-radius: 14px;
  overflow: hidden;
  background: #fff;
}
.payroll-employee {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.2;
}
.payroll-employee strong {
  color: #0f172a;
  font-weight: 800;
}
.payroll-employee span,
.deduction-meta .hint {
  color: #94a3b8;
  font-size: 11px;
}
.leave-editor {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: center;
}
.leave-editor :deep(.el-input-number) {
  width: 76px;
}
.leave-actions {
  display: flex;
  gap: 2px;
  align-items: center;
  justify-content: center;
}
.leave-actions :deep(.el-button) {
  padding: 2px 4px;
  height: 20px;
  font-size: 11px;
}
.deduction-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
  color: #475569;
  line-height: 1.25;
}
.mobile-native-date-input {
  width: 100%;
  min-height: 40px;
  padding: 0 12px;
  border: 1px solid #cbd5e1;
  border-radius: 10px;
  background: #fff;
  color: #334155;
  font-size: 16px;
}
.mobile-native-month-input {
  width: auto;
}

@media (max-width: 900px) {
  .payroll-toolbar {
    align-items: stretch;
    flex-direction: column;
  }
  .payroll-summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
