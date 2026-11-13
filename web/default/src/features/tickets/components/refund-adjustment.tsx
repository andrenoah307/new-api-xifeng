import { Download, RefreshCw, X } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'

import {
  refundDiagnosticLines,
  type RefundDiagnostic,
} from '../../../../../shared/refund-diagnostics.mjs'
import {
  RefundCalculationDetails,
  type RefundCalculationPayment,
} from './refund-calculation-details'

type Calculation = {
  cash_cents: number
  payments: RefundCalculationPayment[]
  invoices: { invoice_ticket_id: number; company_name: string }[]
}
type Plan = {
  calculation_basis?: string
  application_balance?: {
    wallet_amount: string
    noncash_amount: string
    cash_amount: string
  }
  ticket_id: number
  revision: string
  requested: string
  balance_before: string
  debit_amount: string
  accounted_amount: string
  known_amount: string
  balance_after: string
  grace_amount: string
  can_apply: boolean
  applied: boolean
  blocker?: string
  diagnostic?: RefundDiagnostic
  calculation?: Calculation
  liabilities: {
    id: string
    request_id: string
    node_name: string
    created_at: number
    request_done: boolean
    task_done: boolean
  }[]
  reason?: string
  actor_id?: number
  applied_at?: number
}

export function RefundAdjustment({
  ticketId,
  disabled,
  onOpenChange,
  onApplied,
}: {
  ticketId: number
  disabled: boolean
  onOpenChange: (open: boolean) => void
  onApplied: (calculation: Calculation) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [plan, setPlan] = useState<Plan | null>(null)
  const [error, setError] = useState('')
  const [reason, setReason] = useState('')
  const [recovery, setRecovery] = useState(false)
  const [settlement, setSettlement] = useState(false)
  const [applicationConfirmed, setApplicationConfirmed] = useState(false)
  const [negativeConfirmed, setNegativeConfirmed] = useState(false)
  const submitting = useRef(false)
  async function preview() {
    if (submitting.current) return
    submitting.current = true
    setOpen(true)
    onOpenChange(true)
    setBusy(true)
    setError('')
    setPlan(null)
    setRecovery(false)
    setSettlement(false)
    setApplicationConfirmed(false)
    setNegativeConfirmed(false)
    try {
      const { data } = await api.get(
        `/api/ticket/refund/${ticketId}/adjustment?basis=application_balance`,
        { skipBusinessError: true }
      )
      if (!data.success) throw new Error(data.message)
      setPlan(data.data)
    } catch (e) {
      setError(e instanceof Error ? t(e.message) : t('Operation failed'))
    } finally {
      submitting.current = false
      setBusy(false)
    }
  }
  async function apply() {
    if (!plan?.can_apply || submitting.current) return
    submitting.current = true
    setBusy(true)
    setError('')
    try {
      const { data } = await api.post(
        `/api/ticket/refund/${ticketId}/adjustment`,
        {
          revision: plan.revision,
          reason: reason.trim(),
          confirm_recovery: recovery,
          confirm_settlement: settlement,
          calculation_basis: plan.calculation_basis ?? '',
          confirm_application_balance: applicationConfirmed,
          confirm_negative: negativeConfirmed,
        },
        { skipBusinessError: true }
      )
      if (!data.success) throw new Error(data.message)
      setPlan(data.data)
      if (data.data.calculation) onApplied(data.data.calculation)
    } catch (e) {
      setError(e instanceof Error ? t(e.message) : t('Operation failed'))
    } finally {
      submitting.current = false
      setBusy(false)
    }
  }
  function download() {
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(plan, null, 2)], { type: 'application/json' })
    )
    const link = document.createElement('a')
    link.href = url
    link.download = `refund-${ticketId}-adjustment.json`
    link.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  if (!open) {
    return (
      <Button
        style={{ height: 'auto', whiteSpace: 'normal', maxWidth: '100%' }}
        disabled={disabled}
        onClick={() => void preview()}
      >
        {t('Review refund adjustment')}
      </Button>
    )
  }
  const needsRecovery = Number(plan?.debit_amount ?? 0) > 0
  const needsSettlement = (plan?.liabilities.length ?? 0) > 0
  const applicationBasis = plan?.calculation_basis === 'application_balance'
  const needsNegative =
    applicationBasis && Number(plan?.application_balance?.cash_amount ?? 0) < 0
  const validReason =
    [...reason.trim()].length >= 5 &&
    [...reason.trim()].length <= 500 &&
    [...reason].every((character) => {
      const code = character.charCodeAt(0)
      return code >= 32 && (code < 127 || code > 159)
    })
  return (
    <section
      className='min-w-0 space-y-3 border-t py-4 text-sm break-words'
      aria-label={t('Review refund adjustment')}
    >
      <h4>{t('Review refund adjustment')}</h4>
      {error && <p role='alert'>{error}</p>}
      {plan && (
        <>
          <dl className='grid grid-cols-1 gap-2 sm:grid-cols-2'>
            {[
              [t('Requested refund'), plan.requested],
              [t('Balance before adjustment'), plan.balance_before],
              ...(applicationBasis
                ? [
                    [
                      t('Preserved noncash credits'),
                      plan.application_balance?.noncash_amount ?? '-',
                    ],
                    [
                      t('Cash balance after refund'),
                      plan.application_balance?.cash_amount ?? '-',
                    ],
                  ]
                : [
                    [
                      t('Historical promotional credit recovery'),
                      plan.debit_amount,
                    ],
                  ]),
              [
                t('Accounted credits (balance, usage and paid fees)'),
                plan.accounted_amount,
              ],
              [t('Known credits after refunds and holds'), plan.known_amount],
              [t('Balance after adjustment'), plan.balance_after],
              [t('Configured refund grace'), plan.grace_amount],
              [
                t('Cash refund'),
                plan.calculation
                  ? (plan.calculation.cash_cents / 100).toFixed(2)
                  : '-',
              ],
            ].map(([label, value]) => (
              <div key={label}>
                <dt>{label}</dt>
                <dd className='font-mono break-all'>
                  {value === '-' ? value : `¥${value}`}
                </dd>
              </div>
            ))}
          </dl>
          {!applicationBasis && (
            <p>
              {t('Promotional recovery formula')}: {plan.accounted_amount} -{' '}
              {plan.known_amount} = {plan.debit_amount}
            </p>
          )}
          {applicationBasis && (
            <p>
              {t('Cash balance after refund')}: {plan.balance_after} -{' '}
              {plan.application_balance?.noncash_amount} ={' '}
              {plan.application_balance?.cash_amount}
            </p>
          )}
          <p>
            {t('Adjustment balance formula')}: {plan.balance_before} -{' '}
            {plan.debit_amount} = {plan.balance_after}
          </p>
          {plan.calculation && (
            <RefundCalculationDetails
              payments={plan.calculation.payments ?? []}
            />
          )}
          {plan.diagnostic && (
            <div role='alert'>
              {refundDiagnosticLines(plan.diagnostic, t).map((line) => (
                <p key={line}>{line}</p>
              ))}
            </div>
          )}
          {plan.blocker === 'no_adjustment' && (
            <p>
              {t(
                'No recovery or settlement acknowledgement is needed. Recalculate the refund normally.'
              )}
            </p>
          )}
          {plan.blocker === 'unsettled_tasks' && (
            <p role='alert'>
              {t('Unfinished asynchronous tasks prevent this adjustment.')}
            </p>
          )}
          {needsSettlement && (
            <ul className='space-y-2 break-all'>
              {plan.liabilities.map((row) => (
                <li key={row.id}>
                  <p>
                    {t('Request ID')}: {row.request_id}
                  </p>
                  <p>
                    {t('Settlement node')}: {row.node_name} /{' '}
                    {new Date(row.created_at * 1000).toISOString()}
                  </p>
                  <p>
                    {t('Request settled')}: {t(row.request_done ? 'Yes' : 'No')}{' '}
                    / {t('Task settled')}: {t(row.task_done ? 'Yes' : 'No')}
                  </p>
                </li>
              ))}
            </ul>
          )}
          {plan.applied ? (
            <>
              <p role='status'>
                {t('Adjustment saved; payment has not been made.')}
              </p>
              <p>
                {t('Operator')}: {plan.actor_id} /{' '}
                {new Date((plan.applied_at ?? 0) * 1000).toISOString()}
              </p>
              <p className='break-words'>{plan.reason}</p>
              <Button
                style={{
                  height: 'auto',
                  whiteSpace: 'normal',
                  maxWidth: '100%',
                }}
                onClick={download}
              >
                <Download size={16} aria-hidden='true' />
                {t('Download adjustment receipt')}
              </Button>
            </>
          ) : (
            <>
              {applicationBasis && (
                <label className='flex items-start gap-2'>
                  <input
                    type='checkbox'
                    checked={applicationConfirmed}
                    disabled={busy || !plan.can_apply}
                    onChange={(e) => setApplicationConfirmed(e.target.checked)}
                  />
                  <span>
                    {t(
                      'I confirm the application balance basis, with noncash credits preserved and no additional wallet deduction.'
                    )}
                  </span>
                </label>
              )}
              {needsNegative && (
                <label className='flex items-start gap-2'>
                  <input
                    type='checkbox'
                    checked={negativeConfirmed}
                    disabled={busy || !plan.can_apply}
                    onChange={(e) => setNegativeConfirmed(e.target.checked)}
                  />
                  <span>
                    {t(
                      'I accept the negative cash balance within the configured refund grace.'
                    )}
                  </span>
                </label>
              )}
              {needsRecovery && (
                <label className='flex items-start gap-2'>
                  <input
                    type='checkbox'
                    checked={recovery}
                    disabled={busy || !plan.can_apply}
                    onChange={(e) => setRecovery(e.target.checked)}
                  />
                  <span>
                    {t(
                      'I classify this unexplained surplus as historical order discounts or promotional credits and authorize its recovery, including a negative balance.'
                    )}
                  </span>
                </label>
              )}
              {needsSettlement && (
                <label className='flex items-start gap-2'>
                  <input
                    type='checkbox'
                    checked={settlement}
                    disabled={busy || !plan.can_apply}
                    onChange={(e) => setSettlement(e.target.checked)}
                  />
                  <span>
                    {t(
                      'I verified that these requests and settlement queues have finished; I authorize clearing only the listed records.'
                    )}
                  </span>
                </label>
              )}
              <label className='block space-y-2'>
                <span>{t('Adjustment reason and verification evidence')}</span>
                <textarea
                  className='block w-full rounded-md border bg-transparent p-2'
                  rows={3}
                  maxLength={1000}
                  value={reason}
                  disabled={busy || !plan.can_apply}
                  onChange={(e) => setReason(e.target.value)}
                />
              </label>
              <Button
                style={{
                  height: 'auto',
                  whiteSpace: 'normal',
                  maxWidth: '100%',
                }}
                disabled={
                  busy ||
                  !plan.can_apply ||
                  !validReason ||
                  (applicationBasis && !applicationConfirmed) ||
                  (needsNegative && !negativeConfirmed) ||
                  (needsRecovery && !recovery) ||
                  (needsSettlement && !settlement)
                }
                onClick={() => void apply()}
              >
                {applicationBasis
                  ? t('Confirm and recalculate')
                  : t('Confirm recovery and recalculate')}
              </Button>
            </>
          )}
        </>
      )}
      <div className='flex flex-wrap gap-2'>
        <Button
          title={t('Refresh')}
          aria-label={t('Refresh')}
          disabled={busy}
          onClick={() => void preview()}
        >
          <RefreshCw size={16} aria-hidden='true' />
        </Button>
        <Button
          title={t('Close')}
          aria-label={t('Close')}
          disabled={busy}
          onClick={() => {
            setOpen(false)
            onOpenChange(false)
          }}
        >
          <X size={16} aria-hidden='true' />
        </Button>
      </div>
    </section>
  )
}
