import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { StatusBadge } from '@/components/status-badge'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { DataTableFacetedFilter } from '@/components/data-table/toolbar/faceted-filter'
import { DataTablePagination } from '@/components/data-table/core/pagination'
import {
  getCoreRowModel,
  useReactTable,
  type ColumnFiltersState,
} from '@tanstack/react-table'
import { formatTimestamp } from '@/lib/format'
import { getRiskIncidents } from '../../api'
import {
  riskQueryKeys,
  ACTION_OPTIONS,
  DECISION_MAP,
  SCOPE_OPTIONS,
} from '../../constants'

// 桩列：仅为筛选 chip 提供 table.getColumn()，行由下方手写渲染
const filterColumns = [{ accessorKey: 'scope' }, { accessorKey: 'action' }]

export function RiskIncidentsTable() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const pageSize = 10
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [keyword, setKeyword] = useState('')

  const filterValues = (id: string) => {
    const value = columnFilters.find((f) => f.id === id)?.value
    return Array.isArray(value) ? (value as string[]) : []
  }
  const scopeFilter = filterValues('scope')
  const actionFilter = filterValues('action')

  const params = useMemo(
    () => ({
      p: page,
      page_size: pageSize,
      scope: scopeFilter.join(',') || undefined,
      action: actionFilter.join(',') || undefined,
      keyword: keyword || undefined,
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [page, columnFilters, keyword]
  )

  const { data, isLoading } = useQuery({
    queryKey: riskQueryKeys.incidents(params),
    queryFn: () => getRiskIncidents(params),
    placeholderData: (prev) => prev,
  })

  const items = data?.items ?? []
  const total = data?.total ?? 0

  const table = useReactTable({
    data: items,
    columns: filterColumns,
    pageCount: Math.ceil(total / pageSize),
    state: {
      pagination: { pageIndex: page - 1, pageSize },
      columnFilters,
    },
    onPaginationChange: (updater) => {
      const next =
        typeof updater === 'function'
          ? updater({ pageIndex: page - 1, pageSize })
          : updater
      setPage(next.pageIndex + 1)
    },
    onColumnFiltersChange: (updater) => {
      setColumnFilters((prev) =>
        typeof updater === 'function' ? updater(prev) : updater
      )
      setPage(1)
    },
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    manualFiltering: true,
  })

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <DataTableFacetedFilter
          column={table.getColumn('scope')}
          title={t('Scope')}
          options={SCOPE_OPTIONS}
        />
        <DataTableFacetedFilter
          column={table.getColumn('action')}
          title={t('Decision')}
          options={ACTION_OPTIONS}
        />
        <Input
          placeholder={t('Search...')}
          value={keyword}
          onChange={(e) => {
            setKeyword(e.target.value)
            setPage(1)
          }}
          className="h-8 w-[200px]"
        />
      </div>

      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Time')}</TableHead>
              <TableHead>{t('Subject')}</TableHead>
              <TableHead>{t('Group')}</TableHead>
              <TableHead>{t('Rule')}</TableHead>
              <TableHead>{t('Decision')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && items.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="text-center py-8">
                  {t('Loading...')}
                </TableCell>
              </TableRow>
            ) : items.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="text-center py-8 text-muted-foreground">
                  {t('No data')}
                </TableCell>
              </TableRow>
            ) : (
              items.map((item) => {
                const dc =
                  DECISION_MAP[item.decision] ?? DECISION_MAP.allow
                return (
                  <TableRow key={item.id}>
                    <TableCell className="text-xs">
                      {formatTimestamp(item.created_at)}
                    </TableCell>
                    <TableCell>
                      <div className="space-y-0.5">
                        <StatusBadge
                          copyable={false}
                          variant={
                            item.subject_type === 'token'
                              ? 'blue'
                              : 'success'
                          }
                        >
                          {item.subject_type === 'token'
                            ? t('Token')
                            : t('User')}
                        </StatusBadge>
                        <p className="text-muted-foreground text-xs">
                          #{item.subject_id}
                        </p>
                      </div>
                    </TableCell>
                    <TableCell>
                      <StatusBadge variant="cyan">
                        {item.group || '-'}
                      </StatusBadge>
                    </TableCell>
                    <TableCell className="text-sm">
                      {item.rule_name || '-'}
                    </TableCell>
                    <TableCell>
                      <StatusBadge
                        copyable={false}
                        variant={dc.variant as 'danger' | 'warning' | 'neutral'}
                      >
                        {t(dc.labelKey)}
                      </StatusBadge>
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>
      <DataTablePagination table={table} />
    </div>
  )
}
