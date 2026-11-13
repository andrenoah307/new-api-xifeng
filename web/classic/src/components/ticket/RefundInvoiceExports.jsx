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
import { useContext } from 'react';
import { UserContext } from '../../context/User';
import { useEffect, useRef, useState } from 'react';
import { refundDiagnosticLines } from '../../../../shared/refund-diagnostics.mjs';
import { useTranslation } from 'react-i18next';
import { RefundAdjustment } from './RefundAdjustment';
import { Button } from '@douyinfe/semi-ui';
import { API as api } from '../../helpers/api';
import { RefundCalculationDetails } from './RefundCalculationDetails';
export default function RefundInvoiceExports({ ticketId, onAdjusted }) {
  const [state] = useContext(UserContext);
  if (![3, 10, 100].includes(state.user?.role)) return null;
  return (
    <RefundInvoiceExportPanel
      key={ticketId}
      ticketId={ticketId}
      onAdjusted={onAdjusted}
    />
  );
}
function RefundInvoiceExportPanel({ ticketId, onAdjusted }) {
  const { t } = useTranslation();
  const [amount, setAmount] = useState('');
  const [saved, setSaved] = useState(false);
  const [state] = useContext(UserContext);
  const [payments, setPayments] = useState([]);
  const [invoices, setInvoices] = useState([]);
  const [busy, setBusy] = useState(false);
  const [adjustmentOpen, setAdjustmentOpen] = useState(false);
  const [error, setError] = useState('');
  const [diagnostic, setDiagnostic] = useState(null);
  const requestVersion = useRef(0);
  const recalculating = useRef(false);
  useEffect(() => {
    let active = true;
    const version = requestVersion.current;
    api
      .get(`/api/ticket/refund/${ticketId}/invoice-export`, {
        skipBusinessError: true,
      })
      .then(({ data }) => {
        if (active && version === requestVersion.current && data.success) {
          setAmount((data.data.cash_cents / 100).toFixed(2));
          setSaved(true);
          setInvoices(data.data.invoices ?? []);
          setPayments(data.data.payments ?? []);
        }
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [ticketId]);
  async function recalculate() {
    if (busy || recalculating.current) return;
    recalculating.current = true;
    requestVersion.current += 1;
    setBusy(true);
    setError('');
    setDiagnostic(null);
    setSaved(false);
    try {
      const { data } = await api.post(
        `/api/ticket/refund/${ticketId}/invoice-export`,
        {},
        { skipBusinessError: true },
      );
      if (!data.success && data.diagnostic) {
        setDiagnostic(data.diagnostic);
        return;
      }
      if (!data.success) throw new Error(data.message);
      setAmount((data.data.cash_cents / 100).toFixed(2));
      setSaved(true);
      setInvoices(data.data.invoices ?? []);
      setPayments(data.data.payments ?? []);
    } catch (e) {
      setError(e instanceof Error ? t(e.message) : t('Operation failed'));
    } finally {
      recalculating.current = false;
      setBusy(false);
    }
  }
  async function download(kind) {
    if (busy || !saved) return;
    setBusy(true);
    setError('');
    try {
      const response = await api.get(
        `/api/ticket/refund/${ticketId}/invoice-export`,
        { params: { kind }, responseType: 'blob' },
      );
      if (
        !String(response.headers['content-type'] || '').includes('text/csv')
      ) {
        const result = JSON.parse(await response.data.text());
        throw new Error(result.message || t('Operation failed'));
      }
      const url = URL.createObjectURL(response.data);
      const link = document.createElement('a');
      link.href = url;
      link.download = `refund-${ticketId}-${kind}.csv`;
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
      setError(e instanceof Error ? t(e.message) : t('Operation failed'));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className='space-y-3 border-t pt-4'>
      <h3>{t('Refund invoice exports')}</h3>
      <p>
        {t('Cash refund')}: {saved ? `¥${amount}` : '-'}
      </p>
      <div className='flex flex-wrap gap-2'>
        <Button
          disabled={busy || adjustmentOpen}
          onClick={() => void recalculate()}
        >
          {t('Recalculate')}
        </Button>
        {['red', 'reissue'].map((kind, i) => (
          <Button
            key={kind}
            disabled={!saved || busy || adjustmentOpen}
            onClick={() => void download(kind)}
          >
            {t(
              [
                'Export invoices to reverse',
                'Export recalculated reissued invoices',
              ][i],
            )}
          </Button>
        ))}
      </div>
      <RefundAdjustment
        ticketId={ticketId}
        disabled={busy}
        onOpenChange={(open) => {
          requestVersion.current += 1;
          setAdjustmentOpen(open);
          if (open) setSaved(false);
        }}
        onApplied={(calculation) => {
          setAmount((calculation.cash_cents / 100).toFixed(2));
          setPayments(calculation.payments ?? []);
          setInvoices(calculation.invoices ?? []);
          setDiagnostic(null);
          setSaved(true);
          onAdjusted?.();
        }}
      />
      {saved && invoices.length > 0 && (
        <details>
          <summary className='cursor-pointer'>
            {t('Historical invoice tickets')} ({invoices.length})
          </summary>
          <ul className='mt-2 space-y-2'>
            {invoices.map((invoice) => (
              <li key={invoice.invoice_ticket_id}>
                <a
                  className='break-all underline'
                  href={`/console/ticket_admin/${invoice.invoice_ticket_id}`}
                >
                  #{invoice.invoice_ticket_id} {invoice.company_name}
                </a>
              </li>
            ))}
          </ul>
        </details>
      )}
      {error && <p role='alert'>{error}</p>}
      {diagnostic && (
        <div
          role='alert'
          className='space-y-2 border-l-2 border-amber-500 pl-3 text-sm break-words'
        >
          {refundDiagnosticLines(diagnostic, t).map((line) => (
            <p key={line}>{line}</p>
          ))}
          {!!diagnostic.ticket_id && (
            <a
              className='underline'
              href={`/console/ticket_admin/${diagnostic.ticket_id}`}
            >
              {t('Ticket ID')}: #{diagnostic.ticket_id}
            </a>
          )}
          {!!diagnostic.order_id && (
            <p>
              {t('Order ID')}: {diagnostic.order_id}
            </p>
          )}
          {diagnostic.amounts && (
            <details>
              <summary className='cursor-pointer'>{t('Exact amounts')}</summary>
              {refundDiagnosticLines(diagnostic, t, true)
                .slice(0, 3)
                .map((line) => (
                  <p key={line}>{line}</p>
                ))}
            </details>
          )}
        </div>
      )}
      {saved && [10, 100].includes(state.user?.role) && (
        <RefundCalculationDetails payments={payments} />
      )}
    </section>
  );
}
