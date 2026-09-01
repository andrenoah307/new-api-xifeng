import { useEffect, useMemo } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
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
import {
  cleanPressureCoolingGroups,
  getPressureCoolingGroupOptions,
  isPressureCoolingSaveAllowed,
  parsePressureCooling,
  serializePressureCooling,
  type PressureCooling,
} from './pressure-cooling'

// oxlint-disable-next-line react/only-export-components
export { normalizePressureCooling } from './pressure-cooling'

interface Props {
  form: UseFormReturn<ChannelFormValues>
}

export function PressureCoolingEditor({ form }: Props) {
  const { t } = useTranslation()
  const raw = form.watch('pressure_cooling')
  const currentGroups = form.watch('group')
  const availableGroups = useMemo(
    () => (Array.isArray(currentGroups) ? currentGroups : []),
    [currentGroups]
  )
  const data = useMemo(() => parsePressureCooling(raw), [raw])
  const groupOptions = useMemo(
    () => getPressureCoolingGroupOptions(availableGroups),
    [availableGroups]
  )
  const selectedCooldownGroups = useMemo(
    () => cleanPressureCoolingGroups(data.cooldown_groups, availableGroups),
    [availableGroups, data.cooldown_groups]
  )

  useEffect(() => {
    if (data.scope !== 'groups') return
    if (
      selectedCooldownGroups.length === data.cooldown_groups.length &&
      selectedCooldownGroups.every(
        (group, index) => group === data.cooldown_groups[index]
      )
    ) {
      return
    }
    form.setValue(
      'pressure_cooling',
      serializePressureCooling(
        { ...data, cooldown_groups: selectedCooldownGroups },
        availableGroups
      ),
      { shouldDirty: true, shouldValidate: true }
    )
  }, [availableGroups, data, form, selectedCooldownGroups])

  const update = (field: keyof PressureCooling, value: unknown) => {
    const next = { ...data, [field]: value }
    form.setValue(
      'pressure_cooling',
      serializePressureCooling(next, availableGroups),
      { shouldDirty: true, shouldValidate: true }
    )
  }

  return (
    <div className='space-y-3'>
      <div className='flex items-center justify-between'>
        <h4 className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
          {t('Pressure Cooling')}
        </h4>
        <Switch
          checked={data.enabled ?? false}
          onCheckedChange={(v) => update('enabled', v)}
        />
      </div>
      <div className='space-y-1'>
        <Label className='text-xs'>{t('Cooling Scope')}</Label>
        <Select
          value={data.scope}
          onValueChange={(value: string | null) =>
            update('scope', value === 'groups' ? 'groups' : 'channel')
          }
        >
          <SelectTrigger className='h-8 w-full'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='channel'>{t('Entire Channel')}</SelectItem>
            <SelectItem value='groups'>{t('Specific Groups')}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      {data.scope === 'groups' && (
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Cooldown Groups')}</Label>
          <MultiSelect
            options={groupOptions}
            selected={selectedCooldownGroups}
            onChange={(values) => update('cooldown_groups', values)}
            placeholder={t('Select groups to cool')}
            emptyText={t('No channel groups available')}
          />
          {!isPressureCoolingSaveAllowed({
            ...data,
            cooldown_groups: selectedCooldownGroups,
          }) && (
            <p className='text-destructive text-xs' role='alert'>
              {t(
                'At least one cooldown group is required when using specific groups'
              )}
            </p>
          )}
        </div>
      )}
      <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('FRT Threshold (ms)')}</Label>
          <Input
            type='number'
            value={data.frt_threshold_ms ?? ''}
            onChange={(e) =>
              update(
                'frt_threshold_ms',
                e.target.value ? Number(e.target.value) : null
              )
            }
            className='h-8'
          />
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Trigger Percent')}</Label>
          <Input
            type='number'
            step='1'
            min='0'
            max='100'
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
            className='h-8'
          />
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Cooldown Seconds')}</Label>
          <Input
            type='number'
            value={data.cooldown_seconds ?? ''}
            onChange={(e) =>
              update(
                'cooldown_seconds',
                e.target.value ? Number(e.target.value) : null
              )
            }
            className='h-8'
          />
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Observation Window')}</Label>
          <Input
            type='number'
            value={data.observation_window_seconds ?? ''}
            onChange={(e) =>
              update(
                'observation_window_seconds',
                e.target.value ? Number(e.target.value) : null
              )
            }
            className='h-8'
          />
        </div>
      </div>
    </div>
  )
}
