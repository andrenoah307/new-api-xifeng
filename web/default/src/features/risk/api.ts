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
  default_recover_after_seconds: number
  trusted_ip_header: string
  async_event_engine: boolean
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
  type: string
  id: string | number
  status: string
  risk_score: number
  last_rule: string
  last_hit_at: number
  blocked_at: number
  group: string
}

export interface RiskIncident {
  id: number
  scope: string
  subject_type: string
  subject_id: string | number
  rule_name: string
  decision: string
  metric_snapshot: string
  group: string
  created_at: number
}

export interface RiskGroup {
  name: string
  enabled: boolean
  mode: string
  effective_mode: string
  rule_count: number
  subject_count: number
}

export interface IPDiagnosis {
  ip: string
  country: string
  region: string
  city: string
  isp: string
  is_proxy: boolean
  is_vpn: boolean
  is_tor: boolean
  is_datacenter: boolean
  risk_level: string
}

// ─── Moderation Types ───

export interface ModerationConfig {
  enabled: boolean
  mode: string
  base_url: string
  model: string
  api_keys: string
  sampling_rate: number
  queue_size: number
  worker_count: number
  http_timeout: number
  retries: number
  retention_hours: number
  record_unmatched_inputs: boolean
  groups: Record<string, { enabled: boolean; mode: string }>
}

export interface ModerationOverview {
  enabled: boolean
  api_key_count: number
  flagged_24h: number
  event_drop_count: number
}

export interface ModerationRule {
  id: number
  name: string
  description: string
  enabled: boolean
  logic: string
  conditions: ModerationRuleCondition[] | string
  action: string
  action_params: string
  priority: number
  created_at: number
  updated_at: number
}

export interface ModerationRuleCondition {
  category: string
  op: string
  threshold: number
}

export interface ModerationIncident {
  id: number
  request_id: string
  user_id: number
  token_id: number
  model: string
  group: string
  input_summary: string
  flagged: boolean
  categories: string
  scores: string
  matched_rules: string
  action_taken: string
  created_at: number
}

export interface ModerationCategory {
  key: string
  label: string
}

export interface ModerationQueueStats {
  memory_queue_depth: number
  redis_queue_depth: number
  worker_state: string
  drop_count: number
  incident_batcher: string
}

export interface ModerationDebugResult {
  status: string
  flagged: boolean
  categories: Record<string, number>
  matched_rules: string[]
  error?: string
}

// ─── Enforcement Types ───

export interface EnforcementConfig {
  enabled: boolean
  email_enabled: boolean
  email_on_risk_hit: boolean
  email_on_moderation_hit: boolean
  email_on_auto_ban: boolean
  enabled_sources: string[]
  count_window_hours: number
  ban_threshold_default: number
  ban_threshold_risk_distribution: number
  ban_threshold_moderation: number
  email_rate_limit_minutes: number
  email_template_subject: string
  email_template_body: string
}

export interface EnforcementOverview {
  enabled: boolean
  hits_24h: number
  auto_bans_24h: number
  email_reminder_enabled: boolean
}

export interface EnforcementCounter {
  user_id: number
  username: string
  total_hits: number
  risk_distribution_hits: number
  moderation_hits: number
  banned: boolean
  banned_at: number
  last_hit_at: number
}

export interface EnforcementIncident {
  id: number
  user_id: number
  username: string
  source: string
  action: string
  detail: string
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
  return (res.data?.data ?? []) as RiskGroup[]
}

export async function detectIP(ip: string) {
  const res = await api.get('/api/risk/detect-ip', { params: { ip } })
  return res.data?.data as IPDiagnosis
}

// ─── Moderation API ───

export async function getModerationConfig() {
  const res = await api.get('/api/risk/moderation/config')
  return res.data?.data as ModerationConfig
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
  return res.data?.data as ModerationDebugResult
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
