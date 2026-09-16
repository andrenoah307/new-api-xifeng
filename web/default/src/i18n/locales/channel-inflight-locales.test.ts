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
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const locales = ['en', 'fr', 'ja', 'ru', 'vi', 'zh', 'zh-TW']
const requiredKeys = [
  'In flight',
  'Awaiting first byte',
  'Oldest',
  'Clearable',
  'Timeouts',
  'Clear Stalled Connections',
  'Only connections that already outlived the gateway timeout are closed. Connections still inside their timeout are never touched.',
  'Nothing to clear. Zero clearable connections is the normal state.',
  'A class without a configured timeout has no overdue threshold, so its connections can never be cleared.',
  'Connection tracking is degraded on this instance, so the counts may be incomplete.',
  'Counts cover only the instance answering this request.',
  'Cleared {{total}} stalled connections',
  // Reused by the confirmation dialog; a locale that lost them would render blanks.
  'Not configured',
  'Confirm Cleanup',
  'Cleanup failed',
]

for (const locale of locales) {
  test(`${locale} has complete channel inflight cleanup copy`, () => {
    const document = JSON.parse(
      readFileSync(new URL(`./${locale}.json`, import.meta.url), 'utf8')
    ) as { translation?: Record<string, string> }

    for (const key of requiredKeys) {
      const value = document.translation?.[key]
      assert.equal(typeof value, 'string', `${locale} is missing ${key}`)
      assert.notEqual(String(value).trim(), '')
    }
  })
}

test('the cleared-connections toast avoids the i18next plural trigger', () => {
  // An interpolation named `count` makes i18next resolve `key_one`/`key_other`
  // first, which these flat locale files do not define.
  const key = 'Cleared {{total}} stalled connections'
  assert.equal(key.includes('{{count}}'), false)

  for (const locale of locales) {
    const document = JSON.parse(
      readFileSync(new URL(`./${locale}.json`, import.meta.url), 'utf8')
    ) as { translation?: Record<string, string> }
    const value = String(document.translation?.[key])
    assert.equal(
      value.includes('{{total}}'),
      true,
      `${locale} dropped the {{total}} placeholder`
    )
  }
})
