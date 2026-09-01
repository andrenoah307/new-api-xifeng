/*
Copyright (C) 2025 QuantumNous

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

const PRESSURE_COOLING_FIELDS = [
  'enabled',
  'frt_threshold_ms',
  'trigger_percent',
  'cooldown_seconds',
  'observation_window_seconds',
];

const PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS = Object.freeze({
  enabled: false,
  trigger_percent: 50,
  min_samples: 10,
  condition_mode: 'any',
});

const PRESSURE_COOLING_UPSTREAM_ERROR_FIELDS = [
  'upstream_error_enabled',
  'upstream_error_trigger_percent',
  'upstream_error_min_samples',
  'condition_mode',
];

const normalizeGroups = (groups) =>
  Array.from(
    new Set(
      (Array.isArray(groups) ? groups : [])
        .filter((group) => typeof group === 'string')
        .map((group) => group.trim())
        .filter(Boolean),
    ),
  );

export const getPressureCoolingGroupOptions = (groups) =>
  normalizeGroups(groups).map((group) => ({
    label: group,
    value: group,
  }));

export const normalizePressureCooling = (value) => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;

  const hasOverride =
    PRESSURE_COOLING_FIELDS.some((field) => value[field] != null) ||
    PRESSURE_COOLING_UPSTREAM_ERROR_FIELDS.some(
      (field) => value[field] != null,
    ) ||
    value.scope != null ||
    value.cooldown_groups != null;
  if (!hasOverride) return null;

  const scope = value.scope === 'groups' ? 'groups' : 'channel';
  const cooldownGroups = normalizeGroups(value.cooldown_groups);

  return {
    enabled: typeof value.enabled === 'boolean' ? value.enabled : null,
    frt_threshold_ms:
      typeof value.frt_threshold_ms === 'number' &&
      Number.isFinite(value.frt_threshold_ms)
        ? value.frt_threshold_ms
        : null,
    trigger_percent:
      typeof value.trigger_percent === 'number' &&
      Number.isFinite(value.trigger_percent)
        ? value.trigger_percent
        : null,
    cooldown_seconds:
      typeof value.cooldown_seconds === 'number' &&
      Number.isFinite(value.cooldown_seconds)
        ? value.cooldown_seconds
        : null,
    observation_window_seconds:
      typeof value.observation_window_seconds === 'number' &&
      Number.isFinite(value.observation_window_seconds)
        ? value.observation_window_seconds
        : null,
    upstream_error_enabled:
      typeof value.upstream_error_enabled === 'boolean'
        ? value.upstream_error_enabled
        : PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.enabled,
    upstream_error_trigger_percent:
      typeof value.upstream_error_trigger_percent === 'number' &&
      Number.isFinite(value.upstream_error_trigger_percent)
        ? value.upstream_error_trigger_percent
        : null,
    upstream_error_min_samples:
      typeof value.upstream_error_min_samples === 'number' &&
      Number.isFinite(value.upstream_error_min_samples)
        ? value.upstream_error_min_samples
        : null,
    condition_mode:
      value.condition_mode === 'all'
        ? 'all'
        : PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.condition_mode,
    scope,
    cooldown_groups: scope === 'groups' ? cooldownGroups : [],
  };
};

export const getPressureCoolingValidationError = (value) => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;

  const triggerPercent = value.trigger_percent;
  if (
    triggerPercent != null &&
    (typeof triggerPercent !== 'number' ||
      !Number.isInteger(triggerPercent) ||
      triggerPercent < 1 ||
      triggerPercent > 100)
  ) {
    return 'trigger_percent';
  }

  const percent = value.upstream_error_trigger_percent;
  if (
    percent != null &&
    (typeof percent !== 'number' ||
      !Number.isFinite(percent) ||
      percent < 0 ||
      percent > 100)
  ) {
    return 'upstream_error_trigger_percent';
  }

  const minSamples = value.upstream_error_min_samples;
  if (
    minSamples != null &&
    (typeof minSamples !== 'number' ||
      !Number.isInteger(minSamples) ||
      minSamples < 1 ||
      minSamples > 10000)
  ) {
    return 'upstream_error_min_samples';
  }

  const conditionMode = value.condition_mode;
  if (conditionMode != null && !['any', 'all'].includes(conditionMode)) {
    return 'condition_mode';
  }

  if (
    value.upstream_error_enabled != null &&
    typeof value.upstream_error_enabled !== 'boolean'
  ) {
    return 'upstream_error_enabled';
  }

  return null;
};

export const isPressureCoolingSaveAllowed = (value) => {
  if (getPressureCoolingValidationError(value)) return false;
  return (
    value.scope !== 'groups' ||
    (Array.isArray(value.cooldown_groups) && value.cooldown_groups.length > 0)
  );
};

export const isPressureCoolingSaveable = isPressureCoolingSaveAllowed;

const cleanCooldownGroups = (groups, availableGroups) => {
  const normalized = normalizeGroups(groups);
  if (!Array.isArray(availableGroups)) return normalized;
  const available = new Set(normalizeGroups(availableGroups));
  return normalized.filter((group) => available.has(group));
};

export const cleanPressureCoolingGroups = cleanCooldownGroups;

export const serializePressureCooling = (value, availableGroups) => {
  const normalized = normalizePressureCooling(value);
  if (!normalized) return '';

  const hasValue =
    PRESSURE_COOLING_FIELDS.some((field) => normalized[field] != null) ||
    normalized.scope === 'groups' ||
    normalized.upstream_error_enabled ||
    (normalized.upstream_error_trigger_percent != null &&
      normalized.upstream_error_trigger_percent !==
        PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.trigger_percent) ||
    (normalized.upstream_error_min_samples != null &&
      normalized.upstream_error_min_samples !==
        PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.min_samples) ||
    normalized.condition_mode === 'all';

  const payload = Object.fromEntries(
    PRESSURE_COOLING_FIELDS.map((field) => [field, normalized[field]]),
  );
  if (normalized.scope === 'groups') {
    payload.scope = 'groups';
    payload.cooldown_groups = cleanCooldownGroups(
      normalized.cooldown_groups,
      availableGroups,
    );
  }

  // Keep legacy payloads byte-for-byte compatible. Global defaults are
  // inherited when the upstream-error fields are omitted.
  if (normalized.upstream_error_enabled) {
    payload.upstream_error_enabled = true;
  }
  if (
    normalized.upstream_error_trigger_percent != null &&
    normalized.upstream_error_trigger_percent !==
      PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.trigger_percent
  ) {
    payload.upstream_error_trigger_percent =
      normalized.upstream_error_trigger_percent;
  }
  if (
    normalized.upstream_error_min_samples != null &&
    normalized.upstream_error_min_samples !==
      PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.min_samples
  ) {
    payload.upstream_error_min_samples = normalized.upstream_error_min_samples;
  }
  if (
    normalized.condition_mode !==
    PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.condition_mode
  ) {
    payload.condition_mode = normalized.condition_mode;
  }

  if (!hasValue) return '';
  return JSON.stringify(payload);
};

export const parsePressureCooling = (value) => {
  if (!value || typeof value !== 'string' || !value.trim()) {
    return {
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
    };
  }
  try {
    return (
      normalizePressureCooling(JSON.parse(value)) || {
        enabled: null,
        scope: 'channel',
        cooldown_groups: [],
        frt_threshold_ms: null,
        trigger_percent: null,
        cooldown_seconds: null,
        observation_window_seconds: null,
        upstream_error_enabled:
          PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.enabled,
        upstream_error_trigger_percent: null,
        upstream_error_min_samples: null,
        condition_mode: PRESSURE_COOLING_UPSTREAM_ERROR_DEFAULTS.condition_mode,
      }
    );
  } catch {
    return {
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
    };
  }
};
