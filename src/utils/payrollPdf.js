import html2canvas from 'html2canvas'
import { jsPDF } from 'jspdf'

const safeFilename = value => value.replace(/[\\/:*?"<>|]/g, '-').replace(/\s+/g, '-')

export const payrollPdfFilename = (employee, month) => {
  const name = employee?.employee_name || '员工'
  return safeFilename(`${month || '本月'}-${name}-薪资明细.pdf`)
}

export async function createPayrollPdf(pageElements) {
  if (!pageElements.length) throw new Error('薪资明细尚未准备完成')

  const pdf = new jsPDF({ orientation: 'portrait', unit: 'mm', format: 'a4', compress: true })
  const pageWidth = pdf.internal.pageSize.getWidth()
  const pageHeight = pdf.internal.pageSize.getHeight()

  for (let index = 0; index < pageElements.length; index += 1) {
    const renderTarget = pageElements[index].cloneNode(true)
    Object.assign(renderTarget.style, {
      position: 'fixed',
      left: '0',
      top: '0',
      zIndex: '-2147483647',
      pointerEvents: 'none'
    })
    document.body.appendChild(renderTarget)
    let canvas
    try {
      canvas = await html2canvas(renderTarget, {
        backgroundColor: '#ffffff',
        scale: 2,
        useCORS: true,
        logging: false,
        scrollX: 0,
        scrollY: 0
      })
    } finally {
      renderTarget.remove()
    }
    if (index > 0) pdf.addPage('a4', 'portrait')
    pdf.addImage(canvas.toDataURL('image/jpeg', 0.94), 'JPEG', 0, 0, pageWidth, pageHeight, undefined, 'FAST')
  }

  return pdf.output('blob')
}

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export async function sharePayrollPdf({ blob, filename, employee, month }) {
  const file = new File([blob], filename, { type: 'application/pdf' })
  const shareData = {
    files: [file],
    title: `${employee.employee_name} ${month} 薪资明细`,
    text: `${employee.employee_name} ${month} 薪资明细 PDF`
  }
  const supportsFileShare = typeof navigator.share === 'function' &&
    (typeof navigator.canShare !== 'function' || navigator.canShare(shareData))

  if (supportsFileShare) {
    try {
      await navigator.share(shareData)
      return 'shared'
    } catch (error) {
      if (error?.name === 'AbortError') return 'cancelled'
      // 系统分享不可用或调用失败时仍保留文件，避免用户重复生成。
    }
  }

  downloadBlob(blob, filename)
  return 'downloaded'
}
