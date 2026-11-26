export function generateInvoiceCSV(items: Array<{
  ticket_id: number
  email: string
  total_money: number
  tax_number: string
  company_name: string
  remark?: string
}>, serviceName: string): string
