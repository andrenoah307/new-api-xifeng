export function rateAccentColor(rate: number | null | undefined): string {
  if (rate == null || rate < 0) return 'hsl(var(--muted-foreground))'
  if (rate >= 99) return 'hsl(var(--success, 142 76% 36%))'
  if (rate >= 95) return 'hsl(var(--success, 142 76% 36%) / 0.8)'
  if (rate >= 80) return 'hsl(var(--warning, 38 92% 50%))'
  return 'hsl(var(--destructive))'
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
}): boolean {
  return group.is_online ?? (group.online_channels ?? 0) > 0
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
      return (a.availability_rate ?? 100) - (b.availability_rate ?? 100)
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
  avgFrt: number | null | undefined
): string {
  if (rate == null || rate < 0) return 'hsl(var(--muted) / 0.5)'
  if (rate >= 99) {
    if (avgFrt != null && avgFrt > 8000) return 'hsl(var(--warning, 38 92% 50%))'
    return 'hsl(var(--success, 142 76% 36%))'
  }
  if (rate >= 95) return 'hsl(var(--success, 142 76% 36%) / 0.7)'
  if (rate >= 80) return 'hsl(var(--warning, 38 92% 50%))'
  if (rate >= 50) return '#f97316'
  return 'hsl(var(--destructive))'
}

export function segmentLabel(
  rate: number | null | undefined,
  avgFrt: number | null | undefined,
  t: (key: string) => string
): string {
  if (rate == null || rate < 0) return t('No data available')
  if (rate >= 99) {
    if (avgFrt != null && avgFrt > 8000) return t('Slow Response')
    return t('Normal')
  }
  if (rate >= 95) return t('Minor Jitter')
  if (rate >= 80) return t('Partial Anomaly')
  if (rate >= 50) return t('Severe Anomaly')
  return t('Failure')
}

export function alignAndFillHistory(
  history: { recorded_at: number; availability_rate?: number | null; cache_hit_rate?: number | null }[],
  intervalMinutes: number
): { time: string; value: number; type: 'availability' | 'cache' }[] {
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

  const result: { time: string; value: number; type: 'availability' | 'cache' }[] = []
  let lastAvail: number | null = null
  let lastCache: number | null = null

  for (let t = startMs; t <= endMs; t += stepMs) {
    const aligned = Math.round(t / stepMs) * stepMs
    const entry = byTime[aligned]
    if (entry) {
      if (entry.availability_rate != null && entry.availability_rate >= 0)
        lastAvail = entry.availability_rate
      if (entry.cache_hit_rate != null && entry.cache_hit_rate >= 0)
        lastCache = entry.cache_hit_rate
    }
    const timeStr = new Date(aligned).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    })
    if (lastAvail !== null) {
      result.push({ time: timeStr, value: lastAvail, type: 'availability' })
    }
    if (lastCache !== null) {
      result.push({ time: timeStr, value: lastCache, type: 'cache' })
    }
  }

  return result
}
