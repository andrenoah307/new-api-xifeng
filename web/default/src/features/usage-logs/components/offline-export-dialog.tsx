import { useTranslation } from 'react-i18next'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { formatTimestampToDate } from '@/lib/format'
import { submitOfflineExport } from '../api'
import type { CommonLogFilters } from '../types'

interface OfflineExportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  filters: CommonLogFilters
  logType?: string
}

export function OfflineExportDialog({
  open,
  onOpenChange,
  filters,
  logType,
}: OfflineExportDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const userEmail = useAuthStore((s) => s.auth.user?.email ?? '')

  const mutation = useMutation({
    mutationFn: submitOfflineExport,
    onSuccess: (data) => {
      if (data.success) {
        toast.success(t('Task submitted, you will be notified by email when complete'))
        queryClient.invalidateQueries({ queryKey: ['export-tasks'] })
        onOpenChange(false)
      } else {
        toast.error(data.message || t('Export failed'))
      }
    },
    onError: () => {
      toast.error(t('Export failed'))
    },
  })

  const handleSubmit = () => {
    if (!filters.startTime || !filters.endTime) {
      toast.error(t('Please select a time range'))
      return
    }

    const startTs = Math.floor(filters.startTime.getTime() / 1000)
    const endTs = Math.floor(filters.endTime.getTime() / 1000)

    mutation.mutate({
      filters: {
        start_timestamp: startTs,
        end_timestamp: endTs,
        ...(filters.model ? { model_name: filters.model } : {}),
        ...(filters.token ? { token_name: filters.token } : {}),
        ...(filters.channel ? { channel_id: Number(filters.channel) } : {}),
      },
    })
  }

  const startLabel = filters.startTime
    ? formatTimestampToDate(Math.floor(filters.startTime.getTime() / 1000))
    : '-'
  const endLabel = filters.endTime
    ? formatTimestampToDate(Math.floor(filters.endTime.getTime() / 1000))
    : '-'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Offline Export')}</DialogTitle>
          <DialogDescription>
            {t('Export logs in the background and receive the file by email')}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
          {/* Filter summary */}
          <div className='bg-muted rounded-lg p-3 text-sm space-y-1'>
            <div className='flex justify-between'>
              <span className='text-muted-foreground'>{t('Time Range')}</span>
              <span>{startLabel} ~ {endLabel}</span>
            </div>
            {filters.model && (
              <div className='flex justify-between'>
                <span className='text-muted-foreground'>{t('Model Name')}</span>
                <span>{filters.model}</span>
              </div>
            )}
            {filters.token && (
              <div className='flex justify-between'>
                <span className='text-muted-foreground'>{t('Token Name')}</span>
                <span>{filters.token}</span>
              </div>
            )}
            {filters.channel && (
              <div className='flex justify-between'>
                <span className='text-muted-foreground'>{t('Channel ID')}</span>
                <span>{filters.channel}</span>
              </div>
            )}
            {logType && (
              <div className='flex justify-between'>
                <span className='text-muted-foreground'>{t('Log Type')}</span>
                <span>{logType}</span>
              </div>
            )}
          </div>

          {/* Email info */}
          <div className='text-sm'>
            <span className='text-muted-foreground'>{t('Notification Email')}: </span>
            <span className='font-medium'>{userEmail || t('No email bound, please set in account settings')}</span>
          </div>

          {/* Limits info */}
          <p className='text-xs text-muted-foreground'>
            {t('Offline export limits: one task per 24 hours, results available for 72 hours after completion')}
          </p>
        </div>

        <DialogFooter>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={mutation.isPending}
          >
            {mutation.isPending ? t('Submitting...') : t('Submit Export Task')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
