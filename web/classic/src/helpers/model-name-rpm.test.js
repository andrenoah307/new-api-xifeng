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

import assert from 'node:assert/strict';
import { describe, test } from 'node:test';

import {
  MODEL_NAME_RPM_MAX_GLOBAL,
  deleteModelNameRPMGroupTotalRule,
  deleteModelNameRPMRule,
  parseModelNameRPMConfig,
  upsertModelNameRPMGroupTotalRule,
  upsertModelNameRPMRule,
  validateModelNameRPMGroupTotalRule,
  validateModelNameRPMRule,
} from './model-name-rpm.js';

function rule(overrides = {}) {
  return {
    modelName: 'gpt-4o',
    globalRpm: 100,
    userRpm: 0,
    groups: [],
    ...overrides,
  };
}

describe('parseModelNameRPMConfig', () => {
  test('treats an empty document as a disabled, ruleless config', () => {
    assert.deepEqual(parseModelNameRPMConfig(''), {
      ok: true,
      enabled: false,
      rules: [],
      groupTotals: [],
    });
    assert.deepEqual(parseModelNameRPMConfig('   '), {
      ok: true,
      enabled: false,
      rules: [],
      groupTotals: [],
    });
  });

  test('reads enabled flag, global, per-user and group limits', () => {
    const value = JSON.stringify({
      enabled: true,
      models: {
        'gpt-4o': {
          global_rpm: 100,
          user_rpm: 20,
          group_rpm: { vip: 30, free: 5 },
        },
        'claude-3': { global_rpm: 7 },
      },
    });

    assert.deepEqual(parseModelNameRPMConfig(value), {
      ok: true,
      enabled: true,
      groupTotals: [],
      rules: [
        {
          modelName: 'gpt-4o',
          globalRpm: 100,
          userRpm: 20,
          groups: [
            { groupName: 'vip', rpm: 30 },
            { groupName: 'free', rpm: 5 },
          ],
        },
        { modelName: 'claude-3', globalRpm: 7, userRpm: 0, groups: [] },
      ],
    });
  });

  test('normalizes missing and zero user_rpm to the disabled editor value', () => {
    const value = JSON.stringify({
      models: {
        missing: { global_rpm: 10 },
        zero: { global_rpm: 10, user_rpm: 0 },
        one: { global_rpm: 10, user_rpm: 1 },
        equal: { global_rpm: 10, user_rpm: 10 },
        over: { global_rpm: 10, user_rpm: 11 },
      },
    });

    assert.deepEqual(parseModelNameRPMConfig(value), {
      ok: true,
      enabled: false,
      groupTotals: [],
      rules: [
        { modelName: 'missing', globalRpm: 10, userRpm: 0, groups: [] },
        { modelName: 'zero', globalRpm: 10, userRpm: 0, groups: [] },
        { modelName: 'one', globalRpm: 10, userRpm: 1, groups: [] },
        { modelName: 'equal', globalRpm: 10, userRpm: 10, groups: [] },
        { modelName: 'over', globalRpm: 10, userRpm: 11, groups: [] },
      ],
    });
  });

  test('accepts a document without a models key', () => {
    assert.deepEqual(parseModelNameRPMConfig('{"enabled":true}'), {
      ok: true,
      enabled: true,
      rules: [],
      groupTotals: [],
    });
  });

  test('keeps unknown per-rule keys parseable', () => {
    const value = '{"models":{"m":{"global_rpm":5,"future_field":true}}}';
    assert.deepEqual(parseModelNameRPMConfig(value), {
      ok: true,
      enabled: false,
      rules: [{ modelName: 'm', globalRpm: 5, userRpm: 0, groups: [] }],
      groupTotals: [],
    });
  });

  test('reads a groups-only document and accepts boundary totals', () => {
    assert.deepEqual(
      parseModelNameRPMConfig(
        JSON.stringify({
          enabled: true,
          groups: {
            'vip:cheap': { total_rpm: 1 },
            enterprise: { total_rpm: MODEL_NAME_RPM_MAX_GLOBAL },
          },
        }),
      ),
      {
        ok: true,
        enabled: true,
        rules: [],
        groupTotals: [
          { groupName: 'vip:cheap', totalRpm: 1 },
          { groupName: 'enterprise', totalRpm: MODEL_NAME_RPM_MAX_GLOBAL },
        ],
      },
    );
  });

  test('treats missing and null groups as an empty map', () => {
    for (const value of [
      '{"enabled":true}',
      '{"enabled":true,"groups":null}',
    ]) {
      assert.deepEqual(parseModelNameRPMConfig(value), {
        ok: true,
        enabled: true,
        rules: [],
        groupTotals: [],
      });
    }
  });

  const malformedGroupDocuments = [
    ['a non-object groups value', []],
    ['a non-object group rule', { vip: 30 }],
    ['a missing total_rpm', { vip: {} }],
    ['a zero total_rpm', { vip: { total_rpm: 0 } }],
    ['a negative total_rpm', { vip: { total_rpm: -1 } }],
    ['a fractional total_rpm', { vip: { total_rpm: 1.5 } }],
    ['a string total_rpm', { vip: { total_rpm: '30' } }],
    [
      'an over-limit total_rpm',
      { vip: { total_rpm: MODEL_NAME_RPM_MAX_GLOBAL + 1 } },
    ],
    ['an empty group name', { '': { total_rpm: 30 } }],
    ['a whitespace-only group name', { '  ': { total_rpm: 30 } }],
    ['a 65-rune group name', { ['😀'.repeat(65)]: { total_rpm: 30 } }],
    [
      'a control character in the group name',
      { 'vip\u0000': { total_rpm: 30 } },
    ],
  ];

  for (const [label, groups] of malformedGroupDocuments) {
    test(`refuses ${label}`, () => {
      assert.deepEqual(
        parseModelNameRPMConfig(JSON.stringify({ enabled: true, groups })),
        { ok: false },
      );
    });
  }

  const unrepresentableDocuments = [
    ['invalid JSON', '{'],
    ['a JSON array', '[]'],
    ['a JSON scalar', '"text"'],
    ['a non-object models value', '{"models":[]}'],
    ['a non-object rule', '{"models":{"m":5}}'],
    ['a missing global_rpm', '{"models":{"m":{}}}'],
    ['a non-numeric global_rpm', '{"models":{"m":{"global_rpm":"5"}}}'],
    ['a fractional global_rpm', '{"models":{"m":{"global_rpm":1.5}}}'],
    ['a string user_rpm', '{"models":{"m":{"global_rpm":5,"user_rpm":"1"}}}'],
    [
      'a fractional user_rpm',
      '{"models":{"m":{"global_rpm":5,"user_rpm":1.5}}}',
    ],
    ['a negative user_rpm', '{"models":{"m":{"global_rpm":5,"user_rpm":-1}}}'],
    [
      'a non-object group_rpm',
      '{"models":{"m":{"global_rpm":5,"group_rpm":[]}}}',
    ],
    [
      'a non-numeric group limit',
      '{"models":{"m":{"global_rpm":5,"group_rpm":{"vip":"3"}}}}',
    ],
  ];

  for (const [label, value] of unrepresentableDocuments) {
    test(`refuses to represent ${label}`, () => {
      assert.deepEqual(parseModelNameRPMConfig(value), { ok: false });
    });
  }
});

describe('upsertModelNameRPMRule', () => {
  test('adds a rule while preserving group totals and unknown top-level keys', () => {
    const value = JSON.stringify({
      enabled: true,
      future_top_level: 'keep me',
      groups: { vip: { total_rpm: 30, future_group_field: true } },
      models: { existing: { global_rpm: 4 } },
    });

    const next = JSON.parse(
      upsertModelNameRPMRule(value, null, rule({ globalRpm: 60 })),
    );

    assert.deepEqual(next, {
      enabled: true,
      future_top_level: 'keep me',
      groups: { vip: { total_rpm: 30, future_group_field: true } },
      models: {
        existing: { global_rpm: 4 },
        'gpt-4o': { global_rpm: 60 },
      },
    });
  });

  test('renames a model without dropping unknown per-rule keys', () => {
    const value = JSON.stringify({
      groups: { vip: { total_rpm: 30 } },
      future_top_level: true,
      models: { old: { global_rpm: 4, future_field: 1 } },
    });

    const next = JSON.parse(
      upsertModelNameRPMRule(value, 'old', rule({ modelName: 'new' })),
    );

    assert.deepEqual(next.models, {
      new: { global_rpm: 100, future_field: 1 },
    });
    assert.deepEqual(next.groups, { vip: { total_rpm: 30 } });
    assert.equal(next.future_top_level, true);
  });

  test('writes group limits and removes them when the last group is deleted', () => {
    const withGroups = upsertModelNameRPMRule(
      '{"models":{}}',
      null,
      rule({ groups: [{ groupName: 'vip', rpm: 10 }] }),
    );
    assert.deepEqual(JSON.parse(withGroups).models['gpt-4o'], {
      global_rpm: 100,
      group_rpm: { vip: 10 },
    });

    const cleared = upsertModelNameRPMRule(withGroups, 'gpt-4o', rule());
    assert.deepEqual(JSON.parse(cleared).models['gpt-4o'], { global_rpm: 100 });
  });

  test('writes a positive user limit and removes user_rpm when disabled', () => {
    const withUserLimit = upsertModelNameRPMRule(
      '{"models":{"gpt-4o":{"global_rpm":100,"future_field":true}}}',
      'gpt-4o',
      rule({ userRpm: 20 }),
    );
    assert.deepEqual(JSON.parse(withUserLimit).models['gpt-4o'], {
      global_rpm: 100,
      user_rpm: 20,
      future_field: true,
    });

    const disabled = upsertModelNameRPMRule(
      withUserLimit,
      'gpt-4o',
      rule({ userRpm: 0 }),
    );
    assert.deepEqual(JSON.parse(disabled).models['gpt-4o'], {
      global_rpm: 100,
      future_field: true,
    });
  });

  test('serializes an unlimited global limit as an explicit zero', () => {
    const next = upsertModelNameRPMRule(
      '{"models":{"gpt-4o":{"global_rpm":100}}}',
      'gpt-4o',
      rule({ globalRpm: 0, userRpm: 10 }),
    );
    assert.deepEqual(JSON.parse(next).models['gpt-4o'], {
      global_rpm: 0,
      user_rpm: 10,
    });
  });

  test('creates the models container when the document has none', () => {
    const next = JSON.parse(
      upsertModelNameRPMRule('{"enabled":false}', null, rule()),
    );
    assert.deepEqual(next, {
      enabled: false,
      models: { 'gpt-4o': { global_rpm: 100 } },
    });
  });
});

describe('deleteModelNameRPMRule', () => {
  test('removes only the requested model and keeps group totals and other keys', () => {
    const value = JSON.stringify({
      enabled: true,
      groups: { vip: { total_rpm: 30 } },
      future_top_level: true,
      models: { a: { global_rpm: 1 }, b: { global_rpm: 2 } },
    });

    assert.deepEqual(JSON.parse(deleteModelNameRPMRule(value, 'a')), {
      enabled: true,
      groups: { vip: { total_rpm: 30 } },
      future_top_level: true,
      models: { b: { global_rpm: 2 } },
    });
  });
});

describe('group total RPM document updates', () => {
  const groupTotal = (overrides = {}) => ({
    groupName: 'vip',
    totalRpm: 30,
    ...overrides,
  });

  test('adds, updates, renames, and deletes a group total', () => {
    const initial = JSON.stringify({
      enabled: true,
      models: { m: { global_rpm: 10 } },
      future_top_level: 'keep',
    });
    const added = upsertModelNameRPMGroupTotalRule(initial, null, groupTotal());
    assert.deepEqual(JSON.parse(added).groups, { vip: { total_rpm: 30 } });

    const updated = upsertModelNameRPMGroupTotalRule(
      added,
      'vip',
      groupTotal({ totalRpm: 40 }),
    );
    assert.deepEqual(JSON.parse(updated).groups, { vip: { total_rpm: 40 } });

    const renamed = upsertModelNameRPMGroupTotalRule(
      updated,
      'vip',
      groupTotal({ groupName: 'vip:new', totalRpm: 50 }),
    );
    assert.deepEqual(JSON.parse(renamed), {
      enabled: true,
      models: { m: { global_rpm: 10 } },
      groups: { 'vip:new': { total_rpm: 50 } },
      future_top_level: 'keep',
    });

    assert.deepEqual(
      JSON.parse(deleteModelNameRPMGroupTotalRule(renamed, 'vip:new')),
      {
        enabled: true,
        models: { m: { global_rpm: 10 } },
        groups: {},
        future_top_level: 'keep',
      },
    );
  });

  test('preserves unknown fields on an edited group total', () => {
    const next = JSON.parse(
      upsertModelNameRPMGroupTotalRule(
        '{"groups":{"vip":{"total_rpm":30,"future_field":true}}}',
        'vip',
        groupTotal({ totalRpm: 40 }),
      ),
    );
    assert.deepEqual(next.groups.vip, {
      total_rpm: 40,
      future_field: true,
    });
  });
});

describe('validateModelNameRPMGroupTotalRule', () => {
  const groupTotal = (overrides = {}) => ({
    groupName: 'vip:cheap',
    totalRpm: 30,
    ...overrides,
  });

  test('accepts colons, spaces, Unicode names, and total RPM boundaries', () => {
    for (const input of [
      groupTotal({ totalRpm: 1 }),
      groupTotal({
        groupName: 'vip cheap',
        totalRpm: MODEL_NAME_RPM_MAX_GLOBAL,
      }),
      groupTotal({ groupName: '会员组' }),
    ]) {
      assert.equal(validateModelNameRPMGroupTotalRule(input, []), null);
    }
  });

  const errorCases = [
    ['group-total-name-required', groupTotal({ groupName: '' }), []],
    ['group-total-name-required', groupTotal({ groupName: '　' }), []],
    [
      'group-total-name-too-long',
      groupTotal({ groupName: '😀'.repeat(65) }),
      [],
    ],
    ['group-total-name-control', groupTotal({ groupName: 'vip\n' }), []],
    ['group-total-name-duplicate', groupTotal(), ['vip:cheap']],
    ['group-total-rpm-range', groupTotal({ totalRpm: 0 }), []],
    ['group-total-rpm-range', groupTotal({ totalRpm: -1 }), []],
    ['group-total-rpm-range', groupTotal({ totalRpm: 1.5 }), []],
    [
      'group-total-rpm-range',
      groupTotal({ totalRpm: MODEL_NAME_RPM_MAX_GLOBAL + 1 }),
      [],
    ],
  ];

  for (const [code, input, otherNames] of errorCases) {
    test(`reports ${code}`, () => {
      assert.deepEqual(validateModelNameRPMGroupTotalRule(input, otherNames), {
        code,
      });
    });
  }
});

describe('validateModelNameRPMRule', () => {
  test('accepts a rule within the backend limits', () => {
    assert.equal(
      validateModelNameRPMRule(
        rule({ groups: [{ groupName: 'vip', rpm: 100 }] }),
        ['other'],
      ),
      null,
    );
  });

  const ruleErrorCases = [
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
  ];

  for (const [index, [code, input, otherNames]] of ruleErrorCases.entries()) {
    test(`reports ${code} (case ${index + 1})`, () => {
      assert.deepEqual(validateModelNameRPMRule(input, otherNames), { code });
    });
  }

  test('accepts the maximum allowed global limit', () => {
    assert.equal(
      validateModelNameRPMRule(
        rule({ globalRpm: MODEL_NAME_RPM_MAX_GLOBAL }),
        [],
      ),
      null,
    );
  });

  test('accepts an unlimited global limit backed by a sub-limit', () => {
    assert.equal(
      validateModelNameRPMRule(rule({ globalRpm: 0, userRpm: 10 }), []),
      null,
    );
    assert.equal(
      validateModelNameRPMRule(
        rule({ globalRpm: 0, groups: [{ groupName: 'vip', rpm: 30 }] }),
        [],
      ),
      null,
    );
  });

  test('drops the global ceiling comparison when the global limit is unlimited', () => {
    assert.equal(
      validateModelNameRPMRule(
        rule({
          globalRpm: 0,
          userRpm: MODEL_NAME_RPM_MAX_GLOBAL,
          groups: [{ groupName: 'vip', rpm: MODEL_NAME_RPM_MAX_GLOBAL }],
        }),
        [],
      ),
      null,
    );
  });

  test('keeps the sub-limit ceiling even when the global limit is unlimited', () => {
    assert.deepEqual(
      validateModelNameRPMRule(
        rule({ globalRpm: 0, userRpm: MODEL_NAME_RPM_MAX_GLOBAL + 1 }),
        [],
      ),
      { code: 'user-rpm-range' },
    );
    assert.deepEqual(
      validateModelNameRPMRule(
        rule({
          globalRpm: 0,
          groups: [{ groupName: 'vip', rpm: MODEL_NAME_RPM_MAX_GLOBAL + 1 }],
        }),
        [],
      ),
      { code: 'group-rpm-range', groupIndex: 0 },
    );
  });

  test('accepts disabled, minimum and global-equal user limits', () => {
    for (const userRpm of [0, 1, 100]) {
      assert.equal(validateModelNameRPMRule(rule({ userRpm }), []), null);
    }
  });

  const userRPMErrorCases = [
    ['user-rpm-range', -1],
    ['user-rpm-range', 1.5],
    ['user-rpm-exceeds-global', 101],
  ];

  for (const [code, userRpm] of userRPMErrorCases) {
    test(`reports ${code} for user RPM ${userRpm}`, () => {
      assert.deepEqual(validateModelNameRPMRule(rule({ userRpm }), []), {
        code,
      });
    });
  }

  const groupErrorCases = [
    ['group-name-required', { groupName: '', rpm: 5 }],
    ['group-name-too-long', { groupName: 'g'.repeat(65), rpm: 5 }],
    ['group-name-whitespace', { groupName: 'v ip', rpm: 5 }],
    ['group-rpm-range', { groupName: 'vip', rpm: 0 }],
    ['group-rpm-exceeds-global', { groupName: 'vip', rpm: 101 }],
  ];

  for (const [code, group] of groupErrorCases) {
    test(`reports ${code} with the offending group index`, () => {
      const input = rule({ groups: [{ groupName: 'ok', rpm: 1 }, group] });
      assert.deepEqual(validateModelNameRPMRule(input, []), {
        code,
        groupIndex: 1,
      });
    });
  }

  test('reports duplicate group names', () => {
    const input = rule({
      groups: [
        { groupName: 'vip', rpm: 1 },
        { groupName: 'vip', rpm: 2 },
      ],
    });
    assert.deepEqual(validateModelNameRPMRule(input, []), {
      code: 'group-name-duplicate',
      groupIndex: 1,
    });
  });
});
