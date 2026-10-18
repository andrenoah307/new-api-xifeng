import { useMutation, useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, Download, Settings } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { useDebounce } from '@/hooks'
import { useIsAdmin } from '@/hooks/use-admin'
import { hasPermission } from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import {
  exportFinanceRecords,
  getFinancePolicy,
  getFinanceRecords,
  type FinanceFilter,
} from './api'
import { FinancePolicyDialog } from './policy-dialog'
import { FinanceRecordsTable } from './records-table'

export default function FinanceRecords() {
  const { t } = useTranslation()
  const admin = useIsAdmin()
  const user = useAuthStore((state) => state.auth.user)
  const canRead = hasPermission(user, 'finance', 'read')
  const canExport = canRead && hasPermission(user, 'finance', 'export')
  const [requestedView, setView] = useState<'invoices' | 'topups'>('invoices')
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState('all')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [range, setRange] = useState<{ start?: Date; end?: Date }>({})
  const [settingsOpen, setSettingsOpen] = useState(false)
  const search = useDebounce(keyword, 350)
  const policy = useQuery({
    queryKey: ['finance', 'policy', user?.id],
    queryFn: getFinancePolicy,
    enabled: canRead,
  })
  const hasInvoices = (policy.data?.invoice_statuses.length ?? 0) > 0
  const hasTopups = (policy.data?.topup_statuses.length ?? 0) > 0
  const view =
    (requestedView === 'invoices' && hasInvoices) || !hasTopups
      ? 'invoices'
      : 'topups'
  const activeStatus = (
    view === 'invoices'
      ? policy.data?.invoice_statuses.includes(Number(status))
      : policy.data?.topup_statuses.includes(status)
  )
    ? status
    : 'all'
  const filter: FinanceFilter = {
    view,
    keyword: search || undefined,
    invoice_status:
      view === 'invoices' && activeStatus !== 'all'
        ? Number(activeStatus)
        : undefined,
    status:
      view === 'topups' && activeStatus !== 'all' ? activeStatus : undefined,
    start_time: range.start
      ? Math.floor(range.start.getTime() / 1000)
      : undefined,
    end_time: range.end ? Math.floor(range.end.getTime() / 1000) : undefined,
  }
  const records = useQuery({
    queryKey: [
      'finance',
      'records',
      user?.id,
      user?.permissions,
      filter,
      page,
      pageSize,
      policy.data,
    ],
    queryFn: () =>
      getFinanceRecords({ ...filter, p: page, page_size: pageSize }),
    enabled: canRead && !!policy.data,
    gcTime: 0,
  })
  const download = useMutation({
    mutationFn: () => exportFinanceRecords(filter),
  })
  const invoiceLabels: Record<number, string> = {
    1: t('Pending Issuance'),
    2: t('Issued'),
    3: t('Rejected'),
  }
  const topupLabels: Record<string, string> = {
    success: t('Success'),
    pending: t('Pending'),
    failed: t('Failed'),
    expired: t('Expired'),
  }
  const statuses =
    view === 'invoices'
      ? (policy.data?.invoice_statuses ?? []).map((value) => ({
          value: String(value),
          label: invoiceLabels[value],
        }))
      : (policy.data?.topup_statuses ?? []).map((value) => ({
          value,
          label: topupLabels[value],
        }))
  const total = records.data?.total ?? 0
  if (!canRead) return null
  return (
    <>
      <div className='space-y-4'>
        <div className='flex flex-wrap justify-end gap-2'>
          {admin && (
            <Button
              variant='outline'
              size='sm'
              aria-label={t('Financial record visibility')}
              title={t('Financial record visibility')}
              disabled={!policy.data}
              onClick={() => setSettingsOpen(true)}
            >
              <Settings className='size-4' />
              <span className='hidden sm:inline'>
                {t('Financial record visibility')}
              </span>
            </Button>
          )}
          {canExport && (
            <Button
              variant='outline'
              size='sm'
              aria-label={t('Export filtered records')}
              title={t('Export filtered records')}
              disabled={
                download.isPending ||
                !records.data ||
                total === 0 ||
                records.isError
              }
              onClick={() => download.mutate()}
            >
              <Download className='size-4' />
              <span className='hidden sm:inline'>
                {download.isPending
                  ? t('Exporting...')
                  : t('Export filtered records')}
              </span>
            </Button>
          )}
        </div>
        <div className='space-y-4'>
          <Tabs
            value={view}
            onValueChange={(value) => {
              setView(value as 'invoices' | 'topups')
              setPage(1)
              setStatus('all')
            }}
          >
            <TabsList>
              {hasInvoices && (
                <TabsTrigger value='invoices'>
                  {t('Invoice applications')}
                </TabsTrigger>
              )}
              {hasTopups && (
                <TabsTrigger value='topups'>
                  {t('All-user top-up records')}
                </TabsTrigger>
              )}
            </TabsList>
          </Tabs>
          <div className='flex flex-wrap items-center gap-2'>
            <Input
              className='w-full sm:w-72'
              aria-label={t('Search')}
              placeholder={t('Search')}
              value={keyword}
              onChange={(event) => {
                setKeyword(event.target.value)
                setPage(1)
              }}
            />
            <Select
              value={activeStatus}
              onValueChange={(value) => {
                if (value) {
                  setStatus(value)
                  setPage(1)
                }
              }}
            >
              <SelectTrigger className='w-40' aria-label={t('Status')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='all'>{t('All')}</SelectItem>
                {statuses.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <CompactDateTimeRangePicker
              start={range.start}
              end={range.end}
              onChange={(value) => {
                setRange(value)
                setPage(1)
              }}
            />
          </div>
          {(policy.isError || records.isError) && (
            <p role='alert' className='text-destructive text-sm'>
              {t('Failed to load data')}
            </p>
          )}
          <FinanceRecordsTable
            key={`${view}-${page}-${search}`}
            items={records.isError ? [] : (records.data?.items ?? [])}
            invoices={view === 'invoices'}
            loading={policy.isLoading || records.isLoading}
          />
          <div className='flex flex-wrap items-center justify-between gap-3 text-sm'>
            <span>
              {t('Total')}: {total}
            </span>
            <div className='flex items-center gap-2'>
              <Select
                value={String(pageSize)}
                onValueChange={(value) => {
                  if (value) {
                    setPageSize(Number(value))
                    setPage(1)
                  }
                }}
              >
                <SelectTrigger className='w-20' aria-label={t('Rows per page')}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {[20, 50, 100].map((size) => (
                    <SelectItem key={size} value={String(size)}>
                      {size}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                variant='outline'
                size='icon'
                aria-label={t('Previous page')}
                disabled={page === 1 || records.isFetching}
                onClick={() => setPage((prev) => prev - 1)}
              >
                <ChevronLeft className='size-4' />
              </Button>
              <span className='min-w-12 text-center tabular-nums'>
                {page} / {Math.max(1, Math.ceil(total / pageSize))}
              </span>
              <Button
                variant='outline'
                size='icon'
                aria-label={t('Next page')}
                disabled={page * pageSize >= total || records.isFetching}
                onClick={() => setPage((prev) => prev + 1)}
              >
                <ChevronRight className='size-4' />
              </Button>
            </div>
          </div>
        </div>
      </div>
      {admin && settingsOpen && policy.data && (
        <FinancePolicyDialog
          policy={policy.data}
          onClose={() => {
            setSettingsOpen(false)
            setPage(1)
            setStatus('all')
          }}
        />
      )}
    </>
  )
}
