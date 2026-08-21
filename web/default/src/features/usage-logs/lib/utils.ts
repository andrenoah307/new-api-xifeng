/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
/**
 * Utility functions for usage logs feature
 */
import axios from 'axios'

import {
  LOG_TYPES,
  DISPLAYABLE_LOG_TYPES,
  TIMING_LOG_TYPES,
} from '../constants'
import type { GetLogsParams } from '../types'

export { buildQueryParams } from './query-params'

// ============================================================================
// Type Checkers & Utilities
// ============================================================================

/**
 * Check if log type is displayable (has detailed info)
 */
export function isDisplayableLogType(type: number): boolean {
  return (DISPLAYABLE_LOG_TYPES as readonly number[]).includes(type)
}

/**
 * Check if log type shows timing info
 */
export function isTimingLogType(type: number): boolean {
  return (TIMING_LOG_TYPES as readonly number[]).includes(type)
}

/**
 * Get log type configuration by type number
 */
export function getLogTypeConfig(type: number) {
  return LOG_TYPES.find((t) => t.value === type) || LOG_TYPES[0]
}

/**
 * Check if log uses per-call billing
 */
export function isPerCallBilling(modelPrice?: number): boolean {
  return (modelPrice ?? 0) > 0
}

/**
 * Get default time range (today 00:00:00 to today 23:59:59)
 */
export function getEndOfToday(): Date {
  const end = new Date()
  end.setHours(23, 59, 59, 999)
  return end
}

export function getDefaultTimeRange(): { start: Date; end: Date } {
  const now = new Date()
  const start = new Date(now)
  start.setHours(0, 0, 0, 0)
  const end = getEndOfToday()

  return { start, end }
}

/**
 * Convert milliseconds timestamp to seconds for API
 */
function timestampToSeconds(ms: number): number {
  return Math.floor(ms / 1000)
}

const LOG_STATS_PAGINATION_KEYS = new Set(['page', 'pageSize'])

export function getLogStatsSearchParams(
  searchParams: Record<string, unknown>
): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(searchParams).filter(
      ([key]) => !LOG_STATS_PAGINATION_KEYS.has(key)
    )
  )
}

export function buildLogStatsQueryKey(
  isAdmin: boolean,
  searchParams: Record<string, unknown>
) {
  return [
    'usage-logs-stats',
    isAdmin,
    getLogStatsSearchParams(searchParams),
  ] as const
}

const DEFAULT_USAGE_LOG_QUERY_TIMEOUT_MS = 35_000
const USAGE_LOG_QUERY_TIMEOUT_BUFFER_MS = 5_000
const MAX_USAGE_LOG_QUERY_TIMEOUT_SECONDS = 600
const MILLISECONDS_PER_SECOND = 1_000

export function resolveUsageLogQueryTimeoutMs(raw: unknown): number {
  let timeoutSeconds: number
  if (typeof raw === 'number') {
    timeoutSeconds = raw
  } else if (typeof raw === 'string' && raw.trim() !== '') {
    timeoutSeconds = Number(raw)
  } else {
    return DEFAULT_USAGE_LOG_QUERY_TIMEOUT_MS
  }

  if (!Number.isFinite(timeoutSeconds)) {
    return DEFAULT_USAGE_LOG_QUERY_TIMEOUT_MS
  }
  if (timeoutSeconds <= 0) {
    return 0
  }

  // The browser must give up after the backend so users receive its readable 503.
  return (
    Math.min(timeoutSeconds, MAX_USAGE_LOG_QUERY_TIMEOUT_SECONDS) *
      MILLISECONDS_PER_SECOND +
    USAGE_LOG_QUERY_TIMEOUT_BUFFER_MS
  )
}

export function shouldRetryUsageLogQuery(
  failureCount: number,
  error: unknown
): boolean {
  if (failureCount !== 0 || !axios.isAxiosError(error) || error.response) {
    return false
  }

  switch (error.code) {
    case 'ECONNABORTED':
    case 'ETIMEDOUT':
    case 'ERR_CANCELED':
      return false
    default:
      return true
  }
}

/**
 * Build time range parameters with default values
 * Shared logic for all log types
 */
function buildTimeRangeParams(
  searchParams: Record<string, unknown>,
  useMilliseconds: boolean
): { start_timestamp?: number; end_timestamp?: number } {
  const hasTimeParams = searchParams.startTime ?? searchParams.endTime
  const defaultTimeRange = !hasTimeParams ? getDefaultTimeRange() : null

  const convertTimestamp = (timestamp: number) =>
    useMilliseconds ? timestamp : timestampToSeconds(timestamp)

  const getTimestamp = (paramTime?: unknown, defaultTime?: Date) => {
    const time = (paramTime as number) || defaultTime?.getTime()
    return time ? convertTimestamp(time) : undefined
  }

  return {
    start_timestamp: getTimestamp(
      searchParams.startTime,
      defaultTimeRange?.start
    ),
    end_timestamp: getTimestamp(searchParams.endTime, defaultTimeRange?.end),
  }
}

/**
 * Build base parameters with time range (for drawing and task logs)
 * @param useMilliseconds - Whether to use millisecond timestamps (true for drawing logs, false for task logs)
 */
export function buildBaseParams(config: {
  page: number
  pageSize: number
  searchParams: Record<string, unknown>
  useMilliseconds?: boolean
  totalCount?: number
}): {
  p: number
  page_size: number
  channel_id?: string
  start_timestamp?: number
  end_timestamp?: number
  total_count?: number
} {
  const {
    page,
    pageSize,
    searchParams,
    useMilliseconds = false,
    totalCount,
  } = config

  return {
    p: page,
    page_size: pageSize,
    ...(searchParams.channel
      ? {
          channel_id: String(searchParams.channel),
        }
      : {}),
    ...buildTimeRangeParams(searchParams, useMilliseconds),
    ...(totalCount && totalCount > 0 && Number.isFinite(totalCount)
      ? { total_count: totalCount }
      : {}),
  }
}

/**
 * Build API params from search params and column filters (for common logs)
 */
export function buildApiParams(config: {
  page: number
  pageSize: number
  searchParams: Record<string, unknown>
  columnFilters?: Array<{ id: string; value: unknown }>
  isAdmin: boolean
  totalCount?: number
}): GetLogsParams {
  const {
    page,
    pageSize,
    searchParams,
    columnFilters = [],
    isAdmin,
    totalCount,
  } = config

  // Helper to process type parameter (single value from array)
  const processType = (value: unknown): number | undefined => {
    const parseType = (raw: unknown): number | undefined => {
      const type = Number(raw)
      return Number.isFinite(type) ? type : undefined
    }

    if (Array.isArray(value) && value.length === 1) {
      return parseType(value[0])
    }
    if (typeof value === 'string' && value !== '') {
      return parseType(value)
    }
    return undefined
  }

  // Build base params from search params
  const params: GetLogsParams = {
    p: page,
    page_size: pageSize,
    ...(searchParams.type ? { type: processType(searchParams.type) } : {}),
    ...(searchParams.model ? { model_name: String(searchParams.model) } : {}),
    ...(searchParams.token ? { token_name: String(searchParams.token) } : {}),
    ...(searchParams.group ? { group: String(searchParams.group) } : {}),
    ...(isAdmin && searchParams.channel
      ? { channel: Number(searchParams.channel) || 0 }
      : {}),
    ...(isAdmin && searchParams.username
      ? { username: String(searchParams.username) }
      : {}),
    ...(searchParams.requestId
      ? { request_id: String(searchParams.requestId) }
      : {}),
    ...(searchParams.upstreamRequestId
      ? { upstream_request_id: String(searchParams.upstreamRequestId) }
      : {}),
    ...buildTimeRangeParams(searchParams, false),
    ...(totalCount && totalCount > 0 && Number.isFinite(totalCount)
      ? { total_count: totalCount }
      : {}),
  }

  // Override with column filters if present
  if (columnFilters.length > 0) {
    columnFilters.forEach(({ id, value }) => {
      if (value === undefined || value === null || value === '') return

      switch (id) {
        case 'type':
          params.type = processType(value)
          break
        case 'model_name':
          params.model_name = String(value)
          break
        case 'token_name':
          params.token_name = String(value)
          break
        case 'group':
          params.group = String(value)
          break
        case 'channel':
          if (isAdmin) params.channel = Number(value) || 0
          break
        case 'username':
          if (isAdmin) params.username = String(value)
          break
      }
    })
  }

  return params
}

const PROXY_ID_PATTERNS = [
  /\s*\(request id: [^)]*\)/g,
  /\s*\(request_ori_id: [^)]*\)/g,
  /\s*（traceid: [^）]*）/g,
]

export function stripProxyIdSuffixes(msg: string | null | undefined): string {
  if (!msg) return msg ?? ''
  let result = msg
  for (const pattern of PROXY_ID_PATTERNS) {
    result = result.replace(pattern, '')
  }
  return result.trimEnd()
}

const LOCAL_REQUEST_ID_PATTERN = /\s*\(request id: [^)]*\)/g

export function stripLocalRequestId(msg: string | null | undefined): string {
  if (!msg) return msg ?? ''
  return msg.replace(LOCAL_REQUEST_ID_PATTERN, '').trimEnd()
}
