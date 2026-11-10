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
import { Calculator, Search } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';
import { FinanceCharts } from './Charts';
import { formatReportAmount as amount } from './format';
import { financePeriod, financeTime } from './query-range';

const metrics = [
  ['cost', 'Cost'],
  ['known_revenue', 'Consumption'],
  ['known_profit', 'Profit'],
  ['known_profit_margin', 'Profit margin'],
];

export function FinanceQuery(props) {
  const { t } = useTranslation();
  const [range, setRange] = useState(() => financePeriod('recent'));
  const [start, setStart] = useState(() => financeTime(range.start));
  const [end, setEnd] = useState(() => financeTime(range.end));
  const [channel, setChannel] = useState('');
  const [period, setPeriod] = useState('recent');
  const [error, setError] = useState('');
  const [filter, setFilter] = useState('');
  const [data, setData] = useState(null);
  const [failure, setFailure] = useState(null);
  const [fetching, setFetching] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const currentRange = useRef(range);
  currentRange.current = range;
  useEffect(() => {
    if (!props.active) return;
    let active = true;
    const abort = new AbortController();
    setData(null);
    async function fetchReport() {
      setFetching(true);
      try {
        const response = await API.get('/api/channel-finance/query', {
          params: range,
          signal: abort.signal,
          skipErrorHandler: true,
        });
        if (!response.data.success) throw new Error(response.data.message);
        if (active && currentRange.current === range) {
          setData(response.data.data);
          setFailure(null);
        }
      } catch (err) {
        if (active) setFailure(err);
      } finally {
        if (active) setFetching(false);
      }
    }
    void fetchReport();
    const timer = setInterval(fetchReport, 30000);
    return () => {
      active = false;
      abort.abort();
      clearInterval(timer);
    };
  }, [range, refresh, props.userId, props.active]);
  const result = {
    data,
    error: failure,
    isFetching: fetching,
    refetch: () => setRefresh((value) => value + 1),
  };
  const rows =
    result.data?.channels.filter(
      (row) =>
        row.original_count + row.fallback_count > 0 &&
        `${row.channel_id} ${row.channel_name}`
          .toLowerCase()
          .includes(filter.toLowerCase()),
    ) ?? [];
  function submit(event) {
    event.preventDefault();
    const next = {
      start: Date.parse(start + '+08:00') / 1000,
      end: Date.parse(end + '+08:00') / 1000,
      channel_id: Number(channel),
    };
    if (
      !Number.isSafeInteger(next.start) ||
      !Number.isSafeInteger(next.end) ||
      next.start < 1 ||
      next.end <= next.start ||
      next.end > Date.now() / 1000 ||
      !Number.isSafeInteger(next.channel_id) ||
      next.channel_id < 0
    ) {
      setError(
        t(
          'Select a valid historical time range and confirm the estimation basis.',
        ),
      );
      return;
    }
    setError('');
    setRange(next);
    if (
      next.start === range.start &&
      next.end === range.end &&
      next.channel_id === range.channel_id
    )
      void result.refetch();
  }
  return (
    <div className='space-y-5 min-w-0'>
      <form
        onSubmit={submit}
        className='flex flex-wrap items-end gap-3 border-b pb-5'
      >
        <label className='grid gap-2 text-sm'>
          {t('Time period')}
          <select
            className='h-9 rounded-md border bg-transparent px-2'
            value={period}
            onChange={(event) => {
              const value = event.target.value;
              setPeriod(value);
              if (value !== 'custom') {
                const next = financePeriod(value);
                setStart(financeTime(next.start));
                setEnd(financeTime(next.end));
              }
            }}
          >
            <option value='recent'>{t('Last 24 complete hours')}</option>
            <option value='day'>{t('Previous calendar day')}</option>
            <option value='week'>{t('Previous calendar week')}</option>
            <option value='half_month'>{t('Previous calendar half-month')}</option>
            <option value='month'>{t('Previous calendar month')}</option>
            <option value='quarter'>{t('Previous calendar quarter')}</option>
            <option value='year'>{t('Previous calendar year')}</option>
            <option value='custom'>{t('Custom')}</option>
          </select>
        </label>
        <label className='grid min-w-0 gap-2 text-sm'>
          {t('Start time')} (UTC+8)
          <input
            aria-label={t('Start time')}
            className='h-9 min-w-0 max-w-full rounded-md border bg-transparent px-2'
            type='datetime-local'
            step='1'
            required
            value={start}
            onChange={(event) => {
              setStart(event.target.value);
              setPeriod('custom');
            }}
          />
        </label>
        <label className='grid min-w-0 gap-2 text-sm'>
          {t('End time')} (UTC+8)
          <input
            aria-label={t('End time')}
            className='h-9 min-w-0 max-w-full rounded-md border bg-transparent px-2'
            type='datetime-local'
            step='1'
            required
            value={end}
            onChange={(event) => {
              setEnd(event.target.value);
              setPeriod('custom');
            }}
          />
        </label>
        <label className='grid gap-2 text-sm'>
          {t('Channel ID')}
          <input
            className='h-9 w-28 rounded-md border bg-transparent px-2'
            type='number'
            min='0'
            step='1'
            value={channel}
            onChange={(event) => setChannel(event.target.value)}
          />
        </label>
        <button
          type='submit'
          disabled={result.isFetching}
          className='inline-flex h-9 items-center gap-2 rounded-md border px-3 text-sm disabled:opacity-50'
        >
          <Search size={16} />
          {t('Query')}
        </button>
      </form>
      {(error || result.error) && (
        <p role='alert' className='text-red-600'>
          {error ||
            ((result.error?.response?.data?.message ||
              result.error?.message) ===
            'Too many summaries. Narrow the time range.'
              ? t('Too many summaries. Narrow the time range.')
              : t('Failed to load report'))}
        </p>
      )}
      {result.isFetching && (
        <p role='status' className='text-sm'>
          {t('Loading...')}
        </p>
      )}
      {result.data && !result.error && (
        <>
          <div className='flex flex-wrap justify-between gap-2 text-sm'>
            <span>
              {financeTime(range.start).replace('T', ' ')} ~{' '}
              {financeTime(range.end).replace('T', ' ')} (UTC+8)
            </span>
            <span>
              {t('Queried at')}:{' '}
              {financeTime(result.data.queried_at).replace('T', ' ')}
            </span>
          </div>
          {!result.data.complete ? (
            <section
              className='space-y-3 border-y py-4'
              aria-label={t('Missing summaries')}
            >
              <h3 className='font-semibold'>{t('Missing summaries')}</h3>
              <ul className='max-h-64 space-y-2 overflow-y-auto'>
                {result.data.missing.map((item) => (
                  <li
                    key={item.start}
                    className='flex flex-wrap items-center justify-between gap-2 text-sm'
                  >
                    <span>
                      {financeTime(item.start).replace('T', ' ')} ~{' '}
                      {financeTime(item.end).replace('T', ' ')} (UTC+8)
                    </span>
                    <button
                      type='button'
                      className='inline-flex items-center gap-2 rounded-md border px-3 py-2'
                      onClick={() =>
                        props.onSummarize({
                          ...item,
                          channel_id: range.channel_id,
                        })
                      }
                    >
                      <Calculator size={16} />
                      {t('Create summary')}
                    </button>
                  </li>
                ))}
              </ul>
              <button
                type='button'
                className='inline-flex items-center gap-2 rounded-md border px-3 py-2 text-sm'
                onClick={() => props.onSummarize(range)}
              >
                <Calculator size={16} />
                {t('Summarize selected range')}
              </button>
            </section>
          ) : (
            <>
              <dl className='grid grid-cols-2 gap-5 border-b py-5 lg:grid-cols-4'>
                {metrics.map(([key, label]) => (
                  <div key={key}>
                    <dt className='text-sm opacity-70'>{t(label)}</dt>
                    <dd className='mt-1 break-all text-lg font-semibold tabular-nums'>
                      {amount(result.data?.total?.[key] ?? null, key)}
                    </dd>
                  </div>
                ))}
              </dl>
              <label className='grid max-w-sm gap-2 text-sm'>
                {t('Filter channels')}
                <input
                  className='h-9 rounded-md border bg-transparent px-2'
                  value={filter}
                  onChange={(event) => setFilter(event.target.value)}
                />
              </label>
              <FinanceCharts rows={rows} />
              <div className='overflow-x-auto rounded-md border'>
                <table className='w-full text-sm'>
                  <thead>
                    <tr>
                      <th className='p-3 text-left'>{t('Channel')}</th>
                      {metrics.map(([key, label]) => (
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
                    {rows.map((row) => (
                      <tr key={row.channel_id} className='border-t'>
                        <th className='min-w-40 max-w-60 break-words p-3 text-left font-medium'>
                          {row.channel_name || '-'} #{row.channel_id}
                        </th>
                        {metrics.map(([key]) => (
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
                        <td colSpan={5} className='p-8 text-center'>
                          {t('No data')}
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
              <details className='text-sm'>
                <summary className='cursor-pointer'>
                  {t('Summary sources')} ({result.data.sources.length})
                </summary>
                <ul className='mt-3 max-h-48 space-y-2 overflow-y-auto'>
                  {result.data.sources.map((source, index) => (
                    <li key={index} className='break-all'>
                      #{source.job_id} ·{' '}
                      {financeTime(source.start).replace('T', ' ')} ~{' '}
                      {financeTime(source.end).replace('T', ' ')} (UTC+8)
                    </li>
                  ))}
                </ul>
              </details>
            </>
          )}
        </>
      )}
    </div>
  );
}
