import { ElMessage } from 'element-plus'

const baseURL = import.meta.env.VITE_API_BASE_URL || ''

export async function downloadExcel(url, params = {}) {
  try {
    const token = localStorage.getItem('erp_token') || localStorage.getItem('pos_token')
    if (!token) {
      ElMessage.error('未登录或登录已过期')
      return
    }

    // Build query string, ignore empty values
    const cleanedParams = Object.fromEntries(
      Object.entries(params).filter(([_, v]) => v !== null && v !== undefined && v !== '')
    )
    const queryString = new URLSearchParams(cleanedParams).toString()
    const fullUrl = baseURL + url + (queryString ? `?${queryString}` : '')

    ElMessage.info('正在生成导出文件，请稍候...')

    // Fetch API
    const response = await fetch(fullUrl, {
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${token}`
      }
    })

    if (!response.ok) {
      const errorData = await response.json().catch(() => ({}))
      throw new Error(errorData.msg || `导出失败 (${response.status})`)
    }

    // Extract filename from Content-Disposition
    const disposition = response.headers.get('content-disposition')
    let filename = 'export.xlsx'
    if (disposition) {
      const utf8Matches = /filename\*=utf-8''([^;\n]*)/i.exec(disposition)
      if (utf8Matches && utf8Matches[1]) {
        filename = decodeURIComponent(utf8Matches[1])
      } else {
        const matches = /filename[^;=\n]*=((['"]).*?\2|[^;\n]*)/i.exec(disposition)
        if (matches && matches[1]) {
          filename = matches[1].replace(/['"]/g, '')
        }
      }
    }

    const blob = await response.blob()
    const downloadUrl = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = downloadUrl
    link.download = filename
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(downloadUrl)
    ElMessage.success('导出成功')
  } catch (error) {
    console.error('Export error:', error)
    ElMessage.error(error.message || '导出失败，请检查网络或联系管理员')
  }
}
