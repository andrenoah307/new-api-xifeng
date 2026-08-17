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
import { RefreshIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { ChevronDown, ChevronUp, Gauge } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { Spinner } from '@/components/ui/spinner'
import { PanelWrapper } from '@/features/dashboard/components/ui/panel-wrapper'
import { getRateLimitCapacity } from '@/features/dashboard/api'
import {
  normalizePersonalRPMItems,
  personalRPMDisplayState,
  PERSONAL_RPM_STALE_TIME,
} from '@/features/dashboard/lib/personal-rpm'
import type {
  RateLimitCapacityGroup,
  RateLimitCapacityMetric,
  RateLimitCapacityPersonal,
  RateLimitCapacityResponse,
} from '@/features/dashboard/types'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

const RETRY_COOLDOWN = 3_000

// eslint-disable-next-line react-refresh/only-export-components -- shared with the focused refresh behavior tests
export async function refetchCapacityQueries(
  anyExpanded: boolean,
  refetchTop: () => Promise<unknown>,
  refetchAll: () => Promise<unknown>
): Promise<void> {
  const requests = [refetchTop()]
  if (anyExpanded) requests.push(refetchAll())
  await Promise.all(requests)
}

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

function CapacitySectionHeader(props: {
  title: string
  windowLabel: string
  total: number
  displayedCount: number
  expanded: boolean
  loading: boolean
  controlsId: string
  label: 'models' | 'groups'
  onIntent: () => void
  onToggle: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='bg-muted/30 border-border/60 flex items-center justify-between gap-3 border-b px-4 py-3 sm:px-5'>
      <div className='min-w-0'>
        <h3 className='text-sm font-semibold'>{props.title}</h3>
        <p className='text-muted-foreground mt-0.5 text-xs'>
          {props.windowLabel} · {t('All users')}
        </p>
      </div>
      {(props.expanded || props.total > props.displayedCount) && (
        <CapacityExpandButton
          expanded={props.expanded}
          total={props.total}
          loading={props.loading}
          label={props.label}
          controlsId={props.controlsId}
          onIntent={props.onIntent}
          onToggle={props.onToggle}
        />
      )}
    </div>
  )
}

function CapacityGroupCard(props: {
  group: RateLimitCapacityGroup
  index: number
  expanded: boolean
  loading: boolean
  onIntent: () => void
  onToggle: () => void
}) {
  const { t } = useTranslation()
  const contentId = `rate-limit-capacity-group-${props.index}`
  const items = props.expanded ? props.group.items : props.group.items.slice(0, 3)
  return (
    <article className='rounded-lg border border-border/60 bg-card/30 p-3'>
      <div className='min-w-0'>
        <div className='truncate text-sm font-semibold' title={props.group.group}>
          {props.group.group}
        </div>
        <div className='text-muted-foreground mt-0.5 text-xs'>
          {t('{{count}} models', { count: props.group.total })}
        </div>
      </div>
      <div
        id={contentId}
        className='mt-3 grid gap-3'
        style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(13rem, 1fr))' }}
      >
        {items.map((item) => (
          <CapacityRow
            key={`${item.model}:${item.group ?? ''}`}
            label={item.model}
            metric={item}
          />
        ))}
      </div>
      {(props.expanded || props.group.total > items.length) && (
        <div className='mt-2 flex justify-end'>
          <CapacityExpandButton
            expanded={props.expanded}
            total={props.group.total}
            loading={props.loading}
            label='models'
            controlsId={contentId}
            onIntent={props.onIntent}
            onToggle={props.onToggle}
          />
        </div>
      )}
    </article>
  )
}

function PersonalSection(props: { personal: RateLimitCapacityPersonal }) {
  const { t } = useTranslation()
  const personal = props.personal
  const items = normalizePersonalRPMItems(personal.items)
  const displayState = personalRPMDisplayState(personal.status, items)

  return (
    <section aria-label={t('My model RPM')}>
      <div className='bg-muted/30 border-border/60 border-b px-4 py-3 sm:px-5'>
        <h3 className='text-sm font-semibold'>{t('My model RPM')}</h3>
        <p className='text-muted-foreground mt-0.5 text-xs'>{t('Last 60 seconds')}</p>
      </div>
      {displayState === 'empty' && (
        <div className='text-muted-foreground px-4 py-4 text-sm sm:px-5'>
          {t('No request data yet')}
        </div>
      )}
      {displayState === 'unavailable' && (
        <div className='text-muted-foreground px-4 py-4 text-sm sm:px-5'>
          {t('Temporarily unavailable')}
        </div>
      )}
      {displayState === 'available' && (
        <div
          className='grid gap-3 px-4 py-3 sm:px-5'
          style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(15rem, 1fr))' }}
        >
          {items.map((item) => (
            <CapacityRow
              key={item.model}
              label={item.model}
              metric={item}
            />
          ))}
        </div>
      )}
    </section>
  )
}

function CapacityMetadata(props: {
  data: RateLimitCapacityResponse
  refreshing: boolean
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const instanceOnly =
    props.data.instance_only ||
    props.data.backend_scope === 'instance' ||
    Boolean(props.data.personal?.instance_only)
  return (
    <div className='text-muted-foreground flex flex-wrap items-center justify-between gap-3 px-4 py-3 text-xs sm:px-5'>
      <div className='flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1'>
        <span>
          {t('Data updated: {{time}}', {
            time: formatObservedAt(props.data.observed_at),
          })}
        </span>
        <span>{t('Click refresh for the latest data')}</span>
        {instanceOnly && (
          <span className='text-warning'>{t('Instance-only data')}</span>
        )}
      </div>
      <Button
        type='button'
        variant='outline'
        size='sm'
        className='min-w-28'
        onClick={props.onRefresh}
        disabled={props.refreshing}
        aria-label={t('Refresh')}
        aria-busy={props.refreshing}
      >
        {props.refreshing ? (
          <Spinner data-icon='inline-start' aria-label={t('Loading')} />
        ) : (
          <HugeiconsIcon
            icon={RefreshIcon}
            data-icon='inline-start'
            aria-hidden='true'
          />
        )}
        {props.refreshing ? t('Refreshing...') : t('Refresh')}
      </Button>
    </div>
  )
}

function CapacityExpandButton(props: {
  expanded: boolean
  total: number
  loading: boolean
  label: 'models' | 'groups'
  controlsId: string
  onIntent: () => void
  onToggle: () => void
}) {
  const { t } = useTranslation()
  let icon = <ChevronDown className='size-3.5' aria-hidden='true' />
  let label = t('Show all {{total}} models', { total: props.total })
  if (props.expanded) {
    icon = <ChevronUp className='size-3.5' aria-hidden='true' />
    label = t('Collapse')
  } else if (props.loading) {
    icon = <Spinner className='size-3.5' aria-label={t('Loading')} />
  }
  if (!props.expanded && props.label === 'groups') {
    label = t('Show all {{total}} groups', { total: props.total })
  }
  return (
    <button
      type='button'
      className='text-primary focus-visible:ring-ring inline-flex min-h-8 items-center gap-1 rounded-md px-2 text-xs font-medium outline-none focus-visible:ring-2'
      aria-expanded={props.expanded}
      aria-controls={props.controlsId}
      onMouseEnter={props.onIntent}
      onFocus={props.onIntent}
      onTouchStart={props.onIntent}
      onClick={props.onToggle}
    >
      {label}
      {icon}
    </button>
  )
}

export function RateLimitCapacityPanel() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const [globalExpanded, setGlobalExpanded] = useState(false)
  const [groupsExpanded, setGroupsExpanded] = useState(false)
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>(
    {},
  )
  const allAttemptAtRef = useRef(0)

  const identityKey = `${user?.id ?? 'anonymous'}:${user?.group ?? ''}`
  const queryKeyBase = ['dashboard', 'rate-limit-capacity', identityKey]

  useEffect(() => {
    setGlobalExpanded(false)
    setGroupsExpanded(false)
    setExpandedGroups({})
  }, [identityKey])

  const queryCapacity = useCallback(async (scope: 'top' | 'all') => {
    if (scope === 'all') allAttemptAtRef.current = Date.now()
    const result = await getRateLimitCapacity(scope)
    if (!result.success || !result.data) {
      throw new Error(result.message || 'capacity request failed')
    }
    return result.data
  }, [])

  const anyGroupExpanded = Object.values(expandedGroups).some(Boolean)
  const anyExpanded = globalExpanded || groupsExpanded || anyGroupExpanded

  const topQuery = useQuery({
    queryKey: [...queryKeyBase, 'top'],
    queryFn: () => queryCapacity('top'),
    enabled: Boolean(user),
    staleTime: PERSONAL_RPM_STALE_TIME,
    refetchOnWindowFocus: false,
    retry: 1,
    retryDelay: RETRY_COOLDOWN,
  })
  const allQuery = useQuery({
    queryKey: [...queryKeyBase, 'all'],
    queryFn: () => queryCapacity('all'),
    enabled: anyExpanded && Boolean(user),
    staleTime: PERSONAL_RPM_STALE_TIME,
    refetchOnWindowFocus: false,
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
      (allIsStale && elapsed >= PERSONAL_RPM_STALE_TIME)
    ) {
      void refetchAll()
    }
  }, [allData, allIsError, allIsFetching, allIsStale, refetchAll])

  const handleRefresh = useCallback(() => {
    void refetchCapacityQueries(anyExpanded, topQuery.refetch, refetchAll)
  }, [anyExpanded, refetchAll, topQuery.refetch])

  const toggleGlobal = useCallback(() => {
    if (!globalExpanded) handleExpandIntent()
    setGlobalExpanded((value) => !value)
  }, [globalExpanded, handleExpandIntent])

  const toggleGroups = useCallback(() => {
    if (!groupsExpanded) handleExpandIntent()
    setGroupsExpanded((value) => !value)
  }, [groupsExpanded, handleExpandIntent])

  const toggleGroup = useCallback(
    (groupName: string) => {
      if (!expandedGroups[groupName]) handleExpandIntent()
      setExpandedGroups((value) => ({
        ...value,
        [groupName]: !value[groupName],
      }))
    },
    [expandedGroups, handleExpandIntent],
  )

  const topData = topQuery.data
  // The all snapshot is a source only while an area is expanded. Once
  // collapsed, return to the top snapshot for personal data and metadata.
  const data = anyExpanded && allData ? allData : (topData || allData)
  const site = data?.site
  const global = site?.global
  const groups = site?.groups
  const globalItems = global?.items ?? []
  const groupItems = groups?.groups ?? []
  const displayedGlobalItems = globalExpanded
    ? globalItems
    : globalItems.slice(0, 3)
  const displayedGroups = groupsExpanded ? groupItems : groupItems.slice(0, 3)
  const topLoading = topQuery.isPending && !topData
  const refreshing = topQuery.isFetching || allIsFetching

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

      {anyExpanded && allIsFetching && (
        <div className='text-muted-foreground flex items-center gap-2 border-border/60 border-b px-4 py-3 text-xs sm:px-5'>
          <Spinner className='size-3.5' aria-label={t('Loading')} />
          {t('Loading')}
        </div>
      )}
      {anyExpanded && allIsError && (
        <div className='text-muted-foreground border-border/60 border-b px-4 py-3 text-xs sm:px-5'>
          {t('Temporarily unavailable')}
        </div>
      )}

      {global && (global.total > 0 || globalItems.length > 0) && (
        <section aria-label={t('Site model RPM')}>
          <CapacitySectionHeader
            title={t('Site model RPM')}
            windowLabel={t('60-second window')}
            total={global.total}
            displayedCount={displayedGlobalItems.length}
            expanded={globalExpanded}
            loading={allIsFetching}
            label='models'
            controlsId='rate-limit-capacity-global'
            onIntent={handleExpandIntent}
            onToggle={toggleGlobal}
          />
          <div
            id='rate-limit-capacity-global'
            className='grid gap-3 px-4 py-3 sm:px-5'
            style={{
              gridTemplateColumns: 'repeat(auto-fill, minmax(15rem, 1fr))',
            }}
          >
            {displayedGlobalItems.map((item) => (
              <CapacityRow
                key={`${item.model}:${item.group ?? ''}`}
                label={item.model}
                metric={item}
              />
            ))}
          </div>
        </section>
      )}

      {groups && (groups.total > 0 || groupItems.length > 0) && (
        <section aria-label={t('Site group RPM')}>
          <CapacitySectionHeader
            title={t('Site group RPM')}
            windowLabel={t('60-second window')}
            total={groups.total}
            displayedCount={displayedGroups.length}
            expanded={groupsExpanded}
            loading={allIsFetching}
            label='groups'
            controlsId='rate-limit-capacity-groups'
            onIntent={handleExpandIntent}
            onToggle={toggleGroups}
          />
          <div
            id='rate-limit-capacity-groups'
            className='grid gap-3 px-4 py-3 sm:px-5'
            style={{
              gridTemplateColumns: 'repeat(auto-fill, minmax(20rem, 1fr))',
            }}
          >
            {displayedGroups.map((group, index) => (
              <CapacityGroupCard
                key={group.group}
                group={group}
                index={index}
                expanded={Boolean(expandedGroups[group.group])}
                loading={allIsFetching}
                onIntent={handleExpandIntent}
                onToggle={() => toggleGroup(group.group)}
              />
            ))}
          </div>
        </section>
      )}

      {data?.personal && <PersonalSection personal={data.personal} />}
      {data && (
        <CapacityMetadata
          data={data}
          refreshing={refreshing}
          onRefresh={handleRefresh}
        />
      )}
    </PanelWrapper>
  )
}
