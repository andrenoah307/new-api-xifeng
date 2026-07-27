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
import { readFileSync } from 'node:fs'
import test from 'node:test'

import {
  reconstructBillingProcess,
  type BillingProcessInput,
  type BillingProcessOtherInput,
} from './billing-process.ts'

interface ProductionFixtureCase {
  id: string
  log: BillingProcessInput['log']
  other: BillingProcessOtherInput
  expected?: unknown
  expected_case?: string
}

interface ProductionFixture {
  quota_per_unit: number
  cases: ProductionFixtureCase[]
}

const fixture = JSON.parse(
  readFileSync(
    new URL(
      '../../../../../test-fixtures/billing-process/production.json',
      import.meta.url
    ),
    'utf8'
  )
) as ProductionFixture

const fixtureById = new Map(
  fixture.cases.map((entry) => [entry.id, entry] as const)
)

function encodeExpression(expression: string): string {
  return Buffer.from(expression, 'utf8').toString('base64')
}

function reconstruct(
  log: BillingProcessInput['log'],
  other: BillingProcessOtherInput,
  quotaPerUnit = fixture.quota_per_unit
) {
  return reconstructBillingProcess({ log, other, quotaPerUnit })
}

function ratioOther(
  overrides: BillingProcessOtherInput = {}
): BillingProcessOtherInput {
  return {
    model_price: -1,
    model_ratio: 1,
    completion_ratio: 1,
    group_ratio: 1,
    user_group_ratio: -1,
    cache_tokens: 0,
    cache_creation_tokens: 0,
    ...overrides,
  }
}

function tieredOther(
  expression: string,
  overrides: BillingProcessOtherInput = {}
): BillingProcessOtherInput {
  return {
    billing_mode: 'tiered_expr',
    expr_b64: encodeExpression(expression),
    matched_tier: 'standard',
    group_ratio: 1,
    user_group_ratio: -1,
    cache_tokens: 0,
    cache_creation_tokens: 0,
    ...overrides,
  }
}

test('reconstructs every shared production fixture exactly', () => {
  for (const entry of fixture.cases) {
    const referenced = entry.expected_case
      ? fixtureById.get(entry.expected_case)
      : undefined
    const expected = entry.expected ?? referenced?.expected
    assert.notEqual(expected, undefined)
    assert.deepStrictEqual(
      reconstruct(entry.log, entry.other),
      expected,
      entry.id
    )
  }
})

test('strict parser preserves coefficient direction, zero references, and string labels', () => {
  const coefficientFirst = reconstruct(
    { prompt_tokens: 100, completion_tokens: 0, quota: 45 },
    tieredOther('tier("standard", 0.5 * cr + p * 1 + c * 0)', {
      cache_tokens: 20,
    })
  )
  assert.equal(coefficientFirst.ok, true)
  if (!coefficientFirst.ok) return
  assert.equal(coefficientFirst.tokens.p, 80)
  assert.equal(coefficientFirst.expressionOutput, 90)

  const zeroReference = reconstruct(
    { prompt_tokens: 100, completion_tokens: 0, quota: 40 },
    tieredOther('tier("standard", cr * 0 + p * 1 + c * 0)', {
      cache_tokens: 20,
    })
  )
  assert.equal(zeroReference.ok, true)
  if (!zeroReference.ok) return
  assert.equal(zeroReference.tokens.p, 80)
  assert.ok(zeroReference.usedVariables.includes('cr'))

  const stringLabel = reconstruct(
    { prompt_tokens: 100, completion_tokens: 0, quota: 50 },
    tieredOther('tier("cr", p * 1 + c * 0)', {
      matched_tier: 'cr',
      cache_tokens: 20,
    })
  )
  assert.equal(stringLabel.ok, true)
  if (!stringLabel.ok) return
  assert.equal(stringLabel.tokens.p, 100)
  assert.equal(stringLabel.usedVariables.includes('cr'), false)
})

test('strict parser fully consumes expressions and rejects unsupported syntax', () => {
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
    'not-tier("standard", p * 1)',
    'tier("unterminated, p * 1)',
    'tier("\\x", p * 1)',
    'tier("standard", p * 1',
    'len < 100 ? tier("standard", p * 1)',
  ]
  for (const expression of expressions) {
    assert.deepStrictEqual(
      reconstruct(
        { prompt_tokens: 100, completion_tokens: 0, quota: 50 },
        tieredOther(expression)
      ),
      { ok: false, reason: 'unsupported_expression' }
    )
  }
})

test('strict parser accepts nested ternaries and records condition variables', () => {
  const expression =
    'len <= 50 ? tier("short", p * 1) : p > 50 && c >= 0 ? tier("standard", p * 1) : tier("other", p * 2)'
  const result = reconstruct(
    { prompt_tokens: 100, completion_tokens: 0, quota: 50 },
    tieredOther(expression)
  )
  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.deepStrictEqual(result.usedVariables, ['p', 'c', 'len'])

  const escapedLabel = reconstruct(
    { prompt_tokens: 100, completion_tokens: 0, quota: 50 },
    tieredOther(
      '50 < len ? tier("sta\\\"ndard", p * 1) : tier("other", p * 2)',
      { matched_tier: 'sta"ndard' }
    )
  )
  assert.equal(escapedLabel.ok, true)
})

test('tiered lookup and encoding failures are explicit', () => {
  const log = { prompt_tokens: 100, completion_tokens: 0, quota: 50 }
  const valid = tieredOther('tier("standard", p * 1)')
  const cases: Array<[BillingProcessOtherInput, string]> = [
    [{ ...valid, expr_b64: undefined }, 'invalid_expression_encoding'],
    [{ ...valid, expr_b64: '%%%' }, 'invalid_expression_encoding'],
    [
      { ...valid, expr_b64: encodeExpression('v2:tier("standard", p * 1)') },
      'unsupported_expression',
    ],
    [{ ...valid, matched_tier: undefined }, 'matched_tier_missing'],
    [{ ...valid, matched_tier: 'missing' }, 'matched_tier_not_found'],
    [
      {
        ...valid,
        expr_b64: encodeExpression(
          'len < 50 ? tier("standard", p * 1) : tier("standard", p * 1)'
        ),
      },
      'matched_tier_ambiguous',
    ],
    [
      { ...valid, expr_b64: encodeExpression('tier("standard", img * 1)') },
      'missing_token_dimension',
    ],
  ]
  for (const [other, reason] of cases) {
    assert.deepStrictEqual(reconstruct(log, other), { ok: false, reason })
  }
  assert.deepStrictEqual(reconstruct({ ...log, quota: 49 }, valid), {
    ok: false,
    reason: 'quota_mismatch',
  })
})

test('zero user ratio is authoritative', () => {
  const result = reconstruct(
    { prompt_tokens: 100, completion_tokens: 0, quota: 0 },
    tieredOther('tier("standard", p * 1)', {
      group_ratio: 3,
      user_group_ratio: 0,
    })
  )
  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.equal(result.effectiveGroupRatio, 0)
  assert.equal(result.groupRatioSource, 'user')
})

test('reconstructs supported OpenAI and Claude ratio token semantics', () => {
  const openAI = reconstruct(
    { prompt_tokens: 100, completion_tokens: 10, quota: 120 },
    ratioOther({
      model_ratio: 2,
      completion_ratio: 3,
      group_ratio: 0.5,
      cache_tokens: 20,
      cache_ratio: 0.5,
    })
  )
  assert.equal(openAI.ok, true)
  if (!openAI.ok) return
  assert.equal(openAI.mode, 'ratio_openai')
  assert.equal(openAI.tokens.p, 80)
  assert.equal(openAI.quota, 120)

  const claude = reconstruct(
    { prompt_tokens: 100, completion_tokens: 10, quota: 170 },
    ratioOther({
      claude: true,
      model_ratio: 2,
      completion_ratio: 3,
      group_ratio: 0.5,
      cache_tokens: 20,
      cache_ratio: 0.1,
      cache_creation_tokens: 30,
      cache_creation_ratio: 1.25,
    })
  )
  assert.equal(claude.ok, true)
  if (!claude.ok) return
  assert.equal(claude.mode, 'ratio_claude')
  assert.equal(claude.tokens.p, 100)
  assert.equal(claude.quotaBeforeRound, 169.5)
  assert.equal(claude.quota, 170)
})

test('reconstructs Claude split cache creation buckets', () => {
  const result = reconstruct(
    { prompt_tokens: 100, completion_tokens: 10, quota: 186 },
    ratioOther({
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
    })
  )
  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.deepStrictEqual(
    result.terms.map((term) => [term.kind, term.tokens]),
    [
      ['input', 100],
      ['cache_read', 20],
      ['cache_creation', 15],
      ['cache_creation_5m', 10],
      ['cache_creation_1h', 5],
      ['output', 10],
    ]
  )
  assert.equal(result.quota, 186)
})

test('unsupported billing paths return stable degradation reasons', () => {
  const log = { prompt_tokens: 100, completion_tokens: 10, quota: 110 }
  const cases: Array<[BillingProcessOtherInput, string]> = [
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
  ]
  for (const [extra, reason] of cases) {
    assert.deepStrictEqual(reconstruct(log, ratioOther(extra)), {
      ok: false,
      reason,
    })
  }
  assert.deepStrictEqual(reconstruct(log, ratioOther({ model_price: 0.01 })), {
    ok: false,
    reason: 'unsupported_per_call',
  })
  assert.deepStrictEqual(
    reconstruct(
      log,
      ratioOther({
        cache_creation_tokens_5m: 1,
        cache_creation_ratio_5m: 1,
      })
    ),
    { ok: false, reason: 'unsupported_cache_layout' }
  )
})

test('invalid and unverifiable numeric inputs fail closed', () => {
  const log = { prompt_tokens: 100, completion_tokens: 10, quota: 110 }
  assert.deepStrictEqual(reconstructBillingProcess(null), {
    ok: false,
    reason: 'invalid_input',
  })
  const cases: Array<
    [BillingProcessInput['log'], BillingProcessOtherInput, number, string]
  > = [
    [log, ratioOther(), 0, 'invalid_quota_per_unit'],
    [
      { ...log, prompt_tokens: -1 },
      ratioOther(),
      500000,
      'invalid_token_value',
    ],
    [
      { ...log, completion_tokens: 1.5 },
      ratioOther(),
      500000,
      'invalid_token_value',
    ],
    [{ ...log, quota: undefined }, ratioOther(), 500000, 'invalid_token_value'],
    [log, ratioOther({ group_ratio: -1 }), 500000, 'invalid_group_ratio'],
    [log, ratioOther({ user_group_ratio: -2 }), 500000, 'invalid_group_ratio'],
    [log, ratioOther({ model_ratio: -1 }), 500000, 'invalid_ratio'],
    [log, ratioOther({ completion_ratio: undefined }), 500000, 'invalid_ratio'],
    [
      log,
      ratioOther({ cache_tokens: 1, cache_ratio: undefined }),
      500000,
      'invalid_ratio',
    ],
    [log, ratioOther({ cache_tokens: -1 }), 500000, 'invalid_token_value'],
  ]
  for (const [caseLog, other, quotaPerUnit, reason] of cases) {
    assert.deepStrictEqual(reconstruct(caseLog, other, quotaPerUnit), {
      ok: false,
      reason,
    })
  }
})

test('ratio minimum-one behavior matches settlement', () => {
  const other = ratioOther({ model_ratio: 0.000001 })
  const upstream = reconstruct(
    { prompt_tokens: 1, completion_tokens: 0, quota: 1 },
    other
  )
  assert.equal(upstream.ok && upstream.quota, 1)

  const local = reconstruct(
    { prompt_tokens: 1, completion_tokens: 0, quota: 0 },
    { ...other, admin_info: { local_count_tokens: true } }
  )
  assert.equal(local.ok && local.quota, 0)

  const empty = reconstruct(
    { prompt_tokens: 0, completion_tokens: 0, quota: 0 },
    other
  )
  assert.equal(empty.ok && empty.quota, 0)
})
