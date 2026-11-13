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
import React, { useRef, useState } from 'react';
import { Button, Modal } from '@douyinfe/semi-ui';
import { RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';

export default function InvoiceTypeDialog({ order, onClose, onApplied }) {
  const { t } = useTranslation();
  const [invoiceType, setInvoiceType] = useState(order.invoice_type || 'none');
  const [plan, setPlan] = useState(null);
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const submitting = useRef(false);
  async function submit(apply) {
    if (
      submitting.current ||
      (apply &&
        (!plan?.can_apply ||
          (plan.requires_negative_confirmation && !confirmed)))
    )
      return;
    submitting.current = true;
    setBusy(true);
    setError('');
    try {
      const { data } = await API.request({
        method: apply ? 'PUT' : 'POST',
        url: apply
          ? '/api/user/topup/invoice-type'
          : '/api/user/topup/invoice-type/preview',
        data: {
          id: order.id,
          invoice_type: invoiceType,
          expected_invoice_type: order.invoice_type || '',
          revision: apply ? plan.revision : undefined,
          confirm_negative: confirmed,
        },
      });
      if (!data.success) throw new Error(data.message);
      if (apply) onApplied();
      else {
        setPlan(data.data);
        setConfirmed(false);
      }
    } catch (err) {
      setError(err.message || 'Operation failed');
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }
  return (
    <Modal
      visible
      centered
      getPopupContainer={() => document.body}
      title={t('Invoice type')}
      width='min(520px, calc(100vw - 32px))'
      bodyStyle={{ maxHeight: '65dvh', overflowY: 'auto' }}
      okText={t('Save')}
      cancelText={t('Cancel')}
      confirmLoading={busy}
      maskClosable={!busy}
      closeOnEsc={!busy}
      closable={!busy}
      okButtonProps={{
        disabled:
          !plan?.can_apply ||
          (plan.requires_negative_confirmation && !confirmed),
      }}
      onCancel={() => {
        if (!submitting.current) onClose();
      }}
      onOk={() => submit(true)}
    >
      <p className='mb-4 break-all'>{order.trade_no}</p>
      <fieldset disabled={busy} className='space-y-2'>
        {['ordinary', 'special', 'none'].map((type) => (
          <label key={type} className='flex items-center gap-2'>
            <input
              type='radio'
              name='topup-invoice-type'
              value={type}
              checked={invoiceType === type}
              onChange={() => {
                setInvoiceType(type);
                setPlan(null);
                setConfirmed(false);
                setError('');
              }}
            />
            {type === 'ordinary' && t('Ordinary VAT invoice')}
            {type === 'special' && t('Special VAT invoice')}
            {type === 'none' && '-'}
          </label>
        ))}
      </fieldset>
      <Button
        className='my-3'
        icon={<RefreshCw size={16} />}
        disabled={busy || invoiceType === order.invoice_type}
        onClick={() => submit(false)}
      >
        {t('Preview fee adjustment')}
      </Button>
      {plan && (
        <div className='space-y-2 break-words'>
          <p>
            {t('Paid amount')}: CNY {(plan.paid_cents / 100).toFixed(2)}
          </p>
          <p>
            {t('Invoice fee: {{before}} → {{after}} CNY', {
              before: (plan.tax_before_cents / 100).toFixed(2),
              after: (plan.tax_after_cents / 100).toFixed(2),
            })}
          </p>
          <p>
            {t(
              'Wallet adjustment: {{change}}; balance {{before}} → {{after}}; allowance {{limit}}',
              {
                change: plan.balance_change,
                before: plan.balance_before,
                after: plan.balance_after,
                limit: plan.grace_amount,
              },
            )}
          </p>
          {!plan.can_apply && (
            <p role='alert'>
              {t(
                'Invoice fee deduction exceeds the configured overdraft allowance',
              )}
            </p>
          )}
          {plan.requires_negative_confirmation && (
            <label className='flex items-start gap-2 text-red-600'>
              <input
                className='mt-1 shrink-0'
                type='checkbox'
                checked={confirmed}
                disabled={busy}
                onChange={(event) => setConfirmed(event.target.checked)}
              />
              <span>
                {t(
                  'I confirm this deduction will leave a negative balance of {{balance}}.',
                  { balance: plan.balance_after },
                )}
              </span>
            </label>
          )}
        </div>
      )}
      {error && (
        <p role='alert' className='mt-2 break-words text-red-600'>
          {t(error)}
        </p>
      )}
    </Modal>
  );
}
