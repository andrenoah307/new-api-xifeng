import { useState, useMemo, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { Plus } from 'lucide-react'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
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
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { toast } from 'sonner'
import { getUserTickets, closeUserTicket } from '../api'
import {
  DEFAULT_PAGE_SIZE,
  getStatusOptions,
  getTypeOptions,
} from '../constants'
import { ticketQueryKeys } from '../lib/ticket-actions'
import { useTicketColumns } from '../components/ticket-columns'
import { CreateTicketDialog } from '../components/dialogs/create-ticket-dialog'

const route = getRouteApi('/_authenticated/tickets/')

const emptyStaffMap = new Map()

export default function TicketListPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const queryClient = useQueryClient()

  const [createOpen, setCreateOpen] = useState(false)
  const [statusFilter, setStatusFilter] = useState('__all__')
  const [typeFilter, setTypeFilter] = useState('__all__')
  const [closeTicketId, setCloseTicketId] = useState<number | null>(null)

  const {
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
  })

  const queryParams = useMemo(
    () => ({
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
      status: statusFilter === '__all__' ? undefined : statusFilter,
      type: typeFilter === '__all__' ? undefined : typeFilter,
    }),
    [pagination, statusFilter, typeFilter]
  )

  const { data, isLoading } = useQuery({
    queryKey: ticketQueryKeys.userList(queryParams),
    queryFn: () => getUserTickets(queryParams),
    placeholderData: (prev) => prev,
  })

  const items = useMemo(() => data?.items ?? [], [data])
  const totalCount = data?.total ?? 0

  const closeMutation = useMutation({
    mutationFn: closeUserTicket,
    onSuccess: () => {
      toast.success(t('Ticket closed'))
      setCloseTicketId(null)
      queryClient.invalidateQueries({ queryKey: ticketQueryKeys.userLists() })
    },
  })

  const columns = useTicketColumns({
    admin: false,
    showAssignee: false,
    staffMap: emptyStaffMap,
    onCloseTicket: setCloseTicketId,
  })

  const table = useReactTable({
    data: items,
    columns,
    pageCount: Math.ceil(totalCount / pagination.pageSize),
    state: { pagination },
    onPaginationChange,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  const handleRowClick = useCallback(
    (ticketId: number) => {
      navigate({ to: '/tickets/$ticketId', params: { ticketId: String(ticketId) } })
    },
    [navigate]
  )

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Tickets')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('View and manage support tickets')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="mr-1.5 h-4 w-4" />
            {t('Create Ticket')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className="space-y-3 sm:space-y-4">
            <div className="flex flex-wrap items-center gap-2">
              <Select value={statusFilter} onValueChange={setStatusFilter}>
                <SelectTrigger className="h-8 w-[120px]">
                  <SelectValue placeholder={t('Status')} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__all__">{t('All')}</SelectItem>
                  {getStatusOptions().map((o) => (
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
                            {flexRender(cell.column.columnDef.cell, cell.getContext())}
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
      <CreateTicketDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={(id) =>
          navigate({ to: '/tickets/$ticketId', params: { ticketId: String(id) } })
        }
      />
      <ConfirmDialog
        open={closeTicketId !== null}
        onOpenChange={(open) => !open && setCloseTicketId(null)}
        title={t('Close Ticket')}
        desc={items.find((t) => t.id === closeTicketId)?.type === 'refund'
          ? t('Closing will unfreeze the frozen refund quota. Are you sure?')
          : t('Are you sure you want to close this ticket?')}
        handleConfirm={() =>
          closeTicketId && closeMutation.mutate(closeTicketId)
        }
        isLoading={closeMutation.isPending}
      />
    </>
  )
}
