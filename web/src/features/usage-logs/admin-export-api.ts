import { api } from '@/lib/api'

// ============================================================================
// Types
// ============================================================================

export interface ExportTask {
  id: number
  user_id: number
  username: string
  email: string
  status: number
  filters: string
  file_path: string
  file_size: number
  row_count: number
  progress: number
  error_message: string
  created_time: number
  started_time: number
  completed_time: number
}

export interface ExportQueueStats {
  queue_depth: number
  queue_capacity: number
  worker_count: number
  total_storage_bytes: number
  pending_count: number
  processing_count: number
  done_count: number
  failed_count: number
}

export interface ExportConfig {
  enabled: boolean
  worker_count: number
  queue_size: number
  max_file_size_mb: number
  max_total_storage_mb: number
  user_cooldown_hours: number
  retention_hours: number
}

// ============================================================================
// Export Task Status Constants
// ============================================================================

export const EXPORT_STATUS = {
  PENDING: 0,
  PROCESSING: 1,
  DONE: 2,
  FAILED: 3,
  CANCELLED: 4,
} as const

// ============================================================================
// Admin Export Management APIs
// ============================================================================

export async function getAdminExportTasks(page = 1, pageSize = 10) {
  const res = await api.get(
    `/api/log/export-tasks?page=${page}&page_size=${pageSize}`
  )
  return res.data?.data as { items: ExportTask[]; total: number }
}

export async function getExportQueueStats() {
  const res = await api.get('/api/log/export-queue-stats')
  return res.data?.data as ExportQueueStats
}

export async function getExportConfig() {
  const res = await api.get('/api/log/export-config')
  return res.data?.data as ExportConfig
}

export async function saveExportConfig(config: ExportConfig) {
  const res = await api.put('/api/log/export-config', config)
  return res.data
}

export async function cancelExportTask(taskId: number) {
  const res = await api.post(`/api/log/export-tasks/${taskId}/cancel`)
  return res.data
}

export function getAdminExportDownloadUrl(taskId: number): string {
  return `/api/log/export-tasks/${taskId}/download`
}
