import React, { useEffect, useState } from 'react';
import { Button } from '@douyinfe/semi-ui';
import { Eye, Mail, Plus, RefreshCw, Save, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';
import CostConfirmation from './CostConfirmation';
import './automation.css';

const periods = [
  ['day', 'Previous day'],
  ['week', 'Previous week'],
  ['month', 'Previous month'],
  ['quarter', 'Previous quarter'],
  ['year', 'Previous year'],
];
const metrics = [
  ['consumption', 'Consumption'],
  ['cost', 'Cost_finance'],
  ['profit', 'Profit'],
  ['profit_margin', 'Profit margin'],
];
const cadences = [
  ['day', 'Daily'],
  ['week', 'Weekly'],
  ['month', 'Monthly'],
  ['quarter', 'Quarterly'],
  ['year', 'Yearly'],
];
const formatTime = (value) =>
  new Date(value * 1000).toLocaleString('zh-CN', {
    timeZone: 'Asia/Shanghai',
    hour12: false,
  });

export default function FinanceAutomation({ userId }) {
  const { t } = useTranslation();
  const [data, setData] = useState(null);
  const [error, setError] = useState('');
  const [reload, setReload] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    let pending = false;
    const load = async () => {
      if (pending || document.hidden) return;
      pending = true;
      try {
        const result = await API.get('/api/channel-finance/automation', {
          signal: controller.signal,
          disableDuplicate: true,
        });
        if (!result.data.success) throw new Error(result.data.message);
        if (!controller.signal.aborted) {
          setData(result.data.data);
          setError('');
        }
      } catch (failure) {
        if (!controller.signal.aborted) setError(failure.message);
      } finally {
        pending = false;
      }
    };
    load();
    const timer = setInterval(load, 30000);
    document.addEventListener('visibilitychange', load);
    return () => {
      controller.abort();
      clearInterval(timer);
      document.removeEventListener('visibilitychange', load);
    };
  }, [userId, reload]);
  if (!data)
    return <p role={error ? 'alert' : undefined}>{error || t('Loading...')}</p>;
  return (
    <AutomationEditor
      key={data.revision}
      data={data}
      userId={userId}
      reload={() => setReload((value) => value + 1)}
    />
  );
}

function AutomationEditor({ data, userId, reload }) {
  const { t } = useTranslation();
  const [config, setConfig] = useState(data.config);
  const [recipients, setRecipients] = useState(
    data.config.recipients.join('\n'),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [confirm, setConfirm] = useState(null);
  const [preview, setPreview] = useState(null);
  const [detail, setDetail] = useState(null);
  const update = (key, value) => setConfig((old) => ({ ...old, [key]: value }));
  const fieldUpdate = (index, key, value) =>
    setConfig((old) => ({
      ...old,
      fields: old.fields.map((field, i) =>
        i === index ? { ...field, [key]: value } : field,
      ),
    }));
  const payload = () => ({
    ...config,
    recipients: recipients
      .split(/[\n;,]+/)
      .map((item) => item.trim())
      .filter(Boolean),
  });
  const save = async (value) => {
    setBusy(true);
    setError('');
    setMessage('');
    try {
      const result = await API.put('/api/channel-finance/automation', {
        revision: data.revision,
        config: value,
      });
      if (!result.data.success) throw new Error(result.data.message);
      setConfirm(null);
      setMessage(t('Saved'));
      reload();
    } catch (failure) {
      setError(failure.response?.data?.message || failure.message);
    } finally {
      setBusy(false);
    }
  };
  const mail = async (action) => {
    setBusy(true);
    setError('');
    setMessage('');
    try {
      const result = await API.post(
        `/api/channel-finance/automation/mail/${action}`,
        payload(),
      );
      if (!result.data.success) throw new Error(result.data.message);
      setPreview(result.data.data);
      if (action === 'test') setMessage(t('Test email sent'));
    } catch (failure) {
      setError(failure.response?.data?.message || failure.message);
    } finally {
      setBusy(false);
    }
  };
  const inspect = async (id) => {
    try {
      const result = await API.get(
        `/api/channel-finance/automation/runs/${id}`,
      );
      if (!result.data.success) throw new Error(result.data.message);
      setDetail(result.data.data);
    } catch (failure) {
      setError(failure.message);
    }
  };
  const control = async (run, action) => {
    if (
      !window.confirm(
        t(
          action === 'retry'
            ? 'Retry may resend uncertain email. Continue?'
            : 'Dismiss this automation run?',
        ),
      )
    )
      return;
    setBusy(true);
    setError('');
    try {
      const result = await API.post(
        `/api/channel-finance/automation/runs/${run.id}/${action}?confirm_duplicate_risk=true`,
      );
      if (!result.data.success) throw new Error(result.data.message);
      setDetail(null);
      reload();
    } catch (failure) {
      setError(failure.response?.data?.message || failure.message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className='revenue-automation'>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          const value = payload();
          if (value.enabled) setConfirm(value);
          else save(value);
        }}
      >
        <fieldset disabled={busy}>
          <legend>{t('Scheduled summaries')}</legend>
          <label className='revenue-check'>
            <input
              type='checkbox'
              checked={config.enabled}
              onChange={(event) => update('enabled', event.target.checked)}
            />
            {t('Enabled')}
          </label>
          <label>
            {t('Execution time (UTC+8)')}
            <input
              type='time'
              required
              value={config.time}
              onChange={(event) => update('time', event.target.value)}
            />
          </label>
          <div className='revenue-options'>
            {periods.map(([key, label]) => (
              <label key={key} className='revenue-check'>
                <input
                  type='checkbox'
                  checked={config.periods.includes(key)}
                  onChange={(event) =>
                    update(
                      'periods',
                      event.target.checked
                        ? [...config.periods, key]
                        : config.periods.filter((item) => item !== key),
                    )
                  }
                />
                {t(label)}
              </label>
            ))}
          </div>
          <p>
            {t('Next run')}:{' '}
            {config.enabled && data.next_at ? formatTime(data.next_at) : '-'}
          </p>
        </fieldset>
        <fieldset disabled={busy}>
          <legend>{t('Email notifications')}</legend>
          <label className='revenue-check'>
            <input
              type='checkbox'
              checked={config.mail_enabled}
              onChange={(event) => update('mail_enabled', event.target.checked)}
            />
            {t('Enabled')}
          </label>
          <label>
            {t('Mail cadence')}
            <select
              value={config.mail_period}
              onChange={(event) => update('mail_period', event.target.value)}
            >
              {cadences.map(([key, label]) => (
                <option key={key} value={key}>
                  {t(label)}
                </option>
              ))}
            </select>
          </label>
          <label>
            {t('Recipients')}
            <textarea
              rows={3}
              value={recipients}
              onChange={(event) => setRecipients(event.target.value)}
            />
          </label>
          <p>{t('Known-cost records only')}</p>
          {config.fields.map((field, index) => (
            <div className='revenue-field' key={index}>
              <label>
                {t('Field key')}
                <input
                  required
                  pattern='[\p{L}][\p{L}\p{N}_]{0,47}'
                  value={field.key}
                  onChange={(event) =>
                    fieldUpdate(index, 'key', event.target.value)
                  }
                />
              </label>
              <label>
                {t('Metric')}
                <select
                  value={field.metric}
                  onChange={(event) =>
                    fieldUpdate(index, 'metric', event.target.value)
                  }
                >
                  {metrics.map(([key, label]) => (
                    <option key={key} value={key}>
                      {t(label)}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                {t('Time window')}
                <select
                  value={field.period}
                  onChange={(event) =>
                    fieldUpdate(index, 'period', event.target.value)
                  }
                >
                  {periods.map(([key, label]) => (
                    <option key={key} value={key}>
                      {t(label)}
                    </option>
                  ))}
                  <option value='days'>{t('Previous complete days')}</option>
                </select>
              </label>
              <label>
                {t('Days')}
                <input
                  type='number'
                  min={1}
                  max={366}
                  disabled={field.period !== 'days'}
                  value={field.days}
                  onChange={(event) =>
                    fieldUpdate(index, 'days', Number(event.target.value))
                  }
                />
              </label>
              <label>
                {t('Channel ID (0 = all)')}
                <input
                  type='number'
                  min={0}
                  value={field.channel_id}
                  onChange={(event) =>
                    fieldUpdate(index, 'channel_id', Number(event.target.value))
                  }
                />
              </label>
              <Button
                htmlType='button'
                icon={<Trash2 size={16} />}
                title={t('Delete')}
                aria-label={t('Delete')}
                onClick={() =>
                  update(
                    'fields',
                    config.fields.filter((_, i) => i !== index),
                  )
                }
              />
            </div>
          ))}
          <Button
            htmlType='button'
            icon={<Plus size={16} />}
            disabled={config.fields.length >= 20}
            onClick={() =>
              update('fields', [
                ...config.fields,
                {
                  key: `field_${config.fields.length + 1}`,
                  metric: 'profit',
                  period: 'day',
                  days: 1,
                  channel_id: 0,
                },
              ])
            }
          >
            {t('Add field')}
          </Button>
          {data.chinese_template && (
            <Button
              type='button'
              icon={<Mail size={16} />}
              onClick={() => {
                setConfig((old) => ({
                  ...old,
                  subject: data.chinese_template.subject,
                  body: data.chinese_template.body,
                }));
                setPreview(null);
              }}
            >
              {t('Use Chinese report template')}
            </Button>
          )}
          <label>
            {t('Subject')}
            <input
              maxLength={200}
              value={config.subject}
              onChange={(event) => update('subject', event.target.value)}
            />
          </label>
          <label>
            {t('Body')}
            <textarea
              rows={8}
              maxLength={32768}
              value={config.body}
              onChange={(event) => update('body', event.target.value)}
            />
          </label>
          <label>
            {t('Insert placeholder')}
            <select
              value=''
              onChange={(event) =>
                update('body', `${config.body}{{${event.target.value}}}`)
              }
            >
              <option value='' disabled>
                {t('Insert placeholder')}
              </option>
              {[
                'system_name',
                'scheduled_at',
                'generated_at',
                'run_id',
                'summary_table',
                'channel_reports',
                ...config.fields.flatMap((field) =>
                  ['value', 'start', 'end', 'unit', 'coverage'].map(
                    (part) => `fields.${field.key}.${part}`,
                  ),
                ),
              ].map((key) => (
                <option key={key} value={key}>{`{{${key}}}`}</option>
              ))}
            </select>
          </label>
          <div className='revenue-options'>
            <Button
              htmlType='button'
              icon={<Eye size={16} />}
              onClick={() => mail('preview')}
            >
              {t('Preview sample')}
            </Button>
            <Button
              htmlType='button'
              icon={<Mail size={16} />}
              onClick={() => mail('test')}
            >
              {t('Send test to myself')}
            </Button>
          </div>
        </fieldset>
        <Button
          htmlType='submit'
          theme='solid'
          loading={busy}
          icon={<Save size={16} />}
        >
          {t('Save')}
        </Button>
      </form>
      {error && <p role='alert'>{error}</p>}
      {message && <p role='status'>{message}</p>}
      {preview && (
        <section>
          <h3>
            {t('Preview sample')}: {preview.subject}
          </h3>
          <iframe
            title={t('Preview sample')}
            sandbox=''
            referrerPolicy='no-referrer'
            srcDoc={`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; form-action 'none'; base-uri 'none'">${preview.body}`}
          />
        </section>
      )}
      <section>
        <div className='revenue-options'>
          <h3>{t('Execution history')}</h3>
          <Button
            htmlType='button'
            icon={<RefreshCw size={16} />}
            title={t('Refresh')}
            aria-label={t('Refresh')}
            onClick={reload}
          />
        </div>
        <div className='revenue-table'>
          <table>
            <thead>
              <tr>
                <th>{t('Scheduled time')}</th>
                <th>{t('Status')}</th>
                <th>{t('Details')}</th>
              </tr>
            </thead>
            <tbody>
              {data.runs.map((run) => (
                <tr key={run.id}>
                  <td>{formatTime(run.scheduled_at)}</td>
                  <td>{t(`Revenue status ${run.status}`)}</td>
                  <td>
                    <Button
                      htmlType='button'
                      icon={<Eye size={16} />}
                      title={t('Details')}
                      aria-label={t('Details')}
                      onClick={() => inspect(run.id)}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {detail && (
          <div className='revenue-run'>
            <p>{detail.run.id}</p>
            {detail.run.error && <p role='alert'>{detail.run.error}</p>}
            {detail.data.job_id && (
              <p>
                {t('Summary task')}: {detail.data.job_id}
              </p>
            )}
            {detail.data.deliveries?.map((item) => (
              <p key={item.recipient}>
                {item.recipient}: {t(`Revenue status ${item.status}`)}
              </p>
            ))}
            {['failed', 'unknown'].includes(detail.run.status) && (
              <div className='revenue-options'>
                <Button
                  disabled={busy}
                  onClick={() => control(detail.run, 'retry')}
                >
                  {t('Retry')}
                </Button>
                <Button
                  disabled={busy}
                  onClick={() => control(detail.run, 'dismiss')}
                >
                  {t('Dismiss')}
                </Button>
              </div>
            )}
            <dl>
              {Object.entries(detail.data.values ?? {})
                .filter(
                  ([key]) =>
                    key !== 'summary_table' && key !== 'channel_reports',
                )
                .map(([key, value]) => (
                  <div key={key}>
                    <dt>{key}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
            </dl>
          </div>
        )}
      </section>
      {confirm && (
        <CostConfirmation
          confirmLabel='Confirm and enable'
          request={{
            request_key: 'automation',
            start: 1,
            end: 2,
            channel_id: 0,
            fallback_mode: 'billed_basis',
          }}
          userId={userId}
          busy={busy}
          error={error}
          onClose={() => setConfirm(null)}
          onConfirm={(fingerprint) => save({ ...confirm, fingerprint })}
        />
      )}
    </div>
  );
}
