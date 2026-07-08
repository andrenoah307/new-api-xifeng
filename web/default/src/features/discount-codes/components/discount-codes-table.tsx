import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  type SortingState,
  type VisibilityState,
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { useMediaQuery } from '@/hooks'
import { useTranslation } from 'react-i18next'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
} from '@/components/data-table'
import { getDiscountCodes, searchDiscountCodes } from '../api'
import {
  DISCOUNT_CODE_STATUS,
  getDiscountCodeStatusOptions,
} from '../constants'
import type { DiscountCode } from '../types'
import { useDiscountCodesColumns } from './discount-codes-columns'
import { useDiscountCodes } from './discount-codes-provider'

const route = getRouteApi('/_authenticated/discount-codes/')

function isDisabledRow(dc: DiscountCode) {
  return dc.status !== DISCOUNT_CODE_STATUS.ENABLED
}

export function DiscountCodesTable() {
  const { t } = useTranslation()
  const columns = useDiscountCodesColumns()
  const { refreshTrigger } = useDiscountCodes()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [rowSelection, setRowSelection] = useState({})
  const [sorting, setSorting] = useState<SortingState>([])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})

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
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
    ],
  })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'discount-codes',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      refreshTrigger,
    ],
    queryFn: async () => {
      const hasFilter = globalFilter?.trim()
      const params = {
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }

      const result = hasFilter
        ? await searchDiscountCodes({ ...params, keyword: globalFilter })
        : await getDiscountCodes(params)

      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const discountCodes = data?.items || []

  const table = useReactTable({
    data: discountCodes,
    columns,
    state: {
      sorting,
      columnVisibility,
      rowSelection,
      columnFilters,
      globalFilter,
      pagination,
    },
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    globalFilterFn: (row, _columnId, filterValue) => {
      const name = String(row.getValue('name')).toLowerCase()
      const id = String(row.getValue('id'))
      const code = String(row.original.code).toLowerCase()
      const searchValue = String(filterValue).toLowerCase()

      return (
        name.includes(searchValue) ||
        id.includes(searchValue) ||
        code.includes(searchValue)
      )
    },
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFacetedRowModel: getFacetedRowModel(),
    getFacetedUniqueValues: getFacetedUniqueValues(),
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: !globalFilter,
    pageCount: Math.ceil((data?.total || 0) / pagination.pageSize),
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  const statusOptions = useMemo(
    () => getDiscountCodeStatusOptions(t),
    [t]
  )

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Discount Codes Found')}
      emptyDescription={t(
        'No discount codes available. Create your first discount code to get started.'
      )}
      skeletonKeyPrefix='discount-codes-skeleton'
      toolbarProps={{
        searchPlaceholder: t('Filter by name, code or ID...'),
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: statusOptions,
          },
        ],
      }}
      getRowClassName={(row, { isMobile }) =>
        isDisabledRow(row.original)
          ? isMobile
            ? DISABLED_ROW_MOBILE
            : DISABLED_ROW_DESKTOP
          : undefined
      }
    />
  )
}
