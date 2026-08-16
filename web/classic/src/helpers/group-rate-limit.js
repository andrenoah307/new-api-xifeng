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

// Keep this module structurally identical to the Default pure helper. The
// option is a map from an exact group name to [total, successful] counts.

export const GROUP_RATE_LIMIT_MAX_COUNT = 2147483647;
export const GROUP_RATE_LIMIT_MIN_SUCCESS_COUNT = 1;

export const GROUP_RATE_LIMIT_INVALID_DOCUMENT_ERROR =
  'Invalid group rate limit document';

function isPlainObject(value) {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return false;
  }
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function isSafeInteger(value) {
  return typeof value === 'number' && Number.isSafeInteger(value);
}

function isControlCharacter(character) {
  const code = character.codePointAt(0) ?? 0;
  return code <= 0x1f || code === 0x7f || (code >= 0x80 && code <= 0x9f);
}

function cloneDocument(doc) {
  const clone = { ...doc };
  for (const [groupName, limits] of Object.entries(clone)) {
    if (Array.isArray(limits)) clone[groupName] = [...limits];
  }
  return clone;
}

function writeRule(doc, groupName, rule) {
  Object.defineProperty(doc, groupName, {
    configurable: true,
    enumerable: true,
    value: rule,
    writable: true,
  });
}

export function parseGroupRateLimitConfig(rawJson) {
  if (typeof rawJson !== 'string') {
    return { ok: false, doc: null, rules: [] };
  }
  if (rawJson.trim() === '') {
    return { ok: true, doc: {}, rules: [] };
  }

  let doc;
  try {
    doc = JSON.parse(rawJson);
  } catch {
    return { ok: false, doc: null, rules: [] };
  }

  if (!isPlainObject(doc)) return { ok: false, doc, rules: [] };

  const rules = [];
  for (const [groupName, rawLimits] of Object.entries(doc)) {
    if (
      !Array.isArray(rawLimits) ||
      rawLimits.length !== 2 ||
      !isSafeInteger(rawLimits[0]) ||
      !isSafeInteger(rawLimits[1])
    ) {
      return { ok: false, doc, rules: [] };
    }
    rules.push({
      groupName,
      totalCount: rawLimits[0],
      successCount: rawLimits[1],
    });
  }

  return { ok: true, doc, rules };
}

function hasControlCharacter(name) {
  for (const character of name) {
    if (isControlCharacter(character)) return true;
  }
  return false;
}

export function validateGroupRateLimitRule(rule) {
  const errors = [];
  if (!isPlainObject(rule)) {
    return { ok: false, errors: ['rule-invalid'] };
  }

  if (typeof rule.groupName !== 'string' || rule.groupName.trim() === '') {
    errors.push('group-name-required');
  } else if (hasControlCharacter(rule.groupName)) {
    errors.push('group-name-control');
  }

  if (
    !isSafeInteger(rule.totalCount) ||
    rule.totalCount < 0 ||
    rule.totalCount > GROUP_RATE_LIMIT_MAX_COUNT
  ) {
    errors.push('total-count-range');
  }

  if (
    !isSafeInteger(rule.successCount) ||
    rule.successCount < GROUP_RATE_LIMIT_MIN_SUCCESS_COUNT ||
    rule.successCount > GROUP_RATE_LIMIT_MAX_COUNT
  ) {
    errors.push('success-count-range');
  }

  return { ok: errors.length === 0, errors };
}

function mutationError(rawJson, error) {
  return { ok: false, json: rawJson, error };
}

export function upsertGroupRateLimitRule(
  rawJson,
  rule,
  originalGroupName = null,
) {
  const parsed = parseGroupRateLimitConfig(rawJson);
  if (!parsed.ok) {
    return mutationError(rawJson, GROUP_RATE_LIMIT_INVALID_DOCUMENT_ERROR);
  }

  const doc = cloneDocument(parsed.doc);
  if (originalGroupName !== null && originalGroupName !== rule.groupName) {
    delete doc[originalGroupName];
  }
  writeRule(doc, rule.groupName, [rule.totalCount, rule.successCount]);

  return { ok: true, json: JSON.stringify(doc, null, 2), error: null };
}

export function deleteGroupRateLimitRule(rawJson, groupName) {
  const parsed = parseGroupRateLimitConfig(rawJson);
  if (!parsed.ok) {
    return mutationError(rawJson, GROUP_RATE_LIMIT_INVALID_DOCUMENT_ERROR);
  }

  const doc = cloneDocument(parsed.doc);
  delete doc[groupName];
  return { ok: true, json: JSON.stringify(doc, null, 2), error: null };
}
