import * as z from 'zod'
import type { Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

const schema = z.object({
  enabled: z.boolean(),
  title: z.string().optional(),
  content: z.string().optional(),
  blockedCountries: z.string().optional(),
  silenceMinutes: z.number().int().min(0).max(43200),
})

type FormValues = z.infer<typeof schema>

type Props = {
  defaultValues: {
    enabled: string
    title: string
    content: string
    blockedCountries: string
    silenceMinutes: string
  }
}

function parseBool(v: string): boolean {
  return v === 'true' || v === '1'
}

function parseBlockedCountries(raw: string): string[] {
  if (!raw) return []
  const trimmed = raw.trim()
  if (trimmed.startsWith('[')) {
    try {
      const parsed = JSON.parse(trimmed) as unknown
      if (Array.isArray(parsed)) {
        return parsed
          .map((v) => String(v).trim().toUpperCase())
          .filter((v) => v.length > 0)
      }
    } catch {
      // fall through to line split
    }
  }
  return trimmed
    .split(/[\n,]/)
    .map((s) => s.trim().toUpperCase())
    .filter((s) => s.length > 0)
}

function serializeBlockedCountries(textarea: string): string {
  return JSON.stringify(parseBlockedCountries(textarea))
}

function blockedCountriesToTextarea(raw: string): string {
  return parseBlockedCountries(raw).join('\n')
}

export function CnDisclaimerSection({ defaultValues }: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const normalized: FormValues = {
    enabled: parseBool(defaultValues.enabled || 'true'),
    title: defaultValues.title ?? '',
    content: defaultValues.content ?? '',
    blockedCountries: blockedCountriesToTextarea(
      defaultValues.blockedCountries || ''
    ),
    silenceMinutes: Number.parseInt(defaultValues.silenceMinutes || '60', 10),
  }

  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<FormValues>({
      resolver: zodResolver(schema) as Resolver<FormValues, unknown, FormValues>,
      defaultValues: normalized,
      onSubmit: async (_data, changedFields) => {
        const mapping: Record<keyof FormValues, string> = {
          enabled: 'cn_disclaimer.enabled',
          title: 'cn_disclaimer.title',
          content: 'cn_disclaimer.content',
          blockedCountries: 'cn_disclaimer.blocked_countries',
          silenceMinutes: 'cn_disclaimer.silence_minutes',
        }
        for (const [field, value] of Object.entries(changedFields)) {
          const key = mapping[field as keyof FormValues]
          if (!key) continue
          let v: string
          if (field === 'enabled') {
            v = value ? 'true' : 'false'
          } else if (field === 'blockedCountries') {
            v = serializeBlockedCountries(String(value ?? ''))
          } else if (field === 'silenceMinutes') {
            const n = Number.parseInt(String(value ?? '0'), 10)
            v = String(Number.isFinite(n) && n >= 0 ? n : 60)
          } else {
            v = String(value ?? '')
          }
          await updateOption.mutateAsync({ key, value: v })
        }
      },
    })

  return (
    <>
      <FormNavigationGuard when={isDirty} />
      <SettingsSection
        title={t('Regional access disclaimer')}
        description={t(
          'Show a disclaimer modal to users from specific regions until they acknowledge it.'
        )}
      >
        <Form {...form}>
          <form onSubmit={handleSubmit} className='space-y-6'>
            <FormDirtyIndicator isDirty={isDirty} />

            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-md border p-3'>
                  <div className='space-y-0.5'>
                    <FormLabel>{t('Enabled')}</FormLabel>
                    <FormDescription>
                      {t(
                        'When disabled, the disclaimer modal will never be shown.'
                      )}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='title'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Title')}</FormLabel>
                  <FormControl>
                    <Input {...field} />
                  </FormControl>
                  <FormDescription>
                    {t('Heading shown at the top of the disclaimer dialog.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='content'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Content')}</FormLabel>
                  <FormControl>
                    <Textarea rows={8} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Body text of the disclaimer. Line breaks are preserved.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='blockedCountries'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Blocked countries')}</FormLabel>
                  <FormControl>
                    <Textarea
                      rows={4}
                      placeholder={'CN'}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'ISO country codes (uppercase), one per line or comma-separated. Defaults to CN.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='silenceMinutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Silence (minutes)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      value={field.value}
                      onChange={(e) => {
                        const n = Number.parseInt(e.target.value, 10)
                        field.onChange(Number.isFinite(n) && n >= 0 ? n : 0)
                      }}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      '0 = show on every visit. 60 = silence the modal for 60 minutes after a user acknowledges.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='flex justify-end'>
              <button
                type='submit'
                disabled={!isDirty || isSubmitting}
                className='bg-primary text-primary-foreground rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50'
              >
                {t('Save')}
              </button>
            </div>
          </form>
        </Form>
      </SettingsSection>
    </>
  )
}
