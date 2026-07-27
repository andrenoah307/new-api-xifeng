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
  start_time?: number
  end_time?: number
}

export async function getTopups(
  params: TopupListParams,
  admin: boolean
): Promise<TopupListResponse> {
  const url = admin ? '/api/user/topup' : '/api/user/topup/self'
  const res = await api.get(url, { params })
  return res.data?.data ?? { items: [], total: 0 }
}

export async function completeTopupOrder(tradeNo: string): Promise<boolean> {
  const res = await api.post('/api/user/topup/complete', {
    trade_no: tradeNo,
  })
  return res.data?.success ?? false
}
