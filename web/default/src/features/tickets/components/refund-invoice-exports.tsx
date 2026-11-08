import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

export function RefundInvoiceExports({ ticketId }: { ticketId: number }) {
  const role = useAuthStore((state) => state.auth.user?.role)
  if (![3, 10, 100].includes(role ?? 0)) return null
  return <RefundInvoiceExportPanel key={ticketId} ticketId={ticketId} />
}
function RefundInvoiceExportPanel({ ticketId }: { ticketId: number }) {
  const { t } = useTranslation()
  const [amount, setAmount] = useState('')
  const [saved, setSaved] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    let active = true
    setAmount('')
    setSaved(false)
    api
      .get(`/api/ticket/refund/${ticketId}/invoice-export`, {
        skipBusinessError: true,
      })
      .then(({ data }) => {
        if (active && data.success) {
          setAmount((data.data.cash_cents / 100).toFixed(2))
          setSaved(true)
        }
      })
      .catch(() => {})
    return () => {
      active = false
    }
  }, [ticketId])
  async function recalculate() {
    if (busy) return
    setBusy(true)
    setError('')
    setSaved(false)
    try {
      const { data } = await api.post(
        `/api/ticket/refund/${ticketId}/invoice-export`,
        {}
      )
      if (!data.success) throw new Error(data.message)
      setAmount((data.data.cash_cents / 100).toFixed(2))
      setSaved(true)
    } catch (e) {
      setError(e instanceof Error ? t(e.message) : t('Operation failed'))
    } finally {
      setBusy(false)
    }
  }
  async function download(kind: string) {
    if (busy || !saved) return
    setBusy(true)
    setError('')
    try {
      const response = await api.get(
        `/api/ticket/refund/${ticketId}/invoice-export`,
        { params: { kind }, responseType: 'blob' }
      )
      if (
        !String(response.headers['content-type'] || '').includes('text/csv')
      ) {
        const result = JSON.parse(await response.data.text())
        throw new Error(result.message || t('Operation failed'))
      }
      const url = URL.createObjectURL(response.data)
      const link = document.createElement('a')
      link.href = url
      link.download = `refund-${ticketId}-${kind}.csv`
      link.click()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (e) {
      setError(e instanceof Error ? t(e.message) : t('Operation failed'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <section className='space-y-3 border-t pt-4'>
      <h3>{t('Refund invoice exports')}</h3>
      <p>
        {t('Cash refund')}: {saved ? `¥${amount}` : '-'}
      </p>
      <div className='flex flex-wrap gap-2'>
        <Button disabled={busy} onClick={() => void recalculate()}>
          {t('Recalculate')}
        </Button>
        {['history', 'red', 'reissue'].map((kind, i) => (
          <Button
            key={kind}
            disabled={!saved || busy}
            onClick={() => void download(kind)}
          >
            {t(
              [
                'Export issued invoices',
                'Export invoices to reverse',
                'Export recalculated reissued invoices',
              ][i]
            )}
          </Button>
        ))}
      </div>
      {error && <p role='alert'>{error}</p>}
    </section>
  )
}
