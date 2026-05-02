import { api } from '@/lib/api'

export interface MonitoringGroup {
  group_name: string
  is_online: boolean
  online_channels: number
  total_channels: number
  availability_rate: number | null
  cache_hit_rate: number | null
  avg_frt: number | null
  avg_response_time: number | null
  first_response_time: number | null
  last_test_model: string
  group_ratio: number | null
  updated_at: number
}

export interface MonitoringGroupWithHistory extends MonitoringGroup {
  history: MonitoringHistoryPoint[]
  aggregation_interval_minutes: number
}

export interface MonitoringHistoryPoint {
  recorded_at: number
  availability_rate: number | null
  avg_frt: number | null
  cache_hit_rate: number | null
}

export interface ChannelStat {
  channel_id: number
  channel_name: string
  channel_status: number
  enabled: boolean
  availability_rate: number | null
  cache_hit_rate: number | null
  first_response_time: number | null
  avg_frt: number | null
  avg_response_time: number | null
  test_model: string
  last_test_time: string | null
  is_online: boolean
}

export interface GroupDetail extends MonitoringGroup {
  channel_stats: ChannelStat[]
}

export interface HistoryResponse {
  history?: MonitoringHistoryPoint[]
  data?: MonitoringHistoryPoint[]
  aggregation_interval_minutes?: number
}

export async function getMonitoringGroups(
  admin: boolean
): Promise<MonitoringGroup[]> {
  const prefix = admin ? 'admin' : 'public'
  const res = await api.get(`/api/monitoring/${prefix}/groups`)
  return res.data.data ?? []
}

export async function getGroupHistory(
  groupName: string,
  admin: boolean
): Promise<{ history: MonitoringHistoryPoint[]; intervalMinutes: number }> {
  const prefix = admin ? 'admin' : 'public'
  const res = await api.get(
    `/api/monitoring/${prefix}/groups/${encodeURIComponent(groupName)}/history`
  )
  const data = res.data.data
  const history = data?.history ?? data ?? []
  const intervalMinutes = data?.aggregation_interval_minutes ?? 5
  return { history, intervalMinutes }
}

export async function getGroupDetail(groupName: string): Promise<GroupDetail> {
  const res = await api.get(
    `/api/monitoring/admin/groups/${encodeURIComponent(groupName)}`
  )
  return res.data.data
}

export async function refreshMonitoringData(): Promise<void> {
  await api.post('/api/monitoring/admin/refresh')
}
