import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { ColumnDef } from '@tanstack/react-table'
import { formatTimestampToDate } from '@/lib/format'
import { formatQuota } from '@/lib/format'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import type { Ticket, StaffUser } from '../api'
import {
  roleBadgeVariant,
  roleBadgeLabel,
  TICKET_STATUS,
} from '../constants'
import {
  TicketStatusBadge,
  TicketPriorityBadge,
  TicketTypeBadge,
} from './ticket-status-badge'

function TicketAmountBadge({ ticket }: { ticket: Ticket }) {
  if (ticket.type === 'refund' && ticket.refund_quota != null) {
    return (
      <span className="text-muted-foreground ml-1.5 text-[11px]">
        {formatQuota(ticket.refund_quota)}
      </span>
    )
  }
  if (ticket.type === 'invoice' && ticket.invoice_money != null) {
    return (
      <span className="text-muted-foreground ml-1.5 text-[11px]">
        ¥{ticket.invoice_money.toFixed(2)}
      </span>
    )
  }
  return null
}

export function useTicketColumns(opts: {
  admin: boolean
  showAssignee: boolean
  staffMap: Map<number, StaffUser>
  onCloseTicket?: (id: number) => void
}): ColumnDef<Ticket>[] {
  const { t } = useTranslation()
  const { admin, showAssignee, staffMap, onCloseTicket } = opts

  return useMemo((): ColumnDef<Ticket>[] => {
    const cols: ColumnDef<Ticket>[] = [
      {
        accessorKey: 'id',
        header: 'ID',
        size: 60,
        cell: ({ row }) => (
          <span className="text-muted-foreground font-mono text-xs">
            #{row.original.id}
          </span>
        ),
        meta: { label: 'ID', mobileHidden: true },
      },
      {
        accessorKey: 'subject',
        header: t('Subject'),
        cell: ({ row }) => (
          <div className="min-w-0">
            <div className="truncate text-sm font-medium">
              {row.original.subject}
              {row.original.type === 'invoice' && row.original.company_name && (
                <span className="text-muted-foreground ml-1.5 text-xs font-normal">
                  ({row.original.company_name})
                </span>
              )}
            </div>
            <div className="flex items-center">
              <TicketTypeBadge type={row.original.type} />
              <TicketAmountBadge ticket={row.original} />
            </div>
          </div>
        ),
        meta: { label: t('Subject'), mobileTitle: true },
      },
      {
        accessorKey: 'status',
        header: t('Status'),
        size: 100,
        cell: ({ row }) => (
          <TicketStatusBadge status={row.original.status} />
        ),
        meta: { label: t('Status'), mobileBadge: true },
      },
      {
        accessorKey: 'priority',
        header: t('Priority'),
        size: 100,
        cell: ({ row }) => (
          <TicketPriorityBadge priority={row.original.priority} />
        ),
        meta: { label: t('Priority'), mobileHidden: true },
      },
    ]

    if (admin) {
      cols.push({
        accessorKey: 'username',
        header: t('User'),
        cell: ({ row }) => (
          <div className="text-xs">
            <span>{row.original.username}</span>
            <span className="text-muted-foreground ml-1">
              #{row.original.user_id}
            </span>
          </div>
        ),
        meta: { label: t('User'), mobileHidden: true },
      })
    }

    if (admin && showAssignee) {
      cols.push({
        accessorKey: 'assignee_id',
        header: t('Assignee'),
        cell: ({ row }) => {
          const aid = row.original.assignee_id
          if (!aid) {
            return (
              <span className="text-muted-foreground text-xs italic">
                {t('Unassigned')}
              </span>
            )
          }
          const staff = staffMap.get(aid)
          if (!staff) {
            return <span className="text-xs">#{aid}</span>
          }
          return (
            <div className="flex items-center gap-1.5 text-xs">
              <span>{staff.display_name || staff.username}</span>
              <StatusBadge
                label={t(roleBadgeLabel(staff.role))}
                variant={roleBadgeVariant(staff.role)}
                size="sm"
                copyable={false}
              />
            </div>
          )
        },
        meta: { label: t('Assignee'), mobileHidden: true },
      })
    }

    cols.push({
      accessorKey: 'updated_time',
      header: t('Updated'),
      size: 160,
      cell: ({ row }) => (
        <span className="text-muted-foreground text-xs">
          {formatTimestampToDate(row.original.updated_time)}
        </span>
      ),
      meta: { label: t('Updated'), mobileHidden: true },
    })

    if (!admin && onCloseTicket) {
      cols.push({
        id: 'actions',
        header: t('Actions'),
        size: 100,
        cell: ({ row }) => {
          if (row.original.status === TICKET_STATUS.CLOSED) return null
          return (
            <Button
              variant="ghost"
              size="sm"
              className="text-destructive"
              onClick={(e) => {
                e.stopPropagation()
                onCloseTicket(row.original.id)
              }}
            >
              {t('Close')}
            </Button>
          )
        },
        meta: { label: t('Actions') },
      })
    }

    return cols
  }, [t, admin, showAssignee, staffMap, onCloseTicket])
}
