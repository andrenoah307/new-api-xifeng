import { useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useForm, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { createGeneralTicket, createRefundTicket } from '../../api'
import { ticketQueryKeys } from '../../lib/ticket-actions'
import { PAYEE_TYPE_OPTIONS } from '../../constants'

const generalSchema = z.object({
  type: z.enum(['general', 'refund']),
  subject: z.string().min(1),
  priority: z.coerce.number().int().min(1).max(3),
  content: z.string().min(1).max(5000),
  // refund-specific
  refund_amount: z.coerce.number().optional(),
  payee_type: z.string().optional(),
  payee_name: z.string().optional(),
  payee_account: z.string().optional(),
  payee_bank: z.string().optional(),
  contact: z.string().optional(),
  reason: z.string().optional(),
})

type FormValues = z.infer<typeof generalSchema>

interface CreateTicketDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated?: (id: number) => void
}

export function CreateTicketDialog({
  open,
  onOpenChange,
  onCreated,
}: CreateTicketDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [ticketType, setTicketType] = useState<'general' | 'refund'>('general')

  const form = useForm<FormValues>({
    resolver: zodResolver(generalSchema) as Resolver<FormValues>,
    defaultValues: {
      type: 'general',
      subject: '',
      priority: 2,
      content: '',
      refund_amount: 0,
      payee_type: 'alipay',
      payee_name: '',
      payee_account: '',
      payee_bank: '',
      contact: '',
      reason: '',
    },
  })

  const createGeneral = useMutation({
    mutationFn: createGeneralTicket,
    onSuccess: (data) => {
      toast.success(t('Ticket created'))
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userLists(),
      })
      onOpenChange(false)
      form.reset()
      if (data?.id) onCreated?.(data.id)
    },
  })

  const createRefund = useMutation({
    mutationFn: createRefundTicket,
    onSuccess: (data) => {
      toast.success(t('Refund ticket created'))
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userLists(),
      })
      onOpenChange(false)
      form.reset()
      if (data?.id) onCreated?.(data.id)
    },
  })

  const isPending = createGeneral.isPending || createRefund.isPending

  const onSubmit = useCallback(
    (values: FormValues) => {
      if (ticketType === 'refund') {
        createRefund.mutate({
          subject: values.subject || t('Refund Ticket'),
          priority: values.priority,
          content: values.content,
          refund_amount: values.refund_amount ?? 0,
          payee_type: values.payee_type ?? 'alipay',
          payee_name: values.payee_name ?? '',
          payee_account: values.payee_account ?? '',
          payee_bank: values.payee_bank ?? '',
          contact: values.contact ?? '',
          reason: values.reason ?? '',
          attachment_ids: [],
        })
      } else {
        createGeneral.mutate({
          subject: values.subject,
          type: 'general',
          priority: values.priority,
          content: values.content,
          attachment_ids: [],
        })
      }
    },
    [ticketType, createGeneral, createRefund, t]
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('Create Ticket')}</DialogTitle>
          <DialogDescription>
            {t('Submit a new support ticket')}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <div>
              <FormLabel>{t('Type')}</FormLabel>
              <Select
                value={ticketType}
                onValueChange={(v) => setTicketType(v as 'general' | 'refund')}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="general">
                    {t('General Ticket')}
                  </SelectItem>
                  <SelectItem value="refund">
                    {t('Refund Ticket')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>

            <FormField
              control={form.control}
              name="subject"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Subject')}</FormLabel>
                  <FormControl>
                    <Input {...field} maxLength={255} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="priority"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Priority')}</FormLabel>
                  <Select
                    value={String(field.value)}
                    onValueChange={(v) => field.onChange(Number(v))}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="1">{t('Priority High')}</SelectItem>
                      <SelectItem value="2">{t('Priority Medium')}</SelectItem>
                      <SelectItem value="3">{t('Priority Low')}</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="content"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Content')}</FormLabel>
                  <FormControl>
                    <Textarea {...field} maxLength={5000} rows={4} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {ticketType === 'refund' && (
              <>
                <FormField
                  control={form.control}
                  name="refund_amount"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Refund Amount')}</FormLabel>
                      <FormControl>
                        <Input type="number" step="any" min={0} {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="payee_type"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Payee Type')}</FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {PAYEE_TYPE_OPTIONS.map((o) => (
                            <SelectItem key={o.value} value={o.value}>
                              {t(o.label)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <div className="grid grid-cols-2 gap-3">
                  <FormField
                    control={form.control}
                    name="payee_name"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Payee Name')}</FormLabel>
                        <FormControl>
                          <Input {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name="payee_account"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Payee Account')}</FormLabel>
                        <FormControl>
                          <Input {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <FormField
                    control={form.control}
                    name="payee_bank"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Payee Bank')}</FormLabel>
                        <FormControl>
                          <Input {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name="contact"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Contact')}</FormLabel>
                        <FormControl>
                          <Input {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
                <FormField
                  control={form.control}
                  name="reason"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Reason')}</FormLabel>
                      <FormControl>
                        <Textarea {...field} maxLength={1000} rows={3} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </>
            )}

            <DialogFooter>
              <Button type="submit" disabled={isPending}>
                {isPending ? t('Submitting...') : t('Submit')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
