import React, { useEffect, useState } from 'react';
import {
  Button,
  InputNumber,
  Select,
  Spin,
  Switch,
  Typography,
} from '@douyinfe/semi-ui';
import { Plus, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const defaults = {
  enabled: false,
  rules: [
    { id: 'warning', percent: 50, cooldown_days: 7, scope: 'any' },
    { id: 'critical', percent: 70, cooldown_days: 3, scope: 'any' },
  ],
};

export default function TokenUsageAlertSetting() {
  const { t } = useTranslation();
  const [config, setConfig] = useState(defaults);
  const [status, setStatus] = useState(null);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [preview, setPreview] = useState(null);
  const [previewScope, setPreviewScope] = useState('total');
  const [previewBusy, setPreviewBusy] = useState(false);
  useEffect(() => {
    let active = true;
    API.get('/api/option/')
      .then(({ data }) => {
        if (!active) return;
        if (!data.success) throw new Error(data.message);
        const option = data.data.find(
          (item) => item.key === 'TokenUsageAlertRules',
        );
        if (option) setConfig(JSON.parse(option.value));
        setLoaded(true);
      })
      .catch((error) => {
        if (active) showError(error.message);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    const refresh = () =>
      API.get('/api/option/token_usage_alerts/status')
        .then(({ data }) => {
          if (!active) return;
          if (!data.success) throw new Error(data.message);
          setStatus(data.data);
        })
        .catch(() => {
          if (active) setStatus(null);
        });
    refresh();
    const timer = setInterval(refresh, 30000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, []);
  const update = (id, key, value) =>
    setConfig((previous) => ({
      ...previous,
      rules: previous.rules.map((rule) =>
        rule.id === id ? { ...rule, [key]: value } : rule,
      ),
    }));
  const save = async () => {
    if (!loaded) return;
    if (
      (config.enabled && !config.rules.length) ||
      config.rules.length > 20 ||
      config.rules.some(
        (rule) =>
          !Number.isInteger(rule.percent) ||
          rule.percent < 1 ||
          rule.percent > 100 ||
          !Number.isInteger(rule.cooldown_days) ||
          rule.cooldown_days < 1 ||
          rule.cooldown_days > 365,
      )
    ) {
      showError(t('Invalid API Key usage alert settings'));
      return;
    }
    setSaving(true);
    try {
      const { data } = await API.put('/api/option/', {
        key: 'TokenUsageAlertRules',
        value: JSON.stringify(config),
      });
      if (!data.success) {
        showError(t(data.message));
        return;
      }
      showSuccess(t('Saved successfully'));
    } catch (error) {
      showError(error.message);
    } finally {
      setSaving(false);
    }
  };
  return (
    <Spin spinning={loading}>
      <section className='p-4'>
        <Typography.Title heading={5}>
          {t('User API Key usage email alerts')}
        </Typography.Title>
        <div className='flex items-center gap-3 my-4'>
          <label htmlFor='token-alert-enabled'>{t('Enable alerts')}</label>
          <Switch
            id='token-alert-enabled'
            checked={config.enabled}
            onChange={(enabled) =>
              setConfig((previous) => ({ ...previous, enabled }))
            }
          />
        </div>
        {config.rules.map((rule, index) => (
          <div
            key={rule.id}
            className='grid grid-cols-1 sm:grid-cols-4 gap-3 border-b py-4'
          >
            <div>
              <label htmlFor={`token-alert-scope-${index}`}>
                {t('Quota scope')}
              </label>
              <Select
                id={`token-alert-scope-${index}`}
                style={{ width: '100%' }}
                value={rule.scope}
                onChange={(value) => update(rule.id, 'scope', value)}
                optionList={[
                  { value: 'any', label: t('Total and period quotas') },
                  { value: 'total', label: t('Total quota') },
                  { value: 'period', label: t('Current period quota') },
                ]}
              />
            </div>
            <div>
              <label htmlFor={`token-alert-percent-${index}`}>
                {t('Usage threshold (%)')}
              </label>
              <InputNumber
                id={`token-alert-percent-${index}`}
                style={{ width: '100%' }}
                min={1}
                max={100}
                precision={0}
                value={rule.percent}
                onChange={(value) => update(rule.id, 'percent', value)}
              />
            </div>
            <div>
              <label htmlFor={`token-alert-days-${index}`}>
                {t('Email cooldown (days)')}
              </label>
              <InputNumber
                id={`token-alert-days-${index}`}
                style={{ width: '100%' }}
                min={1}
                max={365}
                precision={0}
                value={rule.cooldown_days}
                onChange={(value) => update(rule.id, 'cooldown_days', value)}
              />
            </div>
            <div className='flex items-end'>
              <Button
                icon={<Trash2 size={18} />}
                aria-label={t('Delete rule')}
                title={t('Delete rule')}
                onClick={() =>
                  setConfig((previous) => ({
                    ...previous,
                    rules: previous.rules.filter((item) => item.id !== rule.id),
                  }))
                }
              />
            </div>
          </div>
        ))}
        <div className='flex gap-3 my-4'>
          <Button
            disabled={config.rules.length >= 20}
            icon={<Plus size={18} />}
            onClick={() =>
              setConfig((previous) => ({
                ...previous,
                rules: [
                  ...previous.rules,
                  {
                    id: crypto.randomUUID(),
                    percent: 90,
                    cooldown_days: 1,
                    scope: 'any',
                  },
                ],
              }))
            }
          >
            {t('Add rule')}
          </Button>
        </div>
        <div className='flex flex-col gap-3 my-4'>
          <label>
            {t('Subject')}
            <input
              className='w-full border p-2'
              maxLength={800}
              value={config.subject || ''}
              onChange={(e) =>
                setConfig((old) => ({ ...old, subject: e.target.value }))
              }
            />
          </label>
          <label>
            {t('Body')}
            <textarea
              className='w-full border p-2'
              rows={8}
              maxLength={32768}
              value={config.body || ''}
              onChange={(e) =>
                setConfig((old) => ({ ...old, body: e.target.value }))
              }
            />
          </label>
          <select
            aria-label={t('Insert placeholder')}
            className='w-full border p-2'
            value=''
            onChange={(e) => {
              const key = e.target.value;
              if (key)
                setConfig((old) => ({
                  ...old,
                  body: `${old.body || ''}{{${key}}}`,
                }));
            }}
          >
            <option value=''>{t('Insert placeholder')}</option>
            {status?.placeholders?.map((key) => (
              <option value={key} key={key}>
                {key}
              </option>
            ))}
          </select>
          <div className='flex flex-wrap gap-2'>
            <Button
              disabled={!status?.builtin_template}
              onClick={() => {
                setConfig((old) => ({ ...old, ...status.builtin_template }));
                setPreview(null);
              }}
            >
              {t('Use built-in notification template')}
            </Button>
            <select
              aria-label={t('Quota scope')}
              className='border p-2'
              value={previewScope}
              onChange={(e) => setPreviewScope(e.target.value)}
            >
              <option value='total'>{t('Total quota')}</option>
              <option value='period'>{t('Current period quota')}</option>
            </select>
            <Button
              disabled={!loaded}
              loading={previewBusy}
              onClick={async () => {
                setPreviewBusy(true);
                setPreview(null);
                try {
                  const { data } = await API.post(
                    '/api/option/token_usage_alerts/preview',
                    { config, scope: previewScope },
                  );
                  if (!data.success) throw new Error(data.message);
                  setPreview(data.data);
                } catch (error) {
                  showError(t(error.message));
                } finally {
                  setPreviewBusy(false);
                }
              }}
            >
              {t('Preview sample')}
            </Button>
          </div>
          {preview && (
            <section>
              <h3>{preview.subject}</h3>
              <iframe
                title={t('Preview sample')}
                sandbox=''
                style={{ width: '100%', height: 384, border: '1px solid #ddd' }}
                srcDoc={`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; form-action 'none'; base-uri 'none'">${preview.body}`}
              />
            </section>
          )}
        </div>
        <Button
          theme='solid'
          loading={saving}
          disabled={!loaded || loading}
          onClick={save}
        >
          {t('Save')}
        </Button>
        {status && (
          <dl className='grid grid-cols-2 sm:grid-cols-4 gap-3 border-t py-4'>
            <div>
              <dt>{t('Redis status')}</dt>
              <dd>
                {status.redis_available ? t('Available') : t('Unavailable')}
              </dd>
            </div>
            <div>
              <dt>{t('Queued emails')}</dt>
              <dd>{status.pending}</dd>
            </div>
            <div>
              <dt>{t('Cached API Keys')}</dt>
              <dd>{status.cached_keys}</dd>
            </div>
            <div>
              <dt>{t('Uncertain deliveries')}</dt>
              <dd>
                {status.deliveries?.find((row) => row.status === 'unknown')
                  ?.count ?? 0}
              </dd>
            </div>
          </dl>
        )}
      </section>
    </Spin>
  );
}
