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

import { buildLogStatsQueryKey, getLogStatsSearchParams } from './utils.ts'

test('stats search params exclude pagination while retaining every filter', () => {
  const searchParams = {
    page: 4,
    pageSize: 100,
    type: ['2'],
    model: 'gpt-5',
    requestId: 'req-1',
  }

  assert.deepEqual(getLogStatsSearchParams(searchParams), {
    type: ['2'],
    model: 'gpt-5',
    requestId: 'req-1',
  })
})

test('stats query key is stable across page and page size changes', () => {
  const first = buildLogStatsQueryKey(true, {
    page: 1,
    pageSize: 20,
    group: 'default',
  })
  const second = buildLogStatsQueryKey(true, {
    page: 8,
    pageSize: 100,
    group: 'default',
  })
  const changedFilter = buildLogStatsQueryKey(true, {
    page: 8,
    pageSize: 100,
    group: 'premium',
  })

  assert.deepEqual(first, second)
  assert.notDeepEqual(first, changedFilter)
})
