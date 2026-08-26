/* eslint-disable react-refresh/only-export-components */
import { useMemo } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import type { ChannelFormValues } from '../../lib/channel-form'

export interface ChannelRateLimit {
  enabled: boolean
  rpm: number
  concurrency: number
  on_limit: string
  queue_max_wait_ms: number
  queue_depth: number
}

export const DEFAULT_CHANNEL_RATE_LIMIT: ChannelRateLimit = {
  enabled: false,
  rpm: 0,
  concurrency: 0,
  on_limit: 'skip',
  // Keep queue defaults aligned with service/channel_limiter/limiter.go.
  queue_max_wait_ms: 2000,
  queue_depth: 20,
}

export function parseChannelRateLimit(
  val: string | undefined
): ChannelRateLimit {
  if (!val) return { ...DEFAULT_CHANNEL_RATE_LIMIT }
  try {
    const parsed: unknown = JSON.parse(val)
    if (
      typeof parsed !== 'object' ||
      parsed === null ||
      Array.isArray(parsed)
    ) {
      return { ...DEFAULT_CHANNEL_RATE_LIMIT }
    }
    return {
      ...DEFAULT_CHANNEL_RATE_LIMIT,
      ...(parsed as Partial<ChannelRateLimit>),
    }
  } catch {
    return { ...DEFAULT_CHANNEL_RATE_LIMIT }
  }
}

export function serializeChannelRateLimit(value: ChannelRateLimit): string {
  return JSON.stringify(value)
}

interface Props {
  form: UseFormReturn<ChannelFormValues>
}

export function ChannelRateLimitEditor({ form }: Props) {
  const { t } = useTranslation()
  const raw = form.watch('channel_rate_limit')
  const data = useMemo(() => parseChannelRateLimit(raw), [raw])

  const update = (field: keyof ChannelRateLimit, value: unknown) => {
    const next = { ...data, [field]: value }
    form.setValue('channel_rate_limit', serializeChannelRateLimit(next), {
      shouldDirty: true,
    })
  }

  return (
    <div className='space-y-3'>
      <div className='flex items-center justify-between'>
        <h4 className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
          {t('Channel Rate Limit')}
        </h4>
        <Switch
          checked={data.enabled}
          onCheckedChange={(v) => update('enabled', v)}
        />
      </div>
      <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Requests Per Minute')}</Label>
          <Input
            type='number'
            value={data.rpm}
            onChange={(e) => update('rpm', Number(e.target.value))}
            className='h-8'
          />
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Concurrency')}</Label>
          <Input
            type='number'
            value={data.concurrency}
            onChange={(e) => update('concurrency', Number(e.target.value))}
            className='h-8'
          />
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('On-Limit Strategy')}</Label>
          <Select
            value={data.on_limit}
            onValueChange={(v) => update('on_limit', v)}
          >
            <SelectTrigger className='h-8'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='skip'>{t('Skip')}</SelectItem>
              <SelectItem value='queue'>{t('Queue')}</SelectItem>
              <SelectItem value='reject'>{t('Reject')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Max Queue Wait')}</Label>
          <Input
            type='number'
            value={data.queue_max_wait_ms}
            onChange={(e) =>
              update('queue_max_wait_ms', Number(e.target.value))
            }
            className='h-8'
          />
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Queue Depth')}</Label>
          <Input
            type='number'
            value={data.queue_depth}
            onChange={(e) => update('queue_depth', Number(e.target.value))}
            className='h-8'
          />
        </div>
      </div>
    </div>
  )
}
