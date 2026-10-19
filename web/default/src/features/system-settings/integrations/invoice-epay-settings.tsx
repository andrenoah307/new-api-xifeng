import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'

interface InvoiceEpayConfig {
  address: string
  partner_id: string
  key_configured: boolean
  payment_methods: string[]
}

export function InvoiceEpaySettings() {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<InvoiceEpayConfig | null>(null)
  const [secret, setSecret] = useState('')
  const query = useQuery({
    queryKey: ['invoice-epay-config'],
    queryFn: async () => {
      const { data } = await api.get('/api/option/invoice-epay')
      if (!data.success) throw new Error(data.message)
      return data.data as InvoiceEpayConfig
    },
    gcTime: 0,
  })
  const config = draft ?? query.data
  const save = useMutation({
    mutationFn: async () => {
      const { data } = await api.put('/api/option/invoice-epay', {
        ...config,
        key: secret,
      })
      if (!data.success) throw new Error(data.message)
      return data.data as InvoiceEpayConfig
    },
    onSuccess: (value) => {
      setDraft(value)
      setSecret('')
      toast.success(t('Saved successfully'))
    },
  })
  if (!config) {
    return <p>{query.isError ? t('Failed to load') : t('Loading...')}</p>
  }
  return (
    <section className='space-y-4'>
      <h3 className='font-medium'>{t('Invoice-eligible recharge gateway')}</h3>
      <div className='grid gap-4 sm:grid-cols-2'>
        <div className='space-y-2'>
          <Label htmlFor='invoice-epay-address'>{t('Epay endpoint')}</Label>
          <Input
            id='invoice-epay-address'
            value={config.address}
            onChange={(event) =>
              setDraft({ ...config, address: event.target.value })
            }
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='invoice-epay-id'>{t('Epay merchant ID')}</Label>
          <Input
            id='invoice-epay-id'
            value={config.partner_id}
            onChange={(event) =>
              setDraft({ ...config, partner_id: event.target.value })
            }
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='invoice-epay-key'>{t('Epay secret key')}</Label>
          <Input
            id='invoice-epay-key'
            type='password'
            autoComplete='new-password'
            value={secret}
            onChange={(event) => setSecret(event.target.value)}
            placeholder={
              config.key_configured ? t('Enter new key to update') : ''
            }
          />
        </div>
      </div>
      <fieldset className='space-y-2' disabled={save.isPending}>
        <legend className='mb-2 font-medium'>{t('Payment Method')}</legend>
        {['alipay', 'wxpay'].map((method) => (
          <label key={method} className='flex items-center gap-2'>
            <input
              type='checkbox'
              checked={config.payment_methods.includes(method)}
              onChange={(event) =>
                setDraft({
                  ...config,
                  payment_methods: event.target.checked
                    ? [...config.payment_methods, method]
                    : config.payment_methods.filter(
                        (value) => value !== method
                      ),
                })
              }
            />
            {method === 'alipay' ? t('Alipay') : t('WeChat Pay')}
          </label>
        ))}
      </fieldset>
      <Button
        type='button'
        disabled={
          save.isPending ||
          !config.address ||
          !config.partner_id ||
          (!config.key_configured && !secret)
        }
        onClick={() => save.mutate()}
      >
        {t('Save')}
      </Button>
    </section>
  )
}
