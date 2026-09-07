/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { after, describe, test } from 'node:test'

import { downloadTextFile } from './download'

const globalRecord = globalThis as typeof globalThis & Record<string, unknown>
const originalDocument = globalRecord.document
const originalCreateObjectURL = URL.createObjectURL
const originalRevokeObjectURL = URL.revokeObjectURL
const anchors: Array<{ download: string; clickCount: number }> = []
const revokedUrls: string[] = []

Object.defineProperty(globalRecord, 'document', {
  configurable: true,
  value: {
    createElement: () => {
      const anchor = { download: '', clickCount: 0 }
      anchors.push(anchor)
      return {
        href: '',
        get download() {
          return anchor.download
        },
        set download(value: string) {
          anchor.download = value
        },
        click: () => {
          anchor.clickCount++
        },
      }
    },
  },
})
Object.defineProperty(URL, 'createObjectURL', {
  configurable: true,
  value: () => 'blob:test',
})
Object.defineProperty(URL, 'revokeObjectURL', {
  configurable: true,
  value: (url: string) => {
    revokedUrls.push(url)
  },
})

after(() => {
  if (originalDocument === undefined) {
    Reflect.deleteProperty(globalRecord, 'document')
  } else {
    Object.defineProperty(globalRecord, 'document', {
      configurable: true,
      value: originalDocument,
    })
  }
  Object.defineProperty(URL, 'createObjectURL', {
    configurable: true,
    value: originalCreateObjectURL,
  })
  Object.defineProperty(URL, 'revokeObjectURL', {
    configurable: true,
    value: originalRevokeObjectURL,
  })
})

describe('downloadTextFile', () => {
  test('sanitizes filenames and releases the object URL after the click', async () => {
    const cases = [
      { filename: ' path/to\\codes.txt ', expected: 'pathtocodes.txt' },
      { filename: 'codes\u0000\u001f.txt', expected: 'codes.txt' },
      { filename: '   ', expected: 'download.txt' },
      { filename: '/\\\u0000\t', expected: 'download.txt' },
    ]

    for (const currentCase of cases) {
      downloadTextFile('code', currentCase.filename)
      const anchor = anchors.at(-1)
      assert.ok(anchor)
      assert.equal(anchor.download, currentCase.expected)
      assert.equal(anchor.clickCount, 1)
    }

    // Revoking while the click is still being dispatched cancels the download
    // in some browsers, so the URL must survive the synchronous call.
    assert.deepEqual(revokedUrls, [])

    await new Promise((resolve) => setTimeout(resolve, 0))

    assert.deepEqual(
      revokedUrls,
      cases.map(() => 'blob:test')
    )
  })
})
