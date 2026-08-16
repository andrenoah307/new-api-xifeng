export const PERSONAL_RPM_REFRESH_INTERVAL = 15_000

export interface PersonalRPMItem {
  model: string
  rpm: number
}

export type PersonalRPMDisplayState = 'available' | 'empty' | 'unavailable'

export function normalizePersonalRPMItems(value: unknown): PersonalRPMItem[] {
  if (!Array.isArray(value)) return []
  return value
    .filter(
      (item): item is PersonalRPMItem =>
        Boolean(item) &&
        typeof item === 'object' &&
        typeof (item as PersonalRPMItem).model === 'string' &&
        (item as PersonalRPMItem).model.length > 0 &&
        Number.isFinite((item as PersonalRPMItem).rpm) &&
        (item as PersonalRPMItem).rpm > 0
    )
    .slice()
    .sort((a, b) => {
      if (a.rpm !== b.rpm) return b.rpm - a.rpm
      return a.model < b.model ? -1 : a.model > b.model ? 1 : 0
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
