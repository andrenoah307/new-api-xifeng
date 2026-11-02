import { zodResolver } from '@hookform/resolvers/zod'
import { Add01Icon, Delete02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'

import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const schema = z
  .object({
    enabled: z.boolean(),
    rules: z
      .array(
        z.object({
          id: z.string().regex(/^[A-Za-z0-9_-]{1,48}$/),
          percent: z.number().int().min(1).max(100),
          cooldown_days: z.number().int().min(1).max(365),
          scope: z.enum(['any', 'total', 'period']),
        })
      )
      .max(20),
  })
  .refine((value) => !value.enabled || value.rules.length > 0)

type AlertConfig = z.infer<typeof schema>
interface AlertStatus {
  redis_available: boolean
  pending: number
  cached_keys: number
  dropped_notices: number
  deliveries: { status: string; count: number }[]
}

const defaults: AlertConfig = {
  enabled: false,
  rules: [
    { id: 'warning', percent: 50, cooldown_days: 7, scope: 'any' },
    { id: 'critical', percent: 70, cooldown_days: 3, scope: 'any' },
  ],
}

export function TokenUsageAlertSettingsSection(props: {
  settings: Record<string, string>
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const raw = props.settings.TokenUsageAlertRules
  const parsed = useMemo(() => {
    try {
      return schema.parse(JSON.parse(raw))
    } catch {
      return null
    }
  }, [raw])
  const values = parsed ?? defaults
  const form = useForm<AlertConfig>({
    resolver: zodResolver(schema),
    defaultValues: values,
  })
  useResetForm(form, values)
  const rules = useFieldArray({
    control: form.control,
    name: 'rules',
    keyName: 'formId',
  })
  const status = useQuery({
    queryKey: ['token-usage-alert-status'],
    queryFn: async () => {
      const response = await api.get<{
        success: boolean
        data: AlertStatus
        message: string
      }>('/api/option/token_usage_alerts/status')
      if (!response.data.success) throw new Error(response.data.message)
      return response.data.data
    },
    refetchInterval: 30000,
  })
  const submit = form.handleSubmit(
    (data) =>
      parsed &&
      updateOption.mutate({
        key: 'TokenUsageAlertRules',
        value: JSON.stringify(data),
      })
  )

  return (
    <SettingsSection title={t('User API Key usage email alerts')}>
      <form onSubmit={submit} className='flex flex-col gap-5'>
        <FieldGroup>
          <Field orientation='horizontal'>
            <FieldLabel htmlFor='token-usage-alert-enabled'>
              {t('Enable alerts')}
            </FieldLabel>
            <Switch
              id='token-usage-alert-enabled'
              checked={form.watch('enabled')}
              onCheckedChange={(value) =>
                form.setValue('enabled', value, { shouldDirty: true })
              }
            />
          </Field>
          {rules.fields.map((rule, index) => (
            <div
              key={rule.formId}
              className='grid grid-cols-1 gap-3 border-b pb-4 sm:grid-cols-[1fr_1fr_1fr_auto] sm:items-end'
            >
              <Field>
                <FieldLabel htmlFor={`alert-scope-${index}`}>
                  {t('Quota scope')}
                </FieldLabel>
                <NativeSelect
                  id={`alert-scope-${index}`}
                  {...form.register(`rules.${index}.scope`)}
                >
                  <NativeSelectOption value='any'>
                    {t('Total and period quotas')}
                  </NativeSelectOption>
                  <NativeSelectOption value='total'>
                    {t('Total quota')}
                  </NativeSelectOption>
                  <NativeSelectOption value='period'>
                    {t('Current period quota')}
                  </NativeSelectOption>
                </NativeSelect>
              </Field>
              <Field>
                <FieldLabel htmlFor={`alert-percent-${index}`}>
                  {t('Usage threshold (%)')}
                </FieldLabel>
                <Input
                  id={`alert-percent-${index}`}
                  type='number'
                  min={1}
                  max={100}
                  step={1}
                  {...form.register(`rules.${index}.percent`, {
                    valueAsNumber: true,
                  })}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`alert-days-${index}`}>
                  {t('Email cooldown (days)')}
                </FieldLabel>
                <Input
                  id={`alert-days-${index}`}
                  type='number'
                  min={1}
                  max={365}
                  step={1}
                  {...form.register(`rules.${index}.cooldown_days`, {
                    valueAsNumber: true,
                  })}
                />
              </Field>
              <Button
                type='button'
                variant='ghost'
                size='icon'
                aria-label={t('Delete rule')}
                title={t('Delete rule')}
                onClick={() => rules.remove(index)}
              >
                <HugeiconsIcon icon={Delete02Icon} size={18} />
              </Button>
            </div>
          ))}
        </FieldGroup>
        {(!parsed || Object.keys(form.formState.errors).length > 0) && (
          <p role='alert' className='text-destructive text-sm'>
            {t('Invalid API Key usage alert settings')}
          </p>
        )}
        <div className='flex flex-wrap gap-2'>
          <Button
            type='button'
            variant='outline'
            disabled={rules.fields.length >= 20}
            onClick={() =>
              rules.append({
                id: crypto.randomUUID(),
                percent: 90,
                cooldown_days: 1,
                scope: 'any',
              })
            }
          >
            <HugeiconsIcon icon={Add01Icon} size={18} />
            {t('Add rule')}
          </Button>
          <Button type='submit' disabled={!parsed || updateOption.isPending}>
            {t('Save')}
          </Button>
        </div>
      </form>
      {status.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Unable to load API Key usage alert status')}
        </p>
      )}
      {status.data && (
        <dl className='grid grid-cols-2 gap-3 border-t pt-4 text-sm sm:grid-cols-4'>
          <div>
            <dt>{t('Redis status')}</dt>
            <dd>
              {status.data.redis_available ? t('Available') : t('Unavailable')}
            </dd>
          </div>
          <div>
            <dt>{t('Queued emails')}</dt>
            <dd>{status.data.pending}</dd>
          </div>
          <div>
            <dt>{t('Cached API Keys')}</dt>
            <dd>{status.data.cached_keys}</dd>
          </div>
          <div>
            <dt>{t('Uncertain deliveries')}</dt>
            <dd>
              {status.data.deliveries?.find((row) => row.status === 'unknown')
                ?.count ?? 0}
            </dd>
          </div>
        </dl>
      )}
    </SettingsSection>
  )
}
