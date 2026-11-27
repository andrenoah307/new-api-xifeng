import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Mail, RefreshCw, Send, Square, Trash2, Upload } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

type Item = {
  ticket_id: number
  subject: string
  recipient: string
  files: string[]
  state: string
  reason: string
}
type Batch = {
  queue?: {
    status: string
    position: number
    items: unknown[]
    next_at: number
    reason?: string
  }
  id: string
  expires_at: number
  cleaned_at: number
  cancelled_at: number
  retention_hours: number
  reply: string
  status: number
  items: Item[]
  ignored: string[]
}
type Recent = { id: string; created_at: number }

export function InvoiceMailBatch() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const [open, setOpen] = useState(false)
  const [batch, setBatch] = useState<Batch | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [cancelConfirmation, setCancelConfirmation] = useState(false)
  const [confirmation, setConfirmation] = useState<{
    ticket?: number
    review?: string
  } | null>(null)
  const client = useQueryClient()
  const allowed = user?.role === 3 || user?.role === 100
  const activeQueue = ['queued', 'running'].includes(batch?.queue?.status ?? '')
  const batchId = batch?.id
  useEffect(() => {
    if (!open || !allowed || !batchId || !activeQueue || busy) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    const poll = async () => {
      try {
        const response = await api.get(`/api/ticket/invoice-mail/${batchId}`, {
          disableDuplicate: true,
          signal: controller.signal,
        })
        if (!response.data.success) throw new Error('batch')
        if (!controller.signal.aborted) {
          setBatch((previous) =>
            previous?.id === batchId ? response.data.data : previous
          )
          if (
            !['queued', 'running'].includes(response.data.data.queue?.status)
          ) {
            void client.invalidateQueries({ queryKey: ['tickets'] })
          }
        }
      } catch {
        if (!controller.signal.aborted) setError(t('Invoice batch unavailable'))
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(poll, 3000)
      }
    }
    timer = setTimeout(poll, 3000)
    return () => {
      controller.abort()
      clearTimeout(timer)
    }
  }, [open, allowed, batchId, activeQueue, busy, client, t])
  const recent = useQuery({
    queryKey: ['invoice-mail-batches', user?.id, user?.role],
    enabled: open && allowed,
    gcTime: 0,
    queryFn: async () => {
      const response = await api.get('/api/ticket/invoice-mail', {
        disableDuplicate: true,
      })
      if (!response.data.success) throw new Error(response.data.message)
      return response.data.data as Recent[]
    },
  })
  const load = useMutation({
    mutationFn: async (id: string) => {
      const response = await api.get(`/api/ticket/invoice-mail/${id}`, {
        disableDuplicate: true,
      })
      if (!response.data.success) throw new Error(response.data.message)
      return response.data.data as Batch
    },
    onSuccess: (data) => {
      setBatch(data)
    },
    onError: () => setError(t('Invoice batch unavailable')),
  })
  const upload = useMutation({
    mutationFn: async (file: File) => {
      if (
        !file.name.toLowerCase().endsWith('.zip') ||
        file.size > 25 * 1024 * 1024
      ) {
        throw new Error('archive')
      }
      const form = new FormData()
      form.append('file', file)
      const response = await api.post('/api/ticket/invoice-mail', form, {
        timeout: 120000,
      })
      if (!response.data.success) throw new Error(response.data.message)
      return response.data.data as Batch
    },
    onSuccess: (data) => {
      setBatch(data)
      setError('')
      void recent.refetch()
    },
    onError: () => setError(t('Invalid invoice ZIP archive')),
  })
  const cancel = useMutation({
    mutationFn: async () => {
      if (!batch) throw new Error('batch')
      const response = await api.post(
        `/api/ticket/invoice-mail/${batch.id}/cancel`,
        {},
        { timeout: 120000 }
      )
      if (!response.data.success) throw new Error(response.data.message)
      return response.data.data as Batch
    },
    onSuccess: (data) => {
      setBatch(data)
      setError('')
      void recent.refetch()
    },
    onError: () => setError(t('Invoice batch unavailable')),
  })
  const send = async () => {
    if (!batch || !confirmation || busy) return
    const request = confirmation
    setConfirmation(null)
    setBusy(true)
    setError('')
    try {
      const response = await api.post(
        `/api/ticket/invoice-mail/${batch.id}${request.ticket ? `/${request.ticket}` : ''}/send`,
        { review: request.review ?? '' },
        { timeout: 60000 }
      )
      if (!response.data.success) throw new Error(response.data.message)
      if (response.data.data) setBatch(response.data.data)
    } catch {
      setError(t('Invoice delivery failed; refresh the batch before retrying'))
    } finally {
      await load.mutateAsync(batch.id).catch(() => {})
      setBusy(false)
      void client.invalidateQueries({ queryKey: ['tickets'] })
    }
  }
  const pause = async () => {
    if (!batch || busy) return
    setBusy(true)
    try {
      const response = await api.post(
        `/api/ticket/invoice-mail/${batch.id}/pause`,
        {},
        { timeout: 120000 }
      )
      if (!response.data.success) throw new Error('batch')
      setBatch(response.data.data)
    } catch {
      setError(t('Invoice batch unavailable'))
    } finally {
      setBusy(false)
    }
  }
  if (!allowed) return null
  const locked = busy || upload.isPending || load.isPending || cancel.isPending
  const inactive =
    !!batch &&
    (batch.cancelled_at > 0 ||
      batch.cleaned_at > 0 ||
      batch.expires_at * 1000 <= Date.now())
  const ready =
    batch?.items.filter((item) => ['ready', 'failed'].includes(item.state))
      .length ?? 0
  const labels: Record<string, string> = {
    ready: t('Ready to send'),
    failed: t('Failed'),
    sent: t('Sent'),
    sending: t('Sending'),
    unknown: t('Delivery needs review'),
    blocked: t('Blocked'),
    queued: t('Queued'),
    running: t('Sending'),
    paused: t('Paused'),
    completed: t('Completed'),
    'SMTP retry scheduled': t('SMTP retry scheduled'),
  }
  return (
    <>
      <Button variant='outline' size='sm' onClick={() => setOpen(true)}>
        <Upload size={16} />
        {t('Batch invoice upload')}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!locked) setOpen(value)
        }}
      >
        <DialogContent className='max-h-[90dvh] overflow-y-auto sm:max-w-4xl'>
          <DialogHeader>
            <DialogTitle>{t('Batch invoice upload')}</DialogTitle>
          </DialogHeader>
          <div className='grid grid-cols-[minmax(0,1fr)_auto] items-end gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]'>
            <label className='col-span-2 min-w-0 space-y-2 sm:col-span-1'>
              <span>{t('Invoice ZIP (up to 25 MB)')}</span>
              <Input
                type='file'
                accept='.zip,application/zip'
                disabled={locked}
                onChange={(event) => {
                  const file = event.target.files?.[0]
                  if (file) upload.mutate(file)
                  event.target.value = ''
                }}
              />
            </label>
            <label className='min-w-0 space-y-2'>
              <span>{t('Recent invoice batches')}</span>
              <select
                className='bg-background h-9 w-full rounded-md border px-2 text-sm'
                value={batch?.id ?? ''}
                disabled={locked}
                onChange={(event) => {
                  if (event.target.value) load.mutate(event.target.value)
                }}
              >
                <option value=''>{t('Select')}</option>
                {recent.data?.map((row) => (
                  <option key={row.id} value={row.id}>
                    {new Date(row.created_at * 1000).toLocaleString()} ·{' '}
                    {row.id.slice(0, 8)}
                  </option>
                ))}
              </select>
            </label>
            <Button
              variant='outline'
              size='icon'
              title={t('Refresh')}
              aria-label={t('Refresh')}
              disabled={locked || !batch}
              onClick={() => batch && load.mutate(batch.id)}
            >
              <RefreshCw size={16} />
            </Button>
          </div>
          {(error || recent.isError) && (
            <p role='alert' className='text-destructive text-sm'>
              {error || t('Invoice batch unavailable')}
            </p>
          )}
          {upload.isPending && <p role='status'>{t('Loading...')}</p>}
          {batch && (
            <>
              <div
                className='flex flex-wrap justify-between gap-2 text-sm'
                aria-live='polite'
              >
                <span>
                  {t('Invoices')}: {batch.items.length} · {t('Attachments')}:{' '}
                  {batch.items.reduce(
                    (sum, item) => sum + item.files.length,
                    0
                  )}{' '}
                  · {t('Sent')}:{' '}
                  {batch.items.filter((item) => item.state === 'sent').length}
                </span>
                <span>
                  {t('Expires at')}:{' '}
                  {new Date(batch.expires_at * 1000).toLocaleString()}
                </span>
              </div>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Retention: {{hours}} hours. Completed batches are cleaned immediately; expired files are checked every 10 minutes.',
                  { hours: batch.retention_hours }
                )}
              </p>
              {batch.cancelled_at > 0 && (
                <p role='status' className='text-sm'>
                  {t('Batch cancelled')}
                </p>
              )}
              {batch.queue && (
                <p role='status' className='text-sm'>
                  {t('Invoice queue')}: {labels[batch.queue.status]} ·{' '}
                  {batch.queue.position}/{batch.queue.items.length}
                  {activeQueue && (
                    <>
                      {' '}
                      · {t('Next attempt')}:{' '}
                      {new Date(batch.queue.next_at * 1000).toLocaleString()}
                    </>
                  )}
                  {batch.queue.reason &&
                    (activeQueue ||
                      batch.queue.reason !== 'SMTP retry scheduled') && (
                      <>
                        {' '}
                        · {labels[batch.queue.reason] ?? t(batch.queue.reason)}
                      </>
                    )}
                </p>
              )}
              {inactive && batch.cleaned_at === 0 && (
                <p role='status' className='text-sm'>
                  {t('Cleanup pending; sending is disabled')}
                </p>
              )}
              <div className='max-h-[40dvh] overflow-auto border-y'>
                <div className='divide-y sm:hidden'>
                  {batch.items.map((item) => (
                    <div
                      key={item.ticket_id}
                      className='space-y-2 py-3 text-sm'
                    >
                      <div className='flex flex-wrap justify-between gap-2 font-medium'>
                        <span>#{item.ticket_id}</span>
                        <span>{labels[item.state] ?? item.state}</span>
                      </div>
                      <p className='break-words'>{item.subject}</p>
                      <p className='break-all'>{item.recipient || '-'}</p>
                      <details>
                        <summary>
                          {t('Attachments')}: {item.files.length}
                        </summary>
                        {item.files.map((name) => (
                          <div key={name} className='break-all'>
                            {name}
                          </div>
                        ))}
                      </details>
                      {item.reason && item.state !== 'sent' && (
                        <p className='text-destructive'>{t(item.reason)}</p>
                      )}
                      {item.state === 'unknown' && (
                        <div className='flex flex-wrap gap-2'>
                          <Button
                            size='sm'
                            variant='outline'
                            disabled={locked || activeQueue}
                            onClick={() =>
                              setConfirmation({
                                ticket: item.ticket_id,
                                review: 'delivered',
                              })
                            }
                          >
                            <Mail size={14} />
                            {t('Confirm delivered')}
                          </Button>
                          <Button
                            size='sm'
                            variant='outline'
                            disabled={locked || inactive || activeQueue}
                            onClick={() =>
                              setConfirmation({
                                ticket: item.ticket_id,
                                review: 'not_delivered',
                              })
                            }
                          >
                            <Send size={14} />
                            {t('Confirm not delivered and retry')}
                          </Button>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
                <table className='hidden w-full text-left text-sm sm:table'>
                  <thead>
                    <tr className='border-b'>
                      <th className='p-2'>{t('Ticket')}</th>
                      <th className='p-2'>{t('Recipient')}</th>
                      <th className='p-2'>{t('Attachments')}</th>
                      <th className='p-2'>{t('Status')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {batch.items.map((item) => (
                      <tr key={item.ticket_id} className='border-b align-top'>
                        <td className='max-w-48 p-2 break-words'>
                          #{item.ticket_id}
                          <div>{item.subject}</div>
                        </td>
                        <td className='max-w-56 p-2 break-all'>
                          {item.recipient || '-'}
                        </td>
                        <td className='p-2'>
                          <details>
                            <summary className='cursor-pointer'>
                              {item.files.length}
                            </summary>
                            {item.files.map((name) => (
                              <div key={name} className='max-w-48 break-all'>
                                {name}
                              </div>
                            ))}
                          </details>
                        </td>
                        <td className='max-w-64 p-2'>
                          <span>{labels[item.state] ?? item.state}</span>
                          {item.reason && item.state !== 'sent' && (
                            <div className='text-destructive'>
                              {t(item.reason)}
                            </div>
                          )}
                          {item.state === 'unknown' && (
                            <div className='mt-2 flex flex-wrap gap-2'>
                              <Button
                                size='sm'
                                variant='outline'
                                disabled={locked || activeQueue}
                                onClick={() =>
                                  setConfirmation({
                                    ticket: item.ticket_id,
                                    review: 'delivered',
                                  })
                                }
                              >
                                <Mail size={14} />
                                {t('Confirm delivered')}
                              </Button>
                              <Button
                                size='sm'
                                variant='outline'
                                disabled={locked || inactive || activeQueue}
                                onClick={() =>
                                  setConfirmation({
                                    ticket: item.ticket_id,
                                    review: 'not_delivered',
                                  })
                                }
                              >
                                <Send size={14} />
                                {t('Confirm not delivered and retry')}
                              </Button>
                            </div>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <dl className='space-y-2 text-sm'>
                <dt className='font-medium'>{t('Reply after delivery')}</dt>
                <dd className='break-words whitespace-pre-wrap'>
                  {batch.reply}
                </dd>
                <dt className='font-medium'>
                  {t('Ticket status after delivery')}
                </dt>
                <dd>
                  {batch.status === 3 ? t('Resolved') : t('Processing')} ·{' '}
                  {t('Issued')}
                </dd>
              </dl>
              {batch.ignored?.length > 0 && (
                <details>
                  <summary>
                    {t('Ignored files')}: {batch.ignored.length}
                  </summary>
                  <ul className='max-h-32 overflow-auto text-sm'>
                    {batch.ignored.map((name) => (
                      <li key={name} className='break-all'>
                        {name}
                      </li>
                    ))}
                  </ul>
                </details>
              )}
              {batch.cleaned_at > 0 && (
                <p role='status' className='text-sm'>
                  {t('Temporary invoice files deleted')}
                </p>
              )}
              <div className='flex flex-wrap justify-end gap-2'>
                {batch.cleaned_at === 0 && (
                  <Button
                    variant='outline'
                    disabled={locked}
                    onClick={() => setCancelConfirmation(true)}
                  >
                    <Trash2 size={16} />
                    {t('Cancel batch and clean files')}
                  </Button>
                )}
                {activeQueue ? (
                  <Button
                    variant='outline'
                    disabled={locked}
                    onClick={() => void pause()}
                  >
                    <Square size={16} />
                    {t('Stop')}
                  </Button>
                ) : (
                  <Button
                    disabled={locked || inactive || ready === 0}
                    onClick={() => setConfirmation({})}
                  >
                    <Send size={16} />
                    {t('Send invoices')} ({ready})
                  </Button>
                )}
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={cancelConfirmation}
        onOpenChange={setCancelConfirmation}
        title={t('Cancel batch and clean files')}
        desc={t(
          'Disable further sending and delete this batch’s files? Sent emails cannot be recalled. Delivery records are kept; an in-progress email may finish.'
        )}
        handleConfirm={() => {
          setCancelConfirmation(false)
          cancel.mutate()
        }}
        confirmText={t('Confirm')}
      />
      <ConfirmDialog
        open={confirmation !== null}
        onOpenChange={(value) => {
          if (!value) setConfirmation(null)
        }}
        title={t('Confirm invoice delivery')}
        desc={
          confirmation?.review
            ? t(
                'Verify the SMTP delivery record before confirming. This action updates the invoice and ticket.'
              )
            : t(
                'Send attachments to the previewed recipients and update these tickets?'
              )
        }
        handleConfirm={() => void send()}
        confirmText={t('Confirm')}
      />
    </>
  )
}
