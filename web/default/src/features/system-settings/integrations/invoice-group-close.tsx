import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'

import {
  previewInvoiceGroupClosures,
  closeInvoiceGroupTicket,
  type InvoiceClosureItem,
} from '../../../../../shared/invoice-group-close.js'

export function InvoiceGroupClose({ disabled }: { disabled: boolean }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [convert, setConvert] = useState(false)
  const [busy, setBusy] = useState(false)
  const [items, setItems] = useState<InvoiceClosureItem[] | null>(null)
  const active = useRef(true)
  const running = useRef(false)
  useEffect(() => {
    active.current = true
    return () => {
      active.current = false
    }
  }, [])
  const preview = async () => {
    if (running.current) return
    running.current = true
    setBusy(true)
    setItems(null)
    try {
      const rows = await previewInvoiceGroupClosures(api.post, convert)
      if (active.current) setItems(rows)
    } catch (error) {
      if (active.current) {
        toast.error(
          error instanceof Error ? error.message : t('Failed to load')
        )
      }
    } finally {
      running.current = false
      if (active.current) setBusy(false)
    }
  }
  const execute = async () => {
    if (running.current || !items) return
    running.current = true
    setBusy(true)
    const results = [...items]
    try {
      for (let i = 0; i < results.length && active.current; i++) {
        if (results[i].applied) continue
        try {
          results[i] = await closeInvoiceGroupTicket(
            api.post,
            results[i],
            convert
          )
        } catch (error) {
          results[i] = {
            ...results[i],
            error: error instanceof Error ? error.message : t('Failed to load'),
          }
        }
        if (active.current) setItems([...results])
      }
    } finally {
      running.current = false
      if (active.current) setBusy(false)
    }
  }
  return (
    <>
      <Button
        variant='outline'
        className='h-auto whitespace-normal'
        disabled={disabled}
        onClick={() => {
          setItems(null)
          setOpen(true)
        }}
      >
        {t('Close pending invoice tickets in selected groups')}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) setOpen(value)
        }}
      >
        <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-3xl'>
          <DialogHeader>
            <DialogTitle className='pr-6'>
              {t('Close pending invoice tickets in selected groups')}
            </DialogTitle>
          </DialogHeader>
          <DialogDescription>
            {t(
              'Close all listed tickets. Issued invoices remain unchanged. Tax conversion does not block ticket closure.'
            )}
          </DialogDescription>
          <div className='flex items-center gap-2'>
            <Checkbox
              id='convert-invoice-order-tax'
              checked={convert}
              disabled={busy}
              onCheckedChange={(value) => {
                setConvert(value === true)
                setItems(null)
              }}
            />
            <Label htmlFor='convert-invoice-order-tax'>
              {t(
                'Also convert prepaid order tax to balance and remove invoice eligibility'
              )}
            </Label>
          </div>
          <Button variant='outline' disabled={busy} onClick={preview}>
            {t('Preview')}
          </Button>
          {items && (
            <div className='overflow-x-auto'>
              <table className='w-full min-w-[640px] text-left text-sm [&_td]:px-2 [&_td]:py-2 [&_th]:px-2 [&_th]:py-2'>
                <thead>
                  <tr>
                    <th>{t('Ticket ID')}</th>
                    <th>{t('User ID')}</th>
                    <th>{t('Invoice title')}</th>
                    <th>{t('Invoice amount')}</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((item) => (
                    <tr key={item.ticket_id} className='border-t'>
                      <td>{item.ticket_id}</td>
                      <td>
                        {item.user_id} ({item.username})
                      </td>
                      <td className='max-w-64 break-words'>
                        {item.invoice_title || '-'}
                      </td>
                      <td>{(item.amount ?? 0).toFixed(2)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {items.length === 0 && <p>{t('No data')}</p>}
            </div>
          )}
          {items?.some((item) => item.applied || item.error) && (
            <div role='status' className='space-y-2 text-sm'>
              <p>
                {t('Closed')}: {items.filter((item) => item.applied).length} /{' '}
                {items.length}
              </p>
              {items
                .filter((item) => item.error || item.warning)
                .map((item) => (
                  <p key={item.ticket_id}>
                    #{item.ticket_id}: {t(item.error || item.warning || '')}
                  </p>
                ))}
            </div>
          )}
          <Button
            disabled={busy || !items?.some((item) => !item.applied)}
            onClick={execute}
          >
            {t('Close all listed tickets')}
          </Button>
        </DialogContent>
      </Dialog>
    </>
  )
}
