import axios from 'axios'

// 动态获取当前访问的域名/IP，并拼接后端端口 8080
const baseURL = `http://${window.location.hostname}:8080`

const service = axios.create({
  baseURL,
  timeout: 10000
})

export default service
