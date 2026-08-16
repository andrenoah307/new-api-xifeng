/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronUp, Gauge } from 'lucide-react'
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IconBadge } from '@/components/ui/icon-badge'
import { Spinner } from '@/components/ui/spinner'
import { PanelWrapper } from '@/features/dashboard/components/ui/panel-wrapper'
import { getRateLimitCapacity } from '@/features/dashboard/api'
import {
  normalizePersonalRPMItems,
  PERSONAL_RPM_REFRESH_INTERVAL,
} from '@/features/dashboard/lib/personal-rpm'
import type {
  RateLimitCapacityItem,
  RateLimitCapacityMetric,
  RateLimitCapacityPersonal,
  RateLimitCapacityResponse,
} from '@/features/dashboard/types'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

const RETRY_COOLDOWN = 3_000

function formatCount(value: number): string {
  return Number.isFinite(value) ? value.toLocaleString() : '0'
}

function formatPercent(value: number | null | undefined): string | null {
  if (value == null || !Number.isFinite(value)) return null
  const percent = value * 100
  if (percent >= 100) {
    // Round over-limit values upward so a genuine value such as 100.04% is
    // never presented as an apparently safe 100%.
    return `${Math.ceil(percent * 10) / 10}%`
  }
  return `${percent.toFixed(1)}%`
}

function formatObservedAt(value: string | undefined): string {
  if (!value) return '--'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '--' : date.toLocaleString()
}

function isMetricAvailable(metric: RateLimitCapacityMetric): boolean {
  return (
    metric.available &&
    (metric.unlimited ||
      (typeof metric.current === 'number' && Number.isFinite(metric.current)))
  )
}

function CapacityBar(props: { metric: RateLimitCapacityMetric }) {
  const { metric } = props
  if (
    metric.unlimited ||
    !isMetricAvailable(metric) ||
    metric.utilization == null
  ) {
    return null
  }

  const width = Math.min(100, Math.max(0, metric.utilization * 100))
  let barTone = 'bg-primary'
  if (metric.over_limit) {
    barTone = 'bg-destructive'
  } else if (metric.utilization >= 0.8) {
    barTone = 'bg-warning'
  }

  return (
    <div
      className='bg-muted h-1.5 w-full overflow-hidden rounded-full'
      aria-hidden='true'
    >
      <div
        className={cn('h-full rounded-full transition-[width]', barTone)}
        style={{ width: `${width}%` }}
      />
    </div>
  )
}

function CapacityRow(props: {
  label: string
  metric: RateLimitCapacityMetric
  group?: string
}) {
  const { t } = useTranslation()
  const metric = props.metric
  const available = isMetricAvailable(metric)
  const percent = formatPercent(metric.utilization)
  let value = t('Temporarily unavailable')
  if (available && metric.unlimited) {
    value = t('Unlimited')
  } else if (available && metric.current !== null) {
    value = `${formatCount(metric.current)} / ${formatCount(metric.limit)}`
  }

  return (
    <div className='rounded-lg border border-border/60 bg-card/40 p-3'>
      <div className='min-w-0 truncate text-sm font-medium' title={props.label}>
        {props.label}
      </div>
      {props.group && (
        <div className='text-muted-foreground mt-0.5 truncate text-xs'>
          {props.group}
        </div>
      )}
      <div className='mt-2 flex min-w-0 items-baseline gap-2'>
        <div
          className={cn(
            'min-w-0 truncate font-mono text-sm font-semibold tabular-nums',
            metric.over_limit && 'text-destructive',
            !metric.over_limit &&
              available &&
              metric.utilization != null &&
              metric.utilization >= 0.8 &&
              'text-warning'
          )}
        >
          {value}
        </div>
        {available && percent && (
          <div className='text-muted-foreground shrink-0 text-xs font-normal'>
            {percent}
          </div>
        )}
      </div>
      <div className='mt-2'>
        <CapacityBar metric={metric} />
      </div>
    </div>
  )
}

function SiteSection(props: {
  title: string
  windowLabel: string
  items: RateLimitCapacityItem[]
}) {
  const { t } = useTranslation()
  return (
    <section aria-label={props.title}>
      <div className='bg-muted/30 border-border/60 border-b px-4 py-3 sm:px-5'>
        <h3 className='text-sm font-semibold'>{props.title}</h3>
        <p className='text-muted-foreground mt-0.5 text-xs'>
          {props.windowLabel} · {t('All users')}
        </p>
      </div>
      <div
        className='grid gap-3 px-4 py-3 sm:px-5'
        style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(15rem, 1fr))' }}
      >
        {props.items.map((item) => (
          <CapacityRow
            key={`${item.model}:${item.group ?? ''}`}
            label={item.model}
            group={item.group}
            metric={item}
          />
        ))}
      </div>
    </section>
  )
}

function PersonalSection(props: { personal: RateLimitCapacityPersonal }) {
  const { t } = useTranslation()
  const personal = props.personal
  const items = normalizePersonalRPMItems(personal.items)
  const unavailable = personal.status === 'unavailable' || personal.status === 'overflow'
  const showEmpty = personal.status === 'empty' || (!unavailable && items.length === 0)

  return (
    <section aria-label={t('My model RPM')}>
      <div className='bg-muted/30 border-border/60 border-b px-4 py-3 sm:px-5'>
        <h3 className='text-sm font-semibold'>{t('My model RPM')}</h3>
        <p className='text-muted-foreground mt-0.5 text-xs'>{t('Last 60 seconds')}</p>
      </div>
      {showEmpty && (
        <div className='text-muted-foreground px-4 py-4 text-sm sm:px-5'>
          {t('No request data yet')}
        </div>
      )}
      {unavailable && (
        <div className='text-muted-foreground px-4 py-4 text-sm sm:px-5'>
          {t('Temporarily unavailable')}
        </div>
      )}
      {!showEmpty && !unavailable && (
        <div
          className='grid gap-3 px-4 py-3 sm:px-5'
          style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(13rem, 1fr))' }}
        >
          {items.map((item) => (
            <div
              key={item.model}
              className='rounded-lg border border-border/60 bg-card/40 p-3'
            >
              <div className='min-w-0 truncate text-sm' title={item.model}>
                {item.model}
              </div>
              <div className='mt-2 truncate font-mono text-sm font-semibold tabular-nums'>
                {formatCount(item.rpm)} RPM
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

function CapacityMetadata(props: { data: RateLimitCapacityResponse }) {
  const { t } = useTranslation()
  const instanceOnly =
    props.data.instance_only ||
    props.data.backend_scope === 'instance' ||
    Boolean(props.data.personal?.instance_only)
  return (
    <div className='text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 text-xs sm:px-5'>
      <span>
        {t('Data updated: {{time}}', {
          time: formatObservedAt(props.data.observed_at),
        })}
      </span>
      <span>{t('Data refreshes every 15 seconds')}</span>
      {instanceOnly && (
        <span className='text-warning'>{t('Instance-only data')}</span>
      )}
    </div>
  )
}

function CapacityExpandButton(props: {
  expanded: boolean
  total: number
  loading: boolean
  onIntent: () => void
  onToggle: () => void
}) {
  const { t } = useTranslation()
  let icon = <ChevronDown className='size-3.5' aria-hidden='true' />
  if (props.expanded) {
    icon = <ChevronUp className='size-3.5' aria-hidden='true' />
  } else if (props.loading) {
    icon = <Spinner className='size-3.5' aria-label={t('Loading')} />
  }
  return (
    <button
      type='button'
      className='text-primary focus-visible:ring-ring inline-flex min-h-8 items-center gap-1 rounded-md px-2 text-xs font-medium outline-none focus-visible:ring-2'
      aria-expanded={props.expanded}
      aria-controls='rate-limit-capacity-all'
      onMouseEnter={props.onIntent}
      onFocus={props.onIntent}
      onTouchStart={props.onIntent}
      onClick={props.onToggle}
    >
      {props.expanded
        ? t('Hide additional capacity')
        : t('Show all {{total}} items', { total: props.total })}
      {icon}
    </button>
  )
}

export function RateLimitCapacityPanel() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const [expanded, setExpanded] = useState(false)
  const allAttemptAtRef = useRef(0)

  const identityKey = `${user?.id ?? 'anonymous'}:${user?.group ?? ''}`
  const queryKeyBase = ['dashboard', 'rate-limit-capacity', identityKey]

  const queryCapacity = useCallback(async (scope: 'top' | 'all') => {
    if (scope === 'all') allAttemptAtRef.current = Date.now()
    const result = await getRateLimitCapacity(scope)
    if (!result.success || !result.data) {
      throw new Error(result.message || 'capacity request failed')
    }
    return result.data
  }, [])

  const topQuery = useQuery({
    queryKey: [...queryKeyBase, 'top'],
    queryFn: () => queryCapacity('top'),
    enabled: Boolean(user),
    staleTime: PERSONAL_RPM_REFRESH_INTERVAL,
    refetchInterval: PERSONAL_RPM_REFRESH_INTERVAL,
    refetchIntervalInBackground: false,
    retry: 1,
    retryDelay: RETRY_COOLDOWN,
  })
  const allQuery = useQuery({
    queryKey: [...queryKeyBase, 'all'],
    queryFn: () => queryCapacity('all'),
    enabled: expanded && Boolean(user),
    staleTime: PERSONAL_RPM_REFRESH_INTERVAL,
    retry: false,
  })
  const {
    data: allData,
    isError: allIsError,
    isFetching: allIsFetching,
    isStale: allIsStale,
    refetch: refetchAll,
  } = allQuery

  const handleExpandIntent = useCallback(() => {
    const elapsed = Date.now() - allAttemptAtRef.current
    if (allIsFetching) return
    if (allIsError && elapsed < RETRY_COOLDOWN) return
    if (
      !allData ||
      (allIsError && elapsed >= RETRY_COOLDOWN) ||
      (allIsStale && elapsed >= PERSONAL_RPM_REFRESH_INTERVAL)
    ) {
      void refetchAll()
    }
  }, [allData, allIsError, allIsFetching, allIsStale, refetchAll])

  const handleToggle = useCallback(() => {
    if (expanded) {
      setExpanded(false)
      return
    }
    handleExpandIntent()
    setExpanded(true)
  }, [expanded, handleExpandIntent])

  const topData = topQuery.data
  const data = (expanded && allData) || topData || allData
  const site = data?.site
  const global = site?.global
  const groups = site?.groups
  const hasGroups = (groups?.total ?? 0) > 0
  const hasHiddenItems = Boolean(
    site &&
      ((global?.total ?? 0) > (global?.items.length ?? 0) ||
        (groups?.total ?? 0) > (groups?.items.length ?? 0))
  )
  const showExpandControl = hasHiddenItems || expanded
  const topLoading = topQuery.isPending && !topData

  return (
    <PanelWrapper
      title={
        <span className='flex items-center gap-2'>
          <IconBadge tone='info' size='sm'>
            <Gauge />
          </IconBadge>
          {t('RPM overview')}
        </span>
      }
      description={t('Read-only rate-limit capacity snapshot')}
      loading={topLoading}
      contentClassName='p-0'
    >
      {data?.degraded && (
        <div className='border-border/60 bg-warning/10 text-warning border-b px-4 py-3 text-xs sm:px-5'>
          {t('Some capacity data is temporarily unavailable')}
        </div>
      )}
      {topQuery.isError && !data && (
        <div className='text-muted-foreground border-border/60 border-b px-4 py-4 text-sm sm:px-5'>
          {t('Temporarily unavailable')}
        </div>
      )}

      {global?.items.length ? (
        <SiteSection
          title={t('Site model RPM')}
          windowLabel={t('60-second window')}
          items={global.items}
        />
      ) : null}

      {!hasGroups && showExpandControl && (
        <div className='border-border/60 flex items-center justify-between gap-3 border-y bg-transparent px-4 py-2 sm:px-5'>
          <div className='text-muted-foreground text-xs'>{t('Site model RPM')}</div>
          <CapacityExpandButton
            expanded={expanded}
            total={global?.total ?? 0}
            loading={allIsFetching}
            onIntent={handleExpandIntent}
            onToggle={handleToggle}
          />
        </div>
      )}
      {!hasGroups && (
        <div id='rate-limit-capacity-all' hidden={!expanded}>
          {expanded && allQuery.isFetching && (
            <div className='text-muted-foreground flex items-center gap-2 px-4 py-3 text-xs sm:px-5'>
              <Spinner className='size-3.5' aria-label={t('Loading')} />
              {t('Loading')}
            </div>
          )}
          {expanded && allQuery.isError && !allData && (
            <div className='text-muted-foreground px-4 py-3 text-xs sm:px-5'>
              {t('Temporarily unavailable')}
            </div>
          )}
        </div>
      )}

      {hasGroups && (
        <>
          <div className='border-border/60 flex items-center justify-between gap-3 border-y bg-transparent px-4 py-2 sm:px-5'>
            <div className='text-muted-foreground text-xs'>
              {t('Site group RPM')}
            </div>
            {showExpandControl && (
              <CapacityExpandButton
                expanded={expanded}
                total={(global?.total ?? 0) + (groups?.total ?? 0)}
                loading={allIsFetching}
                onIntent={handleExpandIntent}
                onToggle={handleToggle}
              />
            )}
          </div>
          {!expanded && (
            <SiteSection
              title={t('Site group RPM')}
              windowLabel={t('60-second window')}
              items={groups?.items ?? []}
            />
          )}
          <div id='rate-limit-capacity-all' hidden={!expanded}>
            {expanded && allQuery.isFetching && (
              <div className='text-muted-foreground flex items-center gap-2 px-4 py-3 text-xs sm:px-5'>
                <Spinner className='size-3.5' aria-label={t('Loading')} />
                {t('Loading')}
              </div>
            )}
            {expanded && allQuery.isError && !allData && (
              <div className='text-muted-foreground px-4 py-3 text-xs sm:px-5'>
                {t('Temporarily unavailable')}
              </div>
            )}
            {expanded && !allData && (allQuery.isFetching || allQuery.isError) && (
              <SiteSection
                title={t('Site group RPM')}
                windowLabel={t('60-second window')}
                items={groups?.items ?? []}
              />
            )}
            {expanded && allData?.site?.groups.items && (
              <SiteSection
                title={t('Site group RPM')}
                windowLabel={t('60-second window')}
                items={allData.site.groups.items}
              />
            )}
            {expanded && !allData && !allQuery.isFetching && !allQuery.isError && (
              <SiteSection
                title={t('Site group RPM')}
                windowLabel={t('60-second window')}
                items={groups?.items ?? []}
              />
            )}
          </div>
        </>
      )}

      {data?.personal && <PersonalSection personal={data.personal} />}
      {data && <CapacityMetadata data={data} />}
    </PanelWrapper>
  )
}
