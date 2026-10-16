import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'

import { getFinanceChoices, saveFinancePolicy, type FinancePolicy } from './api'

export function FinancePolicyDialog(props: {
  policy: FinancePolicy
  onClose: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [draft, setDraft] = useState(props.policy)
  const choices = useQuery({
    queryKey: ['finance', 'choices'],
    queryFn: getFinanceChoices,
  })
  const save = useMutation({
    mutationFn: () => saveFinancePolicy(draft),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['finance'] })
      toast.success(t('Saved successfully'))
      props.onClose()
    },
  })
  const invoices = [
    { value: 1, label: t('Pending Issuance') },
    { value: 2, label: t('Issued') },
    { value: 3, label: t('Rejected') },
  ]
  const topups = [
    { value: 'success', label: t('Success') },
    { value: 'pending', label: t('Pending') },
    { value: 'failed', label: t('Failed') },
    { value: 'expired', label: t('Expired') },
  ]
  const methods = [
    ...new Set([
      ...(choices.data ?? []).map((item) => item.payment_method),
      ...draft.payment_methods,
    ]),
  ]
  const sources = [
    ...new Set([
      ...(choices.data ?? []).map((item) => item.source),
      ...draft.sources,
    ]),
  ]
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !save.isPending) props.onClose()
      }}
    >
      <DialogContent className='max-h-[90dvh] overflow-y-auto sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>{t('Financial record visibility')}</DialogTitle>
        </DialogHeader>
        <fieldset className='space-y-3'>
          <legend className='mb-2 text-sm font-medium'>
            {t('Invoice applications')}
          </legend>
          <div className='flex flex-wrap gap-4'>
            {invoices.map((item) => (
              <label
                key={item.value}
                className='flex items-center gap-2 text-sm'
              >
                <Checkbox
                  checked={draft.invoice_statuses.includes(item.value)}
                  onCheckedChange={(checked) =>
                    setDraft((prev) => ({
                      ...prev,
                      invoice_statuses: checked
                        ? [...prev.invoice_statuses, item.value]
                        : prev.invoice_statuses.filter(
                            (value) => value !== item.value
                          ),
                    }))
                  }
                />
                {item.label}
              </label>
            ))}
          </div>
        </fieldset>
        <fieldset className='space-y-3'>
          <legend className='mb-2 text-sm font-medium'>
            {t('All-user top-up records')}
          </legend>
          <div className='flex flex-wrap gap-4'>
            {topups.map((item) => (
              <label
                key={item.value}
                className='flex items-center gap-2 text-sm'
              >
                <Checkbox
                  checked={draft.topup_statuses.includes(item.value)}
                  onCheckedChange={(checked) =>
                    setDraft((prev) => ({
                      ...prev,
                      topup_statuses: checked
                        ? [...prev.topup_statuses, item.value]
                        : prev.topup_statuses.filter(
                            (value) => value !== item.value
                          ),
                    }))
                  }
                />
                {item.label}
              </label>
            ))}
          </div>
        </fieldset>
        <div className='space-y-2'>
          <label htmlFor='finance-methods' className='text-sm font-medium'>
            {t('Allowed payment methods')}
          </label>
          <MultiSelect
            id='finance-methods'
            options={methods.map((value) => ({
              value: value || '__empty__',
              label: value || t('Unspecified'),
            }))}
            selected={draft.payment_methods.map(
              (value) => value || '__empty__'
            )}
            onChange={(values) =>
              setDraft((prev) => ({
                ...prev,
                payment_methods: values.map((value) =>
                  value === '__empty__' ? '' : value
                ),
              }))
            }
            placeholder={t('All payment methods')}
          />
        </div>
        <div className='space-y-2'>
          <label htmlFor='finance-sources' className='text-sm font-medium'>
            {t('Allowed top-up sources')}
          </label>
          <MultiSelect
            id='finance-sources'
            options={sources.map((value) => ({
              value: value || '__empty__',
              label: value || t('Unspecified'),
            }))}
            selected={draft.sources.map((value) => value || '__empty__')}
            onChange={(values) =>
              setDraft((prev) => ({
                ...prev,
                sources: values.map((value) =>
                  value === '__empty__' ? '' : value
                ),
              }))
            }
            placeholder={t('All top-up sources')}
          />
        </div>
        <p className='text-muted-foreground text-sm'>
          {t(
            'An invoice is visible only when all linked top-ups match the allowed payment methods and sources.'
          )}
        </p>
        {choices.isError && (
          <p role='alert' className='text-destructive text-sm'>
            {t('Failed to load data')}
          </p>
        )}
        <DialogFooter>
          <Button
            variant='outline'
            disabled={save.isPending}
            onClick={props.onClose}
          >
            {t('Cancel')}
          </Button>
          <Button
            disabled={save.isPending || choices.isLoading || choices.isError}
            onClick={() => save.mutate()}
          >
            {t('Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
