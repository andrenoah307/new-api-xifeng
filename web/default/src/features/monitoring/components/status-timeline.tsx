import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { MonitoringHistoryPoint } from '../api'
import {
  formatFRT,
  formatDateTime,
  segmentColor,
  segmentLabel,
} from '../constants'

interface StatusTimelineProps {
  history: MonitoringHistoryPoint[]
  segmentCount?: number
  compact?: boolean
}

export default function StatusTimeline({
  history,
  segmentCount = 60,
  compact = false,
}: StatusTimelineProps) {
  const { t } = useTranslation()

  const segments = useMemo(() => {
    const sorted = (history || [])
      .filter(
        (h): h is MonitoringHistoryPoint =>
          h != null && typeof h.recorded_at === 'number'
      )
      .sort((a, b) => a.recorded_at - b.recorded_at)

    const slice = sorted.slice(-segmentCount)
    const pad: (MonitoringHistoryPoint | null)[] = Array(
      Math.max(0, segmentCount - slice.length)
    ).fill(null)
    return [...pad, ...slice]
  }, [history, segmentCount])

  const filled = segments.filter((s) => s != null).length
  const height = compact ? 22 : 32

  return (
    <div className="space-y-1.5">
      {!compact && (
        <div className="text-muted-foreground flex items-center justify-between text-[10px] uppercase tracking-wider">
          <span>
            {filled <= 1
              ? t('History (latest)')
              : t('History ({{n}} entries)', { n: filled })}
          </span>
          <span className="opacity-60">{t('Left to right: old to new')}</span>
        </div>
      )}
      <TooltipProvider delayDuration={100}>
        <div
          className="bg-muted/50 flex w-full overflow-hidden rounded-md"
          style={{ height, gap: 2, padding: 2 }}
        >
          {segments.map((seg, idx) => {
            const rate = seg?.availability_rate ?? null
            const avgFrt = seg?.avg_frt ?? null
            const reqCount = seg?.request_count ?? null
            const bg = segmentColor(rate, avgFrt, reqCount)
            const isEmpty = seg == null

            return (
              <Tooltip key={idx}>
                <TooltipTrigger asChild>
                  <div
                    className="flex-1 rounded-sm transition-opacity hover:opacity-80"
                    style={{
                      background: bg,
                      opacity: isEmpty ? 0.35 : 1,
                      minWidth: 2,
                    }}
                  />
                </TooltipTrigger>
                <TooltipContent side="top" className="max-w-xs">
                  {isEmpty ? (
                    <span className="text-xs">{t('No data available')}</span>
                  ) : (
                    <div className="space-y-0.5 text-xs">
                      <div className="font-medium">
                        {formatDateTime(seg.recorded_at)}
                      </div>
                      <div>
                        {t('Status')}: {segmentLabel(rate, avgFrt, t, reqCount)}
                      </div>
                      {rate != null && rate >= 0 && (
                        <div>
                          {t('Availability')}: {rate.toFixed(1)}%
                        </div>
                      )}
                      {avgFrt != null && avgFrt > 0 && (
                        <div>FRT: {formatFRT(avgFrt)}</div>
                      )}
                      {seg.cache_hit_rate != null &&
                        seg.cache_hit_rate >= 0 && (
                          <div>
                            {t('Cache Hit Rate')}: {seg.cache_hit_rate.toFixed(1)}%
                          </div>
                        )}
                    </div>
                  )}
                </TooltipContent>
              </Tooltip>
            )
          })}
        </div>
      </TooltipProvider>
    </div>
  )
}
