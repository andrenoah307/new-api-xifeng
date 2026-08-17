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

// Pure helpers shared by the model-name RPM visual editor. The JSON string
// stored in the option map stays the single source of truth: the visual editor
// parses it on render and writes a serialized document back on every change.
// Mirrors web/default/src/features/system-settings/request-limits/lib/model-name-rpm.ts.

export const MODEL_NAME_RPM_MAX_GLOBAL = 1000000;
export const MODEL_NAME_RPM_MAX_MODEL_NAME_LENGTH = 255;
export const MODEL_NAME_RPM_MAX_GROUP_NAME_LENGTH = 64;

const CONTROL_CHARACTER_MAX = 0x1f;
const DELETE_CHARACTER = 0x7f;
const C1_CONTROL_CHARACTER_MAX = 0x9f;

function isRecord(value) {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isCountableInteger(value) {
  return typeof value === 'number' && Number.isSafeInteger(value);
}

// Mirrors unicode.IsSpace plus unicode.IsControl on the Go side.
function hasForbiddenNameCharacter(name) {
  for (const character of name) {
    if (/\s/.test(character)) return true;
    const code = character.codePointAt(0) ?? 0;
    if (code <= CONTROL_CHARACTER_MAX) return true;
    if (code >= DELETE_CHARACTER && code <= C1_CONTROL_CHARACTER_MAX)
      return true;
  }
  return false;
}

function runeLength(value) {
  return [...value].length;
}

/**
 * Parses the stored document into editable rows. It reports `ok: false` for any
 * document the visual editor could not round-trip, so the caller can refuse to
 * switch modes instead of silently rewriting the administrator's JSON.
 */
export function parseModelNameRPMConfig(value) {
  if (!value || value.trim() === '') {
    return { ok: true, enabled: false, rules: [] };
  }

  let parsed;
  try {
    parsed = JSON.parse(value);
  } catch {
    return { ok: false };
  }
  if (!isRecord(parsed)) return { ok: false };

  const enabled = parsed.enabled === true;
  const models = parsed.models;
  if (models === undefined || models === null) {
    return { ok: true, enabled, rules: [] };
  }
  if (!isRecord(models)) return { ok: false };

  const rules = [];
  for (const [modelName, rawRule] of Object.entries(models)) {
    if (!isRecord(rawRule)) return { ok: false };
    if (!isCountableInteger(rawRule.global_rpm)) return { ok: false };

    const rawUserRpm = rawRule.user_rpm;
    let userRpm = 0;
    if (rawUserRpm !== undefined) {
      if (!isCountableInteger(rawUserRpm) || rawUserRpm < 0) {
        return { ok: false };
      }
      userRpm = rawUserRpm;
    }

    const groups = [];
    const rawGroups = rawRule.group_rpm;
    if (rawGroups !== undefined && rawGroups !== null) {
      if (!isRecord(rawGroups)) return { ok: false };
      for (const [groupName, rpm] of Object.entries(rawGroups)) {
        if (!isCountableInteger(rpm)) return { ok: false };
        groups.push({ groupName, rpm });
      }
    }

    rules.push({
      modelName,
      globalRpm: rawRule.global_rpm,
      userRpm,
      groups,
    });
  }

  return { ok: true, enabled, rules };
}

function readConfigObject(value) {
  try {
    const parsed = JSON.parse(value);
    return isRecord(parsed) ? { ...parsed } : {};
  } catch {
    return {};
  }
}

function readModelsObject(config) {
  return isRecord(config.models) ? { ...config.models } : {};
}

/**
 * Writes one rule back into the document. Unknown top-level keys and unknown
 * per-rule keys are preserved, so switching to the visual editor never drops
 * fields a future backend version may add.
 */
export function upsertModelNameRPMRule(value, previousModelName, rule) {
  const config = readConfigObject(value);
  const models = readModelsObject(config);

  const sourceKey = previousModelName ?? rule.modelName;
  const existing = isRecord(models[sourceKey]) ? { ...models[sourceKey] } : {};
  if (previousModelName !== null && previousModelName !== rule.modelName) {
    delete models[previousModelName];
  }

  existing.global_rpm = rule.globalRpm;
  if (rule.userRpm === 0) {
    delete existing.user_rpm;
  } else {
    existing.user_rpm = rule.userRpm;
  }
  if (rule.groups.length === 0) {
    delete existing.group_rpm;
  } else {
    const groupRpm = {};
    for (const group of rule.groups) {
      groupRpm[group.groupName] = group.rpm;
    }
    existing.group_rpm = groupRpm;
  }

  models[rule.modelName] = existing;
  config.models = models;
  return JSON.stringify(config, null, 2);
}

export function deleteModelNameRPMRule(value, modelName) {
  const config = readConfigObject(value);
  const models = readModelsObject(config);
  delete models[modelName];
  config.models = models;
  return JSON.stringify(config, null, 2);
}

function validateName(name, maxLength, codes) {
  if (name === '') return codes.required;
  if (runeLength(name) > maxLength) return codes.tooLong;
  if (hasForbiddenNameCharacter(name)) return codes.whitespace;
  return null;
}

/**
 * Form-level validation only: it catches the mistakes an administrator can fix
 * without a round trip. The backend stays the authority for everything else,
 * including normalized model-name collisions.
 */
export function validateModelNameRPMRule(rule, otherModelNames) {
  const modelNameError = validateName(
    rule.modelName,
    MODEL_NAME_RPM_MAX_MODEL_NAME_LENGTH,
    {
      required: 'model-name-required',
      tooLong: 'model-name-too-long',
      whitespace: 'model-name-whitespace',
    },
  );
  if (modelNameError) return { code: modelNameError };
  if (otherModelNames.includes(rule.modelName)) {
    return { code: 'model-name-duplicate' };
  }

  if (
    !Number.isSafeInteger(rule.globalRpm) ||
    rule.globalRpm < 1 ||
    rule.globalRpm > MODEL_NAME_RPM_MAX_GLOBAL
  ) {
    return { code: 'global-rpm-range' };
  }

  if (!Number.isSafeInteger(rule.userRpm) || rule.userRpm < 0) {
    return { code: 'user-rpm-range' };
  }
  if (rule.userRpm > rule.globalRpm) {
    return { code: 'user-rpm-exceeds-global' };
  }

  const seenGroups = new Set();
  for (let index = 0; index < rule.groups.length; index++) {
    const group = rule.groups[index];
    const groupNameError = validateName(
      group.groupName,
      MODEL_NAME_RPM_MAX_GROUP_NAME_LENGTH,
      {
        required: 'group-name-required',
        tooLong: 'group-name-too-long',
        whitespace: 'group-name-whitespace',
      },
    );
    if (groupNameError) return { code: groupNameError, groupIndex: index };
    if (seenGroups.has(group.groupName)) {
      return { code: 'group-name-duplicate', groupIndex: index };
    }
    seenGroups.add(group.groupName);

    if (!Number.isSafeInteger(group.rpm) || group.rpm < 1) {
      return { code: 'group-rpm-range', groupIndex: index };
    }
    if (group.rpm > rule.globalRpm) {
      return { code: 'group-rpm-exceeds-global', groupIndex: index };
    }
  }

  return null;
}
