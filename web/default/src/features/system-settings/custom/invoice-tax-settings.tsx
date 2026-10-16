import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  getInvoiceTaxConfig,
  saveInvoiceTaxConfig,
  invoiceTaxModes,
  type InvoiceTaxConfig,
} from '@/features/tickets/invoice-tax'

export function InvoiceTaxSettings() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery({
    queryKey: ['invoice-tax-config'],
    queryFn: getInvoiceTaxConfig,
  })
  const [draft, setDraft] = useState<InvoiceTaxConfig | null>(null)
  const config = draft ?? query.data
  const save = useMutation({
    mutationFn: saveInvoiceTaxConfig,
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ['invoice-tax-config'] })
      toast.success(t('Saved successfully'))
    },
  })
  if (!config) {
    return <p>{query.isError ? t('Failed to load') : t('Loading...')}</p>
  }
  const valid = [config.ordinary_rate_bps, config.special_rate_bps].every(
    (rate) => Number.isInteger(rate) && rate >= 0 && rate < 10000
  )
  return (
    <section className='space-y-4 border-t pt-4'>
      <h3 className='font-medium'>{t('Invoice tax charges')}</h3>
      <div className='flex items-center gap-2'>
        <Switch
          id='invoice-tax-enabled'
          checked={config.enabled}
          onCheckedChange={(enabled) => setDraft({ ...config, enabled })}
        />
        <Label htmlFor='invoice-tax-enabled'>
          {t('Collect invoice tax charges')}
        </Label>
      </div>
      <div className='grid gap-4 sm:grid-cols-2'>
        {(['ordinary', 'special'] as const).map((type) => (
          <div key={type} className='space-y-2'>
            <Label htmlFor={`invoice-rate-${type}`}>
              {t(
                type === 'ordinary'
                  ? 'Ordinary VAT invoice rate (%)'
                  : 'Special VAT invoice rate (%)'
              )}
            </Label>
            <Input
              id={`invoice-rate-${type}`}
              type='number'
              min='0'
              max='99.99'
              step='0.01'
              value={config[`${type}_rate_bps`] / 100}
              onChange={(event) =>
                setDraft({
                  ...config,
                  [`${type}_rate_bps`]: Math.round(
                    Number(event.target.value) * 100
                  ),
                })
              }
            />
          </div>
        ))}
      </div>
      <Label>{t('Tax calculation mode')}</Label>
      <Select
        value={config.mode}
        onValueChange={(mode) => {
          if (mode) setDraft({ ...config, mode })
        }}
      >
        <SelectTrigger className='w-full'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {invoiceTaxModes.map((mode) => (
            <SelectItem key={mode.value} value={mode.value}>
              {t(mode.label)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Example: net amount CNY 10,000. Rates below use the current draft settings.'
        )}
      </p>
      <div className='overflow-x-auto'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Tax calculation mode')}</TableHead>
              <TableHead>{t('Invoice type')}</TableHead>
              <TableHead>{t('Additional payment (CNY)')}</TableHead>
              <TableHead>{t('Invoice total (CNY)')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {invoiceTaxModes.flatMap((mode) =>
              (['ordinary', 'special'] as const).map((type) => {
                const rate = config[`${type}_rate_bps`] / 10000
                const cents = Math.round(
                  mode.value === 'gross_up'
                    ? 1000000 / (1 - rate)
                    : 1000000 *
                        (1 + rate) ** (mode.value === 'compound' ? 2 : 1)
                )
                return (
                  <TableRow key={`${mode.value}-${type}`}>
                    <TableCell>
                      {t(mode.label)}
                      <div className='text-muted-foreground text-xs'>
                        {mode.formula}
                      </div>
                    </TableCell>
                    <TableCell>
                      {t(
                        type === 'ordinary'
                          ? 'Ordinary VAT invoice'
                          : 'Special VAT invoice'
                      )}
                    </TableCell>
                    <TableCell>
                      {valid ? ((cents - 1000000) / 100).toFixed(2) : '-'}
                    </TableCell>
                    <TableCell>
                      {valid ? (cents / 100).toFixed(2) : '-'}
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>
      <Button
        disabled={!valid || save.isPending}
        onClick={() => save.mutate(config)}
      >
        {t('Save')}
      </Button>
    </section>
  )
}
