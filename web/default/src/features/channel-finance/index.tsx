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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Calculator, Download, Pause, Play, RefreshCw, X } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { api } from '@/lib/api'
import { downloadTextFile } from '@/lib/download'
import { useAuthStore } from '@/stores/auth-store'

import { CostConfirmation, type FinanceRequest } from './cost-confirmation'
import { reportCSV } from './export'
import { formatReportAmount as amount } from './format'
import { FinanceTaskTable } from './task-table'
import type { FinanceJob, FinanceReport, FinanceRow } from './types'

const statuses = {
  pending: 'Pending',
  running: 'Running',
  paused: 'Paused',
  cancelled: 'Cancelled',
  failed: 'Failed',
  completed: 'Completed',
} as const
const columns = [
  ['consume_amount', 'Consumption'],
  ['refund_amount', 'Refunds'],
  ['net_amount', 'Net consumption'],
  ['original_cost', 'Snapshot cost'],
  ['fallback_cost', 'Estimated historical cost'],
  ['cost', 'Cost'],
  ['known_profit', 'Known-cost profit'],
  ['profit', 'Profit'],
  ['profit_margin', 'Profit margin'],
  ['cost_ratio', 'Cost ratio'],
  ['coverage', 'Cost coverage'],
  ['consume_count', 'Consumption records'],
  ['original_count', 'Snapshot records'],
  ['fallback_count', 'Estimated records'],
  ['missing_count', 'Unknown-cost records'],
  ['stale_count', 'Stale-rate records'],
  ['refund_count', 'Refund records'],
] as const

function localTime(value: Date) {
  return new Date(value.getTime() - value.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16)
}

export function ChannelFinance() {
  const user = useAuthStore((state) => state.auth.user)
  if (user?.role !== 100) return null
  return (
    <ChannelFinanceContent key={`${user.id}:${user.role}`} userId={user.id} />
  )
}

function ChannelFinanceContent(props: { userId: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [start, setStart] = useState(() =>
    localTime(new Date(new Date().setHours(0, 0, 0, 0)))
  )
  const [end, setEnd] = useState(() => localTime(new Date()))
  const [channel, setChannel] = useState('')
  const [fallback, setFallback] = useState(false)
  const [pendingRequest, setPendingRequest] = useState<FinanceRequest | null>(
    null
  )
  const [selected, setSelected] = useState<string | null>(null)
  const [filter, setFilter] = useState('')
  const [error, setError] = useState('')
  const prefix = ['channel-finance', props.userId, 100] as const
  const id = selected
  const report = useQuery({
    queryKey: [...prefix, 'report', id],
    queryFn: async ({ signal }) => {
      const response = await api.get(`/api/channel-finance/jobs/${id}/report`, {
        signal,
        disableDuplicate: true,
        skipErrorHandler: true,
      })
      if (!response.data.success) {
        throw new Error(response.data.message || t('Failed to load report'))
      }
      return response.data.data as FinanceReport
    },
    enabled: id !== null,
    gcTime: 0,
    refetchInterval: (query) =>
      ['pending', 'running'].includes(query.state.data?.job.status ?? '')
        ? 2000
        : false,
  })
  const mutate = useMutation({
    mutationFn: async (request: { path: string; body?: unknown }) => {
      if (
        useAuthStore.getState().auth.user?.id !== props.userId ||
        useAuthStore.getState().auth.user?.role !== 100
      ) {
        throw new Error(t('Forbidden'))
      }
      const response = await api.post(
        `/api/channel-finance/jobs${request.path}`,
        request.body,
        { skipErrorHandler: true }
      )
      if (!response.data.success) {
        throw new Error(response.data.message || t('Operation failed'))
      }
      return response.data.data as FinanceJob
    },
    onSuccess: (job) => {
      if (job?.id) setSelected(job.id)
      setError('')
      setPendingRequest(null)
      void client.invalidateQueries({ queryKey: prefix })
    },
    onError: (failure: { message: string; response?: { status?: number } }) => {
      if (failure.response?.status === 409) {
        setError(
          t('Channel cost settings changed. Review the channel list again.')
        )
        void client.invalidateQueries({ queryKey: [...prefix, 'cost-preview'] })
      } else setError(failure.message)
    },
  })
  function generate(event: React.FormEvent) {
    event.preventDefault()
    const startAt = Math.floor(new Date(start).getTime() / 1000)
    const endAt = Math.floor(new Date(end).getTime() / 1000)
    const channelId = channel === '' ? 0 : Number(channel)
    if (
      !Number.isFinite(startAt) ||
      !Number.isFinite(endAt) ||
      endAt <= startAt ||
      endAt > Date.now() / 1000 ||
      startAt < 1 ||
      !Number.isSafeInteger(channelId) ||
      channelId < 0
    ) {
      setError(
        t(
          'Select a valid historical time range and confirm the estimation basis.'
        )
      )
      return
    }
    setError('')
    const body: FinanceRequest = {
      request_key: crypto.randomUUID(),
      start: startAt,
      end: endAt,
      channel_id: channelId,
      fallback_mode: fallback ? 'billed_basis' : 'none',
    }
    if (fallback) setPendingRequest(body)
    else mutate.mutate({ path: '', body })
  }
  const job = report.data?.job
  const progress = job?.total_windows
    ? Math.min(100, (job.processed_windows / job.total_windows) * 100)
    : 0
  const rows =
    report.data?.channels.filter((row) =>
      `${row.channel_id} ${row.channel_name}`
        .toLowerCase()
        .includes(filter.toLowerCase())
    ) ?? []
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Revenue')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {pendingRequest && (
          <CostConfirmation
            request={pendingRequest}
            userId={props.userId}
            busy={mutate.isPending}
            error={error}
            onClose={() => setPendingRequest(null)}
            onConfirm={(fingerprint) =>
              mutate.mutate({
                path: '',
                body: { ...pendingRequest, cost_fingerprint: fingerprint },
              })
            }
          />
        )}
        <form onSubmit={generate} className='border-b pb-5'>
          <FieldSet className='mb-5 gap-3'>
            <FieldLegend variant='label'>{t('Summary workflow')}</FieldLegend>
            <RadioGroup
              value={fallback ? 'billed_basis' : 'none'}
              onValueChange={(value) => {
                setFallback(value === 'billed_basis')
              }}
              className='gap-3'
            >
              <Field orientation='horizontal'>
                <RadioGroupItem id='finance-existing' value='none' />
                <FieldLabel htmlFor='finance-existing'>
                  {t('Existing costs only (may be incomplete)')}
                </FieldLabel>
              </Field>
              <Field orientation='horizontal'>
                <RadioGroupItem id='finance-complete' value='billed_basis' />
                <FieldLabel htmlFor='finance-complete'>
                  {t('Full summary with channel-rate cost estimates')}
                </FieldLabel>
              </Field>
            </RadioGroup>
          </FieldSet>
          <FieldGroup className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
            <Field>
              <FieldLabel htmlFor='finance-start'>{t('Start time')}</FieldLabel>
              <Input
                id='finance-start'
                type='datetime-local'
                required
                value={start}
                onChange={(event) => setStart(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='finance-end'>{t('End time')}</FieldLabel>
              <Input
                id='finance-end'
                type='datetime-local'
                required
                value={end}
                onChange={(event) => setEnd(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='finance-channel'>
                {t('Channel ID (empty for all)')}
              </FieldLabel>
              <Input
                id='finance-channel'
                type='number'
                min='1'
                step='1'
                value={channel}
                onChange={(event) => setChannel(event.target.value)}
              />
            </Field>
          </FieldGroup>
          <Button className='mt-4' type='submit' disabled={mutate.isPending}>
            <Calculator />
            {t('Generate report')}
          </Button>
        </form>
        {(error || report.isError) && (
          <p role='alert' className='text-destructive mt-4'>
            {error || t('Failed to load report')}
          </p>
        )}
        <FinanceTaskTable
          userId={props.userId}
          selected={selected}
          onSelect={setSelected}
        />
        <div className='my-4 flex flex-wrap items-center gap-3'>
          {id && (
            <h2 className='min-w-0 flex-1 text-sm font-semibold break-all'>
              {t('Report task')} · {id}
            </h2>
          )}
          <Button
            variant='outline'
            aria-label={t('Refresh')}
            title={t('Refresh')}
            onClick={() => void client.invalidateQueries({ queryKey: prefix })}
          >
            <RefreshCw />
          </Button>
          {job && (
            <>
              {['pending', 'running'].includes(job.status) && (
                <Button
                  variant='outline'
                  disabled={mutate.isPending}
                  onClick={() => mutate.mutate({ path: `/${job.id}/pause` })}
                >
                  <Pause />
                  {t('Pause')}
                </Button>
              )}
              {['paused', 'failed'].includes(job.status) && (
                <Button
                  variant='outline'
                  disabled={mutate.isPending}
                  onClick={() => mutate.mutate({ path: `/${job.id}/resume` })}
                >
                  <Play />
                  {t('Resume')}
                </Button>
              )}
              {['pending', 'running', 'paused'].includes(job.status) && (
                <Button
                  variant='outline'
                  disabled={mutate.isPending}
                  onClick={() => mutate.mutate({ path: `/${job.id}/cancel` })}
                >
                  <X />
                  {t('Cancel')}
                </Button>
              )}
            </>
          )}
        </div>
        {job && (
          <div className='space-y-2 border-b pb-4 text-sm'>
            <div className='flex flex-wrap justify-between gap-2'>
              <span>
                {t(statuses[job.status])} · {job.processed_windows}/
                {job.total_windows} · {progress.toFixed(1)}%
              </span>
              <span>
                {t('Aggregated through')}:{' '}
                {new Date(job.cursor * 1000).toLocaleString()}
              </span>
            </div>
            <progress
              className='accent-primary h-2 w-full'
              value={progress}
              max={100}
            />
            {job.policy && (
              <p className='max-h-24 overflow-y-auto break-words'>
                {t('Summary workflow')}:{' '}
                {t(
                  job.policy.fallback_mode === 'billed_basis'
                    ? 'Full summary with channel-rate cost estimates'
                    : 'Existing costs only (may be incomplete)'
                )}
                {job.policy.rates
                  ?.map(
                    (rate) =>
                      ` · #${rate.channel_id}: ${rate.ratio}${rate.peak ? ` (${rate.peak.start}-${rate.peak.end} ${rate.peak.timezone} × ${rate.peak.ratio})` : ''}`
                  )
                  .join('')}
              </p>
            )}
            {job.error && (
              <p role='alert' className='text-destructive break-words'>
                {job.error}
              </p>
            )}
          </div>
        )}
        {report.data && (
          <>
            <dl className='grid grid-cols-2 gap-5 border-b py-5 lg:grid-cols-4'>
              {(
                ['net_amount', 'cost', 'known_profit', 'coverage'] as const
              ).map((key) => (
                <div key={key}>
                  <dt className='text-muted-foreground text-sm'>
                    {t(columns.find((column) => column[0] === key)?.[1] ?? key)}
                  </dt>
                  <dd className='mt-1 text-lg font-semibold break-all tabular-nums'>
                    {amount(report.data.total[key], key)}
                  </dd>
                </div>
              ))}
            </dl>
            <div className='my-4 flex flex-wrap items-end gap-3'>
              <Field className='max-w-sm flex-1'>
                <FieldLabel htmlFor='finance-filter'>
                  {t('Filter channels')}
                </FieldLabel>
                <Input
                  id='finance-filter'
                  value={filter}
                  onChange={(event) => setFilter(event.target.value)}
                />
              </Field>
              <Button
                variant='outline'
                disabled={!rows.length}
                onClick={() => {
                  if (!report.data) return
                  downloadTextFile(
                    reportCSV(report.data.job, rows),
                    `revenue-${report.data.job.id}-filtered.csv`
                  )
                }}
              >
                <Download />
                {t('Export filtered results')}
              </Button>
            </div>
            <div className='overflow-x-auto rounded-md border'>
              <table className='w-full text-sm'>
                <thead className='bg-muted'>
                  <tr>
                    <th className='p-3 text-left whitespace-nowrap'>
                      {t('Channel')}
                    </th>
                    {columns.map(([key, label]) => (
                      <th
                        key={key}
                        className='p-3 text-right whitespace-nowrap'
                      >
                        {t(label)}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row: FinanceRow) => (
                    <tr key={row.channel_id} className='border-t'>
                      <th className='max-w-60 min-w-40 p-3 text-left font-medium break-words'>
                        {row.channel_name || '-'}{' '}
                        <span className='text-muted-foreground'>
                          #{row.channel_id}
                        </span>
                      </th>
                      {columns.map(([key]) => (
                        <td
                          key={key}
                          className='p-3 text-right whitespace-nowrap tabular-nums'
                        >
                          {amount(row[key], key)}
                        </td>
                      ))}
                    </tr>
                  ))}
                  {!rows.length && (
                    <tr>
                      <td
                        colSpan={columns.length + 1}
                        className='text-muted-foreground p-8 text-center'
                      >
                        {t('No data')}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
