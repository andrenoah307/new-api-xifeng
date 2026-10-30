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
import React, { useContext, useEffect, useState } from 'react';
import { Navigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Button } from '@douyinfe/semi-ui';
import { ChevronLeft, ChevronRight, RefreshCw } from 'lucide-react';
import { API } from '../../helpers';
import { UserContext } from '../../context/User';
import RefundFinanceDetail, {
  refundCash,
} from '../../components/ticket/RefundFinanceDetail';

export default function RefundFinancePage() {
  const [state] = useContext(UserContext);
  if (!state.user) return null;
  if (![3, 10, 100].includes(state.user.role))
    return <Navigate to='/forbidden' replace />;
  return <RefundFinanceList key={`${state.user.id}:${state.user.role}`} />;
}
function RefundFinanceList() {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState(null);
  const [result, setResult] = useState({ items: [], total: 0 });
  const [revision, setRevision] = useState(0);
  const [error, setError] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    setResult({ items: [], total: 0 });
    API.get('/api/ticket/refund/finance-list', {
      params: { page, page_size: 20 },
      signal: controller.signal,
      disableDuplicate: true,
    })
      .then((response) => {
        if (!response.data.success) throw new Error(response.data.message);
        if (!controller.signal.aborted) {
          setResult(response.data.data);
          setError('');
        }
      })
      .catch((failure) => {
        if (!controller.signal.aborted) setError(failure.message);
      });
    return () => controller.abort();
  }, [page, revision]);
  const item =
    result.items.find((row) => row.ticket_id === selected) ?? result.items[0];
  return (
    <main className='mx-2 mt-16 min-w-0 space-y-5 py-5'>
      <h1 className='text-xl font-semibold'>{t('Refund reconciliation')}</h1>
      <div className='flex items-center justify-end gap-2'>
        <Button
          title={t('Refresh')}
          aria-label={t('Refresh')}
          icon={<RefreshCw size={16} />}
          onClick={() => setRevision((value) => value + 1)}
        />
        <Button
          title={t('Previous page')}
          aria-label={t('Previous page')}
          icon={<ChevronLeft size={16} />}
          disabled={page === 1}
          onClick={() => setPage((value) => value - 1)}
        />
        <span>{page}</span>
        <Button
          title={t('Next page')}
          aria-label={t('Next page')}
          icon={<ChevronRight size={16} />}
          disabled={page * 20 >= result.total}
          onClick={() => setPage((value) => value + 1)}
        />
      </div>
      {error && (
        <p role='alert' className='text-red-600'>
          {t(error)}
        </p>
      )}
      <div className='overflow-x-auto'>
        <table className='w-full text-sm'>
          <thead>
            <tr>
              {['Ticket ID', 'User ID', 'Cash refund', 'Status', 'Actions'].map(
                (label) => (
                  <th key={label} className='border-b p-3 text-left'>
                    {t(label)}
                  </th>
                ),
              )}
            </tr>
          </thead>
          <tbody>
            {result.items.map((row) => (
              <tr key={row.ticket_id}>
                <td className='p-3'>#{row.ticket_id}</td>
                <td className='p-3'>{row.user_id}</td>
                <td className='p-3'>
                  {refundCash(row.refund_cash_cents ?? 0)}
                </td>
                <td className='p-3'>
                  {t(row.refund_status === 1 ? 'Pending' : 'Processed')}
                </td>
                <td className='p-3'>
                  <Button onClick={() => setSelected(row.ticket_id)}>
                    {t('Details')}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {result.items.length === 0 && (
          <p className='p-8 text-center'>{t('No data')}</p>
        )}
      </div>
      {item?.finance && (
        <RefundFinanceDetail
          key={item.ticket_id}
          finance={item.finance}
          ticketId={item.ticket_id}
          onUpdated={() => setRevision((value) => value + 1)}
        />
      )}
    </main>
  );
}
