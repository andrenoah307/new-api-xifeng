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
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ChevronLeft, ChevronRight, RefreshCw } from 'lucide-react'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import type { TicketRefund } from './api'
import { RefundFinanceDetail } from './components/refund-finance-detail'
import { refundCash } from './refund-finance-format'

export function RefundFinancePage() {
  const user = useAuthStore((state) => state.auth.user)
  if (!user || ![3, 10, 100].includes(user.role)) return null
  return (
    <RefundFinanceList
      key={`${user.id}:${user.role}`}
      userId={user.id}
      role={user.role}
    />
  )
}
function RefundFinanceList(props: { userId: number; role: number }) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<number | null>(null)
  const query = useQuery({
    queryKey: [
      'tickets',
      'refund-finance-list',
      props.userId,
      props.role,
      page,
    ],
    queryFn: async ({ signal }) => {
      const response = await api.get('/api/ticket/refund/finance-list', {
        params: { page, page_size: 20 },
        signal,
        disableDuplicate: true,
      })
      if (!response.data.success) throw new Error(response.data.message)
      return response.data.data as { items: TicketRefund[]; total: number }
    },
    gcTime: 0,
  })
  const item =
    query.data?.items.find((row) => row.ticket_id === selected) ??
    query.data?.items[0]
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Refund reconciliation')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mb-4 flex items-center justify-end gap-2'>
          <Button
            variant='outline'
            aria-label={t('Refresh')}
            title={t('Refresh')}
            onClick={() => void query.refetch()}
          >
            <RefreshCw />
          </Button>
          <Button
            variant='outline'
            aria-label={t('Previous page')}
            title={t('Previous page')}
            disabled={page === 1}
            onClick={() => setPage((value) => value - 1)}
          >
            <ChevronLeft />
          </Button>
          <span>{page}</span>
          <Button
            variant='outline'
            aria-label={t('Next page')}
            title={t('Next page')}
            disabled={page * 20 >= (query.data?.total ?? 0)}
            onClick={() => setPage((value) => value + 1)}
          >
            <ChevronRight />
          </Button>
        </div>
        {query.isError && (
          <p role='alert' className='text-destructive'>
            {t(query.error.message)}
          </p>
        )}
        <div className='mb-6 overflow-x-auto'>
          <table className='w-full text-sm'>
            <thead>
              <tr>
                {[
                  'Ticket ID',
                  'User ID',
                  'Cash refund',
                  'Status',
                  'Actions',
                ].map((label) => (
                  <th key={label} className='border-b p-3 text-left'>
                    {t(label)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {query.data?.items.map((row) => (
                <tr
                  key={row.ticket_id}
                  className={
                    row.ticket_id === item?.ticket_id ? 'bg-muted' : ''
                  }
                >
                  <td className='p-3'>#{row.ticket_id}</td>
                  <td className='p-3'>{row.user_id}</td>
                  <td className='p-3'>
                    {refundCash(row.refund_cash_cents ?? 0)}
                  </td>
                  <td className='p-3'>
                    {t(row.refund_status === 1 ? 'Pending' : 'Processed')}
                  </td>
                  <td className='p-3'>
                    <Button
                      variant='outline'
                      onClick={() => setSelected(row.ticket_id)}
                    >
                      {t('Details')}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {query.data?.items.length === 0 && (
            <p className='p-8 text-center'>{t('No data')}</p>
          )}
        </div>
        {item?.finance && (
          <RefundFinanceDetail
            key={item.ticket_id}
            finance={item.finance}
            ticketId={item.ticket_id}
            onUpdated={() => void query.refetch()}
          />
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
