import type { StatusVariant } from '@/components/status-badge'

// ============================================================================
// Ticket Status
// ============================================================================

export const TICKET_STATUS = {
  PENDING: 1,
  IN_PROGRESS: 2,
  RESOLVED: 3,
  CLOSED: 4,
} as const

export const TICKET_STATUS_CONFIG: Record<
  number,
  { labelKey: string; variant: StatusVariant }
> = {
  1: { labelKey: 'Pending', variant: 'info' },
  2: { labelKey: 'Processing', variant: 'warning' },
  3: { labelKey: 'Resolved', variant: 'success' },
  4: { labelKey: 'Closed', variant: 'neutral' },
}

export function getStatusOptions(allowClosed = false) {
  const options = [
    { value: '1', label: 'Pending' },
    { value: '2', label: 'Processing' },
    { value: '3', label: 'Resolved' },
  ]
  if (allowClosed) options.push({ value: '4', label: 'Closed' })
  return options
}

// ============================================================================
// Ticket Type
// ============================================================================

export const TICKET_TYPE_CONFIG: Record<string, { labelKey: string }> = {
  general: { labelKey: 'General Ticket' },
  refund: { labelKey: 'Refund Ticket' },
  invoice: { labelKey: 'Invoice Ticket' },
}

export function getTypeOptions(includeInvoice = false) {
  const options = [
    { value: 'general', label: 'General Ticket' },
    { value: 'refund', label: 'Refund Ticket' },
  ]
  if (includeInvoice) options.push({ value: 'invoice', label: 'Invoice Ticket' })
  return options
}

// ============================================================================
// Priority
// ============================================================================

export const TICKET_PRIORITY_CONFIG: Record<
  number,
  { labelKey: string; variant: StatusVariant }
> = {
  1: { labelKey: 'Priority High', variant: 'danger' },
  2: { labelKey: 'Priority Medium', variant: 'info' },
  3: { labelKey: 'Priority Low', variant: 'neutral' },
}

export function getPriorityOptions() {
  return [
    { value: '1', label: 'Priority High' },
    { value: '2', label: 'Priority Medium' },
    { value: '3', label: 'Priority Low' },
  ]
}

// ============================================================================
// Invoice Status
// ============================================================================

export const INVOICE_STATUS_CONFIG: Record<
  number,
  { labelKey: string; variant: StatusVariant }
> = {
  1: { labelKey: 'Pending', variant: 'warning' },
  2: { labelKey: 'Issued', variant: 'success' },
  3: { labelKey: 'Rejected', variant: 'danger' },
}

// ============================================================================
// Refund Status
// ============================================================================

export const REFUND_STATUS = {
  PENDING: 1,
  REFUNDED: 2,
  REJECTED: 3,
} as const

export const REFUND_STATUS_CONFIG: Record<
  number,
  { labelKey: string; variant: StatusVariant }
> = {
  1: { labelKey: 'Pending Review', variant: 'warning' },
  2: { labelKey: 'Refunded', variant: 'success' },
  3: { labelKey: 'Rejected', variant: 'danger' },
}

export const PAYEE_TYPE_OPTIONS = [
  { value: 'alipay', label: 'Alipay' },
  { value: 'wechat', label: 'WeChat' },
  { value: 'bank', label: 'Bank Card' },
  { value: 'other', label: 'Other' },
]

// ============================================================================
// Helpers
// ============================================================================

export const DEFAULT_PAGE_SIZE = 10

export function canReply(status: number) {
  return status !== TICKET_STATUS.CLOSED
}

export function canClose(status: number) {
  return status !== TICKET_STATUS.CLOSED
}

export function roleBadgeVariant(role: number): StatusVariant {
  if (role >= 100) return 'danger'
  if (role >= 10) return 'orange'
  if (role >= 5) return 'cyan'
  return 'info'
}

export function roleBadgeLabel(role: number): string {
  if (role >= 100) return 'Super Admin'
  if (role >= 10) return 'Admin'
  if (role >= 5) return 'Staff'
  return 'User'
}

export function humanFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
