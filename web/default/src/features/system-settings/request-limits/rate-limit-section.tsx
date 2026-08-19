/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { Code2, Palette } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

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
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  parseGroupRateLimitConfig,
  validateGroupRateLimitRule,
} from './lib/group-rate-limit'
import { parseModelNameRPMConfig } from './lib/model-name-rpm'
import { ModelNameRPMVisualEditor } from './model-name-rpm-visual-editor'
import { RateLimitVisualEditor } from './rate-limit-visual-editor'

const isValidGroupRateLimit = (value: string | undefined) => {
  const parsed = parseGroupRateLimitConfig(value ?? '')
  return (
    parsed.ok &&
    parsed.rules.every((rule) => validateGroupRateLimitRule(rule).ok)
  )
}

const isValidJsonDocument = (value: string): boolean => {
  try {
    JSON.parse(value)
    return true
  } catch {
    return false
  }
}

function isJsonObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function getModelNameRPMEnabled(value: string): boolean {
  try {
    const parsed: unknown = JSON.parse(value)
    return isJsonObject(parsed) && typeof parsed.enabled === 'boolean'
      ? parsed.enabled
      : false
  } catch {
    return false
  }
}

function setModelNameRPMEnabled(value: string, enabled: boolean): string {
  let parsed: unknown = {}

  try {
    parsed = JSON.parse(value)
  } catch {
    parsed = {}
  }

  const config = isJsonObject(parsed) ? { ...parsed } : {}
  config.enabled = enabled
  return JSON.stringify(config, null, 2)
}

const createRateLimitSchema = (t: (key: string) => string) =>
  z.object({
    RateLimitCapacityCardEnabled: z.boolean(),
    ModelRequestRateLimitEnabled: z.boolean(),
    ModelRequestRateLimitDurationMinutes: z.number().min(0),
    ModelRequestRateLimitCount: z.number().min(0).max(100000000),
    ModelRequestRateLimitSuccessCount: z.number().min(1).max(100000000),
    ModelRequestRateLimitGroup: z
      .string()
      .optional()
      .refine(isValidGroupRateLimit, {
        message: t('Invalid JSON format or values out of allowed range'),
      }),
    ModelNameRPMRateLimit: z.string().refine(isValidJsonDocument, {
      message: t('Invalid JSON format'),
    }),
  })

type RateLimitFormValues = z.infer<ReturnType<typeof createRateLimitSchema>>

type RateLimitSectionProps = {
  defaultValues: RateLimitFormValues
}

export function RateLimitSection({ defaultValues }: RateLimitSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [groupEditorMode, setGroupEditorMode] = useState<
    'visual' | 'json' | null
  >(null)
  // null means "not chosen yet": the mode then follows whether the stored
  // document can be represented visually without rewriting it.
  const [rpmEditorMode, setRpmEditorMode] = useState<'visual' | 'json' | null>(
    null
  )

  const rateLimitSchema = createRateLimitSchema(t)

  const form = useForm<RateLimitFormValues>({
    resolver: zodResolver(rateLimitSchema),
    mode: 'onChange', // Enable real-time validation
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (values: RateLimitFormValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) =>
        key !== 'ModelNameRPMRateLimit' &&
        value !== defaultValues[key as keyof RateLimitFormValues]
    )

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value: value ?? '' })
    }

    if (values.ModelNameRPMRateLimit !== defaultValues.ModelNameRPMRateLimit) {
      await updateOption.mutateAsync({
        key: 'ModelNameRPMRateLimit',
        value: values.ModelNameRPMRateLimit,
      })
    }
  }

  return (
    <SettingsSection title={t('Rate Limiting')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save rate limits'
          />
          <FormField
            control={form.control}
            name='ModelRequestRateLimitEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable rate limiting')}</FormLabel>
                  <FormDescription>
                    {t(
                      'This controls model request rate limiting. Web/API route throttling is configured by environment variables and may still return 429.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <div className='grid gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='ModelRequestRateLimitDurationMinutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Limit period')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={0}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseInt(e.target.value) || 0)
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('minutes')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Time window for rate limiting')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ModelRequestRateLimitCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max requests per period')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={0}
                        max={100000000}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseInt(e.target.value) || 0)
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Including failed requests, 0 = unlimited')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ModelRequestRateLimitSuccessCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max successful requests')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={1}
                        max={100000000}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseInt(e.target.value) || 1)
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Only successful requests')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='ModelRequestRateLimitGroup'
            render={({ field }) => {
              const groupVisualMode =
                groupEditorMode ??
                (parseGroupRateLimitConfig(field.value ?? '').ok
                  ? 'visual'
                  : 'json')

              return (
                <FormItem>
                  <div className='flex items-center justify-between'>
                    <FormLabel>{t('Group-based rate limits')}</FormLabel>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={() => {
                        if (groupVisualMode === 'visual') {
                          setGroupEditorMode('json')
                          return
                        }
                        if (!parseGroupRateLimitConfig(field.value ?? '').ok) {
                          toast.error(
                            t(
                              'Fix the JSON before switching to the visual editor.'
                            )
                          )
                          setGroupEditorMode('json')
                          return
                        }
                        setGroupEditorMode('visual')
                      }}
                    >
                      {groupVisualMode === 'visual' ? (
                        <>
                          <Code2 className='mr-2 h-4 w-4' />
                          {t('JSON Mode')}
                        </>
                      ) : (
                        <>
                          <Palette className='mr-2 h-4 w-4' />
                          {t('Visual Mode')}
                        </>
                      )}
                    </Button>
                  </div>
                  <FormControl>
                    {groupVisualMode === 'visual' ? (
                      <RateLimitVisualEditor
                        value={field.value ?? ''}
                        onChange={field.onChange}
                      />
                    ) : (
                      <Textarea
                        rows={8}
                        placeholder={`{\n  "default": [200, 100],\n  "vip": [0, 1000]\n}`}
                        className='font-mono text-sm'
                        {...field}
                      />
                    )}
                  </FormControl>
                  {groupVisualMode === 'json' && (
                    <FormDescription>
                      <div className='space-y-1 text-xs'>
                        <p className='font-semibold'>{t('Format:')}</p>
                        <ul className='list-inside list-disc space-y-0.5 pl-2'>
                          <li>
                            {t('JSON object:')}{' '}
                            {`{"groupName": [maxRequests, maxSuccess]}`}
                          </li>
                          <li>
                            {t('Example:')}{' '}
                            {`{"default": [200, 100], "vip": [0, 1000]}`}
                          </li>
                          <li>
                            {t(
                              'maxRequests ≥ 0, maxSuccess ≥ 1, both ≤ 2,147,483,647'
                            )}
                          </li>
                          <li>
                            {t(
                              'Group config overrides global limits, shares the same period'
                            )}
                          </li>
                        </ul>
                      </div>
                    </FormDescription>
                  )}
                  <FormMessage />
                </FormItem>
              )
            }}
          />

          <div className='border-border/70 mt-2 space-y-4 border-t pt-6'>
            <div className='space-y-1'>
              <h4 className='text-sm font-semibold'>
                {t('Model name RPM rate limiting')}
              </h4>
            </div>

            <FormField
              control={form.control}
              name='RateLimitCapacityCardEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('RPM overview card')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Disabled by default. When enabled, the user dashboard displays and queries the RPM overview card.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='ModelNameRPMRateLimit'
              render={({ field }) => {
                const rpmVisualMode =
                  rpmEditorMode ??
                  (parseModelNameRPMConfig(field.value).ok ? 'visual' : 'json')

                return (
                  <FormItem>
                    <div className='flex min-w-0 flex-row items-center justify-between gap-4 py-2.5'>
                      <div className='min-w-0 space-y-0.5'>
                        <Label htmlFor='model-name-rpm-rate-limit-enabled'>
                          {t('Enable model name RPM rate limiting')}
                        </Label>
                      </div>
                      <Switch
                        id='model-name-rpm-rate-limit-enabled'
                        checked={getModelNameRPMEnabled(field.value)}
                        onCheckedChange={(checked) =>
                          field.onChange(
                            setModelNameRPMEnabled(field.value, checked)
                          )
                        }
                        aria-label={t('Enable model name RPM rate limiting')}
                      />
                    </div>

                    <div className='flex items-center justify-between'>
                      <FormLabel>{t('Model name RPM configuration')}</FormLabel>
                      <Button
                        type='button'
                        variant='outline'
                        size='sm'
                        onClick={() => {
                          if (rpmVisualMode === 'visual') {
                            setRpmEditorMode('json')
                            return
                          }
                          if (!parseModelNameRPMConfig(field.value).ok) {
                            toast.error(
                              t(
                                'Fix the JSON before switching to the visual editor.'
                              )
                            )
                            return
                          }
                          setRpmEditorMode('visual')
                        }}
                      >
                        {rpmVisualMode === 'visual' ? (
                          <>
                            <Code2 className='mr-2 h-4 w-4' />
                            {t('JSON Mode')}
                          </>
                        ) : (
                          <>
                            <Palette className='mr-2 h-4 w-4' />
                            {t('Visual Mode')}
                          </>
                        )}
                      </Button>
                    </div>
                    <FormControl>
                      {rpmVisualMode === 'visual' ? (
                        <ModelNameRPMVisualEditor
                          value={field.value}
                          onChange={field.onChange}
                        />
                      ) : (
                        <Textarea
                          rows={12}
                          placeholder={t(
                            'Model name RPM configuration example'
                          )}
                          className='font-mono text-sm'
                          spellCheck={false}
                          {...field}
                        />
                      )}
                    </FormControl>
                    <FormDescription>
                      <ul className='list-inside list-disc space-y-1'>
                        <li>
                          {t(
                            'Models not listed in the models section are not subject to model-specific RPM limits.'
                          )}
                        </li>
                        <li>
                          {t(
                            'Top-level group limits apply across every model in the group, including models not listed in the models section: total_rpm caps all users combined, user_rpm caps each user, and 0 disables that limit.'
                          )}
                        </li>
                        <li>
                          {t(
                            'Group limits are stricter sub-limits of the global limit; both apply to each request (one request uses both the global and group buckets).'
                          )}
                        </li>
                        <li>
                          {t(
                            'global_rpm must be an integer from 0 to 1,000,000; 0 means unlimited (usage is still counted) and then at least one user_rpm or group_rpm is required. Delete a model rule to disable it; set enabled to false to disable all rules.'
                          )}
                        </li>
                      </ul>
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )
              }}
            />
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
