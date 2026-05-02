import {
  useCallback,
  useMemo,
  useRef,
  useState,
  useEffect,
  memo,
} from 'react'
import { useTranslation } from 'react-i18next'
import {
  useQuery,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { Activity, RefreshCw, Search, X } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { useIsAdmin } from '@/hooks/use-admin'
import type { MonitoringGroupWithHistory } from '../api'
import {
  getMonitoringGroups,
  getGroupHistory,
  refreshMonitoringData,
} from '../api'
import type { SortMode } from '../constants'
import {
  avgAvailability,
  compareGroups,
  isGroupOnline,
  loadSortMode,
  saveSortMode,
  rateAccentColor,
} from '../constants'
import GroupStatusCard from './group-status-card'
import GroupDetailPanel from './group-detail-panel'

const POLL_INTERVAL_MS = 60_000

// Isolated countdown component to avoid re-rendering the entire tree every second
const RefreshButton = memo(function RefreshButton({
  admin,
  refreshing,
  onRefresh,
  lastUpdated,
}: {
  admin: boolean
  refreshing: boolean
  onRefresh: () => void
  lastUpdated: number | null
}) {
  const { t } = useTranslation()
  const [countdown, setCountdown] = useState(POLL_INTERVAL_MS)

  useEffect(() => {
    setCountdown(POLL_INTERVAL_MS)
  }, [lastUpdated])

  useEffect(() => {
    const id = setInterval(() => {
      setCountdown((c) => Math.max(0, c - 1000))
    }, 1000)
    return () => clearInterval(id)
  }, [])

  const label = `${Math.floor(countdown / 60000)}:${String(
    Math.floor((countdown % 60000) / 1000)
  ).padStart(2, '0')}`

  return (
    <Button
      variant="ghost"
      size="sm"
      disabled={!admin || refreshing}
      onClick={admin ? onRefresh : undefined}
      title={
        admin
          ? t('Refresh now') + ' · ' + t('Next auto refresh {{c}}', { c: label })
          : t('Next auto refresh {{c}}', { c: label })
      }
    >
      <RefreshCw
        size={14}
        className={refreshing ? 'animate-spin' : undefined}
      />
      <span className="text-muted-foreground ml-1 font-mono text-[11px]">
        {label}
      </span>
    </Button>
  )
})

function EmptyState({
  icon,
  title,
  desc,
}: {
  icon: React.ReactNode
  title: string
  desc?: string
}) {
  return (
    <div className="border-border flex flex-col items-center justify-center rounded-2xl border border-dashed py-24 text-center">
      <div className="text-muted-foreground mb-4">{icon}</div>
      <p className="text-foreground text-base font-semibold">{title}</p>
      {desc && (
        <p className="text-muted-foreground mt-1.5 text-sm">{desc}</p>
      )}
    </div>
  )
}

export default function MonitoringDashboard() {
  const { t } = useTranslation()
  const admin = useIsAdmin()
  const queryClient = useQueryClient()

  const [keyword, setKeyword] = useState('')
  const [sortMode, setSortMode] = useState<SortMode>(loadSortMode)
  const [selectedGroup, setSelectedGroup] =
    useState<MonitoringGroupWithHistory | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [lastUpdated, setLastUpdated] = useState<number | null>(null)

  // Cache history data across poll cycles - only refresh on manual action
  const historyCache = useRef<
    Record<
      string,
      { history: MonitoringGroupWithHistory['history']; intervalMinutes: number }
    >
  >({})

  // 1. Fetch groups list with auto-refresh
  const {
    data: rawGroups,
    isLoading: groupsLoading,
  } = useQuery({
    queryKey: ['monitoring', 'groups', admin],
    queryFn: () => getMonitoringGroups(admin),
    refetchInterval: POLL_INTERVAL_MS,
    staleTime: POLL_INTERVAL_MS - 5_000,
  })

  // 2. Once we have groups, fetch history for each in parallel (only on first load)
  const groupNames = useMemo(
    () => (rawGroups ?? []).map((g) => g.group_name).sort(),
    [rawGroups]
  )
  const groupNamesKey = groupNames.join(',')

  const {
    data: historyMap,
    isLoading: historyLoading,
  } = useQuery({
    queryKey: ['monitoring', 'allHistory', groupNamesKey, admin],
    queryFn: async () => {
      const results = await Promise.all(
        groupNames.map(async (name) => {
          try {
            const data = await getGroupHistory(name, admin)
            return [name, data] as const
          } catch {
            return [name, { history: [], intervalMinutes: 5 }] as const
          }
        })
      )
      const map: Record<
        string,
        { history: MonitoringGroupWithHistory['history']; intervalMinutes: number }
      > = {}
      for (const [name, data] of results) {
        map[name] = data
      }
      // Update the ref cache
      historyCache.current = { ...historyCache.current, ...map }
      return map
    },
    enabled: groupNames.length > 0,
    staleTime: Infinity, // History only refreshes on manual action
  })

  // 3. Merge groups + history
  const groups: MonitoringGroupWithHistory[] = useMemo(() => {
    if (!rawGroups) return []
    const cache = historyMap ?? historyCache.current
    return rawGroups.map((g) => {
      const h = cache[g.group_name]
      return {
        ...g,
        history: h?.history ?? [],
        aggregation_interval_minutes: h?.intervalMinutes ?? 5,
      }
    })
  }, [rawGroups, historyMap])

  // Update lastUpdated when groups change
  useEffect(() => {
    if (rawGroups && rawGroups.length > 0) {
      setLastUpdated(Date.now())
    }
  }, [rawGroups])

  // Manual refresh mutation
  const refreshMutation = useMutation({
    mutationFn: async () => {
      if (admin) {
        await refreshMonitoringData()
      }
      // Re-fetch everything including history
      await queryClient.invalidateQueries({
        queryKey: ['monitoring', 'groups'],
      })
      await queryClient.invalidateQueries({
        queryKey: ['monitoring', 'allHistory'],
      })
    },
    onSuccess: () => {
      if (admin) toast.success(t('Refresh successful'))
      setLastUpdated(Date.now())
    },
    onError: () => {
      toast.error(t('Refresh failed'))
    },
  })

  const handleRefresh = useCallback(() => {
    refreshMutation.mutate()
  }, [refreshMutation])

  const handleSortChange = useCallback((value: string) => {
    const mode = value as SortMode
    setSortMode(mode)
    saveSortMode(mode)
  }, [])

  const handleCardClick = useCallback(
    (group: MonitoringGroupWithHistory) => {
      if (!admin) return
      setSelectedGroup(group)
      setDetailOpen(true)
    },
    [admin]
  )

  const handleDetailClose = useCallback((open: boolean) => {
    setDetailOpen(open)
    if (!open) setSelectedGroup(null)
  }, [])

  // Filter and sort
  const visible = useMemo(() => {
    const kw = keyword.trim().toLowerCase()
    const filtered = kw
      ? groups.filter((g) =>
          (g.group_name || '').toLowerCase().includes(kw)
        )
      : groups
    return [...filtered].sort((a, b) => compareGroups(a, b, sortMode))
  }, [groups, keyword, sortMode])

  const onlineCount = groups.filter(isGroupOnline).length
  const offlineCount = groups.length - onlineCount
  const avgAvail = avgAvailability(groups)

  const loading = groupsLoading && groups.length === 0

  return (
    <div className="mt-[60px] px-4 pb-12 sm:px-8 lg:px-10">
      <div className="mx-auto w-full max-w-[1440px]">
        {/* Title bar */}
        <div className="flex flex-wrap items-center justify-between gap-4 py-6 sm:py-8">
          <div className="flex flex-wrap items-baseline gap-x-4 gap-y-2">
            <h1 className="text-foreground m-0 text-xl font-semibold tracking-tight">
              {t('Group Monitoring')}
            </h1>
            {!loading && groups.length > 0 && (
              <div className="text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
                <span className="inline-flex items-center gap-1.5">
                  <span className="bg-success inline-block h-1.5 w-1.5 rounded-full" style={{ background: 'hsl(var(--success, 142 76% 36%))' }} />
                  <span className="text-foreground font-mono">
                    {onlineCount}
                  </span>
                  <span>{t('Online')}</span>
                </span>
                <span className="inline-flex items-center gap-1.5">
                  <span
                    className="inline-block h-1.5 w-1.5 rounded-full"
                    style={{
                      background:
                        offlineCount > 0
                          ? 'hsl(var(--destructive))'
                          : 'hsl(var(--muted-foreground) / 0.3)',
                    }}
                  />
                  <span className="text-foreground font-mono">
                    {offlineCount}
                  </span>
                  <span>{t('Offline')}</span>
                </span>
                {avgAvail != null && (
                  <span className="inline-flex items-baseline gap-1.5">
                    <span>{t('Average Availability')}</span>
                    <span
                      className="font-mono"
                      style={{ color: rateAccentColor(avgAvail) }}
                    >
                      {avgAvail.toFixed(1)}%
                    </span>
                  </span>
                )}
              </div>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <div className="relative">
              <Search
                size={14}
                className="text-muted-foreground absolute left-2.5 top-1/2 -translate-y-1/2"
              />
              <Input
                className="h-9 w-[200px] pl-8 pr-8"
                placeholder={t('Search groups')}
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
              />
              {keyword && (
                <button
                  className="text-muted-foreground hover:text-foreground absolute right-2 top-1/2 -translate-y-1/2"
                  onClick={() => setKeyword('')}
                  type="button"
                >
                  <X size={14} />
                </button>
              )}
            </div>
            <Select value={sortMode} onValueChange={handleSortChange}>
              <SelectTrigger className="h-9 w-[130px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="status">
                  {t('Sort by status')}
                </SelectItem>
                <SelectItem value="name">
                  {t('Sort by name')}
                </SelectItem>
                <SelectItem value="availability">
                  {t('Sort by availability')}
                </SelectItem>
              </SelectContent>
            </Select>
            <RefreshButton
              admin={admin}
              refreshing={refreshMutation.isPending}
              onRefresh={handleRefresh}
              lastUpdated={lastUpdated}
            />
          </div>
        </div>

        {/* Card grid */}
        {loading ? (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 sm:gap-5 lg:grid-cols-3 2xl:grid-cols-4">
            {[1, 2, 3, 4, 5, 6].map((i) => (
              <div
                key={i}
                className="border-border bg-card rounded-2xl border p-5"
              >
                <Skeleton className="mb-4 h-5 w-1/2" />
                <Skeleton className="mb-2 h-4 w-full" />
                <Skeleton className="mb-2 h-4 w-3/4" />
                <Skeleton className="mt-4 h-6 w-full" />
              </div>
            ))}
          </div>
        ) : groups.length === 0 ? (
          <EmptyState
            icon={<Activity size={32} className="opacity-40" />}
            title={t('No monitoring groups')}
            desc={t('Configure in System Settings - Group Monitoring')}
          />
        ) : visible.length === 0 ? (
          <EmptyState
            icon={<X size={32} className="opacity-40" />}
            title={t('No groups matching "{{kw}}"', { kw: keyword })}
          />
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 sm:gap-5 lg:grid-cols-3 2xl:grid-cols-4">
            {visible.map((g) => (
              <GroupStatusCard
                key={g.group_name}
                group={g}
                onClick={admin ? handleCardClick : undefined}
              />
            ))}
          </div>
        )}

        <GroupDetailPanel
          open={detailOpen}
          group={selectedGroup}
          onOpenChange={handleDetailClose}
        />
      </div>
    </div>
  )
}
