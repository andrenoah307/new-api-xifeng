import { useState, useMemo, useEffect, useCallback } from 'react'
import { cn } from '@/lib/utils'
import { useTranslation } from 'react-i18next'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useStatus } from '@/hooks/use-status'
import {
  Dialog,
  DialogContent,
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import {
  createInvoiceTicket,
  getEligibleInvoiceOrders,
  getInvoiceProfile,
  getTicketLimitStatus,
  checkInvoiceRefundConflict,
  type TicketInvoiceOrder,
} from '../../api'
import { ticketQueryKeys } from '../../lib/ticket-actions'

const TAX_NUMBER_REGEX = /^[A-Z0-9]{18}$/

function createSchema(t: (key: string) => string) {
  return z.object({
    company_name: z.string().min(1),
    tax_number: z
      .string()
      .min(1)
      .transform((v) => v.toUpperCase())
      .pipe(
        z.string().regex(
          TAX_NUMBER_REGEX,
          t('Tax number must be 18 uppercase alphanumeric characters')
        )
      ),
    email: z.string().email(),
    content: z.string().max(100).optional(),
    bank_name: z.string().optional(),
    bank_account: z.string().optional(),
    company_address: z.string().optional(),
    company_phone: z.string().optional(),
  })
}

type FormValues = z.infer<ReturnType<typeof createSchema>>

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated?: (id: number) => void
}

export function CreateInvoiceTicketDialog({
  open,
  onOpenChange,
  onCreated,
}: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { status } = useStatus()
  const schema = useMemo(() => createSchema(t), [t])
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [refundConflictAcked, setRefundConflictAcked] = useState(false)
  const [invoiceType, setInvoiceType] = useState(1)
  const minInvoiceAmount = Number(status?.min_invoice_amount) || 0
  const specialEnabled = status?.invoice_special_enabled === true
  const regularFeeRate = Number(status?.invoice_regular_fee_rate) || 0
  const specialFeeRate = Number(status?.invoice_special_fee_rate) || 0

  // 票种选项：普票恒可用，增票需管理员在后台开放；说明文本由管理员自由撰写
  const invoiceTypeOptions = useMemo(
    () => [
      {
        value: 1,
        label: t('Regular Invoice'),
        feeRate: regularFeeRate,
        description: String(status?.invoice_regular_description ?? ''),
      },
      ...(specialEnabled
        ? [
            {
              value: 2,
              label: t('VAT Special Invoice'),
              feeRate: specialFeeRate,
              description: String(status?.invoice_special_description ?? ''),
            },
          ]
        : []),
    ],
    [t, regularFeeRate, specialFeeRate, specialEnabled, status]
  )
  const selectedFeeRate = invoiceType === 2 ? specialFeeRate : regularFeeRate

  const { data: orders = [], isLoading: ordersLoading } = useQuery({
    queryKey: ticketQueryKeys.eligibleOrders(),
    queryFn: getEligibleInvoiceOrders,
    enabled: open,
  })

  const { data: refundConflict } = useQuery({
    queryKey: ticketQueryKeys.invoiceRefundCheck(),
    queryFn: checkInvoiceRefundConflict,
    enabled: open,
  })

  const { data: limitStatus } = useQuery({
    queryKey: ['ticket', 'limit-status'],
    queryFn: getTicketLimitStatus,
    enabled: open,
  })

  const { data: invoiceProfile } = useQuery({
    queryKey: ['ticket', 'invoice-profile'],
    queryFn: getInvoiceProfile,
    enabled: open,
  })

  const isLimited = limitStatus?.limited === true

  useEffect(() => {
    if (!open) {
      setSelectedIds(new Set())
      setRefundConflictAcked(false)
    }
  }, [open])

  const invoiceAmount = useMemo(
    () =>
      orders
        .filter((o) => selectedIds.has(o.id))
        .reduce((sum, o) => sum + Number(o.money || 0), 0),
    [selectedIds, orders]
  )

  // 手续费按后端同口径预估：所选订单的实际到账额度之和 × 费率。
  // 后端 calcInvoiceFeeQuota 用 quota_granted 而非人民币金额（充值单位混合且可能有折扣倍率）。
  const estimatedFeeQuota = useMemo(() => {
    if (selectedFeeRate <= 0) return 0
    const baseQuota = orders
      .filter((o) => selectedIds.has(o.id))
      .reduce((sum, o) => sum + Number(o.quota_granted || 0), 0)
    return Math.round((baseQuota * selectedFeeRate) / 100)
  }, [selectedIds, orders, selectedFeeRate])

  const toggleOrder = useCallback((id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const toggleAll = useCallback(() => {
    setSelectedIds((prev) =>
      prev.size === orders.length
        ? new Set()
        : new Set(orders.map((o) => o.id))
    )
  }, [orders])

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      company_name: '',
      tax_number: '',
      email: '',
      content: '',
      bank_name: '',
      bank_account: '',
      company_address: '',
      company_phone: '',
    },
  })

  // 默认填充最近一次申请的发票抬头（仅在表单还是空白时，避免覆盖用户已输入内容）
  useEffect(() => {
    if (!open || !invoiceProfile) return
    const { company_name, tax_number, email } = form.getValues()
    if (company_name || tax_number || email) return
    form.reset({
      company_name: invoiceProfile.company_name ?? '',
      tax_number: invoiceProfile.tax_number ?? '',
      email: invoiceProfile.email ?? '',
      content: '',
      bank_name: invoiceProfile.bank_name ?? '',
      bank_account: invoiceProfile.bank_account ?? '',
      company_address: invoiceProfile.company_address ?? '',
      company_phone: invoiceProfile.company_phone ?? '',
    })
    if (invoiceProfile.invoice_type === 2 && specialEnabled) {
      setInvoiceType(2)
    }
  }, [open, invoiceProfile, form, specialEnabled])

  const mutation = useMutation({
    mutationFn: createInvoiceTicket,
    onSuccess: (data) => {
      if (!data) return
      toast.success(t('Invoice ticket submitted'))
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userLists(),
      })
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.eligibleOrders(),
      })
      queryClient.invalidateQueries({
        queryKey: ['ticket', 'invoice-profile'],
      })
      onOpenChange(false)
      form.reset()
      if (data?.id) onCreated?.(data.id)
    },
  })

  const onSubmit = useCallback(
    (values: FormValues) => {
      if (isLimited) return

      if (selectedIds.size === 0) {
        toast.error(t('Please select at least one order'))
        return
      }

      if (minInvoiceAmount > 0 && invoiceAmount < minInvoiceAmount) {
        toast.error(
          t('Invoice amount must be at least {{amount}} CNY', {
            amount: minInvoiceAmount,
          })
        )
        return
      }

      // 增票（增值税专用发票）按规定需要完整的开户与联系信息
      if (
        invoiceType === 2 &&
        (!values.bank_name?.trim() ||
          !values.bank_account?.trim() ||
          !values.company_address?.trim() ||
          !values.company_phone?.trim())
      ) {
        toast.error(
          t(
            'VAT special invoice requires bank name, bank account, company address and phone'
          )
        )
        return
      }

      mutation.mutate({
        subject: t('Invoice Application'),
        company_name: values.company_name,
        tax_number: values.tax_number,
        email: values.email ?? '',
        content: values.content ?? '',
        invoice_type: invoiceType,
        bank_name: values.bank_name?.trim() ?? '',
        bank_account: values.bank_account?.trim() ?? '',
        company_address: values.company_address?.trim() ?? '',
        company_phone: values.company_phone?.trim() ?? '',
        topup_order_ids: Array.from(selectedIds),
        refund_conflict_acknowledged: refundConflictAcked,
      })
    },
    [
      isLimited,
      selectedIds,
      minInvoiceAmount,
      invoiceAmount,
      invoiceType,
      mutation,
      t,
      refundConflictAcked,
    ]
  )

  const isSubmitDisabled =
    isLimited ||
    mutation.isPending ||
    selectedIds.size === 0 ||
    (minInvoiceAmount > 0 && invoiceAmount < minInvoiceAmount) ||
    (refundConflict?.has_refunds && !refundConflictAcked)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="no-scrollbar max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('Apply for Invoice')}</DialogTitle>
        </DialogHeader>

        <div className="space-y-5">
          {isLimited && (
            <Alert variant="warning">
              <AlertDescription>
                {t('You\'ve used this week\'s limit for creating tickets / invoice requests on a low balance. If you need help, please contact support first.')}
              </AlertDescription>
            </Alert>
          )}

          {refundConflict?.has_refunds && (
            <Alert variant="warning">
              <AlertDescription className="space-y-2">
                <p className="font-medium">{t('Refund conflict warning')}</p>
                <div className="flex items-center gap-2 pt-1">
                  <Checkbox
                    id="refund-conflict-ack"
                    checked={refundConflictAcked}
                    onCheckedChange={(checked) => setRefundConflictAcked(checked === true)}
                  />
                  <label htmlFor="refund-conflict-ack" className="text-xs cursor-pointer">
                    {t('Refund conflict acknowledge')}
                  </label>
                </div>
              </AlertDescription>
            </Alert>
          )}

          {/* Step 1: Order selection */}
          <div>
            <h4 className="mb-2 text-sm font-medium">
              1. {t('Select Top-up Orders')}
            </h4>
            <div className="max-h-[200px] overflow-y-auto rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-10">
                      <Checkbox
                        checked={
                          orders.length > 0 &&
                          selectedIds.size === orders.length
                        }
                        onCheckedChange={toggleAll}
                      />
                    </TableHead>
                    <TableHead>{t('Trade No.')}</TableHead>
                    <TableHead className="w-[100px]">
                      {t('Payment Method')}
                    </TableHead>
                    <TableHead className="w-[100px]">
                      {t('Paid Amount')}
                    </TableHead>
                    <TableHead className="w-[160px]">
                      {t('Top-up Time')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {ordersLoading ? (
                    <TableRow>
                      <TableCell colSpan={5} className="text-center">
                        {t('Loading...')}
                      </TableCell>
                    </TableRow>
                  ) : orders.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={5}
                        className="text-muted-foreground text-center"
                      >
                        {t('No eligible orders')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    orders.map((order: TicketInvoiceOrder) => (
                      <TableRow
                        key={order.id}
                        className="cursor-pointer"
                        onClick={() => toggleOrder(order.id)}
                      >
                        <TableCell>
                          <Checkbox
                            checked={selectedIds.has(order.id)}
                            onCheckedChange={() => toggleOrder(order.id)}
                          />
                        </TableCell>
                        <TableCell className="truncate text-xs">
                          {order.trade_no}
                        </TableCell>
                        <TableCell className="text-xs">
                          {order.payment_method || '-'}
                        </TableCell>
                        <TableCell className="text-xs">
                          ¥{Number(order.money || 0).toFixed(2)}
                        </TableCell>
                        <TableCell className="text-xs">
                          {order.complete_time
                            ? formatTimestampToDate(order.complete_time)
                            : '-'}
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
            <div className="bg-muted mt-2 flex flex-wrap items-center justify-between gap-x-4 gap-y-1 rounded-md px-3 py-2 text-sm">
              <span className="text-muted-foreground">
                {t('Selected')}: {selectedIds.size}/{orders.length} {t('orders_unit')}
              </span>
              <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span>
                  <span className="text-muted-foreground mr-1">
                    {t('Invoice Amount')}:
                  </span>
                  {/* 未达最低开票金额时标红，让用户不用点提交就知道差在哪 */}
                  <span
                    className={
                      minInvoiceAmount > 0 && invoiceAmount < minInvoiceAmount
                        ? 'text-destructive font-medium'
                        : 'font-medium'
                    }
                  >
                    ¥{invoiceAmount.toFixed(2)}
                  </span>
                </span>
                {selectedFeeRate > 0 && (
                  <span>
                    <span className="text-muted-foreground mr-1">
                      {t('Fee ({{rate}}%)', { rate: selectedFeeRate })}:
                    </span>
                    <span className="font-medium">
                      {formatQuota(estimatedFeeQuota)}
                    </span>
                  </span>
                )}
                {minInvoiceAmount > 0 && (
                  <span className="text-muted-foreground text-xs">
                    （
                    {t('Minimum invoice amount: {{amount}} CNY', {
                      amount: minInvoiceAmount,
                    })}
                    ）
                  </span>
                )}
              </span>
              {selectedFeeRate > 0 && (
                <span className="text-muted-foreground w-full text-xs">
                  {t(
                    'The invoice fee will be deducted from your account balance once the invoice is issued.'
                  )}
                </span>
              )}
            </div>
          </div>

          {/* Step 2: Invoice type selection */}
          <div>
            <h4 className="mb-2 text-sm font-medium">
              2. {t('Select Invoice Type')}
            </h4>
            <div className="space-y-2">
              {invoiceTypeOptions.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => setInvoiceType(option.value)}
                  className={cn(
                    'w-full rounded-md border p-3 text-left transition-colors',
                    invoiceType === option.value
                      ? 'border-primary ring-primary ring-1'
                      : 'hover:border-muted-foreground/40'
                  )}
                >
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <span className="text-sm font-medium">{option.label}</span>
                    {option.feeRate > 0 && (
                      <span className="bg-destructive/10 text-destructive rounded px-1.5 py-0.5 text-xs">
                        {t('Fee rate {{rate}}% (deducted from balance)', {
                          rate: option.feeRate,
                        })}
                      </span>
                    )}
                  </div>
                  {option.description && (
                    <p className="text-muted-foreground mt-1 text-xs whitespace-pre-wrap">
                      {option.description}
                    </p>
                  )}
                </button>
              ))}
            </div>
          </div>

          {/* Step 3: Invoice details form */}
          <div>
            <h4 className="mb-2 text-sm font-medium">
              3. {t('Fill in Invoice Header')}
            </h4>
            <Form {...form}>
              <form
                id="invoice-ticket-form"
                onSubmit={form.handleSubmit(onSubmit)}
                className="space-y-3"
              >
                <div className="grid grid-cols-2 gap-3">
                  <FormField
                    control={form.control}
                    name="company_name"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Organization Name')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            placeholder={t('Full company name')}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name="tax_number"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Taxpayer ID')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            placeholder={t(
                              'Unified social credit code'
                            )}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <FormItem>
                    <FormLabel>{t('Invoice Content')}</FormLabel>
                    <Input
                      disabled
                      value={
                        String(status?.invoice_service_name ?? '').trim() ||
                        t(
                          '*Production and Living Services*Technical Service Fee'
                        )
                      }
                    />
                  </FormItem>
                  <FormField
                    control={form.control}
                    name="email"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Receiving Email')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            type="email"
                            placeholder={t(
                              'Email to receive invoice'
                            )}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
                {/* 增票需要完整开户与联系信息（普票选填，隐藏以保持表单简洁） */}
                {invoiceType === 2 && (
                  <>
                    <div className="grid grid-cols-2 gap-3">
                      <FormField
                        control={form.control}
                        name="bank_name"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Bank Name')}</FormLabel>
                            <FormControl>
                              <Input {...field} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name="bank_account"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Bank Account')}</FormLabel>
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
                        name="company_address"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Company Address')}</FormLabel>
                            <FormControl>
                              <Input {...field} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name="company_phone"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Company Phone')}</FormLabel>
                            <FormControl>
                              <Input {...field} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>
                  </>
                )}
                <FormField
                  control={form.control}
                  name="content"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Invoice Notes')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          maxLength={100}
                          placeholder={t('Brief description of purpose')}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </form>
            </Form>
          </div>
        </div>

        <DialogFooter>
          <Button
            type="submit"
            form="invoice-ticket-form"
            disabled={isSubmitDisabled}
          >
            {mutation.isPending ? t('Submitting...') : t('Submit Application')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
