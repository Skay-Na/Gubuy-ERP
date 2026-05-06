<template>
  <div class="space-y-8 animate-in fade-in slide-in-from-bottom-4 duration-700">
    <!-- Header -->
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
      <div>
        <h1 class="text-3xl font-black text-slate-900 tracking-tight">系统设置</h1>
        <p class="text-slate-500 mt-1">管理系统全局配置与底层维护操作</p>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-8">
      <!-- Left Column: General Info -->
      <div class="lg:col-span-2 space-y-8">
        <el-card shadow="never" class="!rounded-[2rem] border-slate-200/60 overflow-hidden relative">
          <div class="absolute top-0 right-0 p-8 opacity-5">
            <el-icon size="120"><Setting /></el-icon>
          </div>
          <h3 class="text-lg font-bold mb-6 flex items-center">
            <div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center mr-3">
              <el-icon><Monitor /></el-icon>
            </div>
            系统信息
          </h3>
          <div class="space-y-4">
            <div class="flex justify-between py-3 border-b border-slate-50">
              <span class="text-slate-500">软件版本</span>
              <span class="font-mono font-bold">v2.0.4-stable</span>
            </div>
            <div class="flex justify-between py-3 border-b border-slate-50">
              <span class="text-slate-500">后端运行环境</span>
              <span class="font-mono text-blue-600">Go 1.21 / Gin / GORM</span>
            </div>
            <div class="flex justify-between py-3 border-b border-slate-50">
              <span class="text-slate-500">前端技术栈</span>
              <span class="font-mono text-indigo-600">Vue 3 / Vite / Tailwind</span>
            </div>
          </div>
        </el-card>

        <el-card shadow="never" class="!rounded-[2rem] border-slate-200/60">
          <h3 class="text-lg font-bold mb-6 flex items-center">
            <div class="w-8 h-8 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center mr-3">
              <el-icon><Download /></el-icon>
            </div>
            数据维护
          </h3>
          <div class="flex items-center justify-between p-6 bg-slate-50 rounded-2xl border border-slate-100">
            <div>
              <p class="font-bold text-slate-800">全量数据备份</p>
              <p class="text-xs text-slate-500 mt-1">导出当前数据库的完整 SQL 备份文件</p>
            </div>
            <el-button 
              type="primary" 
              plain
              class="!rounded-xl font-bold"
              @click="handleDownloadBackup"
            >
              立即下载备份
            </el-button>
          </div>

          <div class="flex items-center justify-between p-6 bg-slate-50 rounded-2xl border border-slate-100 mt-4">
            <div>
              <p class="font-bold text-slate-800">全量数据恢复</p>
              <p class="text-xs text-slate-500 mt-1">从 SQL 备份文件恢复 (会覆盖现有数据)</p>
            </div>
            <el-upload
              action="/api/system/restore"
              :show-file-list="false"
              :on-success="handleRestoreSuccess"
              :on-error="handleRestoreError"
              :before-upload="beforeRestoreUpload"
              accept=".sql"
            >
              <el-button 
                type="warning" 
                plain
                class="!rounded-xl font-bold"
                :loading="restoring"
              >
                上传并恢复
              </el-button>
            </el-upload>
          </div>
        </el-card>

        <el-card shadow="never" class="!rounded-[2rem] border-slate-200/60">
          <h3 class="text-lg font-bold mb-6 flex items-center">
            <div class="w-8 h-8 rounded-lg bg-emerald-50 text-emerald-600 flex items-center justify-center mr-3">
              <el-icon><Lock /></el-icon>
            </div>
            安全策略
          </h3>
          <div class="p-6 bg-slate-50 rounded-2xl border border-slate-100">
            <p class="text-sm text-slate-600 leading-relaxed">
              当前系统采用本地密码验证模式。为了保证数据安全，建议定期修改管理员密码，并确保数据库连接字符串已加密或通过环境变量管理。
            </p>
          </div>
        </el-card>
      </div>

      <!-- Right Column: Danger Zone -->
      <div class="space-y-8">
        <el-card shadow="never" class="!rounded-[2rem] border-red-100 bg-red-50/30 overflow-hidden relative">
          <div class="absolute -right-4 -bottom-4 opacity-10 text-red-600 rotate-12">
            <el-icon size="100"><Warning /></el-icon>
          </div>
          
          <h3 class="text-lg font-bold text-red-600 mb-6 flex items-center">
            <div class="w-8 h-8 rounded-lg bg-red-100 text-red-600 flex items-center justify-center mr-3">
              <el-icon><Delete /></el-icon>
            </div>
            危险区域
          </h3>
          
          <div class="space-y-4 relative z-10">
            <p class="text-xs text-red-500 font-medium uppercase tracking-wider">该操作不可逆</p>
            <p class="text-sm text-slate-600 leading-relaxed">
              <strong>恢复出厂设置</strong> 将清空所有业务数据（包括订单、商品、财务记录及管理员账号）。执行后系统将返回初始状态。
            </p>
            
            <el-button 
              type="danger" 
              class="w-full !rounded-xl !h-12 font-bold shadow-lg shadow-red-500/20 mt-4"
              @click="showResetConfirm = true"
            >
              开始恢复出厂设置
            </el-button>
          </div>
        </el-card>
      </div>
    </div>

    <!-- Reset Confirmation Dialog -->
    <el-dialog
      v-model="showResetConfirm"
      title="确定要恢复出厂设置吗？"
      width="400px"
      center
      destroy-on-close
      class="rounded-[2rem]"
    >
      <div class="py-2">
        <div class="p-4 bg-red-50 rounded-2xl border border-red-100 mb-6">
          <p class="text-xs text-red-600 leading-relaxed">
            警告：此操作将永久删除数据库中的所有记录。如果你确认要继续，请在下方输入 <span class="font-black font-mono">RESET</span>。
          </p>
        </div>
        
        <el-input 
          v-model="confirmText" 
          placeholder="请输入 RESET" 
          size="large"
          class="mb-6"
        />

        <div class="flex gap-4">
          <el-button 
            class="flex-1 !rounded-xl !h-12 font-bold"
            @click="showResetConfirm = false"
          >
            取消
          </el-button>
          <el-button 
            type="danger" 
            class="flex-1 !rounded-xl !h-12 font-bold shadow-lg shadow-red-500/20"
            :disabled="confirmText !== 'RESET'"
            :loading="resetting"
            @click="handleFactoryReset"
          >
            确认重置
          </el-button>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { Setting, Monitor, Lock, Warning, Delete, Download } from '@element-plus/icons-vue'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'

const router = useRouter()
const showResetConfirm = ref(false)
const confirmText = ref('')
const resetting = ref(false)
const restoring = ref(false)

const handleDownloadBackup = () => {
  // 直接利用浏览器下载功能访问接口
  window.open('/api/system/backup', '_blank')
  ElMessage.success('备份导出请求已发送')
}

const beforeRestoreUpload = (file) => {
  return new Promise((resolve, reject) => {
    ElMessageBox.confirm(
      '警告：从备份恢复将彻底覆盖当前所有业务数据！此操作无法撤销。建议在恢复前先导出当前数据的备份。',
      '确认恢复数据',
      {
        confirmButtonText: '我明白风险，开始恢复',
        cancelButtonText: '取消',
        type: 'warning',
        confirmButtonClass: 'el-button--danger',
        roundButton: true
      }
    ).then(() => {
      restoring.value = true
      resolve(true)
    }).catch(() => {
      reject(false)
    })
  })
}

const handleRestoreSuccess = (res) => {
  restoring.value = false
  if (res.code === 200) {
    ElMessage.success({
      message: '数据恢复成功！系统即将刷新...',
      duration: 3000,
      onClose: () => {
        window.location.reload()
      }
    })
  } else {
    ElMessage.error(res.msg || '恢复失败')
  }
}

const handleRestoreError = () => {
  restoring.value = false
  ElMessage.error('上传恢复失败，请检查文件格式或网络连接')
}

const handleFactoryReset = async () => {
  if (confirmText.value !== 'RESET') return

  resetting.value = true
  try {
    const res = await axios.post('/api/system/reset', {
      confirm: confirmText.value
    })
    if (res.data.code === 200) {
      ElMessage.success('系统已恢复出厂设置，即将跳转...')
      setTimeout(() => {
        window.location.href = '/' // 强制刷新并回到入口页
      }, 2000)
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '操作失败')
  } finally {
    resetting.value = false
  }
}
</script>

<style scoped>
:deep(.el-card) {
  border: 1px solid rgba(226, 232, 240, 0.6);
}
</style>
