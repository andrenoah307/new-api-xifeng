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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  MODEL_NAME_RPM_MAX_GLOBAL,
  deleteModelNameRPMRule,
  parseModelNameRPMConfig,
  upsertModelNameRPMRule,
  validateModelNameRPMRule,
  type ModelNameRPMErrorCode,
  type ModelNameRPMGroupLimit,
  type ModelNameRPMRule,
} from './model-name-rpm.ts'

function rule(overrides: Partial<ModelNameRPMRule> = {}): ModelNameRPMRule {
  return {
    modelName: 'gpt-4o',
    globalRpm: 100,
    userRpm: 0,
    groups: [],
    ...overrides,
  }
}

describe('parseModelNameRPMConfig', () => {
  test('treats an empty document as a disabled, ruleless config', () => {
    assert.deepEqual(parseModelNameRPMConfig(''), {
      ok: true,
      enabled: false,
      rules: [],
    })
    assert.deepEqual(parseModelNameRPMConfig('   '), {
      ok: true,
      enabled: false,
      rules: [],
    })
  })

  test('reads enabled flag, global limits and group sub-limits', () => {
    const value = JSON.stringify({
      enabled: true,
      models: {
        'gpt-4o': { global_rpm: 100, group_rpm: { vip: 30, free: 5 } },
        'claude-3': { global_rpm: 7 },
      },
    })

    assert.deepEqual(parseModelNameRPMConfig(value), {
      ok: true,
      enabled: true,
      rules: [
        {
          modelName: 'gpt-4o',
          globalRpm: 100,
          userRpm: 0,
          groups: [
            { groupName: 'vip', rpm: 30 },
            { groupName: 'free', rpm: 5 },
          ],
        },
        { modelName: 'claude-3', globalRpm: 7, userRpm: 0, groups: [] },
      ],
    })
  })

  test('reads missing, disabled, boundary, and over-global per-user limits', () => {
    const value = JSON.stringify({
      models: {
        missing: { global_rpm: 10 },
        disabled: { global_rpm: 10, user_rpm: 0 },
        one: { global_rpm: 10, user_rpm: 1 },
        equal: { global_rpm: 10, user_rpm: 10 },
        over: { global_rpm: 10, user_rpm: 11 },
      },
    })

    assert.deepEqual(parseModelNameRPMConfig(value), {
      ok: true,
      enabled: false,
      rules: [
        { modelName: 'missing', globalRpm: 10, userRpm: 0, groups: [] },
        { modelName: 'disabled', globalRpm: 10, userRpm: 0, groups: [] },
        { modelName: 'one', globalRpm: 10, userRpm: 1, groups: [] },
        { modelName: 'equal', globalRpm: 10, userRpm: 10, groups: [] },
        { modelName: 'over', globalRpm: 10, userRpm: 11, groups: [] },
      ],
    })
  })

  test('accepts a document without a models key', () => {
    assert.deepEqual(parseModelNameRPMConfig('{"enabled":true}'), {
      ok: true,
      enabled: true,
      rules: [],
    })
  })

  test('keeps unknown per-rule keys parseable', () => {
    const value = '{"models":{"m":{"global_rpm":5,"future_field":true}}}'
    assert.deepEqual(parseModelNameRPMConfig(value), {
      ok: true,
      enabled: false,
      rules: [{ modelName: 'm', globalRpm: 5, userRpm: 0, groups: [] }],
    })
  })

  const unrepresentableDocuments: [string, string][] = [
    ['invalid JSON', '{'],
    ['a JSON array', '[]'],
    ['a JSON scalar', '"text"'],
    ['a non-object models value', '{"models":[]}'],
    ['a non-object rule', '{"models":{"m":5}}'],
    ['a missing global_rpm', '{"models":{"m":{}}}'],
    ['a non-numeric global_rpm', '{"models":{"m":{"global_rpm":"5"}}}'],
    ['a fractional global_rpm', '{"models":{"m":{"global_rpm":1.5}}}'],
    [
      'a string user_rpm',
      '{"models":{"m":{"global_rpm":5,"user_rpm":"1"}}}',
    ],
    [
      'a null user_rpm',
      '{"models":{"m":{"global_rpm":5,"user_rpm":null}}}',
    ],
    [
      'a fractional user_rpm',
      '{"models":{"m":{"global_rpm":5,"user_rpm":1.5}}}',
    ],
    [
      'a negative user_rpm',
      '{"models":{"m":{"global_rpm":5,"user_rpm":-1}}}',
    ],
    [
      'a non-object group_rpm',
      '{"models":{"m":{"global_rpm":5,"group_rpm":[]}}}',
    ],
    [
      'a non-numeric group limit',
      '{"models":{"m":{"global_rpm":5,"group_rpm":{"vip":"3"}}}}',
    ],
  ]

  for (const [label, value] of unrepresentableDocuments) {
    test(`refuses to represent ${label}`, () => {
      assert.deepEqual(parseModelNameRPMConfig(value), { ok: false })
    })
  }
})

describe('upsertModelNameRPMRule', () => {
  test('adds a rule while preserving unknown top-level keys', () => {
    const value = JSON.stringify({
      enabled: true,
      future_top_level: 'keep me',
      models: { existing: { global_rpm: 4 } },
    })

    const next = JSON.parse(
      upsertModelNameRPMRule(value, null, rule({ globalRpm: 60 }))
    )

    assert.deepEqual(next, {
      enabled: true,
      future_top_level: 'keep me',
      models: {
        existing: { global_rpm: 4 },
        'gpt-4o': { global_rpm: 60 },
      },
    })
  })

  test('renames a model without dropping unknown per-rule keys', () => {
    const value = JSON.stringify({
      models: { old: { global_rpm: 4, future_field: 1 } },
    })

    const next = JSON.parse(
      upsertModelNameRPMRule(value, 'old', rule({ modelName: 'new' }))
    )

    assert.deepEqual(next.models, {
      new: { global_rpm: 100, future_field: 1 },
    })
  })

  test('writes group limits and removes them when the last group is deleted', () => {
    const withGroups = upsertModelNameRPMRule(
      '{"models":{}}',
      null,
      rule({ groups: [{ groupName: 'vip', rpm: 10 }] })
    )
    assert.deepEqual(JSON.parse(withGroups).models['gpt-4o'], {
      global_rpm: 100,
      group_rpm: { vip: 10 },
    })

    const cleared = upsertModelNameRPMRule(withGroups, 'gpt-4o', rule())
    assert.deepEqual(JSON.parse(cleared).models['gpt-4o'], { global_rpm: 100 })
  })

  test('writes a per-user limit and deletes the key when it is disabled', () => {
    const configured = upsertModelNameRPMRule(
      '{"models":{"gpt-4o":{"global_rpm":10,"future_field":true}}}',
      'gpt-4o',
      rule({ globalRpm: 100, userRpm: 20 })
    )
    assert.deepEqual(JSON.parse(configured).models['gpt-4o'], {
      global_rpm: 100,
      user_rpm: 20,
      future_field: true,
    })

    const disabled = upsertModelNameRPMRule(
      configured,
      'gpt-4o',
      rule({ userRpm: 0 })
    )
    assert.deepEqual(JSON.parse(disabled).models['gpt-4o'], {
      global_rpm: 100,
      future_field: true,
    })
  })

  test('serializes an unlimited global limit as an explicit zero', () => {
    const next = upsertModelNameRPMRule(
      '{"models":{"gpt-4o":{"global_rpm":100}}}',
      'gpt-4o',
      rule({ globalRpm: 0, userRpm: 10 })
    )
    assert.deepEqual(JSON.parse(next).models['gpt-4o'], {
      global_rpm: 0,
      user_rpm: 10,
    })
  })

  test('creates the models container when the document has none', () => {
    const next = JSON.parse(
      upsertModelNameRPMRule('{"enabled":false}', null, rule())
    )
    assert.deepEqual(next, {
      enabled: false,
      models: { 'gpt-4o': { global_rpm: 100 } },
    })
  })
})

describe('deleteModelNameRPMRule', () => {
  test('removes only the requested model and keeps other keys', () => {
    const value = JSON.stringify({
      enabled: true,
      models: { a: { global_rpm: 1 }, b: { global_rpm: 2 } },
    })

    assert.deepEqual(JSON.parse(deleteModelNameRPMRule(value, 'a')), {
      enabled: true,
      models: { b: { global_rpm: 2 } },
    })
  })
})

describe('validateModelNameRPMRule', () => {
  test('accepts a rule within the backend limits', () => {
    assert.equal(
      validateModelNameRPMRule(rule({ groups: [{ groupName: 'vip', rpm: 100 }] }), [
        'other',
      ]),
      null
    )
  })

  const ruleErrorCases: [ModelNameRPMErrorCode, ModelNameRPMRule, string[]][] = [
    ['model-name-required', rule({ modelName: '' }), []],
    ['model-name-too-long', rule({ modelName: 'a'.repeat(256) }), []],
    ['model-name-whitespace', rule({ modelName: 'gpt 4o' }), []],
    ['model-name-duplicate', rule(), ['gpt-4o']],
    ['unlimited-without-sublimit', rule({ globalRpm: 0 }), []],
    ['global-rpm-range', rule({ globalRpm: -1 }), []],
    ['global-rpm-range', rule({ globalRpm: 1.5 }), []],
    [
      'global-rpm-range',
      rule({ globalRpm: MODEL_NAME_RPM_MAX_GLOBAL + 1 }),
      [],
    ],
  ]

  for (const [index, [code, input, otherNames]] of ruleErrorCases.entries()) {
    test(`reports ${code} (case ${index + 1})`, () => {
      assert.deepEqual(validateModelNameRPMRule(input, otherNames), { code })
    })
  }

  test('accepts the maximum allowed global limit', () => {
    assert.equal(
      validateModelNameRPMRule(rule({ globalRpm: MODEL_NAME_RPM_MAX_GLOBAL }), []),
      null
    )
  })

  test('accepts an unlimited global limit backed by a sub-limit', () => {
    assert.equal(
      validateModelNameRPMRule(rule({ globalRpm: 0, userRpm: 10 }), []),
      null
    )
    assert.equal(
      validateModelNameRPMRule(
        rule({ globalRpm: 0, groups: [{ groupName: 'vip', rpm: 30 }] }),
        []
      ),
      null
    )
  })

  test('drops the global ceiling comparison when the global limit is unlimited', () => {
    assert.equal(
      validateModelNameRPMRule(
        rule({
          globalRpm: 0,
          userRpm: MODEL_NAME_RPM_MAX_GLOBAL,
          groups: [{ groupName: 'vip', rpm: MODEL_NAME_RPM_MAX_GLOBAL }],
        }),
        []
      ),
      null
    )
  })

  test('keeps the sub-limit ceiling even when the global limit is unlimited', () => {
    assert.deepEqual(
      validateModelNameRPMRule(
        rule({ globalRpm: 0, userRpm: MODEL_NAME_RPM_MAX_GLOBAL + 1 }),
        []
      ),
      { code: 'user-rpm-range' }
    )
    assert.deepEqual(
      validateModelNameRPMRule(
        rule({
          globalRpm: 0,
          groups: [{ groupName: 'vip', rpm: MODEL_NAME_RPM_MAX_GLOBAL + 1 }],
        }),
        []
      ),
      { code: 'group-rpm-range', groupIndex: 0 }
    )
  })

  test('accepts disabled and boundary per-user limits', () => {
    assert.equal(validateModelNameRPMRule(rule({ userRpm: 0 }), []), null)
    assert.equal(validateModelNameRPMRule(rule({ userRpm: 1 }), []), null)
    assert.equal(validateModelNameRPMRule(rule({ userRpm: 100 }), []), null)
  })

  test('validates the per-user limit range and global ceiling', () => {
    assert.deepEqual(validateModelNameRPMRule(rule({ userRpm: -1 }), []), {
      code: 'user-rpm-range',
    })
    assert.deepEqual(validateModelNameRPMRule(rule({ userRpm: 1.5 }), []), {
      code: 'user-rpm-range',
    })
    assert.deepEqual(validateModelNameRPMRule(rule({ userRpm: 101 }), []), {
      code: 'user-rpm-exceeds-global',
    })
  })

  const groupErrorCases: [ModelNameRPMErrorCode, ModelNameRPMGroupLimit][] = [
    ['group-name-required', { groupName: '', rpm: 5 }],
    ['group-name-too-long', { groupName: 'g'.repeat(65), rpm: 5 }],
    ['group-name-whitespace', { groupName: 'v ip', rpm: 5 }],
    ['group-rpm-range', { groupName: 'vip', rpm: 0 }],
    ['group-rpm-exceeds-global', { groupName: 'vip', rpm: 101 }],
  ]

  for (const [code, group] of groupErrorCases) {
    test(`reports ${code} with the offending group index`, () => {
      const input = rule({ groups: [{ groupName: 'ok', rpm: 1 }, group] })
      assert.deepEqual(validateModelNameRPMRule(input, []), {
        code,
        groupIndex: 1,
      })
    })
  }

  test('reports duplicate group names', () => {
    const input = rule({
      groups: [
        { groupName: 'vip', rpm: 1 },
        { groupName: 'vip', rpm: 2 },
      ],
    })
    assert.deepEqual(validateModelNameRPMRule(input, []), {
      code: 'group-name-duplicate',
      groupIndex: 1,
    })
  })
})
