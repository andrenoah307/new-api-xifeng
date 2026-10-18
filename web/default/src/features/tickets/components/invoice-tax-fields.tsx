import { useQuery } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import {
  getInvoiceTaxConfig,
  getInvoiceTaxQuote,
  invoiceTaxModes,
  type InvoiceTaxSelection,
} from '../invoice-tax'

export function InvoiceTaxNotice() {
  const { t } = useTranslation()
  const config = useQuery({
    queryKey: ['invoice-tax-config'],
    queryFn: getInvoiceTaxConfig,
  })
  if (!config.data?.enabled) return null
  return (
    <p className='text-sm text-amber-700 dark:text-amber-400'>
      {t(
        'Recharge amounts exclude invoice tax charges. An invoice application requires a separate tax payment at the configured rate.'
      )}
    </p>
  )
}

export function InvoiceTaxFields({
  ids,
  open,
  onChange,
}: {
  ids: number[]
  open: boolean
  onChange: (value: InvoiceTaxSelection) => void
}) {
  const { t } = useTranslation()
  const [type, setType] = useState('ordinary')
  const [payment, setPayment] = useState('balance')
  const config = useQuery({
    queryKey: ['invoice-tax-config'],
    queryFn: getInvoiceTaxConfig,
    enabled: open,
  })
  const quote = useQuery({
    queryKey: ['invoice-tax-quote', ids, type, config.data],
    queryFn: () => getInvoiceTaxQuote(ids, type),
    enabled: open && !!config.data?.enabled && ids.length > 0,
    refetchOnWindowFocus: 'always',
    retry: false,
  })
  const method = quote.data?.can_balance ? payment : 'online'
  useEffect(() => {
    onChange({
      ready:
        !!config.data &&
        (!config.data.enabled || !!quote.data) &&
        !quote.isFetching &&
        !quote.isError &&
        !config.isError,
      invoice_type: type,
      tax_payment_method: method,
      tax_quote: quote.data?.token ?? '',
    })
  }, [
    config.data,
    config.isError,
    quote.data,
    quote.isFetching,
    quote.isError,
    type,
    method,
    onChange,
  ])
  if (!config.data) {
    return <p>{config.isError ? t('Failed to load') : t('Loading...')}</p>
  }
  if (!config.data.enabled) return null
  return (
    <section className='space-y-3 border-y py-3'>
      <InvoiceTaxNotice />
      <p className='text-muted-foreground text-xs'>
        {t(
          'Only verified CNY recharge records are eligible. Foreign-currency records are not supported yet.'
        )}
      </p>
      <div className='flex items-center justify-between gap-2'>
        <Label>{t('Invoice type')}</Label>
        <Button
          type='button'
          variant='ghost'
          size='icon'
          aria-label={t('Refresh')}
          title={t('Refresh')}
          disabled={!ids.length || quote.isFetching}
          onClick={() => quote.refetch()}
        >
          <RefreshCw className='size-4' />
        </Button>
      </div>
      <Select
        value={type}
        onValueChange={(value) => {
          if (value) setType(value)
        }}
      >
        <SelectTrigger className='w-full'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value='ordinary'>{t('Ordinary VAT invoice')}</SelectItem>
          <SelectItem value='special'>{t('Special VAT invoice')}</SelectItem>
        </SelectContent>
      </Select>
      {quote.isError && (
        <p className='text-destructive text-sm'>
          {t(
            'Tax quote unavailable. Refresh the selected orders and try again.'
          )}
        </p>
      )}
      {quote.data && !quote.isFetching && (
        <>
          <dl className='grid grid-cols-2 gap-2 text-sm'>
            <dt>{t('Tax calculation mode')}</dt>
            <dd>
              {t(
                invoiceTaxModes.find((mode) => mode.value === quote.data.mode)
                  ?.label ?? quote.data.mode
              )}
            </dd>
            <dt>{t('Rate (%)')}</dt>
            <dd>{quote.data.rate_bps / 100}%</dd>
            <dt>{t('Net amount (CNY)')}</dt>
            <dd>{(quote.data.base_cents / 100).toFixed(2)}</dd>
            <dt>{t('Additional payment (CNY)')}</dt>
            <dd>{(quote.data.fee_cents / 100).toFixed(2)}</dd>
            <dt>{t('Invoice total (CNY)')}</dt>
            <dd>{(quote.data.total_cents / 100).toFixed(2)}</dd>
          </dl>
          {quote.data.fee_cents > 0 && (
            <>
              <Select
                value={method}
                onValueChange={(value) => {
                  if (value) setPayment(value)
                }}
              >
                <SelectTrigger
                  className='w-full'
                  aria-label={t('Payment Method')}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem
                    value='balance'
                    disabled={!quote.data.can_balance}
                  >
                    {t('Pay from balance')}
                  </SelectItem>
                  <SelectItem value='online'>
                    {t('Pay full tax charge online')}
                  </SelectItem>
                </SelectContent>
              </Select>
              {!quote.data.can_balance && (
                <p className='text-sm text-amber-700'>
                  {t(
                    'Balance payment is unavailable or insufficient. Pay the full tax charge online; split payments are not supported.'
                  )}
                </p>
              )}
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Tax payments rejected with the invoice are refunded to your account balance. CNY 1 equals one balance unit.'
                )}
              </p>
            </>
          )}
        </>
      )}
    </section>
  )
}
