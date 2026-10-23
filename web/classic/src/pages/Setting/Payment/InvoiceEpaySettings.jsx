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
import React, { useEffect, useState } from 'react';
import { Button, Input } from '@douyinfe/semi-ui';
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';

function routingDraft(value) {
  return {
    gateways:
      value.gateways ??
      (value.address
        ? [
            {
              id: 'legacy',
              name: 'Epay',
              account_name: '',
              address: value.address,
              partner_id: value.partner_id || '',
              key_configured: !!value.key_configured,
              payment_methods: value.payment_methods || [],
            },
          ]
        : []),
    strategy: value.strategy || 'round_robin',
    private_gateway_name: value.private_gateway_name || 'Epay',
    private_below_cents:
      value.gateways === null ? 50000 : value.private_below_cents,
  };
}
export default function InvoiceEpaySettings() {
  const { t } = useTranslation();
  const [config, setConfig] = useState(null);
  const [threshold, setThreshold] = useState(null);
  const [saving, setSaving] = useState(false);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true;
    API.get('/api/option/invoice-epay')
      .then(({ data }) => {
        if (!data.success) throw new Error(data.message);
        if (active) setConfig(routingDraft(data.data));
      })
      .catch((error) => {
        if (active) {
          setFailed(true);
          showError(error.message);
        }
      });
    return () => {
      active = false;
    };
  }, []);
  const thresholdText =
    threshold ?? String((config?.private_below_cents ?? 50000) / 100);
  const thresholdCents = /^\d+(\.\d{1,2})?$/.test(thresholdText)
    ? Math.round(Number(thresholdText) * 100)
    : Number.NaN;
  const save = async () => {
    setSaving(true);
    try {
      const { data } = await API.put('/api/option/invoice-epay', {
        ...config,
        private_below_cents: thresholdCents,
      });
      if (!data.success) throw new Error(data.message);
      setConfig(routingDraft(data.data));
      setThreshold(null);
      showSuccess(t('Saved successfully'));
    } catch (error) {
      showError(error.message);
    } finally {
      setSaving(false);
    }
  };
  if (!config) return <p>{failed ? t('加载失败') : t('Loading...')}</p>;
  const invalid =
    !config.private_gateway_name.trim() ||
    !Number.isSafeInteger(thresholdCents) ||
    thresholdCents < 0 ||
    thresholdCents > 100000000000 ||
    config.gateways.some(
      (gateway) =>
        !gateway.name.trim() ||
        !gateway.account_name.trim() ||
        !gateway.address.trim() ||
        !gateway.partner_id.trim() ||
        (!gateway.key_configured && !gateway.key?.trim()),
    );
  const updateGateway = (id, patch) =>
    setConfig({
      ...config,
      gateways: config.gateways.map((gateway) =>
        gateway.id === id ? { ...gateway, ...patch } : gateway,
      ),
    });
  const moveGateway = (index, offset) => {
    const gateways = [...config.gateways];
    [gateways[index], gateways[index + offset]] = [
      gateways[index + offset],
      gateways[index],
    ];
    setConfig({ ...config, gateways });
  };
  return (
    <section className='space-y-4 mb-6'>
      <h3>{t('Invoice payment routing')}</h3>
      <fieldset disabled={saving} className='min-w-0 space-y-4'>
        <label className='block space-y-2'>
          <span>{t('Private payment gateway name')}</span>
          <Input
            maxLength={200}
            value={config.private_gateway_name}
            onChange={(private_gateway_name) =>
              setConfig({ ...config, private_gateway_name })
            }
          />
        </label>
        <div className='grid gap-4 sm:grid-cols-2'>
          <label className='space-y-2'>
            <span>{t('Corporate gateway threshold (CNY)')}</span>
            <input
              className='w-full border rounded-md p-2 bg-transparent'
              type='number'
              min='0'
              max='1000000000'
              step='0.01'
              value={thresholdText}
              onChange={(event) => setThreshold(event.target.value)}
            />
          </label>
          <label className='space-y-2'>
            <span>{t('Routing strategy')}</span>
            <select
              className='w-full border rounded-md p-2 bg-transparent'
              value={config.strategy}
              onChange={(event) =>
                setConfig({ ...config, strategy: event.target.value })
              }
            >
              <option value='round_robin'>{t('Round robin')}</option>
              <option value='random'>{t('Random')}</option>
            </select>
          </label>
        </div>
        <p className='text-sm text-gray-500'>
          {t(
            'Invoice payments below the threshold use the private gateway; tax still applies. Payments at or above the threshold use corporate gateways.',
          )}
        </p>
        {config.gateways.map((gateway, index) => (
          <div key={gateway.id} className='space-y-4 rounded-md border p-4'>
            <div className='flex items-center justify-between gap-2'>
              <span className='min-w-0 break-words'>
                {t('Corporate gateway')} {index + 1}
              </span>
              <div className='flex shrink-0 gap-1'>
                <Button
                  title={t('Move up')}
                  aria-label={t('Move up')}
                  disabled={saving || index === 0}
                  icon={<ArrowUp size={16} />}
                  onClick={() => moveGateway(index, -1)}
                />
                <Button
                  title={t('Move down')}
                  aria-label={t('Move down')}
                  disabled={saving || index === config.gateways.length - 1}
                  icon={<ArrowDown size={16} />}
                  onClick={() => moveGateway(index, 1)}
                />
                <Button
                  title={t('Delete')}
                  aria-label={t('Delete')}
                  disabled={saving}
                  icon={<Trash2 size={16} />}
                  onClick={() =>
                    setConfig({
                      ...config,
                      gateways: config.gateways.filter(
                        (item) => item.id !== gateway.id,
                      ),
                    })
                  }
                />
              </div>
            </div>
            <div className='grid gap-4 sm:grid-cols-2'>
              <label className='space-y-2'>
                <span>{t('Payment gateway name')}</span>
                <Input
                  maxLength={200}
                  value={gateway.name}
                  onChange={(name) => updateGateway(gateway.id, { name })}
                />
              </label>
              <label className='space-y-2'>
                <span>{t('Corporate account name')}</span>
                <Input
                  maxLength={200}
                  value={gateway.account_name}
                  onChange={(account_name) =>
                    updateGateway(gateway.id, { account_name })
                  }
                />
              </label>
              <label className='space-y-2'>
                <span>{t('支付地址')}</span>
                <Input
                  value={gateway.address}
                  onChange={(address) => updateGateway(gateway.id, { address })}
                />
              </label>
              <label className='space-y-2'>
                <span>{t('易支付商户ID')}</span>
                <Input
                  value={gateway.partner_id}
                  onChange={(partner_id) =>
                    updateGateway(gateway.id, { partner_id })
                  }
                />
              </label>
              <label className='space-y-2'>
                <span>{t('密钥')}</span>
                <Input
                  mode='password'
                  autoComplete='new-password'
                  value={gateway.key || ''}
                  placeholder={
                    gateway.key_configured ? t('Enter new key to update') : ''
                  }
                  onChange={(key) => updateGateway(gateway.id, { key })}
                />
              </label>
            </div>
            <div className='flex flex-wrap gap-4'>
              {['alipay', 'wxpay'].map((method) => (
                <label key={method} className='flex items-center gap-2'>
                  <input
                    type='checkbox'
                    checked={gateway.payment_methods.includes(method)}
                    onChange={(event) =>
                      updateGateway(gateway.id, {
                        payment_methods: event.target.checked
                          ? [...gateway.payment_methods, method]
                          : gateway.payment_methods.filter(
                              (value) => value !== method,
                            ),
                      })
                    }
                  />
                  {method === 'alipay' ? t('Alipay') : t('WeChat Pay')}
                </label>
              ))}
            </div>
          </div>
        ))}
        <div className='flex flex-wrap gap-2'>
          <Button
            disabled={saving || config.gateways.length >= 20}
            icon={<Plus size={16} />}
            onClick={() =>
              setConfig({
                ...config,
                gateways: [
                  ...config.gateways,
                  {
                    id: crypto.randomUUID(),
                    name: '',
                    account_name: '',
                    address: '',
                    partner_id: '',
                    key_configured: false,
                    key: '',
                    payment_methods: ['alipay', 'wxpay'],
                  },
                ],
              })
            }
          >
            {t('Add corporate gateway')}
          </Button>
          <Button loading={saving} disabled={invalid || saving} onClick={save}>
            {t('Save')}
          </Button>
        </div>
      </fieldset>
    </section>
  );
}
