// 后端 decimal 字段用 -1 表示"无数据"（model/group_monitoring.go default:-1）。
// 只有 null/负数才视为无数据，0~3% 的低命中率是真实数值，必须正常显示。
export function normalizeRate(
  rate: number | null | undefined
): number | null {
  if (rate == null || rate < 0) return null
  return rate
}

// 卡片与详情面板统一的口径：优先用历史数据按请求量加权，回退到汇总字段
export function resolveDisplayRate(
  history: Parameters<typeof computeRateFromHistory>[0],
  field: 'availability_rate' | 'cache_hit_rate',
  summaryRate: number | null | undefined
): number | null {
  return computeRateFromHistory(history, field) ?? normalizeRate(summaryRate)
}

export function rateAccentColor(rate: number | null | undefined): string {
  if (rate == null || rate < 0) return 'var(--muted-foreground)'
  if (rate >= 99) return '#22c55e'
  if (rate >= 95) return 'rgba(34,197,94,0.8)'
  if (rate >= 80) return '#eab308'
  return 'var(--destructive)'
}

export function rateVariant(
  rate: number | null | undefined
): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (rate == null) return 'outline'
  if (rate >= 95) return 'default'
  if (rate >= 80) return 'secondary'
  return 'destructive'
}

export function formatFRT(ms: number | null | undefined): string {
  if (ms == null || ms <= 0) return '—'
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`
  return `${Math.round(ms)}ms`
}

export function formatClock(unixSec: number | null | undefined): string {
  if (!unixSec || unixSec <= 0) return ''
  return new Date(unixSec * 1000).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

export function formatDateTime(unixSec: number | null | undefined): string {
  if (!unixSec) return '-'
  return new Date(unixSec * 1000).toLocaleString([], {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

export function isGroupOnline(group: {
  is_online?: boolean
  online_channels?: number
  total_channels?: number
}): boolean {
  if (group.is_online != null) return group.is_online
  return (group.online_channels ?? 0) > 0
}

export function avgAvailability(
  groups: { availability_rate?: number | null }[]
): number | null {
  const valid = groups
    .map((g) => g.availability_rate)
    .filter((r): r is number => r != null && r >= 0)
  if (valid.length === 0) return null
  return valid.reduce((s, v) => s + v, 0) / valid.length
}

export function computeRateFromHistory(
  history:
    | {
        request_count?: number | null
        availability_rate?: number | null
        cache_hit_rate?: number | null
      }[]
    | undefined,
  field: 'availability_rate' | 'cache_hit_rate'
): number | null {
  if (!history || history.length === 0) return null
  const valid = history.filter(
    (h) =>
      h.request_count != null &&
      h.request_count > 0 &&
      h[field] != null &&
      (h[field] as number) >= 0
  )
  if (valid.length === 0) return null
  const totalRequests = valid.reduce((s, h) => s + (h.request_count as number), 0)
  if (totalRequests === 0) return null
  const weightedSum = valid.reduce(
    (s, h) => s + (h[field] as number) * (h.request_count as number),
    0
  )
  return weightedSum / totalRequests
}

export type SortMode = 'status' | 'name' | 'availability'

export function compareGroups<
  T extends {
    is_online?: boolean
    online_channels?: number
    group_name?: string
    availability_rate?: number | null
  },
>(a: T, b: T, mode: SortMode): number {
  switch (mode) {
    case 'name':
      return (a.group_name ?? '').localeCompare(b.group_name ?? '')
    case 'availability':
      return (a.availability_rate ?? -1) - (b.availability_rate ?? -1)
    case 'status':
    default: {
      const aOn = isGroupOnline(a)
      const bOn = isGroupOnline(b)
      if (aOn !== bOn) return aOn ? 1 : -1
      // 与 availability 分支一致：无数据（null/-1 哨兵）统一按 -1 排序
      return (a.availability_rate ?? -1) - (b.availability_rate ?? -1)
    }
  }
}

const SORT_KEY = 'monitoring-sort-mode'

export function loadSortMode(): SortMode {
  try {
    const v = localStorage.getItem(SORT_KEY)
    if (v === 'name' || v === 'availability' || v === 'status') return v
  } catch {
    /* noop */
  }
  return 'status'
}

export function saveSortMode(mode: SortMode): void {
  try {
    localStorage.setItem(SORT_KEY, mode)
  } catch {
    /* noop */
  }
}

export function segmentColor(
  rate: number | null | undefined,
  avgFrt: number | null | undefined,
  requestCount?: number | null | undefined
): string {
  if (requestCount != null && requestCount <= 0)
    return 'color-mix(in oklch, var(--muted) 50%, transparent)'
  if (rate != null && rate < 0)
    return 'color-mix(in oklch, var(--muted) 50%, transparent)'
  if (avgFrt == null || avgFrt <= 0)
    return 'color-mix(in oklch, var(--muted) 50%, transparent)'
  if (avgFrt < 8000) return '#22c55e'
  return '#eab308'
}

export function segmentLabel(
  rate: number | null | undefined,
  avgFrt: number | null | undefined,
  t: (key: string) => string,
  requestCount?: number | null | undefined
): string {
  if (requestCount != null && requestCount <= 0) return t('No data available')
  if (rate != null && rate < 0) return t('No data available')
  if (avgFrt == null || avgFrt <= 0) return t('No data available')
  if (avgFrt < 8000) return t('Normal')
  return t('Slow Response')
}

export function alignAndFillHistory(
  history: { recorded_at: number; availability_rate?: number | null; cache_hit_rate?: number | null }[],
  intervalMinutes: number
): { time: string; value: number | null; type: 'availability' | 'cache' }[] {
  if (!history || history.length === 0) return []

  const sorted = [...history].sort(
    (a, b) => a.recorded_at - b.recorded_at
  )

  const startMs = sorted[0].recorded_at * 1000
  const endMs = sorted[sorted.length - 1].recorded_at * 1000
  const stepMs = (intervalMinutes || 5) * 60 * 1000

  const byTime: Record<number, (typeof sorted)[0]> = {}
  for (const h of sorted) {
    const aligned = Math.round((h.recorded_at * 1000) / stepMs) * stepMs
    byTime[aligned] = h
  }

  // 空档产出 null（图上显示为断线）而不是沿用上一个值——
  // 否则宕机时段会被画成一条正常的直线，低估故障
  const hasAvail = sorted.some(
    (h) => h.availability_rate != null && h.availability_rate >= 0
  )
  const hasCache = sorted.some(
    (h) => h.cache_hit_rate != null && h.cache_hit_rate >= 0
  )

  const result: { time: string; value: number | null; type: 'availability' | 'cache' }[] = []

  for (let t = startMs; t <= endMs; t += stepMs) {
    const aligned = Math.round(t / stepMs) * stepMs
    const entry = byTime[aligned]
    const avail = normalizeRate(entry?.availability_rate)
    const cache = normalizeRate(entry?.cache_hit_rate)
    const timeStr = new Date(aligned).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    })
    if (hasAvail) {
      result.push({ time: timeStr, value: avail, type: 'availability' })
    }
    if (hasCache) {
      result.push({ time: timeStr, value: cache, type: 'cache' })
    }
  }

  return result
}
