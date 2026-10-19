import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CreditCard } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { api } from '@/lib/api'
import { submitPaymentForm } from '../lib'

interface Quote {
  total_cents: number
  net_cents: number
  tax_cents: number
  credit_amount: string
  token: string
}

export function EpayCheckout({
  methods,
  minimum,
  discountsEnabled,
}: {
  methods: { type: string; name: string }[]
  minimum: number
  discountsEnabled: boolean
}) {
  const { t } = useTranslation()
  const [amount, setAmount] = useState(String(minimum))
  const [settledAmount, setSettledAmount] = useState<string | null>(null)
  useEffect(() => {
    const timer = setTimeout(() => setSettledAmount(amount), 500)
    return () => clearTimeout(timer)
  }, [amount])
  const isTyping = amount !== settledAmount
  const [invoiceType, setInvoiceType] = useState('none')
  const [method, setMethod] = useState(methods[0]?.type ?? 'alipay')
  const [discount, setDiscount] = useState('')
  const [discountInput, setDiscountInput] = useState('')
  const attempt = useRef({ token: '', id: '' })
  const cents = /^\d+(\.\d{1,2})?$/.test(amount)
    ? Math.round(Number(amount) * 100)
    : 0
  const request = {
    total_cents: cents,
    invoice_type: invoiceType,
    payment_method: method,
    discount_code: discount.trim(),
  }
  const quote = useQuery({
    queryKey: ['epay-checkout', request],
    queryFn: async ({ signal }) => {
      const { data } = await api.post(
        '/api/user/epay/checkout/quote',
        request,
        { signal, skipErrorHandler: true }
      )
      if (!data.success) throw new Error(data.message)
      return data.data as Quote
    },
    enabled: !isTyping && cents >= minimum * 100 && cents > 0,
    retry: false,
    gcTime: 0,
  })
  const pay = useMutation({
    mutationFn: async () => {
      if (isTyping || cents <= 0 || cents < minimum * 100 || !quote.data || quote.isFetching || quote.isError || discountInput.trim() !== discount) return
      if (attempt.current.token !== quote.data.token) {
        attempt.current = { token: quote.data.token, id: crypto.randomUUID() }
      }
      const { data } = await api.post('/api/user/epay/checkout/pay', {
        ...request,
        token: quote.data.token,
        request_id: attempt.current.id,
      })
      if (!data.success) throw new Error(data.message)
      submitPaymentForm(data.data.url, data.data.params)
    },
    onError: () => {
      toast.error(t('Payment request failed'))
      quote.refetch()
    },
  })
  return (
    <section className='space-y-4 border-b pb-5'>
      <div className='grid gap-4 sm:grid-cols-2'>
        <div className='space-y-2'>
          <Label htmlFor='epay-payment-total'>
            {t('Payment amount (CNY)')}
          </Label>
          <Input
            id='epay-payment-total'
            type='number'
            min={minimum}
            step='0.01'
            value={amount}
            onChange={(event) => {
              setSettledAmount(null)
              setAmount(event.target.value)
            }}
            disabled={pay.isPending}
          />
        </div>
        <div className='space-y-2'>
          <Label>{t('Invoice type')}</Label>
          <Select
            value={invoiceType}
            onValueChange={(value) => value && setInvoiceType(value)}
            disabled={pay.isPending}
          >
            <SelectTrigger className='w-full' aria-label={t('Invoice type')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='none'>{t('No invoice')}</SelectItem>
              <SelectItem value='ordinary'>
                {t('Ordinary VAT invoice')}
              </SelectItem>
              <SelectItem value='special'>
                {t('Special VAT invoice')}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        {discountsEnabled && (
          <div className='space-y-2'>
            <Label htmlFor='epay-discount'>{t('Discount Code')}</Label>
            <Input
              id='epay-discount'
              value={discountInput}
              onChange={(event) => setDiscountInput(event.target.value)}
              onBlur={() => setDiscount(discountInput.trim())}
              disabled={pay.isPending}
            />
          </div>
        )}
        <div className='space-y-2'>
          <Label>{t('Payment Method')}</Label>
          <Select
            value={method}
            onValueChange={(value) => value && setMethod(value)}
            disabled={pay.isPending}
          >
            <SelectTrigger className='w-full' aria-label={t('Payment Method')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {methods.map((item) => (
                <SelectItem key={item.type} value={item.type}>
                  {item.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Invoice eligibility and type are fixed at checkout. Tax is included in the payment amount.'
        )}
      </p>
      {(isTyping || quote.isFetching) && <p>{t('Loading...')}</p>}
      {!isTyping && !quote.isFetching && quote.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t(
            'Tax quote unavailable. Refresh the selected orders and try again.'
          )}
        </p>
      )}
      {!isTyping && !quote.isFetching && !quote.isError && quote.data && cents > 0 && (
        <dl className='grid grid-cols-2 gap-x-4 gap-y-2 text-sm'>
          <dt>{t('Balance credited')}</dt>
          <dd className='text-right font-medium'>{quote.data.credit_amount}</dd>
          <dt>{t('Net amount (CNY)')}</dt>
          <dd className='text-right'>
            {(quote.data.net_cents / 100).toFixed(2)}
          </dd>
          <dt>{t('Tax charge (CNY)')}</dt>
          <dd className='text-right'>
            {(quote.data.tax_cents / 100).toFixed(2)}
          </dd>
          <dt>{t('Payment amount (CNY)')}</dt>
          <dd className='text-right font-medium'>
            {(quote.data.total_cents / 100).toFixed(2)}
          </dd>
        </dl>
      )}
      <Button
        type='button'
        disabled={
          isTyping ||
          !quote.data ||
          quote.isFetching ||
          quote.isError ||
          discountInput.trim() !== discount ||
          cents < minimum * 100 ||
          cents <= 0 ||
          pay.isPending
        }
        onClick={() => pay.mutate()}
      >
        <CreditCard className='size-4' />
        {t('Pay')}
      </Button>
    </section>
  )
}
