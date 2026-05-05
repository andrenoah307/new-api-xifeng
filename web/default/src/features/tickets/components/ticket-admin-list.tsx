import { useMemo, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { useMediaQuery } from '@/hooks'
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
import { DataTablePagination } from '@/components/data-table/pagination'
import { PageFooterPortal, SectionPageLayout } from '@/components/layout'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Input } from '@/components/ui/input'
import { getAdminTickets, getStaffList, type StaffUser } from '../api'
import {
  DEFAULT_PAGE_SIZE,
  getStatusOptions,
  getTypeOptions,
} from '../constants'
import { ticketQueryKeys } from '../lib/ticket-actions'
import { useTicketColumns } from '../components/ticket-columns'

const route = getRouteApi('/_authenticated/ticket-admin/')

export default function TicketAdminListPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const viewerIsAdmin = useIsAdmin()

  const search = route.useSearch()
  const routeNavigate = route.useNavigate()

  const scope = search.scope ?? (viewerIsAdmin ? 'all' : 'mine')
  const statusFilter = search.status || '__all__'
  const typeFilter = search.type || '__all__'
  const companyName = search.company_name || ''

  const setScope = useCallback(
    (val: string) => {
      routeNavigate({
        search: (prev: Record<string, unknown>) => ({
          ...prev,
          scope: val,
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
          company_name: '',
          page: 1,
        }),
      })
    },
    [routeNavigate]
  )

  const setCompanyName = useCallback(
    (val: string) => {
      routeNavigate({
        search: (prev: Record<string, unknown>) => ({
          ...prev,
          company_name: val,
          page: 1,
        }),
      })
    },
    [routeNavigate]
  )

  const showCompanySearch =
    typeFilter === '__all__' || typeFilter === 'invoice'

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
      company_name: companyName || undefined,
      scope,
    }),
    [pagination, statusFilter, typeFilter, keyword, companyName, scope]
  )

  const { data, isLoading } = useQuery({
    queryKey: ticketQueryKeys.adminList(queryParams),
    queryFn: () => getAdminTickets(queryParams),
    placeholderData: (prev) => prev,
  })

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
    state: { pagination, globalFilter, columnFilters },
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
      })
    },
    [navigate]
  )

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Ticket Admin')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('Manage support tickets as admin')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className="space-y-3 sm:space-y-4">
          <Tabs value={scope} onValueChange={setScope}>
            <TabsList>
              {viewerIsAdmin && (
                <TabsTrigger value="all">{t('All Tickets')}</TabsTrigger>
              )}
              <TabsTrigger value="mine">{t('My Tickets')}</TabsTrigger>
              <TabsTrigger value="unassigned">
                {t('Unassigned')}
              </TabsTrigger>
            </TabsList>
          </Tabs>

          <DataTableToolbar
            table={table}
            searchPlaceholder={t('Search tickets...')}
            additionalSearch={
              <div className="flex flex-wrap items-center gap-2">
                {showCompanySearch && (
                  <Input
                    className="h-8 w-[200px]"
                    placeholder={t('Invoice title (company)')}
                    value={companyName}
                    onChange={(e) => setCompanyName(e.target.value)}
                  />
                )}
                <Select value={statusFilter} onValueChange={setStatusFilter}>
                  <SelectTrigger className="h-8 w-[120px]">
                    <SelectValue placeholder={t('Status')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__all__">{t('All')}</SelectItem>
                    {getStatusOptions(true).map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {t(o.label)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select value={typeFilter} onValueChange={setTypeFilter}>
                  <SelectTrigger className="h-8 w-[130px]">
                    <SelectValue placeholder={t('Type')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__all__">{t('All')}</SelectItem>
                    {getTypeOptions(true).map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {t(o.label)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            }
          />
          {isMobile ? (
            <MobileCardList table={table} isLoading={isLoading} onRowClick={(row) => handleRowClick(row.original.id)} />
          ) : isLoading && items.length === 0 ? (
            <div className="rounded-md border">
              <Table>
                <TableBody>
                  <TableSkeleton table={table} rowCount={8} />
                </TableBody>
              </Table>
            </div>
          ) : items.length === 0 ? (
            <div className="rounded-md border">
              <Table>
                <TableBody>
                  <TableEmpty colSpan={columns.length} />
                </TableBody>
              </Table>
            </div>
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  {table.getHeaderGroups().map((hg) => (
                    <TableRow key={hg.id}>
                      {hg.headers.map((h) => (
                        <TableHead key={h.id}>
                          {h.isPlaceholder
                            ? null
                            : flexRender(h.column.columnDef.header, h.getContext())}
                        </TableHead>
                      ))}
                    </TableRow>
                  ))}
                </TableHeader>
                <TableBody>
                  {table.getRowModel().rows.map((row) => (
                    <TableRow
                      key={row.id}
                      className="cursor-pointer"
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
  )
}
