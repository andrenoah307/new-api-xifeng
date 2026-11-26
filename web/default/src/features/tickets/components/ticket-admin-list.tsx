import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type RowSelectionState,
} from '@tanstack/react-table'
import { Download, MailOpen } from 'lucide-react'
import { useMemo, useEffect, useCallback, useState } from 'react'
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
import { PageFooterPortal, SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
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
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useMediaQuery } from '@/hooks'
import { useIsAdmin } from '@/hooks/use-admin'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  batchUpdateTicketStatus,
  batchMarkTicketsRead,
  getAdminTickets,
  getStaffList,
  type StaffUser,
} from '../api'
import { useTicketColumns } from '../components/ticket-columns'
import {
  DEFAULT_PAGE_SIZE,
  getStatusOptions,
  getTypeOptions,
} from '../constants'
import { ticketQueryKeys } from '../lib/ticket-actions'
import { ExportInvoiceDialog } from './dialogs/export-invoice-dialog'
import { FinanceRefundList } from './finance-refund-list'

const route = getRouteApi('/_authenticated/ticket-admin/')

export default function TicketAdminListPage() {
  const role = useAuthStore((state) => state.auth.user?.role)
  if (role === ROLE.FINANCE) return <FinanceRefundList />
  return <StaffTicketList />
}

function StaffTicketList() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const viewerIsAdmin = useIsAdmin()
  const viewerRole = useAuthStore((state) => state.auth.user?.role)

  const search = route.useSearch()
  const routeNavigate = route.useNavigate()

  const [exportOpen, setExportOpen] = useState(false)
  const [reversalExportOpen, setReversalExportOpen] = useState(false)
  const queryClient = useQueryClient()
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
  const [batchStatus, setBatchStatus] = useState('2')
  const [confirmation, setConfirmation] = useState<{
    ids: number[]
    status: number | 'read'
  } | null>(null)
  const batchMutation = useMutation({
    mutationFn: (request: { ids: number[]; status: number | 'read' }) =>
      request.status === 'read'
        ? batchMarkTicketsRead(request.ids)
        : batchUpdateTicketStatus(request.ids, request.status),
    onSuccess: (success, request) => {
      if (!success) return
      toast.success(
        request.status === 'read'
          ? t('Tickets marked as read')
          : t('Ticket status updated')
      )
      setConfirmation(null)
      setRowSelection({})
      void queryClient.invalidateQueries({ queryKey: ticketQueryKeys.all })
    },
  })

  const scope = search.scope ?? (viewerIsAdmin ? 'all' : 'mine')
  const statusFilter = search.status || '__all__'
  const typeFilter = search.type || '__all__'

  const setScope = useCallback(
    (val: string) => {
      routeNavigate({
        search: (prev: Record<string, unknown>) => ({
          ...prev,
          scope: val as 'all' | 'mine' | 'unassigned',
          page: 1,
        }),
      })
    },
    [routeNavigate]
  )

  const setStatusFilter = useCallback(
    (val: string) => {
      routeNavigate({
        search: (prev: Record<string, unknown>) => ({
          ...prev,
          status: val === '__all__' ? '' : val,
          page: 1,
        }),
      })
    },
    [routeNavigate]
  )

  const setTypeFilter = useCallback(
    (val: string) => {
      routeNavigate({
        search: (prev: Record<string, unknown>) => ({
          ...prev,
          type: val === '__all__' ? '' : val,
          page: 1,
        }),
      })
    },
    [routeNavigate]
  )

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

  const { data: staffList } = useQuery({
    queryKey: ticketQueryKeys.staff(),
    queryFn: getStaffList,
    staleTime: 5 * 60 * 1000,
  })

  const staffMap = useMemo(() => {
    const map = new Map<number, StaffUser>()
    ;(staffList ?? []).forEach((s) => map.set(s.id, s))
    return map
  }, [staffList])

  const queryParams = useMemo(
    () => ({
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
      status: statusFilter === '__all__' ? undefined : statusFilter,
      type: typeFilter === '__all__' ? undefined : typeFilter,
      keyword: keyword || undefined,
      scope,
      unread: search.unread || undefined,
    }),
    [pagination, statusFilter, typeFilter, keyword, scope, search.unread]
  )

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ticketQueryKeys.adminList(queryParams),
    queryFn: () => getAdminTickets(queryParams),
    placeholderData: (prev) => prev,
    refetchInterval: 30_000,
  })
  useEffect(() => {
    setRowSelection({})
  }, [queryParams])

  const items = useMemo(() => data?.items ?? [], [data])
  const totalCount = data?.total ?? 0

  const columns = useTicketColumns({
    admin: true,
    showAssignee: viewerIsAdmin,
    staffMap,
  })

  const table = useReactTable({
    data: items,
    columns,
    pageCount: Math.ceil(totalCount / pagination.pageSize),
    state: { pagination, globalFilter, columnFilters, rowSelection },
    getRowId: (row) => String(row.id),
    onRowSelectionChange: setRowSelection,
    enableRowSelection: !isFetching && !batchMutation.isPending,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    manualFiltering: true,
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  const handleRowClick = useCallback(
    (ticketId: number) => {
      navigate({
        to: '/ticket-admin/$ticketId',
        params: { ticketId: String(ticketId) },
        search,
      })
    },
    [navigate, search]
  )

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Ticket Admin')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Manage support tickets as admin')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Content>
          <div className='space-y-3 sm:space-y-4'>
            <Tabs value={scope} onValueChange={setScope}>
              <TabsList>
                {viewerIsAdmin && (
                  <TabsTrigger value='all'>{t('All Tickets')}</TabsTrigger>
                )}
                <TabsTrigger value='mine'>{t('My Tickets')}</TabsTrigger>
                <TabsTrigger value='unassigned'>{t('Unassigned')}</TabsTrigger>
              </TabsList>
            </Tabs>

            <DataTableToolbar
              table={table}
              searchPlaceholder={t(
                'Search by subject, username, ID, company name or payee...'
              )}
              additionalSearch={
                <div className='flex flex-wrap items-center gap-2'>
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
                      <SelectItem value='__all__'>{t('All')}</SelectItem>
                      {getStatusOptions(true).map((o) => (
                        <SelectItem key={o.value} value={o.value}>
                          {t(o.label)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Select
                    value={typeFilter}
                    onValueChange={(value) => {
                      if (value != null) setTypeFilter(value)
                    }}
                  >
                    <SelectTrigger className='h-8 w-[130px]'>
                      <SelectValue placeholder={t('Type')} />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='__all__'>{t('All')}</SelectItem>
                      {getTypeOptions(true).map((o) => (
                        <SelectItem key={o.value} value={o.value}>
                          {t(o.label)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => setExportOpen(true)}
                  >
                    <Download className='mr-1.5 h-4 w-4' />
                    {t('Export Invoice')}
                  </Button>
                  {[3, 10, 100].includes(viewerRole ?? 0) && (
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => setReversalExportOpen(true)}
                    >
                      <Download className='mr-1.5 h-4 w-4' />
                      {t('Export Reversal Invoices')}
                    </Button>
                  )}
                  <Select
                    value={search.unread || '__all__'}
                    onValueChange={(value) => {
                      if (value != null) {
                        routeNavigate({
                          search: (prev) => ({
                            ...prev,
                            unread:
                              value === '__all__'
                                ? ''
                                : (value as 'true' | 'false'),
                            page: 1,
                          }),
                        })
                      }
                    }}
                  >
                    <SelectTrigger
                      className='h-8 w-[160px]'
                      aria-label={t('Message status')}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value='__all__'>
                          {t('All messages')}
                        </SelectItem>
                        <SelectItem value='true'>
                          {t('Unread messages')}
                        </SelectItem>
                        <SelectItem value='false'>
                          {t('Read messages')}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </div>
              }
            />
            {table.getSelectedRowModel().rows.length > 0 && (
              <div className='flex flex-wrap items-center gap-2'>
                <span className='text-sm'>
                  {t('Selected {{count}} tickets', {
                    count: table.getSelectedRowModel().rows.length,
                  })}
                </span>
                <Select
                  value={batchStatus}
                  onValueChange={(value) => {
                    if (value != null) setBatchStatus(value)
                  }}
                  disabled={batchMutation.isPending}
                >
                  <SelectTrigger
                    className='h-8 w-[150px]'
                    aria-label={t('Status')}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {getStatusOptions(true).map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {t(option.label)}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <Button
                  size='sm'
                  disabled={
                    isFetching ||
                    batchMutation.isPending ||
                    table.getSelectedRowModel().rows.length > 100
                  }
                  onClick={() =>
                    setConfirmation({
                      ids: table
                        .getSelectedRowModel()
                        .rows.map((row) => row.original.id),
                      status: Number(batchStatus),
                    })
                  }
                >
                  {t('Update selected tickets')}
                </Button>
                <Button
                  size='sm'
                  variant='outline'
                  disabled={
                    isFetching ||
                    batchMutation.isPending ||
                    table.getSelectedRowModel().rows.length > 100
                  }
                  onClick={() =>
                    setConfirmation({
                      ids: table
                        .getSelectedRowModel()
                        .rows.map((row) => row.original.id),
                      status: 'read',
                    })
                  }
                >
                  <MailOpen className='size-4' aria-hidden />
                  {t('Mark as read')}
                </Button>
                <Button
                  size='sm'
                  variant='ghost'
                  disabled={batchMutation.isPending}
                  onClick={() => setRowSelection({})}
                >
                  {t('Clear selection')}
                </Button>
              </div>
            )}
            {isMobile && (
              <MobileCardList
                table={table}
                isLoading={isLoading}
                onRowClick={(row) => handleRowClick(row.original.id)}
              />
            )}
            {!isMobile && isLoading && items.length === 0 && (
              <div className='rounded-md border'>
                <Table>
                  <TableBody>
                    <TableSkeleton table={table} rowCount={8} />
                  </TableBody>
                </Table>
              </div>
            )}
            {!isMobile && !isLoading && items.length === 0 && (
              <div className='rounded-md border'>
                <Table>
                  <TableBody>
                    <TableEmpty colSpan={columns.length} />
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
                        {hg.headers.map((h) => (
                          <TableHead key={h.id}>
                            {h.isPlaceholder
                              ? null
                              : flexRender(
                                  h.column.columnDef.header,
                                  h.getContext()
                                )}
                          </TableHead>
                        ))}
                      </TableRow>
                    ))}
                  </TableHeader>
                  <TableBody>
                    {table.getRowModel().rows.map((row) => (
                      <TableRow
                        key={row.id}
                        className='cursor-pointer'
                        onClick={() => handleRowClick(row.original.id)}
                      >
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
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <ExportInvoiceDialog open={exportOpen} onOpenChange={setExportOpen} />
      <ExportInvoiceDialog
        refund
        open={reversalExportOpen}
        onOpenChange={setReversalExportOpen}
      />
      <ConfirmDialog
        open={confirmation !== null}
        onOpenChange={(open) => {
          if (!open && !batchMutation.isPending) setConfirmation(null)
        }}
        title={
          confirmation?.status === 'read'
            ? t('Mark as read')
            : t('Confirm Batch Update')
        }
        desc={
          confirmation?.status === 'read'
            ? t('Mark {{count}} selected tickets as read for the team?', {
                count: confirmation.ids.length,
              })
            : t(
                'Update {{count}} tickets to {{status}}? Closing refund tickets cancels pending refunds.',
                {
                  count: confirmation?.ids.length ?? 0,
                  status: t(
                    getStatusOptions(true).find(
                      (option) => Number(option.value) === confirmation?.status
                    )?.label ?? ''
                  ),
                }
              )
        }
        isLoading={batchMutation.isPending}
        destructive={confirmation?.status === 4}
        handleConfirm={() => {
          if (confirmation) batchMutation.mutate(confirmation)
        }}
      />
    </>
  )
}
