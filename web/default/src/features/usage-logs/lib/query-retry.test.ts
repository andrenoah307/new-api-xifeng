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

import { shouldRetryUsageLogQuery } from './utils.ts'

test('usage log queries retry only the first axios failure without an HTTP response', () => {
  const tests = [
    {
      name: 'axios failure without response or code retries once',
      failureCount: 0,
      error: { isAxiosError: true },
      want: true,
    },
    {
      name: 'second network failure stops retrying',
      failureCount: 1,
      error: { isAxiosError: true },
      want: false,
    },
    {
      name: 'HTTP response is not retried',
      failureCount: 0,
      error: { isAxiosError: true, response: { status: 503 } },
      want: false,
    },
    {
      name: 'non-axios error is not retried',
      failureCount: 0,
      error: new Error('query failed'),
      want: false,
    },
    {
      name: 'ECONNABORTED client timeout is not retried',
      failureCount: 0,
      error: { isAxiosError: true, code: 'ECONNABORTED' },
      want: false,
    },
    {
      name: 'ETIMEDOUT client timeout is not retried',
      failureCount: 0,
      error: { isAxiosError: true, code: 'ETIMEDOUT' },
      want: false,
    },
    {
      name: 'ERR_CANCELED request is not retried',
      failureCount: 0,
      error: { isAxiosError: true, code: 'ERR_CANCELED' },
      want: false,
    },
    {
      name: 'ERR_NETWORK failure retries once',
      failureCount: 0,
      error: { isAxiosError: true, code: 'ERR_NETWORK' },
      want: true,
    },
  ]

  for (const testCase of tests) {
    assert.equal(
      shouldRetryUsageLogQuery(testCase.failureCount, testCase.error),
      testCase.want,
      testCase.name
    )
  }
})
