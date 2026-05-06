import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import pinia from './store'

// Element Plus
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'

// Tailwind
import './style.css'

// Axios Global Config
import axios from 'axios'
axios.defaults.baseURL = `http://${window.location.hostname}:8080`

const app = createApp(App)

// Register all icons
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(pinia)
app.use(router)
app.use(ElementPlus, {
  locale: zhCn,
})

app.mount('#app')
