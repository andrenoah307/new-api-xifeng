import type { InvitationCode } from './api'

export const INVITATION_CODE_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
} as const

export const DEFAULT_PAGE_SIZE = 20

export function isExpired(record: InvitationCode): boolean {
  return (
    record.expired_time !== 0 &&
    record.expired_time < Math.floor(Date.now() / 1000)
  )
}

export function isExhausted(record: InvitationCode): boolean {
  return record.max_uses > 0 && record.used_count >= record.max_uses
}

export type InvitationStatusKey = 'disabled' | 'expired' | 'exhausted' | 'active'

export function getStatusKey(record: InvitationCode): InvitationStatusKey {
  if (record.status === INVITATION_CODE_STATUS.DISABLED) return 'disabled'
  if (isExpired(record)) return 'expired'
  if (isExhausted(record)) return 'exhausted'
  return 'active'
}

export const STATUS_CONFIG: Record<
  InvitationStatusKey,
  { labelKey: string; variant: 'danger' | 'warning' | 'violet' | 'success' }
> = {
  disabled: { labelKey: 'Disabled', variant: 'danger' },
  expired: { labelKey: 'Expired', variant: 'warning' },
  exhausted: { labelKey: 'Exhausted', variant: 'violet' },
  active: { labelKey: 'Active', variant: 'success' },
}
