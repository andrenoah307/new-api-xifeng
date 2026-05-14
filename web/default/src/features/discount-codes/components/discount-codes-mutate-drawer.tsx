import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { DateTimePicker } from '@/components/datetime-picker'
import {
  createDiscountCode,
  updateDiscountCode,
  getDiscountCode,
} from '../api'
import {
  DISCOUNT_CODE_VALIDATION,
  SUCCESS_MESSAGES,
  getDiscountCodeFormErrorMessages,
} from '../constants'
import { type DiscountCode } from '../types'
import { useDiscountCodes } from './discount-codes-provider'

// ============================================================================
// Form Schema & Types
// ============================================================================

function getFormSchema(t: TFunction) {
  const msg = getDiscountCodeFormErrorMessages(t)
  return z.object({
    name: z
      .string()
      .max(
        DISCOUNT_CODE_VALIDATION.NAME_MAX_LENGTH,
        t('Name must be at most {{max}} characters', {
          max: DISCOUNT_CODE_VALIDATION.NAME_MAX_LENGTH,
        })
      )
      .optional()
      .or(z.literal('')),
    code: z.string().optional().or(z.literal('')),
    discount_rate: z
      .number()
      .min(DISCOUNT_CODE_VALIDATION.RATE_MIN, msg.RATE_INVALID)
      .max(DISCOUNT_CODE_VALIDATION.RATE_MAX, msg.RATE_INVALID),
    start_time: z.date().optional(),
    end_time: z.date().optional(),
    max_uses_per_user: z.number().min(0),
    max_uses_total: z.number().min(0),
    count: z
      .number()
      .min(DISCOUNT_CODE_VALIDATION.COUNT_MIN, msg.COUNT_INVALID)
      .max(DISCOUNT_CODE_VALIDATION.COUNT_MAX, msg.COUNT_INVALID)
      .optional(),
  })
}

type FormValues = {
  name?: string
  code?: string
  discount_rate: number
  start_time?: Date
  end_time?: Date
  max_uses_per_user: number
  max_uses_total: number
  count?: number
}

const DEFAULT_VALUES: FormValues = {
  name: '',
  code: '',
  discount_rate: 90,
  start_time: undefined,
  end_time: undefined,
  max_uses_per_user: 0,
  max_uses_total: 0,
  count: 1,
}

function transformToPayload(data: FormValues) {
  return {
    name: data.name || '',
    code: data.code || '',
    discount_rate: data.discount_rate,
    start_time: data.start_time
      ? Math.floor(data.start_time.getTime() / 1000)
      : 0,
    end_time: data.end_time
      ? Math.floor(data.end_time.getTime() / 1000)
      : 0,
    max_uses_per_user: data.max_uses_per_user,
    max_uses_total: data.max_uses_total,
    count: data.count || 1,
  }
}

function transformToFormDefaults(dc: DiscountCode): FormValues {
  return {
    name: dc.name,
    code: dc.code,
    discount_rate: dc.discount_rate,
    start_time: dc.start_time > 0 ? new Date(dc.start_time * 1000) : undefined,
    end_time: dc.end_time > 0 ? new Date(dc.end_time * 1000) : undefined,
    max_uses_per_user: dc.max_uses_per_user,
    max_uses_total: dc.max_uses_total,
    count: 1,
  }
}

// ============================================================================
// Component
// ============================================================================

type DiscountCodesMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: DiscountCode
}

export function DiscountCodesMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: DiscountCodesMutateDrawerProps) {
  const { t } = useTranslation()
  const isUpdate = !!currentRow
  const { triggerRefresh } = useDiscountCodes()
  const [isSubmitting, setIsSubmitting] = useState(false)

  const form = useForm<FormValues>({
    resolver: zodResolver(getFormSchema(t)),
    defaultValues: DEFAULT_VALUES,
  })

  useEffect(() => {
    if (open && isUpdate && currentRow) {
      getDiscountCode(currentRow.id).then((result) => {
        if (result.success && result.data) {
          form.reset(transformToFormDefaults(result.data))
        }
      })
    } else if (open && !isUpdate) {
      form.reset(DEFAULT_VALUES)
    }
  }, [open, isUpdate, currentRow, form])

  const onSubmit = async (data: FormValues) => {
    setIsSubmitting(true)
    try {
      const payload = transformToPayload(data)

      if (isUpdate && currentRow) {
        const result = await updateDiscountCode({
          ...payload,
          id: currentRow.id,
        })
        if (result.success) {
          toast.success(t(SUCCESS_MESSAGES.DISCOUNT_CODE_UPDATED))
          onOpenChange(false)
          triggerRefresh()
        }
      } else {
        const result = await createDiscountCode(payload)
        if (result.success) {
          const count = result.data?.length || 0
          toast.success(
            count > 1
              ? t('Successfully created {{count}} discount codes', { count })
              : t(SUCCESS_MESSAGES.DISCOUNT_CODE_CREATED)
          )
          onOpenChange(false)
          triggerRefresh()
        }
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) form.reset()
      }}
    >
      <SheetContent className='flex h-dvh w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-[600px]'>
        <SheetHeader className='border-b px-4 py-3 text-start sm:px-6 sm:py-4'>
          <SheetTitle>
            {isUpdate
              ? t('Edit Discount Code')
              : t('Create Discount Code')}
          </SheetTitle>
          <SheetDescription>
            {isUpdate
              ? t('Update the discount code by providing necessary info.')
              : t('Add new discount code(s) by providing necessary info.')}{' '}
            {t('Click save when you&apos;re done.')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='discount-code-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className='flex-1 space-y-4 overflow-y-auto px-3 py-3 pb-4 sm:space-y-6 sm:px-4'
          >
            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Name')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder={t('Enter a name')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Optional name for this discount code (max 100 characters)')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='code'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Code')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder={t('Leave blank to auto-generate')}
                      disabled={isUpdate}
                    />
                  </FormControl>
                  <FormDescription>
                    {isUpdate
                      ? t('Code cannot be changed after creation')
                      : t('Leave blank to auto-generate')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='discount_rate'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Discount Rate')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='number'
                      min={DISCOUNT_CODE_VALIDATION.RATE_MIN}
                      max={DISCOUNT_CODE_VALIDATION.RATE_MAX}
                      placeholder='90'
                      onChange={(e) =>
                        field.onChange(parseInt(e.target.value, 10) || 0)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t('90 = 10% off, pay 90%. Range: 1-99')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='start_time'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Start Time')}</FormLabel>
                  <FormControl>
                    <DateTimePicker
                      value={field.value}
                      onChange={field.onChange}
                      placeholder={t('No limit')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Leave empty for no start time limit')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='end_time'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('End Time')}</FormLabel>
                  <FormControl>
                    <DateTimePicker
                      value={field.value}
                      onChange={field.onChange}
                      placeholder={t('No limit')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Leave empty for no end time limit')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='max_uses_per_user'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max Uses Per User')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='number'
                      min='0'
                      placeholder='0'
                      onChange={(e) =>
                        field.onChange(parseInt(e.target.value, 10) || 0)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t('0 = unlimited')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='max_uses_total'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max Total Uses')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='number'
                      min='0'
                      placeholder='0'
                      onChange={(e) =>
                        field.onChange(parseInt(e.target.value, 10) || 0)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t('0 = unlimited')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {!isUpdate && (
              <FormField
                control={form.control}
                name='count'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Batch create count')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min='1'
                        max='100'
                        placeholder={t('Number of codes to create')}
                        onChange={(e) =>
                          field.onChange(parseInt(e.target.value, 10) || 1)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Create multiple discount codes at once (1-100)')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
          </form>
        </Form>
        <SheetFooter className='grid grid-cols-2 gap-2 border-t px-4 py-3 sm:flex sm:px-6 sm:py-4'>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
          <Button
            form='discount-code-form'
            type='submit'
            disabled={isSubmitting}
          >
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
