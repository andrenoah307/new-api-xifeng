import { z } from 'zod'

// ============================================================================
// Discount Code Schema & Types
// ============================================================================

export const discountCodeSchema = z.object({
  id: z.number(),
  code: z.string(),
  name: z.string(),
  discount_rate: z.number(), // 1-99, e.g. 90 = pay 90%
  start_time: z.number(), // unix timestamp, 0 = no limit
  end_time: z.number(), // unix timestamp, 0 = no limit
  max_uses_total: z.number(), // 0 = unlimited
  max_uses_per_user: z.number(), // 0 = unlimited
  used_count: z.number(),
  status: z.number(), // 1=enabled, 2=disabled
  created_time: z.number(),
})

export type DiscountCode = z.infer<typeof discountCodeSchema>

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface GetDiscountCodesParams {
  p?: number
  page_size?: number
}

export interface GetDiscountCodesResponse {
  success: boolean
  message?: string
  data?: {
    items: DiscountCode[]
    total: number
    page: number
    page_size: number
  }
}

export interface SearchDiscountCodesParams {
  keyword?: string
  p?: number
  page_size?: number
}

export interface DiscountCodeFormData {
  id?: number
  name?: string
  code?: string
  discount_rate: number
  start_time: number
  end_time: number
  max_uses_per_user: number
  max_uses_total: number
  count?: number // Only for create
  status?: number // Only for status update
}

// ============================================================================
// Dialog Types
// ============================================================================

export type DiscountCodesDialogType = 'create' | 'update' | 'delete'
