import { useMutation, useQuery } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'

interface Gateway {
  id: string
  name: string
  account_name: string
  address: string
  partner_id: string
  key_configured: boolean
  key?: string
  payment_methods: string[]
}
interface RoutingConfig extends Partial<Gateway> {
  gateways: Gateway[] | null
  strategy: string
  private_below_cents: number
  private_gateway_name: string
}
function routingDraft(
  value: RoutingConfig
): RoutingConfig & { gateways: Gateway[] } {
  return {
    gateways:
      value.gateways ??
      (value.address
        ? [
            {
              id: 'legacy',
              name: 'Epay',
              account_name: '',
              address: value.address,
              partner_id: value.partner_id ?? '',
              key_configured: !!value.key_configured,
              payment_methods: value.payment_methods ?? [],
            },
          ]
        : []),
    strategy: value.strategy || 'round_robin',
    private_gateway_name: value.private_gateway_name || 'Epay',
    private_below_cents:
      value.gateways === null ? 50000 : value.private_below_cents,
  }
}
export function InvoiceEpaySettings() {
  const { t } = useTranslation()
  const [threshold, setThreshold] = useState<string | null>(null)
  const [draft, setDraft] = useState<ReturnType<typeof routingDraft> | null>(
    null
  )
  const query = useQuery({
    queryKey: ['invoice-epay-config'],
    queryFn: async () => {
      const { data } = await api.get('/api/option/invoice-epay')
      if (!data.success) throw new Error(data.message)
      return routingDraft(data.data)
    },
    gcTime: 0,
  })
  const config = draft ?? query.data
  const thresholdText =
    threshold ?? String((config?.private_below_cents ?? 50000) / 100)
  const thresholdCents = /^\d+(\.\d{1,2})?$/.test(thresholdText)
    ? Math.round(Number(thresholdText) * 100)
    : Number.NaN
  const save = useMutation({
    mutationFn: async () => {
      const { data } = await api.put('/api/option/invoice-epay', {
        ...config,
        private_below_cents: thresholdCents,
      })
      if (!data.success) throw new Error(data.message)
      return routingDraft(data.data)
    },
    onSuccess: (value) => {
      setDraft(value)
      setThreshold(null)
      toast.success(t('Saved successfully'))
    },
  })
  if (!config) {
    return <p>{query.isError ? t('Failed to load') : t('Loading...')}</p>
  }
  const invalid =
    !config.private_gateway_name.trim() ||
    !Number.isSafeInteger(thresholdCents) ||
    thresholdCents < 0 ||
    thresholdCents > 100000000000 ||
    config.gateways.some(
      (gateway) =>
        !gateway.name.trim() ||
        !gateway.account_name.trim() ||
        !gateway.address.trim() ||
        !gateway.partner_id.trim() ||
        (!gateway.key_configured && !gateway.key?.trim())
    )
  const updateGateway = (id: string, patch: Partial<Gateway>) =>
    setDraft({
      ...config,
      gateways: config.gateways.map((gateway) =>
        gateway.id === id ? { ...gateway, ...patch } : gateway
      ),
    })
  const moveGateway = (index: number, offset: number) => {
    const gateways = [...config.gateways]
    const current = gateways[index]
    const other = gateways[index + offset]
    if (!current || !other) {
      return
    }
    gateways[index] = other
    gateways[index + offset] = current
    setDraft({ ...config, gateways })
  }
  return (
    <section className='space-y-4'>
      <h3 className='font-medium'>{t('Invoice payment routing')}</h3>
      <fieldset disabled={save.isPending} className='min-w-0 space-y-4'>
        <label className='block space-y-2 text-sm'>
          <span>{t('Private payment gateway name')}</span>
          <Input
            maxLength={200}
            value={config.private_gateway_name}
            onChange={(event) =>
              setDraft({ ...config, private_gateway_name: event.target.value })
            }
          />
        </label>
        <div className='grid gap-4 sm:grid-cols-2'>
          <label className='space-y-2 text-sm'>
            <span>{t('Corporate gateway threshold (CNY)')}</span>
            <Input
              type='number'
              min='0'
              max='1000000000'
              step='0.01'
              value={thresholdText}
              onChange={(event) => setThreshold(event.target.value)}
            />
          </label>
          <label className='space-y-2 text-sm'>
            <span>{t('Routing strategy')}</span>
            <select
              className='border-input bg-background h-9 w-full rounded-md border px-3'
              value={config.strategy}
              onChange={(event) =>
                setDraft({ ...config, strategy: event.target.value })
              }
            >
              <option value='round_robin'>{t('Round robin')}</option>
              <option value='random'>{t('Random')}</option>
            </select>
          </label>
        </div>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Invoice payments below the threshold use the private gateway; tax still applies. Payments at or above the threshold use corporate gateways.'
          )}
        </p>
        {config.gateways.map((gateway, index) => (
          <div key={gateway.id} className='space-y-4 rounded-md border p-4'>
            <div className='flex items-center justify-between gap-2'>
              <span className='min-w-0 text-sm font-medium break-words'>
                {t('Corporate gateway')} {index + 1}
              </span>
              <div className='flex shrink-0 gap-1'>
                <Button
                  type='button'
                  size='icon'
                  variant='ghost'
                  title={t('Move up')}
                  aria-label={t('Move up')}
                  disabled={index === 0}
                  onClick={() => moveGateway(index, -1)}
                >
                  <ArrowUp className='size-4' />
                </Button>
                <Button
                  type='button'
                  size='icon'
                  variant='ghost'
                  title={t('Move down')}
                  aria-label={t('Move down')}
                  disabled={index === config.gateways.length - 1}
                  onClick={() => moveGateway(index, 1)}
                >
                  <ArrowDown className='size-4' />
                </Button>
                <Button
                  type='button'
                  size='icon'
                  variant='ghost'
                  title={t('Delete')}
                  aria-label={t('Delete')}
                  onClick={() =>
                    setDraft({
                      ...config,
                      gateways: config.gateways.filter(
                        (item) => item.id !== gateway.id
                      ),
                    })
                  }
                >
                  <Trash2 className='size-4' />
                </Button>
              </div>
            </div>
            <div className='grid gap-4 sm:grid-cols-2'>
              <label className='space-y-2 text-sm'>
                <span>{t('Payment gateway name')}</span>
                <Input
                  maxLength={200}
                  value={gateway.name}
                  onChange={(event) =>
                    updateGateway(gateway.id, { name: event.target.value })
                  }
                />
              </label>
              <label className='space-y-2 text-sm'>
                <span>{t('Corporate account name')}</span>
                <Input
                  maxLength={200}
                  value={gateway.account_name}
                  onChange={(event) =>
                    updateGateway(gateway.id, {
                      account_name: event.target.value,
                    })
                  }
                />
              </label>
              <label className='space-y-2 text-sm'>
                <span>{t('Epay endpoint')}</span>
                <Input
                  value={gateway.address}
                  onChange={(event) =>
                    updateGateway(gateway.id, { address: event.target.value })
                  }
                />
              </label>
              <label className='space-y-2 text-sm'>
                <span>{t('Epay merchant ID')}</span>
                <Input
                  value={gateway.partner_id}
                  onChange={(event) =>
                    updateGateway(gateway.id, {
                      partner_id: event.target.value,
                    })
                  }
                />
              </label>
              <label className='space-y-2 text-sm'>
                <span>{t('Epay secret key')}</span>
                <Input
                  type='password'
                  autoComplete='new-password'
                  value={gateway.key ?? ''}
                  placeholder={
                    gateway.key_configured ? t('Enter new key to update') : ''
                  }
                  onChange={(event) =>
                    updateGateway(gateway.id, { key: event.target.value })
                  }
                />
              </label>
            </div>
            <div className='flex flex-wrap gap-4'>
              {['alipay', 'wxpay'].map((method) => (
                <label key={method} className='flex items-center gap-2 text-sm'>
                  <input
                    type='checkbox'
                    checked={gateway.payment_methods.includes(method)}
                    onChange={(event) =>
                      updateGateway(gateway.id, {
                        payment_methods: event.target.checked
                          ? [...gateway.payment_methods, method]
                          : gateway.payment_methods.filter(
                              (value) => value !== method
                            ),
                      })
                    }
                  />
                  {method === 'alipay' ? t('Alipay') : t('WeChat Pay')}
                </label>
              ))}
            </div>
          </div>
        ))}
        <div className='flex flex-wrap gap-2'>
          <Button
            type='button'
            variant='outline'
            disabled={config.gateways.length >= 20}
            onClick={() =>
              setDraft({
                ...config,
                gateways: [
                  ...config.gateways,
                  {
                    id: crypto.randomUUID(),
                    name: '',
                    account_name: '',
                    address: '',
                    partner_id: '',
                    key_configured: false,
                    key: '',
                    payment_methods: ['alipay', 'wxpay'],
                  },
                ],
              })
            }
          >
            <Plus className='size-4' />
            {t('Add corporate gateway')}
          </Button>
          <Button
            type='button'
            disabled={invalid || save.isPending}
            onClick={() => save.mutate()}
          >
            {t('Save')}
          </Button>
        </div>
      </fieldset>
    </section>
  )
}
