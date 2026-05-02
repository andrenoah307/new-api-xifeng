import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { UseFormReturn } from 'react-hook-form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Separator } from '@/components/ui/separator'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { ChannelFormValues } from '../../lib/channel-form'

interface RateLimit {
  enabled: boolean
  rpm: number
  concurrency: number
  on_limit: string
  queue_max_wait_ms: number
  queue_depth: number
}

const defaults: RateLimit = {
  enabled: false,
  rpm: 0,
  concurrency: 0,
  on_limit: 'skip',
  queue_max_wait_ms: 30000,
  queue_depth: 100,
}

function parse(val: string | undefined): RateLimit {
  if (!val) return { ...defaults }
  try {
    return { ...defaults, ...JSON.parse(val) }
  } catch {
    return { ...defaults }
  }
}

interface Props {
  form: UseFormReturn<ChannelFormValues>
}

export function ChannelRateLimitEditor({ form }: Props) {
  const { t } = useTranslation()
  const raw = form.watch('channel_rate_limit')
  const data = useMemo(() => parse(raw), [raw])

  const update = (field: keyof RateLimit, value: unknown) => {
    const next = { ...data, [field]: value }
    form.setValue('channel_rate_limit', JSON.stringify(next), {
      shouldDirty: true,
    })
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-sm font-medium">{t('Channel Rate Limit')}</h4>
        <Switch
          checked={data.enabled}
          onCheckedChange={(v) => update('enabled', v)}
        />
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div className="space-y-1">
          <Label className="text-xs">{t('Requests Per Minute')}</Label>
          <Input
            type="number"
            value={data.rpm}
            onChange={(e) => update('rpm', Number(e.target.value))}
            className="h-8"
          />
        </div>
        <div className="space-y-1">
          <Label className="text-xs">{t('Concurrency')}</Label>
          <Input
            type="number"
            value={data.concurrency}
            onChange={(e) => update('concurrency', Number(e.target.value))}
            className="h-8"
          />
        </div>
        <div className="space-y-1">
          <Label className="text-xs">{t('On-Limit Strategy')}</Label>
          <Select
            value={data.on_limit}
            onValueChange={(v) => update('on_limit', v)}
          >
            <SelectTrigger className="h-8">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="skip">{t('Skip')}</SelectItem>
              <SelectItem value="queue">{t('Queue')}</SelectItem>
              <SelectItem value="reject">{t('Reject')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <Label className="text-xs">{t('Max Queue Wait')}</Label>
          <Input
            type="number"
            value={data.queue_max_wait_ms}
            onChange={(e) =>
              update('queue_max_wait_ms', Number(e.target.value))
            }
            className="h-8"
          />
        </div>
      </div>
      <Separator />
    </div>
  )
}
