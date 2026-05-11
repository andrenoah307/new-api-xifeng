import { api } from '@/lib/api'

// ============================================================================
// Types
// ============================================================================

export interface Ticket {
  id: number
  subject: string
  type: string
  status: number
  priority: number
  user_id: number
  username: string
  assignee_id: number
  created_time: number
  updated_time: number
  refund_quota?: number
  refund_status?: number
  invoice_money?: number
  company_name?: string
}

export interface TicketMessage {
  id: number
  ticket_id: number
  user_id: number
  username: string
  role: number
  content: string
  attachments: TicketAttachment[] | null
  created_time: number
}

export interface TicketAttachment {
  id: number
  file_name: string
  mime_type: string
  size: number
}

export interface TicketInvoice {
  id: number
  ticket_id: number
  company_name: string
  tax_number: string
  email: string
  bank_name: string
  bank_account: string
  company_address: string
  company_phone: string
  invoice_status: number
  total_money: number
  issued_time: number
  created_time: number
}

export interface TicketInvoiceOrder {
  id: number
  trade_no: string
  money: number
  create_time: number
  complete_time: number
  payment_method: string
}

export interface TicketRefund {
  id: number
  ticket_id: number
  user_id: number
  refund_status: number
  refund_quota: number
  frozen_quota: number
  user_quota_snapshot: number
  actual_refund_quota: number
  quota_mode: string
  payee_type: string
  payee_name: string
  payee_account: string
  payee_bank: string
  contact: string
  reason: string
  created_time: number
  processed_time: number
}

export interface StaffUser {
  id: number
  username: string
  display_name: string
  role: number
}

export interface UserProfile {
  user_id: number
  username: string
  display_name: string
  email: string
  role: number
  status: number
  group: string
  created_time: number
  quota: number
  used_quota: number
  request_count: number
  pending_refund_quota: number
  recent_logs: Array<{
    created_at: number
    model_name: string
    token_name: string
    group: string
    quota: number
    prompt_tokens: number
    completion_tokens: number
  }>
  model_usage: Array<{
    model_name: string
    count: number
    quota: number
    token_used: number
  }>
}

interface TicketListResponse {
  items: Ticket[]
  total: number
}

interface TicketDetailResponse {
  ticket: Ticket
  messages: TicketMessage[]
  invoice?: TicketInvoice
  invoice_orders?: TicketInvoiceOrder[]
  refund?: TicketRefund
}

// ============================================================================
// User API
// ============================================================================

export async function getUserTickets(params: {
  p: number
  page_size: number
  status?: string
  type?: string
}): Promise<TicketListResponse> {
  const res = await api.get('/api/ticket/self', { params })
  return res.data?.data ?? { items: [], total: 0 }
}

export async function getUserTicketDetail(
  id: number
): Promise<TicketDetailResponse | null> {
  const res = await api.get(`/api/ticket/self/${id}`)
  return res.data?.data ?? null
}

export async function sendUserMessage(
  ticketId: number,
  content: string,
  attachmentIds: number[]
): Promise<boolean> {
  const res = await api.post(`/api/ticket/self/${ticketId}/message`, {
    content,
    attachment_ids: attachmentIds,
  })
  return res.data?.success ?? false
}

export async function closeUserTicket(id: number): Promise<boolean> {
  const res = await api.put(`/api/ticket/self/${id}/close`)
  return res.data?.success ?? false
}

export async function createGeneralTicket(data: {
  subject: string
  type: string
  priority: number
  content: string
  attachment_ids: number[]
}): Promise<{ id: number } | null> {
  const res = await api.post('/api/ticket/', data)
  return res.data?.data ?? null
}

export async function createRefundTicket(data: {
  subject: string
  priority: number
  refund_quota: number
  payee_type: string
  payee_name: string
  payee_account: string
  payee_bank: string
  contact: string
  reason: string
  attachment_ids: number[]
}): Promise<{ id: number } | null> {
  const res = await api.post('/api/ticket/refund/', data)
  return res.data?.data ?? null
}

export async function createInvoiceTicket(data: {
  subject: string
  company_name: string
  tax_number: string
  content: string
  email: string
  topup_order_ids: number[]
}): Promise<{ id: number } | null> {
  const res = await api.post('/api/ticket/invoice/', data)
  return res.data?.data ?? null
}

export async function getEligibleInvoiceOrders(): Promise<
  TicketInvoiceOrder[]
> {
  const res = await api.get('/api/ticket/invoice/eligible_orders')
  return Array.isArray(res.data?.data) ? res.data.data : []
}

export async function getCurrentUserQuota(): Promise<{
  quota: number
  max_refundable_quota: number
} | null> {
  const res = await api.get('/api/user/self')
  return res.data?.data ?? null
}

// ============================================================================
// Admin API
// ============================================================================

export async function getAdminTickets(params: {
  p: number
  page_size: number
  status?: string
  type?: string
  keyword?: string
  scope?: string
}): Promise<TicketListResponse> {
  const res = await api.get('/api/ticket/admin', { params })
  return res.data?.data ?? { items: [], total: 0 }
}

export async function getAdminTicketDetail(
  id: number
): Promise<TicketDetailResponse | null> {
  const res = await api.get(`/api/ticket/admin/${id}`)
  return res.data?.data ?? null
}

export async function getAdminInvoiceDetail(
  ticketId: number
): Promise<{ invoice: TicketInvoice; orders: TicketInvoiceOrder[] } | null> {
  const res = await api.get(`/api/ticket/admin/${ticketId}/invoice`)
  return res.data?.data ?? null
}

export async function getAdminRefundDetail(
  ticketId: number
): Promise<TicketRefund | null> {
  const res = await api.get(`/api/ticket/admin/${ticketId}/refund`)
  return res.data?.data?.refund ?? null
}

export async function getAdminUserProfile(
  ticketId: number
): Promise<UserProfile | null> {
  const res = await api.get(`/api/ticket/admin/${ticketId}/user-profile`)
  return res.data?.data ?? null
}

export async function getStaffList(): Promise<StaffUser[]> {
  const res = await api.get('/api/ticket/admin/staff')
  return Array.isArray(res.data?.data) ? res.data.data : []
}

export async function sendAdminMessage(
  ticketId: number,
  content: string,
  attachmentIds: number[]
): Promise<boolean> {
  const res = await api.post(`/api/ticket/admin/${ticketId}/message`, {
    content,
    attachment_ids: attachmentIds,
  })
  return res.data?.success ?? false
}

export async function updateTicketStatus(
  ticketId: number,
  status: number,
  priority: number
): Promise<boolean> {
  const res = await api.put(`/api/ticket/admin/${ticketId}/status`, {
    status,
    priority,
  })
  return res.data?.success ?? false
}

export async function assignTicket(
  ticketId: number,
  assigneeId: number,
  expectedAssigneeId: number
): Promise<boolean> {
  const res = await api.put(`/api/ticket/admin/${ticketId}/assign`, {
    assignee_id: assigneeId,
    expected_assignee_id: expectedAssigneeId,
  })
  return res.data?.success ?? false
}

export async function updateRefundStatus(
  ticketId: number,
  refundStatus: number,
  extra?: { quota_mode?: string; actual_refund_quota?: number }
): Promise<boolean> {
  const res = await api.put(`/api/ticket/admin/${ticketId}/refund/status`, {
    refund_status: refundStatus,
    ...extra,
  })
  return res.data?.success ?? false
}

export async function updateInvoiceStatus(
  ticketId: number,
  invoiceStatus: number
): Promise<boolean> {
  const res = await api.put(`/api/ticket/admin/${ticketId}/invoice/status`, {
    invoice_status: invoiceStatus,
  })
  return res.data?.success ?? false
}

export async function getUserQuota(
  userId: number
): Promise<{ quota: number } | null> {
  const res = await api.get(`/api/user/${userId}`)
  return res.data?.data ?? null
}

// ============================================================================
// Attachment API
// ============================================================================

export async function uploadAttachment(
  file: File
): Promise<TicketAttachment | null> {
  const formData = new FormData()
  formData.append('file', file)
  const res = await api.post('/api/ticket/attachment', formData)
  return res.data?.data ?? null
}

export async function deleteAttachment(id: number): Promise<boolean> {
  const res = await api.delete(`/api/ticket/attachment/${id}`)
  return res.data?.success ?? false
}

export function getAttachmentUrl(id: number, inline = false): string {
  return `/api/ticket/attachment/${id}${inline ? '?inline=1' : ''}`
}

export interface InvoiceExportItem {
  ticket_id: number
  company_name: string
  tax_number: string
  email: string
  total_money: number
  order_count: number
  status: number
  created_time: number
}

export interface InvoiceExportListParams {
  p: number
  page_size: number
  keyword?: string
  status?: number
  start_time?: number
  end_time?: number
}

export async function getInvoiceExportList(
  params: InvoiceExportListParams
): Promise<{ items: InvoiceExportItem[]; total: number }> {
  const res = await api.get('/api/ticket/admin/invoice/export-list', { params })
  return res.data?.data ?? { items: [], total: 0 }
}
