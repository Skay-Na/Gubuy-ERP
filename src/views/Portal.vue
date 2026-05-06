<template>
  <div class="min-h-screen w-full bg-slate-900 flex items-center justify-center p-6 font-sans">
    <div class="max-w-4xl w-full grid md:grid-cols-2 gap-8">
      <!-- Admin Portal Card -->
      <div 
        @click="goToAdmin"
        class="group relative bg-slate-800 border border-slate-700 p-10 rounded-[2rem] shadow-2xl hover:bg-slate-700/50 hover:border-blue-500/50 transition-all cursor-pointer overflow-hidden"
      >
        <div class="absolute -right-10 -top-10 w-40 h-40 bg-blue-600/10 rounded-full blur-3xl group-hover:bg-blue-600/20 transition-all"></div>
        <div class="relative z-10">
          <div class="w-16 h-16 bg-blue-600 rounded-2xl flex items-center justify-center mb-6 shadow-lg shadow-blue-500/20 group-hover:scale-110 transition-transform">
            <el-icon size="32" color="white"><Management /></el-icon>
          </div>
          <h2 class="text-3xl font-black text-white mb-3 tracking-tight">管理后台</h2>
          <p class="text-slate-400 text-sm leading-relaxed mb-8">
            深度数据分析、库存全口径调拨、财务盈亏审计以及员工薪资管理。建议在办公室电脑端使用。
          </p>
          <div class="flex items-center text-blue-400 font-bold text-sm">
            进入管理系统 <el-icon class="ml-2 group-hover:translate-x-1 transition-transform"><Right /></el-icon>
          </div>
        </div>
      </div>

      <!-- Staff Terminal Card -->
      <div 
        @click="goToTerminal"
        class="group relative bg-slate-800 border border-slate-700 p-10 rounded-[2rem] shadow-2xl hover:bg-slate-700/50 hover:border-indigo-500/50 transition-all cursor-pointer overflow-hidden"
      >
        <div class="absolute -right-10 -top-10 w-40 h-40 bg-indigo-600/10 rounded-full blur-3xl group-hover:bg-indigo-600/20 transition-all"></div>
        <div class="relative z-10">
          <div class="w-16 h-16 bg-indigo-600 rounded-2xl flex items-center justify-center mb-6 shadow-lg shadow-indigo-500/20 group-hover:scale-110 transition-transform">
            <el-icon size="32" color="white"><Monitor /></el-icon>
          </div>
          <h2 class="text-3xl font-black text-white mb-3 tracking-tight">收银终端</h2>
          <p class="text-slate-400 text-sm leading-relaxed mb-8">
            专为门店营业员设计。快速查货、移动收银、自动计提成。适配平板与移动设备，极简操作流。
          </p>
          <div class="flex items-center text-indigo-400 font-bold text-sm">
            开启营业模式 <el-icon class="ml-2 group-hover:translate-x-1 transition-transform"><Right /></el-icon>
          </div>
        </div>
      </div>
    </div>

    <!-- Branding Footer -->
    <div class="absolute bottom-10 left-0 w-full text-center">
      <div class="text-slate-600 text-xs font-mono tracking-widest uppercase">
        WONCHON ERP & POS SYSTEM v2.0
      </div>
    </div>

    <!-- Admin Login Dialog -->
    <el-dialog
      v-model="adminDialogVisible"
      title="管理员身份验证"
      width="360px"
      center
      destroy-on-close
      class="admin-login-dialog"
    >
      <div class="py-4">
        <p class="text-center text-slate-500 text-sm mb-6">请输入管理员登录密码以继续</p>
        <el-input 
          v-model="adminPassword" 
          type="password" 
          placeholder="登录密码" 
          show-password 
          size="large"
          @keyup.enter="handleAdminLogin"
          class="mb-4"
        />
        <el-button 
          type="primary" 
          size="large" 
          class="w-full !rounded-xl !h-12 font-bold shadow-lg shadow-blue-500/20" 
          :loading="loggingIn"
          @click="handleAdminLogin"
        >
          立即进入
        </el-button>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Management, Monitor, Right } from '@element-plus/icons-vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'

const router = useRouter()
const adminDialogVisible = ref(false)
const adminPassword = ref('')
const loggingIn = ref(false)

const goToAdmin = () => {
  adminDialogVisible.value = true
  adminPassword.value = ''
}

const handleAdminLogin = async () => {
  if (!adminPassword.value) return
  
  loggingIn.value = true
  try {
    const res = await axios.post('/api/admin/verify', {
      password: adminPassword.value
    })
    if (res.data.code === 200) {
      ElMessage.success('验证成功')
      adminDialogVisible.value = false
      router.push('/admin')
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.msg || '验证失败，请检查密码')
  } finally {
    loggingIn.value = false
  }
}

const goToTerminal = () => {
  router.push('/terminal')
}
</script>

<style scoped>
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;700;900&display=swap');
.font-sans {
  font-family: 'Inter', system-ui, -apple-system, sans-serif;
}
</style>
