import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Activity, Database, Zap } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { useIsAdmin } from '@/hooks/use-admin'
import { useIsMobile } from '@/hooks/use-mobile'
import type { MonitoringGroupWithHistory, ChannelStat } from '../api'
import { getGroupDetail, getGroupHistory } from '../api'
import {
  formatFRT,
  isGroupOnline,
  rateAccentColor,
  rateVariant,
} from '../constants'
import StatusTimeline from './status-timeline'
import AvailabilityCacheChart from './availability-chart'

interface GroupDetailPanelProps {
  open: boolean
  group: MonitoringGroupWithHistory | null
  onOpenChange: (open: boolean) => void
}

function StatCard({
  icon,
  label,
  value,
  valueColor,
}: {
  icon: React.ReactNode
  label: string
  value: string
  valueColor?: string
}) {
  return (
    <div className="border-border bg-card rounded-xl border p-3">
      <div className="text-muted-foreground flex items-center gap-1.5">
        {icon}
        <span className="text-[10px] font-semibold uppercase tracking-wider">
          {label}
        </span>
      </div>
      <div
        className="text-foreground mt-1.5 font-mono text-xl font-semibold leading-none"
        style={valueColor ? { color: valueColor } : undefined}
      >
        {value}
      </div>
    </div>
  )
}

function channelStatusVariant(
  enabled: boolean
): 'default' | 'secondary' | 'destructive' | 'outline' {
  return enabled ? 'default' : 'outline'
}

export default function GroupDetailPanel({
  open,
  group,
  onOpenChange,
}: GroupDetailPanelProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const admin = useIsAdmin()

  const groupName = group?.group_name ?? ''

  const {
    data: detail,
    isLoading: detailLoading,
  } = useQuery({
    queryKey: ['monitoring', 'detail', groupName],
    queryFn: () => getGroupDetail(groupName),
    enabled: open && !!groupName && admin,
    staleTime: 30_000,
  })

  const {
    data: historyData,
    isLoading: historyLoading,
  } = useQuery({
    queryKey: ['monitoring', 'history', groupName, admin],
    queryFn: () => getGroupHistory(groupName, admin),
    enabled: open && !!groupName,
    staleTime: 30_000,
  })

  const history = historyData?.history ?? []
  const intervalMinutes = historyData?.intervalMinutes ?? 5
  const loading = detailLoading || historyLoading

  const online = group ? isGroupOnline(group) : false
  const availRate =
    group?.availability_rate != null && group.availability_rate >= 0
      ? group.availability_rate
      : null
  const cacheRate =
    group?.cache_hit_rate != null && group.cache_hit_rate >= 0
      ? group.cache_hit_rate
      : null
  const showCache = cacheRate != null && cacheRate >= 3

  const channelData = useMemo(() => {
    if (!detail?.channel_stats) return []
    return detail.channel_stats
  }, [detail])

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className={isMobile ? 'w-full max-w-full' : 'w-full max-w-2xl'}
      >
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            <Badge variant={online ? 'default' : 'destructive'}>
              {online ? t('Online') : t('Offline')}
            </Badge>
            <span>{groupName}</span>
            <span className="text-muted-foreground text-sm font-normal">
              {t('Group Detail')}
            </span>
          </SheetTitle>
        </SheetHeader>

        <div className="space-y-5 overflow-y-auto p-4 sm:p-5">
          {/* Hero metrics */}
          <div className="grid grid-cols-3 gap-3">
            <StatCard
              icon={<Activity size={12} />}
              label={t('Availability')}
              value={availRate != null ? `${availRate.toFixed(1)}%` : 'N/A'}
              valueColor={rateAccentColor(availRate)}
            />
            <StatCard
              icon={<Zap size={12} />}
              label="FRT"
              value={formatFRT(
                group?.avg_frt ?? group?.first_response_time
              )}
            />
            <StatCard
              icon={<Database size={12} />}
              label={t('Cache Hit Rate')}
              value={showCache ? `${cacheRate.toFixed(1)}%` : '—'}
              valueColor={showCache ? rateAccentColor(cacheRate) : undefined}
            />
          </div>

          {/* Status timeline */}
          <div>
            <h4 className="mb-2 text-sm font-semibold">
              {t('Status Timeline')}
            </h4>
            {history.length > 0 ? (
              <StatusTimeline history={history} segmentCount={60} />
            ) : (
              <div className="border-border text-muted-foreground flex h-12 items-center justify-center rounded-md border border-dashed text-xs">
                {t('No history data')}
              </div>
            )}
          </div>

          {/* Trend chart */}
          {history.length > 0 && (
            <div>
              <h4 className="mb-2 text-sm font-semibold">{t('Trend')}</h4>
              <div className="border-border bg-card rounded-xl border p-3">
                <AvailabilityCacheChart
                  history={history}
                  intervalMinutes={intervalMinutes}
                />
              </div>
            </div>
          )}

          {/* Channel table (admin only) */}
          {admin && (
            <div>
              <h4 className="mb-2 text-sm font-semibold">
                {t('Channel Details')}
              </h4>
              {loading ? (
                <div className="space-y-2">
                  <Skeleton className="h-8 w-full" />
                  <Skeleton className="h-8 w-full" />
                  <Skeleton className="h-8 w-full" />
                  <Skeleton className="h-8 w-full" />
                </div>
              ) : channelData.length === 0 ? (
                <p className="text-muted-foreground text-sm">
                  {t('No channel data')}
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-border border-b">
                        <th className="text-muted-foreground px-2 py-2 text-left text-xs font-medium">
                          ID
                        </th>
                        <th className="text-muted-foreground px-2 py-2 text-left text-xs font-medium">
                          {t('Name')}
                        </th>
                        <th className="text-muted-foreground px-2 py-2 text-left text-xs font-medium">
                          {t('Status')}
                        </th>
                        <th className="text-muted-foreground px-2 py-2 text-left text-xs font-medium">
                          {t('Availability')}
                        </th>
                        <th className="text-muted-foreground px-2 py-2 text-left text-xs font-medium">
                          {t('Cache Rate')}
                        </th>
                        <th className="text-muted-foreground px-2 py-2 text-left text-xs font-medium">
                          FRT
                        </th>
                        <th className="text-muted-foreground px-2 py-2 text-left text-xs font-medium">
                          {t('Test Model')}
                        </th>
                        <th className="text-muted-foreground px-2 py-2 text-left text-xs font-medium">
                          {t('Last Test')}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {channelData.map((ch: ChannelStat) => (
                        <tr
                          key={ch.channel_id}
                          className="border-border border-b last:border-0"
                        >
                          <td className="text-muted-foreground px-2 py-1.5 font-mono text-xs">
                            {ch.channel_id}
                          </td>
                          <td
                            className="max-w-[140px] truncate px-2 py-1.5"
                            title={ch.channel_name}
                          >
                            {ch.channel_name}
                          </td>
                          <td className="px-2 py-1.5">
                            <Badge
                              variant={channelStatusVariant(ch.enabled)}
                            >
                              {ch.enabled ? t('Enabled') : t('Disabled')}
                            </Badge>
                          </td>
                          <td className="px-2 py-1.5">
                            <Badge
                              variant={rateVariant(ch.availability_rate)}
                            >
                              {ch.availability_rate != null
                                ? `${ch.availability_rate.toFixed(1)}%`
                                : '-'}
                            </Badge>
                          </td>
                          <td className="px-2 py-1.5">
                            <Badge
                              variant={
                                ch.cache_hit_rate != null &&
                                ch.cache_hit_rate >= 3
                                  ? rateVariant(ch.cache_hit_rate)
                                  : 'outline'
                              }
                            >
                              {ch.cache_hit_rate != null &&
                              ch.cache_hit_rate >= 3
                                ? `${ch.cache_hit_rate.toFixed(1)}%`
                                : '—'}
                            </Badge>
                          </td>
                          <td className="text-muted-foreground px-2 py-1.5 font-mono text-xs">
                            {formatFRT(ch.first_response_time)}
                          </td>
                          <td
                            className="max-w-[140px] truncate px-2 py-1.5 text-xs"
                            title={ch.test_model}
                          >
                            {ch.test_model || '-'}
                          </td>
                          <td className="text-muted-foreground px-2 py-1.5 text-xs">
                            {ch.last_test_time
                              ? new Date(ch.last_test_time).toLocaleString(
                                  [],
                                  {
                                    month: '2-digit',
                                    day: '2-digit',
                                    hour: '2-digit',
                                    minute: '2-digit',
                                    hour12: false,
                                  }
                                )
                              : '-'}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
