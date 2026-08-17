import type { RateLimitCapacityItem } from '@/features/dashboard/types'

export const PERSONAL_RPM_STALE_TIME = 15_000

export type PersonalRPMItem = RateLimitCapacityItem

export type PersonalRPMDisplayState = 'available' | 'empty' | 'unavailable'

export function normalizePersonalRPMItems(value: unknown): PersonalRPMItem[] {
  if (!Array.isArray(value)) return []
  return value
    .filter((item): item is PersonalRPMItem => {
      if (!item || typeof item !== 'object') return false
      const metric = item as Partial<PersonalRPMItem>
      if (typeof metric.model !== 'string' || metric.model.length === 0) {
        return false
      }
      if (
        metric.current !== null &&
        (typeof metric.current !== 'number' ||
          !Number.isFinite(metric.current) ||
          metric.current < 0)
      ) {
        return false
      }
      if (
        typeof metric.limit !== 'number' ||
        !Number.isFinite(metric.limit) ||
        metric.limit < 0
      ) {
        return false
      }
      if (
        metric.utilization !== null &&
        (typeof metric.utilization !== 'number' ||
          !Number.isFinite(metric.utilization) ||
          metric.utilization < 0)
      ) {
        return false
      }
      return (
        typeof metric.available === 'boolean' &&
        typeof metric.unlimited === 'boolean' &&
        typeof metric.over_limit === 'boolean'
      )
    })
    .sort((a, b) => {
      const aCurrent =
        a.available && typeof a.current === 'number' ? a.current : null
      const bCurrent =
        b.available && typeof b.current === 'number' ? b.current : null
      if (aCurrent !== null && bCurrent !== null && aCurrent !== bCurrent) {
        return bCurrent - aCurrent
      }
      if (aCurrent !== null && bCurrent === null) return -1
      if (aCurrent === null && bCurrent !== null) return 1
      if (a.model < b.model) return -1
      if (a.model > b.model) return 1
      return 0
    })
}

export function personalRPMDisplayState(
  status: string | undefined,
  items: PersonalRPMItem[]
): PersonalRPMDisplayState {
  if (status === 'unavailable' || status === 'overflow') return 'unavailable'
  if (status === 'empty' || items.length === 0) return 'empty'
  return 'available'
}
