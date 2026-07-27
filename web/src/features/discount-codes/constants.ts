import { type TFunction } from 'i18next'
import { type StatusBadgeProps } from '@/components/status-badge'

// ============================================================================
// Discount Code Status Configuration
// ============================================================================

export const DISCOUNT_CODE_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
} as const

export const DISCOUNT_CODE_STATUS_VALUES = Object.values(
  DISCOUNT_CODE_STATUS
).map((value) => String(value)) as `${number}`[]

// labelKey values are i18n keys; use t(config.labelKey) in components
export const DISCOUNT_CODE_STATUSES: Record<
  number,
  Pick<StatusBadgeProps, 'variant' | 'showDot'> & {
    labelKey: string
    value: number
  }
> = {
  [DISCOUNT_CODE_STATUS.ENABLED]: {
    labelKey: 'Enabled',
    variant: 'success',
    value: DISCOUNT_CODE_STATUS.ENABLED,
    showDot: true,
  },
  [DISCOUNT_CODE_STATUS.DISABLED]: {
    labelKey: 'Disabled',
    variant: 'neutral',
    value: DISCOUNT_CODE_STATUS.DISABLED,
    showDot: true,
  },
} as const

export function getDiscountCodeStatusOptions(t: TFunction) {
  return Object.values(DISCOUNT_CODE_STATUSES).map((config) => ({
    label: t(config.labelKey),
    value: String(config.value),
  }))
}

// ============================================================================
// Validation Constants
// ============================================================================

export const DISCOUNT_CODE_VALIDATION = {
  NAME_MAX_LENGTH: 100,
  RATE_MIN: 1,
  RATE_MAX: 99,
  COUNT_MIN: 1,
  COUNT_MAX: 100,
} as const

// ============================================================================
// Error Messages
// ============================================================================

export const ERROR_MESSAGES = {
  UNEXPECTED: 'An unexpected error occurred',
  LOAD_FAILED: 'Failed to load discount codes',
  SEARCH_FAILED: 'Failed to search discount codes',
  CREATE_FAILED: 'Failed to create discount code',
  UPDATE_FAILED: 'Failed to update discount code',
  DELETE_FAILED: 'Failed to delete discount code',
  STATUS_UPDATE_FAILED: 'Failed to update discount code status',
  RATE_INVALID: 'Discount rate must be between {{min}} and {{max}}',
  COUNT_INVALID: 'Count must be between {{min}} and {{max}}',
} as const

export function getDiscountCodeFormErrorMessages(t: TFunction) {
  return {
    RATE_INVALID: t(ERROR_MESSAGES.RATE_INVALID, {
      min: DISCOUNT_CODE_VALIDATION.RATE_MIN,
      max: DISCOUNT_CODE_VALIDATION.RATE_MAX,
    }),
    COUNT_INVALID: t(ERROR_MESSAGES.COUNT_INVALID, {
      min: DISCOUNT_CODE_VALIDATION.COUNT_MIN,
      max: DISCOUNT_CODE_VALIDATION.COUNT_MAX,
    }),
  } as const
}

// ============================================================================
// Success Messages
// ============================================================================

export const SUCCESS_MESSAGES = {
  DISCOUNT_CODE_CREATED: 'Discount code(s) created successfully',
  DISCOUNT_CODE_UPDATED: 'Discount code updated successfully',
  DISCOUNT_CODE_DELETED: 'Discount code deleted successfully',
  DISCOUNT_CODE_ENABLED: 'Discount code enabled successfully',
  DISCOUNT_CODE_DISABLED: 'Discount code disabled successfully',
  COPY_SUCCESS: 'Copied to clipboard',
} as const
