import { useQuery, useQueryClient } from '@tanstack/react-query'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { User } from '../../types'
import { useUsers } from '../users-provider'

interface ReceiptAccount {
  gateway_id: string
  gateway_name: string
  account_name: string
}
interface ReceiptQuote extends ReceiptAccount {
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
  gateway_id: string
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
  const [gatewayID, setGatewayID] = useState(pending?.gateway_id || '')
  const accounts = useQuery({
    queryKey: ['manual-topup-accounts', actor],
    queryFn: async ({ signal }) => {
      const { data } = await api.get('/api/user/topup/manual/accounts', {
        signal,
      })
      if (!data.success) throw new Error(data.message || 'Request failed')
      return data.data as ReceiptAccount[]
    },
    enabled: !pending,
  })
  const [quote, setQuote] = useState<ReceiptQuote | null>(
    pending?.quote || null
  )
  const [busy, setBusy] = useState(false)
  const running = useRef(false)
  const { triggerRefresh } = useUsers()
  const queryClient = useQueryClient()
  const cents = Math.round(Number(amount) * 100)
  const valid =
    /^\d+(\.\d{1,2})?$/.test(amount) &&
    Number.isSafeInteger(cents) &&
    cents > 0 &&
    (!!pending ||
      !!quote ||
      (invoiceType === 'none' && !gatewayID) ||
      (!accounts.isError &&
        !!accounts.data?.some((account) => account.gateway_id === gatewayID)))
  const invoiceLabels: Record<string, string> = {
    none: '-',
    ordinary: t('Ordinary VAT invoice'),
    special: t('Special VAT invoice'),
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
        gateway_id: gatewayID,
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
        gateway_id: gatewayID,
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
          {['ordinary', 'special', 'none'].map((kind) => (
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
        <div className='space-y-2'>
          <Label htmlFor='manual-topup-account'>
            {t('Corporate account name')}
          </Label>
          {quote ? (
            <p className='break-words'>
              {quote.account_name || '-'}
              {quote.gateway_name && ` (${quote.gateway_name})`}
            </p>
          ) : (
            <Select
              value={gatewayID ? `account:${gatewayID}` : null}
              onValueChange={(value) =>
                setGatewayID(
                  value?.startsWith('account:') ? value.slice(8) : ''
                )
              }
              disabled={busy || accounts.isPending || accounts.isError}
            >
              <SelectTrigger id='manual-topup-account' className='w-full'>
                <SelectValue placeholder={t('Select corporate account')} />
              </SelectTrigger>
              <SelectContent>
                {invoiceType === 'none' && (
                  <SelectItem value='none'>-</SelectItem>
                )}
                {(accounts.data || []).map((account) => (
                  <SelectItem
                    key={account.gateway_id}
                    value={`account:${account.gateway_id}`}
                  >
                    <span className='min-w-0 break-words whitespace-normal'>
                      {account.account_name} ({account.gateway_name})
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          {!quote && accounts.isPending && (
            <p className='text-sm'>{t('Loading...')}</p>
          )}
          {!quote && accounts.isError && (
            <Button variant='outline' onClick={() => void accounts.refetch()}>
              {t('Retry')}
            </Button>
          )}
          {!quote &&
            invoiceType !== 'none' &&
            accounts.isSuccess &&
            accounts.data.length === 0 && (
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Configure a corporate account in payment gateway settings first.'
                )}
              </p>
            )}
        </div>
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
