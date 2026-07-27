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
import { readFileSync } from 'node:fs';
import test from 'node:test';

import { reconstructBillingProcess } from './billing-process.js';

const fixture = JSON.parse(
  readFileSync(
    new URL(
      '../../../test-fixtures/billing-process/production.json',
      import.meta.url,
    ),
    'utf8',
  ),
);

const fixtureById = new Map(fixture.cases.map((entry) => [entry.id, entry]));

function encodeExpression(expression) {
  return Buffer.from(expression, 'utf8').toString('base64');
}

function reconstruct(log, other, quotaPerUnit = fixture.quota_per_unit) {
  return reconstructBillingProcess({ log, other, quotaPerUnit });
}

function ratioInput(overrides = {}) {
  return {
    log: { prompt_tokens: 100, completion_tokens: 10, quota: 110 },
    other: {
      model_price: -1,
      model_ratio: 1,
      completion_ratio: 1,
      group_ratio: 1,
      user_group_ratio: -1,
      cache_tokens: 0,
      cache_creation_tokens: 0,
    },
    ...overrides,
  };
}

function tieredInput(expression, overrides = {}) {
  return {
    log: { prompt_tokens: 100, completion_tokens: 0, quota: 50 },
    other: {
      billing_mode: 'tiered_expr',
      expr_b64: encodeExpression(expression),
      matched_tier: 'standard',
      group_ratio: 1,
      user_group_ratio: -1,
      cache_tokens: 0,
      cache_creation_tokens: 0,
    },
    ...overrides,
  };
}

test('reconstructs every shared production fixture exactly', () => {
  for (const entry of fixture.cases) {
    const expected =
      entry.expected ?? fixtureById.get(entry.expected_case).expected;
    assert.deepStrictEqual(
      reconstruct(entry.log, entry.other),
      expected,
      entry.id,
    );
  }
});

test('strict tier parser supports coefficient-first multiplication and zero coefficients', () => {
  const coefficientFirst = tieredInput(
    'tier("standard", 0.5 * cr + p * 1 + c * 0)',
    {
      log: { prompt_tokens: 100, completion_tokens: 0, quota: 45 },
      other: {
        ...tieredInput('tier("standard", p * 1)').other,
        expr_b64: encodeExpression(
          'tier("standard", 0.5 * cr + p * 1 + c * 0)',
        ),
        cache_tokens: 20,
      },
    },
  );
  const coefficientResult = reconstruct(
    coefficientFirst.log,
    coefficientFirst.other,
  );
  assert.equal(coefficientResult.ok, true);
  assert.equal(coefficientResult.tokens.p, 80);
  assert.equal(coefficientResult.expressionOutput, 90);

  const zeroCache = tieredInput('tier("standard", cr * 0 + p * 1 + c * 0)', {
    log: { prompt_tokens: 100, completion_tokens: 0, quota: 40 },
    other: {
      ...tieredInput('tier("standard", p * 1)').other,
      expr_b64: encodeExpression('tier("standard", cr * 0 + p * 1 + c * 0)'),
      cache_tokens: 20,
    },
  });
  const zeroResult = reconstruct(zeroCache.log, zeroCache.other);
  assert.equal(zeroResult.ok, true);
  assert.equal(zeroResult.tokens.p, 80);
  assert.ok(zeroResult.usedVariables.includes('cr'));
});

test('tier labels are not mistaken for used variables', () => {
  const input = tieredInput('tier("cr", p * 1 + c * 0)', {
    other: {
      ...tieredInput('tier("standard", p * 1)').other,
      expr_b64: encodeExpression('tier("cr", p * 1 + c * 0)'),
      matched_tier: 'cr',
      cache_tokens: 20,
    },
  });
  const result = reconstruct(input.log, input.other);
  assert.equal(result.ok, true);
  assert.equal(result.tokens.p, 100);
  assert.equal(result.usedVariables.includes('cr'), false);
});

test('strict tier parser rejects partial or unsupported syntax', () => {
  const expressions = [
    'tier("standard", min(p, 1))',
    'tier("standard", max(p, 1))',
    'tier("standard", (p * 1))',
    'tier("standard", p * 1) + 7',
    'tier("standard", p * param("size"))',
    'header("x") == "y" ? tier("standard", p * 1) : tier("other", p * 2)',
    'tier("standard", p * -1)',
    'tier("standard", p * 1 +)',
    'tier("standard", p)',
    'tier("unterminated, p * 1)',
    'tier("\\x", p * 1)',
    'tier("standard", p * 1',
    'len < 100 ? tier("standard", p * 1)',
  ];
  for (const expression of expressions) {
    const input = tieredInput(expression);
    assert.deepStrictEqual(reconstruct(input.log, input.other), {
      ok: false,
      reason: 'unsupported_expression',
    });
  }
});

test('tier parser handles nested ternaries and condition variables', () => {
  const expression =
    'len <= 50 ? tier("short", p * 1) : p > 50 && c >= 0 ? tier("standard", p * 1) : tier("other", p * 2)';
  const input = tieredInput(expression);
  const result = reconstruct(input.log, input.other);
  assert.equal(result.ok, true);
  assert.deepStrictEqual(result.usedVariables, ['p', 'c', 'len']);

  const escapedExpression =
    '50 < len ? tier("sta\\\"ndard", p * 1) : tier("other", p * 2)';
  const escapedInput = tieredInput(escapedExpression, {
    other: {
      ...tieredInput('tier("standard", p * 1)').other,
      expr_b64: encodeExpression(escapedExpression),
      matched_tier: 'sta"ndard',
    },
  });
  assert.equal(reconstruct(escapedInput.log, escapedInput.other).ok, true);
});

test('tiered failures are explicit and fail closed', () => {
  const valid = tieredInput('tier("standard", p * 1)');
  const cases = [
    [{ ...valid.other, expr_b64: undefined }, 'invalid_expression_encoding'],
    [{ ...valid.other, expr_b64: '%%%' }, 'invalid_expression_encoding'],
    [
      {
        ...valid.other,
        expr_b64: encodeExpression('v2:tier("standard", p * 1)'),
      },
      'unsupported_expression',
    ],
    [{ ...valid.other, matched_tier: undefined }, 'matched_tier_missing'],
    [{ ...valid.other, matched_tier: 'missing' }, 'matched_tier_not_found'],
    [
      {
        ...valid.other,
        expr_b64: encodeExpression(
          'len < 50 ? tier("standard", p * 1) : tier("standard", p * 1)',
        ),
      },
      'matched_tier_ambiguous',
    ],
    [
      {
        ...valid.other,
        expr_b64: encodeExpression('tier("standard", img * 1)'),
      },
      'missing_token_dimension',
    ],
  ];
  for (const [other, reason] of cases) {
    assert.deepStrictEqual(reconstruct(valid.log, other), {
      ok: false,
      reason,
    });
  }
  assert.deepStrictEqual(
    reconstruct({ ...valid.log, quota: 49 }, valid.other),
    {
      ok: false,
      reason: 'quota_mismatch',
    },
  );
});

test('zero user ratio is authoritative and is never replaced by the group ratio', () => {
  const input = tieredInput('tier("standard", p * 1)', {
    log: { prompt_tokens: 100, completion_tokens: 0, quota: 0 },
    other: {
      ...tieredInput('tier("standard", p * 1)').other,
      user_group_ratio: 0,
      group_ratio: 3,
    },
  });
  const result = reconstruct(input.log, input.other);
  assert.equal(result.ok, true);
  assert.equal(result.effectiveGroupRatio, 0);
  assert.equal(result.groupRatioSource, 'user');
});

test('reconstructs the supported OpenAI ratio text path including cache reads', () => {
  const input = ratioInput({
    log: { prompt_tokens: 100, completion_tokens: 10, quota: 120 },
    other: {
      ...ratioInput().other,
      model_ratio: 2,
      completion_ratio: 3,
      group_ratio: 0.5,
      cache_tokens: 20,
      cache_ratio: 0.5,
    },
  });
  const result = reconstruct(input.log, input.other);
  assert.equal(result.ok, true);
  assert.equal(result.mode, 'ratio_openai');
  assert.equal(result.tokens.p, 80);
  assert.equal(result.quotaBeforeRound, 120);
  assert.equal(result.quota, 120);
});

test('reconstructs Claude ratio text without subtracting cache buckets', () => {
  const input = ratioInput({
    log: { prompt_tokens: 100, completion_tokens: 10, quota: 170 },
    other: {
      ...ratioInput().other,
      claude: true,
      model_ratio: 2,
      completion_ratio: 3,
      group_ratio: 0.5,
      cache_tokens: 20,
      cache_ratio: 0.1,
      cache_creation_tokens: 30,
      cache_creation_ratio: 1.25,
    },
  });
  const result = reconstruct(input.log, input.other);
  assert.equal(result.ok, true);
  assert.equal(result.mode, 'ratio_claude');
  assert.equal(result.tokens.p, 100);
  assert.equal(result.quotaBeforeRound, 169.5);
  assert.equal(result.quota, 170);
});

test('reconstructs Claude split cache creation buckets exactly', () => {
  const input = ratioInput({
    log: { prompt_tokens: 100, completion_tokens: 10, quota: 186 },
    other: {
      ...ratioInput().other,
      claude: true,
      model_ratio: 2,
      completion_ratio: 3,
      group_ratio: 0.5,
      cache_tokens: 20,
      cache_ratio: 0.1,
      cache_creation_tokens: 30,
      cache_creation_ratio: 1.25,
      cache_creation_tokens_5m: 10,
      cache_creation_ratio_5m: 2,
      cache_creation_tokens_1h: 5,
      cache_creation_ratio_1h: 3,
    },
  });
  const result = reconstruct(input.log, input.other);
  assert.equal(result.ok, true);
  assert.equal(result.tokens.p, 100);
  assert.deepStrictEqual(
    result.terms.map((term) => [term.kind, term.tokens]),
    [
      ['input', 100],
      ['cache_read', 20],
      ['cache_creation', 15],
      ['cache_creation_5m', 10],
      ['cache_creation_1h', 5],
      ['output', 10],
    ],
  );
  assert.equal(result.quota, 186);
});

test('unsupported billing paths return stable degradation reason codes', () => {
  const base = ratioInput();
  const cases = [
    [{ is_task: true }, 'unsupported_task'],
    [{ task_id: 'task-1' }, 'unsupported_task'],
    [{ ws: true }, 'unsupported_audio'],
    [{ audio: true }, 'unsupported_audio'],
    [{ audio_input_seperate_price: true }, 'unsupported_audio'],
    [{ audio_input: 1 }, 'unsupported_audio'],
    [{ audio_output: 1 }, 'unsupported_audio'],
    [{ audio_input_token_count: 1 }, 'unsupported_audio'],
    [{ image: true }, 'unsupported_image'],
    [{ image_output: 1 }, 'unsupported_image'],
    [{ web_search: true }, 'unsupported_tool_surcharge'],
    [{ web_search_call_count: 1 }, 'unsupported_tool_surcharge'],
    [{ file_search: true }, 'unsupported_tool_surcharge'],
    [{ file_search_call_count: 1 }, 'unsupported_tool_surcharge'],
    [{ image_generation_call: true }, 'unsupported_tool_surcharge'],
    [{ image_generation_call_price: 0.01 }, 'unsupported_tool_surcharge'],
    [
      { admin_info: { quota_saturation: { kind: 'overflow' } } },
      'unsupported_saturation',
    ],
  ];
  for (const [extra, reason] of cases) {
    assert.deepStrictEqual(reconstruct(base.log, { ...base.other, ...extra }), {
      ok: false,
      reason,
    });
  }

  assert.deepStrictEqual(
    reconstruct(base.log, { ...base.other, model_price: 0.01 }),
    { ok: false, reason: 'unsupported_per_call' },
  );
  assert.deepStrictEqual(
    reconstruct(base.log, {
      ...base.other,
      cache_creation_tokens_5m: 1,
      cache_creation_ratio_5m: 1,
    }),
    { ok: false, reason: 'unsupported_cache_layout' },
  );
});

test('invalid numeric input and unverifiable calculations fail closed', () => {
  const base = ratioInput();
  const cases = [
    [null, base.log, base.other, fixture.quota_per_unit, 'invalid_input'],
    [{}, base.log, base.other, 0, 'invalid_quota_per_unit'],
    [
      {},
      { ...base.log, prompt_tokens: -1 },
      base.other,
      fixture.quota_per_unit,
      'invalid_token_value',
    ],
    [
      {},
      { ...base.log, completion_tokens: 1.5 },
      base.other,
      fixture.quota_per_unit,
      'invalid_token_value',
    ],
    [
      {},
      { ...base.log, quota: undefined },
      base.other,
      fixture.quota_per_unit,
      'invalid_token_value',
    ],
    [
      {},
      base.log,
      { ...base.other, group_ratio: -1 },
      fixture.quota_per_unit,
      'invalid_group_ratio',
    ],
    [
      {},
      base.log,
      { ...base.other, user_group_ratio: -2 },
      fixture.quota_per_unit,
      'invalid_group_ratio',
    ],
    [
      {},
      base.log,
      { ...base.other, model_ratio: -1 },
      fixture.quota_per_unit,
      'invalid_ratio',
    ],
    [
      {},
      base.log,
      { ...base.other, completion_ratio: undefined },
      fixture.quota_per_unit,
      'invalid_ratio',
    ],
    [
      {},
      base.log,
      { ...base.other, cache_tokens: 1, cache_ratio: undefined },
      fixture.quota_per_unit,
      'invalid_ratio',
    ],
    [
      {},
      base.log,
      { ...base.other, cache_tokens: -1 },
      fixture.quota_per_unit,
      'invalid_token_value',
    ],
  ];
  for (const [input, log, other, quotaPerUnit, reason] of cases) {
    const result =
      input === null
        ? reconstructBillingProcess(null)
        : reconstruct(log, other, quotaPerUnit);
    assert.deepStrictEqual(result, { ok: false, reason });
  }
});

test('ratio minimum-one behavior follows the logged settlement contract', () => {
  const base = ratioInput({
    log: { prompt_tokens: 1, completion_tokens: 0, quota: 1 },
    other: { ...ratioInput().other, model_ratio: 0.000001 },
  });
  assert.equal(reconstruct(base.log, base.other).quota, 1);

  const local = {
    ...base,
    log: { ...base.log, quota: 0 },
    other: { ...base.other, admin_info: { local_count_tokens: true } },
  };
  assert.equal(reconstruct(local.log, local.other).quota, 0);

  const empty = {
    ...base,
    log: { prompt_tokens: 0, completion_tokens: 0, quota: 0 },
  };
  assert.equal(reconstruct(empty.log, empty.other).quota, 0);
});
