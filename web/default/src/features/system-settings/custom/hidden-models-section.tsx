import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Textarea } from '@/components/ui/textarea'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const hiddenModelsSchema = z.object({
  HiddenModels: z.string(),
})

type HiddenModelsFormValues = z.infer<typeof hiddenModelsSchema>

type HiddenModelsSectionProps = {
  defaultValue: string
}

export function HiddenModelsSection({ defaultValue }: HiddenModelsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const defaultValues: HiddenModelsFormValues = { HiddenModels: defaultValue }

  const form = useForm({
    resolver: zodResolver(hiddenModelsSchema),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const onSubmit = async (data: HiddenModelsFormValues) => {
    if (data.HiddenModels === defaultValue) return
    await updateOption.mutateAsync({
      key: 'HiddenModels',
      value: data.HiddenModels,
    })
  }

  return (
    <SettingsSection title={t('Hidden Models')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='HiddenModels'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Hidden Models')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={6}
                    placeholder={t('one model name per line')}
                    {...field}
                    onChange={(e) => field.onChange(e.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Models listed here will be hidden from the pricing page but can still be used via API.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
