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

const translations = {
  en: 'Close for today',
  fr: "Fermer pour aujourd'hui",
  ja: '今日は閉じる',
  ru: 'Закрыть на сегодня',
  vi: 'Đóng cho hôm nay',
  zh: '今日关闭',
  'zh-TW': '今日關閉',
} as const

for (const [locale, expected] of Object.entries(translations)) {
  test(`${locale} translates Close for today`, () => {
    const document = JSON.parse(
      readFileSync(new URL(`./${locale}.json`, import.meta.url), 'utf8')
    ) as { translation?: Record<string, unknown> }

    assert.equal(document.translation?.['Close for today'], expected)
  })
}
