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

export type PressureCoolingScope = 'channel' | 'groups'
export type PressureCoolingConditionMode = 'any' | 'all'

export interface PressureCooling {
  enabled: boolean | null
  scope: PressureCoolingScope
  cooldown_groups: string[]
  frt_threshold_ms: number | null
  trigger_percent: number | null
  cooldown_seconds: number | null
  observation_window_seconds: number | null
  upstream_error_enabled: boolean
  upstream_error_trigger_percent: number | null
  upstream_error_min_samples: number | null
  condition_mode: PressureCoolingConditionMode
}

// Must match defaults in setting/operation_setting/pressure_cooling_setting.go.
const PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS = {
  enabled: false,
  trigger_percent: 50,
  min_samples: 10,
  condition_mode: 'any',
} as const

const EMPTY_PRESSURE_COOLING: PressureCooling = {
  enabled: null,
  scope: 'channel',
  cooldown_groups: [],
  frt_threshold_ms: null,
  trigger_percent: null,
  cooldown_seconds: null,
  observation_window_seconds: null,
  upstream_error_enabled: PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.enabled,
  upstream_error_trigger_percent: null,
  upstream_error_min_samples: null,
  condition_mode: PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.condition_mode,
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function readNullableNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function readCooldownGroups(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return [
    ...new Set(
      value.filter((group): group is string => typeof group === 'string')
    ),
  ]
}

export function normalizePressureCooling(
  value: unknown
): PressureCooling | null {
  if (!isRecord(value)) return null

  const hasOverride =
    value.enabled != null ||
    value.scope != null ||
    value.cooldown_groups != null ||
    value.frt_threshold_ms != null ||
    value.trigger_percent != null ||
    value.cooldown_seconds != null ||
    value.observation_window_seconds != null ||
    value.upstream_error_enabled != null ||
    value.upstream_error_trigger_percent != null ||
    value.upstream_error_min_samples != null ||
    value.condition_mode != null
  if (!hasOverride) return null

  return {
    enabled: typeof value.enabled === 'boolean' ? value.enabled : null,
    // Missing scope is deliberately normalized to the legacy channel behavior.
    scope: value.scope === 'groups' ? 'groups' : 'channel',
    cooldown_groups: readCooldownGroups(value.cooldown_groups),
    frt_threshold_ms: readNullableNumber(value.frt_threshold_ms),
    trigger_percent: readNullableNumber(value.trigger_percent),
    cooldown_seconds: readNullableNumber(value.cooldown_seconds),
    observation_window_seconds: readNullableNumber(
      value.observation_window_seconds
    ),
    upstream_error_enabled:
      typeof value.upstream_error_enabled === 'boolean'
        ? value.upstream_error_enabled
        : PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.enabled,
    upstream_error_trigger_percent: readNullableNumber(
      value.upstream_error_trigger_percent
    ),
    upstream_error_min_samples: readNullableNumber(
      value.upstream_error_min_samples
    ),
    condition_mode:
      value.condition_mode === 'all'
        ? 'all'
        : PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.condition_mode,
  }
}

export function parsePressureCooling(val: string | undefined): PressureCooling {
  if (!val?.trim()) return { ...EMPTY_PRESSURE_COOLING, cooldown_groups: [] }
  try {
    return (
      normalizePressureCooling(JSON.parse(val)) ?? {
        ...EMPTY_PRESSURE_COOLING,
        cooldown_groups: [],
      }
    )
  } catch {
    return { ...EMPTY_PRESSURE_COOLING, cooldown_groups: [] }
  }
}

export function getPressureCoolingGroupOptions(groups: readonly string[]) {
  const values = new Set<string>()
  return groups.reduce<{ value: string; label: string }[]>((options, group) => {
    const value = group.trim()
    if (!value || values.has(value)) return options
    values.add(value)
    options.push({ value, label: value })
    return options
  }, [])
}

export function cleanPressureCoolingGroups(
  groups: readonly string[],
  availableGroups?: readonly string[]
): string[] {
  const available = availableGroups
    ? new Set(
        getPressureCoolingGroupOptions(availableGroups).map(
          (option) => option.value
        )
      )
    : null
  return [
    ...new Set(
      groups.filter(
        (group) =>
          typeof group === 'string' && (!available || available.has(group))
      )
    ),
  ]
}

export function serializePressureCooling(
  obj: PressureCooling,
  availableGroups?: readonly string[]
): string {
  const cooldownGroups = cleanPressureCoolingGroups(
    obj.cooldown_groups,
    availableGroups
  )
  const hasValue =
    obj.enabled !== null ||
    obj.frt_threshold_ms !== null ||
    obj.trigger_percent !== null ||
    obj.cooldown_seconds !== null ||
    obj.observation_window_seconds !== null ||
    obj.scope === 'groups' ||
    obj.upstream_error_enabled ||
    (obj.upstream_error_trigger_percent != null &&
      obj.upstream_error_trigger_percent !==
        PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.trigger_percent) ||
    (obj.upstream_error_min_samples != null &&
      obj.upstream_error_min_samples !==
        PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.min_samples) ||
    obj.condition_mode === 'all'
  if (!hasValue) return ''

  const serialized: Record<string, unknown> = {
    enabled: obj.enabled,
    frt_threshold_ms: obj.frt_threshold_ms,
    trigger_percent: obj.trigger_percent,
    cooldown_seconds: obj.cooldown_seconds,
    observation_window_seconds: obj.observation_window_seconds,
  }
  if (obj.scope === 'groups') {
    serialized.scope = 'groups'
    serialized.cooldown_groups = cooldownGroups
  }
  if (obj.upstream_error_enabled) {
    serialized.upstream_error_enabled = true
  }
  if (
    obj.upstream_error_trigger_percent != null &&
    obj.upstream_error_trigger_percent !==
      PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.trigger_percent
  ) {
    serialized.upstream_error_trigger_percent =
      obj.upstream_error_trigger_percent
  }
  if (
    obj.upstream_error_min_samples != null &&
    obj.upstream_error_min_samples !==
      PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.min_samples
  ) {
    serialized.upstream_error_min_samples = obj.upstream_error_min_samples
  }
  if (obj.condition_mode === 'all') {
    serialized.condition_mode = 'all'
  }
  return JSON.stringify(serialized)
}

export function isPressureCoolingSaveAllowed(obj: PressureCooling): boolean {
  return obj.scope !== 'groups' || obj.cooldown_groups.length > 0
}
