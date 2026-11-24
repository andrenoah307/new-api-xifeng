export interface InvoiceClosureItem {
  ticket_id: number
  user_id: number
  username: string
  invoice_title: string
  amount: number
  supplement_tax_cents: number
  order_tax_cents: number
  quota_delta: string
  revision: string
  error?: string
  warning?: string
  applied: boolean
}
type Post = (url: string, body: unknown) => Promise<{ data: { success: boolean; message?: string; data: unknown } }>
export function previewInvoiceGroupClosures(post: Post, convert: boolean): Promise<InvoiceClosureItem[]>
export function closeInvoiceGroupTicket(post: Post, item: InvoiceClosureItem, convert: boolean): Promise<InvoiceClosureItem>
