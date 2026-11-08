import { useTranslation } from 'react-i18next'

export interface RefundCalculationPayment {
  order_id: number
  trade_no: string
  paid_cents: number
  used_cents: number
  refund_cents: number
  refund_tax_cents: number
  calculation?: {
    previously_refunded_cents: number
    remaining_before_cents: number
    remaining_after_cents: number
    used_percent: string
    cumulative_refund_percent: string
  }
}

const money = (cents: number) => `¥${(cents / 100).toFixed(2)}`

export function RefundCalculationDetails(props: {
  payments: RefundCalculationPayment[]
}) {
  const { t } = useTranslation()
  return (
    <div className='flex min-w-0 flex-col gap-4'>
      <h3 className='text-sm font-semibold'>{t('FIFO refund calculation')}</h3>
      {props.payments.map((payment) => (
        <div key={payment.order_id} className='min-w-0 border-t pt-3'>
          <h4 className='font-medium'>
            {t('Recharge order ID')}: {payment.order_id}
          </h4>
          <p className='break-all'>
            {t('Recharge order number')}: {payment.trade_no}
          </p>
          {!payment.calculation ? (
            <p>
              {t('Calculation details unavailable in this historical snapshot')}
            </p>
          ) : (
            <>
              <dl className='my-3 grid grid-cols-1 gap-3 sm:grid-cols-2'>
                {[
                  [t('Original cash paid'), payment.paid_cents],
                  [t('Consumed paid amount'), payment.used_cents],
                  [
                    t('Previously refunded cash'),
                    payment.calculation.previously_refunded_cents,
                  ],
                  [
                    t('Remaining paid amount before refund'),
                    payment.calculation.remaining_before_cents,
                  ],
                  [t('Cash refund'), payment.refund_cents],
                  [
                    t('Refunded invoice service fee included'),
                    payment.refund_tax_cents,
                  ],
                  [
                    t('Remaining paid amount after refund'),
                    payment.calculation.remaining_after_cents,
                  ],
                ].map(([label, cents]) => (
                  <div key={label}>
                    <dt className='text-sm'>{label}</dt>
                    <dd className='font-medium tabular-nums'>
                      {money(Number(cents))}
                    </dd>
                  </div>
                ))}
              </dl>
              <p className='text-sm break-words'>
                {t(
                  'Consumed paid amount = {{paid}} × {{percent}}% ≈ {{used}}',
                  {
                    paid: money(payment.paid_cents),
                    percent: payment.calculation.used_percent,
                    used: money(payment.used_cents),
                  }
                )}
              </p>
              <p className='text-sm break-words'>
                {t(
                  'Cash refund = round({{paid}} × {{percent}}%) − {{previous}} = {{refund}}',
                  {
                    paid: money(payment.paid_cents),
                    percent: payment.calculation.cumulative_refund_percent,
                    previous: money(
                      payment.calculation.previously_refunded_cents
                    ),
                    refund: money(payment.refund_cents),
                  }
                )}
              </p>
            </>
          )}
        </div>
      ))}
    </div>
  )
}
