import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { UseFormReturn } from 'react-hook-form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import type { ChannelFormValues } from '../../lib/channel-form'

interface PressureCooling {
  enabled: boolean | null
  frt_threshold_ms: number | null
  trigger_percent: number | null
  cooldown_seconds: number | null
  observation_window_seconds: number | null
}

function parse(val: string | undefined): PressureCooling {
  if (!val) return { enabled: null, frt_threshold_ms: null, trigger_percent: null, cooldown_seconds: null, observation_window_seconds: null }
  try {
    return JSON.parse(val)
  } catch {
    return { enabled: null, frt_threshold_ms: null, trigger_percent: null, cooldown_seconds: null, observation_window_seconds: null }
  }
}

function serialize(obj: PressureCooling): string {
  const hasValue = obj.enabled !== null || obj.frt_threshold_ms !== null || obj.trigger_percent !== null || obj.cooldown_seconds !== null || obj.observation_window_seconds !== null
  return hasValue ? JSON.stringify(obj) : ''
}

interface Props {
  form: UseFormReturn<ChannelFormValues>
}

export function PressureCoolingEditor({ form }: Props) {
  const { t } = useTranslation()
  const raw = form.watch('pressure_cooling')
  const data = useMemo(() => parse(raw), [raw])

  const update = (field: keyof PressureCooling, value: unknown) => {
    const next = { ...data, [field]: value }
    form.setValue('pressure_cooling', serialize(next), { shouldDirty: true })
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
          {t('Pressure Cooling')}
        </h4>
        <Switch
          checked={data.enabled ?? false}
          onCheckedChange={(v) => update('enabled', v)}
        />
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div className="space-y-1">
          <Label className="text-xs">{t('FRT Threshold (ms)')}</Label>
          <Input
            type="number"
            value={data.frt_threshold_ms ?? ''}
            onChange={(e) => update('frt_threshold_ms', e.target.value ? Number(e.target.value) : null)}
            className="h-8"
          />
        </div>
        <div className="space-y-1">
          <Label className="text-xs">{t('Trigger Percent')}</Label>
          <Input
            type="number"
            step="1"
            min="0"
            max="100"
            value={data.trigger_percent ?? ''}
            // 后端 trigger_percent 是整数（*int），小数会让整个渠道设置反序列化失败
            onChange={(e) => {
              const value = e.target.valueAsNumber
              update(
                'trigger_percent',
                Number.isFinite(value)
                  ? Math.min(100, Math.max(0, Math.round(value)))
                  : null
              )
            }}
            className="h-8"
          />
        </div>
        <div className="space-y-1">
          <Label className="text-xs">{t('Cooldown Seconds')}</Label>
          <Input
            type="number"
            value={data.cooldown_seconds ?? ''}
            onChange={(e) => update('cooldown_seconds', e.target.value ? Number(e.target.value) : null)}
            className="h-8"
          />
        </div>
        <div className="space-y-1">
          <Label className="text-xs">{t('Observation Window')}</Label>
          <Input
            type="number"
            value={data.observation_window_seconds ?? ''}
            onChange={(e) => update('observation_window_seconds', e.target.value ? Number(e.target.value) : null)}
            className="h-8"
          />
        </div>
      </div>
    </div>
  )
}
