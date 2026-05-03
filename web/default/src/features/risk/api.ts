import { api } from '@/lib/api'

// ─── Distribution Detection Types ───

export interface RiskOverview {
  observed_subjects: number
  blocked_subjects: number
  high_risk_subjects: number
  rule_count: number
  enabled_group_count: number
  unconfigured_rule_count: number
  group_unlisted_rule_count: number
}

export interface RiskConfig {
  enabled: boolean
  mode: string
  default_status_code: number
  default_response_message: string
  default_recover_after_secs: number
  default_recover_mode: string
  trusted_ip_header: string
  trusted_ip_header_enabled: boolean
  enabled_groups: string[]
  group_modes: Record<string, string>
  event_queue_size: number
  worker_count: number
}

export interface RiskCondition {
  metric: string
  op: string
  value: number
}

export interface RiskRule {
  id: number
  name: string
  description: string
  enabled: boolean
  scope: string
  detector: string
  match_mode: string
  priority: number
  action: string
  auto_block: boolean
  auto_recover: boolean
  recover_mode: string
  recover_after_seconds: number
  response_status_code: number
  response_message: string
  score_weight: number
  conditions: RiskCondition[] | string
  groups: string[] | string
  created_at: number
  updated_at: number
}

export interface RiskSubject {
  id: number
  subject_type: string
  subject_id: number
  user_id: number
  token_id: number
  username: string
  token_name: string
  token_masked_key: string
  group: string
  status: string
  risk_score: number
  active_rule_names: string
  last_rule_name: string
  last_seen_at: number
  block_until: number
}

export interface RiskIncident {
  id: number
  subject_type: string
  subject_id: number
  user_id: number
  token_id: number
  username: string
  rule_name: string
  decision: string
  action: string
  snapshot: string
  group: string
  risk_score: number
  created_at: number
}

export interface RiskGroup {
  name: string
  enabled: boolean
  mode: string
  effective_mode: string
  rule_count_total: number
  rule_count_enabled: number
  active_subject_count: number
  blocked_subject_count: number
  high_risk_subject_count: number
}

export interface IPDiagnosis {
  current_mode: string
  current_header: string
  effective_client_ip: string
  effective_source: string
  recommended_mode: string
  recommended_header: string
  recommendation_message: string
  items: { header: string; value: string; source: string }[]
}

// ─── Moderation Types ───

export interface ModerationConfigPayload {
  config: ModerationConfig
  key_count: number
}

export interface ModerationConfig {
  enabled: boolean
  mode: string
  base_url: string
  model: string
  api_keys: string[]
  sampling_rate_percent: number
  event_queue_size: number
  worker_count: number
  http_timeout_ms: number
  max_retries: number
  flagged_retention_hours: number
  record_unmatched_inputs: boolean
  enabled_groups: string[]
  group_modes: Record<string, string>
}

export interface ModerationOverview {
  enabled: boolean
  key_count: number
  flagged_24h: number
  queue_dropped: number
  mode: string
  sampling_rate_percent: number
  enabled_group_count: number
  rule_count: number
}

export interface ModerationRule {
  id: number
  name: string
  description: string
  enabled: boolean
  match_mode: string
  conditions: ModerationRuleCondition[] | string
  action: string
  score_weight: number
  groups: string | string[]
  priority: number
  created_at: number
  updated_at: number
}

export interface ModerationRuleCondition {
  category: string
  op: string
  value: number
}

export interface ModerationIncident {
  id: number
  request_id: string
  user_id: number
  token_id: number
  username: string
  model: string
  group: string
  input_summary: string
  flagged: boolean
  categories: string
  max_score: number
  max_category: string
  matched_rules: string
  decision: string
  created_at: number
}

export interface ModerationCategory {
  name: string
  label: string
  image_scored: boolean
}

export interface ModerationWorkerState {
  id: number
  state: string
  since: number
  last_event_at: number
}

export interface ModerationQueueStats {
  queue_depth_memory: number
  queue_depth_redis: number
  redis_available: boolean
  worker_count: number
  worker_state: ModerationWorkerState[]
  drop_count_total: number
}

export interface ModerationDebugPayload {
  request_id: string
  pending: boolean
  result?: ModerationDebugResult
}

export interface ModerationDebugResult {
  flagged: boolean
  categories: Record<string, number>
  error?: string
  decision?: {
    decision: string
    primary_rule_name: string
    matched_rules: { name: string; action: string }[]
    reason?: string
  }
}

// ─── Enforcement Types ───

export interface EnforcementConfig {
  enabled: boolean
  email_on_hit: boolean
  email_on_auto_ban: boolean
  count_window_hours: number
  ban_threshold: number
  ban_threshold_per_source: Record<string, number>
  enabled_sources: string[]
  email_hit_subject: string
  email_hit_template: string
  email_ban_subject: string
  email_ban_template: string
  hit_email_window_minutes: number
  hit_email_max_per_window: number
}

export interface EnforcementOverview {
  enabled: boolean
  hits_24h: number
  auto_bans_24h: number
  email_on_hit: boolean
  email_on_autoban: boolean
  ban_threshold: number
}

export interface EnforcementCounter {
  id: number
  username: string
  email: string
  enforcement_hit_count_risk: number
  enforcement_hit_count_moderation: number
  enforcement_auto_banned_at: number
  enforcement_last_hit_at: number
}

export interface EnforcementIncident {
  id: number
  user_id: number
  username: string
  source: string
  action: string
  reason: string
  group: string
  hit_count_after: number
  threshold: number
  created_at: number
}

// ─── Distribution Detection API ───

export async function getRiskOverview() {
  const res = await api.get('/api/risk/overview')
  return res.data?.data as RiskOverview
}

export async function getRiskConfig() {
  const res = await api.get('/api/risk/config')
  return res.data?.data as RiskConfig
}

export async function saveRiskConfig(config: Partial<RiskConfig>) {
  const res = await api.put('/api/risk/config', config)
  return res.data
}

export async function getRiskRules() {
  const res = await api.get('/api/risk/rules')
  return (res.data?.data ?? []) as RiskRule[]
}

export async function createRiskRule(rule: Partial<RiskRule>) {
  const res = await api.post('/api/risk/rules', rule)
  return res.data
}

export async function updateRiskRule(id: number, rule: Partial<RiskRule>) {
  const res = await api.put(`/api/risk/rules/${id}`, rule)
  return res.data
}

export async function deleteRiskRule(id: number) {
  const res = await api.delete(`/api/risk/rules/${id}`)
  return res.data
}

export async function getRiskSubjects(params: {
  p: number
  page_size: number
  scope?: string
  status?: string
  keyword?: string
}) {
  const res = await api.get('/api/risk/subjects', { params })
  return res.data?.data as { items: RiskSubject[]; total: number }
}

export async function getRiskIncidents(params: {
  p: number
  page_size: number
  scope?: string
  action?: string
  keyword?: string
}) {
  const res = await api.get('/api/risk/incidents', { params })
  return res.data?.data as { items: RiskIncident[]; total: number }
}

export async function unblockSubject(
  type: string,
  id: string | number,
  group: string
) {
  const res = await api.post(
    `/api/risk/subjects/${type}/${id}/unblock`,
    null,
    { params: { group } }
  )
  return res.data
}

export async function getRiskGroups() {
  const res = await api.get('/api/risk/groups')
  const payload = res.data?.data as
    | { schema_version: number; global_mode: string; items: RiskGroup[] }
    | undefined
  return (payload?.items ?? []) as RiskGroup[]
}

export async function detectIP(ip: string) {
  const res = await api.get('/api/risk/detect-ip', { params: { ip } })
  return res.data?.data as IPDiagnosis
}

// ─── Moderation API ───

export async function getModerationConfig() {
  const res = await api.get('/api/risk/moderation/config')
  const payload = res.data?.data as ModerationConfigPayload | undefined
  return payload
}

export async function saveModerationConfig(config: Partial<ModerationConfig>) {
  const res = await api.put('/api/risk/moderation/config', config)
  return res.data
}

export async function getModerationOverview() {
  const res = await api.get('/api/risk/moderation/overview')
  return res.data?.data as ModerationOverview
}

export async function getModerationIncidents(params: {
  p: number
  page_size: number
  group?: string
  flagged?: string
  keyword?: string
}) {
  const res = await api.get('/api/risk/moderation/incidents', { params })
  return res.data?.data as { items: ModerationIncident[]; total: number }
}

export async function getModerationIncidentDetail(id: number) {
  const res = await api.get(`/api/risk/moderation/incidents/${id}`)
  return res.data?.data as ModerationIncident
}

export async function getModerationRules() {
  const res = await api.get('/api/risk/moderation/rules')
  return (res.data?.data ?? []) as ModerationRule[]
}

export async function createModerationRule(rule: Partial<ModerationRule>) {
  const res = await api.post('/api/risk/moderation/rules', rule)
  return res.data
}

export async function updateModerationRule(
  id: number,
  rule: Partial<ModerationRule>
) {
  const res = await api.put(`/api/risk/moderation/rules/${id}`, rule)
  return res.data
}

export async function deleteModerationRule(id: number) {
  const res = await api.delete(`/api/risk/moderation/rules/${id}`)
  return res.data
}

export async function getModerationCategories() {
  const res = await api.get('/api/risk/moderation/categories')
  return (res.data?.data ?? []) as ModerationCategory[]
}

export async function runModerationDebug(body: {
  text?: string
  images?: string[]
  group?: string
}) {
  const res = await api.post('/api/risk/moderation/debug', body)
  return res.data?.data as { request_id: string }
}

export async function getModerationDebugResult(requestId: string) {
  const res = await api.get(`/api/risk/moderation/debug/${requestId}`)
  return res.data?.data as ModerationDebugPayload
}

export async function getModerationQueueStats() {
  const res = await api.get('/api/risk/moderation/queue_stats')
  return res.data?.data as ModerationQueueStats
}

// ─── Enforcement API ───

export async function getEnforcementConfig() {
  const res = await api.get('/api/risk/enforcement/config')
  return res.data?.data as EnforcementConfig
}

export async function saveEnforcementConfig(
  config: Partial<EnforcementConfig>
) {
  const res = await api.put('/api/risk/enforcement/config', config)
  return res.data
}

export async function getEnforcementOverview() {
  const res = await api.get('/api/risk/enforcement/overview')
  return res.data?.data as EnforcementOverview
}

export async function getEnforcementCounters(params: {
  p: number
  page_size: number
  keyword?: string
}) {
  const res = await api.get('/api/risk/enforcement/counters', { params })
  return res.data?.data as { items: EnforcementCounter[]; total: number }
}

export async function getEnforcementIncidents(params: {
  p: number
  page_size: number
  source?: string
  action?: string
  keyword?: string
}) {
  const res = await api.get('/api/risk/enforcement/incidents', { params })
  return res.data?.data as { items: EnforcementIncident[]; total: number }
}

export async function resetEnforcementCounter(uid: number) {
  const res = await api.post(
    `/api/risk/enforcement/users/${uid}/reset_counter`
  )
  return res.data
}

export async function unbanUser(uid: number) {
  const res = await api.post(`/api/risk/enforcement/users/${uid}/unban`)
  return res.data
}

export async function sendTestEmail() {
  const res = await api.post('/api/risk/enforcement/test_email')
  return res.data
}
