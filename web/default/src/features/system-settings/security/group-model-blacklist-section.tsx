import { useEffect, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
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
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const groupModelBlacklistSchema = z.object({
  enabled: z.boolean(),
  filter_console: z.boolean(),
  block_relay: z.boolean(),
  block_message: z.string(),
  blocked_models: z.string().refine(
    (val) => {
      if (!val.trim()) return true
      try {
        const parsed = JSON.parse(val)
        return typeof parsed === 'object' && !Array.isArray(parsed)
      } catch {
        return false
      }
    },
    'Must be valid JSON object, e.g. {"default": ["o1-*"]}'
  ),
})

type GroupModelBlacklistFormValues = z.output<typeof groupModelBlacklistSchema>

type GroupModelBlacklistProps = {
  defaultValues: {
    'group_model_blacklist.enabled': boolean
    'group_model_blacklist.filter_console': boolean
    'group_model_blacklist.block_relay': boolean
    'group_model_blacklist.block_message': string
    'group_model_blacklist.blocked_models': string
  }
}

type FlatValues = {
  enabled: boolean
  filter_console: boolean
  block_relay: boolean
  block_message: string
  blocked_models: string
}

const PREFIX = 'group_model_blacklist.' as const

const buildFormDefaults = (
  defaults: GroupModelBlacklistProps['defaultValues']
): GroupModelBlacklistFormValues => ({
  enabled: defaults['group_model_blacklist.enabled'],
  filter_console: defaults['group_model_blacklist.filter_console'],
  block_relay: defaults['group_model_blacklist.block_relay'],
  block_message: defaults['group_model_blacklist.block_message'],
  blocked_models: defaults['group_model_blacklist.blocked_models'],
})

const flattenDefaults = (
  defaults: GroupModelBlacklistProps['defaultValues']
): FlatValues => ({
  enabled: defaults['group_model_blacklist.enabled'],
  filter_console: defaults['group_model_blacklist.filter_console'],
  block_relay: defaults['group_model_blacklist.block_relay'],
  block_message: defaults['group_model_blacklist.block_message'],
  blocked_models: defaults['group_model_blacklist.blocked_models'],
})

export function GroupModelBlacklistSection({
  defaultValues,
}: GroupModelBlacklistProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const baselineRef = useRef<FlatValues>(flattenDefaults(defaultValues))

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<GroupModelBlacklistFormValues>({
    resolver: zodResolver(groupModelBlacklistSchema),
    defaultValues: formDefaults,
  })

  useEffect(() => {
    baselineRef.current = flattenDefaults(defaultValues)
    form.reset(buildFormDefaults(defaultValues))
  }, [defaultValues, form])

  const onSubmit = async (data: GroupModelBlacklistFormValues) => {
    const keys = Object.keys(data) as Array<keyof FlatValues>
    const updates = keys.filter((key) => {
      return data[key] !== baselineRef.current[key]
    })

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      const raw = data[key]
      const value = typeof raw === 'boolean' ? String(raw) : (raw ?? '')
      await updateOption.mutateAsync({
        key: `${PREFIX}${key}`,
        value,
      })
    }

    baselineRef.current = { ...data }
  }

  return (
    <SettingsSection
      title={t('Group Model Blacklist')}
      description={t('Control model access based on user group')}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-6'>
          <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Enable Group Model Blacklist')}
                    </FormLabel>
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
              name='filter_console'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Console Visibility Filtering')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Hide blocked models from pricing page and model lists for restricted groups'
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
              name='block_relay'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('API Relay Blocking')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Return 403 when blocked model is requested by a restricted group'
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
          </div>

          <FormField
            control={form.control}
            name='block_message'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Custom Block Message')}</FormLabel>
                <FormControl>
                  <Input {...field} />
                </FormControl>
                <FormDescription>
                  {t(
                    'Custom error message when model is blocked by group restriction (leave empty for default)'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='blocked_models'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Group-Model Blacklist')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={6}
                    placeholder='{"default": ["o1-*"], "vip": ["dall-e-*"]}'
                    className='font-mono text-sm'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'JSON map of group names to blocked model patterns. Example: {"default": ["o1-*"], "vip": ["dall-e-*"]}'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Button type='submit' disabled={updateOption.isPending}>
            {updateOption.isPending
              ? t('Saving...')
              : t('Save group model blacklist settings')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
