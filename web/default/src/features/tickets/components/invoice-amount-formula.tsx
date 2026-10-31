import { useTranslation } from 'react-i18next'

import { refundCash } from '../refund-finance-format'

export function InvoiceAmountFormula(props: {
  net: number
  tax: number
  additional?: number
  total: number
  totalLabel?: string
}) {
  const { t } = useTranslation()
  const parts = [
    { label: 'invoice.amount.net', cents: props.net },
    { label: 'invoice.amount.tax', cents: props.tax },
    { label: 'invoice.amount.additional', cents: props.additional ?? 0 },
  ]
  const valid = [...parts.map((part) => part.cents), props.total].every(
    (value) => Number.isSafeInteger(value) && value >= 0
  )
  if (!valid) return <p>{t('Payment breakdown')}: -</p>
  const visible = parts.filter((part) => part.cents > 0)
  return (
    <div
      className='max-w-full overflow-x-auto py-2'
      tabIndex={0}
      role='region'
      aria-label={t('Payment breakdown')}
    >
      <p
        className='m-0 w-max text-sm whitespace-nowrap tabular-nums'
        data-invoice-formula
      >
        {visible.map((part, index) => (
          <span key={part.label}>
            {index > 0 && <span aria-hidden='true'> + </span>}
            {t(part.label)} <strong>{refundCash(part.cents)}</strong>
          </span>
        ))}
        {visible.length > 0 && <span aria-hidden='true'> = </span>}
        {t(props.totalLabel ?? 'invoice.amount.total')}{' '}
        <strong>{refundCash(props.total)}</strong>
      </p>
    </div>
  )
}
