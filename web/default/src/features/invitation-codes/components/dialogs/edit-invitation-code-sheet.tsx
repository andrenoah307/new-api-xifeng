import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { GeneratedCodesDialog } from '@/components/generated-codes-dialog'
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
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'

import {
  createInvitationCodes,
  updateInvitationCode,
  getInvitationCode,
} from '../../api'
import { invitationCodesQueryKeys } from '../../lib/invitation-code-actions'
import { useInvitationCodes } from '../invitation-codes-provider'

const formSchema = z.object({
  name: z.string().min(1),
  expired_time: z.string().optional(),
  max_uses: z.coerce.number().int().min(0),
  owner_user_id: z.coerce.number().int().min(0),
  count: z.coerce.number().int().min(1),
})

type FormValues = z.infer<typeof formSchema>

export function EditInvitationCodeSheet() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { editingCode, sheetOpen, closeSheet } = useInvitationCodes()
  const isEdit = editingCode !== null
  const [generatedCodes, setGeneratedCodes] = useState<{
    codes: string[]
    partial?: boolean
    filename: string
  } | null>(null)

  const form = useForm<FormValues>({
    resolver: zodResolver(formSchema) as Resolver<FormValues>,
    defaultValues: {
      name: '',
      expired_time: '',
      max_uses: 1,
      owner_user_id: 0,
      count: 1,
    },
  })

  const { data: loadedCode } = useQuery({
    queryKey: invitationCodesQueryKeys.detail(editingCode?.id ?? 0),
    queryFn: () =>
      editingCode ? getInvitationCode(editingCode.id) : Promise.resolve(null),
    enabled: isEdit && sheetOpen,
  })

  useEffect(() => {
    if (!sheetOpen) return
    if (isEdit && loadedCode) {
      form.reset({
        name: loadedCode.name,
        expired_time:
          loadedCode.expired_time && loadedCode.expired_time !== 0
            ? new Date(loadedCode.expired_time * 1000)
                .toISOString()
                .slice(0, 16)
            : '',
        max_uses: loadedCode.max_uses,
        owner_user_id: loadedCode.owner_user_id,
        count: 1,
      })
    } else if (!isEdit) {
      form.reset({
        name: '',
        expired_time: '',
        max_uses: 1,
        owner_user_id: 0,
        count: 1,
      })
    }
  }, [sheetOpen, isEdit, loadedCode, form])

  const createMutation = useMutation({
    mutationFn: createInvitationCodes,
    onSuccess: (result, variables) => {
      const codes = Array.isArray(result.data) ? result.data : []
      if (!result.success && codes.length === 0) return
      if (result.success) {
        toast.success(t('Created successfully'))
      }
      queryClient.invalidateQueries({
        queryKey: invitationCodesQueryKeys.lists(),
      })
      closeSheet()
      if (codes.length > 0) {
        const generated = {
          codes,
          filename: variables.name?.trim() || 'invitation-codes',
          ...(result.success ? {} : { partial: true }),
        }
        setGeneratedCodes(generated)
      }
    },
  })

  const updateMutation = useMutation({
    mutationFn: updateInvitationCode,
    onSuccess: () => {
      toast.success(t('Updated successfully'))
      queryClient.invalidateQueries({
        queryKey: invitationCodesQueryKeys.lists(),
      })
      closeSheet()
    },
  })

  const isPending = createMutation.isPending || updateMutation.isPending
  let submitLabel = t('Generate')
  if (isEdit) submitLabel = t('Save')
  if (isPending) submitLabel = t('Saving...')

  const onSubmit = useCallback(
    (values: FormValues) => {
      const expiredTime = values.expired_time
        ? Math.floor(new Date(values.expired_time).getTime() / 1000)
        : 0

      if (isEdit && editingCode) {
        updateMutation.mutate({
          id: editingCode.id,
          name: values.name,
          status: editingCode.status,
          max_uses: values.max_uses,
          owner_user_id: values.owner_user_id,
          expired_time: expiredTime,
        })
      } else {
        createMutation.mutate({
          name: values.name,
          count: values.count,
          max_uses: values.max_uses,
          owner_user_id: values.owner_user_id,
          expired_time: expiredTime,
        })
      }
    },
    [isEdit, editingCode, createMutation, updateMutation]
  )

  return (
    <Sheet open={sheetOpen} onOpenChange={(open) => !open && closeSheet()}>
      <SheetContent side={isEdit ? 'right' : 'left'} className="sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>
            {isEdit ? t('Edit Invitation Code') : t('Create Invitation Codes')}
          </SheetTitle>
          <SheetDescription>
            {isEdit
              ? t('Update invitation code settings')
              : t('Generate new invitation codes')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="flex flex-1 flex-col gap-6 overflow-y-auto px-1 py-4"
          >
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Name')}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder={t('Invitation code name')} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="expired_time"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Expiration Time')}</FormLabel>
                  <FormControl>
                    <Input
                      type="datetime-local"
                      {...field}
                      placeholder={t('Leave blank for permanent')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Leave blank for permanent')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="grid grid-cols-2 gap-4">
              <FormField
                control={form.control}
                name="max_uses"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Max Uses')}</FormLabel>
                    <FormControl>
                      <Input type="number" min={0} {...field} />
                    </FormControl>
                    <FormDescription>
                      {t('0 means unlimited')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="owner_user_id"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Owner User ID')}</FormLabel>
                    <FormControl>
                      <Input type="number" min={0} {...field} />
                    </FormControl>
                    <FormDescription>
                      {t('0 means no owner')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            {!isEdit && (
              <FormField
                control={form.control}
                name="count"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Generate Count')}</FormLabel>
                    <FormControl>
                      <Input type="number" min={1} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            <SheetFooter className="mt-auto">
              <Button type="submit" disabled={isPending}>
                <Save className="mr-1.5 h-4 w-4" />
                {submitLabel}
              </Button>
            </SheetFooter>
          </form>
        </Form>
      </SheetContent>
      <GeneratedCodesDialog
        open={generatedCodes !== null}
        onOpenChange={(dialogOpen) => {
          if (!dialogOpen) setGeneratedCodes(null)
        }}
        title={t('Invitation codes created')}
        codes={generatedCodes?.codes ?? []}
        filename={generatedCodes?.filename ?? 'invitation-codes'}
        partial={generatedCodes?.partial}
      />
    </Sheet>
  )
}
