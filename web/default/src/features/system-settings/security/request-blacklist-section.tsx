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
import { useQuery } from '@tanstack/react-query'
import { TriangleAlert } from 'lucide-react'
import { useEffect, useMemo, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { MultiSelect } from '@/components/multi-select'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { api } from '@/lib/api'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSystemOptions } from '../hooks/use-system-options'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  parseRequestBlacklistDocument,
  serializeRequestBlacklistDocument,
  type RequestBlacklistFormValues,
} from './request-blacklist'

type RequestBlacklistSectionProps = {
  defaultValues: {
    RequestBlacklist: string
  }
}

const requestBlacklistSchema = z.object({
  mode: z.string(),
  enabledGroups: z.array(z.string()),
  contentWords: z.string(),
  domains: z.string(),
  blockMessage: z.string(),
  blockStatusCode: z.number(),
})

async function getGroups(): Promise<string[]> {
  const response = await api.get('/api/group/')
  const data = response.data?.data
  if (Array.isArray(data)) return data.map(String)
  if (data && typeof data === 'object') return Object.keys(data)
  return []
}

export function RequestBlacklistSection(props: RequestBlacklistSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const systemOptions = useSystemOptions()
  const initialValues = useMemo(
    () => parseRequestBlacklistDocument(props.defaultValues.RequestBlacklist),
    [props.defaultValues.RequestBlacklist]
  )
  const baselineDocumentRef = useRef(
    serializeRequestBlacklistDocument(initialValues)
  )
  const form = useForm<RequestBlacklistFormValues>({
    resolver: zodResolver(requestBlacklistSchema),
    defaultValues: initialValues,
  })
  const { data: groups = [] } = useQuery({
    queryKey: ['groups-list'],
    queryFn: getGroups,
  })
  const groupOptions = useMemo(
    () => groups.map((group) => ({ label: group, value: group })),
    [groups]
  )

  useEffect(() => {
    const values = parseRequestBlacklistDocument(
      props.defaultValues.RequestBlacklist
    )
    baselineDocumentRef.current = serializeRequestBlacklistDocument(values)
    form.reset(values)
  }, [form, props.defaultValues.RequestBlacklist])

  const onSubmit = async (values: RequestBlacklistFormValues) => {
    const document = serializeRequestBlacklistDocument(values)
    if (document === baselineDocumentRef.current) {
      toast.info(t('No changes to save'))
      return
    }

    try {
      const result = await updateOption.mutateAsync({
        key: 'RequestBlacklist',
        value: document,
      })
      if (!result.success) return

      const refreshed = await systemOptions.refetch()
      if (refreshed.error || !refreshed.data?.success) {
        toast.error(
          refreshed.data?.message ||
            t('Saved, but failed to reload normalized settings')
        )
        return
      }

      const normalizedDocument =
        refreshed.data.data.find((option) => option.key === 'RequestBlacklist')
          ?.value ?? '{"mode":"off"}'
      const normalizedValues = parseRequestBlacklistDocument(normalizedDocument)
      baselineDocumentRef.current =
        serializeRequestBlacklistDocument(normalizedValues)
      form.reset(normalizedValues)
    } catch {
      // useUpdateOption reports transport errors.
    }
  }

  const resetToServerValues = () => {
    form.reset(parseRequestBlacklistDocument(baselineDocumentRef.current))
  }

  return (
    <SettingsSection
      title={t('Request Content and Domain Blacklist')}
      description={t(
        'Inspect request text for configured keywords and domain names before relay.'
      )}
    >
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            onReset={resetToServerValues}
            isSaving={updateOption.isPending || systemOptions.isFetching}
            isResetDisabled={!form.formState.isDirty}
            saveLabel='Save request blacklist settings'
          />

          <FormField
            control={form.control}
            name='mode'
            render={({ field }) => (
              <FormItem data-settings-form-span='full'>
                <FormLabel>{t('Mode')}</FormLabel>
                <FormControl>
                  <ToggleGroup
                    value={[field.value]}
                    onValueChange={(value) => {
                      if (value[0]) field.onChange(value[0])
                    }}
                    variant='outline'
                    className='w-full sm:w-fit'
                  >
                    <ToggleGroupItem
                      value='off'
                      className='flex-1 sm:flex-none'
                    >
                      {t('Off')}
                    </ToggleGroupItem>
                    <ToggleGroupItem
                      value='observe'
                      className='flex-1 sm:flex-none'
                    >
                      {t('Observe')}
                    </ToggleGroupItem>
                    <ToggleGroupItem
                      value='enforce'
                      className='flex-1 sm:flex-none'
                    >
                      {t('Enforce')}
                    </ToggleGroupItem>
                  </ToggleGroup>
                </FormControl>
                <FormDescription>
                  {field.value === 'off' &&
                    t('Request blacklist inspection is disabled.')}
                  {field.value === 'observe' &&
                    t('Observe only records audit logs and allows requests.')}
                  {field.value === 'enforce' &&
                    t(
                      'Matching requests are recorded and blocked before relay.'
                    )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='blockStatusCode'
            render={({ field }) => (
              <FormItem data-settings-form-span='full'>
                <FormLabel>{t('Block status code')}</FormLabel>
                <FormControl>
                  <RadioGroup
                    value={String(field.value)}
                    onValueChange={(value) => field.onChange(Number(value))}
                    className='grid gap-3 lg:grid-cols-3'
                  >
                    <label className='border-input flex cursor-pointer items-start gap-3 rounded-lg border p-3'>
                      <RadioGroupItem value='403' className='mt-0.5' />
                      <span className='min-w-0 space-y-1'>
                        <span className='block text-sm font-medium'>
                          {t('403 (default)')}
                        </span>
                        <span className='text-muted-foreground block text-xs'>
                          {t(
                            'Matches this policy: the request is rejected and clients should not retry.'
                          )}
                        </span>
                      </span>
                    </label>
                    <label className='border-input flex cursor-pointer items-start gap-3 rounded-lg border p-3'>
                      <RadioGroupItem value='400' className='mt-0.5' />
                      <span className='min-w-0 space-y-1'>
                        <span className='block text-sm font-medium'>400</span>
                        <span className='text-muted-foreground block text-xs'>
                          {t(
                            'Marks the request content as invalid; some SDKs surface it directly as a parameter error.'
                          )}
                        </span>
                      </span>
                    </label>
                    <label className='border-input flex cursor-pointer items-start gap-3 rounded-lg border p-3'>
                      <RadioGroupItem value='503' className='mt-0.5' />
                      <span className='min-w-0 space-y-1'>
                        <span className='flex items-center gap-1.5 text-sm font-medium'>
                          <TriangleAlert
                            className='size-4'
                            aria-hidden='true'
                          />
                          503
                        </span>
                        <span className='text-muted-foreground block text-xs'>
                          {t(
                            'Client retry mechanisms will resend the same blacklisted content and waste compute.'
                          )}
                        </span>
                      </span>
                    </label>
                  </RadioGroup>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='enabledGroups'
            render={({ field }) => (
              <FormItem data-settings-form-span='full'>
                <FormLabel>{t('Enabled groups')}</FormLabel>
                <FormControl>
                  <MultiSelect
                    options={groupOptions}
                    selected={field.value}
                    onChange={field.onChange}
                    placeholder={t('All groups')}
                    maxVisibleChips={8}
                  />
                </FormControl>
                <FormDescription>
                  {t('Leave empty to apply to all groups.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='contentWords'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Blocked content keywords')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={8}
                    placeholder={t('Enter one keyword per line')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Enter one content keyword per line.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='domains'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Blocked domains')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={8}
                    placeholder={t('example.com')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Enter one hostname per line without http://, paths, or wildcards. example.com matches itself and any subdomain, but not notexample.com or example.com.evil.com.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='blockMessage'
            render={({ field }) => (
              <FormItem data-settings-form-span='full'>
                <FormLabel>{t('Block message')}</FormLabel>
                <FormControl>
                  <Input {...field} />
                </FormControl>
                <FormDescription>
                  {t(
                    'This message is returned with the request ID. Do not include domains, URLs, IP addresses, or keys, or saving will be rejected.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Alert variant='warning'>
            <TriangleAlert aria-hidden='true' />
            <AlertTitle>{t('Coverage and limitations')}</AlertTitle>
            <AlertDescription>
              <ul className='list-disc space-y-1 pl-4'>
                <li>
                  {t(
                    'Only request text is scanned. Domain rules apply only when a domain appears as text in the prompt.'
                  )}
                </li>
                <li>
                  {t(
                    'Not covered: structured URL fields such as image_url, file, and video_url; Midjourney and video or music task prompts; /v1/alpha/search; Realtime voice sessions; transcription prompts; uploaded file contents; and base64.'
                  )}
                </li>
                <li>
                  {t(
                    'This cannot stop short links, redirects, encoding obfuscation, OCR, or targets generated autonomously by an upstream model.'
                  )}
                </li>
                <li>
                  {t(
                    'Unicode domains must be entered as punycode. 例え.jp is saved as xn--r8jz45g.jp; prompt text 例え.jp will not match, while xn--r8jz45g.jp will.'
                  )}
                </li>
                <li>
                  {t(
                    'Real outbound authorization must be enforced where HTTP requests are made, at the egress or tool layer. This feature is not complete penetration protection.'
                  )}
                </li>
              </ul>
            </AlertDescription>
          </Alert>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
