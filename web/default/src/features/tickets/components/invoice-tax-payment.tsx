import { useMutation, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { submitPaymentForm } from '@/features/wallet/lib/payment'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { invoiceTaxModes, type InvoiceTaxCharge } from '../invoice-tax'
import { InvoiceAmountFormula } from './invoice-amount-formula'

export function InvoiceTaxPayment({
  charge,
  rejected,
}: {
  charge: InvoiceTaxCharge
  rejected: boolean
}) {
  const { t } = useTranslation()
  const owner = useAuthStore((state) => state.auth.user?.id === charge.user_id)
  const details = useQuery({
    queryKey: ['invoice-tax-payment', charge.ticket_id],
    enabled: owner,
    queryFn: async () => {
      const { data } = await api.get(
        `/api/ticket/invoice/${charge.ticket_id}/tax`
      )
      if (!data.success) throw new Error(data.message)
      return data.data as {
        charge: InvoiceTaxCharge
        payment_methods: string[]
      }
    },
    refetchInterval: charge.status === 'pending' ? 10000 : false,
  })
  const current = details.data?.charge ?? charge
  const payment = useMutation({
    mutationFn: async (method: string) => {
      const { data } = await api.post(
        `/api/ticket/invoice/${charge.ticket_id}/tax/pay`,
        { payment_method: method }
      )
      if (!data.success) throw new Error(data.message)
      submitPaymentForm(data.data.url, data.data.params)
    },
  })
  const statusLabels: Record<string, string> = {
    pending: t('Tax payment pending'),
    paid: t('Tax paid'),
    refunded: t('Tax refunded to balance'),
  }
  let equationKey = 'invoice.amount.total'
  if (current.status === 'paid') {
    equationKey = 'invoice.amount.paid'
  }
  if (current.status === 'pending') {
    equationKey = 'invoice.amount.due'
  }
  return (
    <section className='space-y-3 border-y py-3 text-sm'>
      <dl className='grid grid-cols-2 gap-2'>
        <dt>{t('Invoice type')}</dt>
        <dd>
          {t(
            current.invoice_type === 'special'
              ? 'Special VAT invoice'
              : 'Ordinary VAT invoice'
          )}
        </dd>
        <dt>{t('Tax calculation mode')}</dt>
        <dd>
          {t(
            invoiceTaxModes.find((mode) => mode.value === current.mode)
              ?.label ?? current.mode
          )}
        </dd>
        <dt>{t('Invoice fee rate (%)')}</dt>
        <dd>{current.rate_bps / 100}%</dd>
        <dt>{t('Status')}</dt>
        <dd>{statusLabels[current.status]}</dd>
      </dl>
      <InvoiceAmountFormula
        net={current.base_cents}
        tax={current.fee_cents}
        total={current.total_cents}
        totalLabel={equationKey}
      />
      {owner && current.status === 'pending' && !rejected && (
        <div className='flex flex-wrap gap-2'>
          {(details.data?.payment_methods ?? [])
            .filter(
              (method) =>
                !current.payment_method || method === current.payment_method
            )
            .map((method) => (
              <Button
                key={method}
                disabled={payment.isPending}
                onClick={() => payment.mutate(method)}
              >
                {t('Pay full tax charge online')} ·{' '}
                {t(method === 'alipay' ? 'Alipay' : 'WeChat Pay')}
              </Button>
            ))}
          {details.isSuccess && details.data.payment_methods.length === 0 && (
            <p>
              {t(
                'Online tax payment is currently unavailable. Contact the administrator.'
              )}
            </p>
          )}
        </div>
      )}
    </section>
  )
}
