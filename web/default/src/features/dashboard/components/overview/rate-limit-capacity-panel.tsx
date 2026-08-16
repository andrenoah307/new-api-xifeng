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
import type {
  RateLimitCapacityItem,
  RateLimitCapacityMetric,
  RateLimitCapacityPersonal,
  RateLimitCapacityResponse,
} from '@/features/dashboard/types'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

const CAPACITY_STALE_TIME = 5_000
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
  model?: string
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
    <div className='border-border/60 border-b px-4 py-3 last:border-b-0 sm:px-5'>
      <div className='flex min-w-0 items-start justify-between gap-3'>
        <div className='min-w-0'>
          <div className='truncate text-sm font-medium'>{props.label}</div>
          {(props.model || props.group) && (
            <div className='text-muted-foreground mt-0.5 truncate text-xs'>
              {props.model}
              {props.group ? ` · ${props.group}` : ''}
            </div>
          )}
        </div>
        <div
          className={cn(
            'shrink-0 text-right font-mono text-sm font-semibold tabular-nums',
            metric.over_limit && 'text-destructive',
            !metric.over_limit &&
              available &&
              metric.utilization != null &&
              metric.utilization >= 0.8 &&
              'text-warning'
          )}
        >
          <div>{value}</div>
          {available && percent && (
            <div className='text-muted-foreground mt-0.5 text-xs font-normal'>
              {percent}
            </div>
          )}
        </div>
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
      {props.items.map((item) => (
        <CapacityRow
          key={`${item.model}:${item.group ?? ''}`}
          label={item.model}
          model={item.model}
          group={item.group}
          metric={item}
        />
      ))}
    </section>
  )
}

function PersonalSection(props: { personal: RateLimitCapacityPersonal }) {
  const { t } = useTranslation()
  const personal = props.personal
  const windowLabel =
    personal.status === 'disabled'
      ? t('Not enabled')
      : t('Configured window: {{minutes}} minutes', {
          minutes: personal.window_minutes,
        })

  return (
    <section aria-label={t('My request limits')}>
      <div className='bg-muted/30 border-border/60 border-b px-4 py-3 sm:px-5'>
        <h3 className='text-sm font-semibold'>{t('My request limits')}</h3>
        <p className='text-muted-foreground mt-0.5 text-xs'>
          {windowLabel}
          {personal.group ? ` · ${personal.group}` : ''}
        </p>
      </div>
      {personal.status === 'disabled' && (
        <div className='text-muted-foreground px-4 py-4 text-sm sm:px-5'>
          {t('Not enabled')}
        </div>
      )}
      {personal.status === 'unconfigured' && (
        <div className='text-muted-foreground px-4 py-4 text-sm sm:px-5'>
          {t('Not configured')}
        </div>
      )}
      {personal.status !== 'disabled' && personal.status !== 'unconfigured' && (
        <>
          {personal.total && (
            <CapacityRow
              label={t('Total requests')}
              metric={personal.total}
            />
          )}
          {personal.success && (
            <CapacityRow
              label={t('Successful requests')}
              metric={personal.success}
            />
          )}
          {!personal.total && !personal.success && (
            <div className='text-muted-foreground px-4 py-4 text-sm sm:px-5'>
              {t('Temporarily unavailable')}
            </div>
          )}
        </>
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
        {t('Snapshot: {{time}}', {
          time: formatObservedAt(props.data.observed_at),
        })}
      </span>
      <span>
        {t('Config versions: {{model}} / {{group}}', {
          model: props.data.model_name_rpm_version,
          group: props.data.group_rate_limit_version,
        })}
      </span>
      <span>{t('Snapshot may be up to 5 seconds old')}</span>
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
    staleTime: CAPACITY_STALE_TIME,
    retry: 1,
    retryDelay: RETRY_COOLDOWN,
  })
  const allQuery = useQuery({
    queryKey: [...queryKeyBase, 'all'],
    queryFn: () => queryCapacity('all'),
    enabled: expanded && Boolean(user),
    staleTime: CAPACITY_STALE_TIME,
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
      (allIsStale && elapsed >= CAPACITY_STALE_TIME)
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
  if (topData?.total === 0 || allData?.total === 0) return null

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
          {t('RPM capacity')}
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
