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
import { describe, test } from 'node:test'

import { formatLogQuota } from '@/lib/format'

import { renderAuditContent } from './format.ts'

const translate = (
  key: string,
  options: Record<string, unknown> = {}
): string =>
  key.replaceAll(/{{(\w+)}}/g, (placeholder, name: string) =>
    Object.hasOwn(options, name)
      ? String(options[name])
      : placeholder
  )

describe('renderAuditContent quota actions', () => {
  test('formats numeric add and subtract quotas with the log quota formatter', () => {
    const quota = 5_000_000
    const formattedQuota = formatLogQuota(quota)

    assert.equal(
      renderAuditContent(
        { op: { action: 'user.quota_add', params: { quota } } },
        translate
      ),
      `Administrator increased account quota by ${formattedQuota}`
    )
    assert.equal(
      renderAuditContent(
        { op: { action: 'user.quota_subtract', params: { quota } } },
        translate
      ),
      `Administrator decreased account quota by ${formattedQuota}`
    )
  })

  test('formats numeric override endpoints with the log quota formatter', () => {
    const from = 500_000
    const to = 5_000_000

    assert.equal(
      renderAuditContent(
        { op: { action: 'user.quota_override', params: { from, to } } },
        translate
      ),
      `Administrator changed account quota from ${formatLogQuota(from)} to ${formatLogQuota(to)}`
    )
  })

  test('preserves historical string quota parameters verbatim', () => {
    const quota = '¥10.000000 额度'
    const from = '$1.250000 quota'
    const to = '10 credits'

    assert.equal(
      renderAuditContent(
        { op: { action: 'user.quota_add', params: { quota } } },
        translate
      ),
      `Administrator increased account quota by ${quota}`
    )
    assert.equal(
      renderAuditContent(
        { op: { action: 'user.quota_subtract', params: { quota } } },
        translate
      ),
      `Administrator decreased account quota by ${quota}`
    )
    assert.equal(
      renderAuditContent(
        { op: { action: 'user.quota_override', params: { from, to } } },
        translate
      ),
      `Administrator changed account quota from ${from} to ${to}`
    )
  })

  test('keeps unknown actions and missing parameters on their existing fallbacks', () => {
    assert.equal(
      renderAuditContent(
        { op: { action: 'user.quota_unknown', params: { quota: 5_000_000 } } },
        translate
      ),
      null
    )
    assert.equal(
      renderAuditContent(
        { op: { action: 'user.quota_add', params: {} } },
        translate
      ),
      'Administrator increased account quota by {{quota}}'
    )
  })
})
