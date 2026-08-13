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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { Separator } from '@/components/ui/separator'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '@/lib/api'

import { SettingsSection } from '../components/settings-section'

const POLL_INTERVAL_MS = 10_000

type InstanceGauges = {
  active_requests: number
  active_body_bytes: number
  max_concurrent: number
  max_body_bytes: number
  cgroup_permille: number
  cgroup_tripped: boolean
  trip_count: number
  forced_reset_count: number
  goroutines: number
}

type InstanceSnapshot = InstanceGauges & {
  node: string
  last_seen_unix: number
  stale_seconds: number
}

type MinutePoint = {
  minute_unix: number
  requests: number
  success: number
  errors: number
  client_gone: number
  prompt_tokens: number
  completion_tokens: number
  quota: number
  rej_gate: number
  rej_concurrency: number
  rej_body: number
  rej_memory: number
  rej_model_rpm: number
  rej_user_rpm: number
}

type ChannelPoint = {
  channel_id: number
  channel_name: string
  concurrency: number
  requests: number
  errors: number
  window_secs: number
}

type RealtimeSnapshot = {
  redis_enabled: boolean
  now_unix: number
  instances: InstanceSnapshot[]
  totals: InstanceGauges
  series: MinutePoint[]
  channels: ChannelPoint[]
  degraded: boolean
  warning?: string
}

type ChannelSortKey = 'concurrency' | 'requests' | 'errors' | 'error_rate'

function formatBytes(bytes: number): string {
  if (!bytes || Number.isNaN(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(
    units.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1024))
  )
  return `${Number.parseFloat((bytes / 1024 ** index).toFixed(1))} ${units[index]}`
}

function formatCompact(value: number): string {
  if (!Number.isFinite(value)) return '0'
  if (Math.abs(value) < 1000) return String(value)
  if (Math.abs(value) < 1_000_000) return `${(value / 1000).toFixed(1)}k`
  return `${(value / 1_000_000).toFixed(1)}M`
}

function errorRate(requests: number, errors: number): number {
  if (requests <= 0) return 0
  return (errors / requests) * 100
}

/**
 * A dependency-free sparkline. The dashboard is a single settings section, so
 * pulling in a charting library for two 60-point lines would cost more than the
 * feature.
 */
function Sparkline(props: {
  values: number[]
  className?: string
  label: string
}) {
  const { values, label } = props
  const width = 100
  const height = 28
  const max = Math.max(1, ...values)
  const step = values.length > 1 ? width / (values.length - 1) : width
  const points = values
    .map((value, index) => {
      const x = index * step
      const y = height - (value / max) * height
      return `${x.toFixed(2)},${y.toFixed(2)}`
    })
    .join(' ')

  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio='none'
      className={props.className}
      role='img'
      aria-label={label}
    >
      <polyline
        points={points}
        fill='none'
        stroke='currentColor'
        strokeWidth='1.5'
        vectorEffect='non-scaling-stroke'
      />
    </svg>
  )
}

export function RealtimeLoadSection() {
  const { t } = useTranslation()
  const [snapshot, setSnapshot] = useState<RealtimeSnapshot | null>(null)
  const [unreachable, setUnreachable] = useState(false)
  const [channelFilter, setChannelFilter] = useState('')
  const [sortKey, setSortKey] = useState<ChannelSortKey>('concurrency')
  const [channelsOpen, setChannelsOpen] = useState(false)
  const inFlightRef = useRef(false)

  const fetchSnapshot = useCallback(async () => {
    // A slow poll must not stack up behind itself when the backend is busy —
    // that is exactly the load the dashboard exists to warn about.
    if (inFlightRef.current) return
    inFlightRef.current = true
    try {
      const res = await api.get('/api/performance/realtime', {
        // The endpoint answers 200 even when degraded, but a network drop would
        // otherwise raise a toast on every tick.
        skipErrorHandler: true,
      })
      if (res.data?.success) {
        setSnapshot(res.data.data as RealtimeSnapshot)
        setUnreachable(false)
      }
    } catch {
      setUnreachable(true)
    } finally {
      inFlightRef.current = false
    }
  }, [])

  useEffect(() => {
    fetchSnapshot()
    let timer: ReturnType<typeof setInterval> | null = null

    const start = () => {
      if (timer !== null) return
      timer = setInterval(fetchSnapshot, POLL_INTERVAL_MS)
    }
    const stop = () => {
      if (timer === null) return
      clearInterval(timer)
      timer = null
    }
    // Polling a hidden tab every ten seconds costs the server for nobody.
    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        fetchSnapshot()
        start()
      } else {
        stop()
      }
    }

    if (document.visibilityState === 'visible') start()
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      stop()
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [fetchSnapshot])

  const series = snapshot?.series ?? []
  const recent = useMemo(() => series.slice(-5), [series])

  const windowTotals = useMemo(() => {
    const empty = {
      requests: 0,
      success: 0,
      errors: 0,
      clientGone: 0,
      promptTokens: 0,
      completionTokens: 0,
      quota: 0,
      rejGate: 0,
      rejConcurrency: 0,
      rejBody: 0,
      rejMemory: 0,
      rejModelRPM: 0,
      rejUserRPM: 0,
    }
    return series.reduce(
      (acc, point) => ({
        requests: acc.requests + point.requests,
        success: acc.success + point.success,
        errors: acc.errors + point.errors,
        clientGone: acc.clientGone + point.client_gone,
        promptTokens: acc.promptTokens + point.prompt_tokens,
        completionTokens: acc.completionTokens + point.completion_tokens,
        quota: acc.quota + point.quota,
        rejGate: acc.rejGate + point.rej_gate,
        rejConcurrency: acc.rejConcurrency + point.rej_concurrency,
        rejBody: acc.rejBody + point.rej_body,
        rejMemory: acc.rejMemory + point.rej_memory,
        rejModelRPM: acc.rejModelRPM + point.rej_model_rpm,
        rejUserRPM: acc.rejUserRPM + point.rej_user_rpm,
      }),
      empty
    )
  }, [series])

  const currentRpm = recent.length > 0 ? recent[recent.length - 1].requests : 0
  const totals = snapshot?.totals

  const concurrencyPercent =
    totals && totals.max_concurrent > 0
      ? Math.min(
          100,
          Math.round((totals.active_requests / totals.max_concurrent) * 100)
        )
      : 0
  const memoryPercent = totals ? totals.cgroup_permille / 10 : 0

  let memoryVariant: StatusVariant = 'success'
  if (totals?.cgroup_tripped) memoryVariant = 'danger'
  else if (memoryPercent >= 75) memoryVariant = 'warning'

  const visibleChannels = useMemo(() => {
    const keyword = channelFilter.trim().toLowerCase()
    const filtered = (snapshot?.channels ?? []).filter((channel) => {
      if (keyword === '') return true
      return (
        String(channel.channel_id).includes(keyword) ||
        channel.channel_name.toLowerCase().includes(keyword)
      )
    })
    return [...filtered].sort((a, b) => {
      if (sortKey === 'error_rate') {
        const diff =
          errorRate(b.requests, b.errors) - errorRate(a.requests, a.errors)
        if (diff !== 0) return diff
      } else if (a[sortKey] !== b[sortKey]) {
        return b[sortKey] - a[sortKey]
      }
      return a.channel_id - b.channel_id
    })
  }, [snapshot?.channels, channelFilter, sortKey])

  const channelWindowMinutes =
    (snapshot?.channels?.[0]?.window_secs ?? 300) / 60

  const sortOptions: Array<{ key: ChannelSortKey; label: string }> = [
    { key: 'concurrency', label: t('Concurrency') },
    { key: 'requests', label: t('Requests') },
    { key: 'errors', label: t('Errors') },
    { key: 'error_rate', label: t('Error Rate') },
  ]

  return (
    <SettingsSection
      title={t('Realtime Load')}
      description={t(
        'Live relay load across every instance, refreshed every 10 seconds. Counters come from Redis and cover the last 60 minutes; nothing here is written to the database.'
      )}
    >
      <div className='flex items-center gap-2'>
        <Button variant='outline' size='sm' onClick={fetchSnapshot}>
          {t('Refresh Stats')}
        </Button>
        {snapshot && (
          <span className='text-muted-foreground text-xs'>
            {t('Instances')}: {snapshot.instances.length}
          </span>
        )}
      </div>

      {unreachable && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Realtime data unavailable')}</AlertTitle>
          <AlertDescription>
            {t('Could not reach the server. Retrying automatically.')}
          </AlertDescription>
        </Alert>
      )}

      {snapshot?.degraded && (
        <Alert>
          <AlertTitle>{t('Showing this instance only')}</AlertTitle>
          <AlertDescription>
            {snapshot.warning === 'redis_disabled'
              ? t(
                  'Redis is not enabled, so cross-instance aggregation and the 60-minute history are unavailable.'
                )
              : t(
                  'Redis could not be reached, so cross-instance aggregation and the 60-minute history are unavailable.'
                )}
          </AlertDescription>
        </Alert>
      )}

      {snapshot && (
        <>
          <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
            <div className='space-y-2 rounded-lg border p-4'>
              <p className='text-sm font-medium'>
                {t('Active Requests / Limit')}
              </p>
              <p className='text-2xl font-semibold'>
                {totals?.active_requests ?? 0}
                <span className='text-muted-foreground text-base font-normal'>
                  {' '}
                  / {totals?.max_concurrent ?? 0}
                </span>
              </p>
              <Progress value={concurrencyPercent} />
              <p className='text-muted-foreground text-xs'>
                {t('Active Request Body / Limit')}:{' '}
                {formatBytes(totals?.active_body_bytes ?? 0)} /{' '}
                {formatBytes(totals?.max_body_bytes ?? 0)}
              </p>
            </div>

            <div className='space-y-2 rounded-lg border p-4'>
              <p className='text-sm font-medium'>{t('Requests per minute')}</p>
              <p className='text-2xl font-semibold'>{currentRpm}</p>
              <Sparkline
                values={series.map((point) => point.requests)}
                label={t('Requests per minute')}
                className='text-primary h-7 w-full'
              />
              <p className='text-muted-foreground text-xs'>
                {t('Last 60 minutes')}:{' '}
                {formatCompact(windowTotals.requests)}
              </p>
            </div>

            <div className='space-y-2 rounded-lg border p-4'>
              <p className='text-sm font-medium'>{t('Memory Pressure')}</p>
              <div className='flex items-center gap-2'>
                <p className='text-2xl font-semibold'>
                  {memoryPercent.toFixed(1)}%
                </p>
                <StatusBadge variant={memoryVariant} copyable={false}>
                  {totals?.cgroup_tripped ? t('Tripped') : t('Normal')}
                </StatusBadge>
              </div>
              <Progress value={Math.min(100, Math.round(memoryPercent))} />
              <p className='text-muted-foreground text-xs'>
                {t('Trip Count')}: {totals?.trip_count ?? 0} ·{' '}
                {t('Forced Reset Count')}: {totals?.forced_reset_count ?? 0}
              </p>
            </div>
          </div>

          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <div className='space-y-2 rounded-lg border p-4'>
              <p className='text-sm font-medium'>{t('Errors per minute')}</p>
              <Sparkline
                values={series.map((point) => point.errors)}
                label={t('Errors per minute')}
                className='h-7 w-full text-red-500'
              />
              <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-1 text-xs'>
                <span className='text-muted-foreground'>
                  {t('Succeeded')} / {t('Failed')}:
                </span>
                <span className='text-right'>
                  {formatCompact(windowTotals.success)} /{' '}
                  {formatCompact(windowTotals.errors)}
                </span>
                <span className='text-muted-foreground'>
                  {t('Error Rate')}:
                </span>
                <span className='text-right'>
                  {errorRate(windowTotals.requests, windowTotals.errors).toFixed(
                    1
                  )}
                  %
                </span>
                <span className='text-muted-foreground'>
                  {t('Client disconnected')}:
                </span>
                <span className='text-right'>
                  {formatCompact(windowTotals.clientGone)}
                </span>
                <span className='text-muted-foreground'>{t('Tokens')}:</span>
                <span className='text-right'>
                  {formatCompact(windowTotals.promptTokens)} +{' '}
                  {formatCompact(windowTotals.completionTokens)}
                </span>
              </div>
            </div>

            <div className='space-y-2 rounded-lg border p-4'>
              <p className='text-sm font-medium'>
                {t('Rejections (last 60 minutes)')}
              </p>
              <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-1 text-xs'>
                <span className='text-muted-foreground'>
                  {t('Concurrent Request Limit Rejections')}:
                </span>
                <span className='text-right'>
                  {formatCompact(windowTotals.rejConcurrency)}
                </span>
                <span className='text-muted-foreground'>
                  {t('Request Body Budget Exhaustion Rejections')}:
                </span>
                <span className='text-right'>
                  {formatCompact(windowTotals.rejBody)}
                </span>
                <span className='text-muted-foreground'>
                  {t('Memory Pressure Rejections')}:
                </span>
                <span className='text-right'>
                  {formatCompact(windowTotals.rejMemory)}
                </span>
                <span className='text-muted-foreground'>
                  {t('Model RPM Rejections')}:
                </span>
                <span className='text-right'>
                  {formatCompact(windowTotals.rejModelRPM)}
                </span>
                <span className='text-muted-foreground'>
                  {t('User Rate Limit Rejections')}:
                </span>
                <span className='text-right'>
                  {formatCompact(windowTotals.rejUserRPM)}
                </span>
                <span className='text-muted-foreground'>
                  {t('Other Gate Rejections')}:
                </span>
                <span className='text-right'>
                  {formatCompact(windowTotals.rejGate)}
                </span>
              </div>
            </div>
          </div>

          <Separator />

          <Collapsible open={channelsOpen} onOpenChange={setChannelsOpen}>
            <div className='flex flex-wrap items-center gap-2'>
              <CollapsibleTrigger
                render={<Button variant='outline' size='sm' />}
              >
                {channelsOpen
                  ? t('Hide per-channel load')
                  : t('Show per-channel load')}
              </CollapsibleTrigger>
              <span className='text-muted-foreground text-xs'>
                {t('{{total}} channels active', {
                  total: snapshot.channels.length,
                })}
              </span>
            </div>

            <CollapsibleContent className='mt-3 space-y-3'>
              <div className='flex flex-wrap items-center gap-2'>
                <Input
                  className='max-w-xs'
                  value={channelFilter}
                  onChange={(event) => setChannelFilter(event.target.value)}
                  placeholder={t('Filter by channel name or ID')}
                />
                <div className='flex flex-wrap items-center gap-1'>
                  <span className='text-muted-foreground text-xs'>
                    {t('Sort by')}:
                  </span>
                  {sortOptions.map((option) => (
                    <Button
                      key={option.key}
                      variant={sortKey === option.key ? 'default' : 'outline'}
                      size='sm'
                      onClick={() => setSortKey(option.key)}
                    >
                      {option.label}
                    </Button>
                  ))}
                </div>
              </div>

              {visibleChannels.length === 0 ? (
                <p className='text-muted-foreground text-xs'>
                  {t('No channel traffic in the current window.')}
                </p>
              ) : (
                <div className='max-h-96 overflow-auto rounded-lg border'>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('Channel')}</TableHead>
                        <TableHead className='text-right'>
                          {t('Concurrency')}
                        </TableHead>
                        <TableHead className='text-right'>
                          {t('Requests')}
                        </TableHead>
                        <TableHead className='text-right'>
                          {t('Errors')}
                        </TableHead>
                        <TableHead className='text-right'>
                          {t('Error Rate')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {visibleChannels.map((channel) => {
                        const rate = errorRate(channel.requests, channel.errors)
                        return (
                          <TableRow key={channel.channel_id}>
                            <TableCell>
                              <span className='font-medium'>
                                {channel.channel_name || t('Unknown')}
                              </span>
                              <span className='text-muted-foreground'>
                                {' '}
                                #{channel.channel_id}
                              </span>
                            </TableCell>
                            <TableCell className='text-right tabular-nums'>
                              {channel.concurrency}
                            </TableCell>
                            <TableCell className='text-right tabular-nums'>
                              {formatCompact(channel.requests)}
                            </TableCell>
                            <TableCell className='text-right tabular-nums'>
                              {formatCompact(channel.errors)}
                            </TableCell>
                            <TableCell className='text-right tabular-nums'>
                              {rate >= 20 ? (
                                <StatusBadge variant='danger' copyable={false}>
                                  {rate.toFixed(1)}%
                                </StatusBadge>
                              ) : (
                                `${rate.toFixed(1)}%`
                              )}
                            </TableCell>
                          </TableRow>
                        )
                      })}
                    </TableBody>
                  </Table>
                </div>
              )}
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Concurrency is live; requests and errors cover the last {{minutes}} minutes.',
                  { minutes: channelWindowMinutes }
                )}
              </p>
            </CollapsibleContent>
          </Collapsible>

          <Separator />

          <div className='space-y-2'>
            <p className='text-sm font-medium'>{t('Instances')}</p>
            <div className='overflow-auto rounded-lg border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Node')}</TableHead>
                    <TableHead className='text-right'>
                      {t('Active Requests')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Request Body')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Memory Pressure')}
                    </TableHead>
                    <TableHead className='text-right'>Goroutines</TableHead>
                    <TableHead className='text-right'>
                      {t('Last seen')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {snapshot.instances.map((instance) => (
                    <TableRow key={instance.node}>
                      <TableCell className='font-medium'>
                        {instance.node}
                        {instance.cgroup_tripped && (
                          <StatusBadge
                            variant='danger'
                            copyable={false}
                            className='ml-2'
                          >
                            {t('Tripped')}
                          </StatusBadge>
                        )}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {instance.active_requests} / {instance.max_concurrent}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatBytes(instance.active_body_bytes)}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {(instance.cgroup_permille / 10).toFixed(1)}%
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {instance.goroutines}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {instance.stale_seconds}s
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </div>
        </>
      )}
    </SettingsSection>
  )
}
