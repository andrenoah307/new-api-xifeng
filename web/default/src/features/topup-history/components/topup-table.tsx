import { useState, useMemo, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type VisibilityState,
} from '@tanstack/react-table'
import { useMediaQuery, useDebounce } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { useIsAdmin } from '@/hooks/use-admin'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  DataTableToolbar,
  TableSkeleton,
  TableEmpty,
  MobileCardList,
} from '@/components/data-table'
import { DataTablePagination } from '@/components/data-table/core/pagination'
import { PageFooterPortal } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { toast } from 'sonner'
import { getTopups, completeTopupOrder } from '../api'
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
  const queryClient = useQueryClient()
  const isMobile = useMediaQuery('(max-width: 640px)')

  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
  const [confirmTradeNo, setConfirmTradeNo] = useState<string | null>(null)

  const search = route.useSearch()
  const navigate = route.useNavigate()

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search,
    navigate,
    pagination: {
      defaultPage: 1,
      defaultPageSize: isMobile ? 10 : DEFAULT_PAGE_SIZE,
    },
    globalFilter: { enabled: true, key: 'keyword' },
  })

  // 状态/日期筛选也进 URL：刷新、返回、分享链接时保留（与 keyword/page 一致）
  const statusFilter = search.status ?? '__all__'
  const dateStart = useMemo(
    () => (search.start ? new Date(search.start * 1000) : undefined),
    [search.start]
  )
  const dateEnd = useMemo(
    () => (search.end ? new Date(search.end * 1000) : undefined),
    [search.end]
  )

  const setStatusFilter = useCallback(
    (v: string | null) => {
      navigate({
        search: (prev) => ({
          ...prev,
          status: !v || v === '__all__' ? undefined : v,
          page: undefined,
        }),
        replace: true,
      })
    },
    [navigate]
  )

  const keyword = globalFilter?.trim() || ''
  const debouncedKeyword = useDebounce(keyword, 1000)

  const queryParams = useMemo(
    () => ({
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
      keyword: debouncedKeyword || undefined,
      status: statusFilter === '__all__' ? undefined : statusFilter,
      start_time: dateStart
        ? Math.floor(dateStart.getTime() / 1000)
        : undefined,
      end_time: dateEnd
        ? Math.floor(dateEnd.getTime() / 1000)
        : undefined,
    }),
    [pagination, debouncedKeyword, statusFilter, dateStart, dateEnd]
  )

  const { data, isLoading } = useQuery({
    queryKey: topupQueryKeys.list({ ...queryParams, admin }),
    queryFn: () => getTopups(queryParams as Parameters<typeof getTopups>[0], admin),
    placeholderData: (prev) => prev,
  })

  const items = useMemo(() => data?.items ?? [], [data])
  const totalCount = data?.total ?? 0

  const completeMutation = useMutation({
    mutationFn: (tradeNo: string) => completeTopupOrder(tradeNo),
    onSuccess: (ok) => {
      if (!ok) return
      toast.success(t('Operation successful'))
      setConfirmTradeNo(null)
      queryClient.invalidateQueries({
        queryKey: topupQueryKeys.lists(),
      })
    },
  })

  const columns = useTopupColumns(admin)

  const actionsColumn = useMemo(() => {
    if (!admin) return null
    return {
      id: 'actions',
      header: t('Actions'),
      cell: ({ row }: { row: { original: { status: string; trade_no: string } } }) => {
        if (row.original.status !== 'pending') return null
        return (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setConfirmTradeNo(row.original.trade_no)}
          >
            {t('Complete Order')}
          </Button>
        )
      },
      size: 120,
      meta: { label: t('Actions') },
    }
  }, [admin, t])

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

  const statusItems = useMemo(
    () =>
      TOPUP_STATUS_OPTIONS.map((opt) => ({ value: opt.value, label: t(opt.label) })),
    [t]
  )

  const handleDateChange = useCallback(
    (range: { start?: Date; end?: Date }) => {
      navigate({
        search: (prev) => ({
          ...prev,
          start: range.start
            ? Math.floor(range.start.getTime() / 1000)
            : undefined,
          end: range.end ? Math.floor(range.end.getTime() / 1000) : undefined,
          page: undefined,
        }),
        replace: true,
      })
    },
    [navigate]
  )

  return (
    <>
      <div className="space-y-3 sm:space-y-4">
        <DataTableToolbar
          table={table}
          searchPlaceholder={t(admin ? 'Search by order number, username or user ID...' : 'Search by order number...')}
          additionalSearch={
            <div className="flex flex-wrap items-center gap-2">
              <Select items={statusItems} value={statusFilter} onValueChange={setStatusFilter}>
                <SelectTrigger className="h-8 w-[120px]">
                  <SelectValue placeholder={t('Status')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {TOPUP_STATUS_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {t(opt.label)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <CompactDateTimeRangePicker
                start={dateStart}
                end={dateEnd}
                onChange={handleDateChange}
                className="w-auto"
              />
            </div>
          }
        />
        {isMobile ? (
          <MobileCardList table={table} isLoading={isLoading} />
        ) : isLoading && items.length === 0 ? (
          <div className="rounded-md border">
            <Table>
              <TableBody>
                <TableSkeleton table={table} rowCount={10} />
              </TableBody>
            </Table>
          </div>
        ) : items.length === 0 ? (
          <div className="rounded-md border">
            <Table>
              <TableBody>
                <TableEmpty colSpan={allColumns.length} />
              </TableBody>
            </Table>
          </div>
        ) : (
          <div className="rounded-md border">
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
    </>
  )
}
