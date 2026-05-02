import { memo } from 'react'
import { useTranslation } from 'react-i18next'
import { Database, Zap } from 'lucide-react'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { MonitoringGroupWithHistory } from '../api'
import {
  formatFRT,
  formatClock,
  isGroupOnline,
  rateAccentColor,
} from '../constants'
import StatusTimeline from './status-timeline'

interface GroupStatusCardProps {
  group: MonitoringGroupWithHistory
  onClick?: (group: MonitoringGroupWithHistory) => void
}

const GroupStatusCard = memo(function GroupStatusCard({
  group,
  onClick,
}: GroupStatusCardProps) {
  const { t } = useTranslation()

  const online = isGroupOnline(group)
  const availRate =
    group.availability_rate != null && group.availability_rate >= 0
      ? group.availability_rate
      : null
  const cacheRate =
    group.cache_hit_rate != null && group.cache_hit_rate >= 0
      ? group.cache_hit_rate
      : null
  const showCache = cacheRate != null && cacheRate >= 3
  const frt = group.avg_frt ?? group.first_response_time

  const dotColor = !online
    ? 'hsl(var(--destructive))'
    : availRate == null
      ? 'hsl(var(--muted-foreground) / 0.4)'
      : rateAccentColor(availRate)

  const headlineColor = !online
    ? 'hsl(var(--destructive))'
    : rateAccentColor(availRate)

  return (
    <div
      className="group border-border bg-card hover:border-primary/40 relative rounded-2xl border p-5 transition-all duration-200 hover:-translate-y-0.5 hover:shadow-lg"
      style={{ cursor: onClick ? 'pointer' : 'default' }}
      onClick={() => onClick?.(group)}
    >
      {/* Header: name + meta on left, big availability on right */}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span
              className={`inline-block h-2 w-2 shrink-0 rounded-full ${!online ? 'animate-pulse' : ''}`}
              style={{ background: dotColor }}
              aria-hidden
            />
            <span
              className="text-foreground block truncate text-sm font-semibold"
              title={group.group_name}
            >
              {group.group_name}
            </span>
          </div>
          <div className="text-muted-foreground mt-1.5 flex items-center gap-1.5 text-[11px]">
            {group.last_test_model && (
              <TooltipProvider delayDuration={200}>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="max-w-[140px] truncate font-mono">
                      {group.last_test_model}
                    </span>
                  </TooltipTrigger>
                  <TooltipContent>{group.last_test_model}</TooltipContent>
                </Tooltip>
              </TooltipProvider>
            )}
            {group.last_test_model && group.group_ratio != null && (
              <span className="opacity-60">&middot;</span>
            )}
            {group.group_ratio != null && (
              <span>
                {group.group_ratio}
                {t('CNY/USD')}
              </span>
            )}
          </div>
        </div>

        <div className="shrink-0 text-right leading-none">
          {availRate != null ? (
            <div
              className="font-mono text-[28px] font-semibold tracking-tight"
              style={{ color: headlineColor }}
            >
              {availRate.toFixed(1)}
              <span className="ml-0.5 text-base font-normal">%</span>
            </div>
          ) : (
            <div className="text-muted-foreground font-mono text-[28px] font-semibold tracking-tight">
              &mdash;
            </div>
          )}
          <div className="text-muted-foreground mt-1 text-[10px] uppercase tracking-wider">
            {online ? t('Availability') : t('Offline')}
          </div>
        </div>
      </div>

      {/* Status timeline */}
      <div className="mt-5">
        {group.history && group.history.length > 0 ? (
          <StatusTimeline history={group.history} segmentCount={32} compact />
        ) : (
          <div className="bg-muted/50 text-muted-foreground flex h-[22px] items-center justify-center rounded-md text-[10px]">
            {t('No history data')}
          </div>
        )}
      </div>

      {/* Footer: inline stats */}
      <div className="text-muted-foreground mt-4 flex items-center justify-between text-[11px]">
        <div className="flex items-center gap-3">
          <span className="inline-flex items-center gap-1">
            <Zap size={11} className="text-muted-foreground/70" />
            <span className="font-mono">{formatFRT(frt)}</span>
          </span>
          <span className="inline-flex items-center gap-1">
            <Database size={11} className="text-muted-foreground/70" />
            <span className="font-mono">
              {showCache ? `${cacheRate.toFixed(1)}%` : '—'}
            </span>
          </span>
        </div>
        <div className="flex items-center gap-3">
          {group.total_channels != null && (
            <span>
              <span className="text-foreground font-mono">
                {group.online_channels ?? 0}
              </span>
              <span className="text-muted-foreground">
                /{group.total_channels}
              </span>
            </span>
          )}
          {group.updated_at > 0 && (
            <span className="text-muted-foreground font-mono">
              {formatClock(group.updated_at)}
            </span>
          )}
        </div>
      </div>
    </div>
  )
})

export default GroupStatusCard
