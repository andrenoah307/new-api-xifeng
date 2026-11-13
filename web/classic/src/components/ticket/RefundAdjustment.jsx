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
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Download, RefreshCw, X } from 'lucide-react';

import { Button } from '@douyinfe/semi-ui';
import { API as api } from '../../helpers/api';
import { refundDiagnosticLines } from '../../../../shared/refund-diagnostics.mjs';
import { RefundCalculationDetails } from './RefundCalculationDetails';

export function RefundAdjustment({
  ticketId,
  disabled,
  onOpenChange,
  onApplied,
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [plan, setPlan] = useState(null);
  const [error, setError] = useState('');
  const [method, setMethod] = useState('application_balance');
  const [negativeConfirmed, setNegativeConfirmed] = useState(false);
  const submitting = useRef(false);
  async function preview(selectedMethod = method) {
    if (submitting.current) return;
    submitting.current = true;
    setOpen(true);
    onOpenChange(true);
    setBusy(true);
    setError('');
    setPlan(null);
    setMethod(selectedMethod);
    setNegativeConfirmed(false);
    try {
      const { data } = await api.get(
        `/api/ticket/refund/${ticketId}/adjustment?basis=${selectedMethod === 'application_balance' ? 'application_balance' : ''}`,
        { skipBusinessError: true },
      );
      if (!data.success) throw new Error(data.message);
      setPlan(data.data);
    } catch (e) {
      setError(e instanceof Error ? t(e.message) : t('Operation failed'));
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }
  async function apply() {
    if (!plan?.can_apply || submitting.current) return;
    submitting.current = true;
    setBusy(true);
    setError('');
    try {
      const { data } = await api.post(
        `/api/ticket/refund/${ticketId}/adjustment`,
        {
          revision: plan.revision,
          calculation_method: method,
          confirm_negative: negativeConfirmed,
        },
        { skipBusinessError: true },
      );
      if (!data.success) throw new Error(data.message);
      setPlan(data.data);
      if (data.data.calculation) onApplied(data.data.calculation);
    } catch (e) {
      setError(e instanceof Error ? t(e.message) : t('Operation failed'));
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }
  function download() {
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(plan, null, 2)], { type: 'application/json' }),
    );
    const link = document.createElement('a');
    link.href = url;
    link.download = `refund-${ticketId}-adjustment.json`;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  if (!open) {
    return (
      <Button
        style={{ height: 'auto', whiteSpace: 'normal', maxWidth: '100%' }}
        disabled={disabled}
        onClick={() => void preview()}
      >
        {t('Refund calculation method')}
      </Button>
    );
  }
  const needsSettlement = (plan?.liabilities.length ?? 0) > 0;
  const applicationBasis = plan?.calculation_basis === 'application_balance';
  const needsNegative =
    plan?.requires_negative_confirmation ??
    Number(
      applicationBasis
        ? (plan?.application_balance?.cash_amount ?? 0)
        : (plan?.balance_after ?? 0),
    ) < 0;
  return (
    <section
      className='min-w-0 space-y-3 break-words border-t py-4 text-sm'
      aria-label={t('Refund calculation method')}
    >
      <h4>{t('Refund calculation method')}</h4>
      {!plan?.applied && (
        <label className='block space-y-2'>
          <span>{t('Refund calculation method')}</span>
          <select
            className='block w-full min-w-0 rounded-md border bg-transparent p-2'
            value={method}
            disabled={busy}
            onChange={(event) => void preview(event.target.value)}
          >
            <option value='application_balance'>
              {t('Application balance')}
            </option>
            <option value='discount_recovery'>
              {t('Recover historical discounts')}
            </option>
          </select>
        </label>
      )}
      {error && <p role='alert'>{error}</p>}
      {plan && (
        <>
          <dl className='grid grid-cols-1 gap-2 sm:grid-cols-2'>
            {[
              [t('Requested refund'), plan.requested],
              [t('Balance before adjustment'), plan.balance_before],
              ...(applicationBasis
                ? [
                    [
                      t('Historical noncash credits (not deducted)'),
                      plan.application_balance?.noncash_amount ?? '-',
                    ],
                    [
                      t('Accounting balance excluding noncash credits'),
                      plan.application_balance?.cash_amount ?? '-',
                    ],
                  ]
                : [
                    [
                      t('Historical promotional credit recovery'),
                      plan.debit_amount,
                    ],
                  ]),
              [
                t('Accounted credits (balance, usage and paid fees)'),
                plan.accounted_amount,
              ],
              [t('Known credits after refunds and holds'), plan.known_amount],
              [t('Balance after adjustment'), plan.balance_after],
              [t('Configured refund grace'), plan.grace_amount],
              ...(applicationBasis
                ? [
                    [
                      t('Referral refund grace'),
                      plan.application_balance?.referral_grace_amount ?? '0',
                    ],
                  ]
                : []),
              [
                t('Cash refund'),
                plan.calculation
                  ? (plan.calculation.cash_cents / 100).toFixed(2)
                  : '-',
              ],
            ].map(([label, value]) => (
              <div key={label}>
                <dt>{label}</dt>
                <dd className='break-all font-mono'>
                  {value === '-' ? value : `¥${value}`}
                </dd>
              </div>
            ))}
          </dl>
          <p>
            {t('Unexplained ledger difference')}: {plan.accounted_amount} -{' '}
            {plan.known_amount} = {plan.difference_amount ?? '-'}
          </p>
          <p>
            {applicationBasis
              ? t('Preserve noncash credits; no additional balance deduction.')
              : t(
                  'Deduct the unexplained difference as historical order discounts or promotional credits.',
                )}
          </p>
          {needsSettlement && !plan.applied && (
            <p role='alert'>
              {t(
                'Confirming accepts the settlement risk and clears the listed residual request records. Running tasks remain blocked.',
              )}
            </p>
          )}
          {!applicationBasis && (
            <p>
              {t('Promotional recovery formula')}: {plan.accounted_amount} -{' '}
              {plan.known_amount} = {plan.debit_amount}
            </p>
          )}
          {applicationBasis && (
            <p>
              {t('Accounting balance excluding noncash credits')}:{' '}
              {plan.balance_after} - {plan.application_balance?.noncash_amount}{' '}
              = {plan.application_balance?.cash_amount}
            </p>
          )}
          <p>
            {t('Adjustment balance formula')}: {plan.balance_before} -{' '}
            {plan.debit_amount} = {plan.balance_after}
          </p>
          {applicationBasis &&
            plan.application_balance?.referral_grace_amount !== undefined && (
              <p>
                {t('Effective refund grace')}: {plan.grace_amount} +{' '}
                {plan.application_balance.referral_grace_amount}
              </p>
            )}
          {plan.calculation && (
            <RefundCalculationDetails
              payments={plan.calculation.payments ?? []}
            />
          )}
          {plan.diagnostic && (
            <div role='alert'>
              {refundDiagnosticLines(plan.diagnostic, t).map((line) => (
                <p key={line}>{line}</p>
              ))}
            </div>
          )}
          {plan.blocker === 'no_adjustment' && (
            <p>
              {t(
                'No recovery or settlement acknowledgement is needed. Recalculate the refund normally.',
              )}
            </p>
          )}
          {plan.blocker === 'unsettled_tasks' && (
            <p role='alert'>
              {t('Unfinished asynchronous tasks prevent this adjustment.')}
            </p>
          )}
          {needsSettlement && (
            <ul className='space-y-2 break-all'>
              {plan.liabilities.map((row) => (
                <li key={row.id}>
                  <p>
                    {t('Request ID')}: {row.request_id}
                  </p>
                  <p>
                    {t('Settlement node')}: {row.node_name} /{' '}
                    {new Date(row.created_at * 1000).toISOString()}
                  </p>
                  <p>
                    {t('Request settled')}: {t(row.request_done ? 'Yes' : 'No')}{' '}
                    / {t('Task settled')}: {t(row.task_done ? 'Yes' : 'No')}
                  </p>
                </li>
              ))}
            </ul>
          )}
          {plan.applied ? (
            <>
              <p role='status'>
                {t('Adjustment saved; payment has not been made.')}
              </p>
              <p>
                {t('Operator')}: {plan.actor_id} /{' '}
                {new Date((plan.applied_at ?? 0) * 1000).toISOString()}
              </p>
              <p className='break-words'>{plan.reason}</p>
              <Button
                style={{
                  height: 'auto',
                  whiteSpace: 'normal',
                  maxWidth: '100%',
                }}
                onClick={download}
              >
                <Download size={16} aria-hidden='true' />
                {t('Download adjustment receipt')}
              </Button>
            </>
          ) : (
            <>
              {needsNegative && (
                <label className='flex items-start gap-2'>
                  <input
                    type='checkbox'
                    checked={negativeConfirmed}
                    disabled={busy || !plan.can_apply}
                    onChange={(e) => setNegativeConfirmed(e.target.checked)}
                  />
                  <span>
                    {t(
                      'I accept the negative cash balance within the configured refund grace.',
                    )}
                  </span>
                </label>
              )}
              <Button
                style={{
                  height: 'auto',
                  whiteSpace: 'normal',
                  maxWidth: '100%',
                }}
                disabled={
                  busy ||
                  !plan.can_apply ||
                  (needsNegative && !negativeConfirmed)
                }
                onClick={() => void apply()}
              >
                {applicationBasis
                  ? t('Confirm and recalculate')
                  : t('Confirm recovery and recalculate')}
              </Button>
            </>
          )}
        </>
      )}
      <div className='flex flex-wrap gap-2'>
        <Button
          title={t('Refresh')}
          aria-label={t('Refresh')}
          disabled={busy}
          onClick={() => void preview()}
        >
          <RefreshCw size={16} aria-hidden='true' />
        </Button>
        <Button
          title={t('Close')}
          aria-label={t('Close')}
          disabled={busy}
          onClick={() => {
            setOpen(false);
            onOpenChange(false);
          }}
        >
          <X size={16} aria-hidden='true' />
        </Button>
      </div>
    </section>
  );
}
