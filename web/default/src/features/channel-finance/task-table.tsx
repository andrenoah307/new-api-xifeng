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
import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, Eye, RefreshCw, Search } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { api } from '@/lib/api'

import type { FinanceJob } from './types'

const statuses = {
  pending: 'Pending',
  running: 'Running',
  paused: 'Paused',
  cancelled: 'Cancelled',
  failed: 'Failed',
  completed: 'Completed',
} as const

interface TaskPage {
  items: FinanceJob[]
  total: number
  page: number
  page_size: number
}

export function FinanceTaskTable(props: {
  userId: number
  selected: string | null
  onSelect: (id: string) => void
}) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [filters, setFilters] = useState({
    keyword: '',
    status: '',
    page: 1,
    page_size: 10,
  })
  useEffect(() => {
    const timer = setTimeout(() => {
      setFilters((current) =>
        current.keyword === search.trim()
          ? current
          : { ...current, keyword: search.trim(), page: 1 }
      )
    }, 350)
    return () => clearTimeout(timer)
  }, [search])
  const jobs = useQuery({
    queryKey: ['channel-finance', props.userId, 100, 'jobs', filters],
    queryFn: async ({ signal }) => {
      const response = await api.get('/api/channel-finance/jobs', {
        signal,
        params: filters,
        disableDuplicate: true,
        skipErrorHandler: true,
      })
      if (!response.data.success) {
        throw new Error(response.data.message || t('Failed to load report'))
      }
      return response.data.data as TaskPage
    },
    gcTime: 0,
    refetchInterval: (query) =>
      query.state.data?.items.some((job) =>
        ['pending', 'running'].includes(job.status)
      )
        ? 2000
        : 10000,
  })
  const first = jobs.data?.items[0]?.id
  const selected = props.selected
  const onSelect = props.onSelect
  useEffect(() => {
    if (selected === null && first) onSelect(first)
  }, [first, selected, onSelect])
  const pages = Math.max(
    1,
    Math.ceil((jobs.data?.total ?? 0) / filters.page_size)
  )
  const total = jobs.data?.total
  useEffect(() => {
    if (total !== undefined) {
      setFilters((current) => {
        const lastPage = Math.max(1, Math.ceil(total / current.page_size))
        return current.page > lastPage
          ? { ...current, page: lastPage }
          : current
      })
    }
  }, [total])
  return (
    <section
      aria-label={t('Report task')}
      className='min-w-0 space-y-3 border-b py-5'
    >
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <h2 className='text-base font-semibold'>{t('Report task')}</h2>
        <Button
          variant='outline'
          size='icon'
          title={t('Refresh')}
          aria-label={t('Refresh')}
          disabled={jobs.isFetching}
          onClick={() => void jobs.refetch()}
        >
          <RefreshCw />
        </Button>
      </div>
      <div className='flex flex-wrap items-end gap-3'>
        <label
          className='grid min-w-0 flex-1 gap-2 text-sm font-medium'
          htmlFor='finance-task-search'
        >
          {t('Search')}
          <div className='relative'>
            <Search
              aria-hidden='true'
              className='text-muted-foreground pointer-events-none absolute top-2.5 left-3 size-4'
            />
            <Input
              id='finance-task-search'
              className='pl-9'
              value={search}
              maxLength={128}
              placeholder={t('Report task')}
              onChange={(event) => setSearch(event.target.value)}
            />
          </div>
        </label>
        <label
          className='grid gap-2 text-sm font-medium'
          htmlFor='finance-task-status'
        >
          {t('Status')}
          <NativeSelect
            id='finance-task-status'
            aria-label={t('Status')}
            value={filters.status}
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                status: event.target.value,
                page: 1,
              }))
            }
          >
            <option value=''>{t('All')}</option>
            {Object.entries(statuses).map(([value, label]) => (
              <option key={value} value={value}>
                {t(label)}
              </option>
            ))}
          </NativeSelect>
        </label>
      </div>
      {jobs.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Failed to load report')}
        </p>
      )}
      <div
        className='overflow-x-auto rounded-md border'
        aria-busy={jobs.isFetching}
      >
        <table className='w-full min-w-[760px] text-sm'>
          <thead className='bg-muted text-muted-foreground'>
            <tr>
              {[
                'Report task',
                'Time range',
                'Channel',
                'Status',
                'Progress',
                'View',
              ].map((label) => (
                <th
                  key={label}
                  scope='col'
                  className={`px-3 py-2.5 font-medium whitespace-nowrap ${label === 'Progress' ? 'text-right' : 'text-left'}`}
                >
                  {t(label)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {jobs.data?.items.map((job) => {
              const progress =
                job.total_windows > 0
                  ? Math.min(
                      100,
                      (job.processed_windows / job.total_windows) * 100
                    )
                  : 0
              return (
                <tr
                  key={job.id}
                  aria-selected={props.selected === job.id}
                  className='hover:bg-muted/50 aria-selected:bg-primary/5 border-t'
                >
                  <th
                    scope='row'
                    className='max-w-56 px-3 py-3 text-left font-medium'
                  >
                    <button
                      type='button'
                      className='text-primary text-left break-all underline-offset-4 hover:underline focus-visible:outline-2'
                      onClick={() => props.onSelect(job.id)}
                    >
                      {job.id}
                    </button>
                  </th>
                  <td className='px-3 py-3 text-xs whitespace-nowrap tabular-nums'>
                    <div>{new Date(job.start * 1000).toLocaleString()}</div>
                    <div className='text-muted-foreground mt-1'>
                      {new Date(job.end * 1000).toLocaleString()}
                    </div>
                  </td>
                  <td className='px-3 py-3 whitespace-nowrap'>
                    {job.channel_id ? `#${job.channel_id}` : t('All')}
                  </td>
                  <td className='px-3 py-3'>
                    <Badge
                      variant={
                        job.status === 'failed' ? 'destructive' : 'outline'
                      }
                    >
                      {t(statuses[job.status])}
                    </Badge>
                  </td>
                  <td className='min-w-32 px-3 py-3'>
                    <div className='text-right text-xs tabular-nums'>
                      {progress.toFixed(1)}%
                    </div>
                    <progress
                      aria-label={t('Progress')}
                      className='accent-primary mt-1 h-1.5 w-full'
                      value={progress}
                      max={100}
                    />
                    <div className='text-muted-foreground text-right text-xs tabular-nums'>
                      {job.processed_windows}/{job.total_windows}
                    </div>
                  </td>
                  <td className='px-3 py-3'>
                    <Button
                      variant={
                        props.selected === job.id ? 'secondary' : 'ghost'
                      }
                      size='icon'
                      title={t('View')}
                      aria-label={`${t('View')} ${job.id}`}
                      aria-pressed={props.selected === job.id}
                      onClick={() => props.onSelect(job.id)}
                    >
                      <Eye />
                    </Button>
                  </td>
                </tr>
              )
            })}
            {!jobs.data?.items.length && (
              <tr>
                <td
                  colSpan={6}
                  className='text-muted-foreground h-24 text-center'
                  role='status'
                >
                  {jobs.isPending ? t('Loading...') : t('No data')}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <div className='text-muted-foreground flex flex-wrap items-center justify-between gap-3 text-sm'>
        <span>
          {t('Total')}: {jobs.data?.total ?? '—'}
        </span>
        <div className='flex flex-wrap items-center gap-2'>
          <label htmlFor='finance-task-size'>{t('Page size')}</label>
          <NativeSelect
            id='finance-task-size'
            aria-label={t('Page size')}
            value={filters.page_size}
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                page_size: Number(event.target.value),
                page: 1,
              }))
            }
          >
            {[10, 20, 50].map((size) => (
              <option value={size} key={size}>
                {size}
              </option>
            ))}
          </NativeSelect>
          <Button
            variant='outline'
            size='icon'
            title={t('Previous')}
            aria-label={t('Previous')}
            disabled={filters.page <= 1 || jobs.isFetching}
            onClick={() =>
              setFilters((current) => ({ ...current, page: current.page - 1 }))
            }
          >
            <ChevronLeft />
          </Button>
          <span className='min-w-12 text-center tabular-nums'>
            {filters.page} / {pages}
          </span>
          <Button
            variant='outline'
            size='icon'
            title={t('Next')}
            aria-label={t('Next')}
            disabled={!jobs.data || filters.page >= pages || jobs.isFetching}
            onClick={() =>
              setFilters((current) => ({ ...current, page: current.page + 1 }))
            }
          >
            <ChevronRight />
          </Button>
        </div>
      </div>
    </section>
  )
}
