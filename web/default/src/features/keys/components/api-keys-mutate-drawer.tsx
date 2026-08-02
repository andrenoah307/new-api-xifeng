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
import {
  CalendarClock,
  ChevronDown,
  KeyRound,
  Settings2,
  WalletCards,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useForm, type SubmitErrorHandler } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DateTimePicker } from '@/components/datetime-picker'
import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
  sideDrawerSwitchItemClassName,
} from '@/components/drawer-layout'
import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { useStatus } from '@/hooks/use-status'
import { getUserModels, getUserGroups } from '@/lib/api'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { cn } from '@/lib/utils'
import { DEFAULT_CURRENCY_CONFIG } from '@/stores/system-config-store'

import { createApiKey, updateApiKey, getApiKey } from '../api'
import { ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import {
  getApiKeyFormSchema,
  type ApiKeyFormValues,
  getApiKeyFormDefaultValues,
  transformFormDataToPayload,
  transformApiKeyToFormDefaults,
} from '../lib/api-key-form'
import {
  cnyToCanonicalQuota,
  convertPeriodLimitUnit,
  formatPeriodResetAt,
  getPeriodResetAt,
  isPositiveIntegerString,
} from '../lib/token-period'
import {
  TOKEN_PERIOD_MAX_DAYS,
  type ApiKey,
  type TokenPeriodLimitUnit,
  type TokenPeriodType,
} from '../types'
import {
  ApiKeyGroupCombobox,
  type ApiKeyGroupOption,
} from './api-key-group-combobox'
import { useApiKeys } from './api-keys-provider'

type ApiKeyMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: ApiKey
}

export function ApiKeysMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: ApiKeyMutateDrawerProps) {
  const { t, i18n } = useTranslation()
  const isUpdate = !!currentRow
  const { triggerRefresh } = useApiKeys()
  const { status } = useStatus()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const periodCanonicalQuotaRef = useRef<number | null>(null)
  const defaultUseAutoGroup = status?.default_use_auto_group === true
  const statusUsdExchangeRate = Number(status?.usd_exchange_rate)
  const statusQuotaPerUnit = Number(status?.quota_per_unit)
  const usdExchangeRate =
    Number.isFinite(statusUsdExchangeRate) && statusUsdExchangeRate > 0
      ? statusUsdExchangeRate
      : 1
  const quotaPerUnit =
    Number.isFinite(statusQuotaPerUnit) && statusQuotaPerUnit > 0
      ? statusQuotaPerUnit
      : DEFAULT_CURRENCY_CONFIG.quotaPerUnit

  // Fetch models
  const { data: modelsData } = useQuery({
    queryKey: ['user-models'],
    queryFn: getUserModels,
    enabled: open,
    staleTime: 0,
  })

  // Fetch groups
  const { data: groupsData } = useQuery({
    queryKey: ['user-groups'],
    queryFn: getUserGroups,
    enabled: open,
    staleTime: 0,
  })

  const models = Array.isArray(modelsData?.data) ? modelsData.data : []
  const groupsRaw = groupsData?.data || {}
  const regionBlockedGroups: string[] = status?.region_blocked_groups ?? []
  const groups: ApiKeyGroupOption[] = Object.entries(groupsRaw)
    .filter(([key]) => !regionBlockedGroups.includes(key))
    .map(([key, info]) => ({
      value: key,
      label: key,
      desc: info.desc || key,
      ratio: info.ratio,
    }))
  const backendHasAuto = groups.some((g) => g.value === 'auto')
  const schema = getApiKeyFormSchema(t)

  const form = useForm<ApiKeyFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getApiKeyFormDefaultValues(defaultUseAutoGroup),
  })

  // Load existing data when updating
  useEffect(() => {
    if (open && isUpdate && currentRow) {
      void getApiKey(currentRow.id).then((result) => {
        if (result.success && result.data) {
          periodCanonicalQuotaRef.current =
            result.data.period_quota_limit > 0
              ? result.data.period_quota_limit
              : null
          form.reset(
            transformApiKeyToFormDefaults(result.data, {
              usdExchangeRate,
              quotaPerUnit,
            })
          )
        }
      })
    } else if (open && !isUpdate) {
      periodCanonicalQuotaRef.current = null
      form.reset(
        getApiKeyFormDefaultValues(defaultUseAutoGroup && backendHasAuto)
      )
    }
  }, [
    open,
    isUpdate,
    currentRow,
    form,
    defaultUseAutoGroup,
    backendHasAuto,
    usdExchangeRate,
    quotaPerUnit,
  ])

  // Correct group after groups load: if the form value is not in available groups, fall back
  useEffect(() => {
    if (groups.length === 0) return
    const currentGroup = form.getValues('group')
    if (currentGroup && !groups.some((g) => g.value === currentGroup)) {
      const fallback =
        groups.find((g) => g.value === 'default')?.value ??
        groups[0]?.value ??
        ''
      form.setValue('group', fallback)
      if (currentGroup === 'auto') {
        form.setValue('cross_group_retry', false)
      }
    }
  }, [groups, form])

  const onSubmit = async (data: ApiKeyFormValues) => {
    setIsSubmitting(true)
    try {
      const basePayload = transformFormDataToPayload(data)

      if (isUpdate && currentRow) {
        const result = await updateApiKey({
          ...basePayload,
          id: currentRow.id,
        })
        if (result.success) {
          toast.success(t(SUCCESS_MESSAGES.API_KEY_UPDATED))
          onOpenChange(false)
          triggerRefresh()
        } else {
          toast.error(result.message || t(ERROR_MESSAGES.UPDATE_FAILED))
        }
      } else {
        // Create mode - handle batch creation
        const count = data.tokenCount || 1
        let successCount = 0

        for (let i = 0; i < count; i++) {
          const result = await createApiKey({
            ...basePayload,
            name:
              i === 0 && data.name
                ? data.name
                : `${data.name || 'default'}-${Math.random().toString(36).slice(2, 8)}`,
          })
          if (result.success) {
            successCount++
          } else {
            toast.error(result.message || t(ERROR_MESSAGES.CREATE_FAILED))
            break
          }
        }

        if (successCount > 0) {
          toast.success(
            t('Successfully created {{count}} API Key(s)', {
              count: successCount,
            })
          )
          onOpenChange(false)
          triggerRefresh()
        }
      }
    } catch {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsSubmitting(false)
    }
  }

  const onInvalid: SubmitErrorHandler<ApiKeyFormValues> = () => {
    toast.error(t('Please fix the highlighted fields before saving'))
  }

  const handleSetExpiry = (months: number, days: number, hours: number) => {
    if (months === 0 && days === 0 && hours === 0) {
      form.setValue('expired_time', undefined)
      return
    }

    const now = new Date()
    now.setMonth(now.getMonth() + months)
    now.setDate(now.getDate() + days)
    now.setHours(now.getHours() + hours)

    form.setValue('expired_time', now)
  }

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const quotaLabel = t('Quota ({{currency}})', { currency: currencyLabel })
  const quotaPlaceholder = tokensOnly
    ? t('Enter quota in tokens')
    : t('Enter quota in {{currency}}', { currency: currencyLabel })
  const watchedValues = form.watch()
  const selectedGroup = watchedValues.group
  const unlimitedQuota = watchedValues.unlimited_quota
  const periodType = watchedValues.period_type
  const periodUnit = watchedValues.period_limit_unit
  const periodEnabled = periodType !== ''
  const storedPeriodResetAt = watchedValues.period_reset_at ?? 0
  const periodResetAt = periodEnabled
    ? storedPeriodResetAt > 0
      ? storedPeriodResetAt
      : getPeriodResetAt(
          periodType,
          periodType === 'days' ? watchedValues.period_days : 0
        )
    : 0

  const periodTypeOptions: Array<{
    value: Exclude<TokenPeriodType, ''>
    label: string
  }> = [
    { value: 'days', label: t('Every N days') },
    { value: 'week', label: t('Every week') },
    { value: 'month', label: t('Every month') },
  ]

  const setPeriodType = (nextType: TokenPeriodType) => {
    const nextDays = nextType === 'days' ? watchedValues.period_days || 1 : 0
    form.setValue('period_type', nextType, {
      shouldDirty: true,
      shouldValidate: true,
    })
    form.setValue('period_days', nextDays, {
      shouldDirty: true,
      shouldValidate: true,
    })
    form.setValue(
      'period_reset_at',
      nextType === ''
        ? 0
        : getPeriodResetAt(nextType, nextType === 'days' ? nextDays : 0),
      { shouldDirty: true }
    )
  }

  const handlePeriodEnabledChange = (enabled: boolean) => {
    if (!enabled) {
      setPeriodType('')
      form.setValue('period_limit_value', '0', {
        shouldDirty: true,
        shouldValidate: true,
      })
      periodCanonicalQuotaRef.current = null
      return
    }

    const nextType: TokenPeriodType = 'week'
    setPeriodType(nextType)
    const currentValue = watchedValues.period_limit_value.trim()
    let nextValue = currentValue
    if (!nextValue || nextValue === '0') {
      nextValue =
        periodUnit === 'cny' ? '10.00' : String(Math.trunc(quotaPerUnit))
    }
    form.setValue('period_limit_value', nextValue, {
      shouldDirty: true,
      shouldValidate: true,
    })
    periodCanonicalQuotaRef.current =
      periodUnit === 'cny'
        ? cnyToCanonicalQuota(nextValue, usdExchangeRate, quotaPerUnit)
        : Number(nextValue)
  }

  const handlePeriodUnitChange = (nextUnit: TokenPeriodLimitUnit) => {
    const currentValue = form.getValues('period_limit_value').trim()
    const converted = convertPeriodLimitUnit(
      currentValue,
      periodUnit,
      nextUnit,
      { usdExchangeRate, quotaPerUnit },
      periodCanonicalQuotaRef.current
    )
    periodCanonicalQuotaRef.current = converted.canonicalQuota

    form.setValue('period_limit_unit', nextUnit, {
      shouldDirty: true,
      shouldValidate: true,
    })
    form.setValue('period_limit_value', converted.value, {
      shouldDirty: true,
      shouldValidate: true,
    })
  }

  const handlePeriodLimitValueChange = (value: string) => {
    const trimmedValue = value.trim()
    let canonicalQuota = 0
    if (periodUnit === 'cny') {
      canonicalQuota = cnyToCanonicalQuota(
        trimmedValue,
        usdExchangeRate,
        quotaPerUnit
      )
    } else if (isPositiveIntegerString(trimmedValue)) {
      canonicalQuota = Number(trimmedValue)
    }
    periodCanonicalQuotaRef.current = Number.isFinite(canonicalQuota)
      ? canonicalQuota
      : null
    form.setValue('period_limit_value', value, {
      shouldDirty: true,
      shouldValidate: true,
    })
  }

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) {
          periodCanonicalQuotaRef.current = null
          form.reset()
        }
      }}
    >
      <SheetContent
        className={sideDrawerContentClassName('max-w-none sm:!max-w-[620px]')}
      >
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isUpdate ? t('Update API Key') : t('Create API Key')}
          </SheetTitle>
          <SheetDescription>
            {isUpdate
              ? t('Update the API key by providing necessary info.')
              : t('Add a new API key by providing necessary info.')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='api-key-form'
            onSubmit={form.handleSubmit(onSubmit, onInvalid)}
            className={sideDrawerFormClassName('gap-5')}
          >
            <SideDrawerSection>
              <SideDrawerSectionHeader
                title={t('Basic Information')}
                description={t('Set API key basic information')}
                icon={<KeyRound className='size-4' />}
                iconTone='info'
              />
              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Name')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder={t('Enter a name')} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='group'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Group')}</FormLabel>
                    <FormControl>
                      <ApiKeyGroupCombobox
                        options={groups}
                        value={field.value}
                        onValueChange={field.onChange}
                        placeholder={t('Select a group')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {selectedGroup === 'auto' && (
                <FormField
                  control={form.control}
                  name='cross_group_retry'
                  render={({ field }) => (
                    <FormItem className={sideDrawerSwitchItemClassName()}>
                      <div className='flex flex-col gap-0.5'>
                        <FormLabel className='text-sm'>
                          {t('Cross-group retry')}
                        </FormLabel>
                        <FormDescription className='line-clamp-2 text-xs sm:line-clamp-none'>
                          {t(
                            'When enabled, if channels in the current group fail, it will try channels in the next group in order.'
                          )}
                        </FormDescription>
                      </div>
                      <FormControl>
                        <Switch
                          checked={!!field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
              )}

              <FormField
                control={form.control}
                name='expired_time'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Expiration Time')}</FormLabel>
                    <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center'>
                      <FormControl>
                        <DateTimePicker
                          value={field.value}
                          onChange={field.onChange}
                          placeholder={t('Never expires')}
                          className='min-w-0 [&_input[type=time]]:w-24 sm:[&_input[type=time]]:w-32'
                        />
                      </FormControl>
                      <div className='grid grid-cols-4 gap-2 sm:flex'>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 0, 0)}
                        >
                          {t('Never')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(1, 0, 0)}
                        >
                          {t('1 Month')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 1, 0)}
                        >
                          {t('1 Day')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 0, 1)}
                        >
                          {t('1 Hour')}
                        </Button>
                      </div>
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {!isUpdate && (
                <FormField
                  control={form.control}
                  name='tokenCount'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Quantity')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min='1'
                          placeholder={t('Number of keys to create')}
                          onChange={(e) =>
                            field.onChange(
                              Number.parseInt(e.target.value, 10) || 1
                            )
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Create multiple API keys at once (random suffix will be added to names)'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </SideDrawerSection>

            <SideDrawerSection>
              <SideDrawerSectionHeader
                title={t('Quota Settings')}
                description={t('Set quota amount and limits')}
                icon={<WalletCards className='size-4' />}
                iconTone='success'
              />
              {!unlimitedQuota && (
                <FormField
                  control={form.control}
                  name='remain_quota_dollars'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{quotaLabel}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          step={tokensOnly ? 1 : 0.01}
                          placeholder={quotaPlaceholder}
                          onChange={(e) =>
                            field.onChange(
                              Number.parseFloat(e.target.value) || 0
                            )
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {tokensOnly
                          ? t('Enter the quota amount in tokens')
                          : t('Enter the quota amount in {{currency}}', {
                              currency: currencyLabel,
                            })}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              <FormField
                control={form.control}
                name='unlimited_quota'
                render={({ field }) => (
                  <FormItem className={sideDrawerSwitchItemClassName()}>
                    <div className='flex flex-col gap-0.5'>
                      <FormLabel className='text-sm'>
                        {t('Unlimited Quota')}
                      </FormLabel>
                      <FormDescription className='text-xs'>
                        {t('Enable unlimited quota for this API key')}
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
            </SideDrawerSection>

            <SideDrawerSection>
              <SideDrawerSectionHeader
                title={t('Period Quota')}
                description={t('Set a recurring quota limit for this API key')}
                icon={<CalendarClock className='size-4' />}
                iconTone='warning'
              />

              <FormField
                control={form.control}
                name='period_type'
                render={() => (
                  <FormItem className={sideDrawerSwitchItemClassName()}>
                    <div className='flex flex-col gap-0.5'>
                      <FormLabel className='text-sm'>
                        {t('Enable period quota')}
                      </FormLabel>
                      <FormDescription className='text-xs'>
                        {t('Limit usage independently for each billing cycle')}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={periodEnabled}
                        onCheckedChange={handlePeriodEnabledChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />

              {periodEnabled && (
                <div className='flex flex-col gap-4'>
                  <FormField
                    control={form.control}
                    name='period_type'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Period type')}</FormLabel>
                        <Select
                          items={periodTypeOptions}
                          value={field.value}
                          onValueChange={(value) => {
                            if (
                              value === 'days' ||
                              value === 'week' ||
                              value === 'month'
                            ) {
                              setPeriodType(value)
                            }
                          }}
                        >
                          <FormControl>
                            <SelectTrigger className='w-full'>
                              <SelectValue />
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent alignItemWithTrigger={false}>
                            <SelectGroup>
                              {periodTypeOptions.map((option) => (
                                <SelectItem
                                  key={option.value}
                                  value={option.value}
                                >
                                  {option.label}
                                </SelectItem>
                              ))}
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  {periodType === 'days' && (
                    <FormField
                      control={form.control}
                      name='period_days'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Number of days')}</FormLabel>
                          <FormControl>
                            <Input
                              {...field}
                              type='number'
                              min={1}
                              max={TOKEN_PERIOD_MAX_DAYS}
                              step={1}
                              onChange={(event) => {
                                const days =
                                  Number.parseInt(event.target.value, 10) || 0
                                field.onChange(days)
                                form.setValue(
                                  'period_reset_at',
                                  getPeriodResetAt('days', days),
                                  { shouldDirty: true }
                                )
                              }}
                            />
                          </FormControl>
                          <FormDescription>
                            {t('Choose a value from 1 to 3650 days')}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  )}

                  <FormField
                    control={form.control}
                    name='period_limit_unit'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Limit unit')}</FormLabel>
                        <FormControl>
                          <ToggleGroup
                            value={[field.value]}
                            onValueChange={(values) => {
                              const nextUnit = values[0]
                              if (nextUnit === 'cny' || nextUnit === 'quota') {
                                handlePeriodUnitChange(nextUnit)
                              }
                            }}
                            variant='outline'
                            size='sm'
                            spacing={1}
                            className='w-full'
                            aria-label={t('Limit unit')}
                          >
                            <ToggleGroupItem value='cny' className='flex-1'>
                              {t('CNY (¥)')}
                            </ToggleGroupItem>
                            <ToggleGroupItem value='quota' className='flex-1'>
                              {t('Native quota')}
                            </ToggleGroupItem>
                          </ToggleGroup>
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='period_limit_value'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {periodUnit === 'cny'
                            ? t('Period limit (¥)')
                            : t('Period limit (quota)')}
                        </FormLabel>
                        <FormControl>
                          <Input
                            value={field.value}
                            type='text'
                            inputMode={
                              periodUnit === 'quota' ? 'numeric' : 'decimal'
                            }
                            placeholder={
                              periodUnit === 'cny'
                                ? '10.00'
                                : String(Math.trunc(quotaPerUnit))
                            }
                            onChange={(event) =>
                              handlePeriodLimitValueChange(event.target.value)
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t('The limit must be greater than zero')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <div className='bg-muted/40 rounded-md border px-3 py-2 text-sm'>
                    <span className='text-muted-foreground'>
                      {t('Next reset')}:{' '}
                    </span>
                    <span className='font-medium tabular-nums'>
                      {formatPeriodResetAt(
                        periodResetAt,
                        i18n.resolvedLanguage || i18n.language
                      )}
                    </span>
                  </div>
                </div>
              )}
            </SideDrawerSection>

            <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
              <SideDrawerSection>
                <CollapsibleTrigger
                  render={
                    <button
                      type='button'
                      className='hover:bg-muted/40 flex w-full items-center gap-3 rounded-md py-1.5 text-left transition-colors'
                    />
                  }
                >
                  <SideDrawerSectionHeader
                    className='flex-1'
                    title={t('Advanced Settings')}
                    description={t('Set API key access restrictions')}
                    icon={<Settings2 className='size-4' />}
                  />
                  <ChevronDown
                    className={cn(
                      'text-muted-foreground size-4 shrink-0 transition-transform',
                      advancedOpen && 'rotate-180'
                    )}
                  />
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <div className='flex flex-col gap-4 pt-2'>
                    <FormField
                      control={form.control}
                      name='model_limits'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Model Limits')}</FormLabel>
                          <FormControl>
                            <MultiSelect
                              options={models.map((m) => ({
                                label: m,
                                value: m,
                              }))}
                              selected={field.value}
                              onChange={field.onChange}
                              placeholder={t(
                                'Select models (empty for allow all)'
                              )}
                            />
                          </FormControl>
                          <FormDescription>
                            {t('Limit which models can be used with this key')}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <FormField
                      control={form.control}
                      name='allow_ips'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>
                            {t('IP Whitelist (supports CIDR)')}
                          </FormLabel>
                          <FormControl>
                            <Textarea
                              {...field}
                              className='min-h-20 resize-none'
                              placeholder={t(
                                'One IP per line (empty for no restriction)'
                              )}
                              rows={3}
                            />
                          </FormControl>
                          <FormDescription>
                            {t(
                              'Do not over-trust this feature. IP may be spoofed. Please use with nginx, CDN and other gateways.'
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>
                </CollapsibleContent>
              </SideDrawerSection>
            </Collapsible>
          </form>
        </Form>
        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose
            render={<Button variant='outline' className='w-full sm:w-auto' />}
          >
            {t('Close')}
          </SheetClose>
          <Button
            type='button'
            onClick={form.handleSubmit(onSubmit, onInvalid)}
            disabled={isSubmitting}
            className='w-full sm:w-auto'
          >
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
