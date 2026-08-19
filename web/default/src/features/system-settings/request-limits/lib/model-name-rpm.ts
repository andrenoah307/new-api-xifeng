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

// Pure helpers shared by the model-name RPM visual editor. The JSON string
// stored in the option map stays the single source of truth: the visual editor
// parses it on render and writes a serialized document back on every change.

export const MODEL_NAME_RPM_MAX_GLOBAL = 1000000
export const MODEL_NAME_RPM_MAX_MODEL_NAME_LENGTH = 255
export const MODEL_NAME_RPM_MAX_GROUP_NAME_LENGTH = 64

export type ModelNameRPMGroupLimit = {
  groupName: string
  rpm: number
}

export type ModelNameRPMRule = {
  modelName: string
  globalRpm: number
  userRpm: number
  groups: ModelNameRPMGroupLimit[]
}

export type ModelNameRPMGroupTotalRule = {
  groupName: string
  totalRpm: number
}

export type ModelNameRPMParseResult =
  | {
      ok: true
      enabled: boolean
      rules: ModelNameRPMRule[]
      groupTotals: ModelNameRPMGroupTotalRule[]
    }
  | { ok: false }

export type ModelNameRPMRuleErrorCode =
  | 'model-name-required'
  | 'model-name-too-long'
  | 'model-name-whitespace'
  | 'model-name-duplicate'
  | 'global-rpm-range'
  | 'unlimited-without-sublimit'
  | 'user-rpm-range'
  | 'user-rpm-exceeds-global'
  | 'group-name-required'
  | 'group-name-too-long'
  | 'group-name-whitespace'
  | 'group-name-duplicate'
  | 'group-rpm-range'
  | 'group-rpm-exceeds-global'

export type ModelNameRPMGroupTotalErrorCode =
  | 'group-total-name-required'
  | 'group-total-name-too-long'
  | 'group-total-name-whitespace'
  | 'group-total-name-duplicate'
  | 'group-total-rpm-range'

export type ModelNameRPMErrorCode =
  | ModelNameRPMRuleErrorCode
  | ModelNameRPMGroupTotalErrorCode

export type ModelNameRPMRuleError<
  TCode extends ModelNameRPMErrorCode = ModelNameRPMErrorCode,
> = {
  code: TCode
  groupIndex?: number
}

const CONTROL_CHARACTER_MAX = 0x1f
const DELETE_CHARACTER = 0x7f
const C1_CONTROL_CHARACTER_MAX = 0x9f

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isCountableInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value)
}

function hasControlCharacter(name: string): boolean {
  for (const character of name) {
    const code = character.codePointAt(0) ?? 0
    if (code <= CONTROL_CHARACTER_MAX) return true
    if (code >= DELETE_CHARACTER && code <= C1_CONTROL_CHARACTER_MAX) {
      return true
    }
  }
  return false
}

// Mirrors unicode.IsSpace plus unicode.IsControl on the Go side.
function hasForbiddenNameCharacter(name: string): boolean {
  for (const character of name) {
    if (/\s/.test(character)) return true
  }
  return hasControlCharacter(name)
}

function runeLength(value: string): number {
  return [...value].length
}

/**
 * Parses the stored document into editable rows. It reports `ok: false` for any
 * document the visual editor could not round-trip, so the caller can refuse to
 * switch modes instead of silently rewriting the administrator's JSON.
 */
export function parseModelNameRPMConfig(
  value: string
): ModelNameRPMParseResult {
  if (!value || value.trim() === '') {
    return { ok: true, enabled: false, rules: [], groupTotals: [] }
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(value)
  } catch {
    return { ok: false }
  }
  if (!isRecord(parsed)) return { ok: false }

  const enabled = parsed.enabled === true
  const models = parsed.models
  if (models !== undefined && models !== null && !isRecord(models)) {
    return { ok: false }
  }

  const rules: ModelNameRPMRule[] = []
  if (isRecord(models)) {
    for (const [modelName, rawRule] of Object.entries(models)) {
      if (!isRecord(rawRule)) return { ok: false }
      if (!isCountableInteger(rawRule.global_rpm)) return { ok: false }

      let userRpm = 0
      if (rawRule.user_rpm !== undefined) {
        if (!isCountableInteger(rawRule.user_rpm) || rawRule.user_rpm < 0) {
          return { ok: false }
        }
        userRpm = rawRule.user_rpm
      }

      const groups: ModelNameRPMGroupLimit[] = []
      const rawGroups = rawRule.group_rpm
      if (rawGroups !== undefined && rawGroups !== null) {
        if (!isRecord(rawGroups)) return { ok: false }
        for (const [groupName, rpm] of Object.entries(rawGroups)) {
          if (!isCountableInteger(rpm)) return { ok: false }
          groups.push({ groupName, rpm })
        }
      }

      rules.push({ modelName, globalRpm: rawRule.global_rpm, userRpm, groups })
    }
  }

  const groupTotals: ModelNameRPMGroupTotalRule[] = []
  const rawGroupTotals = parsed.groups
  if (rawGroupTotals !== undefined && rawGroupTotals !== null) {
    if (!isRecord(rawGroupTotals)) return { ok: false }
    for (const [groupName, rawRule] of Object.entries(rawGroupTotals)) {
      if (!isRecord(rawRule)) return { ok: false }
      const rule = { groupName, totalRpm: rawRule.total_rpm as number }
      if (validateModelNameRPMGroupTotalRule(rule, [])) return { ok: false }
      groupTotals.push(rule)
    }
  }

  return { ok: true, enabled, rules, groupTotals }
}

function readConfigObject(value: string): Record<string, unknown> {
  try {
    const parsed: unknown = JSON.parse(value)
    return isRecord(parsed) ? { ...parsed } : {}
  } catch {
    return {}
  }
}

function readModelsObject(
  config: Record<string, unknown>
): Record<string, unknown> {
  return isRecord(config.models) ? { ...config.models } : {}
}

function readGroupTotalsObject(
  config: Record<string, unknown>
): Record<string, unknown> {
  return isRecord(config.groups) ? { ...config.groups } : {}
}

/**
 * Writes one rule back into the document. Unknown top-level keys and unknown
 * per-rule keys are preserved, so switching to the visual editor never drops
 * fields a future backend version may add.
 */
export function upsertModelNameRPMRule(
  value: string,
  previousModelName: string | null,
  rule: ModelNameRPMRule
): string {
  const config = readConfigObject(value)
  const models = readModelsObject(config)

  const sourceKey = previousModelName ?? rule.modelName
  const existing = isRecord(models[sourceKey]) ? { ...models[sourceKey] } : {}
  if (previousModelName !== null && previousModelName !== rule.modelName) {
    delete models[previousModelName]
  }

  existing.global_rpm = rule.globalRpm
  if (rule.userRpm === 0) {
    delete existing.user_rpm
  } else {
    existing.user_rpm = rule.userRpm
  }
  if (rule.groups.length === 0) {
    delete existing.group_rpm
  } else {
    const groupRpm: Record<string, number> = {}
    for (const group of rule.groups) {
      Object.defineProperty(groupRpm, group.groupName, {
        configurable: true,
        enumerable: true,
        value: group.rpm,
        writable: true,
      })
    }
    existing.group_rpm = groupRpm
  }

  Object.defineProperty(models, rule.modelName, {
    configurable: true,
    enumerable: true,
    value: existing,
    writable: true,
  })
  config.models = models
  return JSON.stringify(config, null, 2)
}

export function deleteModelNameRPMRule(
  value: string,
  modelName: string
): string {
  const config = readConfigObject(value)
  const models = readModelsObject(config)
  delete models[modelName]
  config.models = models
  return JSON.stringify(config, null, 2)
}

export function upsertModelNameRPMGroupTotalRule(
  value: string,
  previousGroupName: string | null,
  rule: ModelNameRPMGroupTotalRule
): string {
  const config = readConfigObject(value)
  const groups = readGroupTotalsObject(config)
  const sourceKey = previousGroupName ?? rule.groupName
  const existing = isRecord(groups[sourceKey]) ? { ...groups[sourceKey] } : {}

  if (previousGroupName !== null && previousGroupName !== rule.groupName) {
    delete groups[previousGroupName]
  }
  existing.total_rpm = rule.totalRpm
  Object.defineProperty(groups, rule.groupName, {
    configurable: true,
    enumerable: true,
    value: existing,
    writable: true,
  })
  config.groups = groups
  return JSON.stringify(config, null, 2)
}

export function deleteModelNameRPMGroupTotalRule(
  value: string,
  groupName: string
): string {
  const config = readConfigObject(value)
  const groups = readGroupTotalsObject(config)
  delete groups[groupName]
  config.groups = groups
  return JSON.stringify(config, null, 2)
}

function validateName<TCode extends ModelNameRPMErrorCode>(
  name: string,
  maxLength: number,
  codes: {
    required: TCode
    tooLong: TCode
    whitespace: TCode
  }
): TCode | null {
  if (name === '') return codes.required
  if (runeLength(name) > maxLength) return codes.tooLong
  if (hasForbiddenNameCharacter(name)) return codes.whitespace
  return null
}

export function validateModelNameRPMGroupTotalRule(
  rule: ModelNameRPMGroupTotalRule,
  otherGroupNames: string[]
): ModelNameRPMRuleError<ModelNameRPMGroupTotalErrorCode> | null {
  const groupNameError = validateName(
    rule.groupName,
    MODEL_NAME_RPM_MAX_GROUP_NAME_LENGTH,
    {
      required: 'group-total-name-required',
      tooLong: 'group-total-name-too-long',
      whitespace: 'group-total-name-whitespace',
    }
  )
  if (groupNameError) return { code: groupNameError }
  if (otherGroupNames.includes(rule.groupName)) {
    return { code: 'group-total-name-duplicate' }
  }
  if (
    !Number.isSafeInteger(rule.totalRpm) ||
    rule.totalRpm < 1 ||
    rule.totalRpm > MODEL_NAME_RPM_MAX_GLOBAL
  ) {
    return { code: 'group-total-rpm-range' }
  }
  return null
}

/**
 * Form-level validation only: it catches the mistakes an administrator can fix
 * without a round trip. The backend stays the authority for everything else,
 * including normalized model-name collisions.
 */
export function validateModelNameRPMRule(
  rule: ModelNameRPMRule,
  otherModelNames: string[]
): ModelNameRPMRuleError<ModelNameRPMRuleErrorCode> | null {
  const modelNameError = validateName(
    rule.modelName,
    MODEL_NAME_RPM_MAX_MODEL_NAME_LENGTH,
    {
      required: 'model-name-required',
      tooLong: 'model-name-too-long',
      whitespace: 'model-name-whitespace',
    }
  )
  if (modelNameError) return { code: modelNameError }
  if (otherModelNames.includes(rule.modelName)) {
    return { code: 'model-name-duplicate' }
  }

  // 0 means unlimited: the bucket is still counted, it just never rejects.
  if (
    !Number.isSafeInteger(rule.globalRpm) ||
    rule.globalRpm < 0 ||
    rule.globalRpm > MODEL_NAME_RPM_MAX_GLOBAL
  ) {
    return { code: 'global-rpm-range' }
  }
  if (
    rule.globalRpm === 0 &&
    rule.userRpm === 0 &&
    rule.groups.length === 0
  ) {
    return { code: 'unlimited-without-sublimit' }
  }

  if (
    !Number.isSafeInteger(rule.userRpm) ||
    rule.userRpm < 0 ||
    rule.userRpm > MODEL_NAME_RPM_MAX_GLOBAL
  ) {
    return { code: 'user-rpm-range' }
  }
  if (rule.globalRpm > 0 && rule.userRpm > rule.globalRpm) {
    return { code: 'user-rpm-exceeds-global' }
  }

  const seenGroups = new Set<string>()
  for (let index = 0; index < rule.groups.length; index++) {
    const group = rule.groups[index]
    const groupNameError = validateName(
      group.groupName,
      MODEL_NAME_RPM_MAX_GROUP_NAME_LENGTH,
      {
        required: 'group-name-required',
        tooLong: 'group-name-too-long',
        whitespace: 'group-name-whitespace',
      }
    )
    if (groupNameError) return { code: groupNameError, groupIndex: index }
    if (seenGroups.has(group.groupName)) {
      return { code: 'group-name-duplicate', groupIndex: index }
    }
    seenGroups.add(group.groupName)

    if (
      !Number.isSafeInteger(group.rpm) ||
      group.rpm < 1 ||
      group.rpm > MODEL_NAME_RPM_MAX_GLOBAL
    ) {
      return { code: 'group-rpm-range', groupIndex: index }
    }
    if (rule.globalRpm > 0 && group.rpm > rule.globalRpm) {
      return { code: 'group-rpm-exceeds-global', groupIndex: index }
    }
  }

  return null
}
