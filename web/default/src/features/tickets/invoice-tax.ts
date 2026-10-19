import { api } from '@/lib/api'

export interface InvoiceTaxConfig {
  enabled: boolean
  legacy_topups_enabled?: boolean
  mode: 'vat' | 'compound' | 'gross_up'
  ordinary_rate_bps: number
  special_rate_bps: number
}

export interface InvoiceTaxQuote {
  prepaid_only: boolean
  prepaid_net_cents: number
  prepaid_tax_cents: number
  invoice_type: string
  mode: string
  rate_bps: number
  base_cents: number
  fee_cents: number
  total_cents: number
  quota: number
  can_balance: boolean
  token: string
}

export interface InvoiceTaxCharge extends Omit<
  InvoiceTaxQuote,
  'can_balance' | 'token' | 'prepaid_only' | 'prepaid_net_cents' | 'prepaid_tax_cents'
> {
  ticket_id: number
  user_id: number
  status: string
  payment_method: string
  trade_no: string
}

export interface InvoiceTaxSelection {
  ready: boolean
  invoice_type: string
  tax_payment_method: string
  tax_quote: string
}

export async function getInvoiceTaxQuote(
  ids: number[],
  type: string
): Promise<InvoiceTaxQuote> {
  const { data } = await api.post('/api/ticket/invoice/tax-quote', {
    topup_order_ids: ids,
    invoice_type: type,
  })
  if (!data.success) throw new Error(data.message)
  return data.data
}

export const invoiceTaxModes = [
  { value: 'vat', label: 'VAT added to net amount', formula: 'B × (1 + r)' },
  {
    value: 'compound',
    label: 'Markup followed by VAT',
    formula: 'B × (1 + r)²',
  },
  {
    value: 'gross_up',
    label: 'Gross-up by total invoice amount',
    formula: 'B ÷ (1 − r)',
  },
] as const

export async function getInvoiceTaxConfig(): Promise<InvoiceTaxConfig> {
  const { data } = await api.get('/api/ticket/invoice/tax-config')
  if (!data.success) throw new Error(data.message)
  return data.data
}

export async function saveInvoiceTaxConfig(config: InvoiceTaxConfig) {
  const { data } = await api.put('/api/option/invoice-tax', config)
  if (!data.success) throw new Error(data.message)
}
