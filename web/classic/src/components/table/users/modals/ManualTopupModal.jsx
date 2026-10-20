import React, { useRef, useState } from 'react';
import { Button, Input, Modal } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../../helpers';

export default function ManualTopupModal({
  user,
  actorId,
  onClose,
  onSuccess,
}) {
  const { t } = useTranslation();
  const storageKey = `manual-topup:${actorId}:${user.id}`;
  const [pending, setPending] = useState(() => {
    try {
      return JSON.parse(localStorage.getItem(storageKey) || 'null');
    } catch {
      return null;
    }
  });
  const requestID = useRef(pending?.request_id || crypto.randomUUID());
  const [amount, setAmount] = useState(
    pending ? (pending.total_cents / 100).toFixed(2) : '',
  );
  const [invoiceType, setInvoiceType] = useState(
    pending?.invoice_type || 'none',
  );
  const [quote, setQuote] = useState(pending?.quote || null);
  const [busy, setBusy] = useState(false);
  const running = useRef(false);
  const cents = Math.round(Number(amount) * 100);
  const valid =
    /^\d+(\.\d{1,2})?$/.test(String(amount)) &&
    Number.isSafeInteger(cents) &&
    cents > 0;

  const submit = async () => {
    if (!valid || running.current) return;
    running.current = true;
    setBusy(true);
    try {
      if (!quote) {
        const { data } = await API.post('/api/user/topup/manual/quote', {
          user_id: user.id,
          total_cents: cents,
          invoice_type: invoiceType,
        });
        if (data.success) setQuote(data.data);
        else showError(data.message || t('Request failed'));
        return;
      }
      const draft = pending || {
        user_id: user.id,
        total_cents: cents,
        invoice_type: invoiceType,
        request_id: requestID.current,
        token: quote.token,
        quote,
      };
      localStorage.setItem(storageKey, JSON.stringify(draft));
      setPending(draft);
      const { data } = await API.post('/api/user/topup/manual', draft);
      if (!data.success) {
        setPending(null);
        setQuote(null);
        showError(data.message || t('Request failed'));
        return;
      }
      localStorage.removeItem(storageKey);
      showSuccess(t('Operation successful'));
      onSuccess();
      onClose();
    } catch {
      showError(t('Request failed'));
    } finally {
      running.current = false;
      setBusy(false);
    }
  };
  return (
    <Modal
      visible
      title={t('Manual top-up')}
      centered
      style={{ width: 'min(520px, calc(100vw - 32px))' }}
      bodyStyle={{ maxHeight: '70dvh', overflowY: 'auto' }}
      getPopupContainer={() => document.body}
      closable={!busy}
      maskClosable={!busy}
      closeOnEsc={!busy}
      onCancel={onClose}
      footer={null}
    >
      <div className='space-y-4'>
        <p>
          {user.username} (ID: {user.id})
        </p>
        <p>
          {t(
            'Record a payment already received. Tax is deducted from the payment amount; the remainder is credited without promotions.',
          )}
        </p>
        <label className='block space-y-2'>
          <span>{t('Payment amount (CNY)')}</span>
          <Input
            id='manual-topup-amount'
            type='number'
            min='0.01'
            step='0.01'
            value={amount}
            disabled={!!quote || busy}
            onChange={setAmount}
          />
        </label>
        <fieldset
          disabled={!!quote || busy}
          className='grid grid-cols-[repeat(auto-fit,minmax(min(100%,180px),1fr))] gap-3'
        >
          <legend className='mb-2 font-medium'>{t('Invoice type')}</legend>
          {['none', 'ordinary', 'special'].map((kind) => (
            <label
              key={kind}
              className='flex min-h-16 min-w-0 cursor-pointer items-center gap-3 rounded-lg border border-[var(--semi-color-border)] p-4 transition-colors hover:bg-[var(--semi-color-fill-0)] has-[:checked]:border-[var(--semi-color-primary)] has-[:checked]:bg-[var(--semi-color-primary-light-default)] has-[:checked]:text-[var(--semi-color-primary)] has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-[var(--semi-color-primary)] has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50'
            >
              <input
                className='h-4 w-4 shrink-0 accent-[var(--semi-color-primary)]'
                type='radio'
                name='manual-invoice-type'
                value={kind}
                checked={invoiceType === kind}
                onChange={() => setInvoiceType(kind)}
              />
              {kind === 'none'
                ? '-'
                : kind === 'ordinary'
                  ? t('Ordinary VAT invoice')
                  : t('Special VAT invoice')}
            </label>
          ))}
        </fieldset>
        {quote && (
          <dl className='grid grid-cols-2 gap-2'>
            <dt>{t('Payment amount (CNY)')}</dt>
            <dd className='text-right'>
              {(quote.total_cents / 100).toFixed(2)}
            </dd>
            <dt>{t('Tax charge (CNY)')}</dt>
            <dd className='text-right'>{(quote.tax_cents / 100).toFixed(2)}</dd>
            <dt>{t('Balance credited')}</dt>
            <dd className='text-right font-medium'>{quote.credit_amount}</dd>
          </dl>
        )}
        <div className='flex justify-end gap-2'>
          {quote && !pending && (
            <Button disabled={busy} onClick={() => setQuote(null)}>
              {t('Back')}
            </Button>
          )}
          <Button
            theme='solid'
            disabled={busy || !valid}
            loading={busy}
            onClick={submit}
          >
            {pending ? t('Retry') : quote ? t('Confirm') : t('Continue')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
