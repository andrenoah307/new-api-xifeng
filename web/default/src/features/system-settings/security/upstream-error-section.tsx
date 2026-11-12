import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'

import { parseUpstreamErrorMessages } from '../../../../../shared/upstream-error-messages'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  upstreamErrorSchema,
  type UpstreamErrorValues,
} from '../lib/upstream-error-schema'

export function UpstreamErrorSection(props: { value: string }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const { register, handleSubmit, reset } = useForm<UpstreamErrorValues>({
    resolver: zodResolver(upstreamErrorSchema),
    defaultValues: parseUpstreamErrorMessages(props.value),
  })
  const saving = useRef(false)
  const form = useRef<HTMLFormElement>(null)
  useEffect(
    () => reset(parseUpstreamErrorMessages(props.value)),
    [props.value, reset]
  )
  const labels = {
    client: t('Client errors (4xx)'),
    server: t('Server errors (5xx or unknown)'),
  }

  async function save(values: UpstreamErrorValues) {
    if (saving.current) return
    saving.current = true
    try {
      await updateOption.mutateAsync({
        key: 'UpstreamErrorMessages',
        value: JSON.stringify(values),
      })
    } catch {
      // The mutation hook displays the save error.
    } finally {
      saving.current = false
    }
  }

  return (
    <SettingsSection title={t('Upstream Error Redaction')}>
      <SettingsForm ref={form} onSubmit={handleSubmit(save)}>
        <FieldGroup>
          {(Object.keys(labels) as (keyof typeof labels)[]).map((key) => (
            <Field key={key}>
              <FieldLabel htmlFor={`upstream-error-${key}`}>
                {labels[key]}
              </FieldLabel>
              <Input
                id={`upstream-error-${key}`}
                {...register(key)}
                maxLength={300}
                required
                disabled={updateOption.isPending}
              />
            </Field>
          ))}
        </FieldGroup>
        <SettingsPageFormActions
          onSave={() => form.current?.requestSubmit()}
          isSaving={updateOption.isPending}
        />
      </SettingsForm>
    </SettingsSection>
  )
}
