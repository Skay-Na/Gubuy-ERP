<template>
  <div class="p-6 max-w-7xl mx-auto">
    <header class="people-header">
      <div>
        <p class="people-eyebrow">TEAM / 员工管理</p>
        <h1>人员与薪资</h1>
        <p>维护在职员工信息，核对每月薪资和考勤。</p>
      </div>
      <div class="people-header-actions">
        <el-button @click="handleExport"><el-icon class="mr-1"><Download /></el-icon>导出人员</el-button>
        <el-button v-if="activeTab === 'employees'" type="primary" plain @click="openReportDrawer">薪资结算</el-button>
        <el-button type="primary" @click="openDialog()">{{ activeTab === 'employees' ? '新增员工' : '新增外包人员' }}</el-button>
      </div>
    </header>

    <div class="people-metrics">
      <div><span>在职正式员工</span><strong>{{ formalCount }}</strong><small>不含已离职档案</small></div>
      <div><span>外包合作人员</span><strong>{{ outsourcedCount }}</strong><small>独立管理</small></div>
      <div><span>员工底薪合计</span><strong>¥{{ formatMoney(baseSalaryTotal) }}</strong><small>不含提成与扣款</small></div>
    </div>

    <div class="people-controls">
      <el-segmented v-model="activeTab" :options="[{ label: '正式员工', value: 'employees' }, { label: '外包人员', value: 'outsourced' }]" />
      <el-input v-model="employeeQuery" clearable :placeholder="activeTab === 'employees' ? '搜索姓名、工号或电话' : '搜索姓名或电话'" class="people-search" />
      <span class="people-count">{{ filteredEmployees.length }} 人</span>
    </div>
    <EmployeeRoster :rows="filteredEmployees" :type="activeTab" :loading="loading" @edit="openDialog" @attendance="viewAttendance" @remove="handleDelete" @offboard="openOffboarding" />
    <EmployeeOffboardingDialog v-model="offboardingVisible" :employee="selectedOffboardingEmployee" :employees="employees" @updated="fetchEmployees" />

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

    <el-drawer v-model="drawerVisible" title="薪资结算" size="100%" append-to-body class="payroll-drawer">
      <PayrollWorkspace
        :month="reportMonth"
        :rows="reportData"
        :loading="reportLoading"
        :leave-saving="leaveSaving"
        :is-mobile="isMobile"
        @month-change="changeReportMonth"
        @refresh="fetchReport"
        @leave-change="updateLeaveDays"
        @open-detail="openPayrollDetail"
        @view-order="viewPayrollOrder"
      />
    </el-drawer>

    <PayrollDetail v-model="payrollDetailVisible" :employee="selectedPayrollEmployee" :month="reportMonth" @view-order="viewPayrollOrder" />

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
import { useRouter } from 'vue-router'
import {
  fetchEmployees as fetchEmployeesApi,
  createEmployee,
  updateEmployee,
  deleteEmployee,
  fetchAttendanceLogs as fetchAttendanceLogsApi,
  fetchCommissionReport,
  recordAttendance
} from '../api/employees'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Download } from '@element-plus/icons-vue'
import { downloadExcel } from '../utils/export'
import dayjs from 'dayjs'
import PayrollDetail from '../components/employee/PayrollDetail.vue'
import PayrollWorkspace from '../components/employee/PayrollWorkspace.vue'
import EmployeeRoster from '../components/employee/EmployeeRoster.vue'
import EmployeeOffboardingDialog from '../components/employee/EmployeeOffboardingDialog.vue'

const router = useRouter()

const employees = ref([])
const offboardingVisible = ref(false)
const selectedOffboardingEmployee = ref(null)
const loading = ref(false)
const windowWidth = ref(window.innerWidth)
const isMobile = computed(() => windowWidth.value < 768)

const handleResize = () => {
  windowWidth.value = window.innerWidth
}

const handleExport = () => {
  downloadExcel('/api/employees/export')
}

const activeTab = ref('employees')
const employeeQuery = ref('')
const formalCount = computed(() => employees.value.filter(row => !row.is_outsourced && row.employment_status !== 'departed').length)
const outsourcedCount = computed(() => employees.value.filter(row => row.is_outsourced).length)
const baseSalaryTotal = computed(() => employees.value.filter(row => !row.is_outsourced && row.employment_status !== 'departed').reduce((total, row) => total + Number(row.base_salary || 0), 0))
const filteredEmployees = computed(() => {
  const keyword = employeeQuery.value.trim().toLowerCase()
  return employees.value.filter(row => {
    if (row.is_outsourced !== (activeTab.value === 'outsourced')) return false
    return !keyword || `${row.name} ${row.emp_no} ${row.phone || ''}`.toLowerCase().includes(keyword)
  })
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
const leaveSaving = ref(false)
let reportRequestId = 0
const payrollDetailVisible = ref(false)
const selectedPayrollEmployeeId = ref(null)
const selectedPayrollEmployee = computed(() => reportData.value.find(row => row.employee_id === selectedPayrollEmployeeId.value) || null)

const openPayrollDetail = row => {
  selectedPayrollEmployeeId.value = row.employee_id
  payrollDetailVisible.value = true
}

const viewPayrollOrder = order => {
	if (!order?.order_no) return
	payrollDetailVisible.value = false
	drawerVisible.value = false
	router.push({ name: 'OrderManage', query: { order_no: order.order_no } })
}

const formatMoney = (value) => Number(value || 0).toFixed(2)

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
    const res = await fetchAttendanceLogsApi({
      employee_id: selectedEmpForAttendance.value.id,
      month: attendanceMonth.value
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

const changeReportMonth = month => {
  if (!month) return
  reportMonth.value = month
  reportData.value = []
  fetchReport()
}

const fetchReport = async () => {
  const requestId = ++reportRequestId
  const month = reportMonth.value
  reportLoading.value = true
  try {
    const res = await fetchCommissionReport(month)
    if (requestId === reportRequestId && res.data.code === 200) {
      reportData.value = res.data.data
    }
  } catch (error) {
    if (requestId === reportRequestId) ElMessage.error('获取薪资报表失败')
  } finally {
    if (requestId === reportRequestId) reportLoading.value = false
  }
}

const updateLeaveDays = async (row, days) => {
  if (leaveSaving.value) return
  leaveSaving.value = true
  const month = reportMonth.value
  try {
    await recordAttendance({
      employee_id: row.employee_id,
      month,
      leave_days: days
    })
    if (month === reportMonth.value) await fetchReport()
  } catch (error) {
    ElMessage.error('更新考勤失败')
    if (month === reportMonth.value) await fetchReport()
  } finally {
    leaveSaving.value = false
  }
}

const fetchEmployees = async () => {
  loading.value = true
  try {
    const res = await fetchEmployeesApi({ include_departed: true })
    if (res.data.code === 200) {
      employees.value = res.data.data
    }
  } catch (error) {
    ElMessage.error('获取员工列表失败')
  } finally {
    loading.value = false
  }
}

const openOffboarding = row => {
  selectedOffboardingEmployee.value = row
  offboardingVisible.value = true
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
      await updateEmployee(form.value.id, payload)
      ElMessage.success('更新成功')
    } else {
      await createEmployee(payload)
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
      await deleteEmployee(row.id)
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
.people-header{display:flex;align-items:flex-end;justify-content:space-between;gap:20px;margin-bottom:22px}.people-eyebrow{margin:0;color:#55708f;font-size:11px;font-weight:800;letter-spacing:.12em}.people-header h1{margin:4px 0;font-size:30px;letter-spacing:-.04em;color:#17243d}.people-header p:last-child{margin:0;color:#7b8798;font-size:13px}.people-header-actions{display:flex;gap:8px;flex-wrap:wrap}.people-header-actions :deep(.el-button+.el-button){margin-left:0}.people-metrics{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin-bottom:20px}.people-metrics>div{display:flex;flex-direction:column;gap:8px;padding:18px 20px;border:1px solid #e4eaf2;border-radius:15px;background:white}.people-metrics span{color:#748299;font-size:12px}.people-metrics strong{color:#17243d;font-size:25px;letter-spacing:-.04em}.people-metrics small{color:#9aa5b5;font-size:11px}.people-controls{display:flex;align-items:center;gap:12px;margin-bottom:12px}.people-search{max-width:310px;margin-left:auto}.people-count{color:#8490a4;font-size:12px;white-space:nowrap}.mobile-native-date-input{width:100%;min-height:40px;padding:0 12px;border:1px solid #cbd5e1;border-radius:10px;background:#fff;color:#334155;font-size:16px}.mobile-native-month-input{width:auto}@media(max-width:760px){.people-header{align-items:flex-start;flex-direction:column}.people-header-actions{width:100%}.people-header-actions :deep(.el-button){flex:1}.people-metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.people-metrics>div:last-child{grid-column:span 2}.people-controls{flex-wrap:wrap}.people-search{order:3;max-width:none;width:100%;margin-left:0}.people-count{margin-left:auto}}
</style>
