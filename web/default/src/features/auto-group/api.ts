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
  const res = await api.get(`/api/auto_group_rule/?${qs.toString()}`)
  return res.data
}

export async function getRule(
  id: number
): Promise<ApiResponse<AutoGroupRule>> {
  const res = await api.get(`/api/auto_group_rule/${id}`)
  return res.data
}

export async function createRule(
  data: RuleFormData
): Promise<ApiResponse<AutoGroupRule>> {
  const res = await api.post('/api/auto_group_rule/', data)
  return res.data
}

export async function updateRule(
  data: RuleFormData & { id: number }
): Promise<ApiResponse<AutoGroupRule>> {
  const res = await api.put('/api/auto_group_rule/', data)
  return res.data
}

export async function deleteRule(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/auto_group_rule/${id}`)
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
  const res = await api.get(`/api/auto_group_enrollment/?${qs.toString()}`)
  return res.data
}

export async function enrollUsers(
  userIds: number[]
): Promise<ApiResponse<{ created: number; count: number; errors?: string[] }>> {
  const res = await api.post('/api/auto_group_enrollment/', {
    user_ids: userIds,
  })
  return res.data
}

export async function unenrollUser(
  id: number
): Promise<ApiResponse<AutoGroupEnrollment>> {
  const res = await api.delete(`/api/auto_group_enrollment/${id}`)
  return res.data
}

// ============================================================================
// Sweep
// ============================================================================

export async function triggerSweep(): Promise<ApiResponse> {
  const res = await api.post('/api/auto_group_enrollment/sweep')
  return res.data
}
