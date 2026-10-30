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
import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { useStatus } from '@/hooks/use-status'
import { toIntlLocale } from '@/i18n/languages'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

type FundingData = {
  consumption: string
  online_topups: string
  manual_credits: string
  total_funding: string
  consumption_ratio: number | null
  calculated_at: number
}

function localHour(timestamp: number) {
  const date = new Date(timestamp * 1000)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16)
}

export function FundingOverview() {
  const { status } = useStatus()
  const user = useAuthStore((state) => state.auth.user)
  if (user?.role !== ROLE.SUPER_ADMIN || !status?.finance_module_enabled) {
    return null
  }
  return <FundingOverviewContent key={user.id} userId={user.id} />
}

function FundingOverviewContent(props: { userId: number }) {
  const { t, i18n } = useTranslation()
  const [range, setRange] = useState(() => {
    const end = Math.floor(Date.now() / 3600000) * 3600
    return { start: end - 86400, end }
  })
  const [preset, setPreset] = useState('1')
  const [start, setStart] = useState(() => localHour(range.start))
  const [end, setEnd] = useState(() => localHour(range.end))
  const [invalid, setInvalid] = useState(false)
  const query = useQuery({
    queryKey: ['root-funding-overview', props.userId, range.start, range.end],
    queryFn: async ({ signal }) => {
      const response = await api.get<{ success: boolean; data: FundingData }>(
        '/api/data/funding',
        {
          params: { start_timestamp: range.start, end_timestamp: range.end },
          signal,
          disableDuplicate: true,
          skipBusinessError: true,
          skipErrorHandler: true,
        }
      )
      if (!response.data.success) {
        throw new Error('Funding overview unavailable')
      }
      return response.data.data
    },
    staleTime: 300000,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const selectPreset = (value: string) => {
    setPreset(value)
    setInvalid(false)
    if (value === 'custom') return
    const until = Math.floor(Date.now() / 3600000) * 3600
    const from = until - Number(value) * 86400
    setStart(localHour(from))
    setEnd(localHour(until))
    setRange({ start: from, end: until })
  }
  const applyRange = () => {
    const from = new Date(start).getTime() / 1000
    const until = new Date(end).getTime() / 1000
    if (
      !Number.isFinite(from) ||
      !Number.isFinite(until) ||
      from <= 0 ||
      until <= from ||
      until - from > 31 * 86400 ||
      from % 3600 !== 0 ||
      until % 3600 !== 0 ||
      until > Date.now() / 1000
    ) {
      setInvalid(true)
      return
    }
    setInvalid(false)
    setRange({ start: from, end: until })
  }
  const data = query.isError ? undefined : query.data
  const locale = toIntlLocale(i18n.language)
  const formatter = new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: 'CNY',
    maximumFractionDigits: 2,
  })
  const items = [
    [t('Consumption amount'), data?.consumption],
    [t('Successful online top-ups'), data?.online_topups],
    [t('Administrator manual credits'), data?.manual_credits],
    [t('Top-ups + manual credits'), data?.total_funding],
  ]
  return (
    <section
      className='min-w-0 space-y-4 border-y py-4'
      aria-label={t('Funding overview')}
    >
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <h3 className='text-base font-semibold'>{t('Funding overview')}</h3>
        <div className='flex flex-wrap items-center gap-2'>
          <NativeSelect
            aria-label={t('Time range')}
            value={preset}
            onChange={(event) => selectPreset(event.target.value)}
          >
            {[1, 3, 7, 30].map((days) => (
              <NativeSelectOption key={days} value={String(days)}>
                {t('Last {{days}} days', { days })}
              </NativeSelectOption>
            ))}
            <NativeSelectOption value='custom'>
              {t('Custom')}
            </NativeSelectOption>
          </NativeSelect>
          <Button
            variant='outline'
            size='icon'
            aria-label={t('Refresh')}
            title={t('Refresh')}
            disabled={query.isFetching}
            onClick={() => {
              if (
                preset !== 'custom' &&
                range.end !== Math.floor(Date.now() / 3600000) * 3600
              ) {
                selectPreset(preset)
              } else {
                void query.refetch()
              }
            }}
          >
            <RefreshCw className='size-4' />
          </Button>
        </div>
      </div>
      {preset === 'custom' && (
        <div className='grid items-end gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]'>
          <label className='min-w-0 flex-1 space-y-1 text-sm'>
            {t('Start time')}
            <Input
              type='datetime-local'
              step={3600}
              value={start}
              onChange={(event) => setStart(event.target.value)}
            />
          </label>
          <label className='min-w-0 flex-1 space-y-1 text-sm'>
            {t('End time')}
            <Input
              type='datetime-local'
              step={3600}
              value={end}
              onChange={(event) => setEnd(event.target.value)}
            />
          </label>
          <Button onClick={applyRange}>{t('Apply')}</Button>
        </div>
      )}
      {invalid && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Select whole hours within a past 31-day range.')}
        </p>
      )}
      <p className='text-muted-foreground text-xs'>
        {new Date(range.start * 1000).toLocaleString()} ~{' '}
        {new Date(range.end * 1000).toLocaleString()} (
        {Intl.DateTimeFormat().resolvedOptions().timeZone})
      </p>
      {query.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t(
            'Funding data unavailable. Check data collection and payment currencies.'
          )}
        </p>
      )}
      <dl
        className='grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-5'
        aria-busy={query.isFetching}
      >
        {items.map(([label, value]) => (
          <div key={label} className='min-w-0'>
            <dt className='text-muted-foreground text-sm'>{label}</dt>
            <dd className='mt-1 text-lg font-semibold break-words tabular-nums'>
              {value == null ? '--' : formatter.format(Number(value))}
            </dd>
          </div>
        ))}
        <div className='min-w-0'>
          <dt className='text-muted-foreground text-sm'>
            {t('Consumption share')}
          </dt>
          <dd className='mt-1 text-lg font-semibold tabular-nums'>
            {data?.consumption_ratio == null
              ? '--'
              : new Intl.NumberFormat(locale, {
                  style: 'percent',
                  maximumFractionDigits: 2,
                }).format(data.consumption_ratio)}
          </dd>
        </div>
      </dl>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Hourly consumption; gross funding before refunds. Manual credits include audited balance additions and manual top-up credits.'
        )}{' '}
        {data &&
          t('Calculated at {{time}}', {
            time: new Date(data.calculated_at * 1000).toLocaleString(),
          })}
      </p>
    </section>
  )
}
