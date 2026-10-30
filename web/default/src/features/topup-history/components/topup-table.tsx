import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type VisibilityState,
} from '@tanstack/react-table'
import { Pencil } from 'lucide-react'
import { useState, useMemo, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  DataTableToolbar,
  TableSkeleton,
  TableEmpty,
  MobileCardList,
} from '@/components/data-table'
import { DataTablePagination } from '@/components/data-table/pagination'
import { PageFooterPortal } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ticketQueryKeys } from '@/features/tickets/lib/ticket-actions'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { useMediaQuery, useDebounce } from '@/hooks'
import { useIsAdmin } from '@/hooks/use-admin'
import { useStatus } from '@/hooks/use-status'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { useAuthStore } from '@/stores/auth-store'

import {
  getTopups,
  completeTopupOrder,
  updateTopupInvoiceType,
  type TopupRecord,
} from '../api'
import {
  DEFAULT_PAGE_SIZE,
  TOPUP_STATUS_OPTIONS,
  topupQueryKeys,
} from '../constants'
import { useTopupColumns } from './topup-columns'

const route = getRouteApi('/_authenticated/topup-history/')

export function TopupTable() {
  const { t } = useTranslation()
  const admin = useIsAdmin()
  const { status } = useStatus()
  const financeEnabled = !!status?.finance_module_enabled
  const userId = useAuthStore((state) => state.auth.user?.id)
  const queryClient = useQueryClient()
  const isMobile = useMediaQuery('(max-width: 640px)')

  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
  const [statusFilter, setStatusFilter] = useState('__all__')
  const [invoiceTypeFilter, setInvoiceTypeFilter] = useState('__all__')
  const [editingInvoice, setEditingInvoice] = useState<TopupRecord | null>(null)
  const [invoiceType, setInvoiceType] = useState('none')
  const [dateStart, setDateStart] = useState<Date | undefined>()
  const [dateEnd, setDateEnd] = useState<Date | undefined>()
  const [confirmTradeNo, setConfirmTradeNo] = useState<string | null>(null)

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: {
      defaultPage: 1,
      defaultPageSize: isMobile ? 10 : DEFAULT_PAGE_SIZE,
    },
    globalFilter: { enabled: true, key: 'keyword' },
  })

  const keyword = globalFilter?.trim() || ''
  const debouncedKeyword = useDebounce(keyword, 1000)

  const queryParams = useMemo(
    () => ({
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
      keyword: debouncedKeyword || undefined,
      status: statusFilter === '__all__' ? undefined : statusFilter,
      invoice_type:
        financeEnabled && admin && invoiceTypeFilter !== '__all__'
          ? invoiceTypeFilter
          : undefined,
      start_time: dateStart
        ? Math.floor(dateStart.getTime() / 1000)
        : undefined,
      end_time: dateEnd ? Math.floor(dateEnd.getTime() / 1000) : undefined,
    }),
    [
      pagination,
      debouncedKeyword,
      statusFilter,
      dateStart,
      dateEnd,
      admin,
      invoiceTypeFilter,
      financeEnabled,
    ]
  )

  const { data, isLoading } = useQuery({
    queryKey: topupQueryKeys.list({ ...queryParams, admin, userId }),
    queryFn: () =>
      getTopups(queryParams as Parameters<typeof getTopups>[0], admin),
    gcTime: 0,
  })

  const items = useMemo(() => data?.items ?? [], [data])
  const totalCount = data?.total ?? 0

  const completeMutation = useMutation({
    mutationFn: (tradeNo: string) => completeTopupOrder(tradeNo),
    onSuccess: () => {
      toast.success(t('Operation successful'))
      setConfirmTradeNo(null)
      queryClient.invalidateQueries({
        queryKey: topupQueryKeys.lists(),
      })
    },
  })

  const columns = useTopupColumns(admin)

  const invoiceMutation = useMutation({
    mutationFn: async () => {
      if (!admin || !editingInvoice) return
      await updateTopupInvoiceType(
        editingInvoice.id,
        invoiceType,
        editingInvoice.invoice_type ?? ''
      )
    },
    onSuccess: () => {
      setEditingInvoice(null)
      toast.success(t('Operation successful'))
      queryClient.invalidateQueries({ queryKey: topupQueryKeys.lists() })
      queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.eligibleOrders(),
      })
    },
  })

  const actionsColumn = useMemo(() => {
    if (!admin) return null
    return {
      id: 'actions',
      header: t('Actions'),
      cell: ({ row }: { row: { original: TopupRecord } }) => {
        if (
          financeEnabled &&
          row.original.status === 'success' &&
          !row.original.invoice_status
        ) {
          return (
            <Button
              variant='ghost'
              size='icon'
              title={t('Invoice type')}
              aria-label={t('Invoice type')}
              onClick={() => {
                setEditingInvoice(row.original)
                setInvoiceType(row.original.invoice_type || 'none')
              }}
            >
              <Pencil className='size-4' />
            </Button>
          )
        }
        if (row.original.status !== 'pending') return null
        return (
          <Button
            variant='outline'
            size='sm'
            onClick={() => setConfirmTradeNo(row.original.trade_no)}
          >
            {t('Complete Order')}
          </Button>
        )
      },
      size: 120,
      meta: { label: t('Actions') },
    }
  }, [admin, t, financeEnabled])

  const allColumns = useMemo(
    () => (actionsColumn ? [...columns, actionsColumn] : columns),
    [columns, actionsColumn]
  )

  const table = useReactTable({
    data: items,
    columns: allColumns,
    pageCount: Math.ceil(totalCount / pagination.pageSize),
    state: {
      columnFilters,
      columnVisibility,
      pagination,
      globalFilter,
    },
    onColumnFiltersChange,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange,
    onGlobalFilterChange,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    manualFiltering: true,
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  const handleDateChange = useCallback(
    (range: { start?: Date; end?: Date }) => {
      setDateStart(range.start)
      setDateEnd(range.end)
    },
    []
  )

  return (
    <>
      <div className='space-y-3 sm:space-y-4'>
        <DataTableToolbar
          table={table}
          searchPlaceholder={t(
            admin
              ? 'Search by order number, username or user ID...'
              : 'Search by order number...'
          )}
          additionalSearch={
            <div className='flex flex-wrap items-center gap-2'>
              {admin && financeEnabled && (
                <Select
                  value={invoiceTypeFilter}
                  onValueChange={(value) => {
                    if (value != null) {
                      setInvoiceTypeFilter(value)
                      onPaginationChange({ ...pagination, pageIndex: 0 })
                    }
                  }}
                >
                  <SelectTrigger
                    className='h-8 w-[190px]'
                    aria-label={t('Invoice type')}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='__all__'>{t('All')}</SelectItem>
                    <SelectItem value='ordinary'>
                      {t('Ordinary VAT invoice')}
                    </SelectItem>
                    <SelectItem value='special'>
                      {t('Special VAT invoice')}
                    </SelectItem>
                    <SelectItem value='none'>-</SelectItem>
                  </SelectContent>
                </Select>
              )}
              <Select
                value={statusFilter}
                onValueChange={(value) => {
                  if (value != null) setStatusFilter(value)
                }}
              >
                <SelectTrigger className='h-8 w-[120px]'>
                  <SelectValue placeholder={t('Status')} />
                </SelectTrigger>
                <SelectContent>
                  {TOPUP_STATUS_OPTIONS.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {t(opt.label)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <CompactDateTimeRangePicker
                start={dateStart}
                end={dateEnd}
                onChange={handleDateChange}
                className='w-auto'
              />
            </div>
          }
        />
        {isMobile && <MobileCardList table={table} isLoading={isLoading} />}
        {!isMobile && isLoading && items.length === 0 && (
          <div className='rounded-md border'>
            <Table>
              <TableBody>
                <TableSkeleton table={table} rowCount={10} />
              </TableBody>
            </Table>
          </div>
        )}
        {!isMobile && !isLoading && items.length === 0 && (
          <div className='rounded-md border'>
            <Table>
              <TableBody>
                <TableEmpty colSpan={allColumns.length} />
              </TableBody>
            </Table>
          </div>
        )}
        {!isMobile && items.length > 0 && (
          <div className='rounded-md border'>
            <Table>
              <TableHeader>
                {table.getHeaderGroups().map((hg) => (
                  <TableRow key={hg.id}>
                    {hg.headers.map((header) => (
                      <TableHead key={header.id}>
                        {header.isPlaceholder
                          ? null
                          : flexRender(
                              header.column.columnDef.header,
                              header.getContext()
                            )}
                      </TableHead>
                    ))}
                  </TableRow>
                ))}
              </TableHeader>
              <TableBody>
                {table.getRowModel().rows.map((row) => (
                  <TableRow key={row.id}>
                    {row.getVisibleCells().map((cell) => (
                      <TableCell key={cell.id}>
                        {flexRender(
                          cell.column.columnDef.cell,
                          cell.getContext()
                        )}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
      <PageFooterPortal>
        <DataTablePagination table={table} />
      </PageFooterPortal>
      <ConfirmDialog
        open={confirmTradeNo !== null}
        onOpenChange={(open) => !open && setConfirmTradeNo(null)}
        title={t('Confirm Manual Fulfillment')}
        desc={t(
          'Are you sure you want to mark this order as successful and credit the user?'
        )}
        handleConfirm={() =>
          confirmTradeNo && completeMutation.mutate(confirmTradeNo)
        }
        isLoading={completeMutation.isPending}
      />
      {admin && (
        <ConfirmDialog
          open={!!editingInvoice}
          onOpenChange={(open) => {
            if (!open && !invoiceMutation.isPending) setEditingInvoice(null)
          }}
          title={t('Invoice type')}
          desc={editingInvoice?.trade_no ?? ''}
          confirmText={t('Save')}
          isLoading={invoiceMutation.isPending}
          handleConfirm={() => invoiceMutation.mutate()}
        >
          <fieldset className='space-y-2' disabled={invoiceMutation.isPending}>
            {['ordinary', 'special', 'none'].map((type) => (
              <label key={type} className='flex items-center gap-2'>
                <input
                  type='radio'
                  name='topup-invoice-type'
                  value={type}
                  checked={invoiceType === type}
                  onChange={() => setInvoiceType(type)}
                />
                {type === 'ordinary' && t('Ordinary VAT invoice')}
                {type === 'special' && t('Special VAT invoice')}
                {type === 'none' && '-'}
              </label>
            ))}
          </fieldset>
        </ConfirmDialog>
      )}
    </>
  )
}
