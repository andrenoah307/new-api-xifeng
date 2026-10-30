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
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Calculator, Pause, Play, Plus, RefreshCw, X } from 'lucide-react'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { NativeSelect } from '@/components/ui/native-select'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import type { FinanceJob, FinanceReport, FinanceRow } from './types'
import { formatReportAmount as amount } from './format'

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
    localTime(new Date(new Date().setHours(0, 0, 0, 0))),
  )
  const [end, setEnd] = useState(() => localTime(new Date()))
  const [channel, setChannel] = useState('')
  const [fallback, setFallback] = useState(false)
  const [ratio, setRatio] = useState('1')
  const [rates, setRates] = useState([
    { key: crypto.randomUUID(), channel: '', ratio: '1' },
  ])
  const [accepted, setAccepted] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)
  const [filter, setFilter] = useState('')
  const [error, setError] = useState('')
  const prefix = ['channel-finance', props.userId, 100] as const
  const jobs = useQuery({
    queryKey: [...prefix, 'jobs'],
    queryFn: async ({ signal }) => {
      const response = await api.get('/api/channel-finance/jobs', {
        signal,
        disableDuplicate: true,
      })
      if (!response.data.success) {
        throw new Error(response.data.message || t('Failed to load report'))
      }
      return response.data.data as FinanceJob[]
    },
    gcTime: 0,
    refetchInterval: (query) =>
      query.state.data?.some((job) =>
        ['pending', 'running'].includes(job.status),
      )
        ? 2000
        : false,
  })
  const id = selected ?? jobs.data?.[0]?.id
  const report = useQuery({
    queryKey: [...prefix, 'report', id],
    queryFn: async ({ signal }) => {
      const response = await api.get(`/api/channel-finance/jobs/${id}/report`, {
        signal,
        disableDuplicate: true,
      })
      if (!response.data.success) {
        throw new Error(response.data.message || t('Failed to load report'))
      }
      return response.data.data as FinanceReport
    },
    enabled: id !== undefined,
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
      )
      if (!response.data.success) {
        throw new Error(response.data.message || t('Operation failed'))
      }
      return response.data.data as FinanceJob
    },
    onSuccess: (job) => {
      if (job?.id) setSelected(job.id)
      setError('')
      void client.invalidateQueries({ queryKey: prefix })
    },
    onError: (failure) => setError(failure.message),
  })
  function generate(event: React.FormEvent) {
    event.preventDefault()
    const startAt = Math.floor(new Date(start).getTime() / 1000)
    const endAt = Math.floor(new Date(end).getTime() / 1000)
    const channelId = channel === '' ? 0 : Number(channel)
    const configuredRates = channelId > 0
        ? [{ channel_id: channelId, ratio }]
        : rates.map((row) => ({
            channel_id: Number(row.channel),
            ratio: row.ratio,
          }))
    const submittedRates = fallback ? configuredRates : []
    if (
      !Number.isFinite(startAt) ||
      !Number.isFinite(endAt) ||
      endAt <= startAt ||
      endAt > Date.now() / 1000 ||
      endAt - startAt > 31 * 86400 ||
      !Number.isSafeInteger(channelId) ||
      channelId < 0 ||
      (fallback &&
        (!accepted ||
          submittedRates.length === 0 ||
          new Set(submittedRates.map((row) => row.channel_id)).size !==
            submittedRates.length ||
          submittedRates.some(
            (row) =>
              !Number.isSafeInteger(row.channel_id) ||
              row.channel_id <= 0 ||
              !/^[0-9]{1,18}(?:\.[0-9]{1,18})?$/.test(row.ratio) ||
              Number(row.ratio) > 10000,
          )))
    ) {
      setError(
        t(
          'Select a valid past period of up to 31 days and confirm the estimation basis.',
        ),
      )
      return
    }
    mutate.mutate({
      path: '',
      body: {
        request_key: crypto.randomUUID(),
        start: startAt,
        end: endAt,
        channel_id: channelId,
        fallback_mode: fallback ? 'billed_basis' : 'none',
        rates: submittedRates,
      },
    })
  }
  const job = report.data?.job
  const progress = job?.total_windows
    ? Math.min(100, (job.processed_windows / job.total_windows) * 100)
    : 0
  const rows =
    report.data?.channels.filter((row) =>
      `${row.channel_id} ${row.channel_name}`
        .toLowerCase()
        .includes(filter.toLowerCase()),
    ) ?? []
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Channel finance')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <form onSubmit={generate} className='border-b pb-5'>
          <FieldGroup className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
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
            <Field>
              <FieldLabel htmlFor='finance-basis'>
                {t('Historical cost basis')}
              </FieldLabel>
              <NativeSelect
                id='finance-basis'
                value={fallback ? 'billed_basis' : 'none'}
                onChange={(event) => {
                  setFallback(event.target.value === 'billed_basis')
                  setAccepted(false)
                }}
              >
                <option value='none'>{t('Snapshot only')}</option>
                <option value='billed_basis'>
                  {t('Billing-basis estimate')}
                </option>
              </NativeSelect>
            </Field>
          </FieldGroup>
          {fallback && (
            <FieldGroup className='mt-4 gap-3'>
              {channel !== '' ? (
                <Field className='max-w-xs'>
                  <FieldLabel htmlFor='finance-ratio'>
                    {t('Historical cost ratio')}
                  </FieldLabel>
                  <Input
                    id='finance-ratio'
                    type='number'
                    min='0'
                    max='10000'
                    step='any'
                    value={ratio}
                    onChange={(event) => setRatio(event.target.value)}
                  />
                </Field>
              ) : (
                <div className='space-y-3'>
                  {rates.map((row) => (
                    <div key={row.key} className='flex items-end gap-3'>
                      <Field className='min-w-0 max-w-xs'>
                        <FieldLabel htmlFor={`finance-channel-${row.key}`}>
                          {t('Channel')}
                        </FieldLabel>
                        <Input
                          id={`finance-channel-${row.key}`}
                          type='number'
                          min='1'
                          step='1'
                          value={row.channel}
                          onChange={(event) => {
                            setRates((values) =>
                              values.map((item) =>
                                item.key === row.key
                                  ? { ...item, channel: event.target.value }
                                  : item,
                              ),
                            )
                            setAccepted(false)
                          }}
                        />
                      </Field>
                      <Field className='min-w-0 max-w-xs'>
                        <FieldLabel htmlFor={`finance-ratio-${row.key}`}>
                          {t('Historical cost ratio')}
                        </FieldLabel>
                        <Input
                          id={`finance-ratio-${row.key}`}
                          type='number'
                          min='0'
                          max='10000'
                          step='any'
                          value={row.ratio}
                          onChange={(event) => {
                            setRates((values) =>
                              values.map((item) =>
                                item.key === row.key
                                  ? { ...item, ratio: event.target.value }
                                  : item,
                              ),
                            )
                            setAccepted(false)
                          }}
                        />
                      </Field>
                      <Button
                        type='button'
                        variant='outline'
                        title={t('Remove')}
                        aria-label={t('Remove')}
                        onClick={() => {
                          setRates((values) =>
                            values.filter((item) => item.key !== row.key),
                          )
                          setAccepted(false)
                        }}
                      >
                        <X />
                      </Button>
                    </div>
                  ))}
                  <Button
                    type='button'
                    variant='outline'
                    onClick={() => {
                      setRates((values) => [
                        ...values,
                        { key: crypto.randomUUID(), channel: '', ratio: '1' },
                      ])
                      setAccepted(false)
                    }}
                  >
                    <Plus />
                    {t('Add')}
                  </Button>
                </div>
              )}
              <Field orientation='horizontal'>
                <Checkbox
                  id='finance-accept'
                  checked={accepted}
                  onCheckedChange={(value) => setAccepted(value === true)}
                />
                <FieldLabel htmlFor='finance-accept'>
                  {t(
                    'I accept billing-basis estimates for missing snapshots throughout this report period.',
                  )}
                </FieldLabel>
              </Field>
            </FieldGroup>
          )}
          <Button className='mt-4' type='submit' disabled={mutate.isPending}>
            <Calculator />
            {t('Generate report')}
          </Button>
        </form>
        {(error || jobs.isError || report.isError) && (
          <p role='alert' className='mt-4 text-destructive'>
            {error || t('Failed to load report')}
          </p>
        )}
        <div className='my-4 flex flex-wrap items-end gap-3'>
          <Field className='min-w-0 flex-1'>
            <FieldLabel htmlFor='finance-job'>{t('Report task')}</FieldLabel>
            <NativeSelect
              id='finance-job'
              className='w-full'
              value={id ?? ''}
              onChange={(event) => setSelected(event.target.value)}
            >
              <option value='' disabled>
                {t('No data')}
              </option>
              {jobs.data?.map((item) => (
                <option key={item.id} value={item.id}>
                  #{item.id} · {t(statuses[item.status])} ·{' '}
                  {new Date(item.start * 1000).toLocaleString()} -{' '}
                  {new Date(item.end * 1000).toLocaleString()}
                </option>
              ))}
            </NativeSelect>
          </Field>
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
              className='h-2 w-full accent-primary'
              value={progress}
              max={100}
            />
            {job.policy && (
              <p>
                {t('Historical cost basis')}:{' '}
                {t(
                  job.policy.fallback_mode === 'billed_basis'
                    ? 'Billing-basis estimate'
                    : 'Snapshot only',
                )}
                {job.policy.rates
                  ?.map((rate) => ` · #${rate.channel_id}: ${rate.ratio}`)
                  .join('')}
              </p>
            )}
            {job.error && (
              <p role='alert' className='break-words text-destructive'>
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
                  <dt className='text-sm text-muted-foreground'>
                    {t(columns.find((column) => column[0] === key)?.[1] ?? key)}
                  </dt>
                  <dd className='mt-1 break-all text-lg font-semibold tabular-nums'>
                    {amount(report.data.total[key], key)}
                  </dd>
                </div>
              ))}
            </dl>
            <Field className='my-4 max-w-sm'>
              <FieldLabel htmlFor='finance-filter'>
                {t('Filter channels')}
              </FieldLabel>
              <Input
                id='finance-filter'
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
              />
            </Field>
            <div className='overflow-x-auto rounded-md border'>
              <table className='w-full text-sm'>
                <thead className='bg-muted'>
                  <tr>
                    <th className='whitespace-nowrap p-3 text-left'>
                      {t('Channel')}
                    </th>
                    {columns.map(([key, label]) => (
                      <th
                        key={key}
                        className='whitespace-nowrap p-3 text-right'
                      >
                        {t(label)}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row: FinanceRow) => (
                    <tr key={row.channel_id} className='border-t'>
                      <th className='min-w-40 max-w-60 break-words p-3 text-left font-medium'>
                        {row.channel_name || '-'}{' '}
                        <span className='text-muted-foreground'>
                          #{row.channel_id}
                        </span>
                      </th>
                      {columns.map(([key]) => (
                        <td
                          key={key}
                          className='whitespace-nowrap p-3 text-right tabular-nums'
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
                        className='p-8 text-center text-muted-foreground'
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
