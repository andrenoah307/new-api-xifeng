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
import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { formatReportAmount } from './format'

describe('finance report presentation', () => {
  it('preserves large decimal amounts without converting to floating point', () => {
    assert.equal(formatReportAmount('9007199254740993.1234564', 'cost'),
      `${9007199254740993n.toLocaleString()}.123456`,
    )
    assert.equal(formatReportAmount('999.9999995', 'cost'),
      `${1000n.toLocaleString()}.000000`,
    )
    assert.equal(formatReportAmount('-0.0000005', 'profit'), '-0.000001')
  })
  it('distinguishes missing profit, zero cost, and exact percentages', () => {
    assert.equal(formatReportAmount(null, 'profit'), '-')
    assert.equal(formatReportAmount('0', 'cost'), '0.000000')
    assert.equal(formatReportAmount('0.123456', 'coverage'), '12.35%')
    assert.equal(formatReportAmount('-0.013456', 'profit_margin'), '-1.35%')
  })
})
