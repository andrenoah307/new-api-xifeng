import type { RiskCondition } from './api'

export interface MetricDefinition {
  labelKey: string
  value: string
  allowedScopes: string[]
}

export const METRIC_DEFINITIONS: MetricDefinition[] = [
  { labelKey: 'Distinct IPs (10m)', value: 'distinct_ip_10m', allowedScopes: ['token', 'user'] },
  { labelKey: 'Distinct IPs (1h)', value: 'distinct_ip_1h', allowedScopes: ['token', 'user'] },
  { labelKey: 'Distinct UAs (10m)', value: 'distinct_ua_10m', allowedScopes: ['token'] },
  { labelKey: 'Tokens per IP (10m)', value: 'tokens_per_ip_10m', allowedScopes: ['token'] },
  { labelKey: 'Requests (1m)', value: 'request_count_1m', allowedScopes: ['token', 'user'] },
  { labelKey: 'Requests (10m)', value: 'request_count_10m', allowedScopes: ['token', 'user'] },
  { labelKey: 'Inflight Now', value: 'inflight_now', allowedScopes: ['token', 'user'] },
  { labelKey: 'Rule Hits (24h)', value: 'rule_hit_count_24h', allowedScopes: ['token', 'user'] },
  { labelKey: 'Risk Score', value: 'risk_score', allowedScopes: ['token', 'user'] },
]

export const METRIC_LABEL_MAP: Record<string, string> = Object.fromEntries(
  METRIC_DEFINITIONS.map((d) => [d.value, d.labelKey])
)

export const METRIC_SCOPE_MAP: Record<string, string[]> = Object.fromEntries(
  METRIC_DEFINITIONS.map((d) => [d.value, d.allowedScopes])
)

export const OP_OPTIONS = [
  { label: '>=', value: '>=' },
  { label: '>', value: '>' },
  { label: '<=', value: '<=' },
  { label: '<', value: '<' },
  { label: '=', value: '==' },
  { label: '!=', value: '!=' },
]

export function getMetricOptionsForScope(scope: string) {
  return METRIC_DEFINITIONS
    .filter((d) => d.allowedScopes.includes(scope))
    .map(({ labelKey, value }) => ({ label: labelKey, value }))
}

export function getDefaultMetricForScope(scope: string) {
  return getMetricOptionsForScope(scope)[0]?.value || 'distinct_ip_10m'
}

export function isMetricAllowedForScope(metric: string, scope: string) {
  return (METRIC_SCOPE_MAP[metric] ?? []).includes(scope)
}

export function sanitizeConditionsForScope(
  conditions: RiskCondition[],
  scope: string
) {
  const fallback = getDefaultMetricForScope(scope)
  let changed = false
  const next = (conditions ?? []).map((c) => {
    if (isMetricAllowedForScope(c.metric, scope)) return c
    changed = true
    return { ...c, metric: fallback }
  })
  return { changed, conditions: next }
}

export function safeParseJSON<T>(value: unknown, fallback: T): T {
  if (!value) return fallback
  if (Array.isArray(value)) return value as T
  if (typeof value === 'string') {
    try {
      return JSON.parse(value) as T
    } catch {
      return fallback
    }
  }
  return fallback
}

export function emptyRuleForm() {
  return {
    id: 0,
    name: '',
    description: '',
    enabled: false,
    scope: 'token',
    detector: 'distribution',
    match_mode: 'all',
    priority: 50,
    action: 'observe',
    auto_block: false,
    auto_recover: true,
    recover_mode: 'ttl',
    recover_after_seconds: 900,
    response_status_code: 429,
    response_message: 'Request triggered risk control, please try again later',
    score_weight: 20,
    conditions: [{ metric: 'distinct_ip_10m', op: '>=', value: 3 }] as RiskCondition[],
    groups: [] as string[],
  }
}

export function emptyModerationRuleForm() {
  return {
    id: 0,
    name: '',
    description: '',
    enabled: false,
    match_mode: 'or',
    conditions: [{ category: '', op: '>=', value: 0.5 }],
    action: 'block',
    action_params: '',
    priority: 50,
  }
}

export const SUBJECT_STATUS_MAP: Record<
  string,
  { labelKey: string; variant: string }
> = {
  blocked: { labelKey: 'Blocked', variant: 'danger' },
  observe: { labelKey: 'Observing', variant: 'warning' },
  normal: { labelKey: 'Normal', variant: 'neutral' },
}

export const DECISION_MAP: Record<
  string,
  { labelKey: string; variant: string }
> = {
  block: { labelKey: 'Block', variant: 'danger' },
  observe: { labelKey: 'Observe', variant: 'warning' },
  allow: { labelKey: 'Allow', variant: 'neutral' },
}

export const SCOPE_OPTIONS = [
  { label: 'Token', value: 'token' },
  { label: 'User', value: 'user' },
]

export const MODE_OPTIONS = [
  { label: 'Enforce', value: 'enforce' },
  { label: 'Observe Only', value: 'observe_only' },
]

export const ACTION_OPTIONS = [
  { label: 'Block', value: 'block' },
  { label: 'Observe', value: 'observe' },
]

export const MATCH_MODE_OPTIONS = [
  { label: 'All (AND)', value: 'all' },
  { label: 'Any (OR)', value: 'any' },
]

export const MODERATION_ACTION_OPTIONS = [
  { label: 'Block', value: 'block' },
  { label: 'Observe', value: 'observe' },
  { label: 'Flag', value: 'flag' },
]

export const ENFORCEMENT_SOURCE_OPTIONS = [
  { label: 'Risk Distribution', value: 'risk_distribution' },
  { label: 'Content Moderation', value: 'moderation' },
]

export const ENFORCEMENT_ACTION_OPTIONS = [
  { label: 'Email Reminder', value: 'email' },
  { label: 'Counter Increment', value: 'counter' },
  { label: 'Auto Ban', value: 'auto_ban' },
]

export const riskQueryKeys = {
  all: ['risk'] as const,
  overview: () => [...riskQueryKeys.all, 'overview'] as const,
  config: () => [...riskQueryKeys.all, 'config'] as const,
  rules: () => [...riskQueryKeys.all, 'rules'] as const,
  subjects: (params: Record<string, unknown>) =>
    [...riskQueryKeys.all, 'subjects', params] as const,
  incidents: (params: Record<string, unknown>) =>
    [...riskQueryKeys.all, 'incidents', params] as const,
  groups: () => [...riskQueryKeys.all, 'groups'] as const,
  moderation: {
    all: ['risk', 'moderation'] as const,
    config: () => ['risk', 'moderation', 'config'] as const,
    overview: () => ['risk', 'moderation', 'overview'] as const,
    rules: () => ['risk', 'moderation', 'rules'] as const,
    categories: () => ['risk', 'moderation', 'categories'] as const,
    incidents: (params: Record<string, unknown>) =>
      ['risk', 'moderation', 'incidents', params] as const,
    queueStats: () => ['risk', 'moderation', 'queueStats'] as const,
  },
  enforcement: {
    all: ['risk', 'enforcement'] as const,
    config: () => ['risk', 'enforcement', 'config'] as const,
    overview: () => ['risk', 'enforcement', 'overview'] as const,
    counters: (params: Record<string, unknown>) =>
      ['risk', 'enforcement', 'counters', params] as const,
    incidents: (params: Record<string, unknown>) =>
      ['risk', 'enforcement', 'incidents', params] as const,
  },
}
