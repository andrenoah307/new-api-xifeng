export interface InvoiceClosureItem {
  ticket_id: number
  user_id: number
  supplement_tax_cents: number
  order_tax_cents: number
  quota_delta: string
  revision: string
  error?: string
  applied: boolean
}
type Post = (url: string, body: unknown) => Promise<{ data: { success: boolean; message?: string; data: unknown } }>
export function previewInvoiceGroupClosures(post: Post, convert: boolean): Promise<InvoiceClosureItem[]>
export function closeInvoiceGroupTicket(post: Post, item: InvoiceClosureItem, convert: boolean): Promise<InvoiceClosureItem>
export function invoiceClosureStatus(item: InvoiceClosureItem): string
