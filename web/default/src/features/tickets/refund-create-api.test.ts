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
import { api } from '@/lib/api'
import { createRefundTicket } from './api'

test('refund quote rejection is an error and cannot trigger successful creation', async () => {
  const original = api.post
  api.post = (async () => ({
    data: { success: false, message: 'invoice tax quote changed' },
  })) as typeof api.post
  try {
    await assert.rejects(
      createRefundTicket({
        subject: 'Refund',
        priority: 2,
        refund_quota: 500000,
        payee_type: 'alipay',
        payee_name: 'Test',
        payee_account: 'test',
        payee_bank: '',
        contact: 'test',
        reason: 'test',
        attachment_ids: [],
        request_key: 'test-key',
        finance_quote: 'stale-quote',
      }),
      /invoice tax quote changed/,
    )
  } finally {
    api.post = original
  }
})
