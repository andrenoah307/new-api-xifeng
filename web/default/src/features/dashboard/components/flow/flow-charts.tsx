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
import { Fragment, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { VChart } from '@visactor/react-vchart'
import {
  Activity,
  ChevronRight,
  CircleAlert,
  EyeOff,
  GitBranch,
  Hash,
  Info,
  Loader2,
  Route,
  WalletCards,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Toggle } from '@/components/ui/toggle'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { getFlowQuotaDates } from '@/features/dashboard/api'
import {
  buildDashboardFlowData,
  buildFlowSankeySpec,
  buildQueryParams,
  getDefaultDays,
  getFlowStages,
} from '@/features/dashboard/lib'
import {
  compactFlowSelectionLabel,
  flowDisplayState,
  requireSuccessfulFlowRows,
} from '@/features/dashboard/lib/flow-selection'
import type {
  DashboardFilters,
  FlowMetric,
  FlowNodeKind,
  FlowOverflowMode,
  FlowRole,
} from '@/features/dashboard/types'
import { formatQuota } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { computeTimeRange } from '@/lib/time'
import { useChartTheme } from '@/lib/use-chart-theme'
import { cn } from '@/lib/utils'
import { VCHART_OPTION } from '@/lib/vchart'
import { useAuthStore } from '@/stores/auth-store'

interface FlowChartsProps {
  filters?: DashboardFilters
}

const FLOW_METRIC_OPTIONS = [
  { value: 'quota', labelKey: 'By quota', icon: WalletCards },
  { value: 'tokens', labelKey: 'By tokens', icon: Hash },
  { value: 'requests', labelKey: 'By requests', icon: Activity },
] as const

const FLOW_TOP_LIMIT_OPTIONS = [10, 20, 50, 100] as const

const DEFAULT_FLOW_TOP_NODE_LIMIT = 50

const FLOW_OVERFLOW_MODE_OPTIONS = [
  { value: 'aggregate', labelKey: 'Merge into Other' },
  { value: 'hide', labelKey: 'Hide' },
] as const

// A Sankey needs at least two columns to render any link.
const MIN_VISIBLE_STAGES = 2

const FLOW_STAGE_META: Record<
  FlowNodeKind,
  { labelKey: string; descKey: string }
> = {
  user: {
    labelKey: 'User',
    descKey: 'The user who made the requests',
  },
  node: {
    labelKey: 'Node',
    descKey: 'The deployment node that handled the requests',
  },
  token: {
    labelKey: 'Token',
    descKey: 'The API key used for the requests',
  },
  group: {
    labelKey: 'Group',
    descKey: 'The user group applied to the requests',
  },
  model: {
    labelKey: 'Model',
    descKey: 'The model that was requested',
  },
  channel: {
    labelKey: 'Channel',
    descKey: 'The upstream channel that served the requests',
  },
}

const FLOW_OTHER_NODE_LABEL_KEYS: Record<FlowNodeKind, string> = {
  user: 'Other users',
  node: 'Other nodes',
  token: 'Other tokens',
  group: 'Other groups',
  model: 'Other models',
  channel: 'Other channels',
}

export function FlowCharts(props: FlowChartsProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const user = useAuthStore((state) => state.auth.user)
  const isRoot = Boolean(user?.role && user.role >= ROLE.SUPER_ADMIN)
  const isAdmin = Boolean(user?.role && user.role >= ROLE.ADMIN)
  const flowRole: FlowRole = isRoot ? 'root' : isAdmin ? 'admin' : 'user'
  const [metric, setMetric] = useState<FlowMetric>('quota')
  const [topNodeLimit, setTopNodeLimit] = useState(DEFAULT_FLOW_TOP_NODE_LIMIT)
  const [overflowMode, setOverflowMode] =
    useState<FlowOverflowMode>('aggregate')
  const [selectedUsers, setSelectedUsers] = useState<string[]>([])
  const [hiddenStages, setHiddenStages] = useState<FlowNodeKind[]>([])

  const stages = useMemo(() => getFlowStages(flowRole), [flowRole])
  const visibleStages = useMemo(
    () => stages.filter((stage) => !hiddenStages.includes(stage)),
    [stages, hiddenStages]
  )
  const toggleStage = (stage: FlowNodeKind) => {
    setHiddenStages((prev) => {
      const hidden = new Set(prev)
      if (hidden.has(stage)) {
        hidden.delete(stage)
      } else {
        const remaining = stages.filter((item) => !hidden.has(item)).length
        if (remaining <= MIN_VISIBLE_STAGES) return prev
        hidden.add(stage)
      }
      return stages.filter((item) => hidden.has(item))
    })
  }

  const timeRange = useMemo(
    () =>
      computeTimeRange(
        getDefaultDays(props.filters?.time_granularity),
        props.filters?.start_timestamp,
        props.filters?.end_timestamp
      ),
    [
      props.filters?.end_timestamp,
      props.filters?.start_timestamp,
      props.filters?.time_granularity,
    ]
  )
  const flowQueryParams = useMemo(
    () => buildQueryParams(timeRange, props.filters),
    [props.filters, timeRange]
  )

  const {
    data: flowRows,
    error: flowError,
    isError,
    isLoading,
  } = useQuery({
    queryKey: ['dashboard', 'flow', flowQueryParams, flowRole],
    queryFn: () => getFlowQuotaDates(flowQueryParams, isAdmin),
    select: (res) =>
      requireSuccessfulFlowRows(res, t('Please try again later.')),
    staleTime: 60_000,
  })

  const flowData = useMemo(
    () =>
      buildDashboardFlowData(isLoading ? [] : (flowRows ?? []), metric, {
        role: flowRole,
        selectedUsers,
        visibleStages,
        topNodeLimit,
        overflowMode,
        deletedTokenLabel: (tokenId) => t('Deleted ({{id}})', { id: tokenId }),
        otherNodeLabel: (kind) => t(FLOW_OTHER_NODE_LABEL_KEYS[kind]),
      }),
    [
      flowRole,
      flowRows,
      isLoading,
      metric,
      overflowMode,
      selectedUsers,
      topNodeLimit,
      visibleStages,
      t,
    ]
  )
  const userFilterOptions = useMemo(
    () =>
      flowData.filterOptions.users.map((user) => ({
        label: `${user.label} · ${user.valueLabel}`,
        value: user.value,
      })),
    [flowData.filterOptions.users]
  )
  const chartTitle = t('Flow')
  const flowSpec = useMemo(
    () =>
      buildFlowSankeySpec(flowData.flow, chartTitle, formatQuota, {
        quota: t('Quota'),
        tokens: t('Tokens'),
        requests: t('Requests'),
        share: t('Share'),
      }),
    [chartTitle, flowData.flow, t]
  )
  const chartTheme = resolvedTheme === 'dark' ? 'dark' : 'light'
  const chartKey = [
    metric,
    topNodeLimit,
    overflowMode,
    flowRole,
    selectedUsers.join(','),
    visibleStages.join(','),
    flowRows?.length ?? 0,
    resolvedTheme,
  ].join('-')
  const displayState = flowDisplayState({
    isLoading,
    isError,
    linkCount: flowData.flow.links.length,
    themeReady,
  })
  const flowErrorMessage =
    flowError instanceof Error
      ? flowError.message
      : t('Please try again later.')

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-col gap-2 xl:flex-row xl:items-end xl:justify-between'>
        <div className='flex min-w-0 flex-wrap items-end gap-2'>
          <div className='flex min-w-0 flex-col gap-1.5'>
            <div className='flex items-center gap-1.5'>
              <span className='text-muted-foreground text-xs font-medium'>
                {t('Flow width metric')}
              </span>
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <button
                        type='button'
                        className='text-muted-foreground/60 hover:text-foreground flex size-5 shrink-0 items-center justify-center rounded-md'
                        aria-label={t('Flow width metric')}
                      />
                    }
                  >
                    <Info className='size-3.5' />
                  </TooltipTrigger>
                  <TooltipContent className='max-w-[14rem]'>
                    {t('Choose how flow widths are calculated.')}
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
            <Tabs
              value={metric}
              onValueChange={(value) => setMetric(value as FlowMetric)}
              className='shrink-0'
            >
              <TabsList aria-label={t('Flow width metric')}>
                {FLOW_METRIC_OPTIONS.map((option) => {
                  const Icon = option.icon
                  return (
                    <TabsTrigger
                      key={option.value}
                      value={option.value}
                      className='gap-1.5 px-2.5 text-xs'
                    >
                      <Icon data-icon='inline-start' aria-hidden='true' />
                      {t(option.labelKey)}
                    </TabsTrigger>
                  )
                })}
              </TabsList>
            </Tabs>
          </div>

          <div className='flex min-w-0 flex-col gap-1.5'>
            <span className='text-muted-foreground text-xs font-medium'>
              {t('Display limit')}
            </span>
            <Tabs
              value={String(topNodeLimit)}
              onValueChange={(value) => setTopNodeLimit(Number(value))}
              className='shrink-0'
            >
              <TabsList aria-label={t('Display limit')}>
                {FLOW_TOP_LIMIT_OPTIONS.map((limit) => (
                  <TabsTrigger
                    key={limit}
                    value={String(limit)}
                    className='px-2.5 text-xs'
                  >
                    {t('Top {{count}}', { count: limit })}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          </div>

          <div className='flex min-w-0 flex-col gap-1.5'>
            <span className='text-muted-foreground text-xs font-medium'>
              {t('Overflow items')}
            </span>
            <Tabs
              value={overflowMode}
              onValueChange={(value) =>
                setOverflowMode(value as FlowOverflowMode)
              }
              className='shrink-0'
            >
              <TabsList aria-label={t('Overflow items')}>
                {FLOW_OVERFLOW_MODE_OPTIONS.map((option) => (
                  <TabsTrigger
                    key={option.value}
                    value={option.value}
                    className='px-2.5 text-xs'
                  >
                    {t(option.labelKey)}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          </div>
        </div>

        <div className='flex min-w-0 items-center gap-2 xl:justify-end'>
          {isAdmin && (
            <div className='flex min-w-0 flex-col gap-2 sm:flex-row xl:w-[min(24rem,34vw)]'>
              <MultiSelect
                options={userFilterOptions}
                selected={selectedUsers}
                onChange={setSelectedUsers}
                placeholder={t('All users')}
                emptyText={t('No users')}
                maxVisibleChips={2}
                renderSelectedSummary={(values) =>
                  compactFlowSelectionLabel(values.length)
                }
              />
            </div>
          )}
          {isLoading && (
            <Loader2 className='text-muted-foreground size-4 animate-spin' />
          )}
        </div>
      </div>

      <div className='overflow-hidden rounded-lg border'>
        <div className='flex w-full flex-col gap-2 border-b px-3 py-2 sm:px-5 sm:py-3 lg:flex-row lg:items-center lg:justify-between'>
          <div className='flex min-w-0 items-center gap-2'>
            <GitBranch className='text-muted-foreground/60 size-4 shrink-0' />
            <div className='text-sm font-semibold'>{chartTitle}</div>
          </div>
          <TooltipProvider>
            <div className='flex min-w-0 items-center gap-1 overflow-x-auto pb-1 lg:justify-end lg:pb-0'>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <button
                      type='button'
                      className='text-muted-foreground/60 hover:text-foreground flex size-6 shrink-0 items-center justify-center rounded-md'
                      aria-label={t('Show or hide flow columns')}
                    />
                  }
                >
                  <Info className='size-3.5' />
                </TooltipTrigger>
                <TooltipContent className='max-w-[16rem]'>
                  {t('Click a stage to show or hide that column')}
                </TooltipContent>
              </Tooltip>
              {stages.map((stage, index) => {
                const meta = FLOW_STAGE_META[stage]
                const visible = !hiddenStages.includes(stage)
                return (
                  <Fragment key={stage}>
                    {index > 0 && (
                      <ChevronRight className='text-muted-foreground/40 size-3.5 shrink-0' />
                    )}
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <Toggle
                            variant='outline'
                            size='sm'
                            pressed={visible}
                            onPressedChange={() => toggleStage(stage)}
                            aria-label={t(meta.labelKey)}
                            className={cn('shrink-0', !visible && 'opacity-50')}
                          />
                        }
                      >
                        {!visible && <EyeOff className='size-3' />}
                        {t(meta.labelKey)}
                      </TooltipTrigger>
                      <TooltipContent>{t(meta.descKey)}</TooltipContent>
                    </Tooltip>
                  </Fragment>
                )
              })}
            </div>
          </TooltipProvider>
        </div>
        <div className='h-[560px] p-1.5 sm:h-[680px] sm:p-2 2xl:h-[760px]'>
          {displayState === 'loading' ? (
            <Skeleton className='h-full w-full' />
          ) : displayState === 'error' ? (
            <div className='flex h-full items-center justify-center p-4'>
              <Alert variant='destructive' className='max-w-md'>
                <CircleAlert />
                <AlertTitle>{t('Failed to load')}</AlertTitle>
                <AlertDescription>{flowErrorMessage}</AlertDescription>
              </Alert>
            </div>
          ) : displayState === 'empty' ? (
            <Empty className='h-full border-0 py-12'>
              <EmptyHeader>
                <EmptyMedia variant='icon'>
                  <Route />
                </EmptyMedia>
                <EmptyTitle>{t('No flow data available')}</EmptyTitle>
                <EmptyDescription>{t('No data available')}</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <VChart
              key={`flow-${chartKey}`}
              spec={{
                ...flowSpec,
                theme: chartTheme,
                background: 'transparent',
              }}
              option={VCHART_OPTION}
            />
          )}
        </div>
      </div>
    </div>
  )
}
