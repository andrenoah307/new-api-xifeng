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

For commercial licensing, please contact support@quantumnous.com
*/

// The stored option is a map from an exact group name to [total, successful]
// counts. Parsing is deliberately structural; backend business limits are
// checked separately so an administrator can open and repair an out-of-range
// row without the visual editor silently changing it.

export const GROUP_RATE_LIMIT_MAX_COUNT = 2147483647
export const GROUP_RATE_LIMIT_MIN_SUCCESS_COUNT = 1

export type GroupRateLimitRule = {
  groupName: string
  totalCount: number
  successCount: number
}

export type GroupRateLimitErrorCode =
  | 'rule-invalid'
  | 'group-name-required'
  | 'group-name-control'
  | 'group-name-duplicate'
  | 'total-count-range'
  | 'success-count-range'

export type GroupRateLimitParseResult = {
  ok: boolean
  doc: unknown
  rules: GroupRateLimitRule[]
}

export type GroupRateLimitValidationResult = {
  ok: boolean
  errors: GroupRateLimitErrorCode[]
}

export type GroupRateLimitMutationResult = {
  ok: boolean
  json: string
  error: string | null
}

export const GROUP_RATE_LIMIT_INVALID_DOCUMENT_ERROR =
  'Invalid group rate limit document'

function isPlainObject(value: unknown): value is Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return false
  }
  const prototype = Object.getPrototypeOf(value)
  return prototype === Object.prototype || prototype === null
}

function isSafeInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value)
}

function isControlCharacter(character: string): boolean {
  const code = character.codePointAt(0) ?? 0
  return (
    code <= 0x1f ||
    code === 0x7f ||
    (code >= 0x80 && code <= 0x9f)
  )
}

function cloneDocument(doc: Record<string, unknown>): Record<string, unknown> {
  const clone: Record<string, unknown> = { ...doc }
  for (const [groupName, limits] of Object.entries(clone)) {
    if (Array.isArray(limits)) clone[groupName] = [...limits]
  }
  return clone
}

function writeRule(
  doc: Record<string, unknown>,
  groupName: string,
  rule: [number, number]
): void {
  Object.defineProperty(doc, groupName, {
    configurable: true,
    enumerable: true,
    value: rule,
    writable: true,
  })
}

/**
 * Parse without filtering. A single malformed entry makes the whole document
 * non-visualizable, while `doc` still exposes the complete parsed value for a
 * caller that needs to inspect it.
 */
export function parseGroupRateLimitConfig(rawJson: string): GroupRateLimitParseResult {
  if (typeof rawJson !== 'string') {
    return { ok: false, doc: null, rules: [] }
  }
  if (rawJson.trim() === '') {
    return { ok: true, doc: {}, rules: [] }
  }

  let doc: unknown
  try {
    doc = JSON.parse(rawJson)
  } catch {
    return { ok: false, doc: null, rules: [] }
  }

  if (!isPlainObject(doc)) return { ok: false, doc, rules: [] }

  const rules: GroupRateLimitRule[] = []
  for (const [groupName, rawLimits] of Object.entries(doc)) {
    if (
      !Array.isArray(rawLimits) ||
      rawLimits.length !== 2 ||
      !isSafeInteger(rawLimits[0]) ||
      !isSafeInteger(rawLimits[1])
    ) {
      return { ok: false, doc, rules: [] }
    }
    rules.push({
      groupName,
      totalCount: rawLimits[0],
      successCount: rawLimits[1],
    })
  }

  return { ok: true, doc, rules }
}

function hasControlCharacter(name: string): boolean {
  for (const character of name) {
    if (isControlCharacter(character)) return true
  }
  return false
}

export function validateGroupRateLimitRule(
  rule: GroupRateLimitRule
): GroupRateLimitValidationResult {
  const errors: GroupRateLimitErrorCode[] = []
  if (!isPlainObject(rule)) {
    return { ok: false, errors: ['rule-invalid'] }
  }

  if (
    typeof rule.groupName !== 'string' ||
    rule.groupName.trim() === ''
  ) {
    errors.push('group-name-required')
  } else if (hasControlCharacter(rule.groupName)) {
    errors.push('group-name-control')
  }

  if (
    !isSafeInteger(rule.totalCount) ||
    rule.totalCount < 0 ||
    rule.totalCount > GROUP_RATE_LIMIT_MAX_COUNT
  ) {
    errors.push('total-count-range')
  }

  if (
    !isSafeInteger(rule.successCount) ||
    rule.successCount < GROUP_RATE_LIMIT_MIN_SUCCESS_COUNT ||
    rule.successCount > GROUP_RATE_LIMIT_MAX_COUNT
  ) {
    errors.push('success-count-range')
  }

  return { ok: errors.length === 0, errors }
}

function mutationError(
  rawJson: string,
  error: string
): GroupRateLimitMutationResult {
  return { ok: false, json: rawJson, error }
}

export function upsertGroupRateLimitRule(
  rawJson: string,
  rule: GroupRateLimitRule,
  originalGroupName: string | null = null
): GroupRateLimitMutationResult {
  const parsed = parseGroupRateLimitConfig(rawJson)
  if (!parsed.ok) {
    return mutationError(rawJson, GROUP_RATE_LIMIT_INVALID_DOCUMENT_ERROR)
  }

  const doc = cloneDocument(parsed.doc as Record<string, unknown>)
  if (
    originalGroupName !== null &&
    originalGroupName !== rule.groupName
  ) {
    delete doc[originalGroupName]
  }
  writeRule(doc, rule.groupName, [rule.totalCount, rule.successCount])

  return { ok: true, json: JSON.stringify(doc, null, 2), error: null }
}

export function deleteGroupRateLimitRule(
  rawJson: string,
  groupName: string
): GroupRateLimitMutationResult {
  const parsed = parseGroupRateLimitConfig(rawJson)
  if (!parsed.ok) {
    return mutationError(rawJson, GROUP_RATE_LIMIT_INVALID_DOCUMENT_ERROR)
  }

  const doc = cloneDocument(parsed.doc as Record<string, unknown>)
  delete doc[groupName]
  return { ok: true, json: JSON.stringify(doc, null, 2), error: null }
}
