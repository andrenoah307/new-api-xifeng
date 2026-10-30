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
import { api } from '@/lib/api'

export interface RefundAllocation {
  quota_per_unit: string
  used_invoice_cents: number
  original_tax_cents: number
  top_up_id: number
  trade_no: string
  invoice_ticket_id: number
  original_quota: number
  used_quota: number
  refundable_quota: number
  refund_quota: number
  original_paid_cents: number
  refund_cents: number
  refund_tax_cents: number
  remaining_invoice_cents: number
}
export interface InvoiceCorrection {
  id: number
  invoice_ticket_id: number
  original_cents: number
  refund_cents: number
  reissue_cents: number
  status: string
  original_reference: string
  red_reference: string
  reissue_reference: string
  evidence: string
}
export interface RefundFinanceDetail {
  quote_token?: string
  version: number
  refund_cash_cents: number
  refund_tax_cents: number
  allocations: RefundAllocation[]
  invoice_corrections: InvoiceCorrection[]
}
export async function quoteRefundFinance(quota: number, signal?: AbortSignal) {
  const response = await api.get('/api/ticket/refund/finance-quote', {
    params: { refund_quota: quota },
    signal,
    disableDuplicate: true,
  })
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data as RefundFinanceDetail | null
}
