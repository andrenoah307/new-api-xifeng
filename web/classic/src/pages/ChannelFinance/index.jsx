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
import React, { useContext, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate } from 'react-router-dom';
import { Button } from '@douyinfe/semi-ui';
import { Calculator, Pause, Play, Plus, RefreshCw, X } from 'lucide-react';
import { API } from '../../helpers';
import { UserContext } from '../../context/User';
import './style.css';
import { formatReportAmount as amount } from './format';

const statuses = {
  pending: 'Pending',
  running: 'Running',
  paused: 'Paused',
  cancelled: 'Cancelled',
  failed: 'Failed',
  completed: 'Completed',
};
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
];

function localTime(value) {
  return new Date(value.getTime() - value.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
}

export default function ChannelFinance() {
  const [state] = useContext(UserContext);
  if (!state.user) return null;
  if (state.user?.role !== 100) return <Navigate to='/forbidden' replace />;
  return (
    <ChannelFinanceContent
      key={`${state.user.id}:${state.user.role}`}
      userId={state.user.id}
    />
  );
}

function ChannelFinanceContent({ userId }) {
  const { t } = useTranslation();
  const [state] = useContext(UserContext);
  const identity = useRef(state.user);
  identity.current = state.user;
  const [start, setStart] = useState(() =>
    localTime(new Date(new Date().setHours(0, 0, 0, 0))),
  );
  const [end, setEnd] = useState(() => localTime(new Date()));
  const [channel, setChannel] = useState('');
  const [fallback, setFallback] = useState(false);
  const [ratio, setRatio] = useState('1');
  const [rates, setRates] = useState([
    { key: crypto.randomUUID(), channel: '', ratio: '1' },
  ]);
  const [accepted, setAccepted] = useState(false);
  const [selected, setSelected] = useState(null);
  const [filter, setFilter] = useState('');
  const [error, setError] = useState('');
  const [jobs, setJobs] = useState([]);
  const [report, setReport] = useState(null);
  const [busy, setBusy] = useState(false);
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    let timer;
    let active = true;
    setReport(null);
    async function load() {
      if (identity.current?.role !== 100 || identity.current?.id !== userId)
        return;
      try {
        const response = await API.get('/api/channel-finance/jobs', {
          signal: controller.signal,
          disableDuplicate: true,
        });
        if (!response.data.success)
          throw new Error(response.data.message || t('Failed to load report'));
        if (!active) return;
        setJobs(response.data.data);
        const id = selected ?? response.data.data[0]?.id;
        let pending = response.data.data.some((item) =>
          ['pending', 'running'].includes(item.status),
        );
        if (id !== undefined) {
          const result = await API.get(
            `/api/channel-finance/jobs/${id}/report`,
            { signal: controller.signal, disableDuplicate: true },
          );
          if (!result.data.success)
            throw new Error(result.data.message || t('Failed to load report'));
          if (!active) return;
          setReport(result.data.data);
          pending ||= ['pending', 'running'].includes(
            result.data.data.job.status,
          );
        }
        setError('');
        if (pending && active) timer = setTimeout(load, 2000);
      } catch (failure) {
        if (active && !controller.signal.aborted) setError(failure.message);
      }
    }
    void load();
    return () => {
      active = false;
      controller.abort();
      clearTimeout(timer);
    };
  }, [selected, refresh, userId, t]);

  async function mutate(path, body) {
    if (
      busy ||
      identity.current?.role !== 100 ||
      identity.current?.id !== userId
    )
      return;
    setBusy(true);
    try {
      const result = await API.post(`/api/channel-finance/jobs${path}`, body);
      if (!result.data.success)
        throw new Error(result.data.message || t('Operation failed'));
      if (result.data.data?.id) setSelected(result.data.data.id);
      setRefresh((value) => value + 1);
      setError('');
    } catch (failure) {
      setError(failure.message);
    } finally {
      setBusy(false);
    }
  }
  function generate(event) {
    event.preventDefault();
    const startAt = Math.floor(new Date(start).getTime() / 1000);
    const endAt = Math.floor(new Date(end).getTime() / 1000);
    const channelId = channel === '' ? 0 : Number(channel);
    const submittedRates = fallback
      ? channelId > 0
        ? [{ channel_id: channelId, ratio }]
        : rates.map((row) => ({
            channel_id: Number(row.channel),
            ratio: row.ratio,
          }))
      : [];
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
      );
      return;
    }
    void mutate('', {
      request_key: crypto.randomUUID(),
      start: startAt,
      end: endAt,
      channel_id: channelId,
      fallback_mode: fallback ? 'billed_basis' : 'none',
      rates: submittedRates,
    });
  }
  const job = report?.job;
  const progress = job?.total_windows
    ? Math.min(100, (job.processed_windows / job.total_windows) * 100)
    : 0;
  const rows =
    report?.channels.filter((row) =>
      `${row.channel_id} ${row.channel_name}`
        .toLowerCase()
        .includes(filter.toLowerCase()),
    ) ?? [];
  return (
    <main className='channel-finance'>
      <h1>{t('Channel finance')}</h1>
      <form onSubmit={generate}>
        <div className='finance-fields'>
          <label>
            {t('Start time')}
            <input
              type='datetime-local'
              required
              value={start}
              onChange={(event) => setStart(event.target.value)}
            />
          </label>
          <label>
            {t('End time')}
            <input
              type='datetime-local'
              required
              value={end}
              onChange={(event) => setEnd(event.target.value)}
            />
          </label>
          <label>
            {t('Channel ID (empty for all)')}
            <input
              type='number'
              min='1'
              step='1'
              value={channel}
              onChange={(event) => setChannel(event.target.value)}
            />
          </label>
          <label>
            {t('Historical cost basis')}
            <select
              value={fallback ? 'billed_basis' : 'none'}
              onChange={(event) => {
                setFallback(event.target.value === 'billed_basis');
                setAccepted(false);
              }}
            >
              <option value='none'>{t('Snapshot only')}</option>
              <option value='billed_basis'>
                {t('Billing-basis estimate')}
              </option>
            </select>
          </label>
        </div>
        {fallback && (
          <div className='finance-estimate'>
            {channel !== '' ? (
              <label>
                {t('Historical cost ratio')}
                <input
                  type='number'
                  min='0'
                  max='10000'
                  step='any'
                  value={ratio}
                  onChange={(event) => setRatio(event.target.value)}
                />
              </label>
            ) : (
              <div className='finance-rates'>
                {rates.map((row) => (
                  <div key={row.key} className='finance-rate-row'>
                    <label>
                      {t('Channel')}
                      <input
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
                          );
                          setAccepted(false);
                        }}
                      />
                    </label>
                    <label>
                      {t('Historical cost ratio')}
                      <input
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
                          );
                          setAccepted(false);
                        }}
                      />
                    </label>
                    <Button
                      htmlType='button'
                      title={t('Remove')}
                      aria-label={t('Remove')}
                      icon={<X size={16} />}
                      onClick={() => {
                        setRates((values) =>
                          values.filter((item) => item.key !== row.key),
                        );
                        setAccepted(false);
                      }}
                    />
                  </div>
                ))}
                <Button
                  htmlType='button'
                  icon={<Plus size={16} />}
                  onClick={() => {
                    setRates((values) => [
                      ...values,
                      { key: crypto.randomUUID(), channel: '', ratio: '1' },
                    ]);
                    setAccepted(false);
                  }}
                >
                  {t('Add')}
                </Button>
              </div>
            )}
            <label className='finance-check'>
              <input
                type='checkbox'
                checked={accepted}
                onChange={(event) => setAccepted(event.target.checked)}
              />
              {t(
                'I accept billing-basis estimates for missing snapshots throughout this report period.',
              )}
            </label>
          </div>
        )}
        <Button
          htmlType='submit'
          theme='solid'
          disabled={busy}
          icon={<Calculator size={16} />}
        >
          {t('Generate report')}
        </Button>
      </form>
      {error && (
        <p role='alert' className='finance-error'>
          {error}
        </p>
      )}
      <div className='finance-toolbar'>
        <label className='finance-job'>
          {t('Report task')}
          <select
            value={selected ?? jobs[0]?.id ?? ''}
            onChange={(event) => setSelected(event.target.value)}
          >
            <option value='' disabled>
              {t('No data')}
            </option>
            {jobs.map((item) => (
              <option key={item.id} value={item.id}>
                #{item.id} · {t(statuses[item.status])} ·{' '}
                {new Date(item.start * 1000).toLocaleString()} -{' '}
                {new Date(item.end * 1000).toLocaleString()}
              </option>
            ))}
          </select>
        </label>
        <Button
          title={t('Refresh')}
          aria-label={t('Refresh')}
          icon={<RefreshCw size={16} />}
          onClick={() => setRefresh((value) => value + 1)}
        />
        {job && (
          <>
            {['pending', 'running'].includes(job.status) && (
              <Button
                disabled={busy}
                icon={<Pause size={16} />}
                onClick={() => mutate(`/${job.id}/pause`)}
              >
                {t('Pause')}
              </Button>
            )}
            {['paused', 'failed'].includes(job.status) && (
              <Button
                disabled={busy}
                icon={<Play size={16} />}
                onClick={() => mutate(`/${job.id}/resume`)}
              >
                {t('Resume')}
              </Button>
            )}
            {['pending', 'running', 'paused'].includes(job.status) && (
              <Button
                disabled={busy}
                icon={<X size={16} />}
                onClick={() => mutate(`/${job.id}/cancel`)}
              >
                {t('Cancel')}
              </Button>
            )}
          </>
        )}
      </div>
      {job && (
        <section className='finance-progress'>
          <div>
            {t(statuses[job.status])} · {job.processed_windows}/
            {job.total_windows} · {progress.toFixed(1)}%
          </div>
          <div>
            {t('Aggregated through')}:{' '}
            {new Date(job.cursor * 1000).toLocaleString()}
          </div>
          <progress value={progress} max={100} />
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
            <p role='alert' className='finance-error'>
              {job.error}
            </p>
          )}
        </section>
      )}
      {report && (
        <>
          <dl className='finance-summary'>
            {['net_amount', 'cost', 'known_profit', 'coverage'].map((key) => (
              <div key={key}>
                <dt>{t(columns.find((column) => column[0] === key)[1])}</dt>
                <dd>{amount(report.total[key], key)}</dd>
              </div>
            ))}
          </dl>
          <label className='finance-filter'>
            {t('Filter channels')}
            <input
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
            />
          </label>
          <div className='finance-table'>
            <table>
              <thead>
                <tr>
                  <th>{t('Channel')}</th>
                  {columns.map(([key, label]) => (
                    <th key={key}>{t(label)}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.channel_id}>
                    <th>
                      {row.channel_name || '-'} #{row.channel_id}
                    </th>
                    {columns.map(([key]) => (
                      <td key={key}>{amount(row[key], key)}</td>
                    ))}
                  </tr>
                ))}
                {!rows.length && (
                  <tr>
                    <td colSpan={columns.length + 1}>{t('No data')}</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </main>
  );
}
