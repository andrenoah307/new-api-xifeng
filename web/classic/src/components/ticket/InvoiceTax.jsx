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
import React, { useEffect, useState, useContext } from 'react';
import { StatusContext } from '../../context/Status';
import { useTranslation } from 'react-i18next';
import { RefreshCw } from 'lucide-react';
import {
  Banner,
  Button,
  InputNumber,
  Select,
  Space,
  Switch,
  Table,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../helpers';
import { InvoiceAmountFormula } from './InvoiceAmountFormula';

const modes = [
  { value: 'vat', label: 'VAT added to net amount', formula: 'B × (1 + r)' },
  {
    value: 'compound',
    label: 'Markup followed by VAT',
    formula: 'B × (1 + r)²',
  },
  {
    value: 'gross_up',
    label: 'Gross-up by total invoice amount',
    formula: 'B ÷ (1 − r)',
  },
];

function useInvoiceTaxConfig() {
  const [config, setConfig] = useState(null);
  useEffect(() => {
    let active = true;
    API.get('/api/ticket/invoice/tax-config')
      .then(({ data }) => {
        if (!data.success) throw new Error(data.message);
        if (active) setConfig(data.data);
      })
      .catch((error) => {
        if (active) showError(error.message);
      });
    return () => {
      active = false;
    };
  }, []);
  return [config, setConfig];
}

export function InvoiceTaxNotice() {
  const { t } = useTranslation();
  const [config] = useInvoiceTaxConfig();
  return config?.module_enabled && config?.enabled ? (
    <Banner
      type='warning'
      description={t(
        'Invoice eligibility and type are fixed at checkout. Tax is included in the payment amount.',
      )}
    />
  ) : null;
}

export function InvoiceTaxFields({
  ids,
  open,
  onChange,
  type,
  onTypeChange,
  legacyTotalCents,
}) {
  const { t } = useTranslation();
  const [config] = useInvoiceTaxConfig();
  const [payment, setPayment] = useState('balance');
  const [quote, setQuote] = useState(null);
  const [failed, setFailed] = useState(false);
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    if (!open) return;
    const refreshQuote = () => setRefresh((value) => value + 1);
    window.addEventListener('focus', refreshQuote);
    return () => window.removeEventListener('focus', refreshQuote);
  }, [open]);
  const key = JSON.stringify(ids);
  useEffect(() => {
    let active = true;
    setQuote(null);
    setFailed(false);
    const selected = JSON.parse(key);
    if (
      open &&
      config &&
      (config.module_enabled || config.invoice_workflow_required) &&
      selected.length
    ) {
      API.post('/api/ticket/invoice/tax-quote', {
        topup_order_ids: selected,
        invoice_type: type,
      })
        .then(({ data }) => {
          if (!data.success) throw new Error(data.message);
          if (active)
            setQuote({ ...data.data, requestKey: key, requestType: type });
        })
        .catch(() => {
          if (active) setFailed(true);
        });
    }
    return () => {
      active = false;
    };
  }, [key, type, config, open, refresh]);
  const current =
    quote?.requestKey === key && quote?.requestType === type ? quote : null;
  const method = current?.can_balance ? payment : 'online';
  useEffect(() => {
    onChange({
      ready:
        !!config &&
        ((!config.module_enabled && !config.invoice_workflow_required) ||
          !!current),
      invoice_type: type,
      tax_payment_method: method,
      tax_quote: current?.token || '',
    });
  }, [config, current, type, method, onChange]);
  if (!config) return <p>{t('Loading...')}</p>;
  if (!config.module_enabled && !config.invoice_workflow_required)
    return ids.length > 0 ? (
      <InvoiceAmountFormula
        net={legacyTotalCents}
        tax={0}
        total={legacyTotalCents}
      />
    ) : null;
  return (
    <div className='space-y-3 my-4'>
      <InvoiceTaxNotice />
      <div className='flex items-center gap-2'>
        <Select
          className='flex-1 min-w-0'
          aria-label={t('Invoice type')}
          value={type}
          onChange={onTypeChange}
          optionList={[
            { value: 'ordinary', label: t('Ordinary VAT invoice') },
            { value: 'special', label: t('Special VAT invoice') },
          ]}
        />
        <Button
          type='tertiary'
          theme='borderless'
          icon={<RefreshCw size={16} />}
          aria-label={t('刷新')}
          title={t('刷新')}
          disabled={!ids.length}
          onClick={() => setRefresh((value) => value + 1)}
        />
      </div>
      {failed && (
        <Banner
          type='danger'
          description={t(
            'Tax quote unavailable. Refresh the selected orders and try again.',
          )}
        />
      )}
      {current && (
        <>
          <dl className='grid grid-cols-2 gap-2'>
            {!current.prepaid_only && (
              <>
                <dt>{t('Tax calculation mode')}</dt>
                <dd>
                  {t(modes.find((mode) => mode.value === current.mode)?.label)}
                </dd>
                <dt>{t('Invoice fee rate (%)')}</dt>
                <dd>{current.rate_bps / 100}%</dd>
              </>
            )}
          </dl>
          <InvoiceAmountFormula
            net={current.base_cents + current.prepaid_net_cents}
            tax={current.prepaid_tax_cents}
            additional={current.fee_cents}
            total={current.total_cents}
            totalLabel={
              current.prepaid_only
                ? 'invoice.amount.paid'
                : 'invoice.amount.total'
            }
          />
          {current.fee_cents > 0 && (
            <>
              <Select
                className='w-full'
                aria-label={t('Payment Method')}
                value={method}
                onChange={setPayment}
                optionList={[
                  {
                    value: 'balance',
                    label: t('Pay from balance'),
                    disabled: !current.can_balance,
                  },
                  { value: 'online', label: t('Pay full tax charge online') },
                ]}
              />
              {!current.can_balance && (
                <p>
                  {t(
                    'Balance payment is unavailable or insufficient. Pay the full tax charge online; split payments are not supported.',
                  )}
                </p>
              )}
              <p>
                {t(
                  'Tax payments rejected with the invoice are refunded to your account balance. CNY 1 equals one balance unit.',
                )}
              </p>
            </>
          )}
        </>
      )}
    </div>
  );
}

export function InvoiceTaxPayment({ charge, rejected }) {
  const { t } = useTranslation();
  const user = JSON.parse(localStorage.getItem('user') || 'null');
  const owner = user?.id === charge.user_id;
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    if (!owner) return;
    let active = true;
    const load = () =>
      API.get(`/api/ticket/invoice/${charge.ticket_id}/tax`)
        .then(({ data }) => {
          if (active && data.success) setData(data.data);
        })
        .catch(() => {});
    load();
    const timer = setInterval(load, 10000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [charge.ticket_id, owner]);
  const current = data?.charge || charge;
  const pay = async (method) => {
    setLoading(true);
    try {
      const { data } = await API.post(
        `/api/ticket/invoice/${charge.ticket_id}/tax/pay`,
        { payment_method: method },
      );
      if (!data.success) throw new Error(data.message);
      const form = document.createElement('form');
      form.method = 'POST';
      form.action = data.data.url;
      Object.entries(data.data.params).forEach(([key, value]) => {
        const input = document.createElement('input');
        input.type = 'hidden';
        input.name = key;
        input.value = String(value);
        form.appendChild(input);
      });
      document.body.appendChild(form);
      form.submit();
      form.remove();
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };
  const labels = {
    pending: t('Tax payment pending'),
    paid: t('Tax paid'),
    refunded: t('Tax refunded to balance'),
  };
  let equationKey = 'invoice.amount.total';
  if (current.status === 'paid') equationKey = 'invoice.amount.paid';
  if (current.status === 'pending') equationKey = 'invoice.amount.due';
  return (
    <div className='space-y-3 my-4'>
      <dl className='grid grid-cols-2 gap-2'>
        <dt>{t('Invoice type')}</dt>
        <dd>
          {t(
            current.invoice_type === 'special'
              ? 'Special VAT invoice'
              : 'Ordinary VAT invoice',
          )}
        </dd>
        <dt>{t('Tax calculation mode')}</dt>
        <dd>
          {t(
            modes.find((mode) => mode.value === current.mode)?.label ??
              current.mode,
          )}
        </dd>
        <dt>{t('Invoice fee rate (%)')}</dt>
        <dd>{current.rate_bps / 100}%</dd>
        <dt>{t('Status')}</dt>
        <dd>{labels[current.status]}</dd>
      </dl>
      <InvoiceAmountFormula
        net={current.base_cents}
        tax={current.fee_cents}
        total={current.total_cents}
        totalLabel={equationKey}
      />
      {owner && current.status === 'pending' && !rejected && (
        <Space wrap>
          {(data?.payment_methods || [])
            .filter(
              (method) =>
                !current.payment_method || current.payment_method === method,
            )
            .map((method) => (
              <Button
                key={method}
                loading={loading}
                onClick={() => pay(method)}
              >
                {t('Pay full tax charge online')} ·{' '}
                {t(method === 'alipay' ? 'Alipay' : 'WeChat Pay')}
              </Button>
            ))}
          {data && !data.payment_methods.length && (
            <p>
              {t(
                'Online tax payment is currently unavailable. Contact the administrator.',
              )}
            </p>
          )}
        </Space>
      )}
    </div>
  );
}

export function InvoiceTaxSettings() {
  const [statusState, statusDispatch] = useContext(StatusContext);
  const { t } = useTranslation();
  const [config, setConfig] = useInvoiceTaxConfig();
  const [saving, setSaving] = useState(false);
  if (!config) return <p>{t('Loading...')}</p>;
  const legacyBefore = config.legacy_invoice_before ?? 0;
  const validCutoff =
    Number.isSafeInteger(legacyBefore) &&
    legacyBefore >= 0 &&
    legacyBefore <= Date.parse('9999-12-31T00:00:00+08:00') / 1000 &&
    (legacyBefore === 0 || (legacyBefore + 28800) % 86400 === 0);
  const valid =
    validCutoff &&
    [config.ordinary_rate_bps, config.special_rate_bps].every(
      (rate) => Number.isInteger(rate) && rate >= 0 && rate < 10000,
    );
  const rows = modes.flatMap((mode) =>
    ['ordinary', 'special'].map((type) => {
      const rate = config[`${type}_rate_bps`] / 10000;
      const cents = Math.round(
        mode.value === 'gross_up'
          ? 1000000 / (1 - rate)
          : 1000000 * (1 + rate) ** (mode.value === 'compound' ? 2 : 1),
      );
      return {
        id: `${type}-${mode.value}`,
        mode: `${t(mode.label)}: ${mode.formula}`,
        type: t(
          type === 'ordinary' ? 'Ordinary VAT invoice' : 'Special VAT invoice',
        ),
        fee: valid ? ((cents - 1000000) / 100).toFixed(2) : '-',
        total: valid ? (cents / 100).toFixed(2) : '-',
      };
    }),
  );
  const save = async () => {
    setSaving(true);
    try {
      const { data } = await API.put('/api/option/invoice-tax', config);
      if (!data.success) throw new Error(data.message);
      setConfig(data.data);
      statusDispatch({
        type: 'set',
        payload: {
          ...statusState.status,
          finance_module_enabled: config.module_enabled,
        },
      });
      showSuccess(t('Saved successfully'));
    } catch (error) {
      showError(error.message);
    } finally {
      setSaving(false);
    }
  };
  return (
    <section className='space-y-4'>
      <h3>{t('Invoice tax charges')}</h3>
      <Space>
        <Switch
          checked={!!config.module_enabled}
          onChange={(module_enabled) =>
            setConfig({ ...config, module_enabled })
          }
          aria-label={t('Enable finance module')}
        />
        <span>{t('Enable finance module')}</span>
      </Space>
      <Space>
        <Switch
          checked={config.enabled}
          onChange={(enabled) => setConfig({ ...config, enabled })}
          aria-label={t('Collect invoice tax charges')}
        />
        <span>{t('Collect invoice tax charges')}</span>
      </Space>
      <Space>
        <Switch
          checked={!!config.legacy_free_ordinary_enabled}
          onChange={(legacy_free_ordinary_enabled) =>
            setConfig({ ...config, legacy_free_ordinary_enabled })
          }
          aria-label={t('Free ordinary invoices for historical payments')}
        />
        <span>{t('Free ordinary invoices for historical payments')}</span>
      </Space>
      <div className='space-y-2'>
        <label htmlFor='invoice-legacy-before'>
          {t('Allow orders completed before this date to request invoices')}
        </label>
        <input
          id='invoice-legacy-before'
          type='date'
          min='1970-01-02'
          max='9999-12-31'
          className='block w-full rounded border border-[var(--semi-color-border)] bg-transparent p-2'
          value={
            validCutoff && legacyBefore > 0
              ? new Date((legacyBefore + 28800) * 1000)
                  .toISOString()
                  .slice(0, 10)
              : ''
          }
          onChange={(event) =>
            setConfig({
              ...config,
              legacy_invoice_before: event.target.value
                ? Date.parse(`${event.target.value}T00:00:00+08:00`) / 1000
                : 0,
            })
          }
        />
        <p className='text-sm text-[var(--semi-color-text-2)]'>
          {t(
            'The cutoff is 00:00 Beijing time, excluding that date. Leave blank to disable historical eligibility. When free ordinary invoices are enabled, qualifying payments incur no additional invoice fee and cannot use this rule for special invoices; otherwise existing eligibility and fee rules apply.',
          )}
        </p>
      </div>
      {['ordinary', 'special'].map((type) => (
        <div key={type}>
          <label>
            {t(
              type === 'ordinary'
                ? 'Ordinary invoice fee (%)'
                : 'Special invoice fee (%)',
            )}
          </label>
          <InputNumber
            min={0}
            max={99.99}
            step={0.01}
            value={config[`${type}_rate_bps`] / 100}
            onChange={(value) =>
              setConfig({
                ...config,
                [`${type}_rate_bps`]: Math.round(Number(value) * 100),
              })
            }
          />
        </div>
      ))}
      <Select
        className='w-full'
        aria-label={t('Tax calculation mode')}
        value={config.mode}
        onChange={(mode) => setConfig({ ...config, mode })}
        optionList={modes.map((mode) => ({
          value: mode.value,
          label: t(mode.label),
        }))}
      />
      <p>
        {t(
          'Example: net amount CNY 10,000. Rates below use the current draft settings.',
        )}
      </p>
      <Table
        rowKey='id'
        pagination={false}
        scroll={{ x: 700 }}
        dataSource={rows}
        columns={[
          { title: t('Tax calculation mode'), dataIndex: 'mode' },
          { title: t('Invoice type'), dataIndex: 'type' },
          { title: t('Additional payment (CNY)'), dataIndex: 'fee' },
          { title: t('Invoice total (CNY)'), dataIndex: 'total' },
        ]}
      />
      <Button disabled={!valid} loading={saving} onClick={save}>
        {t('Save')}
      </Button>
    </section>
  );
}
