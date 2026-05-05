import { useState, useMemo, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestampToDate } from '@/lib/format'
import {
  createInvoiceTicket,
  getEligibleInvoiceOrders,
  type TicketInvoiceOrder,
} from '../../api'
import { ticketQueryKeys } from '../../lib/ticket-actions'

const schema = z.object({
  company_name: z.string().min(1),
  tax_number: z.string().min(1),
  email: z.string().email(),
  content: z.string().max(100).optional(),
})

type FormValues = z.infer<typeof schema>

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
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())

  const { data: orders = [], isLoading: ordersLoading } = useQuery({
    queryKey: ticketQueryKeys.eligibleOrders(),
    queryFn: getEligibleInvoiceOrders,
    enabled: open,
  })

  useEffect(() => {
    if (!open) setSelectedIds(new Set())
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
    },
  })

  const mutation = useMutation({
    mutationFn: createInvoiceTicket,
    onSuccess: (data) => {
      toast.success(t('Invoice ticket submitted'))
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
      if (selectedIds.size === 0) {
        toast.error(t('Please select at least one order'))
        return
      }
      mutation.mutate({
        subject: t('Invoice Application'),
        company_name: values.company_name,
        tax_number: values.tax_number,
        email: values.email ?? '',
        content: values.content ?? '',
        topup_order_ids: Array.from(selectedIds),
      })
    },
    [selectedIds, mutation, t]
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('Apply for Invoice')}</DialogTitle>
        </DialogHeader>

        <div className="space-y-5">
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
            <div className="bg-muted mt-2 flex items-center justify-between rounded-md px-3 py-2 text-sm">
              <span className="text-muted-foreground">
                {t('Selected')}: {selectedIds.size}/{orders.length} {t('orders_unit')}
              </span>
              <span>
                <span className="text-muted-foreground mr-1">
                  {t('Invoice Amount')}:
                </span>
                <span className="font-medium">
                  ¥{invoiceAmount.toFixed(2)}
                </span>
              </span>
            </div>
          </div>

          {/* Step 2: Invoice details form */}
          <div>
            <h4 className="mb-2 text-sm font-medium">
              2. {t('Fill in Invoice Header')}
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
                      value="*信息技术服务*技术服务费"
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
            disabled={mutation.isPending || selectedIds.size === 0}
          >
            {mutation.isPending ? t('Submitting...') : t('Submit Application')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
