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
    scope,
    cooldown_groups: scope === 'groups' ? cooldownGroups : [],
  };
};

export const isPressureCoolingSaveAllowed = (value) => {
  return value.scope !== 'groups' || value.cooldown_groups.length > 0;
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
    normalized.scope === 'groups';
  if (!hasValue) return '';

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
    };
  }
};
