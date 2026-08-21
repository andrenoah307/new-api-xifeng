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
import test from 'node:test'

import { resolveUsageLogQueryTimeoutMs } from './utils.ts'

test('resolves the usage log query timeout from status seconds', () => {
  const tests: Array<{ name: string; raw: unknown; want: number }> = [
    { name: '60 seconds', raw: 60, want: 65_000 },
    { name: '30 seconds', raw: 30, want: 35_000 },
    { name: 'zero disables the client timeout', raw: 0, want: 0 },
    { name: 'negative disables the client timeout', raw: -1, want: 0 },
    { name: 'maximum timeout', raw: 600, want: 605_000 },
    { name: 'timeout above maximum is clamped', raw: 1200, want: 605_000 },
    { name: 'numeric string', raw: '60', want: 65_000 },
    { name: 'missing value', raw: undefined, want: 35_000 },
    { name: 'null value', raw: null, want: 35_000 },
    { name: 'NaN value', raw: Number.NaN, want: 35_000 },
    { name: 'infinite value', raw: Number.POSITIVE_INFINITY, want: 35_000 },
    { name: 'non-numeric string', raw: 'abc', want: 35_000 },
    { name: 'object value', raw: {}, want: 35_000 },
  ]

  for (const testCase of tests) {
    assert.equal(
      resolveUsageLogQueryTimeoutMs(testCase.raw),
      testCase.want,
      testCase.name
    )
  }
})
