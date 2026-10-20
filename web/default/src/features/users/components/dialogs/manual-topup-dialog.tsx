import { useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
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
import { useAuthStore } from '@/stores/auth-store'

import type { User } from '../../types'
import { useUsers } from '../users-provider'

interface ReceiptQuote {
  total_cents: number
  net_cents: number
  tax_cents: number
  credit_amount: string
  token: string
}
interface ReceiptDraft {
  user_id: number
  total_cents: number
  invoice_type: string
  request_id: string
  token: string
  quote: ReceiptQuote
}

export function ManualTopupDialog({
  user,
  onClose,
}: {
  user: User
  onClose: () => void
}) {
  const { t } = useTranslation()
  const actor = useAuthStore((state) => state.auth.user?.id)
  const storageKey = `manual-topup:${actor}:${user.id}`
  const [pending, setPending] = useState<ReceiptDraft | null>(() => {
    try {
      return JSON.parse(localStorage.getItem(storageKey) || 'null')
    } catch {
      return null
    }
  })
  const requestID = useRef(pending?.request_id || crypto.randomUUID())
  const [amount, setAmount] = useState(
    pending ? (pending.total_cents / 100).toFixed(2) : ''
  )
  const [invoiceType, setInvoiceType] = useState(
    pending?.invoice_type || 'none'
  )
  const [quote, setQuote] = useState<ReceiptQuote | null>(
    pending?.quote || null
  )
  const [busy, setBusy] = useState(false)
  const running = useRef(false)
  const { triggerRefresh } = useUsers()
  const queryClient = useQueryClient()
  const cents = Math.round(Number(amount) * 100)
  const valid =
    /^\d+(\.\d{1,2})?$/.test(amount) && Number.isSafeInteger(cents) && cents > 0
  const invoiceLabels: Record<string, string> = {
    none: '-', ordinary: t('Ordinary VAT invoice'), special: t('Special VAT invoice'),
  }
  let submitLabel = t('Continue')
  if (quote) submitLabel = t('Confirm')
  if (pending) submitLabel = t('Retry')

  const preview = async () => {
    if (!valid || running.current) return
    running.current = true
    setBusy(true)
    try {
      const { data } = await api.post('/api/user/topup/manual/quote', {
        user_id: user.id,
        total_cents: cents,
        invoice_type: invoiceType,
      })
      if (data.success) setQuote(data.data)
    } finally {
      running.current = false
      setBusy(false)
    }
  }
  const save = async () => {
    if (!quote || running.current) return
    running.current = true
    setBusy(true)
    try {
      const draft = pending || {
        user_id: user.id,
        total_cents: cents,
        invoice_type: invoiceType,
        request_id: requestID.current,
        token: quote.token,
        quote,
      }
      // Persist before sending so a lost response or reopened dialog reuses the same receipt.
      localStorage.setItem(storageKey, JSON.stringify(draft))
      setPending(draft)
      const { data } = await api.post('/api/user/topup/manual', draft)
      if (!data.success) {
        setPending(null)
        setQuote(null)
        return
      }
      localStorage.removeItem(storageKey)
      toast.success(t('Operation successful'))
      triggerRefresh()
      queryClient.invalidateQueries({ queryKey: ['topup-history'] })
      queryClient.invalidateQueries({ queryKey: ['finance'] })
      onClose()
    } catch {
      toast.error(t('Request failed'))
    } finally {
      running.current = false
      setBusy(false)
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose()
      }}
    >
      <DialogContent
        className='max-h-[85dvh] overflow-y-auto sm:max-w-lg'
        showCloseButton={!busy}
      >
        <DialogHeader>
          <DialogTitle>{t('Manual top-up')}</DialogTitle>
          <DialogDescription>
            {user.username} (ID: {user.id})
          </DialogDescription>
        </DialogHeader>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Record a payment already received. Tax is deducted from the payment amount; the remainder is credited without promotions.'
          )}
        </p>
        <div className='space-y-2'>
          <Label htmlFor='manual-topup-amount'>
            {t('Payment amount (CNY)')}
          </Label>
          <Input
            id='manual-topup-amount'
            type='number'
            min='0.01'
            step='0.01'
            value={amount}
            disabled={!!quote || busy}
            onChange={(event) => setAmount(event.target.value)}
          />
        </div>
        <fieldset
          disabled={!!quote || busy}
          className='grid grid-cols-[repeat(auto-fit,minmax(min(100%,180px),1fr))] gap-3'
        >
          <legend className='mb-2 font-medium'>{t('Invoice type')}</legend>
          {['none', 'ordinary', 'special'].map((kind) => (
            <label
              key={kind}
              className='border-input hover:bg-accent has-[:checked]:border-primary has-[:checked]:bg-primary/10 has-[:checked]:text-primary has-[:focus-visible]:ring-ring flex min-h-16 min-w-0 cursor-pointer items-center gap-3 rounded-lg border p-4 transition-colors has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50 has-[:focus-visible]:ring-2'
            >
              <input
                className='accent-primary size-4 shrink-0'
                type='radio'
                name='manual-invoice-type'
                value={kind}
                checked={invoiceType === kind}
                onChange={() => setInvoiceType(kind)}
              />
              {invoiceLabels[kind]}
            </label>
          ))}
        </fieldset>
        {quote && (
          <dl className='grid grid-cols-2 gap-2 text-sm'>
            <dt>{t('Payment amount (CNY)')}</dt>
            <dd className='text-right'>
              {(quote.total_cents / 100).toFixed(2)}
            </dd>
            <dt>{t('Tax charge (CNY)')}</dt>
            <dd className='text-right'>{(quote.tax_cents / 100).toFixed(2)}</dd>
            <dt>{t('Balance credited')}</dt>
            <dd className='text-right font-medium'>{quote.credit_amount}</dd>
          </dl>
        )}
        <DialogFooter>
          {quote && !pending && (
            <Button
              variant='outline'
              disabled={busy}
              onClick={() => setQuote(null)}
            >
              {t('Back')}
            </Button>
          )}
          <Button
            disabled={busy || !valid}
            onClick={() => {
              void (quote ? save() : preview()).catch(() => {})
            }}
          >
            {submitLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
