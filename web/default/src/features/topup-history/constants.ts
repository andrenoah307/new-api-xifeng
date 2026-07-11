import type { StatusVariant } from '@/components/status-badge'

export const DEFAULT_PAGE_SIZE = 20

export const TOPUP_STATUS_OPTIONS = [
  { label: 'All', value: '__all__' },
  { label: 'Success', value: 'success' },
  { label: 'Pending', value: 'pending' },
  { label: 'Failed', value: 'failed' },
  { label: 'Expired', value: 'expired' },
] as const

export const STATUS_CONFIG: Record<
  string,
  { labelKey: string; variant: StatusVariant }
> = {
  success: { labelKey: 'Success', variant: 'success' },
  pending: { labelKey: 'Pending', variant: 'warning' },
  failed: { labelKey: 'Failed', variant: 'danger' },
  expired: { labelKey: 'Expired', variant: 'danger' },
}

// 与后端 model.InvoiceStatus* 对齐；'Pending Issuance'/'Issued' 已有译文
export const INVOICE_STATUS_CONFIG: Record<
  number,
  { labelKey: string; variant: StatusVariant }
> = {
  0: { labelKey: 'Not Invoiced', variant: 'neutral' },
  1: { labelKey: 'Pending Issuance', variant: 'warning' },
  2: { labelKey: 'Issued', variant: 'success' },
}

export const PAYMENT_METHOD_MAP: Record<string, string> = {
  stripe: 'Stripe',
  creem: 'Creem',
  waffo: 'Waffo',
  alipay: 'Alipay',
  wxpay: 'WeChat Pay',
}

export function isSubscriptionTopup(record: {
  amount: number
  trade_no: string
}): boolean {
  return (
    Number(record.amount || 0) === 0 &&
    (record.trade_no || '').toLowerCase().startsWith('sub')
  )
}

export const topupQueryKeys = {
  all: ['topup-history'] as const,
  lists: () => [...topupQueryKeys.all, 'list'] as const,
  list: (params: Record<string, unknown>) =>
    [...topupQueryKeys.lists(), params] as const,
}
