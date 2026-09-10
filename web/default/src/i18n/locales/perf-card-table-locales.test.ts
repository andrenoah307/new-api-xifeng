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

// 表头列标签：宽度由最长的一列决定，一旦某个语言写成整句，
// 模型性能表就会退回到本轮要消灭的无意义换行。
const headerKeys = [
  'Model',
  'First token latency short',
  'Latency',
  'Throughput short',
  'Model success rate short',
  'Trend',
]

// 表头 title 里承载的完整语义，长度不受限，但必须存在。
const tooltipKeys = ['First token latency', 'Throughput', 'Model success rate']

const maxHeaderLength = 12

for (const locale of locales) {
  test(`${locale} keeps model performance table headers short`, () => {
    const document = JSON.parse(
      readFileSync(new URL(`./${locale}.json`, import.meta.url), 'utf8')
    ) as { translation?: Record<string, string> }

    for (const key of headerKeys) {
      const value = document.translation?.[key]
      assert.equal(typeof value, 'string', `${locale} is missing ${key}`)
      const text = String(value).trim()
      assert.notEqual(text, '')
      assert.ok(
        text.length <= maxHeaderLength,
        `${locale} header ${key} is ${text.length} chars ("${text}"), max ${maxHeaderLength}`
      )
    }

    for (const key of tooltipKeys) {
      const value = document.translation?.[key]
      assert.equal(typeof value, 'string', `${locale} is missing ${key}`)
      assert.notEqual(String(value).trim(), '')
    }
  })
}
