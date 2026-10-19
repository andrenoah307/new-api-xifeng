import React, { useEffect, useState } from 'react';
import { Button, Input } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';

export default function InvoiceEpaySettings() {
  const { t } = useTranslation();
  const [config, setConfig] = useState(null);
  const [secret, setSecret] = useState('');
  const [saving, setSaving] = useState(false);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true;
    API.get('/api/option/invoice-epay')
      .then(({ data }) => {
        if (!data.success) throw new Error(data.message);
        if (active) setConfig(data.data);
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
  const save = async () => {
    setSaving(true);
    try {
      const { data } = await API.put('/api/option/invoice-epay', {
        ...config,
        key: secret,
      });
      if (!data.success) throw new Error(data.message);
      setConfig(data.data);
      setSecret('');
      showSuccess(t('Saved successfully'));
    } catch (error) {
      showError(error.message);
    } finally {
      setSaving(false);
    }
  };
  if (!config) return <p>{failed ? t('加载失败') : t('Loading...')}</p>;
  return (
    <section className='space-y-4 mb-6'>
      <h3>{t('Invoice-eligible recharge gateway')}</h3>
      <div className='grid gap-4 sm:grid-cols-2'>
        <label>
          {t('支付地址')}
          <Input
            value={config.address}
            onChange={(address) => setConfig({ ...config, address })}
          />
        </label>
        <label>
          {t('商户 ID')}
          <Input
            value={config.partner_id}
            onChange={(partner_id) => setConfig({ ...config, partner_id })}
          />
        </label>
        <label>
          {t('密钥')}
          <Input
            mode='password'
            autoComplete='new-password'
            value={secret}
            onChange={setSecret}
          />
        </label>
      </div>
      <Button
        loading={saving}
        disabled={
          !config.address ||
          !config.partner_id ||
          (!config.key_configured && !secret)
        }
        onClick={save}
      >
        {t('Save')}
      </Button>
    </section>
  );
}
