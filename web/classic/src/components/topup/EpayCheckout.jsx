import React, { useEffect, useRef, useState } from 'react';
import { Button, Input, Select } from '@douyinfe/semi-ui';
import { CreditCard } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';

export default function EpayCheckout({ methods, minimum, discountsEnabled }) {
  const { t } = useTranslation();
  const [amount, setAmount] = useState(String(minimum));
  const [invoiceType, setInvoiceType] = useState('none');
  const [method, setMethod] = useState(methods[0]?.type || 'alipay');
  const [discount, setDiscount] = useState('');
  const [discountInput, setDiscountInput] = useState('');
  const [quote, setQuote] = useState(null);
  const [failed, setFailed] = useState(false);
  const [paying, setPaying] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const attempt = useRef({ token: '', id: '' });
  const cents = /^\d+(\.\d{1,2})?$/.test(amount)
    ? Math.round(Number(amount) * 100)
    : 0;
  const request = {
    total_cents: cents,
    invoice_type: invoiceType,
    payment_method: method,
    discount_code: discount.trim(),
  };
  const key = JSON.stringify(request);
  useEffect(() => {
    const controller = new AbortController();
    setQuote(null);
    setFailed(false);
    if (cents > 0 && cents >= minimum * 100) {
      API.post('/api/user/epay/checkout/quote', JSON.parse(key), {
        signal: controller.signal,
      })
        .then(({ data }) => {
          if (!data.success) throw new Error(data.message);
          if (!controller.signal.aborted)
            setQuote({ ...data.data, requestKey: key });
        })
        .catch(() => {
          if (!controller.signal.aborted) setFailed(true);
        });
    }
    return () => controller.abort();
  }, [key, minimum, refresh]);
  const current = quote?.requestKey === key ? quote : null;
  const pay = async () => {
    if (!current || paying || discountInput.trim() !== discount) return;
    setPaying(true);
    if (attempt.current.token !== current.token)
      attempt.current = { token: current.token, id: crypto.randomUUID() };
    try {
      const { data } = await API.post('/api/user/epay/checkout/pay', {
        ...request,
        token: current.token,
        request_id: attempt.current.id,
      });
      if (!data.success) throw new Error(data.message);
      const form = document.createElement('form');
      form.method = 'POST';
      form.action = data.data.url;
      form.target = '_blank';
      form.rel = 'noopener noreferrer';
      for (const [name, value] of Object.entries(data.data.params)) {
        const input = document.createElement('input');
        input.type = 'hidden';
        input.name = name;
        input.value = String(value);
        form.appendChild(input);
      }
      document.body.appendChild(form);
      try {
        form.submit();
      } finally {
        form.remove();
      }
    } catch (error) {
      showError(t('Payment request failed'));
      setRefresh((value) => value + 1);
    } finally {
      setPaying(false);
    }
  };
  return (
    <section className='space-y-4 border-b pb-5'>
      <div className='grid gap-4 sm:grid-cols-2'>
        <label className='space-y-2'>
          <span>{t('Payment amount (CNY)')}</span>
          <Input
            type='number'
            min={minimum}
            step='0.01'
            value={amount}
            onChange={setAmount}
            disabled={paying}
          />
        </label>
        <label className='space-y-2'>
          <span>{t('Invoice type')}</span>
          <Select
            style={{ width: '100%' }}
            value={invoiceType}
            onChange={setInvoiceType}
            disabled={paying}
            optionList={[
              { value: 'none', label: t('No invoice') },
              { value: 'ordinary', label: t('Ordinary VAT invoice') },
              { value: 'special', label: t('Special VAT invoice') },
            ]}
          />
        </label>
        {discountsEnabled && (
          <label className='space-y-2'>
            <span>{t('Discount Code')}</span>
            <Input id='epay-checkout-discount' value={discountInput} onChange={setDiscountInput} onBlur={() => setDiscount(discountInput.trim())} disabled={paying} />
          </label>
        )}
        <label className='space-y-2'>
          <span>{t('Payment Method')}</span>
          <Select
            style={{ width: '100%' }}
            value={method}
            onChange={setMethod}
            disabled={paying}
            optionList={methods.map((item) => ({
              value: item.type,
              label: item.name,
            }))}
          />
        </label>
      </div>
      <p className='text-xs'>
        {t(
          'Invoice eligibility and type are fixed at checkout. Tax is included in the payment amount.',
        )}
      </p>
      {failed ? (
        <p role='alert'>
          {t(
            'Tax quote unavailable. Refresh the selected orders and try again.',
          )}
        </p>
      ) : current ? (
        <dl className='grid grid-cols-2 gap-2'>
          <dt>{t('Balance credited')}</dt>
          <dd className='text-right'>{current.credit_amount}</dd>
          <dt>{t('Net amount (CNY)')}</dt>
          <dd className='text-right'>{(current.net_cents / 100).toFixed(2)}</dd>
          <dt>{t('Tax charge (CNY)')}</dt>
          <dd className='text-right'>{(current.tax_cents / 100).toFixed(2)}</dd>
          <dt>{t('Payment amount (CNY)')}</dt>
          <dd className='text-right'>
            {(current.total_cents / 100).toFixed(2)}
          </dd>
        </dl>
      ) : cents > 0 && cents >= minimum * 100 ? (
        <p>{t('Loading...')}</p>
      ) : null}
      <Button
        htmlType='button'
        icon={<CreditCard size={16} />}
        loading={paying}
        disabled={!current || failed || paying || discountInput.trim() !== discount}
        onClick={pay}
      >
        {t('Pay')}
      </Button>
    </section>
  );
}
