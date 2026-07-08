import { z } from 'zod'

// ============================================================================
// Condition Schema & Types
// ============================================================================

export const autoGroupConditionSchema = z.object({
  metric: z.string(),
  op: z.string(),
  value: z.number(),
  value_str: z.string().optional(),
  param: z.number().optional(),
})

export type AutoGroupCondition = z.infer<typeof autoGroupConditionSchema>

// ============================================================================
// Rule Schema & Types
// ============================================================================

export const autoGroupRuleSchema = z.object({
  id: z.number(),
  name: z.string(),
  description: z.string(),
  enabled: z.boolean(),
  priority: z.number(),
  target_group: z.string(),
  match_mode: z.enum(['all', 'any']),
  conditions: z.string(),
  created_at: z.number(),
  updated_at: z.number(),
  created_by: z.number(),
  updated_by: z.number(),
})

export type AutoGroupRule = z.infer<typeof autoGroupRuleSchema>

// ============================================================================
// Enrollment Schema & Types
// ============================================================================

export const autoGroupEnrollmentSchema = z.object({
  id: z.number(),
  user_id: z.number(),
  original_group: z.string(),
  current_group: z.string(),
  current_rule_id: z.number(),
  enrolled_at: z.number(),
  updated_at: z.number(),
  enrolled_by: z.number(),
  username: z.string(),
  display_name: z.string(),
})

export type AutoGroupEnrollment = z.infer<typeof autoGroupEnrollmentSchema>

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface GetRulesParams {
  p?: number
  page_size?: number
  keyword?: string
}

export interface GetRulesResponse {
  success: boolean
  message?: string
  data?: {
    items: AutoGroupRule[]
    total: number
    page: number
    page_size: number
  }
}

export interface GetEnrollmentsParams {
  p?: number
  page_size?: number
  keyword?: string
}

export interface GetEnrollmentsResponse {
  success: boolean
  message?: string
  data?: {
    items: AutoGroupEnrollment[]
    total: number
    page: number
    page_size: number
  }
}

// ============================================================================
// Form Data Types
// ============================================================================

export interface RuleFormData {
  id?: number
  name: string
  description: string
  enabled: boolean
  priority: number
  target_group: string
  match_mode: 'all' | 'any'
  conditions: AutoGroupCondition[]
}

export interface EnrollFormData {
  user_ids: number[]
}

// ============================================================================
// Dialog Types
// ============================================================================

export type AutoGroupDialogType =
  | 'create-rule'
  | 'update-rule'
  | 'delete-rule'
  | 'enroll'
  | 'unenroll'
