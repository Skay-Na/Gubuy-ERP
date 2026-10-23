import { shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import request from '../utils/request'

export function useOperationHistory() {
  const operations = shallowRef([])
  const loading = shallowRef(false)
  const undoingId = shallowRef(null)

  async function loadOperations() {
    loading.value = true
    try {
      const response = await request.get('/api/operations')
      operations.value = response.data?.data || []
    } catch {
      operations.value = []
    } finally {
      loading.value = false
    }
  }

  async function undoOperation(operation) {
    undoingId.value = operation.id
    try {
      await request.post(`/api/operations/${operation.id}/undo`)
      ElMessage.success('操作已撤销')
      await loadOperations()
      return true
    } catch {
      ElMessage.error(error.response?.data?.msg || '撤销失败，请刷新后重试')
      return false
    } finally {
      undoingId.value = null
    }
  }

  return {
    operations,
    loading,
    undoingId,
    loadOperations,
    undoOperation
  }
}
