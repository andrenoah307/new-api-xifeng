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
import { MapPatternsField } from '../components/map-patterns-field'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const regionRestrictionSchema = z.object({
  enabled: z.boolean(),
  filter_console: z.boolean(),
  block_relay: z.boolean(),
  xdb_path: z.string(),
  block_message: z.string(),
  console_message: z.string(),
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
    'Must be valid JSON object, e.g. {"CN": ["gpt-4*"]}'
  ),
  blocked_groups: z.string().refine(
    (val) => {
      if (!val.trim()) return true
      try {
        const parsed = JSON.parse(val)
        return typeof parsed === 'object' && !Array.isArray(parsed)
      } catch {
        return false
      }
    },
    'Must be valid JSON object, e.g. {"CN": ["default"]}'
  ),
})

type RegionRestrictionFormValues = z.output<typeof regionRestrictionSchema>

type RegionRestrictionProps = {
  defaultValues: {
    'region_restriction.enabled': boolean
    'region_restriction.filter_console': boolean
    'region_restriction.block_relay': boolean
    'region_restriction.xdb_path': string
    'region_restriction.block_message': string
    'region_restriction.console_message': string
    'region_restriction.blocked_models': string
    'region_restriction.blocked_groups': string
  }
}

type FlatValues = {
  enabled: boolean
  filter_console: boolean
  block_relay: boolean
  xdb_path: string
  block_message: string
  console_message: string
  blocked_models: string
  blocked_groups: string
}

const PREFIX = 'region_restriction.' as const

const buildFormDefaults = (
  defaults: RegionRestrictionProps['defaultValues']
): RegionRestrictionFormValues => ({
  enabled: defaults['region_restriction.enabled'],
  filter_console: defaults['region_restriction.filter_console'],
  block_relay: defaults['region_restriction.block_relay'],
  xdb_path: defaults['region_restriction.xdb_path'],
  block_message: defaults['region_restriction.block_message'],
  console_message: defaults['region_restriction.console_message'],
  blocked_models: defaults['region_restriction.blocked_models'],
  blocked_groups: defaults['region_restriction.blocked_groups'],
})

const flattenDefaults = (
  defaults: RegionRestrictionProps['defaultValues']
): FlatValues => ({
  enabled: defaults['region_restriction.enabled'],
  filter_console: defaults['region_restriction.filter_console'],
  block_relay: defaults['region_restriction.block_relay'],
  xdb_path: defaults['region_restriction.xdb_path'],
  block_message: defaults['region_restriction.block_message'],
  console_message: defaults['region_restriction.console_message'],
  blocked_models: defaults['region_restriction.blocked_models'],
  blocked_groups: defaults['region_restriction.blocked_groups'],
})

export function RegionRestrictionSection({
  defaultValues,
}: RegionRestrictionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const baselineRef = useRef<FlatValues>(flattenDefaults(defaultValues))

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<RegionRestrictionFormValues>({
    resolver: zodResolver(regionRestrictionSchema),
    defaultValues: formDefaults,
  })

  useEffect(() => {
    baselineRef.current = flattenDefaults(defaultValues)
    form.reset(buildFormDefaults(defaultValues))
  }, [defaultValues, form])

  const onSubmit = async (data: RegionRestrictionFormValues) => {
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
      title={t('Region Restriction')}
      description={t('Control model access based on geographic region')}
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
                      {t('Enable Region Restriction')}
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
                        'Hide blocked models from pricing page and model lists'
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
                        'Return 403 when blocked model is requested from restricted region'
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
            name='xdb_path'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('ip2region Database Path')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='data/ip2region.xdb'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Path to the ip2region .xdb file for IP geolocation'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

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
                    'Custom error message when model is blocked by region (leave empty for default)'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='console_message'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Console Region Hint')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Not available in current region')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Text shown next to blocked groups in the console (e.g. token management). Leave empty for the default.'
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
                <FormLabel>{t('Region-Model Blacklist')}</FormLabel>
                <FormControl>
                  <MapPatternsField
                    value={field.value}
                    onChange={field.onChange}
                    keyPlaceholder='CN'
                    patternsPlaceholder='gpt-4*, *'
                    jsonPlaceholder='{"CN": ["gpt-4*"], "RU": ["*"]}'
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'JSON map of country codes to blocked model patterns. Example: {"CN": ["gpt-4*"], "RU": ["*"]}'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='blocked_groups'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Region-Group Blacklist')}</FormLabel>
                <FormControl>
                  <MapPatternsField
                    value={field.value}
                    onChange={field.onChange}
                    keyPlaceholder='CN'
                    patternsPlaceholder='default, *'
                    jsonPlaceholder='{"CN": ["default"], "RU": ["*"]}'
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'JSON map of country codes to blocked group patterns. Groups in the blacklist will be completely unavailable in that region.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Button type='submit' disabled={updateOption.isPending}>
            {updateOption.isPending
              ? t('Saving...')
              : t('Save region restriction settings')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
