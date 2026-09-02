import { useEffect, useMemo } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
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
          id='pressure-cooling-enabled'
          aria-label={t('Pressure Cooling')}
          checked={data.enabled ?? false}
          onCheckedChange={(v) => update('enabled', v)}
        />
      </div>
      <div className='space-y-1'>
        <Label htmlFor='pressure-cooling-scope' className='text-xs'>
          {t('Cooling Scope')}
        </Label>
        <Select
          value={data.scope}
          onValueChange={(value: string | null) =>
            update('scope', value === 'groups' ? 'groups' : 'channel')
          }
        >
          <SelectTrigger id='pressure-cooling-scope' className='h-8 w-full'>
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
      <section
        className='space-y-3 border-t pt-3'
        aria-labelledby='pressure-cooling-trigger-conditions'
      >
        <h5
          id='pressure-cooling-trigger-conditions'
          className='text-muted-foreground text-xs font-medium tracking-wide uppercase'
        >
          {t('Trigger Conditions')}
        </h5>
        <p className='text-muted-foreground text-xs'>
          {t('Both conditions share the same observation window')}
        </p>
        {(data.upstream_error_trigger_percent ?? 0) > 0 ||
        (data.upstream_error_trigger_count ?? 0) > 0 ? (
          <div className='space-y-1'>
            <Label className='text-xs'>{t('Condition Combination')}</Label>
            <RadioGroup
              value={data.condition_mode}
              onValueChange={(value: string | null) =>
                update('condition_mode', value === 'all' ? 'all' : 'any')
              }
              className='flex flex-wrap gap-4'
              aria-label={t('Condition Combination')}
            >
              <div className='flex items-center gap-2'>
                <RadioGroupItem
                  value='any'
                  id='pressure-cooling-condition-any'
                />
                <Label
                  htmlFor='pressure-cooling-condition-any'
                  className='cursor-pointer text-xs font-normal'
                >
                  {t('Match any condition to cool down')}
                </Label>
              </div>
              <div className='flex items-center gap-2'>
                <RadioGroupItem
                  value='all'
                  id='pressure-cooling-condition-all'
                />
                <Label
                  htmlFor='pressure-cooling-condition-all'
                  className='cursor-pointer text-xs font-normal'
                >
                  {t('Match all conditions to cool down')}
                </Label>
              </div>
            </RadioGroup>
          </div>
        ) : null}
        <div className='space-y-2'>
          <div className='flex items-center gap-2'>
            <span className='text-xs' aria-hidden='true'>
              ①
            </span>
            <span className='text-xs font-medium'>
              {t('First Token Latency (FRT)')}
            </span>
            <span className='text-muted-foreground text-xs'>
              {t('Always enabled')}
            </span>
          </div>
          <div className='grid grid-cols-2 gap-3'>
            <div className='space-y-1'>
              <Label
                htmlFor='pressure-cooling-frt-threshold'
                className='text-xs'
              >
                {t('FRT Threshold (ms)')}
              </Label>
              <Input
                id='pressure-cooling-frt-threshold'
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
              <Label
                htmlFor='pressure-cooling-trigger-percent'
                className='text-xs'
              >
                {t('Trigger Percent')} (%)
              </Label>
              <Input
                id='pressure-cooling-trigger-percent'
                type='number'
                step='1'
                min='1'
                max='100'
                value={data.trigger_percent ?? ''}
                // 后端 trigger_percent 是整数（*int），小数会让整个渠道设置反序列化失败
                onChange={(e) => {
                  const value = e.target.valueAsNumber
                  const roundedValue = Math.round(value)
                  update(
                    'trigger_percent',
                    Number.isFinite(value) && roundedValue > 0
                      ? Math.min(100, Math.max(1, roundedValue))
                      : null
                  )
                }}
                className='h-8'
              />
            </div>
          </div>
        </div>
        <div className='space-y-2'>
          <div className='flex items-center gap-2'>
            <span className='text-xs' aria-hidden='true'>
              ②
            </span>
            <span className='text-xs font-medium'>{t('Upstream Errors')}</span>
          </div>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Either upstream error condition can trigger cooling. Enter 0 to disable that condition. Interpret the error count threshold with the observation window; use caution for high-traffic channels, where even a low error rate can quickly reach the absolute count.'
            )}
          </p>
          <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
            <div className='space-y-1'>
              <Label
                htmlFor='pressure-cooling-upstream-error-percent'
                className='text-xs'
              >
                {t('Error Rate')} (%)
              </Label>
              <Input
                id='pressure-cooling-upstream-error-percent'
                type='number'
                step='1'
                min='0'
                max='100'
                placeholder='0'
                value={data.upstream_error_trigger_percent ?? ''}
                onChange={(event) => {
                  const value = event.target.valueAsNumber
                  const roundedValue = Math.round(value)
                  update(
                    'upstream_error_trigger_percent',
                    Number.isFinite(value)
                      ? Math.min(100, Math.max(0, roundedValue))
                      : null
                  )
                }}
              />
            </div>
            <div className='space-y-1'>
              <Label
                htmlFor='pressure-cooling-upstream-error-min-samples'
                className='text-xs'
                title={t(
                  'Minimum attempts for the error rate condition (denominator), not the minimum error count.'
                )}
              >
                {t('Minimum Attempts (rate denominator)')}
              </Label>
              <Input
                id='pressure-cooling-upstream-error-min-samples'
                type='number'
                step='1'
                min='0'
                placeholder='10'
                value={data.upstream_error_min_samples ?? ''}
                onChange={(event) => {
                  const value = event.target.valueAsNumber
                  update(
                    'upstream_error_min_samples',
                    Number.isFinite(value)
                      ? Math.max(0, Math.round(value))
                      : null
                  )
                }}
              />
            </div>
            <div className='space-y-1'>
              <Label
                htmlFor='pressure-cooling-upstream-error-count'
                className='text-xs'
              >
                {t('Error Count Threshold')}
              </Label>
              <Input
                id='pressure-cooling-upstream-error-count'
                type='number'
                step='1'
                min='0'
                placeholder='0'
                value={data.upstream_error_trigger_count ?? ''}
                onChange={(event) => {
                  const value = event.target.valueAsNumber
                  update(
                    'upstream_error_trigger_count',
                    Number.isFinite(value)
                      ? Math.max(0, Math.round(value))
                      : null
                  )
                }}
              />
            </div>
          </div>
        </div>
      </section>
      <section
        className='space-y-3 border-t pt-3'
        aria-labelledby='pressure-cooling-cooldown-settings'
      >
        <h5
          id='pressure-cooling-cooldown-settings'
          className='text-muted-foreground text-xs font-medium tracking-wide uppercase'
        >
          {t('Cooldown Settings')}
        </h5>
        <div className='grid grid-cols-2 gap-3'>
          <div className='space-y-1'>
            <Label
              htmlFor='pressure-cooling-observation-window'
              className='text-xs'
            >
              {t('Observation Window')} ({t('seconds')})
            </Label>
            <Input
              id='pressure-cooling-observation-window'
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
          <div className='space-y-1'>
            <Label
              htmlFor='pressure-cooling-cooldown-seconds'
              className='text-xs'
            >
              {t('Cooldown Seconds')}
            </Label>
            <Input
              id='pressure-cooling-cooldown-seconds'
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
        </div>
      </section>
    </div>
  )
}
