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
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@douyinfe/semi-ui';
import { API as api } from '../../helpers/api';
import { RefundCalculationDetails } from './RefundCalculationDetails';
export default function RefundInvoiceExports({ ticketId }) {
  const [state] = useContext(UserContext);
  if (![3, 10, 100].includes(state.user?.role)) return null;
  return <RefundInvoiceExportPanel key={ticketId} ticketId={ticketId} />;
}
function RefundInvoiceExportPanel({ ticketId }) {
  const { t } = useTranslation();
  const [amount, setAmount] = useState('');
  const [saved, setSaved] = useState(false);
  const [state] = useContext(UserContext);
  const [payments, setPayments] = useState([]);
  const [invoices, setInvoices] = useState([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => {
    let active = true;
    setAmount('');
    setSaved(false);
    api
      .get(`/api/ticket/refund/${ticketId}/invoice-export`, {
        skipBusinessError: true,
      })
      .then(({ data }) => {
        if (active && data.success) {
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
    if (busy) return;
    setBusy(true);
    setError('');
    setSaved(false);
    try {
      const { data } = await api.post(
        `/api/ticket/refund/${ticketId}/invoice-export`,
        {},
      );
      if (!data.success) throw new Error(data.message);
      setAmount((data.data.cash_cents / 100).toFixed(2));
      setSaved(true);
      setInvoices(data.data.invoices ?? []);
      setPayments(data.data.payments ?? []);
    } catch (e) {
      setError(e instanceof Error ? t(e.message) : t('Operation failed'));
    } finally {
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
        <Button disabled={busy} onClick={() => void recalculate()}>
          {t('Recalculate')}
        </Button>
        {['red', 'reissue'].map((kind, i) => (
          <Button
            key={kind}
            disabled={!saved || busy}
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
      {saved && [10, 100].includes(state.user?.role) && (
        <RefundCalculationDetails payments={payments} />
      )}
    </section>
  );
}
