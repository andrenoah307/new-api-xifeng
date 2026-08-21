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

import { api } from '@/lib/api'

import { getAllLogs, getLogStats, getUserInfo } from './api.ts'

test('usage log list and stats use a request-scoped 35 second timeout', async () => {
  const originalAdapter = api.defaults.adapter
  const observedTimeouts: Array<number | undefined> = []

  api.defaults.adapter = async (config) => {
    observedTimeouts.push(config.timeout)
    return {
      data: { success: true, data: {} },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }

  try {
    await getAllLogs()
    await getLogStats()
    await getUserInfo(1)
  } finally {
    api.defaults.adapter = originalAdapter
  }

  assert.deepEqual(observedTimeouts, [35_000, 35_000, 0])
})
