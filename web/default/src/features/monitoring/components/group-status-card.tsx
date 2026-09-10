import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useStatus } from '@/hooks/use-status'

import type { MonitoringGroupWithHistory } from '../api'
import GroupModelPerformance, {
  type GroupModelPerformanceProps,
} from './group-model-performance'
import {
  formatFRT,
  formatClock,
  isGroupOnline,
  rateAccentColor,
  computeRateFromHistory,
} from '../constants'
import StatusTimeline from './status-timeline'

interface GroupStatusCardProps {
  group: MonitoringGroupWithHistory
  onClick?: (group: MonitoringGroupWithHistory) => void
  regionBlockedGroups?: string[]
  modelPerformance?: GroupModelPerformanceProps
  variant?: 'grid' | 'wide'
}

const GroupStatusCard = memo(function GroupStatusCard({
  group,
  onClick,
  regionBlockedGroups = [],
  modelPerformance,
  variant = 'grid',
}: GroupStatusCardProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  // 管理员可在 地区限制 设置里自定义该提示，空则用默认文案
  const regionUnavailableText =
    String(status?.region_console_message ?? '').trim() ||
    t('Not available in current region')

  const online = isGroupOnline(group)
  const noData =
    (group.total_channels ?? 0) === 0 &&
    (group.availability_rate == null || group.availability_rate < 0)
  const historyAvail = computeRateFromHistory(
    group.history,
    'availability_rate'
  )
  const availRate =
    historyAvail ??
    (group.availability_rate != null && group.availability_rate >= 0
      ? group.availability_rate
      : null)
  const historyCacheRate = computeRateFromHistory(
    group.history,
    'cache_hit_rate'
  )
  const cacheRate =
    historyCacheRate ??
    (group.cache_hit_rate != null && group.cache_hit_rate >= 0
      ? group.cache_hit_rate
      : null)
  const showCache = cacheRate != null && cacheRate >= 3
  const frt = group.avg_frt ?? group.first_response_time

  let dotColor = rateAccentColor(availRate)
  if (noData) {
    dotColor = 'color-mix(in oklch, var(--muted-foreground) 40%, transparent)'
  } else if (!online) {
    dotColor = 'var(--destructive)'
  } else if (availRate == null) {
    dotColor = 'color-mix(in oklch, var(--muted-foreground) 40%, transparent)'
  }

  let headlineColor = rateAccentColor(availRate)
  if (noData) {
    headlineColor = 'var(--muted-foreground)'
  } else if (!online) {
    headlineColor = 'var(--destructive)'
  }

  let headlineLabel = t('Group availability')
  if (noData) {
    headlineLabel = t('No data available')
  } else if (!online) {
    headlineLabel = t('Offline')
  }

  const splitLayout = Boolean(modelPerformance) && variant === 'wide'

  // 头部 / 时序条 / 底部统计三段始终是一个整体：四列骨架下它们共同占左两列。
  // 把它们各自直接挂到 Grid 上会被逐格换行摆放，左右分栏就永远不会生效。
  const groupBody = (
    <>
      {/* Header: name + meta on left, channel count + clock on right.
          可用率大号数字已下沉到底部指标栏，这里让出的空间接管两项元信息。 */}
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0 flex-1'>
          <div className='flex items-center gap-2'>
            <span
              className={`inline-block h-2 w-2 shrink-0 rounded-full ${!online && !noData ? 'animate-pulse' : ''}`}
              style={{ background: dotColor }}
              aria-hidden
            />
            <span
              className='text-foreground block truncate text-sm font-semibold'
              title={group.group_name}
            >
              {group.group_name}
            </span>
            {regionBlockedGroups.includes(group.group_name) && (
              <span className='text-destructive shrink-0 text-[10px]'>
                {regionUnavailableText}
              </span>
            )}
          </div>
          <div className='text-muted-foreground mt-1.5 flex items-center gap-1.5 text-[11px]'>
            {group.last_test_model && (
              <TooltipProvider delay={200}>
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <span className='max-w-[140px] truncate font-mono' />
                    }
                  >
                    {group.last_test_model}
                  </TooltipTrigger>
                  <TooltipContent>{group.last_test_model}</TooltipContent>
                </Tooltip>
              </TooltipProvider>
            )}
            {group.last_test_model && group.group_ratio != null && (
              <span className='opacity-60'>&middot;</span>
            )}
            {group.group_ratio != null && (
              <span>
                {group.group_ratio}
                {t('CNY/USD')}
              </span>
            )}
          </div>
        </div>

        <div className='text-muted-foreground shrink-0 space-y-1 text-right text-[11px] leading-none'>
          {group.total_channels != null && (
            <div className='whitespace-nowrap'>
              <span className='text-foreground font-mono'>
                {group.online_channels ?? 0}
              </span>
              <span>/{group.total_channels}</span>
            </div>
          )}
          {group.updated_at > 0 && (
            <div className='font-mono whitespace-nowrap'>
              {formatClock(group.updated_at)}
            </div>
          )}
        </div>
      </div>

      {/* 时序条吃掉整段富余高度并垂直居中：宽卡片里右侧模型表有 6-20 行，
          左列若不拉伸就会在左下角留下与右列等高的死白。窄卡片没有富余，
          flex-1 退化成"占一行"，与改动前等价。 */}
      <div data-perf-row='timeline' className='mt-5 flex flex-1 items-center'>
        <div className='w-full'>
          {group.history && group.history.length > 0 ? (
            <StatusTimeline history={group.history} segmentCount={32} compact />
          ) : (
            <div className='bg-muted/50 text-muted-foreground flex h-[22px] items-center justify-center rounded-md text-[10px]'>
              {t('No history data')}
            </div>
          )}
        </div>
      </div>

      {/* 三个服务质量指标共享同一条基线与同一种字号，才能横向对照。
          此前可用率是头部右上的 28px 大字、首字与缓存是左下角的 11px 小字，
          三者没有可比性。等分三列让每格宽度随卡片自适应。 */}
      <div className='border-border/60 mt-4 grid grid-cols-3 gap-2 border-t pt-3'>
        <div data-perf-stat='availability' className='min-w-0'>
          <div className='text-muted-foreground truncate text-[10px] tracking-wider uppercase'>
            {headlineLabel}
          </div>
          <div
            className='mt-1 font-mono text-xl leading-none font-semibold tracking-tight tabular-nums'
            style={{ color: availRate != null ? headlineColor : undefined }}
          >
            {availRate != null ? (
              <>
                {availRate.toFixed(1)}
                <span className='ml-0.5 text-xs font-normal'>%</span>
              </>
            ) : (
              <span className='text-muted-foreground'>&mdash;</span>
            )}
          </div>
        </div>
        <div data-perf-stat='ttft' className='min-w-0'>
          <div
            className='text-muted-foreground truncate text-[10px] tracking-wider uppercase'
            title={t('Group first token latency')}
          >
            {t('First token latency short')}
          </div>
          <div className='text-foreground mt-1 font-mono text-base leading-none tabular-nums'>
            {formatFRT(frt)}
          </div>
        </div>
        <div data-perf-stat='cache' className='min-w-0'>
          <div className='text-muted-foreground truncate text-[10px] tracking-wider uppercase'>
            {t('Cache')}
          </div>
          <div className='text-foreground mt-1 font-mono text-base leading-none tabular-nums'>
            {showCache ? `${cacheRate.toFixed(1)}%` : '—'}
          </div>
        </div>
      </div>
    </>
  )

  return (
    <div
      className={`group border-border bg-card hover:border-primary/40 relative min-w-0 rounded-2xl border p-5 transition-all duration-200 hover:-translate-y-0.5 hover:shadow-lg ${splitLayout ? '@container/perfcard' : ''}`}
      style={{ cursor: onClick ? 'pointer' : 'default' }}
      onClick={() => onClick?.(group)}
    >
      {splitLayout && modelPerformance ? (
        <div className='grid grid-cols-1 gap-6 @7xl/perfcard:grid-cols-4'>
          <div
            data-perf-col='group'
            className='flex h-full min-w-0 flex-col @7xl/perfcard:col-span-1'
          >
            {groupBody}
          </div>
          <div
            data-perf-col='models'
            className='min-w-0 @7xl/perfcard:col-span-3'
          >
            <GroupModelPerformance {...modelPerformance} />
          </div>
        </div>
      ) : (
        <>
          {groupBody}
          {modelPerformance && (
            <div className='mt-5 min-w-0'>
              <GroupModelPerformance {...modelPerformance} />
            </div>
          )}
        </>
      )}
    </div>
  )
})

export default GroupStatusCard
