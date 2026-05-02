import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
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
import { getRiskSubjects, unblockSubject } from '../../api'
import {
  riskQueryKeys,
  safeParseJSON,
  SUBJECT_STATUS_MAP,
  SCOPE_OPTIONS,
} from '../../constants'

export function RiskSubjectsTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const pageSize = 10
  const [filters, setFilters] = useState({
    scope: '__all__',
    status: '__all__',
    keyword: '',
  })

  const params = useMemo(
    () => ({
      p: page,
      page_size: pageSize,
      scope: filters.scope === '__all__' ? undefined : filters.scope,
      status: filters.status === '__all__' ? undefined : filters.status,
      keyword: filters.keyword || undefined,
    }),
    [page, filters]
  )

  const { data, isLoading } = useQuery({
    queryKey: riskQueryKeys.subjects(params),
    queryFn: () => getRiskSubjects(params),
    placeholderData: (prev) => prev,
  })

  const items = data?.items ?? []
  const total = data?.total ?? 0

  const unblockMutation = useMutation({
    mutationFn: (record: {
      subject_type: string
      subject_id: string | number
      group: string
    }) => unblockSubject(record.subject_type, record.subject_id, record.group),
    onSuccess: () => {
      toast.success(t('Unblock Subject'))
      queryClient.invalidateQueries({ queryKey: riskQueryKeys.all })
    },
  })

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
          value={filters.status}
          onValueChange={(v) =>
            setFilters((p) => ({ ...p, status: v }))
          }
        >
          <SelectTrigger className="h-8 w-[120px]">
            <SelectValue placeholder={t('Status')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t('All')}</SelectItem>
            <SelectItem value="blocked">{t('Blocked')}</SelectItem>
            <SelectItem value="observe">{t('Observing')}</SelectItem>
            <SelectItem value="normal">{t('Normal')}</SelectItem>
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
              <TableHead>{t('Subject')}</TableHead>
              <TableHead>{t('Group')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Risk Score')}</TableHead>
              <TableHead>{t('Hit Rules')}</TableHead>
              <TableHead>{t('Last Seen')}</TableHead>
              <TableHead>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && items.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="text-center py-8">
                  {t('Loading...')}
                </TableCell>
              </TableRow>
            ) : items.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="text-center py-8 text-muted-foreground">
                  {t('No data')}
                </TableCell>
              </TableRow>
            ) : (
              items.map((item, idx) => {
                const sc = SUBJECT_STATUS_MAP[item.status] ?? SUBJECT_STATUS_MAP.normal
                const ruleNames = safeParseJSON(
                  (item as Record<string, unknown>).active_rule_names,
                  []
                ) as string[]
                return (
                  <TableRow key={idx}>
                    <TableCell>
                      <div className="space-y-0.5">
                        <StatusBadge
                          variant={
                            item.type === 'token' ? 'blue' : 'success'
                          }
                        >
                          {item.type === 'token' ? 'Token' : t('User')}
                        </StatusBadge>
                        <p className="text-muted-foreground text-xs">
                          #{item.id}
                        </p>
                      </div>
                    </TableCell>
                    <TableCell>
                      <StatusBadge variant="cyan">
                        {item.group || '-'}
                      </StatusBadge>
                    </TableCell>
                    <TableCell>
                      <StatusBadge variant={sc.variant as 'danger' | 'warning' | 'neutral'}>
                        {t(sc.labelKey)}
                      </StatusBadge>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2 min-w-[120px]">
                        <Progress
                          value={item.risk_score ?? 0}
                          className="h-2 flex-1"
                        />
                        <span className="text-xs tabular-nums">
                          {item.risk_score ?? 0}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {ruleNames.length > 0
                          ? ruleNames.map((n) => (
                              <StatusBadge key={n} variant="warning">
                                {n}
                              </StatusBadge>
                            ))
                          : '-'}
                      </div>
                    </TableCell>
                    <TableCell className="text-xs">
                      {(item as Record<string, unknown>).last_seen_at
                        ? formatTimestamp(
                            (item as Record<string, unknown>).last_seen_at as number
                          )
                        : '-'}
                    </TableCell>
                    <TableCell>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={
                          item.status !== 'blocked' ||
                          unblockMutation.isPending
                        }
                        onClick={() =>
                          unblockMutation.mutate({
                            subject_type: item.type,
                            subject_id: item.id,
                            group: item.group,
                          })
                        }
                      >
                        {t('Unblock')}
                      </Button>
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
