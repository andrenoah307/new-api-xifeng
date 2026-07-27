import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Download, RefreshCw } from 'lucide-react'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Progress } from '@/components/ui/progress'
import { formatTimestampToDate } from '@/lib/format'
import { getCommonHeaders } from '@/lib/api'
import { getUserExportTasks, getExportDownloadUrl } from '../api'
import type { ExportTask } from '../types'

interface ExportTasksSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const STATUS_CONFIG: Record<
  number,
  { label: string; variant: StatusVariant; pulse?: boolean }
> = {
  0: { label: 'Pending', variant: 'warning', pulse: true },
  1: { label: 'Processing', variant: 'blue', pulse: true },
  2: { label: 'Completed', variant: 'success' },
  3: { label: 'Failed', variant: 'danger' },
  4: { label: 'Cancelled', variant: 'grey' },
}

function formatFileSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}

function TaskItem({ task }: { task: ExportTask }) {
  const { t } = useTranslation()
  const config = STATUS_CONFIG[task.status] ?? STATUS_CONFIG[4]

  const handleDownload = async () => {
    try {
      const res = await fetch(getExportDownloadUrl(task.id), {
        credentials: 'include',
        headers: getCommonHeaders(),
      })
      if (!res.ok) throw new Error(res.statusText)
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${task.username || 'export'}-${formatTimestampToDate(task.created_time).replaceAll('-', '').replaceAll(':', '').replace(' ', '-')}.csv.gz`
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
    } catch {
      // silent fail — user sees nothing happen
    }
  }

  return (
    <div className='border rounded-lg p-3 space-y-2'>
      <div className='flex items-center justify-between'>
        <StatusBadge
          label={t(config.label)}
          variant={config.variant}
          pulse={config.pulse}
          copyable={false}
        />
        <span className='text-xs text-muted-foreground'>
          {formatTimestampToDate(task.created_time)}
        </span>
      </div>

      {/* Progress bar for processing tasks */}
      {task.status === 1 && (
        <Progress value={task.progress}>
          <span className='text-xs text-muted-foreground'>
            {task.progress}%
          </span>
        </Progress>
      )}

      {/* Task details */}
      <div className='text-xs text-muted-foreground space-y-0.5'>
        {task.email && (
          <div>{t('Email')}: {task.email}</div>
        )}
        {task.status === 2 && (
          <>
            {task.row_count > 0 && (
              <div>{t('Rows')}: {task.row_count.toLocaleString()}</div>
            )}
            {task.file_size > 0 && (
              <div>{t('File Size')}: {formatFileSize(task.file_size)}</div>
            )}
            {task.completed_time > 0 && (
              <div>{t('Completed')}: {formatTimestampToDate(task.completed_time)}</div>
            )}
          </>
        )}
        {task.status === 3 && task.error_message && (
          <div className='text-destructive'>{task.error_message}</div>
        )}
      </div>

      {/* Download button for completed tasks */}
      {task.status === 2 && (
        <Button
          variant='outline'
          size='sm'
          className='w-full'
          onClick={handleDownload}
        >
          <Download className='mr-1.5 h-3.5 w-3.5' />
          {t('Download')}
        </Button>
      )}
    </div>
  )
}

export function ExportTasksSheet({
  open,
  onOpenChange,
}: ExportTasksSheetProps) {
  const { t } = useTranslation()

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['export-tasks'],
    queryFn: () => getUserExportTasks(1, 20),
    enabled: open,
    refetchInterval: open ? 10000 : false,
  })

  const tasks = data?.data?.items ?? []

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side='right' className='sm:max-w-md'>
        <SheetHeader>
          <div className='flex items-center justify-between pr-8'>
            <SheetTitle>{t('Export Tasks')}</SheetTitle>
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => refetch()}
              disabled={isLoading}
              aria-label={t('Refresh')}
            >
              <RefreshCw className={isLoading ? 'animate-spin' : ''} />
            </Button>
          </div>
          <SheetDescription>
            {t('View and download your offline export tasks')}
          </SheetDescription>
        </SheetHeader>

        <div className='flex-1 overflow-y-auto px-4 pb-4 space-y-3'>
          {isLoading && tasks.length === 0 && (
            <div className='text-center text-sm text-muted-foreground py-8'>
              {t('Loading...')}
            </div>
          )}

          {!isLoading && tasks.length === 0 && (
            <div className='text-center text-sm text-muted-foreground py-8'>
              {t('No export tasks')}
            </div>
          )}

          {tasks.map((task) => (
            <TaskItem key={task.id} task={task} />
          ))}
        </div>
      </SheetContent>
    </Sheet>
  )
}
