/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

import { stripProxyIdSuffixes } from '@/features/usage-logs/lib/utils'

export type FilterAction = 'retry' | 'rewrite' | 'replace'

export interface FilterRule {
  status_codes: number[]
  message_contains: string[]
  error_codes: string[]
  action: FilterAction
  rewrite_message: string
  replace_status_code: number
  replace_message: string
}

export interface ErrorLogInput {
  id?: number | string
  created_at?: number | string | null
  content?: unknown
  model_name?: unknown
  other?: unknown
}

export interface ParsedErrorLog {
  id?: number | string
  createdAt?: number | string | null
  content: string
  modelName: string
  statusCode: number | null
  errorCode: string
  errorType: string
}

const ERROR_FILTER_ACTIONS: ReadonlySet<FilterAction> = new Set([
  'retry',
  'rewrite',
  'replace',
])

const DEFAULT_REPLACE_STATUS_CODE = 200

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function asStatusCodeList(input: unknown): unknown[] {
  if (Array.isArray(input)) return input
  if (typeof input === 'string') {
    return input.split(/[\s,，]+/u)
  }
  return []
}

function asText(value: unknown): string {
  return String(value || '')
}

function parseInteger(value: unknown): number | null {
  if (typeof value === 'number') {
    return Number.isSafeInteger(value) ? value : null
  }

  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  if (!/^[+-]?\d+$/u.test(trimmed)) return null

  const parsed = Number(trimmed)
  return Number.isSafeInteger(parsed) ? parsed : null
}

export function createEmptyRule(): FilterRule {
  return {
    status_codes: [],
    message_contains: [],
    error_codes: [],
    action: 'retry',
    rewrite_message: '',
    replace_status_code: DEFAULT_REPLACE_STATUS_CODE,
    replace_message: '',
  }
}

export function normalizeStringList(input: unknown): string[] {
  const values = Array.isArray(input) ? input : []
  const normalized: string[] = []
  const seen = new Set<string>()

  for (const value of values) {
    const text = asText(value).trim()
    if (!text || seen.has(text)) continue
    seen.add(text)
    normalized.push(text)
  }

  return normalized
}

export function normalizeStatusCodes(input: unknown): number[] {
  const normalized: number[] = []
  const seen = new Set<number>()

  for (const value of asStatusCodeList(input).flatMap((item) => {
    if (typeof item !== 'string') return [item]
    return item.split(/[\s,，]+/u)
  })) {
    const parsed = parseInteger(value)
    if (parsed === null || parsed < 100 || parsed > 599 || seen.has(parsed)) {
      continue
    }
    seen.add(parsed)
    normalized.push(parsed)
  }

  return normalized
}

export function normalizeRule(raw: unknown = {}): FilterRule {
  const source = isRecord(raw) ? raw : {}
  const action = ERROR_FILTER_ACTIONS.has(source.action as FilterAction)
    ? (source.action as FilterAction)
    : 'retry'
  const replaceStatusCode = parseInteger(source.replace_status_code)

  return {
    status_codes: normalizeStatusCodes(source.status_codes),
    message_contains: normalizeStringList(source.message_contains),
    error_codes: normalizeStringList(source.error_codes),
    action,
    rewrite_message: asText(source.rewrite_message),
    replace_status_code:
      replaceStatusCode !== null && replaceStatusCode >= 100
        ? replaceStatusCode
        : DEFAULT_REPLACE_STATUS_CODE,
    replace_message: asText(source.replace_message),
  }
}

export function parseRules(value: string | null | undefined): FilterRule[] {
  if (typeof value !== 'string' || !value.trim()) return []

  try {
    const parsed: unknown = JSON.parse(value)
    if (!Array.isArray(parsed)) return []
    return parsed.map((rule) => normalizeRule(rule))
  } catch {
    return []
  }
}

export function stripErrorContentPrefix(content: unknown): string {
  if (typeof content !== 'string' || !content) return ''
  const withoutStatusPrefix = content.replace(/^status_code=\d+,\s*/u, '')
  return stripProxyIdSuffixes(withoutStatusPrefix).trim()
}

export function parseErrorLog(
  log: ErrorLogInput | null | undefined
): ParsedErrorLog {
  const source = log ?? {}
  let otherData: Record<string, unknown> = {}

  if (typeof source.other === 'string' && source.other) {
    try {
      const parsed: unknown = JSON.parse(source.other)
      if (isRecord(parsed)) otherData = parsed
    } catch {
      otherData = {}
    }
  } else if (isRecord(source.other)) {
    otherData = source.other
  }

  const parsedStatusCode = parseInteger(otherData.status_code)
  const statusCode = parsedStatusCode || null
  const errorCode = otherData.error_code ? String(otherData.error_code) : ''
  const errorType = otherData.error_type ? String(otherData.error_type) : ''

  return {
    id: source.id,
    createdAt: source.created_at,
    content: source.content ? String(source.content) : '',
    modelName: source.model_name ? String(source.model_name) : '',
    statusCode,
    errorCode,
    errorType,
  }
}

export function deduplicateErrors(
  logs: readonly ParsedErrorLog[]
): ParsedErrorLog[] {
  const seen = new Set<string>()
  const deduplicated: ParsedErrorLog[] = []

  for (const log of logs) {
    const key = `${log.statusCode}|${log.errorCode}|${log.content}`
    if (seen.has(key)) continue
    seen.add(key)
    deduplicated.push(log)
  }

  return deduplicated
}

export function hasCondition(
  rule: Partial<FilterRule> | null | undefined
): boolean {
  if (!rule) return false
  if (Array.isArray(rule.status_codes) && rule.status_codes.length > 0) {
    return true
  }

  const messageContains = Array.isArray(rule.message_contains)
    ? rule.message_contains
    : []
  const errorCodes = Array.isArray(rule.error_codes) ? rule.error_codes : []

  return (
    messageContains.some(
      (keyword) => typeof keyword === 'string' && keyword.trim() !== ''
    ) ||
    errorCodes.some((code) => typeof code === 'string' && code.trim() !== '')
  )
}
