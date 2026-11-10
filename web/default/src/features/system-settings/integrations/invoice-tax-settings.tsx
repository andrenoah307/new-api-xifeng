import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
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
  const [overdraftAmount, setOverdraftAmount] = useState<string | null>(null)
  const config = draft ?? query.data
  const save = useMutation({
    mutationFn: saveInvoiceTaxConfig,
    onSuccess: (saved) => {
      setDraft(saved)
      setOverdraftAmount(null)
      client.invalidateQueries({ queryKey: ['invoice-tax-config'] })
      client.invalidateQueries({ queryKey: ['status'] })
      toast.success(t('Saved successfully'))
    },
  })
  if (!config) {
    return <p>{query.isError ? t('Failed to load') : t('Loading...')}</p>
  }
  const legacyBefore = config.legacy_invoice_before ?? 0
  const amount =
    overdraftAmount ??
    ((config.refund_overdraft_limit_cents ?? 0) / 100).toFixed(2)
  const amountParts = /^(0|[1-9]\d?)(?:\.(\d{1,2}))?$/.exec(amount)
  const overdraftCents = amountParts
    ? Number(amountParts[1]) * 100 +
      Number((amountParts[2] ?? '').padEnd(2, '0'))
    : -1
  const validOverdraft = overdraftCents >= 0 && overdraftCents <= 1000
  const validCutoff =
    Number.isSafeInteger(legacyBefore) &&
    legacyBefore >= 0 &&
    legacyBefore <= Date.parse('9999-12-31T00:00:00+08:00') / 1000 &&
    (legacyBefore === 0 || (legacyBefore + 28800) % 86400 === 0)
  const valid =
    validOverdraft &&
    validCutoff &&
    (!config.reconciliation_review_enabled ||
      (Number.isInteger(config.reconciliation_review_count) &&
        (config.reconciliation_review_count ?? 0) >= 1)) &&
    [config.ordinary_rate_bps, config.special_rate_bps].every(
      (rate) => Number.isInteger(rate) && rate >= 0 && rate < 10000
    )
  return (
    <section className='space-y-4 border-t pt-4'>
      <h3 className='font-medium'>{t('Invoice tax charges')}</h3>
      <FieldGroup>
        <Field data-invalid={!validOverdraft}>
          <FieldLabel htmlFor='refund-overdraft-amount'>
            {t('Refund grace amount (CNY, 0 to disable)')}
          </FieldLabel>
          <Input
            id='refund-overdraft-amount'
            inputMode='decimal'
            value={amount}
            disabled={save.isPending}
            aria-invalid={!validOverdraft}
            aria-describedby={
              !validOverdraft ? 'refund-overdraft-error' : undefined
            }
            onChange={(event) => setOverdraftAmount(event.target.value)}
          />
          {!validOverdraft && (
            <FieldError id='refund-overdraft-error'>
              {t(
                'Amount must be between CNY 0 and 10, with at most two decimal places.'
              )}
            </FieldError>
          )}
        </Field>
      </FieldGroup>
      <div className='space-y-2'>
        <Label htmlFor='refund-gift-policy'>
          {t('Refund gift recovery policy')}
        </Label>
        <Select
          value={config.refund_gift_recovery_policy ?? 'prior_expiry'}
          onValueChange={(value) => {
            if (
              value === 'prior_expiry' ||
              value === 'proportional' ||
              value === 'none'
            ) {
              setDraft({ ...config, refund_gift_recovery_policy: value })
            }
          }}
        >
          <SelectTrigger id='refund-gift-policy' className='w-full'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='prior_expiry'>
              {t('Expire earlier gifts')}
            </SelectItem>
            <SelectItem value='proportional'>
              {t('Recover gifts proportionally per order')}
            </SelectItem>
            <SelectItem value='none'>{t('Do not recover gifts')}</SelectItem>
          </SelectContent>
        </Select>
        <p className='text-muted-foreground text-sm'>
          {t('Only applies to unprocessed refunds.')}
        </p>
      </div>
      <div className='flex items-center gap-2'>
        <Switch
          id='refund-settlement-guard-enabled'
          checked={!!config.refund_settlement_guard_enabled}
          onCheckedChange={(refund_settlement_guard_enabled) =>
            setDraft({ ...config, refund_settlement_guard_enabled })
          }
        />
        <Label htmlFor='refund-settlement-guard-enabled'>
          {t('Pause API consumption while a refund is pending')}
        </Label>
      </div>
      <div className='flex items-center gap-2'>
        <Switch
          id='finance-module-enabled'
          checked={!!config.module_enabled}
          onCheckedChange={(module_enabled) =>
            setDraft({ ...config, module_enabled })
          }
        />
        <Label htmlFor='finance-module-enabled'>
          {t('Enable finance module')}
        </Label>
      </div>
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
      <div className='flex items-center gap-2'>
        <Switch
          id='invoice-legacy-free-ordinary'
          checked={!!config.legacy_free_ordinary_enabled}
          onCheckedChange={(legacy_free_ordinary_enabled) =>
            setDraft({ ...config, legacy_free_ordinary_enabled })
          }
        />
        <Label htmlFor='invoice-legacy-free-ordinary'>
          {t('Free ordinary invoices for historical payments')}
        </Label>
      </div>
      <div className='space-y-2'>
        <Label htmlFor='invoice-legacy-before'>
          {t('Allow orders completed before this date to request invoices')}
        </Label>
        <Input
          id='invoice-legacy-before'
          type='date'
          min='1970-01-02'
          max='9999-12-31'
          value={
            validCutoff && legacyBefore > 0
              ? new Date((legacyBefore + 28800) * 1000)
                  .toISOString()
                  .slice(0, 10)
              : ''
          }
          onChange={(event) =>
            setDraft({
              ...config,
              legacy_invoice_before: event.target.value
                ? Date.parse(`${event.target.value}T00:00:00+08:00`) / 1000
                : 0,
            })
          }
        />
        <p className='text-muted-foreground text-sm'>
          {t(
            'The cutoff is 00:00 Beijing time, excluding that date. Leave blank to disable historical eligibility. When free ordinary invoices are enabled, qualifying payments incur no additional invoice fee and cannot use this rule for special invoices; otherwise existing eligibility and fee rules apply.'
          )}
        </p>
      </div>
      <div className='grid gap-4 sm:grid-cols-2'>
        {(['ordinary', 'special'] as const).map((type) => (
          <div key={type} className='space-y-2'>
            <Label htmlFor={`invoice-rate-${type}`}>
              {t(
                type === 'ordinary'
                  ? 'Ordinary invoice fee (%)'
                  : 'Special invoice fee (%)'
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
        type='button'
        disabled={!valid || save.isPending}
        onClick={() =>
          save.mutate({
            ...config,
            refund_overdraft_limit_cents: overdraftCents,
          })
        }
      >
        {t('Save')}
      </Button>
    </section>
  )
}
