import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import {
  refundDiagnosticLines,
  type RefundDiagnostic,
} from '../../../../../shared/refund-diagnostics.mjs'
import { RefundAdjustment } from './refund-adjustment'
import {
  RefundCalculationDetails,
  type RefundCalculationPayment,
} from './refund-calculation-details'

export function RefundInvoiceExports({ ticketId }: { ticketId: number }) {
  const role = useAuthStore((state) => state.auth.user?.role)
  if (![3, 10, 100].includes(role ?? 0)) return null
  return <RefundInvoiceExportPanel key={ticketId} ticketId={ticketId} />
}
function RefundInvoiceExportPanel({ ticketId }: { ticketId: number }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [amount, setAmount] = useState('')
  const [saved, setSaved] = useState(false)
  const role = useAuthStore((state) => state.auth.user?.role)
  const [payments, setPayments] = useState<RefundCalculationPayment[]>([])
  const [invoices, setInvoices] = useState<
    { invoice_ticket_id: number; company_name: string }[]
  >([])
  const [busy, setBusy] = useState(false)
  const [adjustmentOpen, setAdjustmentOpen] = useState(false)
  const [error, setError] = useState('')
  const [diagnostic, setDiagnostic] = useState<RefundDiagnostic | null>(null)
  const requestVersion = useRef(0)
  const recalculating = useRef(false)
  useEffect(() => {
    let active = true
    const version = requestVersion.current
    api
      .get(`/api/ticket/refund/${ticketId}/invoice-export`, {
        skipBusinessError: true,
      })
      .then(({ data }) => {
        if (active && version === requestVersion.current && data.success) {
          setAmount((data.data.cash_cents / 100).toFixed(2))
          setSaved(true)
          setInvoices(data.data.invoices ?? [])
          setPayments(data.data.payments ?? [])
        }
      })
      .catch(() => {})
    return () => {
      active = false
    }
  }, [ticketId])
  async function recalculate() {
    if (busy || recalculating.current) return
    recalculating.current = true
    requestVersion.current += 1
    setBusy(true)
    setError('')
    setDiagnostic(null)
    setSaved(false)
    try {
      const { data } = await api.post(
        `/api/ticket/refund/${ticketId}/invoice-export`,
        {},
        { skipBusinessError: true }
      )
      if (!data.success && data.diagnostic) {
        setDiagnostic(data.diagnostic)
        return
      }
      if (!data.success) throw new Error(data.message)
      setAmount((data.data.cash_cents / 100).toFixed(2))
      setSaved(true)
      setInvoices(data.data.invoices ?? [])
      setPayments(data.data.payments ?? [])
    } catch (e) {
      setError(e instanceof Error ? t(e.message) : t('Operation failed'))
    } finally {
      recalculating.current = false
      setBusy(false)
    }
  }
  async function download(kind: string) {
    if (busy || !saved) return
    setBusy(true)
    setError('')
    try {
      const response = await api.get(
        `/api/ticket/refund/${ticketId}/invoice-export`,
        { params: { kind }, responseType: 'blob' }
      )
      if (
        !String(response.headers['content-type'] || '').includes('text/csv')
      ) {
        const result = JSON.parse(await response.data.text())
        throw new Error(result.message || t('Operation failed'))
      }
      const url = URL.createObjectURL(response.data)
      const link = document.createElement('a')
      link.href = url
      link.download = `refund-${ticketId}-${kind}.csv`
      link.click()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (e) {
      setError(e instanceof Error ? t(e.message) : t('Operation failed'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <section className='space-y-3 border-t pt-4'>
      <h3>{t('Refund invoice exports')}</h3>
      <p>
        {t('Cash refund')}: {saved ? `¥${amount}` : '-'}
      </p>
      <div className='flex flex-wrap gap-2'>
        <Button
          disabled={busy || adjustmentOpen}
          onClick={() => void recalculate()}
        >
          {t('Recalculate')}
        </Button>
        {['red', 'reissue'].map((kind, i) => (
          <Button
            key={kind}
            disabled={!saved || busy || adjustmentOpen}
            onClick={() => void download(kind)}
          >
            {t(
              [
                'Export invoices to reverse',
                'Export recalculated reissued invoices',
              ][i]
            )}
          </Button>
        ))}
      </div>
      <RefundAdjustment
        ticketId={ticketId}
        disabled={busy}
        onOpenChange={(open) => {
          requestVersion.current += 1
          setAdjustmentOpen(open)
          if (open) setSaved(false)
        }}
        onApplied={(calculation) => {
          setAmount((calculation.cash_cents / 100).toFixed(2))
          setPayments(calculation.payments ?? [])
          setInvoices(calculation.invoices ?? [])
          setDiagnostic(null)
          setSaved(true)
          void queryClient.invalidateQueries({ queryKey: ['tickets'] })
          void queryClient.invalidateQueries({ queryKey: ['user'] })
        }}
      />
      {saved && invoices.length > 0 && (
        <details>
          <summary className='cursor-pointer'>
            {t('Historical invoice tickets')} ({invoices.length})
          </summary>
          <ul className='mt-2 space-y-2'>
            {invoices.map((invoice) => (
              <li key={invoice.invoice_ticket_id}>
                <a
                  className='text-primary break-all underline'
                  href={`/ticket-admin/${invoice.invoice_ticket_id}`}
                >
                  #{invoice.invoice_ticket_id} {invoice.company_name}
                </a>
              </li>
            ))}
          </ul>
        </details>
      )}
      {error && <p role='alert'>{error}</p>}
      {diagnostic && (
        <div
          role='alert'
          className='space-y-2 border-l-2 border-amber-500 pl-3 text-sm break-words'
        >
          {refundDiagnosticLines(diagnostic, t).map((line) => (
            <p key={line}>{line}</p>
          ))}
          {!!diagnostic.ticket_id && (
            <a
              className='text-primary underline'
              href={`/ticket-admin/${diagnostic.ticket_id}`}
            >
              {t('Ticket ID')}: #{diagnostic.ticket_id}
            </a>
          )}
          {!!diagnostic.order_id && (
            <p>
              {t('Order ID')}: {diagnostic.order_id}
            </p>
          )}
          {diagnostic.amounts && (
            <details>
              <summary className='cursor-pointer'>{t('Exact amounts')}</summary>
              {refundDiagnosticLines(diagnostic, t, true)
                .slice(0, 3)
                .map((line) => (
                  <p key={line}>{line}</p>
                ))}
            </details>
          )}
        </div>
      )}
      {saved && [10, 100].includes(role ?? 0) && (
        <RefundCalculationDetails payments={payments} />
      )}
    </section>
  )
}
