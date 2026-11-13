import { api } from '@/lib/api'

export interface TopupRecord {
  id: number
  user_id: number
  username?: string
  trade_no: string
  payment_method: string
  amount: number
  money: number
  status: string
  create_time: number
  // 0=未开票 1=开票中 2=已开票（仅 success 订单有意义）
  invoice_status?: number
  invoice_type?: string
  checkout_credit_amount?: string
}

interface TopupListResponse {
  items: TopupRecord[]
  total: number
}

export interface TopupListParams {
  p: number
  page_size: number
  keyword?: string
  status?: string
  invoice_type?: string
  start_time?: number
  end_time?: number
}

export async function getTopups(
  params: TopupListParams,
  admin: boolean
): Promise<TopupListResponse> {
  const url = admin ? '/api/user/topup' : '/api/user/topup/self'
  const res = await api.get(url, { params })
  if (!res.data?.success) throw new Error(res.data?.message)
  return res.data?.data ?? { items: [], total: 0 }
}

export async function completeTopupOrder(tradeNo: string): Promise<boolean> {
  const res = await api.post('/api/user/topup/complete', {
    trade_no: tradeNo,
  })
  if (!res.data?.success) throw new Error(res.data?.message)
  return res.data?.success ?? false
}

export async function updateTopupInvoiceType(
  id: number,
  invoiceType: string,
  expectedInvoiceType: string,
  revision?: string,
  confirmNegative = false
): Promise<InvoiceAdjustmentPlan> {
  const res = await api.request({
    method: revision ? 'PUT' : 'POST',
    url: revision
      ? '/api/user/topup/invoice-type'
      : '/api/user/topup/invoice-type/preview',
    data: {
      id,
      invoice_type: invoiceType,
      expected_invoice_type: expectedInvoiceType,
      revision,
      confirm_negative: confirmNegative,
    },
  })
  if (!res.data?.success) throw new Error(res.data?.message)
  return res.data.data
}

export type InvoiceAdjustmentPlan = {
  revision: string
  paid_cents: number
  tax_before_cents: number
  tax_after_cents: number
  balance_before: string
  balance_after: string
  balance_change: string
  grace_amount: string
  requires_negative_confirmation: boolean
  can_apply: boolean
}
