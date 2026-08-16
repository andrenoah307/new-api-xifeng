/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useMemo, type FormEvent } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Dialog } from '@/components/dialog'
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

import {
  GROUP_RATE_LIMIT_MAX_COUNT,
  type GroupRateLimitRule,
} from './lib/group-rate-limit'

const rateLimitDialogSchema = z.object({
  groupName: z.string().min(1, 'Group name is required'),
  totalCount: z
    .number()
    .int('Must be an integer')
    .min(0, 'Must be ≥ 0')
    .max(GROUP_RATE_LIMIT_MAX_COUNT, 'Must be ≤ 2,147,483,647'),
  successCount: z
    .number()
    .int('Must be an integer')
    .min(1, 'Must be ≥ 1')
    .max(GROUP_RATE_LIMIT_MAX_COUNT, 'Must be ≤ 2,147,483,647'),
})

type RateLimitDialogFormValues = z.infer<typeof rateLimitDialogSchema>

const RATE_LIMIT_FORM_ID = 'rate-limit-form'

export type RateLimitEntryData = GroupRateLimitRule

type RateLimitDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (data: RateLimitEntryData) => boolean
  editData?: RateLimitEntryData | null
  groupOptions: string[]
  existingGroupNames: string[]
}

export function RateLimitDialog({
  open,
  onOpenChange,
  onSave,
  editData,
  groupOptions,
  existingGroupNames,
}: RateLimitDialogProps) {
  const { t } = useTranslation()
  const isEditMode = !!editData

  const form = useForm<RateLimitDialogFormValues>({
    resolver: zodResolver(rateLimitDialogSchema),
    defaultValues: {
      groupName: '',
      totalCount: 0,
      successCount: 1,
    },
  })

  useEffect(() => {
    if (editData) {
      form.reset(editData)
    } else {
      form.reset({
        groupName: '',
        totalCount: 0,
        successCount: 1,
      })
    }
  }, [editData, form, open])

  const selectableGroups = useMemo(() => {
    const catalog = new Set(groupOptions)
    const allNames = [...new Set([...groupOptions, ...existingGroupNames])]
    return allNames
      .filter((name) => name !== '' && (isEditMode || catalog.has(name)))
      .map((name) => ({
        name,
        isDeleted: !catalog.has(name),
      }))
  }, [existingGroupNames, groupOptions, isEditMode])

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault()
    void form.handleSubmit((values) => {
      if (!onSave(values)) return
      form.reset()
      onOpenChange(false)
    })(event)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        isEditMode ? t('Edit group rate limit') : t('Add group rate limit')
      }
      description={t(
        'Configure rate limiting rules for a specific user group.'
      )}
      contentClassName='sm:max-w-[500px]'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={RATE_LIMIT_FORM_ID}>
            {isEditMode ? t('Update') : t('Add')}
          </Button>
        </>
      }
    >
      <form
        id={RATE_LIMIT_FORM_ID}
        onSubmit={handleSubmit}
        className='space-y-4'
      >
        <div className='space-y-2'>
          <Label>{t('Group Name')}</Label>
          <Select
            value={form.watch('groupName')}
            onValueChange={(value: string) => form.setValue('groupName', value)}
          >
            <SelectTrigger className='w-full'>
              <SelectValue placeholder={t('Select a group')} />
            </SelectTrigger>
            <SelectContent>
              {selectableGroups.map((option) => (
                <SelectItem key={option.name} value={option.name}>
                  {option.name}
                  {option.isDeleted ? ` (${t('Deleted')})` : ''}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className='text-muted-foreground text-xs'>
            {isEditMode
              ? t('Existing groups can be renamed or removed.')
              : t('Only groups currently present in the group catalog can be added.')}
          </p>
          {form.formState.errors.groupName ? (
            <p className='text-destructive text-sm'>
              {t(String(form.formState.errors.groupName.message))}
            </p>
          ) : null}
        </div>

        <div className='space-y-2'>
          <Label htmlFor='rate-limit-total'>{t('Max Requests (including failures)')}</Label>
          <div className='flex items-center gap-2'>
            <Input
              id='rate-limit-total'
              type='number'
              min={0}
              max={GROUP_RATE_LIMIT_MAX_COUNT}
              step={1}
              {...form.register('totalCount', { valueAsNumber: true })}
            />
            <span className='text-muted-foreground text-sm'>{t('times')}</span>
          </div>
          <p className='text-muted-foreground text-xs'>
            {t('Total requests allowed per period. 0 = unlimited.')}
          </p>
          {form.formState.errors.totalCount ? (
            <p className='text-destructive text-sm'>
              {t(String(form.formState.errors.totalCount.message))}
            </p>
          ) : null}
        </div>

        <div className='space-y-2'>
          <Label htmlFor='rate-limit-success'>{t('Max Successful Requests')}</Label>
          <div className='flex items-center gap-2'>
            <Input
              id='rate-limit-success'
              type='number'
              min={1}
              max={GROUP_RATE_LIMIT_MAX_COUNT}
              step={1}
              {...form.register('successCount', { valueAsNumber: true })}
            />
            <span className='text-muted-foreground text-sm'>{t('times')}</span>
          </div>
          <p className='text-muted-foreground text-xs'>
            {t('Only successful requests count toward this limit.')}
          </p>
          {form.formState.errors.successCount ? (
            <p className='text-destructive text-sm'>
              {t(String(form.formState.errors.successCount.message))}
            </p>
          ) : null}
        </div>
      </form>
    </Dialog>
  )
}
