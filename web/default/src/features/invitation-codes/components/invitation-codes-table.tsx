import { useState, useMemo, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type SortingState,
  type VisibilityState,
} from '@tanstack/react-table'
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
  DataTableToolbar,
  TableSkeleton,
  TableEmpty,
  MobileCardList,
} from '@/components/data-table'
import { DataTablePagination } from '@/components/data-table/pagination'
import { PageFooterPortal } from '@/components/layout'
import { getInvitationCodes, searchInvitationCodes } from '../api'
import { DEFAULT_PAGE_SIZE } from '../constants'
import { invitationCodesQueryKeys } from '../lib/invitation-code-actions'
import { useInvitationCodesColumns } from './invitation-codes-columns'

const route = getRouteApi('/_authenticated/invitation-codes/')

export function InvitationCodesTable() {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')

  const [sorting, setSorting] = useState<SortingState>([])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
  const [rowSelection, setRowSelection] = useState({})

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
  const shouldSearch = keyword.length > 0

  const { data, isLoading } = useQuery({
    queryKey: invitationCodesQueryKeys.list({
      keyword,
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
    }),
    queryFn: () =>
      shouldSearch
        ? searchInvitationCodes(
            keyword,
            pagination.pageIndex + 1,
            pagination.pageSize
          )
        : getInvitationCodes(
            pagination.pageIndex + 1,
            pagination.pageSize
          ),
    placeholderData: (prev) => prev,
  })

  const items = useMemo(() => data?.items ?? [], [data])
  const totalCount = data?.total ?? 0
  const columns = useInvitationCodesColumns()

  const table = useReactTable({
    data: items,
    columns,
    pageCount: Math.ceil(totalCount / pagination.pageSize),
    state: {
      sorting,
      columnFilters,
      columnVisibility,
      rowSelection,
      pagination,
      globalFilter,
    },
    onRowSelectionChange: setRowSelection,
    onSortingChange: setSorting,
    onColumnFiltersChange,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange,
    onGlobalFilterChange,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    manualSorting: true,
    manualFiltering: true,
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  return (
    <>
      <div className="space-y-3 sm:space-y-4">
        <DataTableToolbar
          table={table}
          searchPlaceholder={t('Search by code, name or ID...')}
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
                  <TableRow
                    key={row.id}
                    data-state={row.getIsSelected() && 'selected'}
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
    </>
  )
}
