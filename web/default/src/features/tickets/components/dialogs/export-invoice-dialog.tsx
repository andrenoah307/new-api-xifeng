import { useState, useMemo, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Download } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
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
import {
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { formatTimestampToDate } from '@/lib/format'
import { toast } from 'sonner'
import {
  getInvoiceExportList,
  type InvoiceExportItem,
} from '../../api'

const INVOICE_STATUS_OPTIONS = [
  { value: '0', label: 'All' },
  { value: '1', label: 'Pending Issuance' },
  { value: '2', label: 'Issued' },
  { value: '3', label: 'Rejected' },
]

const PAGE_SIZE = 20

function csvField(value: string | number): string {
  const s = String(value ?? '')
  if (
    s.includes(',') ||
    s.includes('"') ||
    s.includes('\n') ||
    s.includes('\r')
  ) {
    return '"' + s.replace(/"/g, '""') + '"'
  }
  return s
}

function generateInvoiceCSV(
  items: InvoiceExportItem[],
  serviceName: string
): string {
  const BOM = '﻿'
  const headers = [
    '电子邮箱',
    '数量',
    '单价',
    '金额合计',
    '公司信息',
    '应税服务名称',
  ]
  const lines = [headers.map(csvField).join(',')]
  for (const item of items) {
    const companyInfo = item.tax_number
      ? `发票抬头\n${item.company_name}\n购方税号\n${item.tax_number}`
      : item.company_name
    const row = [
      item.email,
      '',
      '',
      item.total_money.toFixed(2),
      companyInfo,
      serviceName,
    ]
    lines.push(row.map(csvField).join(','))
  }
  return BOM + lines.join('\r\n')
}

function downloadCSV(csv: string, filename: string) {
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

interface ExportInvoiceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ExportInvoiceDialog({
  open,
  onOpenChange,
}: ExportInvoiceDialogProps) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [statusFilter, setStatusFilter] = useState('0')
  const [keyword, setKeyword] = useState('')
  const [searchKeyword, setSearchKeyword] = useState('')
  const [serviceName, setServiceName] = useState('')
  const [dateRange, setDateRange] = useState<{
    start?: Date
    end?: Date
  }>({})
  const [selected, setSelected] = useState<Map<number, InvoiceExportItem>>(
    new Map()
  )

  const queryParams = useMemo(
    () => ({
      p: page - 1,
      page_size: PAGE_SIZE,
      keyword: searchKeyword || undefined,
      invoice_status:
        statusFilter !== '0' ? Number(statusFilter) : undefined,
      start_time: dateRange.start
        ? Math.floor(dateRange.start.getTime() / 1000)
        : undefined,
      end_time: dateRange.end
        ? Math.floor(dateRange.end.getTime() / 1000)
        : undefined,
    }),
    [page, searchKeyword, statusFilter, dateRange]
  )

  const { data, isLoading } = useQuery({
    queryKey: ['invoice-export-list', queryParams],
    queryFn: () => getInvoiceExportList(queryParams),
    enabled: open,
  })

  const items = data?.items ?? []
  const total = data?.total ?? 0

  const allOnPageSelected =
    items.length > 0 && items.every((i) => selected.has(i.ticket_id))

  const toggleItem = useCallback((item: InvoiceExportItem) => {
    setSelected((prev) => {
      const next = new Map(prev)
      if (next.has(item.ticket_id)) {
        next.delete(item.ticket_id)
      } else {
        next.set(item.ticket_id, item)
      }
      return next
    })
  }, [])

  const toggleAll = useCallback(() => {
    setSelected((prev) => {
      const next = new Map(prev)
      if (items.every((i) => prev.has(i.ticket_id))) {
        for (const i of items) next.delete(i.ticket_id)
      } else {
        for (const i of items) next.set(i.ticket_id, i)
      }
      return next
    })
  }, [items])

  const handleSearch = useCallback(() => {
    setSearchKeyword(keyword)
    setPage(1)
  }, [keyword])

  const handleStatusChange = useCallback((v: string) => {
    setStatusFilter(v)
    setPage(1)
  }, [])

  const handleDateChange = useCallback(
    (range: { start?: Date; end?: Date }) => {
      setDateRange(range)
      setPage(1)
    },
    []
  )

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen) {
        setPage(1)
        setStatusFilter('0')
        setKeyword('')
        setSearchKeyword('')
        setServiceName('')
        setDateRange({})
        setSelected(new Map())
      }
      onOpenChange(nextOpen)
    },
    [onOpenChange]
  )

  const handleExport = useCallback(() => {
    if (selected.size === 0) {
      toast.error(t('Please select at least one invoice'))
      return
    }
    if (!serviceName.trim()) {
      toast.error(t('Please enter taxable service name'))
      return
    }
    const csv = generateInvoiceCSV(
      Array.from(selected.values()),
      serviceName.trim()
    )
    const date = new Date().toISOString().slice(0, 10).replace(/-/g, '')
    downloadCSV(csv, `发票登记_${date}.csv`)
    toast.success(
      t('Exported {{count}} invoices', { count: selected.size })
    )
  }, [selected, serviceName, t])

  const table = useReactTable({
    data: items,
    columns: [],
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    pageCount: Math.ceil(total / PAGE_SIZE),
    state: {
      pagination: { pageIndex: page - 1, pageSize: PAGE_SIZE },
    },
    onPaginationChange: (updater) => {
      const next =
        typeof updater === 'function'
          ? updater({ pageIndex: page - 1, pageSize: PAGE_SIZE })
          : updater
      setPage(next.pageIndex + 1)
    },
  })

  const statusLabel = (s: number) => {
    switch (s) {
      case 1:
        return t('Pending Issuance')
      case 2:
        return t('Issued')
      case 3:
        return t('Rejected')
      default:
        return '-'
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('Export Invoice List')}</DialogTitle>
        </DialogHeader>

        <div className="flex flex-wrap items-center gap-2 py-2">
          <Input
            placeholder={t('Search by company name, email or amount...')}
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
            className="h-8 w-[200px]"
          />
          <Select value={statusFilter} onValueChange={handleStatusChange}>
            <SelectTrigger className="h-8 w-[130px]">
              <SelectValue placeholder={t('Invoice Status')} />
            </SelectTrigger>
            <SelectContent>
              {INVOICE_STATUS_OPTIONS.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {t(o.label)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <CompactDateTimeRangePicker
            start={dateRange.start}
            end={dateRange.end}
            onChange={handleDateChange}
            className="w-auto"
          />
        </div>

        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-10">
                  <Checkbox
                    checked={allOnPageSelected}
                    onCheckedChange={toggleAll}
                  />
                </TableHead>
                <TableHead className="w-16">ID</TableHead>
                <TableHead>{t('Company Name')}</TableHead>
                <TableHead>{t('Email')}</TableHead>
                <TableHead className="text-right">
                  {t('Amount')}
                </TableHead>
                <TableHead className="text-center">
                  {t('Orders')}
                </TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Created')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={8} className="text-center py-8">
                    {t('Loading...')}
                  </TableCell>
                </TableRow>
              ) : items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="text-center py-8">
                    {t('No data')}
                  </TableCell>
                </TableRow>
              ) : (
                items.map((item) => (
                  <TableRow key={item.ticket_id}>
                    <TableCell>
                      <Checkbox
                        checked={selected.has(item.ticket_id)}
                        onCheckedChange={() => toggleItem(item)}
                      />
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      #{item.ticket_id}
                    </TableCell>
                    <TableCell className="max-w-[160px] truncate text-xs">
                      {item.company_name}
                    </TableCell>
                    <TableCell className="max-w-[140px] truncate text-xs">
                      {item.email}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs">
                      ¥{item.total_money.toFixed(2)}
                    </TableCell>
                    <TableCell className="text-center text-xs">
                      {item.order_count}
                    </TableCell>
                    <TableCell className="text-xs">
                      {statusLabel(item.invoice_status)}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-xs">
                      {formatTimestampToDate(item.created_time)}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>

        {total > PAGE_SIZE && (
          <DataTablePagination table={table} />
        )}

        <DialogFooter className="flex-col items-stretch gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-muted-foreground text-sm">
              {t('Selected {{count}} invoices', {
                count: selected.size,
              })}
            </span>
            <Input
              placeholder={t('Taxable Service Name')}
              value={serviceName}
              onChange={(e) => setServiceName(e.target.value)}
              className="h-8 w-[180px]"
            />
          </div>
          <Button
            onClick={handleExport}
            disabled={selected.size === 0 || !serviceName.trim()}
          >
            <Download className="mr-1.5 h-4 w-4" />
            {t('Export')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
