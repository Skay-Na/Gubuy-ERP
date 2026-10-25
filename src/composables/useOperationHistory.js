import { shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import { fetchOperations as fetchOperationsApi, undoOperation as undoOperationApi } from '../api/system'

export function useOperationHistory() {
  const operations = shallowRef([])
  const loading = shallowRef(false)
  const undoingId = shallowRef(null)

  async function loadOperations() {
    loading.value = true
    try {
      const response = await fetchOperationsApi()
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
      await undoOperationApi(operation.id)
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
