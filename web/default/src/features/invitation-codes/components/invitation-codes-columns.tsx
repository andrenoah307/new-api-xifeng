import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { ColumnDef } from '@tanstack/react-table'
import { Checkbox } from '@/components/ui/checkbox'
import { StatusBadge } from '@/components/status-badge'
import { formatTimestampToDate } from '@/lib/format'
import type { InvitationCode } from '../api'
import { getStatusKey, STATUS_CONFIG } from '../constants'
import { DataTableRowActions } from './data-table-row-actions'

export function useInvitationCodesColumns(): ColumnDef<InvitationCode>[] {
  const { t } = useTranslation()

  return useMemo(
    (): ColumnDef<InvitationCode>[] => [
      {
        id: 'select',
        header: ({ table }) => (
          <Checkbox
            checked={
              table.getIsAllPageRowsSelected() ||
              (table.getIsSomePageRowsSelected() && 'indeterminate')
            }
            onCheckedChange={(v) => table.toggleAllPageRowsSelected(!!v)}
            aria-label={t('Select all')}
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(v) => row.toggleSelected(!!v)}
            aria-label={t('Select row')}
          />
        ),
        size: 32,
        enableSorting: false,
        enableHiding: false,
      },
      {
        accessorKey: 'id',
        header: 'ID',
        size: 60,
        meta: { label: 'ID', mobileHidden: true },
      },
      {
        accessorKey: 'name',
        header: t('Name'),
        cell: ({ row }) => (
          <span className="max-w-[160px] truncate">
            {row.original.name || '-'}
          </span>
        ),
        meta: { label: t('Name'), mobileTitle: true },
      },
      {
        accessorKey: 'code',
        header: t('Code'),
        cell: ({ row }) => (
          <code className="bg-muted rounded px-1.5 py-0.5 text-xs">
            {row.original.code}
          </code>
        ),
        meta: { label: t('Code') },
      },
      {
        accessorKey: 'status',
        header: t('Status'),
        cell: ({ row }) => {
          const key = getStatusKey(row.original)
          const cfg = STATUS_CONFIG[key]
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
        accessorKey: 'used_count',
        header: t('Usage'),
        cell: ({ row }) => {
          const { used_count, max_uses } = row.original
          const limit = max_uses === 0 ? t('Unlimited') : String(max_uses)
          return (
            <span className="font-mono text-xs">
              {used_count}/{limit}
            </span>
          )
        },
        meta: { label: t('Usage') },
      },
      {
        accessorKey: 'owner_user_id',
        header: t('Owner ID'),
        cell: ({ row }) => (
          <span className="font-mono text-xs">
            {row.original.owner_user_id || '-'}
          </span>
        ),
        meta: { label: t('Owner ID'), mobileHidden: true },
      },
      {
        accessorKey: 'is_admin',
        header: t('Source'),
        cell: ({ row }) => (
          <StatusBadge
            label={row.original.is_admin ? t('Admin') : t('User')}
            variant={row.original.is_admin ? 'blue' : 'cyan'}
            copyable={false}
          />
        ),
        meta: { label: t('Source'), mobileHidden: true },
      },
      {
        accessorKey: 'created_time',
        header: t('Created'),
        cell: ({ row }) => (
          <span className="text-muted-foreground text-xs">
            {formatTimestampToDate(row.original.created_time)}
          </span>
        ),
        meta: { label: t('Created'), mobileHidden: true },
      },
      {
        accessorKey: 'expired_time',
        header: t('Expires'),
        cell: ({ row }) => {
          const ts = row.original.expired_time
          return (
            <span className="text-muted-foreground text-xs">
              {ts === 0 ? t('Never') : formatTimestampToDate(ts)}
            </span>
          )
        },
        meta: { label: t('Expires'), mobileHidden: true },
      },
      {
        id: 'actions',
        cell: ({ row }) => <DataTableRowActions row={row} />,
        size: 48,
      },
    ],
    [t]
  )
}
