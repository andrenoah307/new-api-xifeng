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
import { test } from 'node:test'

import { reportCSV } from './export'
import type { FinanceJob, FinanceRow } from './types'

test('CSV preserves precise amounts, escapes channel text, and leaves unknown profit blank', () => {
  const job = {
    id: 'task',
    status: 'completed',
    start: 100,
    end: 200,
    cursor: 200,
  } as FinanceJob
  const row = {
    channel_id: 7,
    channel_name: ' \t=SUM(1,2)"渠道',
    consume_amount: '9007199254740993.123456789012345678',
    profit: '-0.123456789012345678',
    profit_margin: null,
  } as FinanceRow
  const csv = reportCSV(job, [row])
  assert.ok(csv.startsWith('\ufeff'))
  assert.ok(csv.includes('"\' \t=SUM(1,2)""渠道"'))
  assert.ok(csv.includes('"9007199254740993.123456789012345678"'))
  assert.ok(csv.includes('"-0.123456789012345678",""'))
  const partial = reportCSV({ ...job, status: 'running' }, [row])
  assert.ok(!partial.includes('-0.123456789012345678'))
  assert.equal(reportCSV(job, []).split('\r\n').length, 2)
  assert.ok(
    reportCSV(job, [{ ...row, channel_name: '\u0001@formula' }]).includes(
      "'\u0001@formula"
    )
  )
})
