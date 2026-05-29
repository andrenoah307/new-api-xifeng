import { api } from '@/lib/api'
import type {
  AutoGroupRule,
  ApiResponse,
  GetRulesParams,
  GetRulesResponse,
  GetEnrollmentsParams,
  GetEnrollmentsResponse,
  RuleFormData,
  AutoGroupEnrollment,
} from './types'

// ============================================================================
// Groups
// ============================================================================

export async function getGroups(): Promise<string[]> {
  const res = await api.get('/api/group/')
  const data = res.data?.data
  if (Array.isArray(data)) return data.map(String)
  if (data && typeof data === 'object') return Object.keys(data)
  return []
}

// ============================================================================
// Rules
// ============================================================================

export async function getRules(
  params: GetRulesParams = {}
): Promise<GetRulesResponse> {
  const { p = 1, page_size = 10, keyword } = params
  const qs = new URLSearchParams({
    p: String(p),
    page_size: String(page_size),
  })
  if (keyword) qs.set('keyword', keyword)
  const res = await api.get(`/api/auto_group/rules?${qs.toString()}`)
  return res.data
}

export async function getRule(
  id: number
): Promise<ApiResponse<AutoGroupRule>> {
  const res = await api.get(`/api/auto_group/rules/${id}`)
  return res.data
}

export async function createRule(
  data: RuleFormData
): Promise<ApiResponse<AutoGroupRule>> {
  const res = await api.post('/api/auto_group/rules', data)
  return res.data
}

export async function updateRule(
  data: RuleFormData & { id: number }
): Promise<ApiResponse<AutoGroupRule>> {
  const res = await api.put(`/api/auto_group/rules/${data.id}`, data)
  return res.data
}

export async function deleteRule(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/auto_group/rules/${id}`)
  return res.data
}

// ============================================================================
// Enrollments
// ============================================================================

export async function getEnrollments(
  params: GetEnrollmentsParams = {}
): Promise<GetEnrollmentsResponse> {
  const { p = 1, page_size = 10, keyword } = params
  const qs = new URLSearchParams({
    p: String(p),
    page_size: String(page_size),
  })
  if (keyword) qs.set('keyword', keyword)
  const res = await api.get(`/api/auto_group/enrollments?${qs.toString()}`)
  return res.data
}

export async function enrollUsers(
  userIds: number[]
): Promise<ApiResponse<{ created: number; count: number; errors?: string[] }>> {
  const res = await api.post('/api/auto_group/enrollments', {
    user_ids: userIds,
  })
  return res.data
}

export async function unenrollUser(
  id: number
): Promise<ApiResponse<AutoGroupEnrollment>> {
  const res = await api.delete(`/api/auto_group/enrollments/${id}`)
  return res.data
}

// ============================================================================
// Sweep
// ============================================================================

export async function triggerSweep(): Promise<ApiResponse> {
  const res = await api.post('/api/auto_group/sweep')
  return res.data
}
