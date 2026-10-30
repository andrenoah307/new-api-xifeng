/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { Download, Check } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldLabel, FieldGroup } from '@/components/ui/field'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import type {
  InvoiceCorrection,
  RefundFinanceDetail as FinanceData,
} from '../refund-finance-api'

import { refundCash, refundQuota } from '../refund-finance-format'

export function RefundFinanceDetail(props: {
  finance: FinanceData
  ticketId?: number
  onUpdated?: () => void
}) {
  const { t } = useTranslation()
  const role = useAuthStore((state) => state.auth.user?.role)
  const staff = [3, 10, 100].includes(role ?? 0)
  const [error, setError] = useState('')
  async function download() {
    try {
      const response = await api.get('/api/ticket/refund/finance-export', {
        params: { ticket_id: props.ticketId },
        responseType: 'blob',
      })
      if (String(response.headers['content-type'] ?? '').includes('json')) {
        throw new Error(t('Export failed'))
      }
      const url = URL.createObjectURL(response.data)
      const link = document.createElement('a')
      link.href = url
      link.download = `refund-${props.ticketId}.csv`
      link.click()
      URL.revokeObjectURL(url)
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : t('Export failed'))
    }
  }
  return (
    <section className='space-y-4 border-t pt-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h3 className='font-semibold'>{t('Refund reconciliation')}</h3>
        {staff && props.ticketId && (
          <Button variant='outline' onClick={download}>
            <Download />
            {t('Export')}
          </Button>
        )}
      </div>
      <dl className='grid grid-cols-2 gap-4 text-sm'>
        <div>
          <dt>{t('Cash refund')}</dt>
          <dd className='font-semibold'>
            {refundCash(props.finance.refund_cash_cents)}
          </dd>
        </div>
        <div>
          <dt>{t('Refunded tax')}</dt>
          <dd>{refundCash(props.finance.refund_tax_cents)}</dd>
        </div>
      </dl>
      {error && (
        <p role='alert' className='text-destructive'>
          {t(error)}
        </p>
      )}
      <div className='overflow-x-auto'>
        <table className='w-full text-sm'>
          <thead>
            <tr>
              {[
                'Order number',
                'Paid amount',
                'Original tax',
                'Used invoice amount',
                'Original quota',
                'Used quota',
                'Refundable quota',
                'Refund quota',
                'Cash refund',
                'Refunded tax',
                'Remaining invoice amount',
              ].map((label) => (
                <th
                  key={label}
                  className='whitespace-nowrap border-b p-2 text-left'
                >
                  {t(label)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {props.finance.allocations.map((row) => (
              <tr key={row.top_up_id}>
                <td className='whitespace-nowrap p-2'>
                  {row.trade_no || `#${row.top_up_id}`}
                </td>
                {(
                  [
                    'original_paid_cents',
                    'original_tax_cents',
                    'used_invoice_cents',
                  ] as const
                ).map((key) => (
                  <td key={key} className='whitespace-nowrap p-2'>
                    {refundCash(row[key])}
                  </td>
                ))}
                {(
                  [
                    'original_quota',
                    'used_quota',
                    'refundable_quota',
                    'refund_quota',
                  ] as const
                ).map((key) => (
                  <td key={key} className='whitespace-nowrap p-2'>
                    {refundQuota(row[key], row.quota_per_unit)}
                  </td>
                ))}
                {(
                  [
                    'refund_cents',
                    'refund_tax_cents',
                    'remaining_invoice_cents',
                  ] as const
                ).map((key) => (
                  <td key={key} className='whitespace-nowrap p-2'>
                    {refundCash(row[key])}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {props.finance.invoice_corrections.map((correction) => (
        <InvoiceCorrectionForm
          key={`${correction.invoice_ticket_id}:${correction.id}:${correction.status}`}
          correction={correction}
          editable={staff && !!props.ticketId}
          onUpdated={props.onUpdated}
        />
      ))}
    </section>
  )
}

function InvoiceCorrectionForm(props: {
  correction: InvoiceCorrection
  editable: boolean
  onUpdated?: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [red, setRed] = useState(props.correction.red_reference || '')
  const [reissue, setReissue] = useState(
    props.correction.reissue_reference || '',
  )
  const [evidence, setEvidence] = useState(props.correction.evidence || '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [status, setStatus] = useState(props.correction.status)
  const completed = status === 'completed'
  const processing = status === 'processing'
  async function start() {
    if (busy || !props.editable) return
    setBusy(true)
    try {
      const response = await api.post(
        `/api/ticket/refund/invoice-corrections/${props.correction.id}/start`,
      )
      if (!response.data.success) throw new Error(response.data.message)
      setStatus('processing')
      await client.invalidateQueries({ queryKey: ['tickets'] })
      props.onUpdated?.()
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : t('Operation failed'),
      )
    } finally {
      setBusy(false)
    }
  }
  async function complete(event: React.FormEvent) {
    event.preventDefault()
    if (busy || !props.editable || !processing) return
    setBusy(true)
    try {
      const response = await api.post(
        `/api/ticket/refund/invoice-corrections/${props.correction.id}/complete`,
        {
          red_reference: red.trim(),
          reissue_reference: reissue.trim(),
          evidence: evidence.trim(),
        },
      )
      if (!response.data.success) throw new Error(response.data.message)
      setStatus('completed')
      await client.invalidateQueries({ queryKey: ['tickets'] })
      props.onUpdated?.()
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : t('Operation failed'),
      )
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className='space-y-3 border-t pt-4'>
      <h4 className='font-medium'>
        {t('Invoice correction')} #{props.correction.invoice_ticket_id} ·{' '}
        {completed && t('Completed')}
        {processing && t('Processing')}
        {status === 'pending' && t('Pending')}
        {status === 'cancelled' && t('Cancelled')}
      </h4>
      <p className='text-sm'>
        {t('Original invoice amount')}:{' '}
        {refundCash(props.correction.original_cents)} · {t('Cash refund')}:{' '}
        {refundCash(props.correction.refund_cents)} ·{' '}
        {t('Reissued invoice amount')}:{' '}
        {refundCash(props.correction.reissue_cents)}
      </p>
      <p className='text-sm'>
        {t('Original invoice reference')}:{' '}
        {props.correction.original_reference || '-'}
      </p>
      <FieldGroup className='grid gap-3 sm:grid-cols-2'>
        <Field>
          <FieldLabel htmlFor={`red-${props.correction.id}`}>
            {t('Red invoice reference')}
          </FieldLabel>
          <Input
            id={`red-${props.correction.id}`}
            value={red}
            onChange={(event) => setRed(event.target.value)}
            maxLength={128}
            required
            disabled={!props.editable || !processing}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`reissue-${props.correction.id}`}>
            {t('Reissued invoice reference')}
          </FieldLabel>
          <Input
            id={`reissue-${props.correction.id}`}
            value={reissue}
            onChange={(event) => setReissue(event.target.value)}
            maxLength={128}
            required={props.correction.reissue_cents > 0}
            disabled={
              !props.editable ||
              !processing ||
              props.correction.reissue_cents === 0
            }
          />
        </Field>
        <Field className='sm:col-span-2'>
          <FieldLabel htmlFor={`evidence-${props.correction.id}`}>
            {t('Correction evidence')}
          </FieldLabel>
          <Textarea
            id={`evidence-${props.correction.id}`}
            value={evidence}
            onChange={(event) => setEvidence(event.target.value)}
            maxLength={2000}
            required
            disabled={!props.editable || !processing}
          />
        </Field>
      </FieldGroup>
      {error && (
        <p role='alert' className='text-destructive'>
          {t(error)}
        </p>
      )}
      {props.editable && status === 'pending' && (
        <Button type='button' onClick={start} disabled={busy}>
          {t('Start processing')}
        </Button>
      )}
      {props.editable && processing && (
        <Button
          type='button'
          onClick={complete}
          disabled={
            busy ||
            !red.trim() ||
            !evidence.trim() ||
            (props.correction.reissue_cents > 0 && !reissue.trim())
          }
        >
          <Check />
          {t('Register correction')}
        </Button>
      )}
    </div>
  )
}
