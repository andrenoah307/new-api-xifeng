/*
Copyright (C) 2025 QuantumNous

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
import React, { useContext, useEffect, useState } from 'react';
import { Button, Select } from '@douyinfe/semi-ui';
import { RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';
import { UserContext } from '../../context/User';

function localHour(timestamp) {
  const date = new Date(timestamp * 1000);
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
}

export default function FundingOverview() {
  const [userState] = useContext(UserContext);
  if (userState.user?.role !== 100) return null;
  return <FundingOverviewContent key={userState.user.id} />;
}

function FundingOverviewContent() {
  const { t, i18n } = useTranslation();
  const [range, setRange] = useState(() => {
    const end = Math.floor(Date.now() / 3600000) * 3600;
    return { start: end - 86400, end };
  });
  const [preset, setPreset] = useState('1');
  const [start, setStart] = useState(() => localHour(range.start));
  const [end, setEnd] = useState(() => localHour(range.end));
  const [invalid, setInvalid] = useState(false);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setFailed(false);
    setData(null);
    API.get('/api/data/funding', {
      params: { start_timestamp: range.start, end_timestamp: range.end },
      signal: controller.signal,
      disableDuplicate: true,
      skipErrorHandler: true,
    })
      .then(({ data: result }) => {
        if (controller.signal.aborted) return;
        if (result.success) setData(result.data);
        else setFailed(true);
      })
      .catch(() => {
        if (!controller.signal.aborted) setFailed(true);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [range, refresh]);
  const selectPreset = (value) => {
    setPreset(value);
    setInvalid(false);
    if (value === 'custom') return;
    const until = Math.floor(Date.now() / 3600000) * 3600;
    const from = until - Number(value) * 86400;
    setStart(localHour(from));
    setEnd(localHour(until));
    setRange({ start: from, end: until });
  };
  const applyRange = () => {
    const from = new Date(start).getTime() / 1000;
    const until = new Date(end).getTime() / 1000;
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
      setInvalid(true);
      return;
    }
    setInvalid(false);
    setRange({ start: from, end: until });
  };
  const formatter = new Intl.NumberFormat(i18n.language, {
    style: 'currency',
    currency: 'CNY',
    maximumFractionDigits: 2,
  });
  const items = [
    [t('Consumption amount'), data?.consumption],
    [t('Successful online top-ups'), data?.online_topups],
    [t('Administrator manual credits'), data?.manual_credits],
    [t('Top-ups + manual credits'), data?.total_funding],
  ];
  return (
    <section
      className='my-4 border-y py-4 space-y-4 min-w-0'
      aria-label={t('Funding overview')}
    >
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <h3 className='text-base font-semibold'>{t('Funding overview')}</h3>
        <div className='flex items-center gap-2'>
          <Select
            aria-label={t('Time range')}
            value={preset}
            onChange={selectPreset}
            optionList={[
              ...[1, 3, 7, 30].map((days) => ({
                value: String(days),
                label: t('Last {{days}} days', { days }),
              })),
              { value: 'custom', label: t('Custom') },
            ]}
          />
          <Button
            icon={<RefreshCw size={16} />}
            aria-label={t('Refresh')}
            title={t('Refresh')}
            disabled={loading}
            onClick={() =>
              preset === 'custom'
                ? setRefresh((value) => value + 1)
                : selectPreset(preset)
            }
          />
        </div>
      </div>
      {preset === 'custom' && (
        <div className='grid grid-cols-1 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-end gap-3'>
          <label className='flex-1 min-w-0 text-sm'>
            {t('Start time')}
            <input
              className='block w-full border rounded p-1 bg-transparent'
              type='datetime-local'
              step={3600}
              value={start}
              onChange={(event) => setStart(event.target.value)}
            />
          </label>
          <label className='flex-1 min-w-0 text-sm'>
            {t('End time')}
            <input
              className='block w-full border rounded p-1 bg-transparent'
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
        <p role='alert'>
          {t('Select whole hours within a past 31-day range.')}
        </p>
      )}
      <p className='text-xs text-gray-500'>
        {new Date(range.start * 1000).toLocaleString()} ~{' '}
        {new Date(range.end * 1000).toLocaleString()} (
        {Intl.DateTimeFormat().resolvedOptions().timeZone})
      </p>
      {failed && (
        <p role='alert'>
          {t(
            'Funding data unavailable. Check data collection and payment currencies.',
          )}
        </p>
      )}
      <dl
        className='grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-5 gap-4'
        aria-busy={loading}
      >
        {items.map(([label, value]) => (
          <div key={label} className='min-w-0'>
            <dt className='text-sm text-gray-500'>{label}</dt>
            <dd className='mt-1 text-lg font-semibold break-words tabular-nums'>
              {value == null ? '--' : formatter.format(Number(value))}
            </dd>
          </div>
        ))}
        <div>
          <dt className='text-sm text-gray-500'>{t('Consumption share')}</dt>
          <dd className='mt-1 text-lg font-semibold tabular-nums'>
            {data?.consumption_ratio == null
              ? '--'
              : new Intl.NumberFormat(i18n.language, {
                  style: 'percent',
                  maximumFractionDigits: 2,
                }).format(data.consumption_ratio)}
          </dd>
        </div>
      </dl>
      <p className='text-xs text-gray-500'>
        {t(
          'Hourly consumption; gross funding before refunds. Manual credits include audited balance additions and manual top-up credits.',
        )}{' '}
        {data &&
          t('Calculated at {{time}}', {
            time: new Date(data.calculated_at * 1000).toLocaleString(),
          })}
      </p>
    </section>
  );
}
