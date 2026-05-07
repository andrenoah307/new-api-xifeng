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

  const { data: userQuota, isLoading: quotaLoading } = useQuery({
    queryKey: ['user', 'quota'],
    queryFn: getCurrentUserQuota,
    enabled: open && ticketType === 'refund',
  })

  const balanceDollars = userQuota?.quota != null
    ? quotaUnitsToDollars(userQuota.quota)
    : null

  const maxRefundDollars = userQuota?.max_refundable_quota != null
    ? quotaUnitsToDollars(userQuota.max_refundable_quota)
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
      toast.success(t('Ticket created, quota frozen'))
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
      if (ticketType === 'general') {
        if (!values.subject?.trim()) {
          form.setError('subject', { message: t('Ticket subject is required') })
          return
        }
        if (!values.content?.trim()) {
          form.setError('content', { message: t('Ticket content is required') })
          return
        }
        createGeneral.mutate({
          subject: values.subject!,
          type: 'general',
          priority: values.priority,
          content: values.content || '',
          attachment_ids: attachmentIds,
        })
      } else {
        const amount = values.refund_amount ?? 0
        if (amount <= 0) {
          form.setError('refund_amount', { message: t('Refund amount must be greater than 0') })
          return
        }
        if (maxRefundDollars != null && amount > maxRefundDollars) {
          form.setError('refund_amount', { message: t('Refund amount cannot exceed available quota') })
          return
        }
        if (!values.payee_name?.trim()) {
          form.setError('payee_name', { message: t('Payee name is required') })
          return
        }
        if (!values.payee_account?.trim()) {
          form.setError('payee_account', { message: t('Payee account is required') })
          return
        }
        if (payeeType === 'bank' && !values.payee_bank?.trim()) {
          form.setError('payee_bank', { message: t('Bank name is required') })
          return
        }
        if (!values.contact?.trim()) {
          form.setError('contact', { message: t('Contact info is required') })
          return
        }
        if (!values.reason?.trim() && !values.content?.trim()) {
          form.setError('reason', { message: t('Ticket content is required') })
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
      }
    },
    [ticketType, createGeneral, createRefund, t, attachmentIds, maxRefundDollars, form, payeeType]
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
              <FormLabel>{t('Ticket Type')}</FormLabel>
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

            {ticketType === 'refund' && (
              <Alert>
                <AlertDescription className="space-y-1">
                  {quotaLoading
                    ? t('Loading...')
                    : (
                      <>
                        <div>{t('Current available quota')}：{formatQuota(userQuota?.quota ?? 0)}</div>
                        <div>{t('Max Refundable')}：{formatQuota(userQuota?.max_refundable_quota ?? 0)}</div>
                      </>
                    )}
                </AlertDescription>
              </Alert>
            )}

            {ticketType === 'refund' && (
              <Alert variant="destructive">
                <AlertDescription>
                  {t('Refund freeze warning')}
                </AlertDescription>
              </Alert>
            )}

            <FormField
              control={form.control}
              name="subject"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('Ticket Subject')}
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
                          ? t('Subject placeholder refund')
                          : t('Subject placeholder general')
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
                      <SelectItem value="1">{t('High Priority')}</SelectItem>
                      <SelectItem value="2">{t('Normal Priority')}</SelectItem>
                      <SelectItem value="3">{t('Low Priority')}</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            {ticketType === 'refund' ? (
              <>
                <FormField
                  control={form.control}
                  name="refund_amount"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('Requested Refund Amount')}
                        <span className="text-destructive ml-0.5">*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          step="any"
                          min={0}
                          {...field}
                          placeholder={t('Refund amount placeholder')}
                        />
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
                      <FormLabel>
                        {t('Payee Type')}
                        <span className="text-destructive ml-0.5">*</span>
                      </FormLabel>
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
                        <FormLabel>
                          {t('Payee Name')}
                          <span className="text-destructive ml-0.5">*</span>
                        </FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            placeholder={t('Payee name placeholder')}
                          />
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
                        <FormLabel>
                          {t('Payee Account')}
                          <span className="text-destructive ml-0.5">*</span>
                        </FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            placeholder={t('Payee account placeholder')}
                          />
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
                        <FormLabel>
                          {t('Bank Name')}
                          <span className="text-destructive ml-0.5">*</span>
                        </FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            placeholder={t('Bank name placeholder')}
                          />
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
                      <FormLabel>
                        {t('Contact Info')}
                        <span className="text-destructive ml-0.5">*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder={t('Contact placeholder')}
                        />
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
                      <FormLabel>
                        {t('Refund Reason')}
                        <span className="text-destructive ml-0.5">*</span>
                      </FormLabel>
                      <FormControl>
                        <Textarea
                          {...field}
                          maxLength={5000}
                          rows={3}
                          placeholder={t('Refund reason placeholder')}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <p className="text-muted-foreground text-xs">
                  {t('Refund process note')}
                </p>
              </>
            ) : (
              <>
                <FormField
                  control={form.control}
                  name="content"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('Issue Description')}
                        <span className="text-destructive ml-0.5">*</span>
                      </FormLabel>
                      <FormControl>
                        <Textarea
                          {...field}
                          maxLength={5000}
                          rows={4}
                          placeholder={t('Content placeholder')}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Attachment area */}
                <div>
                  <FormLabel>{t('Attachment (Optional)')}</FormLabel>
                  <div className="mt-1.5 flex items-center gap-2">
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
                        {uploading ? t('Uploading...') : t('Upload Attachment')}
                      </label>
                    </Button>
                    <span className="text-muted-foreground text-xs">
                      {t('Upload limit hint', { n: 5, mb: 50 })}
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
              </>
            )}

            <DialogFooter>
              <Button type="submit" disabled={isPending}>
                {isPending ? t('Submitting...') : t('Submit Ticket')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
