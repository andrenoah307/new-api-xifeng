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

import type { CommonLogFilters } from '../types.ts'
import {
  buildExportParams,
  buildOfflineExportFilters,
} from './export-params.ts'

const startTime = new Date(1_700_000_000_999)
const endTime = new Date(1_700_000_123_456)

test('converts milliseconds to floored seconds and maps every export filter', () => {
  const filters: CommonLogFilters = {
    startTime,
    endTime,
    model: 'gpt-5',
    token: 'production-token',
    group: 'default',
    username: 'admin-user',
    channel: '42',
    requestId: 'req-123',
    upstreamRequestId: 'upstream-456',
  }

  assert.deepEqual(buildExportParams(filters, '2', true), {
    start_timestamp: 1_700_000_000,
    end_timestamp: 1_700_000_123,
    type: 2,
    model_name: 'gpt-5',
    token_name: 'production-token',
    group: 'default',
    request_id: 'req-123',
    upstream_request_id: 'upstream-456',
    username: 'admin-user',
    channel: 42,
  })
})

const missingTimeCases: Array<{
  name: string
  filters: CommonLogFilters
}> = [
  {
    name: 'missing start time',
    filters: { endTime },
  },
  {
    name: 'missing end time',
    filters: { startTime },
  },
  {
    name: 'missing both times',
    filters: {},
  },
]

for (const testCase of missingTimeCases) {
  test(`returns null when ${testCase.name}`, () => {
    assert.equal(buildExportParams(testCase.filters, '2', true), null)
  })
}

test('removes admin-only filters for non-admin exports', () => {
  const filters: CommonLogFilters = {
    startTime,
    endTime,
    username: 'admin-user',
    channel: '42',
    model: 'gpt-5',
  }

  assert.deepEqual(buildExportParams(filters, '2', false), {
    start_timestamp: 1_700_000_000,
    end_timestamp: 1_700_000_123,
    type: 2,
    model_name: 'gpt-5',
  })
})

const logTypeCases = [
  {
    name: 'all log types',
    logType: '0',
    expected: {
      start_timestamp: 1_700_000_000,
      end_timestamp: 1_700_000_123,
    },
  },
  {
    name: 'consumption logs',
    logType: '2',
    expected: {
      start_timestamp: 1_700_000_000,
      end_timestamp: 1_700_000_123,
      type: 2,
    },
  },
] as const

for (const testCase of logTypeCases) {
  test(`maps ${testCase.name}`, () => {
    assert.deepEqual(
      buildExportParams({ startTime, endTime }, testCase.logType, false),
      testCase.expected
    )
  })
}

test('omits empty string filters', () => {
  const filters: CommonLogFilters = {
    startTime,
    endTime,
    model: '',
    token: '',
    group: '',
    username: '',
    channel: '',
    requestId: '',
    upstreamRequestId: '',
  }

  assert.deepEqual(buildExportParams(filters, undefined, true), {
    start_timestamp: 1_700_000_000,
    end_timestamp: 1_700_000_123,
  })
})

test('includes the selected log type in offline export filters, including all types', () => {
  assert.deepEqual(
    buildOfflineExportFilters({ startTime, endTime }, '3'),
    {
      start_timestamp: 1_700_000_000,
      end_timestamp: 1_700_000_123,
      type: 3,
    }
  )
  assert.deepEqual(
    buildOfflineExportFilters({ startTime, endTime }, '0'),
    {
      start_timestamp: 1_700_000_000,
      end_timestamp: 1_700_000_123,
      type: 0,
    }
  )
})
