import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { formatTimestampToDate } from '@/lib/format'
import { DataTableColumnHeader } from '@/components/data-table'
import { MaskedValueDisplay } from '@/components/masked-value-display'
import { StatusBadge } from '@/components/status-badge'
import { DISCOUNT_CODE_STATUSES } from '../constants'
import { type DiscountCode } from '../types'
import { DataTableRowActions } from './data-table-row-actions'

export function useDiscountCodesColumns(): ColumnDef<DiscountCode>[] {
  const { t } = useTranslation()
  return [
    {
      accessorKey: 'id',
      meta: { label: t('ID'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('ID')} />
      ),
      cell: ({ row }) => (
        <div className='w-[60px]'>{row.getValue('id')}</div>
      ),
    },
    {
      id: 'code',
      accessorKey: 'code',
      meta: { label: t('Code') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Code')} />
      ),
      cell: function CodeCell({ row }) {
        const code = row.original.code
        const maskedCode =
          code.length > 16
            ? `${code.slice(0, 8)}${'*'.repeat(16)}${code.slice(-8)}`
            : code
        return (
          <MaskedValueDisplay
            label={t('Full Code')}
            fullValue={code}
            maskedValue={maskedCode}
            copyTooltip={t('Copy code')}
            copyAriaLabel={t('Copy discount code')}
          />
        )
      },
      enableSorting: false,
    },
    {
      accessorKey: 'name',
      meta: { label: t('Name'), mobileTitle: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Name')} />
      ),
      cell: ({ row }) => (
        <div className='max-w-[150px] truncate font-medium'>
          {row.getValue('name') || '-'}
        </div>
      ),
    },
    {
      accessorKey: 'discount_rate',
      meta: { label: t('Discount Rate') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Discount Rate')} />
      ),
      cell: ({ row }) => {
        const rate = row.getValue('discount_rate') as number
        const off = 100 - rate
        return (
          <StatusBadge
            label={`${off}% off (${t('pay')} ${rate}%)`}
            variant='neutral'
            copyable={false}
          />
        )
      },
    },
    {
      accessorKey: 'start_time',
      meta: { label: t('Start Time'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Start Time')} />
      ),
      cell: ({ row }) => {
        const time = row.getValue('start_time') as number
        if (time === 0) {
          return (
            <StatusBadge
              label={t('Unlimited')}
              variant='neutral'
              copyable={false}
            />
          )
        }
        return (
          <div className='min-w-[140px] font-mono text-sm'>
            {formatTimestampToDate(time)}
          </div>
        )
      },
    },
    {
      accessorKey: 'end_time',
      meta: { label: t('End Time'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('End Time')} />
      ),
      cell: ({ row }) => {
        const time = row.getValue('end_time') as number
        if (time === 0) {
          return (
            <StatusBadge
              label={t('Unlimited')}
              variant='neutral'
              copyable={false}
            />
          )
        }
        const isExpired = time < Date.now() / 1000
        return (
          <div
            className={`min-w-[140px] font-mono text-sm ${isExpired ? 'text-destructive' : ''}`}
          >
            {formatTimestampToDate(time)}
          </div>
        )
      },
    },
    {
      accessorKey: 'max_uses_per_user',
      meta: { label: t('Max Uses Per User'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('Max Uses Per User')}
        />
      ),
      cell: ({ row }) => {
        const val = row.getValue('max_uses_per_user') as number
        return val === 0 ? t('Unlimited') : String(val)
      },
    },
    {
      accessorKey: 'max_uses_total',
      meta: { label: t('Max Total Uses'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Max Total Uses')} />
      ),
      cell: ({ row }) => {
        const val = row.getValue('max_uses_total') as number
        return val === 0 ? t('Unlimited') : String(val)
      },
    },
    {
      accessorKey: 'used_count',
      meta: { label: t('Used Count'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Used Count')} />
      ),
      cell: ({ row }) => (
        <div>{row.getValue('used_count')}</div>
      ),
    },
    {
      accessorKey: 'status',
      meta: { label: t('Status'), mobileBadge: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Status')} />
      ),
      cell: ({ row }) => {
        const statusValue = row.getValue('status') as number
        const statusConfig = DISCOUNT_CODE_STATUSES[statusValue]
        if (!statusConfig) return null
        return (
          <StatusBadge
            label={t(statusConfig.labelKey)}
            variant={statusConfig.variant}
            showDot={statusConfig.showDot}
            copyable={false}
          />
        )
      },
      filterFn: (row, id, value) => {
        const statusValue = row.getValue(id) as number
        return value.includes(String(statusValue))
      },
    },
    {
      id: 'actions',
      cell: ({ row }) => <DataTableRowActions row={row} />,
    },
  ]
}
