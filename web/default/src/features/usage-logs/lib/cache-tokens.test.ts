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

import type { LogOtherData } from '../types'
import { reconstructBillingProcess } from './billing-process.ts'
import {
  summarizeCacheTokens,
  type CacheTokenSummary,
} from './cache-tokens.ts'

interface SummaryCase {
  name: string
  other: LogOtherData | null | undefined
  expected: CacheTokenSummary
}

const summaryCases: SummaryCase[] = [
  {
    name: 'keeps the residual when total creation exceeds split buckets',
    other: {
      cache_tokens: 12,
      cache_creation_tokens: 30,
      cache_creation_tokens_5m: 10,
      cache_creation_tokens_1h: 5,
    },
    expected: {
      read: 12,
      writeGeneric: 15,
      write5m: 10,
      write1h: 5,
      writeTotal: 30,
      hasAny: true,
    },
  },
  {
    name: 'uses the split sum when it exceeds total creation',
    other: {
      cache_creation_tokens: 10,
      cache_creation_tokens_5m: 8,
      cache_creation_tokens_1h: 7,
    },
    expected: {
      read: 0,
      writeGeneric: 0,
      write5m: 8,
      write1h: 7,
      writeTotal: 15,
      hasAny: true,
    },
  },
  {
    name: 'has no residual when total creation equals the split sum',
    other: {
      cache_creation_tokens: 15,
      cache_creation_tokens_5m: 10,
      cache_creation_tokens_1h: 5,
    },
    expected: {
      read: 0,
      writeGeneric: 0,
      write5m: 10,
      write1h: 5,
      writeTotal: 15,
      hasAny: true,
    },
  },
  {
    name: 'uses total creation as generic creation without split buckets',
    other: { cache_creation_tokens: 30 },
    expected: {
      read: 0,
      writeGeneric: 30,
      write5m: 0,
      write1h: 0,
      writeTotal: 30,
      hasAny: true,
    },
  },
  {
    name: 'reports read-only cache usage',
    other: { cache_tokens: 25 },
    expected: {
      read: 25,
      writeGeneric: 0,
      write5m: 0,
      write1h: 0,
      writeTotal: 0,
      hasAny: true,
    },
  },
  {
    name: 'reports write-only cache usage',
    other: { cache_creation_tokens: 25 },
    expected: {
      read: 0,
      writeGeneric: 25,
      write5m: 0,
      write1h: 0,
      writeTotal: 25,
      hasAny: true,
    },
  },
  {
    name: 'reports no cache usage for an empty object',
    other: {},
    expected: {
      read: 0,
      writeGeneric: 0,
      write5m: 0,
      write1h: 0,
      writeTotal: 0,
      hasAny: false,
    },
  },
  {
    name: 'reports no cache usage for null',
    other: null,
    expected: {
      read: 0,
      writeGeneric: 0,
      write5m: 0,
      write1h: 0,
      writeTotal: 0,
      hasAny: false,
    },
  },
  {
    name: 'reports no cache usage for undefined',
    other: undefined,
    expected: {
      read: 0,
      writeGeneric: 0,
      write5m: 0,
      write1h: 0,
      writeTotal: 0,
      hasAny: false,
    },
  },
]

describe('summarizeCacheTokens', () => {
  for (const fixture of summaryCases) {
    test(fixture.name, () => {
      const summary = summarizeCacheTokens(fixture.other)

      assert.deepEqual(summary, fixture.expected)
      assert.equal(
        summary.writeTotal,
        summary.writeGeneric + summary.write5m + summary.write1h
      )
      assert.ok(summary.writeTotal >= 0)
    })
  }

  test('normalizes every invalid token field independently', () => {
    const invalidValues: unknown[] = [
      undefined,
      null,
      Number.NaN,
      Number.POSITIVE_INFINITY,
      -1,
      'not-a-number',
      Number.MAX_SAFE_INTEGER + 1,
    ]

    for (const value of invalidValues) {
      const summary = summarizeCacheTokens({
        cache_tokens: value,
        cache_creation_tokens: value,
        cache_creation_tokens_5m: value,
        cache_creation_tokens_1h: value,
      } as unknown as LogOtherData)

      assert.deepEqual(summary, {
        read: 0,
        writeGeneric: 0,
        write5m: 0,
        write1h: 0,
        writeTotal: 0,
        hasAny: false,
      })
      assert.equal(
        summary.writeTotal,
        summary.writeGeneric + summary.write5m + summary.write1h
      )
      assert.ok(summary.writeTotal >= 0)
    }
  })

  test('never reads the derived cache_write_tokens field', () => {
    const summary = summarizeCacheTokens({
      cache_write_tokens: 99,
    } as unknown as LogOtherData)

    assert.equal(summary.writeTotal, 0)
    assert.equal(summary.hasAny, false)
  })
})

test('ratio billing cache creation terms use the same write total', () => {
  const other: LogOtherData = {
    claude: true,
    model_price: -1,
    model_ratio: 2,
    completion_ratio: 3,
    group_ratio: 0.5,
    user_group_ratio: -1,
    cache_tokens: 20,
    cache_ratio: 0.1,
    cache_creation_tokens: 30,
    cache_creation_ratio: 1.25,
    cache_creation_tokens_5m: 10,
    cache_creation_ratio_5m: 2,
    cache_creation_tokens_1h: 5,
    cache_creation_ratio_1h: 3,
  }
  const result = reconstructBillingProcess({
    log: { prompt_tokens: 100, completion_tokens: 10, quota: 186 },
    other,
    quotaPerUnit: 500_000,
  })

  assert.equal(result.ok, true)
  if (!result.ok) return

  const billedWriteTokens = result.terms
    .filter((term) => term.kind.startsWith('cache_creation'))
    .reduce((total, term) => total + term.tokens, 0)
  assert.equal(summarizeCacheTokens(other).writeTotal, billedWriteTokens)
})

test('tiered Claude billing intentionally does not charge the generic residual', () => {
  const other: LogOtherData = {
    billing_mode: 'tiered_expr',
    expr_b64: Buffer.from(
      'tier("standard", cc * 1 + cc1h * 1)',
      'utf8'
    ).toString('base64'),
    matched_tier: 'standard',
    claude: true,
    group_ratio: 1,
    user_group_ratio: -1,
    cache_creation_tokens: 30,
    cache_creation_tokens_5m: 10,
    cache_creation_tokens_1h: 5,
  }
  const result = reconstructBillingProcess({
    log: { prompt_tokens: 100, completion_tokens: 0, quota: 15 },
    other,
    quotaPerUnit: 1_000_000,
  })

  assert.equal(result.ok, true)
  if (!result.ok) return

  const billedWriteTokens = result.terms
    .filter((term) => term.kind.startsWith('cache_creation'))
    .reduce((total, term) => total + term.tokens, 0)
  assert.equal(billedWriteTokens, 15)
  assert.equal(summarizeCacheTokens(other).writeTotal, 30)
  assert.notEqual(summarizeCacheTokens(other).writeTotal, billedWriteTokens)
})
