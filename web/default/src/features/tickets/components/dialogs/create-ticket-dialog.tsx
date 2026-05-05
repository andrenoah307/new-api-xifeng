import { useState, useCallback, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useForm, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Paperclip, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { formatQuota, parseQuotaFromDollars, quotaUnitsToDollars } from '@/lib/format'
import {
  createGeneralTicket,
  createRefundTicket,
  getCurrentUserQuota,
} from '../../api'
import { ticketQueryKeys } from '../../lib/ticket-actions'
import { PAYEE_TYPE_OPTIONS, humanFileSize } from '../../constants'
import { useTicketAttachments } from '../../hooks/use-ticket-attachments'

const generalSchema = z.object({
  type: z.enum(['general', 'refund']),
  subject: z.string().optional(),
  priority: z.coerce.number().int().min(1).max(3),
  content: z.string().max(5000).optional(),
  refund_amount: z.coerce.number().optional(),
  payee_type: z.string().optional(),
  payee_name: z.string().optional(),
  payee_account: z.string().optional(),
  payee_bank: z.string().optional(),
  contact: z.string().optional(),
  reason: z.string().max(5000).optional(),
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

  const {
    attachments,
    uploading,
    attachmentIds,
    handleFiles,
    handlePaste,
    remove,
    reset: resetAttachments,
    discardAll,
  } = useTicketAttachments()

  const { data: userQuota } = useQuery({
    queryKey: ['user', 'quota'],
    queryFn: getCurrentUserQuota,
    enabled: open && ticketType === 'refund',
  })

  const balanceDollars = userQuota?.quota != null
    ? quotaUnitsToDollars(userQuota.quota)
    : null

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

  const payeeType = form.watch('payee_type')

  useEffect(() => {
    if (!open) {
      form.reset()
      setTicketType('general')
      discardAll()
    }
  }, [open])

  const createGeneral = useMutation({
    mutationFn: createGeneralTicket,
    onSuccess: (data) => {
      toast.success(t('Ticket created'))
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userLists(),
      })
      onOpenChange(false)
      form.reset()
      resetAttachments()
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
      queryClient.invalidateQueries({ queryKey: ['user'] })
      onOpenChange(false)
      form.reset()
      resetAttachments()
      if (data?.id) onCreated?.(data.id)
    },
  })

  const isPending = createGeneral.isPending || createRefund.isPending

  const onSubmit = useCallback(
    (values: FormValues) => {
      if (ticketType === 'general' && !values.subject?.trim()) {
        form.setError('subject', { message: t('Subject is required') })
        return
      }
      if (ticketType === 'general' && !values.content?.trim()) {
        form.setError('content', { message: t('Content is required') })
        return
      }
      if (ticketType === 'refund') {
        const amount = values.refund_amount ?? 0
        if (amount <= 0) {
          form.setError('refund_amount', { message: t('Amount must be greater than 0') })
          return
        }
        if (balanceDollars != null && amount > balanceDollars) {
          form.setError('refund_amount', { message: t('Amount exceeds your balance') })
          return
        }
        createRefund.mutate({
          subject: values.subject?.trim() || t('Refund Ticket'),
          priority: values.priority,
          refund_quota: parseQuotaFromDollars(amount),
          payee_type: values.payee_type ?? 'alipay',
          payee_name: values.payee_name ?? '',
          payee_account: values.payee_account ?? '',
          payee_bank: values.payee_type === 'bank' ? (values.payee_bank ?? '') : '',
          contact: values.contact ?? '',
          reason: values.reason || values.content || '',
          attachment_ids: attachmentIds,
        })
      } else {
        createGeneral.mutate({
          subject: values.subject!,
          type: 'general',
          priority: values.priority,
          content: values.content || '',
          attachment_ids: attachmentIds,
        })
      }
    },
    [ticketType, createGeneral, createRefund, t, attachmentIds, balanceDollars, form]
  )

  const handleFileInput = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      if (e.target.files) handleFiles(e.target.files)
      e.target.value = ''
    },
    [handleFiles]
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-h-[85vh] overflow-y-auto sm:max-w-lg"
        onPasteCapture={handlePaste}
      >
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

            {ticketType === 'refund' && balanceDollars != null && (
              <Alert>
                <AlertDescription>
                  {t('Current balance')}: {formatQuota(userQuota!.quota)}
                </AlertDescription>
              </Alert>
            )}

            {ticketType === 'refund' && (
              <Alert variant="destructive">
                <AlertDescription>
                  {t('Submitting a refund will freeze the corresponding balance until processed.')}
                </AlertDescription>
              </Alert>
            )}

            <FormField
              control={form.control}
              name="subject"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('Subject')}
                    {ticketType === 'general' && (
                      <span className="text-destructive ml-0.5">*</span>
                    )}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      maxLength={255}
                      placeholder={
                        ticketType === 'refund'
                          ? t('Optional, auto-generated if empty')
                          : undefined
                      }
                    />
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
                  <FormLabel>
                    {t('Content')}
                    {ticketType === 'general' && (
                      <span className="text-destructive ml-0.5">*</span>
                    )}
                  </FormLabel>
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
                {payeeType === 'bank' && (
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
                )}
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
                <FormField
                  control={form.control}
                  name="reason"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Reason')}</FormLabel>
                      <FormControl>
                        <Textarea {...field} maxLength={5000} rows={3} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </>
            )}

            {/* Attachment area */}
            <div>
              <div className="flex items-center gap-2">
                <input
                  type="file"
                  multiple
                  className="hidden"
                  id="create-ticket-file-input"
                  onChange={handleFileInput}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={uploading}
                  asChild
                >
                  <label
                    htmlFor="create-ticket-file-input"
                    className="cursor-pointer"
                  >
                    <Paperclip className="mr-1.5 h-4 w-4" />
                    {uploading ? t('Uploading...') : t('Attach')}
                  </label>
                </Button>
                <span className="text-muted-foreground text-xs">
                  {t('You can also paste images')}
                </span>
              </div>
              {attachments.length > 0 && (
                <div className="mt-2 flex flex-wrap gap-2">
                  {attachments.map((a) => (
                    <div
                      key={a.id}
                      className="bg-muted flex items-center gap-1.5 rounded-md px-2 py-1 text-xs"
                    >
                      <Paperclip className="h-3 w-3" />
                      <span className="max-w-[120px] truncate">
                        {a.file_name}
                      </span>
                      <span className="text-muted-foreground">
                        {humanFileSize(a.size)}
                      </span>
                      <button
                        type="button"
                        onClick={() => remove(a.id)}
                        className="text-muted-foreground hover:text-foreground ml-1"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>

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
