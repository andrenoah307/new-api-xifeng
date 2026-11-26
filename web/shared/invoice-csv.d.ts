export function generateInvoiceCSV(items: Array<{
  ticket_id: number
  email: string
  total_money: number
  tax_number: string
  company_name: string
  remark?: string
}>, serviceName: string): string

interface FinanceInvoiceCSVItem {
  ticket_id: number
  email: string
  total_money: number
  tax_number: string
  company_name: string
  remark?: string
  invoice_type?: string
  tax_rate_bps?: number
  issuer_amounts?: { account_name: string; amount_cents: number }[]
  correction_rows?: FinanceInvoiceCSVItem[]
  order_numbers?: string[]
  invoice_color?: 'red' | 'blue'
}
export function generateFinanceInvoiceCSV(items: FinanceInvoiceCSVItem[], serviceName: string, refund?: boolean): string
