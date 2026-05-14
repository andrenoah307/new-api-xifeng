import { api } from '@/lib/api'
import type {
  DiscountCode,
  ApiResponse,
  GetDiscountCodesParams,
  GetDiscountCodesResponse,
  SearchDiscountCodesParams,
  DiscountCodeFormData,
} from './types'

// ============================================================================
// Discount Code Management
// ============================================================================

// Get paginated discount codes list
export async function getDiscountCodes(
  params: GetDiscountCodesParams = {}
): Promise<GetDiscountCodesResponse> {
  const { p = 1, page_size = 10 } = params
  const res = await api.get(`/api/discount_code/?p=${p}&page_size=${page_size}`)
  return res.data
}

// Search discount codes by keyword
export async function searchDiscountCodes(
  params: SearchDiscountCodesParams
): Promise<GetDiscountCodesResponse> {
  const { keyword = '', p = 1, page_size = 10 } = params
  const res = await api.get(
    `/api/discount_code/search?keyword=${keyword}&p=${p}&page_size=${page_size}`
  )
  return res.data
}

// Get single discount code by ID
export async function getDiscountCode(
  id: number
): Promise<ApiResponse<DiscountCode>> {
  const res = await api.get(`/api/discount_code/${id}`)
  return res.data
}

// Create discount code(s)
export async function createDiscountCode(
  data: DiscountCodeFormData
): Promise<ApiResponse<string[]>> {
  const res = await api.post('/api/discount_code/', data)
  return res.data
}

// Update discount code
export async function updateDiscountCode(
  data: DiscountCodeFormData & { id: number }
): Promise<ApiResponse<DiscountCode>> {
  const res = await api.put('/api/discount_code/', data)
  return res.data
}

// Update discount code status (enable/disable)
export async function updateDiscountCodeStatus(
  id: number,
  status: number
): Promise<ApiResponse<DiscountCode>> {
  const res = await api.put('/api/discount_code/?status_only=true', {
    id,
    status,
  })
  return res.data
}

// Delete a single discount code
export async function deleteDiscountCode(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/discount_code/${id}/`)
  return res.data
}

// Validate a discount code (user-facing)
export async function validateDiscountCode(
  code: string
): Promise<ApiResponse<{ discount_rate: number; code: string }>> {
  const res = await api.post('/api/user/discount_code/validate', { code })
  return res.data
}
