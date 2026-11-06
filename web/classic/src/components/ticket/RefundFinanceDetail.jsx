/*
Copyright (C) 2023-2026 QuantumNous

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
import React, { useContext, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@douyinfe/semi-ui';
import { Check, Download, RefreshCw, Play } from 'lucide-react';
import { API } from '../../helpers';
import { UserContext } from '../../context/User';

import { refundCash, refundQuota } from './refundFinanceFormat';
export { refundCash } from './refundFinanceFormat';
import { CorrectionWorkflow, ReconciliationPanel, RefundPayoutPanel } from './FinanceWorkflow';

export default function RefundFinanceDetail({
  finance,
  ticketId,
  userId,
  requestedQuota,
  onUpdated,
}) {
  const { t } = useTranslation();
  const [state] = useContext(UserContext);
  const role = state.user?.role;
  const staff = [3, 10, 100].includes(role);
  const [error, setError] = useState('');
  const [quote, setQuote] = useState(null);
  const [busy, setBusy] = useState(false);
  async function confirmAmount(confirm) {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      const response =
        confirm && quote
          ? await API.post('/api/ticket/refund/finance-confirm', {
              ticket_id: ticketId,
              refund_quota: quote.allocations.reduce(
                (sum, row) => sum + row.refund_quota,
                0,
              ),
              quote_token: quote.quote_token,
            })
          : await API.get('/api/ticket/refund/finance-quote', {
              params: { refund_quota: requestedQuota },
            });
      if (!response.data.success) throw new Error(response.data.message);
      if (confirm) window.location.reload();
      else setQuote(response.data.data);
    } catch (failure) {
      setQuote(null);
      setError(failure.message);
    } finally {
      setBusy(false);
    }
  }
  async function download() {
    try {
      const response = await API.get('/api/ticket/refund/finance-export', {
        params: { ticket_id: ticketId },
        responseType: 'blob',
      });
      if (response.headers['content-type']?.includes('json'))
        throw new Error(t('Export failed'));
      const url = URL.createObjectURL(response.data);
      const link = document.createElement('a');
      link.href = url;
      link.download = `refund-${ticketId}.csv`;
      link.click();
      URL.revokeObjectURL(url);
    } catch (failure) {
      setError(failure.message);
    }
  }
  if (
    finance.settlement_status === 'waiting' &&
    finance.allocations.length === 0
  ) {
    return (
      <div className='space-y-3 text-sm' role='status'>
        {t((quote ?? finance).waiting_reason === 'financial_reconciliation' ? 'Financial reconciliation required' : 'Awaiting settlement verification. The refund amount must be confirmed again after verification.')}
        <p>
          {t(
            'New consumption is suspended until this refund is completed or cancelled.',
          )}
        </p>
        {userId === state.user?.id && ticketId && (
          <Button
            disabled={busy}
            icon={<RefreshCw size={16} />}
            onClick={() => confirmAmount(false)}
          >
            {t('Refresh')}
          </Button>
        )}
        {quote?.quote_token && (
          <div className='flex flex-wrap items-center gap-3'>
            <span>
              {t('Cash refund')}: {refundCash(quote.refund_cash_cents)}
            </span>
            <Button
              disabled={busy}
              icon={<Check size={16} />}
              onClick={() => confirmAmount(true)}
            >
              {t('Confirm')}
            </Button>
          </div>
        )}
        {error && <p role='alert'>{t(error)}</p>}
        {staff && ticketId && <ReconciliationPanel ticketId={ticketId} />}
      </div>
    );
  }
  return (
    <section className='space-y-4 border-t pt-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h3 className='font-semibold'>{t('Refund reconciliation')}</h3>
        {staff && ticketId && (
          <Button
            htmlType='button'
            icon={<Download size={16} />}
            onClick={download}
          >
            {t('Export')}
          </Button>
        )}
      </div>
      <dl className='grid grid-cols-2 gap-4 text-sm'>
        <div>
          <dt>{t('Cash refund')}</dt>
          <dd className='font-semibold'>
            {refundCash(finance.refund_cash_cents)}
          </dd>
        </div>
        <div>
          <dt>{t('Refunded tax')}</dt>
          <dd>{refundCash(finance.refund_tax_cents)}</dd>
        </div>
      </dl>
      {error && (
        <p role='alert' className='text-red-600'>
          {t(error)}
        </p>
      )}
      <div className='overflow-x-auto'>
        <table className='w-full text-sm'>
          <thead>
            <tr>
              {[
                'Order number',
                'Paid amount',
                'Original tax',
                'Used invoice amount',
                'Original quota',
                'Used quota',
                'Refundable quota',
                'Refund quota',
                'Cash refund',
                'Refunded tax',
                'Remaining invoice amount',
              ].map((label) => (
                <th
                  key={label}
                  className='whitespace-nowrap border-b p-2 text-left'
                >
                  {t(label)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {finance.allocations.map((row) => (
              <tr key={row.top_up_id}>
                <td className='whitespace-nowrap p-2'>
                  {row.trade_no || `#${row.top_up_id}`}
                </td>
                {[
                  row.original_paid_cents,
                  row.original_tax_cents,
                  row.used_invoice_cents,
                ].map((value, index) => (
                  <td key={index} className='whitespace-nowrap p-2'>
                    {refundCash(value)}
                  </td>
                ))}
                {[
                  row.original_quota,
                  row.used_quota,
                  row.refundable_quota,
                  row.refund_quota,
                ].map((value, index) => (
                  <td key={index} className='whitespace-nowrap p-2'>
                    {refundQuota(value, row.quota_per_unit)}
                  </td>
                ))}
                {[
                  row.refund_cents,
                  row.refund_tax_cents,
                  row.remaining_invoice_cents,
                ].map((value, index) => (
                  <td key={index} className='whitespace-nowrap p-2'>
                    {refundCash(value)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {finance.invoice_corrections.map((correction) => correction.version === 1 ? <CorrectionWorkflow key={`${correction.id}:${correction.phase}:${correction.status}`} correction={correction} editable={staff && !!ticketId} onUpdated={onUpdated} /> : (
        <Correction
          key={`${correction.invoice_ticket_id}:${correction.id}:${correction.status}`}
          correction={correction}
          editable={staff && !!ticketId}
          onUpdated={onUpdated}
        />
      ))}
      {staff && ticketId && <RefundPayoutPanel key={`${ticketId}:${finance.invoice_corrections.map(row=>`${row.id}:${row.status}`).join(',')}`} ticketId={ticketId} onUpdated={onUpdated} />}
    </section>
  );
}

function Correction({ correction, editable, onUpdated }) {
  const { t } = useTranslation();
  const [red, setRed] = useState(correction.red_reference || '');
  const [reissue, setReissue] = useState(correction.reissue_reference || '');
  const [evidence, setEvidence] = useState(correction.evidence || '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [status, setStatus] = useState(correction.status);
  const completed = status === 'completed';
  const processing = status === 'processing';
  async function start() {
    setBusy(true);
    try {
      const response = await API.post(
        `/api/ticket/refund/invoice-corrections/${correction.id}/start`,
      );
      if (!response.data.success) throw new Error(response.data.message);
      setStatus('processing');
      onUpdated?.(correction.id, 'processing');
    } catch (failure) {
      setError(failure.message);
    } finally {
      setBusy(false);
    }
  }
  async function complete() {
    if (busy || !editable || !processing) return;
    setBusy(true);
    try {
      const response = await API.post(
        `/api/ticket/refund/invoice-corrections/${correction.id}/complete`,
        {
          red_reference: red.trim(),
          reissue_reference: reissue.trim(),
          evidence: evidence.trim(),
        },
      );
      if (!response.data.success) throw new Error(response.data.message);
      setStatus('completed');
      onUpdated?.(correction.id, 'completed');
    } catch (failure) {
      setError(failure.message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className='space-y-3 border-t pt-4'>
      <h4 className='font-medium'>
        {t('Invoice correction')} #{correction.invoice_ticket_id} ·{' '}
        {completed && t('Completed')}
        {processing && t('Processing')}
        {status === 'pending' && t('Pending')}
        {status === 'cancelled' && t('Cancelled')}
      </h4>
      <p className='text-sm'>
        {t('Original invoice amount')}: {refundCash(correction.original_cents)}{' '}
        · {t('Cash refund')}: {refundCash(correction.refund_cents)} ·{' '}
        {t('Reissued invoice amount')}: {refundCash(correction.reissue_cents)}
      </p>
      <p className='text-sm'>
        {t('Original invoice reference')}:{' '}
        {correction.original_reference || '-'}
      </p>
      <div className='grid gap-3 sm:grid-cols-2'>
        <label className='flex flex-col gap-2 text-sm'>
          {t('Red invoice reference')}
          <input
            className='rounded border bg-transparent p-2'
            value={red}
            onChange={(event) => setRed(event.target.value)}
            maxLength={128}
            disabled={!editable || !processing}
          />
        </label>
        <label className='flex flex-col gap-2 text-sm'>
          {t('Reissued invoice reference')}
          <input
            className='rounded border bg-transparent p-2'
            value={reissue}
            onChange={(event) => setReissue(event.target.value)}
            maxLength={128}
            disabled={
              !editable || !processing || correction.reissue_cents === 0
            }
          />
        </label>
        <label className='flex flex-col gap-2 text-sm sm:col-span-2'>
          {t('Correction evidence')}
          <textarea
            className='rounded border bg-transparent p-2'
            value={evidence}
            onChange={(event) => setEvidence(event.target.value)}
            maxLength={2000}
            disabled={!editable || !processing}
          />
        </label>
      </div>
      {error && (
        <p role='alert' className='text-red-600'>
          {t(error)}
        </p>
      )}
      {editable && processing && (
        <Button
          htmlType='button'
          theme='solid'
          icon={<Check size={16} />}
          onClick={complete}
          disabled={
            busy ||
            (correction.original_cents > 0 && !red.trim()) ||
            !evidence.trim() ||
            (correction.reissue_cents > 0 && !reissue.trim())
          }
        >
          {t('Register correction')}
        </Button>
      )}
      {editable && status === 'pending' && (
        <Button disabled={busy} icon={<Play size={16} />} onClick={start}>
          {t('Start correction')}
        </Button>
      )}
    </div>
  );
}
