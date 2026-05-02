import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { StatusBadge } from '@/components/status-badge'
import { Input } from '@/components/ui/input'
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
import { DataTablePagination } from '@/components/data-table/pagination'
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { formatTimestamp } from '@/lib/format'
import { getRiskIncidents } from '../../api'
import {
  riskQueryKeys,
  DECISION_MAP,
  SCOPE_OPTIONS,
} from '../../constants'

export function RiskIncidentsTable() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const pageSize = 10
  const [filters, setFilters] = useState({
    scope: '__all__',
    action: '__all__',
    keyword: '',
  })

  const params = useMemo(
    () => ({
      p: page,
      page_size: pageSize,
      scope: filters.scope === '__all__' ? undefined : filters.scope,
      action: filters.action === '__all__' ? undefined : filters.action,
      keyword: filters.keyword || undefined,
    }),
    [page, filters]
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
    columns: [],
    pageCount: Math.ceil(total / pageSize),
    state: {
      pagination: { pageIndex: page - 1, pageSize },
    },
    onPaginationChange: (updater) => {
      const next =
        typeof updater === 'function'
          ? updater({ pageIndex: page - 1, pageSize })
          : updater
      setPage(next.pageIndex + 1)
    },
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
  })

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Select
          value={filters.scope}
          onValueChange={(v) =>
            setFilters((p) => ({ ...p, scope: v }))
          }
        >
          <SelectTrigger className="h-8 w-[120px]">
            <SelectValue placeholder={t('Scope')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t('All')}</SelectItem>
            {SCOPE_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={filters.action}
          onValueChange={(v) =>
            setFilters((p) => ({ ...p, action: v }))
          }
        >
          <SelectTrigger className="h-8 w-[120px]">
            <SelectValue placeholder={t('Decision')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t('All')}</SelectItem>
            <SelectItem value="block">{t('Block')}</SelectItem>
            <SelectItem value="observe">{t('Observe')}</SelectItem>
          </SelectContent>
        </Select>
        <Input
          placeholder={t('Search...')}
          value={filters.keyword}
          onChange={(e) =>
            setFilters((p) => ({ ...p, keyword: e.target.value }))
          }
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
                          variant={
                            item.subject_type === 'token'
                              ? 'blue'
                              : 'success'
                          }
                        >
                          {item.subject_type === 'token'
                            ? 'Token'
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
