import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/status-badge'
import { formatTimestampToDate } from '@/lib/format'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import type { TicketInvoice, TicketInvoiceOrder } from '../api'
import { INVOICE_STATUS_CONFIG } from '../constants'

interface InvoiceDetailProps {
  invoice: TicketInvoice
  orders: TicketInvoiceOrder[]
  readonly?: boolean
  loading?: boolean
  onStatusChange?: (status: number) => void
}

export function InvoiceDetail({
  invoice,
  orders,
  readonly,
  loading,
  onStatusChange,
}: InvoiceDetailProps) {
  const { t } = useTranslation()
  const statusCfg = INVOICE_STATUS_CONFIG[invoice.invoice_status]

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle className="text-base">{t('Invoice Detail')}</CardTitle>
        {statusCfg && (
          <StatusBadge
            label={t(statusCfg.labelKey)}
            variant={statusCfg.variant}
            copyable={false}
          />
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-1 gap-x-4 gap-y-2 text-sm sm:grid-cols-2">
          <div>
            <dt className="text-muted-foreground">{t('Company Name')}</dt>
            <dd className="font-medium">{invoice.company_name}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">{t('Tax Number')}</dt>
            <dd className="font-mono text-xs">{invoice.tax_number}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">{t('Receiving Email')}</dt>
            <dd>{invoice.email || '-'}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">{t('Bank Name')}</dt>
            <dd>{invoice.bank_name || '-'}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">{t('Bank Account')}</dt>
            <dd className="font-mono text-xs">{invoice.bank_account || '-'}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">{t('Company Address')}</dt>
            <dd>{invoice.company_address || '-'}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">{t('Company Phone')}</dt>
            <dd>{invoice.company_phone || '-'}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">{t('Applied Amount')}</dt>
            <dd className="font-mono font-medium text-red-600 dark:text-red-400">
              ¥{invoice.total_money.toFixed(2)}
            </dd>
          </div>
          {invoice.issued_time > 0 && (
            <div>
              <dt className="text-muted-foreground">{t('Issued At')}</dt>
              <dd>{formatTimestampToDate(invoice.issued_time)}</dd>
            </div>
          )}
        </dl>

        <div>
          <h4 className="text-muted-foreground mb-2 text-xs font-medium uppercase">
            {t('Related Orders')}
          </h4>
          {orders.length > 0 ? (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Order Number')}</TableHead>
                    <TableHead>{t('Paid Amount')}</TableHead>
                    <TableHead>{t('Completion Time')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {orders.map((o) => (
                    <TableRow key={o.id}>
                      <TableCell className="font-mono text-xs">
                        {o.trade_no}
                      </TableCell>
                      <TableCell className="font-mono text-xs">
                        ¥{o.money.toFixed(2)}
                      </TableCell>
                      <TableCell className="text-muted-foreground text-xs">
                        {formatTimestampToDate(o.complete_time)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">
              {t('No associated orders')}
            </p>
          )}
        </div>

        {!readonly && invoice.invoice_status === 1 && (
          <div className="flex gap-2 pt-2">
            <Button
              size="sm"
              disabled={loading}
              onClick={() => onStatusChange?.(2)}
            >
              {t('Mark as Issued')}
            </Button>
            <Button
              variant="destructive"
              size="sm"
              disabled={loading}
              onClick={() => onStatusChange?.(3)}
            >
              {t('Reject Application')}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
