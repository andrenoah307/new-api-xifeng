import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, Check, RefreshCw, Play } from 'lucide-react'
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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Field, FieldLabel, FieldGroup } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import {
  type InvoiceCorrection,
  type RefundFinanceDetail as FinanceData,
  quoteRefundFinance,
} from '../refund-finance-api'
import { refundCash } from '../refund-finance-format'
import {
  CorrectionWorkflow,
  ReconciliationPanel,
  RefundPayoutPanel,
} from './finance-workflow'

export function RefundFinanceDetail(props: {
  finance: FinanceData
  ticketId?: number
  userId?: number
  requestedQuota?: number
  onUpdated?: () => void
}) {
  const { t } = useTranslation()
  const giftRecoveryLabels: Record<string, string> = {
    prior_order_expired: t('Earlier order gift expired'),
    proportional_refund: t('Proportional refund recovery'),
  }
  const client = useQueryClient()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  const staff = [3, 10, 100].includes(role ?? 0)
  const isOwner = !!userId && props.userId === userId
  const [confirming, setConfirming] = useState(false)
  const [refreshCooling, setRefreshCooling] = useState(false)
  const canQuote =
    !!userId &&
    (isOwner || staff) &&
    !!props.ticketId &&
    !!props.requestedQuota &&
    props.finance.settlement_status === 'waiting' &&
    props.finance.allocations.length === 0
  const quoteKey = [
    'refund-confirmation-quote',
    props.ticketId,
    userId,
    props.requestedQuota,
  ]
  const quoteQuery = useQuery({
    queryKey: quoteKey,
    queryFn: ({ signal }) =>
      quoteRefundFinance(props.requestedQuota ?? 0, signal, props.ticketId),
    enabled: canQuote,
    retry: false,
    gcTime: 0,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const quote =
    canQuote && !quoteQuery.isFetching && !quoteQuery.isError
      ? quoteQuery.data
      : null
  const busy = confirming || quoteQuery.isFetching
  useEffect(() => {
    if (!canQuote) return
    setRefreshCooling(true)
    if (quoteQuery.isFetching) return
    const timer = setTimeout(() => setRefreshCooling(false), 1500)
    return () => clearTimeout(timer)
  }, [
    canQuote,
    quoteQuery.isFetching,
    quoteQuery.dataUpdatedAt,
    quoteQuery.errorUpdatedAt,
  ])
  const waitingLabels: Record<string, string> = {
    financial_reconciliation: 'Financial reconciliation required',
    awaiting_review: 'Financial reconciliation required',
    awaiting_owner_confirmation: 'Refund amount is ready for your confirmation',
  }
  const [error, setError] = useState('')
  async function confirmAmount(confirm: boolean) {
    if (busy) return
    setError('')
    if (!confirm) {
      if (refreshCooling) return
      await quoteQuery.refetch()
      return
    }
    if (!quote?.quote_token || !canQuote || !isOwner) return
    setConfirming(true)
    try {
      const response = await api.post('/api/ticket/refund/finance-confirm', {
        ticket_id: props.ticketId,
        refund_quota: quote.allocations.reduce(
          (sum, row) => sum + row.refund_quota,
          0
        ),
        quote_token: quote.quote_token,
      })
      if (!response.data.success) throw new Error(response.data.message)
      client.setQueryData(quoteKey, null)
      await client.invalidateQueries({ queryKey: ['tickets'] })
      await client.invalidateQueries({ queryKey: ['user'] })
      props.onUpdated?.()
    } catch (failure) {
      client.setQueryData(quoteKey, null)
      setError(
        failure instanceof Error ? failure.message : t('Operation failed')
      )
    } finally {
      setConfirming(false)
    }
  }
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
  if (
    props.finance.settlement_status === 'waiting' &&
    props.finance.allocations.length === 0
  ) {
    return (
      <div className='space-y-3 text-sm' role='status'>
        {t(
          quote?.quote_token
            ? 'Refund amount is ready for your confirmation'
            : (waitingLabels[(quote ?? props.finance).waiting_reason ?? ''] ??
                'Awaiting settlement verification. The refund amount must be confirmed again after verification.')
        )}
        <p>
          {t(
            'New consumption is suspended until this refund is completed or cancelled.'
          )}
        </p>
        {canQuote && (
          <Button
            variant='outline'
            disabled={busy || refreshCooling}
            onClick={() => confirmAmount(false)}
          >
            <RefreshCw
              className={quoteQuery.isFetching ? 'animate-spin' : undefined}
            />
            {t('Refresh')}
          </Button>
        )}
        {quote?.quote_token && (
          <div className='flex flex-wrap items-center gap-3'>
            <p className='w-full font-medium'>
              {isOwner
                ? t('Confirm the refund amount to continue processing.')
                : t(
                    'Processing can continue after the applicant confirms the refund amount.'
                  )}
            </p>
            {quote.refund_order_type && (
              <p className='w-full'>
                {t(
                  quote.refund_order_type === 'without_invoice'
                    ? 'This refund covers orders without invoice attributes. Apply again after completion for any remaining orders.'
                    : 'This refund covers orders with invoice attributes. Apply again after completion for any remaining orders.'
                )}
              </p>
            )}
            <span>
              {t('Cash refund')}: {refundCash(quote.refund_cash_cents)}
            </span>
            {!!quote.refund_gift_quota && (
              <p className='w-full'>
                {t('Gift credit reclaimed (original value, CNY)')}:{' '}
                {refundCash(quote.refund_gift_value_cents ?? 0)}.{' '}
                {t(
                  'Gift credit is removed from the wallet, not deducted again from the cash refund.'
                )}
              </p>
            )}
            {quote.allocations.some(
              (row) => row.gift_recovery_reason === 'prior_order_expired'
            ) && (
              <p className='w-full'>
                {t('Earlier order gift expired')}:{' '}
                {refundCash(
                  quote.allocations
                    .filter(
                      (row) =>
                        row.gift_recovery_reason === 'prior_order_expired'
                    )
                    .reduce(
                      (sum, row) => sum + (row.refund_gift_value_cents ?? 0),
                      0
                    )
                )}
              </p>
            )}
            {isOwner && (
              <Button disabled={busy} onClick={() => confirmAmount(true)}>
                <Check />
                {t('Confirm')}
              </Button>
            )}
          </div>
        )}
        {quoteQuery.isFetching && canQuote && <p>{t('Loading...')}</p>}
        {(error || quoteQuery.error) && (
          <p role='alert'>{t(error || quoteQuery.error?.message || '')}</p>
        )}
        {staff && props.ticketId && (
          <ReconciliationPanel ticketId={props.ticketId} />
        )}
      </div>
    )
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
        {!!props.finance.refund_gift_quota && (
          <div>
            <dt>{t('Gift credit reclaimed (original value, CNY)')}</dt>
            <dd>{refundCash(props.finance.refund_gift_value_cents ?? 0)}</dd>
          </div>
        )}
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
        <div>
          <dt>{t('Invoice amount to reverse')}</dt>
          <dd>
            {refundCash(
              props.finance.invoice_corrections
                .filter((row) => row.status !== 'cancelled')
                .reduce((sum, row) => sum + row.original_cents, 0)
            )}
          </dd>
        </div>
        <div>
          <dt>{t('Reissued invoice amount')}</dt>
          <dd>
            {refundCash(
              props.finance.invoice_corrections
                .filter((row) => row.status !== 'cancelled')
                .reduce((sum, row) => sum + row.reissue_cents, 0)
            )}
          </dd>
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
                'Consumed actual payment',
                'Cash refund',
                'Refunded tax',
                'Remaining invoice amount',
                'Gift credit reclaimed (original value, CNY)',
                'Paid balance after refund (CNY)',
                'Gift balance after refund (original value, CNY)',
                'Gift recovery reason',
              ].map((label) => (
                <th
                  key={label}
                  className='border-b p-2 text-left whitespace-nowrap'
                >
                  {t(label)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {props.finance.allocations.map((row) => (
              <tr key={row.top_up_id}>
                <td className='p-2 whitespace-nowrap'>
                  {row.trade_no || `#${row.top_up_id}`}
                </td>
                {(
                  [
                    'original_paid_cents',
                    'original_tax_cents',
                    'used_invoice_cents',
                  ] as const
                ).map((key) => (
                  <td key={key} className='p-2 whitespace-nowrap'>
                    {refundCash(row[key])}
                  </td>
                ))}
                {(
                  [
                    'refund_cents',
                    'refund_tax_cents',
                    'remaining_invoice_cents',
                    'refund_gift_value_cents',
                    'remaining_paid_value_cents',
                    'remaining_gift_value_cents',
                  ] as const
                ).map((key) => (
                  <td key={key} className='p-2 whitespace-nowrap'>
                    {!row.gift_policy_version &&
                    [
                      'refund_gift_value_cents',
                      'remaining_paid_value_cents',
                      'remaining_gift_value_cents',
                    ].includes(key)
                      ? '-'
                      : refundCash(row[key] ?? 0)}
                  </td>
                ))}
                <td className='p-2 whitespace-nowrap'>
                  {giftRecoveryLabels[row.gift_recovery_reason ?? ''] ?? '-'}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {staff &&
        props.finance.invoice_corrections.map((correction) =>
          correction.version === 1 ? (
            <CorrectionWorkflow
              key={`${correction.id}:${correction.phase}:${correction.status}`}
              correction={correction}
              editable={staff && !!props.ticketId}
              onUpdated={() => {
                if (props.onUpdated) props.onUpdated()
                else void client.invalidateQueries({ queryKey: ['tickets'] })
              }}
            />
          ) : (
            <InvoiceCorrectionForm
              key={`${correction.invoice_ticket_id}:${correction.id}:${correction.status}`}
              correction={correction}
              editable={staff && !!props.ticketId}
              onUpdated={props.onUpdated}
            />
          )
        )}
      {staff && props.ticketId && (
        <RefundPayoutPanel
          key={`${props.ticketId}:${props.finance.invoice_corrections.map((row) => `${row.id}:${row.status}`).join(',')}`}
          ticketId={props.ticketId}
          onUpdated={() => {
            if (props.onUpdated) props.onUpdated()
            else void client.invalidateQueries({ queryKey: ['tickets'] })
          }}
        />
      )}
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
    props.correction.reissue_reference || ''
  )
  const [evidence, setEvidence] = useState(props.correction.evidence || '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [status, setStatus] = useState(props.correction.status)
  const completed = status === 'completed'
  const processing = status === 'processing'
  async function start() {
    setBusy(true)
    try {
      const response = await api.post(
        `/api/ticket/refund/invoice-corrections/${props.correction.id}/start`
      )
      if (!response.data.success) throw new Error(response.data.message)
      setStatus('processing')
      await client.invalidateQueries({ queryKey: ['tickets'] })
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : t('Operation failed')
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
        }
      )
      if (!response.data.success) throw new Error(response.data.message)
      setStatus('completed')
      await client.invalidateQueries({ queryKey: ['tickets'] })
      props.onUpdated?.()
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : t('Operation failed')
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
            required={props.correction.original_cents > 0}
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
      {props.editable && processing && (
        <Button
          type='button'
          onClick={complete}
          disabled={
            busy ||
            (props.correction.original_cents > 0 && !red.trim()) ||
            !evidence.trim() ||
            (props.correction.reissue_cents > 0 && !reissue.trim())
          }
        >
          <Check />
          {t('Register correction')}
        </Button>
      )}
      {props.editable && status === 'pending' && (
        <Button disabled={busy} onClick={start}>
          <Play />
          {t('Start correction')}
        </Button>
      )}
    </div>
  )
}
