import React, { useEffect, useRef, useState } from 'react';
import { Button, Input, Modal } from '@douyinfe/semi-ui';
import { CreditCard } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';

export default function EpayCheckout({ minimum, discountsEnabled }) {
  const { t } = useTranslation();
  const [amount, setAmount] = useState(String(minimum));
  const [step, setStep] = useState(0);
  const [invoiceType, setInvoiceType] = useState('none');
  const [method, setMethod] = useState('');
  const [discount, setDiscount] = useState('');
  const [channels, setChannels] = useState(null);
  const [methodsFailed, setMethodsFailed] = useState(false);
  const [quote, setQuote] = useState(null);
  const [failed, setFailed] = useState(false);
  const [paying, setPaying] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const attempt = useRef({ token: '', id: '' });
  const submitting = useRef(false);
  const cents = /^\d+(\.\d{1,2})?$/.test(amount)
    ? Math.round(Number(amount) * 100)
    : 0;
  const validAmount = cents > 0 && cents >= minimum * 100;
  useEffect(() => {
    const controller = new AbortController();
    setChannels(null);
    setMethodsFailed(false);
    if (step === 2) {
      API.get('/api/user/epay/checkout/methods', {
        params: { invoice_type: invoiceType },
        signal: controller.signal,
        disableDuplicate: true,
        skipErrorHandler: true,
      })
        .then(({ data }) => {
          if (!data.success) throw new Error(data.message);
          if (!controller.signal.aborted)
            setChannels({ type: invoiceType, methods: data.data });
        })
        .catch(() => {
          if (!controller.signal.aborted) setMethodsFailed(true);
        });
    }
    return () => controller.abort();
  }, [invoiceType, step, refresh]);
  const available = channels?.type === invoiceType ? channels.methods : [];
  const selectedMethod = available.includes(method)
    ? method
    : available[0] || '';
  const methodsReady = step === 2 && !methodsFailed && !!selectedMethod;
  const request = {
    total_cents: cents,
    invoice_type: invoiceType,
    payment_method: selectedMethod,
    discount_code: discount.trim(),
  };
  const key = JSON.stringify(request);
  useEffect(() => {
    const controller = new AbortController();
    setQuote(null);
    setFailed(false);
    const timer = setTimeout(() => {
      if (!methodsReady || !validAmount) return;
      API.post('/api/user/epay/checkout/quote', JSON.parse(key), {
        signal: controller.signal,
        skipErrorHandler: true,
      })
        .then(({ data }) => {
          if (!data.success) throw new Error(data.message);
          if (!controller.signal.aborted)
            setQuote({ ...data.data, requestKey: key });
        })
        .catch(() => {
          if (!controller.signal.aborted) setFailed(true);
        });
    }, 500);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [key, methodsReady, validAmount, refresh]);
  const current = methodsReady && quote?.requestKey === key ? quote : null;
  const pay = async () => {
    if (!current || !validAmount || paying || submitting.current) return;
    submitting.current = true;
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
    } catch {
      showError(t('Payment request failed'));
      setRefresh((value) => value + 1);
    } finally {
      submitting.current = false;
      setPaying(false);
    }
  };
  return (
    <section className='space-y-4 border-b pb-5'>
      <div className='grid gap-4 sm:grid-cols-2'>
        <label className='space-y-2'>
          <span>{t('Payment amount (CNY)')}</span>
          <Input
            id='epay-payment-total'
            type='number'
            min={minimum}
            step='0.01'
            value={amount}
            onChange={setAmount}
            disabled={step !== 0}
          />
        </label>
        {discountsEnabled && (
          <label className='space-y-2'>
            <span>{t('Discount Code')}</span>
            <Input
              id='epay-checkout-discount'
              value={discount}
              onChange={setDiscount}
              disabled={step !== 0}
            />
          </label>
        )}
      </div>
      <Button
        theme='solid'
        type='primary'
        block
        disabled={!validAmount}
        onClick={() => setStep(1)}
      >
        {t('Continue')}
      </Button>
      <Modal
        visible={step !== 0}
        title={step === 1 ? t('Invoice confirmation') : t('Confirm payment')}
        centered
        style={{ width: 'min(520px, calc(100vw - 32px))' }}
        bodyStyle={{ maxHeight: '70dvh', overflowY: 'auto' }}
        getPopupContainer={() => document.body}
        closeOnEsc={!paying}
        maskClosable={!paying}
        closable={!paying}
        onCancel={() => {
          if (!paying) setStep(0);
        }}
        footer={null}
      >
        {step === 1 ? (
          <div className='space-y-4'>
            <p className='whitespace-pre-line leading-relaxed'>
              {t(
                'Choose invoice eligibility before paying. If you need an invoice, select Yes now and choose the invoice type next. Tax charges will be included in your payment and deducted when calculating the credited balance. Selecting No creates a non-invoiceable order without tax charges; this order cannot later be changed to an invoiceable order.',
              )}
            </p>
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
              <Button
                style={{
                  height: 'auto',
                  minHeight: 48,
                  padding: '12px',
                  whiteSpace: 'normal',
                }}
                onClick={() => {
                  setInvoiceType('none');
                  setMethod('');
                  setStep(2);
                }}
              >
                {t('No invoice')}
              </Button>
              <Button
                style={{
                  height: 'auto',
                  minHeight: 48,
                  padding: '12px',
                  whiteSpace: 'normal',
                }}
                theme='solid'
                onClick={() => {
                  setInvoiceType('ordinary');
                  setMethod('');
                  setStep(2);
                }}
              >
                {t('Invoice required')}
              </Button>
            </div>
          </div>
        ) : (
          <div className='space-y-4'>
            <p>
              {t(
                'Invoice eligibility and type are fixed at checkout. Tax is included in the payment amount.',
              )}
            </p>
            {invoiceType !== 'none' && (
              <fieldset
                disabled={paying}
                className='grid grid-cols-[repeat(auto-fit,minmax(min(100%,180px),1fr))] gap-3'
              >
                <legend className='mb-2 font-medium'>
                  {t('Invoice type')}
                </legend>
                {['ordinary', 'special'].map((kind) => (
                  <label
                    key={kind}
                    className='flex min-h-16 min-w-0 cursor-pointer items-center gap-3 rounded-lg border border-[var(--semi-color-border)] p-4 transition-colors hover:bg-[var(--semi-color-fill-0)] has-[:checked]:border-[var(--semi-color-primary)] has-[:checked]:bg-[var(--semi-color-primary-light-default)] has-[:checked]:text-[var(--semi-color-primary)] has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-[var(--semi-color-primary)] has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50'
                  >
                    <input
                      className='h-4 w-4 shrink-0 accent-[var(--semi-color-primary)]'
                      type='radio'
                      name='checkout-invoice-type'
                      value={kind}
                      checked={invoiceType === kind}
                      onChange={() => {
                        setInvoiceType(kind);
                        setMethod('');
                      }}
                    />
                    {kind === 'ordinary'
                      ? t('Ordinary VAT invoice')
                      : t('Special VAT invoice')}
                  </label>
                ))}
              </fieldset>
            )}
            <fieldset
              disabled={paying}
              className='grid grid-cols-[repeat(auto-fit,minmax(min(100%,180px),1fr))] gap-3'
            >
              <legend className='mb-2 font-medium'>
                {t('Payment Method')}
              </legend>
              {!channels && !methodsFailed && <p>{t('Loading...')}</p>}
              {available.map((value) => (
                <label
                  key={value}
                  className='flex min-h-16 min-w-0 cursor-pointer items-center gap-3 rounded-lg border border-[var(--semi-color-border)] p-4 transition-colors hover:bg-[var(--semi-color-fill-0)] has-[:checked]:border-[var(--semi-color-primary)] has-[:checked]:bg-[var(--semi-color-primary-light-default)] has-[:checked]:text-[var(--semi-color-primary)] has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-[var(--semi-color-primary)] has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50'
                >
                  <input
                    className='h-4 w-4 shrink-0 accent-[var(--semi-color-primary)]'
                    type='radio'
                    name='checkout-payment-method'
                    value={value}
                    checked={selectedMethod === value}
                    onChange={() => setMethod(value)}
                  />
                  {value === 'alipay' ? t('Alipay') : t('WeChat Pay')}
                </label>
              ))}
            </fieldset>
            {(methodsFailed || (channels && available.length === 0)) && (
              <p role='alert'>
                {t('No payment methods are available for this invoice choice.')}
              </p>
            )}
            {methodsReady && !current && !failed && <p>{t('Loading...')}</p>}
            {failed && (
              <p role='alert'>
                {t(
                  'Tax quote unavailable. Refresh the selected orders and try again.',
                )}
              </p>
            )}
            {current && (
              <dl className='grid grid-cols-2 gap-2'>
                <dt>{t('Balance credited')}</dt>
                <dd className='text-right'>{current.credit_amount}</dd>
                <dt>{t('Net amount (CNY)')}</dt>
                <dd className='text-right'>
                  {(current.net_cents / 100).toFixed(2)}
                </dd>
                {invoiceType !== 'none' && (
                  <>
                    <dt>{t('Tax charge (CNY)')}</dt>
                    <dd className='text-right'>
                      {(current.tax_cents / 100).toFixed(2)}
                    </dd>
                  </>
                )}
                <dt>{t('Payment amount (CNY)')}</dt>
                <dd className='text-right'>
                  {(current.total_cents / 100).toFixed(2)}
                </dd>
              </dl>
            )}
            <div className='flex flex-wrap justify-end gap-2'>
              <Button disabled={paying} onClick={() => setStep(1)}>
                {t('Back')}
              </Button>
              {(methodsFailed || failed) && (
                <Button onClick={() => setRefresh((value) => value + 1)}>
                  {t('Retry')}
                </Button>
              )}
              <Button
                theme='solid'
                icon={<CreditCard size={16} />}
                disabled={!current || failed || paying}
                loading={paying}
                onClick={pay}
              >
                {t('Pay')}
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </section>
  );
}
