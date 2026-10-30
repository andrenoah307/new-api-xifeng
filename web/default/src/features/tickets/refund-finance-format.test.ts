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
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { refundCash, refundQuota } from './refund-finance-format'

test('refund amounts retain exact cents and reject unsafe numbers', () => {
  assert.equal(refundCash(9007199254740991), '¥90071992547409.91')
  assert.equal(refundCash(1), '¥0.01')
  assert.equal(refundCash(0), '¥0.00')
  assert.equal(refundCash(9007199254740992), '-')
  assert.equal(refundCash(-1), '-')
})
test('refund quotas use immutable decimal quota unit snapshots', () => {
  assert.equal(refundQuota(500000, '500000'), '1.000000')
  assert.equal(refundQuota(3, '0.5'), '6.000000')
  assert.equal(refundQuota(1, '3'), '0.333333')
  assert.equal(refundQuota(2, '3'), '0.666667')
  assert.equal(refundQuota(1, '0'), '-')
})
