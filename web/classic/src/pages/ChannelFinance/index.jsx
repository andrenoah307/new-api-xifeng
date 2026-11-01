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
import { Button, Modal, Tag, Tabs, TabPane } from '@douyinfe/semi-ui';
import {
  Calculator,
  ChevronLeft,
  ChevronRight,
  Eye,
  Download,
  Pause,
  Play,
  RefreshCw,
  Search,
  Trash2,
  X,
} from 'lucide-react';
import { API, downloadTextAsFile } from '../../helpers';
import { UserContext } from '../../context/User';
import './style.css';
import { formatReportAmount as amount } from './format';
import CostConfirmation from './CostConfirmation';
import { reportCSV } from './export';
import { FinanceCharts } from './Charts';
import { FinanceQuery } from './Query';
import FinanceAutomation from './Automation';

const statuses = {
  pending: 'Pending',
  running: 'Running',
  paused: 'Paused',
  cancelled: 'Cancelled',
  failed: 'Failed',
  completed: 'Completed',
};
const statusColors = {
  pending: 'grey',
  running: 'blue',
  paused: 'orange',
  cancelled: 'grey',
  failed: 'red',
  completed: 'green',
};
const columns = [
  ['cost', 'Cost'],
  ['known_revenue', 'Consumption'],
  ['known_profit', 'Profit'],
  ['known_profit_margin', 'Profit margin'],
];

function localTime(value) {
  return new Date(value.getTime() - value.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 19);
}

export default function ChannelFinance() {
  const [state] = useContext(UserContext);
  if (!state.user) return null;
  if (state.user?.role !== 100) return <Navigate to='/forbidden' replace />;
  return (
    <ChannelFinanceViews
      key={`${state.user.id}:${state.user.role}`}
      userId={state.user.id}
    />
  );
}

function ChannelFinanceViews({ userId }) {
  const { t } = useTranslation();
  const [view, setView] = useState('query');
  const [initialRange, setInitialRange] = useState();
  return (
    <main className='channel-finance'>
      <h1>{t('Revenue')}</h1>
      <Tabs activeKey={view} onChange={setView}>
        <TabPane tab={t('Live query')} itemKey='query'>
          <FinanceQuery
            active={view === 'query'}
            userId={userId}
            onSummarize={(range) => {
              setInitialRange(range);
              setView('tasks');
            }}
          />
        </TabPane>
        <TabPane tab={t('Summary tasks')} itemKey='tasks'>
          {view === 'tasks' && (
            <ChannelFinanceContent
              userId={userId}
              initialRange={initialRange}
              key={JSON.stringify(initialRange)}
            />
          )}
        </TabPane>
        <TabPane tab={t('Schedules and notifications')} itemKey='automation'>
          {view === 'automation' && <FinanceAutomation userId={userId} />}
        </TabPane>
      </Tabs>
    </main>
  );
}

function ChannelFinanceContent({ userId, initialRange }) {
  const { t } = useTranslation();
  const [state] = useContext(UserContext);
  const identity = useRef(state.user);
  identity.current = state.user;
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const [start, setStart] = useState(() =>
    localTime(
      initialRange
        ? new Date(initialRange.start * 1000)
        : new Date(new Date().setHours(0, 0, 0, 0)),
    ),
  );
  const [end, setEnd] = useState(() =>
    localTime(initialRange ? new Date(initialRange.end * 1000) : new Date()),
  );
  const [channel, setChannel] = useState(
    initialRange?.channel_id ? String(initialRange.channel_id) : '',
  );
  const [fallback, setFallback] = useState(Boolean(initialRange));
  const [pendingRequest, setPendingRequest] = useState(null);
  const [previewRefresh, setPreviewRefresh] = useState(0);
  const [selected, setSelected] = useState(null);
  const [filter, setFilter] = useState('');
  const [error, setError] = useState('');
  const [jobs, setJobs] = useState([]);
  const [keywordInput, setKeywordInput] = useState('');
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [total, setTotal] = useState(0);
  const [listLoading, setListLoading] = useState(true);
  const [listError, setListError] = useState('');
  const [reportError, setReportError] = useState('');
  const initialSelection = useRef(false);
  const [report, setReport] = useState(null);
  const [busy, setBusy] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const [removing, setRemoving] = useState(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState('');
  const [deleteResult, setDeleteResult] = useState('');
  const deletedIDs = useRef(new Set());
  async function deleteTasks() {
    if (
      deleting ||
      !removing ||
      identity.current?.role !== 100 ||
      identity.current?.id !== userId
    )
      return;
    setDeleting(true);
    setDeleteError('');
    try {
      const response = await API.delete(
        `/api/channel-finance/jobs${removing.id ? `/${encodeURIComponent(removing.id)}` : ''}`,
        { skipErrorHandler: true },
      );
      if (!response.data.success) throw new Error('delete failed');
      if (
        !mounted.current ||
        identity.current?.role !== 100 ||
        identity.current?.id !== userId
      )
        return;
      const { deleted_ids: ids, deleted_count: count } = response.data.data;
      ids.forEach((id) => deletedIDs.current.add(id));
      setJobs((items) =>
        items.filter((item) => !deletedIDs.current.has(item.id)),
      );
      setSelected((current) => (ids.includes(current) ? null : current));
      setReport((current) => (ids.includes(current?.job.id) ? null : current));
      setRemoving(null);
      setDeleteResult(t('Deleted {{count}} report tasks.', { count }));
      setRefresh((value) => value + 1);
    } catch (failure) {
      if (mounted.current)
        setDeleteError(
          failure.response?.status === 409
            ? t('Report task state changed. Refresh and try again.')
            : t('Operation failed'),
        );
    } finally {
      if (mounted.current) setDeleting(false);
    }
  }
  useEffect(() => {
    if (keywordInput.trim() === keyword) return;
    const timer = setTimeout(() => {
      setKeyword(keywordInput.trim());
      setPage(1);
    }, 350);
    return () => clearTimeout(timer);
  }, [keywordInput, keyword]);
  useEffect(() => {
    const controller = new AbortController();
    let timer;
    let active = true;
    setListLoading(true);
    async function load() {
      if (identity.current?.role !== 100 || identity.current?.id !== userId)
        return;
      try {
        const response = await API.get('/api/channel-finance/jobs', {
          params: { page, page_size: pageSize, keyword, status },
          signal: controller.signal,
          disableDuplicate: true,
          skipErrorHandler: true,
        });
        if (!response.data.success)
          throw new Error(response.data.message || t('Failed to load report'));
        if (!active) return;
        const data = response.data.data;
        const lastPage = Math.max(1, Math.ceil(data.total / pageSize));
        if (page > lastPage) {
          setPage(lastPage);
          return;
        }
        setJobs(data.items.filter((item) => !deletedIDs.current.has(item.id)));
        setTotal(data.total);
        if (!initialSelection.current) {
          initialSelection.current = true;
          if (data.items[0]) setSelected((value) => value ?? data.items[0].id);
        }
        setListError('');
        timer = setTimeout(load, 5000);
      } catch (failure) {
        if (active && !controller.signal.aborted) setListError(failure.message);
      } finally {
        if (active) setListLoading(false);
      }
    }
    void load();
    return () => {
      active = false;
      controller.abort();
      clearTimeout(timer);
    };
  }, [keyword, status, page, pageSize, refresh, userId, t]);

  useEffect(() => {
    setReport(null);
    setReportError('');
  }, [selected]);

  useEffect(() => {
    if (!selected) return;
    const controller = new AbortController();
    let timer;
    let active = true;
    async function load() {
      if (identity.current?.role !== 100 || identity.current?.id !== userId)
        return;
      try {
        const result = await API.get(
          `/api/channel-finance/jobs/${selected}/report`,
          {
            signal: controller.signal,
            disableDuplicate: true,
            skipErrorHandler: true,
          },
        );
        if (!result.data.success)
          throw new Error(result.data.message || t('Failed to load report'));
        if (!active || deletedIDs.current.has(selected)) return;
        setReport(result.data.data);
        setReportError('');
        if (['pending', 'running'].includes(result.data.data.job.status))
          timer = setTimeout(load, 2000);
      } catch (failure) {
        if (active && !controller.signal.aborted)
          setReportError(failure.message);
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
      const result = await API.post(`/api/channel-finance/jobs${path}`, body, {
        skipErrorHandler: true,
      });
      if (!result.data.success)
        throw new Error(result.data.message || t('Operation failed'));
      if (result.data.data?.id) setSelected(result.data.data.id);
      setRefresh((value) => value + 1);
      setError('');
      setPendingRequest(null);
    } catch (failure) {
      if (failure.response?.status === 409) {
        setError(
          t('Channel cost settings changed. Review the channel list again.'),
        );
        setPreviewRefresh((value) => value + 1);
      } else setError(failure.message);
    } finally {
      setBusy(false);
    }
  }
  async function download(id) {
    if (downloading) return;
    setDownloading(true);
    try {
      const response = await API.get(
        `/api/channel-finance/jobs/${encodeURIComponent(id)}/export`,
        {
          responseType: 'text',
          skipErrorHandler: true,
          disableDuplicate: true,
        },
      );
      if (!response.headers['content-type']?.startsWith('text/csv'))
        throw new Error(t('Operation failed'));
      if (
        !mounted.current ||
        identity.current?.id !== userId ||
        identity.current?.role !== 100
      )
        return;
      downloadTextAsFile(response.data, `revenue-${id}.csv`);
      setError('');
    } catch {
      setError(t('Operation failed'));
    } finally {
      setDownloading(false);
    }
  }
  function generate(event) {
    event.preventDefault();
    const startAt = Math.floor(new Date(start).getTime() / 1000);
    const endAt = Math.floor(new Date(end).getTime() / 1000);
    const channelId = channel === '' ? 0 : Number(channel);
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
          'Select a valid historical time range and confirm the estimation basis.',
        ),
      );
      return;
    }
    setError('');
    const body = {
      request_key: crypto.randomUUID(),
      start: startAt,
      end: endAt,
      channel_id: channelId,
      fallback_mode: fallback ? 'billed_basis' : 'none',
    };
    if (fallback) setPendingRequest(body);
    else void mutate('', body);
  }
  const job = report?.job;
  const progress = job?.total_windows
    ? Math.min(100, (job.processed_windows / job.total_windows) * 100)
    : 0;
  const rows =
    report?.channels.filter(
      (row) =>
        row.original_count + row.fallback_count > 0 &&
        `${row.channel_id} ${row.channel_name}`
          .toLowerCase()
          .includes(filter.toLowerCase()),
    ) ?? [];
  return (
    <div className='space-y-4'>
      {pendingRequest && (
        <CostConfirmation
          request={pendingRequest}
          refresh={previewRefresh}
          busy={busy}
          error={error}
          onClose={() => setPendingRequest(null)}
          onConfirm={(fingerprint) =>
            void mutate('', {
              ...pendingRequest,
              cost_fingerprint: fingerprint,
            })
          }
        />
      )}
      <form onSubmit={generate}>
        <fieldset className='finance-workflow'>
          <legend>{t('Summary workflow')}</legend>
          <label>
            <input
              type='radio'
              name='finance-workflow'
              value='none'
              checked={!fallback}
              onChange={() => {
                setFallback(false);
              }}
            />
            {t('Existing costs only (may be incomplete)')}
          </label>
          <label>
            <input
              type='radio'
              name='finance-workflow'
              value='billed_basis'
              checked={fallback}
              onChange={() => {
                setFallback(true);
              }}
            />
            {t('Full summary with channel-rate cost estimates')}
          </label>
        </fieldset>
        <div className='finance-fields'>
          <label>
            {t('Start time')}
            <input
              type='datetime-local'
              step='1'
              required
              value={start}
              onChange={(event) => setStart(event.target.value)}
            />
          </label>
          <label>
            {t('End time')}
            <input
              type='datetime-local'
              step='1'
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
        </div>
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
      <section className='finance-jobs' aria-label={t('Report task')}>
        <Modal
          width='min(520px, calc(100vw - 32px))'
          visible={removing !== null}
          title={
            removing?.id
              ? t('Delete report task?')
              : t('Clean up stopped tasks?')
          }
          onCancel={() => {
            if (!deleting) setRemoving(null);
          }}
          onOk={deleteTasks}
          confirmLoading={deleting}
          okText={t('Delete')}
          cancelText={t('Cancel')}
          okButtonProps={{ type: 'danger' }}
          cancelButtonProps={{ disabled: deleting }}
        >
          <p>
            {removing?.id
              ? t(
                  'Delete this report and its saved results? Source logs and account balances will not change.',
                )
              : t(
                  'Delete all cancelled, failed and paused report tasks whose execution has stopped, including tasks outside the current filters? Completed and active tasks will be kept.',
                )}
          </p>
          {removing?.id && <p className='finance-task-id'>{removing.id}</p>}
          {deleteError && (
            <p role='alert' className='finance-error'>
              {deleteError}
            </p>
          )}
        </Modal>
        <div className='finance-jobs-heading'>
          <h2>{t('Report task')}</h2>
          <Button
            icon={<Trash2 size={16} />}
            disabled={deleting}
            onClick={() => {
              setDeleteError('');
              setRemoving({ id: '' });
            }}
          >
            {t('Clean up tasks')}
          </Button>
        </div>
        {deleteResult && <p role='status'>{deleteResult}</p>}
        <div className='finance-toolbar finance-jobs-filters'>
          <label className='finance-job-search'>
            {t('Search')}
            <span className='finance-search-input'>
              <Search size={16} aria-hidden />
              <input
                type='search'
                value={keywordInput}
                maxLength={64}
                placeholder={t('Report task')}
                onChange={(event) => setKeywordInput(event.target.value)}
              />
            </span>
          </label>
          <label>
            {t('Status')}
            <select
              aria-label={t('Status')}
              value={status}
              onChange={(event) => {
                setStatus(event.target.value);
                setPage(1);
              }}
            >
              <option value=''>{t('All')}</option>
              {Object.entries(statuses).map(([value, label]) => (
                <option key={value} value={value}>
                  {t(label)}
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
        </div>
        {listError && (
          <p role='alert' className='finance-error'>
            {listError}
          </p>
        )}
        <div
          className='finance-table finance-jobs-table'
          aria-busy={listLoading}
        >
          <table>
            <thead>
              <tr>
                <th scope='col'>{t('Report task')}</th>
                <th scope='col'>{t('Time range')}</th>
                <th scope='col'>{t('Channel')}</th>
                <th scope='col'>{t('Status')}</th>
                <th scope='col'>{t('Progress')}</th>
                <th scope='col'>{t('操作')}</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((item) => {
                const percent = item.total_windows
                  ? Math.min(
                      100,
                      (item.processed_windows / item.total_windows) * 100,
                    )
                  : 0;
                return (
                  <tr key={item.id} data-selected={selected === item.id}>
                    <th scope='row'>
                      <span className='finance-task-id' title={item.id}>
                        #{item.id}
                      </span>
                    </th>
                    <td className='finance-task-range'>
                      <time
                        dateTime={new Date(item.start * 1000).toISOString()}
                      >
                        {new Date(item.start * 1000).toLocaleString()}
                      </time>
                      <time dateTime={new Date(item.end * 1000).toISOString()}>
                        {new Date(item.end * 1000).toLocaleString()}
                      </time>
                    </td>
                    <td>
                      {item.channel_id ? `#${item.channel_id}` : t('All')}
                    </td>
                    <td>
                      <Tag color={statusColors[item.status]}>
                        {t(statuses[item.status])}
                      </Tag>
                    </td>
                    <td>
                      <div className='finance-task-progress'>
                        <span>{percent.toFixed(1)}%</span>
                        <progress
                          aria-label={`${t('Progress')} #${item.id}`}
                          value={percent}
                          max={100}
                        />
                        <small>
                          {item.processed_windows}/{item.total_windows}
                        </small>
                      </div>
                    </td>
                    <td>
                      <Button
                        theme={selected === item.id ? 'light' : 'borderless'}
                        title={t('View')}
                        aria-label={`${t('View')} #${item.id}`}
                        aria-pressed={selected === item.id}
                        icon={<Eye size={16} />}
                        onClick={() => setSelected(item.id)}
                      />
                      <Button
                        theme='borderless'
                        title={t('Download report CSV')}
                        aria-label={`${t('Download report CSV')} ${item.id}`}
                        disabled={downloading}
                        icon={<Download size={16} />}
                        onClick={() => void download(item.id)}
                      />
                      <Button
                        theme='borderless'
                        title={t('Delete')}
                        aria-label={`${t('Delete')} ${item.id}`}
                        disabled={
                          deleting ||
                          ['pending', 'running'].includes(item.status)
                        }
                        icon={<Trash2 size={16} />}
                        onClick={() => {
                          setDeleteError('');
                          setRemoving({ id: item.id });
                        }}
                      />
                    </td>
                  </tr>
                );
              })}
              {!jobs.length && (
                <tr>
                  <td colSpan={6} className='finance-task-empty'>
                    {t(listLoading ? 'Loading' : 'No data')}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        <div className='finance-task-pagination'>
          <span>
            {t('Total')}: {total}
          </span>
          <label>
            {t('Page size')}
            <select
              aria-label={t('Page size')}
              value={pageSize}
              onChange={(event) => {
                setPageSize(Number(event.target.value));
                setPage(1);
              }}
            >
              {[10, 20, 50].map((size) => (
                <option key={size} value={size}>
                  {size}
                </option>
              ))}
            </select>
          </label>
          <span>
            {page} / {Math.max(1, Math.ceil(total / pageSize))}
          </span>
          <Button
            title={t('Previous')}
            aria-label={t('Previous')}
            icon={<ChevronLeft size={16} />}
            disabled={page <= 1 || listLoading}
            onClick={() => setPage((value) => value - 1)}
          />
          <Button
            title={t('Next')}
            aria-label={t('Next')}
            icon={<ChevronRight size={16} />}
            disabled={page * pageSize >= total || listLoading}
            onClick={() => setPage((value) => value + 1)}
          />
        </div>
      </section>
      {reportError && (
        <p role='alert' className='finance-error'>
          {reportError}
        </p>
      )}
      <div className='finance-toolbar'>
        {selected && (
          <h2 className='finance-selected-task'>
            {t('Report task')} <span>#{selected}</span>
          </h2>
        )}
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
            <p className='max-h-24 overflow-y-auto break-words'>
              {t('Summary workflow')}:{' '}
              {t(
                job.policy.fallback_mode === 'billed_basis'
                  ? 'Full summary with channel-rate cost estimates'
                  : 'Existing costs only (may be incomplete)',
              )}
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
            {columns.map(([key, label]) => (
              <div key={key}>
                <dt>{t(label)}</dt>
                <dd>{amount(report.total[key], key)}</dd>
              </div>
            ))}
          </dl>
          <div className='flex flex-wrap items-end gap-3 my-4'>
            <label className='finance-filter'>
              {t('Filter channels')}
              <input
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
              />
            </label>
            <Button
              icon={<Download size={16} />}
              disabled={!rows.length}
              onClick={() =>
                downloadTextAsFile(
                  reportCSV(report.job, rows),
                  `revenue-${report.job.id}-filtered.csv`,
                )
              }
            >
              {t('Export filtered results')}
            </Button>
          </div>
          <FinanceCharts rows={rows} />
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
    </div>
  );
}
