import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { ColumnDef } from '@tanstack/react-table'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/status-badge'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { formatTimestampToDate } from '@/lib/format'
import type { TopupRecord } from '../api'
import {
  STATUS_CONFIG,
  PAYMENT_METHOD_MAP,
  isSubscriptionTopup,
} from '../constants'

export function useTopupColumns(admin: boolean): ColumnDef<TopupRecord>[] {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()

  return useMemo((): ColumnDef<TopupRecord>[] => {
    const cols: ColumnDef<TopupRecord>[] = []

    if (admin) {
      cols.push({
        accessorKey: 'user_id',
        header: t('User'),
        cell: ({ row }) => {
          const { username, user_id } = row.original
          return (
            <span className="font-mono text-xs">
              {username ? `${username} (${user_id})` : String(user_id ?? '-')}
            </span>
          )
        },
        size: 140,
        meta: { label: t('User'), mobileHidden: true },
      })
    }

    cols.push(
      {
        accessorKey: 'trade_no',
        header: t('Order Number'),
        cell: ({ row }) => (
          <button
            type="button"
            className="text-foreground cursor-pointer truncate font-mono text-xs hover:underline"
            onClick={() => copyToClipboard(row.original.trade_no)}
            title={row.original.trade_no}
          >
            {row.original.trade_no}
          </button>
        ),
        meta: { label: t('Order Number'), mobileTitle: true },
      },
      {
        accessorKey: 'payment_method',
        header: t('Payment Method'),
        cell: ({ row }) => {
          const pm = row.original.payment_method
          const label = PAYMENT_METHOD_MAP[pm]
          return (
            <span className="text-xs">
              {label ? t(label) : pm || '-'}
            </span>
          )
        },
        meta: { label: t('Payment Method') },
      },
      {
        accessorKey: 'amount',
        header: t('Top-up Amount'),
        cell: ({ row }) => {
          if (isSubscriptionTopup(row.original)) {
            return (
              <Badge variant="secondary">{t('Subscription')}</Badge>
            )
          }
          return (
            <span className="font-mono text-xs">{row.original.amount}</span>
          )
        },
        meta: { label: t('Top-up Amount') },
      },
      {
        accessorKey: 'money',
        header: t('Payment Amount'),
        cell: ({ row }) => (
          <span className="font-mono text-xs text-red-600 dark:text-red-400">
            ¥{row.original.money.toFixed(2)}
          </span>
        ),
        meta: { label: t('Payment Amount') },
      },
      {
        accessorKey: 'status',
        header: t('Status'),
        cell: ({ row }) => {
          const s = row.original.status
          const cfg = STATUS_CONFIG[s]
          if (!cfg) return <span className="text-xs">{s}</span>
          return (
            <StatusBadge
              label={t(cfg.labelKey)}
              variant={cfg.variant}
              copyable={false}
            />
          )
        },
        meta: { label: t('Status'), mobileBadge: true },
      },
      {
        accessorKey: 'create_time',
        header: t('Created'),
        cell: ({ row }) => (
          <span className="text-muted-foreground text-xs">
            {formatTimestampToDate(row.original.create_time)}
          </span>
        ),
        meta: { label: t('Created'), mobileHidden: true },
      }
    )

    return cols
  }, [t, admin, copyToClipboard])
}
