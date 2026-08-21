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

import { getPromptCacheSummary } from './cache-tokens.js';

const cases = [
  {
    name: 'keeps the residual when total creation exceeds split buckets',
    other: {
      cache_tokens: 12,
      cache_creation_tokens: 30,
      cache_creation_tokens_5m: 10,
      cache_creation_tokens_1h: 5,
    },
    expected: { cacheReadTokens: 12, cacheWriteTokens: 30 },
  },
  {
    name: 'uses the split sum when it exceeds total creation',
    other: {
      cache_creation_tokens: 10,
      cache_creation_tokens_5m: 8,
      cache_creation_tokens_1h: 7,
    },
    expected: { cacheReadTokens: 0, cacheWriteTokens: 15 },
  },
  {
    name: 'has no residual when total creation equals the split sum',
    other: {
      cache_creation_tokens: 15,
      cache_creation_tokens_5m: 10,
      cache_creation_tokens_1h: 5,
    },
    expected: { cacheReadTokens: 0, cacheWriteTokens: 15 },
  },
  {
    name: 'uses total creation without split buckets',
    other: { cache_creation_tokens: 30 },
    expected: { cacheReadTokens: 0, cacheWriteTokens: 30 },
  },
  {
    name: 'reports read-only cache usage',
    other: { cache_tokens: 25 },
    expected: { cacheReadTokens: 25, cacheWriteTokens: 0 },
  },
  {
    name: 'reports write-only cache usage',
    other: { cache_creation_tokens: 25 },
    expected: { cacheReadTokens: 0, cacheWriteTokens: 25 },
  },
  {
    name: 'reports no cache usage for an empty object',
    other: {},
    expected: null,
  },
  { name: 'reports no cache usage for null', other: null, expected: null },
  {
    name: 'reports no cache usage for undefined',
    other: undefined,
    expected: null,
  },
];

describe('getPromptCacheSummary', () => {
  for (const fixture of cases) {
    test(fixture.name, () => {
      assert.deepEqual(getPromptCacheSummary(fixture.other), fixture.expected);
    });
  }

  test('normalizes every invalid token field independently', () => {
    const invalidValues = [
      undefined,
      null,
      Number.NaN,
      Number.POSITIVE_INFINITY,
      -1,
      'not-a-number',
      Number.MAX_SAFE_INTEGER + 1,
    ];

    for (const value of invalidValues) {
      assert.equal(
        getPromptCacheSummary({
          cache_tokens: value,
          cache_creation_tokens: value,
          cache_creation_tokens_5m: value,
          cache_creation_tokens_1h: value,
        }),
        null,
      );
    }
  });

  test('never reads the derived cache_write_tokens field', () => {
    assert.equal(getPromptCacheSummary({ cache_write_tokens: 99 }), null);
  });
});
