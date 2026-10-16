import { ChevronDown, ChevronRight } from 'lucide-react'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestampToDate } from '@/lib/format'

import type { FinanceRecord } from './api'

export function FinanceRecordsTable(props: {
  items: FinanceRecord[]
  invoices: boolean
  loading: boolean
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<Set<number>>(new Set())
  const statusLabels: Record<string, string> = {
    success: t('Success'),
    pending: t('Pending'),
    failed: t('Failed'),
    expired: t('Expired'),
  }
  const invoiceLabels: Record<number, string> = {
    1: t('Pending Issuance'),
    2: t('Issued'),
    3: t('Rejected'),
  }
  return (
    <div className='overflow-x-auto rounded-md border'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className='w-10' />
            <TableHead>{t('User')}</TableHead>
            <TableHead>{t('Invoice title')}</TableHead>
            <TableHead>
              {props.invoices ? t('Invoice amount') : t('Payment Amount')}
            </TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Created Time')}</TableHead>
            <TableHead>{t('Related top-ups')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.items.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={7}
                className='text-muted-foreground h-32 text-center'
              >
                {props.loading ? t('Loading...') : t('No data')}
              </TableCell>
            </TableRow>
          )}
          {props.items.map((item) => (
            <Fragment key={item.id}>
              <TableRow>
                <TableCell>
                  <Button
                    size='icon'
                    variant='ghost'
                    aria-label={t('Related top-ups')}
                    aria-expanded={expanded.has(item.id)}
                    onClick={() =>
                      setExpanded((prev) => {
                        const next = new Set(prev)
                        if (next.has(item.id)) next.delete(item.id)
                        else next.add(item.id)
                        return next
                      })
                    }
                  >
                    {expanded.has(item.id) ? (
                      <ChevronDown className='size-4' />
                    ) : (
                      <ChevronRight className='size-4' />
                    )}
                  </Button>
                </TableCell>
                <TableCell>
                  <div>{item.username || `#${item.user_id}`}</div>
                  <div className='text-muted-foreground text-xs'>
                    #{item.user_id}
                  </div>
                </TableCell>
                <TableCell className='max-w-72 break-words whitespace-normal'>
                  {item.company_name || '-'}
                  {props.invoices && (
                    <div className='text-muted-foreground text-xs'>
                      {t('Ticket ID')}: {item.ticket_id}
                    </div>
                  )}
                </TableCell>
                <TableCell className='tabular-nums'>
                  {item.money.toFixed(2)}
                </TableCell>
                <TableCell>
                  {props.invoices
                    ? invoiceLabels[item.invoice_status]
                    : (statusLabels[item.status] ?? item.status)}
                </TableCell>
                <TableCell className='whitespace-nowrap'>
                  {formatTimestampToDate(item.created_time)}
                </TableCell>
                <TableCell>
                  {item.topups.length}
                  {item.incomplete_orders && (
                    <div className='text-destructive max-w-56 text-xs whitespace-normal'>
                      {t('Some linked top-ups are unavailable')}
                    </div>
                  )}
                </TableCell>
              </TableRow>
              {expanded.has(item.id) && (
                <TableRow>
                  <TableCell colSpan={7} className='bg-muted/30 p-3'>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Order Number')}</TableHead>
                          <TableHead>{t('Payment Amount')}</TableHead>
                          <TableHead>{t('Payment Method')}</TableHead>
                          <TableHead>{t('Source')}</TableHead>
                          <TableHead>{t('Status')}</TableHead>
                          <TableHead>{t('Created Time')}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {item.topups.map((order) => (
                          <TableRow key={order.id}>
                            <TableCell className='max-w-72 break-all whitespace-normal'>
                              {order.trade_no}
                            </TableCell>
                            <TableCell className='tabular-nums'>
                              {order.money.toFixed(2)}
                            </TableCell>
                            <TableCell>
                              {order.payment_method || t('Unspecified')}
                            </TableCell>
                            <TableCell>
                              {order.source || t('Unspecified')}
                            </TableCell>
                            <TableCell>
                              {statusLabels[order.status] ?? order.status}
                            </TableCell>
                            <TableCell className='whitespace-nowrap'>
                              {formatTimestampToDate(order.create_time)}
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </TableCell>
                </TableRow>
              )}
            </Fragment>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
