import { useMutation } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'

import {
  updateTopupInvoiceType,
  type InvoiceAdjustmentPlan,
  type TopupRecord,
} from '../api'

export function InvoiceTypeDialog(props: {
  order: TopupRecord
  onClose: () => void
  onApplied: () => void
}) {
  const { t } = useTranslation()
  const [invoiceType, setInvoiceType] = useState(
    props.order.invoice_type || 'none'
  )
  const [plan, setPlan] = useState<InvoiceAdjustmentPlan | null>(null)
  const [confirmed, setConfirmed] = useState(false)
  const [error, setError] = useState('')
  const submitting = useRef(false)
  const mutation = useMutation({
    mutationFn: async (apply: boolean) => {
      if (submitting.current) return
      submitting.current = true
      setError('')
      try {
        const result = await updateTopupInvoiceType(
          props.order.id,
          invoiceType,
          props.order.invoice_type || '',
          apply ? plan?.revision : undefined,
          confirmed
        )
        if (apply) props.onApplied()
        else {
          setPlan(result)
          setConfirmed(false)
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Operation failed')
      } finally {
        submitting.current = false
      }
    },
  })
  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open && !submitting.current) props.onClose()
      }}
      title={t('Invoice type')}
      desc={props.order.trade_no}
      confirmText={t('Save')}
      className='max-h-[90dvh] overflow-y-auto'
      isLoading={mutation.isPending}
      disabled={
        !plan?.can_apply || (plan.requires_negative_confirmation && !confirmed)
      }
      handleConfirm={() => {
        if (
          plan?.can_apply &&
          (!plan.requires_negative_confirmation || confirmed)
        ) {
          mutation.mutate(true)
        }
      }}
    >
      <fieldset className='space-y-2' disabled={mutation.isPending}>
        {['ordinary', 'special', 'none'].map((type) => (
          <label key={type} className='flex items-center gap-2'>
            <input
              type='radio'
              name='topup-invoice-type'
              value={type}
              checked={invoiceType === type}
              onChange={() => {
                setInvoiceType(type)
                setPlan(null)
                setConfirmed(false)
                setError('')
              }}
            />
            {type === 'ordinary' && t('Ordinary VAT invoice')}
            {type === 'special' && t('Special VAT invoice')}
            {type === 'none' && '-'}
          </label>
        ))}
      </fieldset>
      <Button
        variant='outline'
        disabled={
          mutation.isPending || invoiceType === props.order.invoice_type
        }
        onClick={() => mutation.mutate(false)}
      >
        <RefreshCw className='size-4' />
        {t('Preview fee adjustment')}
      </Button>
      {plan && (
        <div className='space-y-2 text-sm break-words'>
          <p>
            {t('Paid amount')}: CNY {(plan.paid_cents / 100).toFixed(2)}
          </p>
          <p>
            {t('Invoice fee: {{before}} → {{after}} CNY', {
              before: (plan.tax_before_cents / 100).toFixed(2),
              after: (plan.tax_after_cents / 100).toFixed(2),
            })}
          </p>
          <p>
            {t(
              'Wallet adjustment: {{change}}; balance {{before}} → {{after}}; allowance {{limit}}',
              {
                change: plan.balance_change,
                before: plan.balance_before,
                after: plan.balance_after,
                limit: plan.grace_amount,
              }
            )}
          </p>
          {!plan.can_apply && (
            <p role='alert'>
              {t(
                'Invoice fee deduction exceeds the configured overdraft allowance'
              )}
            </p>
          )}
          {plan.requires_negative_confirmation && (
            <label className='text-destructive flex items-start gap-2'>
              <input
                className='mt-1 shrink-0'
                type='checkbox'
                checked={confirmed}
                disabled={mutation.isPending}
                onChange={(event) => setConfirmed(event.target.checked)}
              />
              <span>
                {t(
                  'I confirm this deduction will leave a negative balance of {{balance}}.',
                  { balance: plan.balance_after }
                )}
              </span>
            </label>
          )}
        </div>
      )}
      {error && (
        <p role='alert' className='text-destructive text-sm break-words'>
          {t(error)}
        </p>
      )}
    </ConfirmDialog>
  )
}
