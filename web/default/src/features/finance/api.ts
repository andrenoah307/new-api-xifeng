import { api } from '@/lib/api'

export interface FinancePolicy {
  invoice_statuses: number[]
  topup_statuses: string[]
  payment_methods: string[]
  sources: string[]
}

export interface FinanceTopup {
  id: number
  trade_no: string
  money: number
  payment_method: string
  source: string
  status: string
  create_time: number
  complete_time: number
}

export interface FinanceRecord {
  id: number
  user_id: number
  username: string
  ticket_id: number
  company_name: string
  money: number
  invoice_status: number
  status: string
  created_time: number
  topups: FinanceTopup[]
  incomplete_orders: boolean
}

export interface FinanceFilter {
  view: 'invoices' | 'topups'
  keyword?: string
  status?: string
  invoice_status?: number
  start_time?: number
  end_time?: number
}

export async function getFinancePolicy(): Promise<FinancePolicy> {
  const response = await api.get('/api/finance/policy')
  if (!response.data.success) throw new Error(response.data.message)
  const policy = response.data.data as FinancePolicy
  return {
    invoice_statuses: policy.invoice_statuses ?? [],
    topup_statuses: policy.topup_statuses ?? [],
    payment_methods: policy.payment_methods ?? [],
    sources: policy.sources ?? [],
  }
}

export async function saveFinancePolicy(policy: FinancePolicy): Promise<void> {
  const response = await api.put('/api/finance/policy', policy)
  if (!response.data.success) throw new Error(response.data.message)
}

export async function getFinanceChoices(): Promise<
  { payment_method: string; source: string }[]
> {
  const response = await api.get('/api/finance/policy/choices')
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data ?? []
}

export async function getFinanceRecords(
  params: FinanceFilter & { p: number; page_size: number }
): Promise<{ items: FinanceRecord[]; total: number }> {
  const response = await api.get('/api/finance/records', { params })
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data
}

export async function exportFinanceRecords(
  params: FinanceFilter
): Promise<void> {
  const response = await api.get('/api/finance/export', {
    params,
    responseType: 'blob',
  })
  const blob = response.data as Blob
  if (!String(response.headers['content-type']).includes('text/csv')) {
    const body = JSON.parse(await blob.text()) as { message?: string }
    throw new Error(body.message)
  }
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `finance-${params.view}-${new Date().toISOString().slice(0, 10)}.csv`
  link.click()
  URL.revokeObjectURL(url)
}
