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
  'Excluded User Groups',
  'Callers in these user groups can never be routed to this channel, whichever group they call through. Use it when the price for that user group is below this channel cost.',
  'Leave empty to exclude nobody',
]

for (const locale of locales) {
  test(`${locale} has complete excluded user group copy`, () => {
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
