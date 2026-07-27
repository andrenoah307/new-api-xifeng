import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Download, XCircle } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Separator } from '@/components/ui/separator'
import { StatusBadge } from '@/components/status-badge'
import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { DataTablePagination } from '@/components/data-table/core/pagination'
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { formatTimestamp } from '@/lib/format'
import { getCommonHeaders } from '@/lib/api'
import {
  getAdminExportTasks,
  getExportQueueStats,
  getExportConfig,
  saveExportConfig,
  cancelExportTask,
  getAdminExportDownloadUrl,
  EXPORT_STATUS,
  type ExportConfig,
  type ExportTask,
} from '../admin-export-api'

// ============================================================================
// Helpers
// ============================================================================

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  const val = bytes / Math.pow(1024, i)
  return `${val.toFixed(i > 0 ? 1 : 0)} ${units[i]}`
}

function formatExportRange(filtersJson: string): string {
  if (!filtersJson) return '-'
  try {
    const f = JSON.parse(filtersJson)
    const s = f.start_timestamp ? formatTimestamp(f.start_timestamp) : ''
    const e = f.end_timestamp ? formatTimestamp(f.end_timestamp) : ''
    if (!s && !e) return '-'
    return `${s} ~ ${e}`
  } catch {
    return '-'
  }
}

type StatusVariant = 'warning' | 'info' | 'success' | 'danger' | 'neutral'

function getStatusVariant(status: number): StatusVariant {
  switch (status) {
    case EXPORT_STATUS.PENDING:
      return 'warning'
    case EXPORT_STATUS.PROCESSING:
      return 'info'
    case EXPORT_STATUS.DONE:
      return 'success'
    case EXPORT_STATUS.FAILED:
      return 'danger'
    case EXPORT_STATUS.CANCELLED:
      return 'neutral'
    default:
      return 'neutral'
  }
}

function getStatusLabel(status: number): string {
  switch (status) {
    case EXPORT_STATUS.PENDING:
      return 'Pending'
    case EXPORT_STATUS.PROCESSING:
      return 'Processing'
    case EXPORT_STATUS.DONE:
      return 'Done'
    case EXPORT_STATUS.FAILED:
      return 'Failed'
    case EXPORT_STATUS.CANCELLED:
      return 'Cancelled'
    default:
      return 'Unknown'
  }
}

// ============================================================================
// Query Keys
// ============================================================================

const exportQueryKeys = {
  all: ['admin-export'] as const,
  tasks: (page: number) => ['admin-export', 'tasks', page] as const,
  stats: () => ['admin-export', 'stats'] as const,
  config: () => ['admin-export', 'config'] as const,
}

// ============================================================================
// Component
// ============================================================================

export function AdminExportTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [taskPage, setTaskPage] = useState(1)
  const [cancelOpen, setCancelOpen] = useState(false)
  const [cancellingTask, setCancellingTask] = useState<ExportTask | null>(null)

  // --- Queue Stats ---
  const { data: stats } = useQuery({
    queryKey: exportQueryKeys.stats(),
    queryFn: getExportQueueStats,
    refetchInterval: 15000,
  })

  // --- Config ---
  const { data: configData } = useQuery({
    queryKey: exportQueryKeys.config(),
    queryFn: getExportConfig,
  })

  const [localConfig, setLocalConfig] = useState<ExportConfig | null>(null)
  useEffect(() => {
    if (configData) setLocalConfig(configData)
  }, [configData])

  const configMutation = useMutation({
    mutationFn: (cfg: ExportConfig) => saveExportConfig(cfg),
    onSuccess: () => {
      toast.success(t('Config saved'))
      queryClient.invalidateQueries({ queryKey: exportQueryKeys.all })
    },
  })

  // --- Tasks ---
  const { data: tasksData, isLoading: tasksLoading } = useQuery({
    queryKey: exportQueryKeys.tasks(taskPage),
    queryFn: () => getAdminExportTasks(taskPage, 10),
    placeholderData: (prev) => prev,
  })

  const tasks = tasksData?.items ?? []
  const taskTotal = tasksData?.total ?? 0

  const taskTable = useReactTable({
    data: tasks,
    columns: [],
    pageCount: Math.ceil(taskTotal / 10),
    state: { pagination: { pageIndex: taskPage - 1, pageSize: 10 } },
    onPaginationChange: (updater) => {
      const next =
        typeof updater === 'function'
          ? updater({ pageIndex: taskPage - 1, pageSize: 10 })
          : updater
      setTaskPage(next.pageIndex + 1)
    },
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
  })

  const cancelMutation = useMutation({
    mutationFn: (taskId: number) => cancelExportTask(taskId),
    onSuccess: () => {
      toast.success(t('Task cancelled'))
      setCancelOpen(false)
      queryClient.invalidateQueries({ queryKey: exportQueryKeys.all })
    },
  })

  return (
    <div className="space-y-6">
      {/* Queue Stats */}
      {stats && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">{t('Queue Stats')}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 gap-2 text-sm sm:grid-cols-4">
              <div>
                <span className="text-muted-foreground">
                  {t('Queue Depth')}:{' '}
                </span>
                {stats.queue_depth} / {stats.queue_capacity}
              </div>
              <div>
                <span className="text-muted-foreground">
                  {t('Workers')}:{' '}
                </span>
                {stats.worker_count}
              </div>
              <div>
                <span className="text-muted-foreground">
                  {t('Storage')}:{' '}
                </span>
                {formatBytes(stats.total_storage_bytes)}
              </div>
              <div>
                <span className="text-muted-foreground">
                  {t('Pending')}:{' '}
                </span>
                {stats.pending_count}
                <span className="text-muted-foreground ml-2">
                  {t('Processing')}:{' '}
                </span>
                {stats.processing_count}
              </div>
              <div>
                <span className="text-muted-foreground">
                  {t('Done')}:{' '}
                </span>
                {stats.done_count}
              </div>
              <div>
                <span className="text-muted-foreground">
                  {t('Failed')}:{' '}
                </span>
                {stats.failed_count}
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Config Panel */}
      {localConfig && (
        <Card>
          <CardHeader>
            <CardTitle>{t('Export Config')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <div className="flex items-center gap-3">
                <Label>{t('Enabled')}</Label>
                <Switch
                  checked={localConfig.enabled}
                  onCheckedChange={(v) =>
                    setLocalConfig((p) => p && { ...p, enabled: v })
                  }
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Worker Count')}</Label>
                <Input
                  type="number"
                  value={localConfig.worker_count}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && { ...p, worker_count: Number(e.target.value) }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Queue Size')}</Label>
                <Input
                  type="number"
                  value={localConfig.queue_size}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && { ...p, queue_size: Number(e.target.value) }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Max File Size (MB)')}</Label>
                <Input
                  type="number"
                  value={localConfig.max_file_size_mb}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && { ...p, max_file_size_mb: Number(e.target.value) }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Max Total Storage (MB)')}</Label>
                <Input
                  type="number"
                  value={localConfig.max_total_storage_mb}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && {
                        ...p,
                        max_total_storage_mb: Number(e.target.value),
                      }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('User Cooldown Hours')}</Label>
                <Input
                  type="number"
                  value={localConfig.user_cooldown_hours}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && {
                        ...p,
                        user_cooldown_hours: Number(e.target.value),
                      }
                    )
                  }
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label>{t('Retention Hours')}</Label>
                <Input
                  type="number"
                  value={localConfig.retention_hours}
                  onChange={(e) =>
                    setLocalConfig((p) =>
                      p && { ...p, retention_hours: Number(e.target.value) }
                    )
                  }
                  className="h-8"
                />
              </div>
            </div>
            <Separator />
            <Button
              size="sm"
              onClick={() => localConfig && configMutation.mutate(localConfig)}
              disabled={configMutation.isPending}
            >
              {t('Save')}
            </Button>
          </CardContent>
        </Card>
      )}

      {/* Task Table */}
      <Card>
        <CardHeader>
          <CardTitle>{t('Export Tasks')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('ID')}</TableHead>
                  <TableHead>{t('Username')}</TableHead>
                  <TableHead>{t('Export Range')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Progress')}</TableHead>
                  <TableHead>{t('Row Count')}</TableHead>
                  <TableHead>{t('File Size')}</TableHead>
                  <TableHead>{t('Created Time')}</TableHead>
                  <TableHead>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tasksLoading && tasks.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={9} className="text-center py-8">
                      {t('Loading...')}
                    </TableCell>
                  </TableRow>
                ) : tasks.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={9}
                      className="text-center py-8 text-muted-foreground"
                    >
                      {t('No data')}
                    </TableCell>
                  </TableRow>
                ) : (
                  tasks.map((task) => (
                    <TableRow key={task.id}>
                      <TableCell className="font-mono text-xs">
                        {task.id}
                      </TableCell>
                      <TableCell>{task.username || '-'}</TableCell>
                      <TableCell className="text-xs">
                        {formatExportRange(task.filters)}
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          variant={getStatusVariant(task.status)}
                          pulse={task.status === EXPORT_STATUS.PROCESSING}
                        >
                          {t(getStatusLabel(task.status))}
                        </StatusBadge>
                      </TableCell>
                      <TableCell className="text-xs">
                        {task.status === EXPORT_STATUS.PROCESSING
                          ? `${task.progress}%`
                          : '-'}
                      </TableCell>
                      <TableCell className="text-xs">
                        {task.row_count > 0 ? task.row_count.toLocaleString() : '-'}
                      </TableCell>
                      <TableCell className="text-xs">
                        {task.file_size > 0 ? formatBytes(task.file_size) : '-'}
                      </TableCell>
                      <TableCell className="text-xs">
                        {formatTimestamp(task.created_time)}
                      </TableCell>
                      <TableCell>
                        <div className="flex gap-1">
                          {task.status === EXPORT_STATUS.DONE && (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-7 w-7"
                              onClick={async () => {
                                try {
                                  const res = await fetch(getAdminExportDownloadUrl(task.id), {
                                    credentials: 'include',
                                    headers: getCommonHeaders(),
                                  })
                                  if (!res.ok) throw new Error(res.statusText)
                                  const blob = await res.blob()
                                  const url = URL.createObjectURL(blob)
                                  const a = document.createElement('a')
                                  a.href = url
                                  a.download = `${task.username || 'export'}-${formatTimestamp(task.created_time).replaceAll('-', '').replaceAll(':', '').replace(' ', '-')}.csv.gz`
                                  document.body.appendChild(a)
                                  a.click()
                                  a.remove()
                                  URL.revokeObjectURL(url)
                                } catch {
                                  toast.error(t('Download failed'))
                                }
                              }}
                              title={t('Download')}
                            >
                              <Download className="h-3.5 w-3.5" />
                            </Button>
                          )}
                          {(task.status === EXPORT_STATUS.PENDING ||
                            task.status === EXPORT_STATUS.PROCESSING) && (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-7 w-7"
                              onClick={() => {
                                setCancellingTask(task)
                                setCancelOpen(true)
                              }}
                              title={t('Cancel')}
                            >
                              <XCircle className="h-3.5 w-3.5" />
                            </Button>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
          <DataTablePagination table={taskTable} />
        </CardContent>
      </Card>

      {/* Cancel Confirm Dialog */}
      <ConfirmDialog
        open={cancelOpen}
        onOpenChange={setCancelOpen}
        title={t('Cancel Export Task')}
        desc={`${t('Task ID')}: ${cancellingTask?.id ?? '-'}`}
        destructive
        handleConfirm={() =>
          cancellingTask && cancelMutation.mutate(cancellingTask.id)
        }
        isLoading={cancelMutation.isPending}
      />
    </div>
  )
}
