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

import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  GROUP_RATE_LIMIT_MAX_COUNT,
  GROUP_RATE_LIMIT_MIN_SUCCESS_COUNT,
  deleteGroupRateLimitRule,
  parseGroupRateLimitConfig,
  upsertGroupRateLimitRule,
  validateGroupRateLimitRule,
} from './group-rate-limit.ts'

describe('parseGroupRateLimitConfig', () => {
  test('normalizes empty input to an empty document', () => {
    assert.deepEqual(parseGroupRateLimitConfig(''), {
      ok: true,
      doc: {},
      rules: [],
    })
    assert.deepEqual(parseGroupRateLimitConfig('  \n\t'), {
      ok: true,
      doc: {},
      rules: [],
    })
  })

  test('returns stable rows and preserves the parsed document', () => {
    const raw = '{"default":[200,100],"vip":[0,1000]}'
    assert.deepEqual(parseGroupRateLimitConfig(raw), {
      ok: true,
      doc: { default: [200, 100], vip: [0, 1000] },
      rules: [
        { groupName: 'default', totalCount: 200, successCount: 100 },
        { groupName: 'vip', totalCount: 0, successCount: 1000 },
      ],
    })
  })

  test('accepts JSON integer literals written with a decimal point', () => {
    const parsed = parseGroupRateLimitConfig('{"vip":[1.0,2.0]}')
    assert.equal(parsed.ok, true)
    assert.deepEqual(parsed.rules, [
      { groupName: 'vip', totalCount: 1, successCount: 2 },
    ])
    assert.equal(
      upsertGroupRateLimitRule(
        '{"vip":[1.0,2.0]}',
        { groupName: 'vip', totalCount: 1, successCount: 2 },
        'vip'
      ).json,
      '{\n  "vip": [\n    1,\n    2\n  ]\n}'
    )
  })

  test('rejects malformed documents without partial rows', () => {
    for (const raw of [
      '{',
      '[]',
      'null',
      '"groups"',
      '{"default":{}}',
      '{"default":[1]}',
      '{"default":[1,2,3]}',
      '{"default":[1.5,2]}',
      '{"default":["1",2]}',
      '{"default":[null,2]}',
      '{"default":[true,2]}',
    ]) {
      const parsed = parseGroupRateLimitConfig(raw)
      assert.equal(parsed.ok, false, raw)
      assert.deepEqual(parsed.rules, [], raw)
    }
  })
})

describe('validateGroupRateLimitRule', () => {
  test('accepts the backend boundary values', () => {
    assert.deepEqual(
      validateGroupRateLimitRule({
        groupName: 'vip',
        totalCount: 0,
        successCount: GROUP_RATE_LIMIT_MIN_SUCCESS_COUNT,
      }),
      { ok: true, errors: [] },
    )
    assert.deepEqual(
      validateGroupRateLimitRule({
        groupName: 'vip',
        totalCount: GROUP_RATE_LIMIT_MAX_COUNT,
        successCount: GROUP_RATE_LIMIT_MAX_COUNT,
      }),
      { ok: true, errors: [] },
    )
  })

  test('reports semantic errors while parse still accepts out-of-range values', () => {
    assert.deepEqual(
      validateGroupRateLimitRule({
        groupName: 'vip',
        totalCount: -1,
        successCount: 0,
      }),
      { ok: false, errors: ['total-count-range', 'success-count-range'] },
    )
    assert.deepEqual(
      validateGroupRateLimitRule({
        groupName: 'vip',
        totalCount: GROUP_RATE_LIMIT_MAX_COUNT + 1,
        successCount: GROUP_RATE_LIMIT_MAX_COUNT + 1,
      }),
      { ok: false, errors: ['total-count-range', 'success-count-range'] },
    )
    assert.equal(
      parseGroupRateLimitConfig('{"vip":[-1,1]}').ok,
      true,
    )
    assert.equal(parseGroupRateLimitConfig('{"vip":[0,0]}').ok, true)
    assert.equal(
      parseGroupRateLimitConfig('{"vip":[2147483648,1]}').ok,
      true,
    )
  })

  test('requires a trimmed non-empty name and rejects control characters', () => {
    assert.deepEqual(
      validateGroupRateLimitRule({
        groupName: ' \t ',
        totalCount: 1,
        successCount: 1,
      }),
      { ok: false, errors: ['group-name-required'] },
    )
    assert.deepEqual(
      validateGroupRateLimitRule({
        groupName: 'vip\nwest',
        totalCount: 1,
        successCount: 1,
      }),
      { ok: false, errors: ['group-name-control'] },
    )
    assert.deepEqual(
      validateGroupRateLimitRule({
        groupName: 'a'.repeat(1000),
        totalCount: 1,
        successCount: 1,
      }),
      { ok: true, errors: [] },
    )
  })
})

describe('incremental mutations', () => {
  test('preserves unknown entries and renames by deleting the old key first', () => {
    const raw = '{"future":[7,8],"old":[4,2]}'
    const result = upsertGroupRateLimitRule(
      raw,
      { groupName: 'new', totalCount: 5, successCount: 2 },
      'old',
    )
    assert.deepEqual(result, {
      ok: true,
      json: '{\n  "future": [\n    7,\n    8\n  ],\n  "new": [\n    5,\n    2\n  ]\n}',
      error: null,
    })
  })

  test('refuses to mutate an unrepresentable document', () => {
    const raw = '{"valid":[1,1],"broken":[1]}'
    assert.deepEqual(
      upsertGroupRateLimitRule(
        raw,
        { groupName: 'new', totalCount: 1, successCount: 1 },
        null,
      ),
      { ok: false, json: raw, error: 'Invalid group rate limit document' },
    )
    assert.deepEqual(deleteGroupRateLimitRule(raw, 'valid'), {
      ok: false,
      json: raw,
      error: 'Invalid group rate limit document',
    })
  })

  test('deleting a missing key keeps the valid document intact', () => {
    assert.deepEqual(deleteGroupRateLimitRule('{"default":[2,1]}', 'missing'), {
      ok: true,
      json: '{\n  "default": [\n    2,\n    1\n  ]\n}',
      error: null,
    })
  })
})
