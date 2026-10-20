import { useMutation, useQuery } from '@tanstack/react-query'
import { CreditCard } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'

import { getPaymentIcon, submitPaymentForm } from '../lib'

interface Quote {
  total_cents: number
  net_cents: number
  tax_cents: number
  credit_amount: string
  token: string
}

export function EpayCheckout({
  minimum,
  discountsEnabled,
}: {
  minimum: number
  discountsEnabled: boolean
}) {
  const { t } = useTranslation()
  const [amount, setAmount] = useState(String(minimum))
  const [settledAmount, setSettledAmount] = useState<string | null>(null)
  const [step, setStep] = useState(0)
  const [invoiceType, setInvoiceType] = useState('none')
  const [method, setMethod] = useState('')
  const [discount, setDiscount] = useState('')
  const attempt = useRef({ token: '', id: '' })
  const submitting = useRef(false)
  useEffect(() => {
    const timer = setTimeout(() => setSettledAmount(amount), 500)
    return () => clearTimeout(timer)
  }, [amount])
  const cents = /^\d+(\.\d{1,2})?$/.test(amount)
    ? Math.round(Number(amount) * 100)
    : 0
  const validAmount =
    amount === settledAmount && cents > 0 && cents >= minimum * 100
  const methods = useQuery({
    queryKey: ['epay-checkout-methods', invoiceType],
    queryFn: async ({ signal }) => {
      const { data } = await api.get('/api/user/epay/checkout/methods', {
        params: { invoice_type: invoiceType },
        signal,
        disableDuplicate: true,
        skipErrorHandler: true,
      })
      if (!data.success) throw new Error(data.message)
      return data.data as string[]
    },
    enabled: step === 2,
    retry: false,
    gcTime: 0,
    staleTime: 0,
  })
  const available = methods.data ?? []
  const selectedMethod = available.includes(method)
    ? method
    : (available[0] ?? '')
  const methodsReady =
    step === 2 && !methods.isFetching && !methods.isError && !!selectedMethod
  const request = {
    total_cents: cents,
    invoice_type: invoiceType,
    payment_method: selectedMethod,
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
    enabled: methodsReady && validAmount,
    retry: false,
    gcTime: 0,
  })
  const canPay =
    methodsReady &&
    validAmount &&
    !!quote.data &&
    !quote.isFetching &&
    !quote.isError
  const pay = useMutation({
    mutationFn: async () => {
      if (!canPay || !quote.data || submitting.current) return
      submitting.current = true
      try {
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
      } finally {
        submitting.current = false
      }
    },
    onError: () => {
      toast.error(t('Payment request failed'))
      methods.refetch()
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
            disabled={step !== 0}
          />
        </div>
        {discountsEnabled && (
          <div className='space-y-2'>
            <Label htmlFor='epay-discount'>{t('Discount Code')}</Label>
            <Input
              id='epay-discount'
              value={discount}
              onChange={(event) => setDiscount(event.target.value)}
              disabled={step !== 0}
            />
          </div>
        )}
      </div>
      <Button
        type='button'
        className='w-full'
        disabled={!validAmount}
        onClick={() => setStep(1)}
      >
        {t('Continue')}
      </Button>
      <Dialog
        open={step !== 0}
        onOpenChange={(open) => {
          if (!open && !pay.isPending) setStep(0)
        }}
      >
        <DialogContent
          className='max-h-[85dvh] overflow-y-auto sm:max-w-lg'
          showCloseButton={!pay.isPending}
        >
          <DialogHeader>
            <DialogTitle>
              {step === 1 ? t('Invoice confirmation') : t('Confirm payment')}
            </DialogTitle>
            <DialogDescription className='leading-relaxed whitespace-pre-line'>
              {step === 1
                ? t(
                    'Choose invoice eligibility before paying. If you need an invoice, select Yes now and choose the invoice type next. Tax charges will be included in your payment and deducted when calculating the credited balance. Selecting No creates a non-invoiceable order without tax charges; this order cannot later be changed to an invoiceable order.'
                  )
                : t(
                    'Invoice eligibility and type are fixed at checkout. Tax is included in the payment amount.'
                  )}
            </DialogDescription>
          </DialogHeader>
          {step === 1 ? (
            <DialogFooter className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
              <Button
                variant='outline'
                className='h-auto min-h-12 py-3 whitespace-normal'
                onClick={() => {
                  setInvoiceType('none')
                  setMethod('')
                  setStep(2)
                }}
              >
                {t('No invoice')}
              </Button>
              <Button
                className='h-auto min-h-12 py-3 whitespace-normal'
                onClick={() => {
                  setInvoiceType('ordinary')
                  setMethod('')
                  setStep(2)
                }}
              >
                {t('Invoice required')}
              </Button>
            </DialogFooter>
          ) : (
            <>
              {invoiceType !== 'none' && (
                <fieldset
                  className='grid grid-cols-[repeat(auto-fit,minmax(min(100%,180px),1fr))] gap-3'
                  disabled={pay.isPending}
                >
                  <legend className='mb-2 font-medium'>
                    {t('Invoice type')}
                  </legend>
                  {['ordinary', 'special'].map((kind) => (
                    <label
                      key={kind}
                      className='border-input hover:bg-accent has-[:checked]:border-primary has-[:checked]:bg-primary/10 has-[:checked]:text-primary has-[:focus-visible]:ring-ring flex min-h-16 min-w-0 cursor-pointer items-center gap-3 rounded-lg border p-4 transition-colors has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50 has-[:focus-visible]:ring-2'
                    >
                      <input
                        className='accent-primary size-4 shrink-0'
                        type='radio'
                        name='checkout-invoice-type'
                        value={kind}
                        checked={invoiceType === kind}
                        onChange={() => {
                          setInvoiceType(kind)
                          setMethod('')
                        }}
                      />
                      {kind === 'ordinary'
                        ? t('Ordinary VAT invoice')
                        : t('Special VAT invoice')}
                    </label>
                  ))}
                </fieldset>
              )}
              <fieldset
                className='grid grid-cols-[repeat(auto-fit,minmax(min(100%,180px),1fr))] gap-3'
                disabled={pay.isPending || methods.isFetching}
              >
                <legend className='mb-2 font-medium'>
                  {t('Payment Method')}
                </legend>
                {methods.isFetching ? (
                  <p>{t('Loading...')}</p>
                ) : (
                  available.map((value) => (
                    <label
                      key={value}
                      className='border-input hover:bg-accent has-[:checked]:border-primary has-[:checked]:bg-primary/10 has-[:checked]:text-primary has-[:focus-visible]:ring-ring flex min-h-16 min-w-0 cursor-pointer items-center gap-3 rounded-lg border p-4 transition-colors has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50 has-[:focus-visible]:ring-2'
                    >
                      <input
                        className='accent-primary size-4 shrink-0'
                        type='radio'
                        name='checkout-payment-method'
                        value={value}
                        checked={selectedMethod === value}
                        onChange={() => setMethod(value)}
                      />
                      <span aria-hidden='true' className='shrink-0'>
                        {getPaymentIcon(value, 'size-6')}
                      </span>
                      {value === 'alipay' ? t('Alipay') : t('WeChat Pay')}
                    </label>
                  ))
                )}
              </fieldset>
              {!methods.isFetching &&
                (methods.isError || available.length === 0) && (
                  <p role='alert'>
                    {t(
                      'No payment methods are available for this invoice choice.'
                    )}
                  </p>
                )}
              {quote.isFetching && <p>{t('Loading...')}</p>}
              {quote.isError && (
                <p role='alert'>
                  {t(
                    'Tax quote unavailable. Refresh the selected orders and try again.'
                  )}
                </p>
              )}
              {canPay && quote.data && (
                <dl className='grid grid-cols-2 gap-2 text-sm'>
                  <dt>{t('Balance credited')}</dt>
                  <dd className='text-right'>{quote.data.credit_amount}</dd>
                  <dt>{t('Net amount (CNY)')}</dt>
                  <dd className='text-right'>
                    {(quote.data.net_cents / 100).toFixed(2)}
                  </dd>
                  {invoiceType !== 'none' && (
                    <>
                      <dt>{t('Tax charge (CNY)')}</dt>
                      <dd className='text-right'>
                        {(quote.data.tax_cents / 100).toFixed(2)}
                      </dd>
                    </>
                  )}
                  <dt>{t('Payment amount (CNY)')}</dt>
                  <dd className='text-right font-medium'>
                    {(quote.data.total_cents / 100).toFixed(2)}
                  </dd>
                </dl>
              )}
              <DialogFooter>
                <Button
                  variant='outline'
                  disabled={pay.isPending}
                  onClick={() => setStep(1)}
                >
                  {t('Back')}
                </Button>
                {(methods.isError || quote.isError) && (
                  <Button
                    variant='outline'
                    onClick={() => {
                      methods.refetch()
                      quote.refetch()
                    }}
                  >
                    {t('Retry')}
                  </Button>
                )}
                <Button
                  disabled={!canPay || pay.isPending}
                  onClick={() => pay.mutate()}
                >
                  <CreditCard className='size-4' />
                  {t('Pay')}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </section>
  )
}
