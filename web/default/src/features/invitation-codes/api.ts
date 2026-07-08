import { api } from '@/lib/api'

// ============================================================================
// Types
// ============================================================================

export interface InvitationCode {
  id: number
  name: string
  code: string
  status: number
  used_count: number
  max_uses: number
  owner_user_id: number
  created_by: number
  is_admin: boolean
  created_time: number
  expired_time: number
}

export interface InvitationCodeUsage {
  id: number
  user_id: number
  username: string
  used_time: number
}

export interface InvitationQuotaInfo {
  limit: number
  remaining: number
  can_generate: boolean
  default_code_max_uses: number
  default_code_valid_days: number
  reason?: string
}

export interface CreateInvitationCodeParams {
  name: string
  count: number
  max_uses: number
  owner_user_id: number
  expired_time: number
}

export interface UpdateInvitationCodeParams {
  id: number
  name: string
  status: number
  max_uses: number
  owner_user_id: number
  expired_time: number
}

interface ListResponse {
  items: InvitationCode[]
  page: number
  total: number
}

// ============================================================================
// Admin API
// ============================================================================

export async function getInvitationCodes(
  page: number,
  pageSize: number
): Promise<ListResponse> {
  const res = await api.get('/api/invitation_code/', {
    params: { p: page, page_size: pageSize },
  })
  return res.data?.data ?? { items: [], page: 1, total: 0 }
}

export async function searchInvitationCodes(
  keyword: string,
  page: number,
  pageSize: number
): Promise<ListResponse> {
  const res = await api.get('/api/invitation_code/search', {
    params: { keyword, p: page, page_size: pageSize },
  })
  return res.data?.data ?? { items: [], page: 1, total: 0 }
}

export async function getInvitationCode(
  id: number
): Promise<InvitationCode | null> {
  const res = await api.get(`/api/invitation_code/${id}`)
  return res.data?.data ?? null
}

export async function createInvitationCodes(
  params: CreateInvitationCodeParams
): Promise<string[]> {
  const res = await api.post('/api/invitation_code/', params)
  const data = res.data?.data
  return Array.isArray(data) ? data : []
}

export async function updateInvitationCode(
  params: UpdateInvitationCodeParams
): Promise<boolean> {
  const res = await api.put('/api/invitation_code/', params)
  return res.data?.success ?? false
}

export async function deleteInvitationCode(id: number): Promise<boolean> {
  const res = await api.delete(`/api/invitation_code/${id}`)
  return res.data?.success ?? false
}

export async function clearInvalidInvitationCodes(): Promise<number> {
  const res = await api.delete('/api/invitation_code/invalid')
  return typeof res.data?.data === 'number' ? res.data.data : 0
}

export async function getInvitationCodeUsages(
  id: number
): Promise<InvitationCodeUsage[]> {
  const res = await api.get(`/api/invitation_code/${id}/usages`)
  return Array.isArray(res.data?.data) ? res.data.data : []
}

// ============================================================================
// User API
// ============================================================================

export async function getUserInvitationQuota(): Promise<InvitationQuotaInfo | null> {
  const res = await api.get('/api/user/invitation_codes/quota')
  return res.data?.data ?? null
}

export async function getUserInvitationCodes(): Promise<InvitationCode[]> {
  const res = await api.get('/api/user/invitation_codes')
  return Array.isArray(res.data?.data) ? res.data.data : []
}

export async function generateUserInvitationCode(): Promise<string | null> {
  const res = await api.post('/api/user/invitation_codes')
  return res.data?.data?.code ?? null
}
