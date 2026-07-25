import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import { StatusBadge } from '@/components/status-badge'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
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
import { CopyField } from './copy-field'

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
  const [revokeOpen, setRevokeOpen] = useState(false)
  const statusCfg = INVOICE_STATUS_CONFIG[invoice.invoice_status]

  // 一次性复制整套开票信息，便于粘贴到开票系统
  const fullInfoText = [
    `${t('Invoice Type')}: ${
      invoice.invoice_type === 2
        ? t('VAT Special Invoice')
        : t('Regular Invoice')
    }`,
    `${t('Company Name')}: ${invoice.company_name}`,
    `${t('Tax Number')}: ${invoice.tax_number}`,
    invoice.bank_name && `${t('Bank Name')}: ${invoice.bank_name}`,
    invoice.bank_account && `${t('Bank Account')}: ${invoice.bank_account}`,
    invoice.company_address &&
      `${t('Company Address')}: ${invoice.company_address}`,
    invoice.company_phone && `${t('Company Phone')}: ${invoice.company_phone}`,
    invoice.email && `${t('Receiving Email')}: ${invoice.email}`,
    `${t('Applied Amount')}: ¥${invoice.total_money.toFixed(2)}`,
    invoice.remark && `${t('Invoice Notes')}: ${invoice.remark}`,
  ]
    .filter(Boolean)
    .join('\n')

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle className="text-base">{t('Invoice Detail')}</CardTitle>
        <div className="flex items-center gap-1.5">
          <CopyButton
            value={fullInfoText}
            variant="outline"
            size="sm"
            className="h-7 gap-1 px-2 text-xs"
            iconClassName="h-3 w-3"
          >
            {t('Copy All')}
          </CopyButton>
          {statusCfg && (
            <StatusBadge
              label={t(statusCfg.labelKey)}
              variant={statusCfg.variant}
              copyable={false}
            />
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-1 gap-x-4 gap-y-2 text-sm sm:grid-cols-2">
          <div>
            <dt className="text-muted-foreground">{t('Invoice Type')}</dt>
            <dd className="font-medium">
              {invoice.invoice_type === 2
                ? t('VAT Special Invoice')
                : t('Regular Invoice')}
              {(invoice.fee_rate ?? 0) > 0 && (
                <span className="text-muted-foreground ml-2 text-xs font-normal">
                  {t('Fee rate {{rate}}%', { rate: invoice.fee_rate })}
                  {(invoice.fee_charged_time ?? 0) > 0 &&
                    `（${formatQuota(invoice.fee_quota ?? 0)}）`}
                </span>
              )}
            </dd>
          </div>
          <CopyField label={t('Company Name')} value={invoice.company_name} />
          <CopyField
            label={t('Tax Number')}
            value={invoice.tax_number}
            valueClassName="font-mono text-xs"
          />
          <CopyField label={t('Receiving Email')} value={invoice.email} />
          <CopyField label={t('Bank Name')} value={invoice.bank_name} />
          <CopyField
            label={t('Bank Account')}
            value={invoice.bank_account}
            valueClassName="font-mono text-xs"
          />
          <CopyField
            label={t('Company Address')}
            value={invoice.company_address}
          />
          <CopyField label={t('Company Phone')} value={invoice.company_phone} />
          {invoice.remark && (
            <CopyField
              label={t('Invoice Notes')}
              value={invoice.remark}
              className="sm:col-span-2"
              multiline
            />
          )}
          <CopyField
            label={t('Applied Amount')}
            value={`¥${invoice.total_money.toFixed(2)}`}
            copyValue={invoice.total_money.toFixed(2)}
            valueClassName="font-mono text-red-600 dark:text-red-400"
          />
          {invoice.issued_time > 0 && (
            <CopyField
              label={t('Issued At')}
              value={formatTimestampToDate(invoice.issued_time)}
            />
          )}
          {(invoice.fee_quota ?? 0) > 0 && (
            <CopyField
              label={t('Fee Charged')}
              value={formatQuota(invoice.fee_quota ?? 0)}
              valueClassName="font-mono text-red-600 dark:text-red-400"
            />
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
                    <TableHead>{t('Creation Time')}</TableHead>
                    <TableHead>{t('Paid Amount')}</TableHead>
                    <TableHead>{t('Completion Time')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {orders.map((o) => (
                    <TableRow key={o.id}>
                      <TableCell className="font-mono text-xs">
                        <span className="inline-flex items-center gap-1">
                          {o.trade_no}
                          <CopyButton
                            value={o.trade_no}
                            size="icon"
                            className="h-5 w-5"
                            iconClassName="h-3 w-3"
                          />
                        </span>
                      </TableCell>
                      <TableCell className="text-muted-foreground text-xs">
                        {formatTimestampToDate(o.create_time)}
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

        {!readonly && (invoice.fee_quota ?? 0) > 0 && (
          <p className="text-xs text-red-600 dark:text-red-400">
            {t(
              'An invoice fee of {{fee}} has been deducted from the user balance. It is not refunded automatically — handle it manually if this invoice is voided later.',
              { fee: formatQuota(invoice.fee_quota ?? 0) }
            )}
          </p>
        )}

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

        {/* 已开票后的纠错入口：改为驳回。手续费不会自动退还，因此二次确认。 */}
        {!readonly && invoice.invoice_status === 2 && (
          <div className="pt-2">
            <Button
              variant="outline"
              size="sm"
              className="text-destructive"
              disabled={loading}
              onClick={() => setRevokeOpen(true)}
            >
              {t('Revoke Issuance')}
            </Button>
          </div>
        )}

        <ConfirmDialog
          open={revokeOpen}
          onOpenChange={setRevokeOpen}
          title={t('Revoke Issuance')}
          desc={
            (invoice.fee_quota ?? 0) > 0
              ? t(
                  'This invoice will go back to rejected. The invoice fee of {{fee}} is NOT refunded automatically, and the linked top-up orders stay locked so they cannot be invoiced again — refund manually if needed.',
                  { fee: formatQuota(invoice.fee_quota ?? 0) }
                )
              : t('This invoice will go back to rejected.')
          }
          confirmText={t('Revoke Issuance')}
          destructive
          isLoading={loading}
          handleConfirm={() => {
            setRevokeOpen(false)
            onStatusChange?.(3)
          }}
        />
      </CardContent>
    </Card>
  )
}
