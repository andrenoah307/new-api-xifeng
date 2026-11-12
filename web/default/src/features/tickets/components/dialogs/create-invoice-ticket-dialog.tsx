import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import {
  useState,
  useMemo,
  useEffect,
  useCallback,
  type ReactNode,
} from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useStatus } from '@/hooks/use-status'
import { formatTimestampToDate } from '@/lib/format'

import { filterInvoiceOrders } from '../../../../../../shared/invoice-order-filter'
import {
  createInvoiceTicket,
  getEligibleInvoiceOrders,
  checkInvoiceRefundConflict,
  type TicketInvoiceOrder,
} from '../../api'
import type { InvoiceTaxSelection } from '../../invoice-tax'
import { ticketQueryKeys } from '../../lib/ticket-actions'
import { InvoiceTaxFields } from '../invoice-tax-fields'

const TAX_NUMBER_REGEX = /^[A-Z0-9]{18}$/

function createSchema(t: (key: string) => string) {
  return z.object({
    company_name: z.string().min(1),
    tax_number: z
      .string()
      .min(1)
      .transform((v) => v.toUpperCase())
      .pipe(
        z
          .string()
          .regex(
            TAX_NUMBER_REGEX,
            t('Tax number must be 18 uppercase alphanumeric characters')
          )
      ),
    email: z.string().email(),
    content: z.string().max(100).optional(),
  })
}

type FormValues = z.infer<ReturnType<typeof createSchema>>

interface Props {
  onBack?: () => void
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated?: (id: number) => void
}

export function CreateInvoiceTicketDialog({
  onBack,
  open,
  onOpenChange,
  onCreated,
}: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { status } = useStatus()
  const schema = useMemo(() => createSchema(t), [t])
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [orderPage, setOrderPage] = useState(1)
  const [orderSearch, setOrderSearch] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  useEffect(() => {
    const timer = setTimeout(() => {
      setAppliedSearch(orderSearch)
      setOrderPage(1)
    }, 300)
    return () => clearTimeout(timer)
  }, [orderSearch])
  const [refundConflictAcked, setRefundConflictAcked] = useState(false)
  const [tax, setTax] = useState<InvoiceTaxSelection>({
    ready: false,
    invoice_type: 'ordinary',
    tax_payment_method: 'balance',
    tax_quote: '',
  })
  const minInvoiceAmount = Number(status?.min_invoice_amount) || 0

  const { data: orders = [], isLoading: ordersLoading } = useQuery({
    queryKey: [...ticketQueryKeys.eligibleOrders(), tax.invoice_type],
    queryFn: () => getEligibleInvoiceOrders(tax.invoice_type),
    enabled: open,
  })

  const { data: refundConflict } = useQuery({
    queryKey: ticketQueryKeys.invoiceRefundCheck(),
    queryFn: checkInvoiceRefundConflict,
    enabled: open,
  })
  const filteredOrders = useMemo(
    () => filterInvoiceOrders(orders, appliedSearch),
    [orders, appliedSearch]
  )

  useEffect(() => {
    if (!open) {
      setOrderSearch('')
      setAppliedSearch('')
      setOrderPage(1)
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

  const toggleOrder = useCallback((id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const toggleAll = useCallback(() => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      const allSelected = filteredOrders.every((order) => prev.has(order.id))
      for (const order of filteredOrders) {
        if (allSelected) next.delete(order.id)
        else next.add(order.id)
      }
      return next
    })
  }, [filteredOrders])

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      company_name: '',
      tax_number: '',
      email: '',
      content: '',
    },
  })

  const mutation = useMutation({
    mutationFn: createInvoiceTicket,
    onSuccess: (data) => {
      toast.success(t('Invoice ticket submitted'))
      queryClient.invalidateQueries({ queryKey: ['user'] })
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userLists(),
      })
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.eligibleOrders(),
      })
      onOpenChange(false)
      form.reset()
      if (data?.id) onCreated?.(data.id)
    },
  })

  const onSubmit = useCallback(
    (values: FormValues) => {
      if (!tax.ready) return
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

      mutation.mutate({
        invoice_type: tax.invoice_type,
        tax_payment_method: tax.tax_payment_method,
        tax_quote: tax.tax_quote,
        subject: t('Invoice Application'),
        company_name: values.company_name,
        tax_number: values.tax_number,
        email: values.email ?? '',
        content: values.content ?? '',
        topup_order_ids: [...selectedIds],
        refund_conflict_acknowledged: refundConflictAcked,
      })
    },
    [
      selectedIds,
      minInvoiceAmount,
      invoiceAmount,
      mutation,
      t,
      refundConflictAcked,
      tax,
    ]
  )

  const isSubmitDisabled =
    !tax.ready ||
    mutation.isPending ||
    selectedIds.size === 0 ||
    (minInvoiceAmount > 0 && invoiceAmount < minInvoiceAmount) ||
    (refundConflict?.has_refunds && !refundConflictAcked)

  const orderPageCount = Math.max(1, Math.ceil(filteredOrders.length / 10))
  const currentOrderPage = Math.min(orderPage, orderPageCount)
  const visibleOrders = filteredOrders.slice(
    (currentOrderPage - 1) * 10,
    currentOrderPage * 10
  )
  let orderRows: ReactNode
  if (ordersLoading) {
    orderRows = (
      <TableRow>
        <TableCell colSpan={5} className='text-center'>
          {t('Loading...')}
        </TableCell>
      </TableRow>
    )
  } else if (filteredOrders.length === 0) {
    orderRows = (
      <TableRow>
        <TableCell colSpan={5} className='text-muted-foreground text-center'>
          {t('No eligible orders')}
        </TableCell>
      </TableRow>
    )
  } else {
    orderRows = visibleOrders.map((order: TicketInvoiceOrder) => (
      <TableRow
        key={order.id}
        className='cursor-pointer'
        onClick={() => toggleOrder(order.id)}
      >
        <TableCell>
          <Checkbox
            checked={selectedIds.has(order.id)}
            onCheckedChange={() => toggleOrder(order.id)}
          />
        </TableCell>
        <TableCell className='truncate text-xs'>{order.trade_no}</TableCell>
        <TableCell className='text-xs'>{order.payment_method || '-'}</TableCell>
        <TableCell className='text-xs'>
          ¥{Number(order.money || 0).toFixed(2)}
        </TableCell>
        <TableCell className='text-xs'>
          {order.create_time ? formatTimestampToDate(order.create_time) : '-'}
        </TableCell>
      </TableRow>
    ))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[85vh] overflow-y-auto sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Apply for Invoice')}</DialogTitle>
        </DialogHeader>

        <div className='min-w-0 space-y-5'>
          {refundConflict?.has_refunds && (
            <Alert variant='warning'>
              <AlertDescription className='space-y-2'>
                <p className='font-medium'>{t('Refund conflict warning')}</p>
                <div className='flex items-center gap-2 pt-1'>
                  <Checkbox
                    id='refund-conflict-ack'
                    checked={refundConflictAcked}
                    onCheckedChange={(checked) =>
                      setRefundConflictAcked(checked === true)
                    }
                  />
                  <label
                    htmlFor='refund-conflict-ack'
                    className='cursor-pointer text-xs'
                  >
                    {t('Refund conflict acknowledge')}
                  </label>
                </div>
              </AlertDescription>
            </Alert>
          )}

          <InvoiceTaxFields
            legacyTotalCents={Math.round(invoiceAmount * 100)}
            ids={[...selectedIds]}
            open={open}
            onChange={setTax}
            type={tax.invoice_type}
            onTypeChange={(invoice_type) => {
              setOrderPage(1)
              setSelectedIds(new Set())
              setTax((current) => ({ ...current, invoice_type, ready: false }))
            }}
          />
          {/* Step 1: Order selection */}
          <div>
            <h4 className='mb-2 text-sm font-medium'>
              1. {t('Select Top-up Orders')}
            </h4>
            <Input
              type='search'
              className='mb-2'
              aria-label={t('Search order number or exact amount')}
              placeholder={t('Search order number or exact amount')}
              value={orderSearch}
              onChange={(event) => setOrderSearch(event.target.value)}
            />
            <p
              className='text-muted-foreground mb-2 text-sm'
              aria-live='polite'
            >
              {t('Matching orders: {{count}}', {
                count: filteredOrders.length,
              })}
            </p>
            <div className='rounded-md border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className='w-10'>
                      <Checkbox
                        checked={
                          filteredOrders.length > 0 &&
                          filteredOrders.every((order) =>
                            selectedIds.has(order.id)
                          )
                        }
                        onCheckedChange={toggleAll}
                      />
                    </TableHead>
                    <TableHead>{t('Trade No.')}</TableHead>
                    <TableHead className='w-[100px]'>
                      {t('Payment Method')}
                    </TableHead>
                    <TableHead className='w-[100px]'>
                      {t('Paid Amount')}
                    </TableHead>
                    <TableHead className='w-[160px]'>{t('Created')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>{orderRows}</TableBody>
              </Table>
            </div>
            <nav
              className='mt-2 flex items-center justify-end gap-2'
              aria-label={t('Select Top-up Orders')}
            >
              <span className='text-sm'>
                {t('Page {{current}} of {{total}}', {
                  current: currentOrderPage,
                  total: orderPageCount,
                })}
              </span>
              <Button
                type='button'
                variant='outline'
                size='icon'
                title={t('Previous page')}
                aria-label={t('Previous page')}
                disabled={ordersLoading || currentOrderPage <= 1}
                onClick={() => setOrderPage(currentOrderPage - 1)}
              >
                <ChevronLeft />
              </Button>
              <Button
                type='button'
                variant='outline'
                size='icon'
                title={t('Next page')}
                aria-label={t('Next page')}
                disabled={ordersLoading || currentOrderPage >= orderPageCount}
                onClick={() => setOrderPage(currentOrderPage + 1)}
              >
                <ChevronRight />
              </Button>
            </nav>
            <div className='bg-muted mt-2 flex flex-wrap items-center justify-between gap-2 rounded-md px-3 py-2 text-sm'>
              <span className='text-muted-foreground'>
                {t('Selected')}: {selectedIds.size}/{orders.length}{' '}
                {t('orders_unit')}
              </span>
              <span>
                <span className='text-muted-foreground mr-1'>
                  {t('Invoice Amount')}:
                </span>
                <span className='font-medium'>¥{invoiceAmount.toFixed(2)}</span>
              </span>
            </div>
            {minInvoiceAmount > 0 && (
              <p className='text-muted-foreground mt-2 text-xs'>
                {t('Minimum invoice amount: {{amount}} CNY', {
                  amount: minInvoiceAmount,
                })}
              </p>
            )}
          </div>

          {/* Step 2: Invoice details form */}
          <div>
            <h4 className='mb-2 text-sm font-medium'>
              2. {t('Fill in Invoice Header')}
            </h4>
            <Form {...form}>
              <form
                id='invoice-ticket-form'
                onSubmit={form.handleSubmit(onSubmit)}
                className='space-y-3'
              >
                <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='company_name'
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
                    name='tax_number'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Taxpayer ID')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            placeholder={t('Unified social credit code')}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
                <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
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
                    name='email'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Receiving Email')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            type='email'
                            placeholder={t('Email to receive invoice')}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
                <FormField
                  control={form.control}
                  name='content'
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
          {onBack && (
            <Button
              type='button'
              variant='outline'
              onClick={onBack}
              disabled={mutation.isPending}
            >
              {t('Back')}
            </Button>
          )}
          <Button
            type='submit'
            form='invoice-ticket-form'
            disabled={isSubmitDisabled}
          >
            {mutation.isPending ? t('Submitting...') : t('Submit Application')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
