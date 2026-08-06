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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
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
import { Progress } from '@/components/ui/progress'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'

/**
 * IMPORTANT: react-hook-form 7 interprets dotted `name` strings as nested
 * paths. If we declare the schema with literal flat keys like
 * `'performance_setting.disk_cache_enabled'`, the form state diverges from
 * what zod validates and saves silently turn into no-ops. So we model the
 * form internally with proper nested objects and only flatten back to the
 * server-side key format right before persisting.
 */
const perfSchema = z.object({
  performance_setting: z.object({
    disk_cache_enabled: z.boolean(),
    disk_cache_threshold_mb: z.coerce.number().min(1),
    disk_cache_max_size_mb: z.coerce.number().min(100),
    disk_cache_path: z.string(),
    monitor_enabled: z.boolean(),
    monitor_cpu_threshold: z.coerce.number().min(0),
    monitor_memory_threshold: z.coerce.number().min(0).max(100),
    monitor_disk_threshold: z.coerce.number().min(0).max(100),
  }),
})

type PerfFormInput = z.input<typeof perfSchema>
type PerfFormValues = z.output<typeof perfSchema>

type FlatPerfDefaults = {
  'performance_setting.disk_cache_enabled': boolean
  'performance_setting.disk_cache_threshold_mb': number
  'performance_setting.disk_cache_max_size_mb': number
  'performance_setting.disk_cache_path': string
  'performance_setting.monitor_enabled': boolean
  'performance_setting.monitor_cpu_threshold': number
  'performance_setting.monitor_memory_threshold': number
  'performance_setting.monitor_disk_threshold': number
}

const buildFormDefaults = (defaults: FlatPerfDefaults): PerfFormInput => ({
  performance_setting: {
    disk_cache_enabled: defaults['performance_setting.disk_cache_enabled'],
    disk_cache_threshold_mb:
      defaults['performance_setting.disk_cache_threshold_mb'],
    disk_cache_max_size_mb:
      defaults['performance_setting.disk_cache_max_size_mb'],
    disk_cache_path: defaults['performance_setting.disk_cache_path'] ?? '',
    monitor_enabled: defaults['performance_setting.monitor_enabled'],
    monitor_cpu_threshold:
      defaults['performance_setting.monitor_cpu_threshold'],
    monitor_memory_threshold:
      defaults['performance_setting.monitor_memory_threshold'],
    monitor_disk_threshold:
      defaults['performance_setting.monitor_disk_threshold'],
  },
})

const normalizeFormValues = (values: PerfFormValues): FlatPerfDefaults => ({
  'performance_setting.disk_cache_enabled':
    values.performance_setting.disk_cache_enabled,
  'performance_setting.disk_cache_threshold_mb':
    values.performance_setting.disk_cache_threshold_mb,
  'performance_setting.disk_cache_max_size_mb':
    values.performance_setting.disk_cache_max_size_mb,
  'performance_setting.disk_cache_path':
    values.performance_setting.disk_cache_path ?? '',
  'performance_setting.monitor_enabled':
    values.performance_setting.monitor_enabled,
  'performance_setting.monitor_cpu_threshold':
    values.performance_setting.monitor_cpu_threshold,
  'performance_setting.monitor_memory_threshold':
    values.performance_setting.monitor_memory_threshold,
  'performance_setting.monitor_disk_threshold':
    values.performance_setting.monitor_disk_threshold,
})

function formatBytes(bytes: number, decimals = 2): string {
  if (!bytes || Number.isNaN(bytes)) return '0 Bytes'
  if (bytes === 0) return '0 Bytes'
  if (bytes < 0) return `-${formatBytes(-bytes, decimals)}`
  const k = 1024
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k))
  if (i < 0 || i >= sizes.length) return `${bytes} Bytes`
  return `${Number.parseFloat((bytes / Math.pow(k, i)).toFixed(decimals))} ${
    sizes[i]
  }`
}

function formatElapsedDuration(
  seconds: number,
  translate: (key: string) => string
): string {
  const totalSeconds = Math.max(0, Math.floor(seconds))
  const days = Math.floor(totalSeconds / 86400)
  const hours = Math.floor((totalSeconds % 86400) / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const remainingSeconds = totalSeconds % 60
  const parts: string[] = []

  if (days > 0) parts.push(`${days} ${translate('days')}`)
  if (hours > 0) parts.push(`${hours}${translate('h')}`)
  if (minutes > 0) parts.push(`${minutes}${translate('m')}`)
  if (remainingSeconds > 0 || parts.length === 0) {
    parts.push(`${remainingSeconds}${translate('s')}`)
  }

  return parts.join(' ')
}

interface Props {
  defaultValues: FlatPerfDefaults
}

type PerformanceStats = {
  cache_stats?: {
    current_disk_usage_bytes: number
    disk_cache_max_bytes: number
    active_disk_files: number
    disk_cache_hits: number
    current_memory_usage_bytes: number
    active_memory_buffers: number
    memory_cache_hits: number
  }
  disk_space_info?: {
    total: number
    free: number
    used: number
    used_percent: number
  }
  memory_stats?: {
    alloc: number
    total_alloc: number
    sys: number
    num_gc: number
    num_goroutine: number
  }
  relay_admission?: {
    active_requests?: number
    active_body_bytes?: number
    max_concurrent_requests?: number
    max_active_body_bytes?: number
    rejected_too_many_concurrent_requests?: number
    rejected_request_body_budget_exhausted?: number
    rejected_memory_pressure?: number
    exempted_memory_pressure?: number
  }
  system_gate?: {
    rejected_cpu_overloaded?: number
    rejected_memory_overloaded?: number
    rejected_disk_overloaded?: number
  }
  cgroup_memory?: {
    available?: boolean
    usage_bytes?: number
    limit_bytes?: number
    usage_permille?: number
    high_percent?: number
    low_percent?: number
    tripped?: boolean
    tripped_since_unix?: number
    trip_count?: number
    forced_reset_count?: number
    max_trip_seconds?: number
    disarmed?: boolean
  }
  disk_cache_info?: {
    path: string
    file_count: number
    total_size: number
  }
  config?: {
    is_running_in_container: boolean
  }
}

export function PerformanceSection(props: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [stats, setStats] = useState<PerformanceStats | null>(null)

  const formDefaults = useMemo(
    () => buildFormDefaults(props.defaultValues),
    [props.defaultValues]
  )

  const form = useForm<PerfFormInput, unknown, PerfFormValues>({
    resolver: zodResolver(perfSchema),
    defaultValues: formDefaults,
  })

  const baselineRef = useRef<FlatPerfDefaults>(props.defaultValues)
  const baselineSerializedRef = useRef<string>(
    JSON.stringify(props.defaultValues)
  )

  useEffect(() => {
    const serialized = JSON.stringify(props.defaultValues)
    if (serialized === baselineSerializedRef.current) return
    baselineRef.current = props.defaultValues
    baselineSerializedRef.current = serialized
    form.reset(buildFormDefaults(props.defaultValues))
  }, [props.defaultValues, form])

  const fetchStats = useCallback(async () => {
    try {
      const res = await api.get('/api/performance/stats')
      if (res.data.success) setStats(res.data.data)
    } catch {
      /* ignore */
    }
  }, [])

  useEffect(() => {
    fetchStats()
  }, [fetchStats])

  const onSubmit = async (values: PerfFormValues) => {
    const normalized = normalizeFormValues(values)
    const changedKeys = (
      Object.keys(normalized) as Array<keyof FlatPerfDefaults>
    ).filter((key) => normalized[key] !== baselineRef.current[key])

    if (changedKeys.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of changedKeys) {
      await updateOption.mutateAsync({
        key,
        value: normalized[key],
      })
    }

    baselineRef.current = normalized
    baselineSerializedRef.current = JSON.stringify(normalized)
    form.reset(buildFormDefaults(normalized))
    fetchStats()
  }

  const clearDiskCache = async () => {
    try {
      const res = await api.delete('/api/performance/disk_cache')
      if (res.data.success) {
        toast.success(t('Disk cache cleared'))
        fetchStats()
      }
    } catch {
      toast.error(t('Cleanup failed'))
    }
  }

  const resetStats = async () => {
    try {
      const res = await api.post('/api/performance/reset_stats')
      if (res.data.success) {
        toast.success(t('Statistics reset'))
        fetchStats()
      }
    } catch {
      toast.error(t('Reset failed'))
    }
  }

  const forceGC = async () => {
    try {
      const res = await api.post('/api/performance/gc')
      if (res.data.success) {
        toast.success(t('GC executed'))
        fetchStats()
      }
    } catch {
      toast.error(t('GC execution failed'))
    }
  }

  const diskEnabled = form.watch('performance_setting.disk_cache_enabled')
  const monitorEnabled = form.watch('performance_setting.monitor_enabled')
  const maxCacheSizeRaw = form.watch(
    'performance_setting.disk_cache_max_size_mb'
  )
  const maxCacheSizeMb =
    typeof maxCacheSizeRaw === 'number'
      ? maxCacheSizeRaw
      : Number(maxCacheSizeRaw) || 0

  const lowDiskSpace =
    diskEnabled &&
    stats?.disk_space_info &&
    stats.disk_space_info.free > 0 &&
    maxCacheSizeMb > 0 &&
    stats.disk_space_info.free < maxCacheSizeMb * 1024 * 1024

  const diskCachePercent =
    stats?.cache_stats?.disk_cache_max_bytes &&
    stats.cache_stats.disk_cache_max_bytes > 0
      ? Math.round(
          (stats.cache_stats.current_disk_usage_bytes /
            stats.cache_stats.disk_cache_max_bytes) *
            100
        )
      : 0

  const relayRequestUsage =
    stats?.relay_admission?.active_requests !== undefined &&
    stats.relay_admission.max_concurrent_requests !== undefined
      ? `${stats.relay_admission.active_requests} / ${stats.relay_admission.max_concurrent_requests}`
      : t('N/A')
  const relayBodyUsage =
    stats?.relay_admission?.active_body_bytes !== undefined &&
    stats.relay_admission.max_active_body_bytes !== undefined
      ? `${formatBytes(stats.relay_admission.active_body_bytes)} / ${formatBytes(stats.relay_admission.max_active_body_bytes)}`
      : t('N/A')
  const cgroupMemoryUsage =
    stats?.cgroup_memory?.usage_bytes !== undefined &&
    stats.cgroup_memory.limit_bytes !== undefined
      ? `${formatBytes(stats.cgroup_memory.usage_bytes)} / ${formatBytes(stats.cgroup_memory.limit_bytes)}`
      : t('N/A')
  const cgroupMemoryPercent =
    stats?.cgroup_memory?.usage_permille !== undefined
      ? `${(stats.cgroup_memory.usage_permille / 10).toFixed(1)}%`
      : t('N/A')
  const cgroupMemoryThresholds =
    stats?.cgroup_memory?.high_percent !== undefined &&
    stats.cgroup_memory.low_percent !== undefined
      ? `${stats.cgroup_memory.high_percent}% / ${stats.cgroup_memory.low_percent}%`
      : t('N/A')

  const maxTripSeconds = stats?.cgroup_memory?.max_trip_seconds
  let cgroupMaxTripDuration = t('N/A')
  if (typeof maxTripSeconds === 'number' && Number.isFinite(maxTripSeconds)) {
    cgroupMaxTripDuration =
      maxTripSeconds === 0
        ? t('Not enabled')
        : `${maxTripSeconds} ${t('seconds')}`
  }
  const trippedSinceUnix = stats?.cgroup_memory?.tripped_since_unix
  const cgroupTrippedDuration =
    typeof trippedSinceUnix === 'number' &&
    Number.isFinite(trippedSinceUnix) &&
    trippedSinceUnix > 0
      ? formatElapsedDuration(
          Math.floor(Date.now() / 1000) - trippedSinceUnix,
          t
        )
      : null

  let cgroupStatusVariant: StatusVariant = 'neutral'
  let cgroupStatusLabel = t('N/A')
  if (stats?.cgroup_memory?.disarmed === true) {
    cgroupStatusVariant = 'danger'
    cgroupStatusLabel = t('Disarmed')
  } else if (stats?.cgroup_memory?.tripped === true) {
    cgroupStatusVariant = 'danger'
    cgroupStatusLabel = t('Tripped')
  } else if (
    stats?.cgroup_memory?.tripped === false &&
    stats?.cgroup_memory?.available !== false
  ) {
    cgroupStatusVariant = 'success'
    cgroupStatusLabel = t('Normal')
  }

  return (
    <SettingsSection title={t('Performance Settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          {/* Disk Cache Settings */}
          <div>
            <h4 className='font-medium'>{t('Disk Cache Settings')}</h4>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'When enabled, large request bodies are temporarily stored on disk instead of memory, significantly reducing memory usage. SSD recommended.'
              )}
            </p>
          </div>

          <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='performance_setting.disk_cache_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable Disk Cache')}</FormLabel>
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
              name='performance_setting.disk_cache_threshold_mb'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Disk Cache Threshold (MB)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!diskEnabled}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Use disk cache when request body exceeds this size')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='performance_setting.disk_cache_max_size_mb'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max Disk Cache Size (MB)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={100}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!diskEnabled}
                    />
                  </FormControl>
                  {stats?.disk_space_info &&
                    stats.disk_space_info.total > 0 && (
                      <FormDescription>
                        {t('Free: {{free}} / Total: {{total}}', {
                          free: formatBytes(stats.disk_space_info.free),
                          total: formatBytes(stats.disk_space_info.total),
                        })}
                      </FormDescription>
                    )}
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {lowDiskSpace && (
            <Alert variant='destructive'>
              <AlertDescription>
                {`${t('Warning')}: ${t('Available disk space')} (${formatBytes(stats?.disk_space_info?.free ?? 0)}) ${t('is less than the configured maximum cache size')} (${maxCacheSizeMb} MB). ${t('This may cause cache failures.')}`}
              </AlertDescription>
            </Alert>
          )}

          {!stats?.config?.is_running_in_container && (
            <FormField
              control={form.control}
              name='performance_setting.disk_cache_path'
              render={({ field }) => (
                <FormItem className='max-w-md'>
                  <FormLabel>{t('Cache Directory')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t(
                        'Leave empty to use system temp directory'
                      )}
                      value={field.value ?? ''}
                      onChange={(event) => field.onChange(event.target.value)}
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                      disabled={!diskEnabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}

          <Separator />

          {/* System Performance Monitor */}
          <div>
            <h4 className='font-medium'>
              {t('System Performance Monitoring')}
            </h4>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'When performance monitoring is enabled and system resource usage exceeds the set threshold, new Relay requests will be rejected.'
              )}
            </p>
          </div>

          <div className='grid grid-cols-1 gap-4 md:grid-cols-4'>
            <FormField
              control={form.control}
              name='performance_setting.monitor_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable Performance Monitoring')}</FormLabel>
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
              name='performance_setting.monitor_cpu_threshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('CPU Threshold (%)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!monitorEnabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='performance_setting.monitor_memory_threshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Memory Threshold (%)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={100}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!monitorEnabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='performance_setting.monitor_disk_threshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Disk Threshold (%)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={100}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!monitorEnabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </SettingsForm>
      </Form>

      <Separator />

      {/* Performance Stats Dashboard */}
      <div className='space-y-4'>
        <div className='flex items-center gap-2'>
          <h4 className='font-medium'>{t('Performance Monitor')}</h4>
          <Button variant='outline' size='sm' onClick={fetchStats}>
            {t('Refresh Stats')}
          </Button>
          <AlertDialog>
            <AlertDialogTrigger render={<Button variant='outline' size='sm' />}>
              {t('Clean up inactive cache')}
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  {t('Confirm cleanup of inactive disk cache?')}
                </AlertDialogTitle>
                <AlertDialogDescription>
                  {t(
                    'This will delete temporary cache files that have not been used for more than 10 minutes'
                  )}
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
                <AlertDialogAction
                  variant='destructive'
                  onClick={clearDiskCache}
                >
                  {t('Confirm')}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
          <Button variant='outline' size='sm' onClick={resetStats}>
            {t('Reset Stats')}
          </Button>
          <Button variant='outline' size='sm' onClick={forceGC}>
            {t('Run GC')}
          </Button>
        </div>

        {stats && (
          <>
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <div className='space-y-2 rounded-lg border p-4'>
                <p className='text-sm font-medium'>
                  {t('Request Body Disk Cache')}
                </p>
                <Progress value={diskCachePercent} />
                <div className='text-muted-foreground flex justify-between text-xs'>
                  <span>
                    {formatBytes(
                      stats.cache_stats?.current_disk_usage_bytes ?? 0
                    )}{' '}
                    /{' '}
                    {formatBytes(stats.cache_stats?.disk_cache_max_bytes ?? 0)}
                  </span>
                  <span>
                    {t('Active Files')}:{' '}
                    {stats.cache_stats?.active_disk_files ?? 0}
                  </span>
                </div>
                <StatusBadge variant='neutral' copyable={false}>
                  {t('Disk Hits')}: {stats.cache_stats?.disk_cache_hits ?? 0}
                </StatusBadge>
              </div>
              <div className='space-y-2 rounded-lg border p-4'>
                <p className='text-sm font-medium'>
                  {t('Request Body Memory Cache')}
                </p>
                <div className='text-muted-foreground flex justify-between text-xs'>
                  <span>
                    {t('Current Cache Size')}:{' '}
                    {formatBytes(
                      stats.cache_stats?.current_memory_usage_bytes ?? 0
                    )}
                  </span>
                  <span>
                    {t('Active Cache Count')}:{' '}
                    {stats.cache_stats?.active_memory_buffers ?? 0}
                  </span>
                </div>
                <StatusBadge variant='neutral' copyable={false}>
                  {t('Memory Hits')}:{' '}
                  {stats.cache_stats?.memory_cache_hits ?? 0}
                </StatusBadge>
              </div>
            </div>

            {stats.disk_space_info && stats.disk_space_info.total > 0 && (
              <div className='rounded-lg border p-4'>
                <p className='mb-2 text-sm font-medium'>
                  {t('Cache Directory Disk Space')}
                </p>
                <Progress
                  value={Math.round(stats.disk_space_info.used_percent)}
                />
                <div className='text-muted-foreground mt-2 flex justify-between text-xs'>
                  <span>
                    {t('Used')}: {formatBytes(stats.disk_space_info.used)}
                  </span>
                  <span>
                    {t('Available')}: {formatBytes(stats.disk_space_info.free)}
                  </span>
                  <span>
                    {t('Total')}: {formatBytes(stats.disk_space_info.total)}
                  </span>
                </div>
              </div>
            )}

            {stats.memory_stats && (
              <div className='rounded-lg border p-4'>
                <p className='mb-2 text-sm font-medium'>
                  {t('System Memory Stats')}
                </p>
                <div className='grid grid-cols-2 gap-2 text-xs md:grid-cols-5'>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Allocated Memory')}:
                    </span>{' '}
                    {formatBytes(stats.memory_stats.alloc)}
                  </div>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Total Allocated')}:
                    </span>{' '}
                    {formatBytes(stats.memory_stats.total_alloc)}
                  </div>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('System Memory')}:
                    </span>{' '}
                    {formatBytes(stats.memory_stats.sys)}
                  </div>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('GC Count')}:
                    </span>{' '}
                    {stats.memory_stats.num_gc}
                  </div>
                  <div>
                    <span className='text-muted-foreground'>Goroutines:</span>{' '}
                    {stats.memory_stats.num_goroutine}
                  </div>
                </div>
              </div>
            )}

            {(stats.relay_admission ||
              stats.system_gate ||
              stats.cgroup_memory) && (
              <div className='grid grid-cols-1 gap-4 lg:grid-cols-3'>
                {stats.relay_admission && (
                  <div className='rounded-lg border p-4'>
                    <p className='mb-2 text-sm font-medium'>
                      {t('Relay Admission')}
                    </p>
                    <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-2 text-xs'>
                      <span className='text-muted-foreground'>
                        {t('Active Requests / Limit')}:
                      </span>
                      <span className='text-right break-all'>
                        {relayRequestUsage}
                      </span>
                      <span className='text-muted-foreground'>
                        {t('Active Request Body / Limit')}:
                      </span>
                      <span className='text-right break-all'>
                        {relayBodyUsage}
                      </span>
                      <span className='text-muted-foreground'>
                        {t('Concurrent Request Limit Rejections')}:
                      </span>
                      <span className='text-right break-all'>
                        {stats.relay_admission
                          .rejected_too_many_concurrent_requests ?? t('N/A')}
                      </span>
                      <span className='text-muted-foreground'>
                        {t('Request Body Budget Exhaustion Rejections')}:
                      </span>
                      <span className='text-right break-all'>
                        {stats.relay_admission
                          .rejected_request_body_budget_exhausted ?? t('N/A')}
                      </span>
                      <span className='text-muted-foreground'>
                        {t('Memory Pressure Rejections')}:
                      </span>
                      <span className='text-right break-all'>
                        {stats.relay_admission?.rejected_memory_pressure ??
                          t('N/A')}
                      </span>
                      <span className='text-muted-foreground'>
                        {t(
                          'GET polls / result fetches exempted during memory pressure (not rejections)'
                        )}
                        :
                      </span>
                      <span className='text-right break-all'>
                        {stats.relay_admission?.exempted_memory_pressure ??
                          t('N/A')}
                      </span>
                    </div>
                  </div>
                )}

                {stats.system_gate && (
                  <div className='rounded-lg border p-4'>
                    <p className='mb-2 text-sm font-medium'>
                      {t('System Gate')}
                    </p>
                    <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-2 text-xs'>
                      <span className='text-muted-foreground'>
                        {t('CPU Overload 503 Rejections')}:
                      </span>
                      <span className='text-right break-all'>
                        {stats.system_gate.rejected_cpu_overloaded ?? t('N/A')}
                      </span>
                      <span className='text-muted-foreground'>
                        {t('Memory Overload 503 Rejections')}:
                      </span>
                      <span className='text-right break-all'>
                        {stats.system_gate.rejected_memory_overloaded ??
                          t('N/A')}
                      </span>
                      <span className='text-muted-foreground'>
                        {t('Disk Overload 503 Rejections')}:
                      </span>
                      <span className='text-right break-all'>
                        {stats.system_gate.rejected_disk_overloaded ?? t('N/A')}
                      </span>
                    </div>
                  </div>
                )}

                {stats.cgroup_memory && (
                  <div className='rounded-lg border p-4'>
                    <p className='mb-2 text-sm font-medium'>
                      {t('Cgroup Memory')}
                    </p>
                    {stats.cgroup_memory.available === true ? (
                      <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-2 text-xs'>
                        <span className='text-muted-foreground'>
                          {t('Usage / Limit')}:
                        </span>
                        <span className='text-right break-all'>
                          {cgroupMemoryUsage} ({cgroupMemoryPercent})
                        </span>
                        <span className='text-muted-foreground'>
                          {t('HIGH / LOW Thresholds')}:
                        </span>
                        <span className='text-right break-all'>
                          {cgroupMemoryThresholds}
                        </span>
                      </div>
                    ) : (
                      <Alert>
                        <AlertDescription className='flex flex-col gap-1 text-xs'>
                          <StatusBadge variant='neutral' copyable={false}>
                            {t('Unavailable / N/A')}
                          </StatusBadge>
                          <span>
                            {t(
                              'Cgroup memory metrics are unavailable because this process is not running in a container or has no cgroup memory limit.'
                            )}
                          </span>
                        </AlertDescription>
                      </Alert>
                    )}
                    <div className='mt-3 grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-2 text-xs'>
                      <span className='text-muted-foreground'>
                        {t('Breaker Status')}:
                      </span>
                      <span className='text-right'>
                        <StatusBadge
                          variant={cgroupStatusVariant}
                          copyable={false}
                        >
                          {cgroupStatusLabel}
                        </StatusBadge>
                      </span>
                      <span className='text-muted-foreground'>
                        {t('Trip Count')}:
                      </span>
                      <span className='text-right break-all'>
                        {stats.cgroup_memory?.trip_count ?? t('N/A')}
                      </span>
                      <span className='text-muted-foreground'>
                        {t('Forced Reset Count')}:
                      </span>
                      <span className='text-right break-all'>
                        {stats.cgroup_memory?.forced_reset_count ?? t('N/A')}
                      </span>
                      <span className='text-muted-foreground'>
                        {t('Max Trip Duration')}:
                      </span>
                      <span className='text-right break-all'>
                        {cgroupMaxTripDuration}
                      </span>
                      {cgroupTrippedDuration !== null && (
                        <>
                          <span className='text-muted-foreground'>
                            {t('Tripped Duration')}:
                          </span>
                          <span className='text-right break-all'>
                            {cgroupTrippedDuration}
                          </span>
                        </>
                      )}
                    </div>
                    {stats.cgroup_memory?.disarmed === true && (
                      <Alert variant='destructive' className='mt-3'>
                        <AlertTitle>{t('Circuit breaker disarmed')}</AlertTitle>
                        <AlertDescription>
                          {t(
                            'The circuit breaker was forcibly reset and disabled after exceeding the maximum trip duration. It will automatically recover once memory falls below the LOW threshold.'
                          )}
                        </AlertDescription>
                      </Alert>
                    )}
                  </div>
                )}
              </div>
            )}

            {stats.disk_cache_info && (
              <div className='rounded-lg border p-4'>
                <p className='mb-2 text-sm font-medium'>
                  {t('Cache Directory Info')}
                </p>
                <div className='grid grid-cols-3 gap-2 text-xs'>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Cache Directory')}:
                    </span>{' '}
                    <span className='font-mono'>
                      {stats.disk_cache_info.path}
                    </span>
                  </div>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Directory File Count')}:
                    </span>{' '}
                    {stats.disk_cache_info.file_count}
                  </div>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Directory Total Size')}:
                    </span>{' '}
                    {formatBytes(stats.disk_cache_info.total_size)}
                  </div>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </SettingsSection>
  )
}
