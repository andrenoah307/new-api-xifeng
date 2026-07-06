/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState, useEffect, useCallback } from 'react'
import { Download, CloudDownload, ListTodo } from 'lucide-react'
import { toast } from 'sonner'
import { useQueryClient, useIsFetching } from '@tanstack/react-query'
import { useNavigate, getRouteApi } from '@tanstack/react-router'
import { type Table } from '@tanstack/react-table'
import { Eye, EyeOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useIsAdmin } from '@/hooks/use-admin'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { DataTableToolbar } from '@/components/data-table'
import { getCommonHeaders } from '@/lib/api'
import { getLogExportUrl } from '../api'
import { LOG_TYPES } from '../constants'
import { buildSearchParams } from '../lib/filter'
import { getDefaultTimeRange } from '../lib/utils'
import type { CommonLogFilters } from '../types'
import { CommonLogsStats } from './common-logs-stats'
import { CompactDateTimeRangePicker } from './compact-date-time-range-picker'
import { ExportTasksSheet } from './export-tasks-sheet'
import { OfflineExportDialog } from './offline-export-dialog'
import { useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')
const logTypeValues = ['0', '1', '2', '3', '4', '5', '6'] as const

type LogTypeValue = (typeof logTypeValues)[number]

function isLogTypeValue(value: string): value is LogTypeValue {
  return (logTypeValues as readonly string[]).includes(value)
}

interface CommonLogsFilterBarProps<TData> {
  table: Table<TData>
}

export function CommonLogsFilterBar<TData>(
  props: CommonLogsFilterBarProps<TData>
) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const searchParams = route.useSearch()
  const isAdmin = useIsAdmin()
  const { status } = useStatus()
  const { sensitiveVisible, setSensitiveVisible } = useUsageLogsContext()
  const fetchingLogs = useIsFetching({ queryKey: ['logs'] })

  const [offlineExportOpen, setOfflineExportOpen] = useState(false)
  const [exportTasksOpen, setExportTasksOpen] = useState(false)

  const [filters, setFilters] = useState<CommonLogFilters>(() => {
    const { start, end } = getDefaultTimeRange()
    return { startTime: start, endTime: end }
  })
  const [logType, setLogType] = useState<LogTypeValue | ''>('')

  useEffect(() => {
    const next: Partial<CommonLogFilters> = {}
    if (searchParams.startTime)
      next.startTime = new Date(searchParams.startTime)
    if (searchParams.endTime) next.endTime = new Date(searchParams.endTime)
    if (searchParams.channel) next.channel = String(searchParams.channel)
    if (searchParams.model) next.model = searchParams.model
    if (searchParams.token) next.token = searchParams.token
    if (searchParams.group) next.group = searchParams.group
    if (searchParams.username) next.username = searchParams.username
    if (searchParams.requestId) next.requestId = searchParams.requestId

    if (Object.keys(next).length > 0) {
      setFilters((prev) => ({ ...prev, ...next }))
    }

    const typeArr = searchParams.type
    if (Array.isArray(typeArr) && typeArr.length === 1) {
      setLogType(typeArr[0])
    }
  }, [
    searchParams.startTime,
    searchParams.endTime,
    searchParams.channel,
    searchParams.model,
    searchParams.token,
    searchParams.group,
    searchParams.username,
    searchParams.requestId,
    searchParams.type,
  ])

  const handleChange = useCallback(
    (field: keyof CommonLogFilters, value: Date | string | undefined) => {
      setFilters((prev) => ({ ...prev, [field]: value }))
    },
    []
  )

  const handleApply = useCallback(() => {
    const filterParams = buildSearchParams(filters, 'common')
    navigate({
      to: '/usage-logs/$section',
      params: { section: 'common' },
      search: {
        ...filterParams,
        ...(logType ? { type: [logType] } : {}),
        page: 1,
      },
    })
    queryClient.invalidateQueries({ queryKey: ['logs'] })
    queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
  }, [filters, logType, navigate, queryClient])

  const handleReset = useCallback(() => {
    const { start, end } = getDefaultTimeRange()
    const resetFilters: CommonLogFilters = { startTime: start, endTime: end }
    setFilters(resetFilters)
    setLogType('')

    navigate({
      to: '/usage-logs/$section',
      params: { section: 'common' },
      search: {
        page: 1,
        startTime: start.getTime(),
        endTime: end.getTime(),
      },
    })
    queryClient.invalidateQueries({ queryKey: ['logs'] })
    queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
  }, [navigate, queryClient])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') handleApply()
    },
    [handleApply]
  )

  const [exporting, setExporting] = useState(false)

  const handleExport = useCallback(async () => {
    setExporting(true)
    const toastId = toast.loading(t('Exporting logs...'))
    try {
      const params = buildSearchParams(filters, 'common')
      const exportParams: Record<string, unknown> = { ...params }
      if (logType) exportParams.type = logType
      const url = getLogExportUrl(exportParams, isAdmin)
      const response = await fetch(url, {
        credentials: 'include',
        headers: getCommonHeaders(),
      })
      if (response.status === 429) {
        toast.error(t('Export rate limit exceeded, please try again later'), { id: toastId, duration: 5000 })
        return
      }
      if (!response.ok) {
        try {
          const err = await response.json()
          toast.error(err.message || t('Export failed'), { id: toastId, duration: 5000 })
        } catch {
          toast.error(t('Export failed'), { id: toastId, duration: 5000 })
        }
        return
      }
      const reader = response.body?.getReader()
      if (!reader) {
        toast.error(t('Export failed'), { id: toastId, duration: 5000 })
        return
      }
      const chunks: Uint8Array[] = []
      let received = 0
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        chunks.push(value)
        received += value.length
        const sizeMB = (received / 1024 / 1024).toFixed(1)
        toast.loading(t('Exporting logs... {{size}} MB', { size: sizeMB }), { id: toastId })
      }
      const blob = new Blob(chunks, { type: 'text/csv' })
      const downloadUrl = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = downloadUrl
      a.download = `logs_${new Date().toISOString().slice(0, 10)}.csv`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(downloadUrl)
      toast.success(t('Export completed'), { id: toastId, duration: 5000 })
    } catch {
      toast.error(t('Export failed'), { id: toastId, duration: 5000 })
    } finally {
      setExporting(false)
    }
  }, [filters, logType, isAdmin, t])

  const hasExpandedFilters =
    !!filters.token ||
    !!filters.username ||
    !!filters.channel ||
    !!filters.requestId

  const hasAdditionalFilters =
    !!filters.model || !!filters.group || !!logType || hasExpandedFilters

  const inputClass = 'w-full sm:w-[140px] lg:w-[160px]'
  const sensitiveType = sensitiveVisible ? 'text' : 'password'

  const offlineExportEnabled = !!status?.enable_log_export_offline

  const statsBar = (
    <div className='flex flex-wrap items-center gap-2'>
      <CommonLogsStats />
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon'
              onClick={handleExport}
              disabled={exporting}
              aria-label={t('Export Logs')}
              className='text-muted-foreground hover:text-foreground size-7'
            />
          }
        >
          <Download className={exporting ? 'animate-pulse' : ''} />
        </TooltipTrigger>
        <TooltipContent>{t('Export Logs')}: {t('Download current page as CSV')}</TooltipContent>
      </Tooltip>
      {offlineExportEnabled && (
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='ghost'
                size='icon'
                onClick={() => setOfflineExportOpen(true)}
                aria-label={t('Offline Export')}
                className='text-muted-foreground hover:text-foreground size-7'
              />
            }
          >
            <CloudDownload />
          </TooltipTrigger>
          <TooltipContent>{t('Offline Export')}: {t('Background export, notified by email, one task per 24h')}</TooltipContent>
        </Tooltip>
      )}
      {offlineExportEnabled && (
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='ghost'
                size='icon'
                onClick={() => setExportTasksOpen(true)}
                aria-label={t('Export Tasks')}
                className='text-muted-foreground hover:text-foreground size-7'
              />
            }
          >
            <ListTodo />
          </TooltipTrigger>
          <TooltipContent>{t('Export Tasks')}: {t('View task status and download completed exports')}</TooltipContent>
        </Tooltip>
      )}
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon'
              onClick={() => setSensitiveVisible(!sensitiveVisible)}
              aria-label={sensitiveVisible ? t('Hide') : t('Show')}
              className='text-muted-foreground hover:text-foreground size-7'
            />
          }
        >
          {sensitiveVisible ? <Eye /> : <EyeOff />}
        </TooltipTrigger>
        <TooltipContent>
          {sensitiveVisible ? t('Hide') : t('Show')}
        </TooltipContent>
      </Tooltip>
      {offlineExportEnabled && (
        <>
          <OfflineExportDialog
            open={offlineExportOpen}
            onOpenChange={setOfflineExportOpen}
            filters={filters}
            logType={logType || undefined}
          />
          <ExportTasksSheet
            open={exportTasksOpen}
            onOpenChange={setExportTasksOpen}
          />
        </>
      )}
    </div>
  )

  return (
    <DataTableToolbar
      table={props.table}
      leftActions={statsBar}
      customSearch={
        <CompactDateTimeRangePicker
          start={filters.startTime}
          end={filters.endTime}
          onChange={({ start, end }) => {
            handleChange('startTime', start)
            handleChange('endTime', end)
          }}
          className='w-full sm:w-[340px]'
        />
      }
      additionalSearch={
        <>
          <Input
            placeholder={t('Model Name')}
            value={filters.model || ''}
            onChange={(e) => handleChange('model', e.target.value)}
            onKeyDown={handleKeyDown}
            className={inputClass}
          />
          <Input
            placeholder={t('Group')}
            type={sensitiveType}
            value={filters.group || ''}
            onChange={(e) => handleChange('group', e.target.value)}
            onKeyDown={handleKeyDown}
            className={inputClass}
          />
          <Select
            items={[
              { value: 'all', label: t('All Types') },
              ...LOG_TYPES.map((type) => ({
                value: String(type.value),
                label: t(type.label),
              })),
            ]}
            value={logType}
            onValueChange={(value) => {
              setLogType(value !== null && isLogTypeValue(value) ? value : '')
            }}
          >
            <SelectTrigger className={inputClass}>
              <SelectValue placeholder={t('All Types')} />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value='all'>{t('All Types')}</SelectItem>
                {LOG_TYPES.map((type) => (
                  <SelectItem key={type.value} value={String(type.value)}>
                    {t(type.label)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </>
      }
      expandable={
        <>
          <Input
            placeholder={t('Token Name')}
            type={sensitiveType}
            value={filters.token || ''}
            onChange={(e) => handleChange('token', e.target.value)}
            onKeyDown={handleKeyDown}
            className={inputClass}
          />
          {isAdmin && (
            <Input
              placeholder={t('Username')}
              type={sensitiveType}
              value={filters.username || ''}
              onChange={(e) => handleChange('username', e.target.value)}
              onKeyDown={handleKeyDown}
              className={inputClass}
            />
          )}
          {isAdmin && (
            <Input
              placeholder={t('Channel ID')}
              value={filters.channel || ''}
              onChange={(e) => handleChange('channel', e.target.value)}
              onKeyDown={handleKeyDown}
              className={inputClass}
            />
          )}
          <Input
            placeholder={t('Request ID')}
            value={filters.requestId || ''}
            onChange={(e) => handleChange('requestId', e.target.value)}
            onKeyDown={handleKeyDown}
            className={inputClass}
          />
        </>
      }
      hasExpandedActiveFilters={hasExpandedFilters}
      hasAdditionalFilters={hasAdditionalFilters}
      onSearch={handleApply}
      searchLoading={fetchingLogs > 0}
      onReset={handleReset}
    />
  )
}
