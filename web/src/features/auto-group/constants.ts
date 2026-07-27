// ============================================================================
// Metrics Configuration
// ============================================================================

export type MetricConfig = {
  key: string
  labelKey: string
  needsParam: boolean
}

export const METRICS: MetricConfig[] = [
  { key: 'total_spend', labelKey: 'Total Spend (USD)', needsParam: false },
  { key: 'recent_spend', labelKey: 'Recent N Hours Spend (USD)', needsParam: true },
  { key: 'yesterday_spend', labelKey: "Yesterday's Spend (USD)", needsParam: false },
  { key: 'total_topup', labelKey: 'Net Top-up (USD)', needsParam: false },
  { key: 'recent_topup', labelKey: 'Recent N Hours Top-up (USD)', needsParam: true },
  { key: 'yesterday_topup', labelKey: "Yesterday's Top-up (USD)", needsParam: false },
  { key: 'total_request_count', labelKey: 'Total Request Count', needsParam: false },
  { key: 'recent_request_count', labelKey: 'Recent N Hours Request Count', needsParam: true },
  { key: 'current_group', labelKey: 'Current Group', needsParam: false },
]

export const METRICS_MAP: Record<string, MetricConfig> = Object.fromEntries(
  METRICS.map((m) => [m.key, m])
)

export const STRING_METRICS = new Set(['current_group'])

// ============================================================================
// Operators
// ============================================================================

export const OPS = [
  { value: '>=', label: '>=' },
  { value: '>', label: '>' },
  { value: '<=', label: '<=' },
  { value: '<', label: '<' },
  { value: '==', label: '==' },
  { value: '!=', label: '!=' },
] as const

export const STRING_OPS = [
  { value: '==', label: '==' },
  { value: '!=', label: '!=' },
] as const

// ============================================================================
// Match Modes
// ============================================================================

export const MATCH_MODES = {
  ALL: 'all',
  ANY: 'any',
} as const

// ============================================================================
// Error Messages
// ============================================================================

export const ERROR_MESSAGES = {
  LOAD_RULES_FAILED: 'Failed to load rules',
  LOAD_ENROLLMENTS_FAILED: 'Failed to load enrollments',
  CREATE_RULE_FAILED: 'Failed to create rule',
  UPDATE_RULE_FAILED: 'Failed to update rule',
  DELETE_RULE_FAILED: 'Failed to delete rule',
  ENROLL_FAILED: 'Failed to enroll users',
  UNENROLL_FAILED: 'Failed to unenroll user',
  SWEEP_FAILED: 'Failed to trigger sweep',
} as const

// ============================================================================
// Success Messages
// ============================================================================

export const SUCCESS_MESSAGES = {
  RULE_CREATED: 'Rule created successfully',
  RULE_UPDATED: 'Rule updated successfully',
  RULE_DELETED: 'Rule deleted successfully',
  RULE_ENABLED: 'Rule enabled successfully',
  RULE_DISABLED: 'Rule disabled successfully',
  USERS_ENROLLED: 'Users enrolled successfully',
  USER_UNENROLLED: 'User unenrolled successfully',
  SWEEP_TRIGGERED: 'Sweep triggered successfully',
} as const
