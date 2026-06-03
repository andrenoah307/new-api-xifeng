import { useState, useEffect } from 'react'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { formatTimestampToDate } from '@/lib/format'
import { submitOfflineExport } from '../api'
import type { CommonLogFilters } from '../types'

interface OfflineExportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  filters: CommonLogFilters
  logType?: string
}

function formatCooldownRemaining(nextAvailableTime: number): string {
  const remaining = Math.max(0, nextAvailableTime - Math.floor(Date.now() / 1000))
  if (remaining <= 0) return ''
  const minutes = Math.floor(remaining / 60)
  const seconds = remaining % 60
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${seconds}s`
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

  const [email, setEmail] = useState(userEmail)
  const [cooldownUntil, setCooldownUntil] = useState<number | null>(null)
  const [cooldownDisplay, setCooldownDisplay] = useState('')

  useEffect(() => {
    if (open) {
      setEmail(userEmail)
      setCooldownUntil(null)
      setCooldownDisplay('')
    }
  }, [open, userEmail])

  // Countdown timer for cooldown
  useEffect(() => {
    if (!cooldownUntil) return
    const tick = () => {
      const remaining = formatCooldownRemaining(cooldownUntil)
      if (!remaining) {
        setCooldownUntil(null)
        setCooldownDisplay('')
      } else {
        setCooldownDisplay(remaining)
      }
    }
    tick()
    const interval = setInterval(tick, 1000)
    return () => clearInterval(interval)
  }, [cooldownUntil])

  const mutation = useMutation({
    mutationFn: submitOfflineExport,
    onSuccess: (data) => {
      if (data.success) {
        toast.success(t('Task submitted, you will be notified by email when complete'))
        queryClient.invalidateQueries({ queryKey: ['export-tasks'] })
        onOpenChange(false)
      } else if (data.next_available_time) {
        setCooldownUntil(data.next_available_time)
        toast.error(data.message || t('Please try again later'))
      } else {
        toast.error(data.message || t('Export failed'))
      }
    },
    onError: () => {
      toast.error(t('Export failed'))
    },
  })

  const handleSubmit = () => {
    if (!email.trim()) {
      toast.error(t('Please enter an email address'))
      return
    }
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
      email: email.trim(),
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

          {/* Email input */}
          <div className='space-y-2'>
            <Label htmlFor='offline-export-email'>{t('Notification Email')}</Label>
            <Input
              id='offline-export-email'
              type='email'
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder={t('Enter email to receive export file')}
            />
          </div>

          {/* Cooldown warning */}
          {cooldownUntil && cooldownDisplay && (
            <p className='text-sm text-warning'>
              {t('Please wait before submitting again')}: {cooldownDisplay}
            </p>
          )}
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
            disabled={mutation.isPending || !!cooldownUntil}
          >
            {mutation.isPending ? t('Submitting...') : t('Submit Export Task')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
