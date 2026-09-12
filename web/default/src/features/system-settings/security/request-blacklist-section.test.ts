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

import {
  parseRequestBlacklistDocument,
  requestBlacklistLinesToList,
  requestBlacklistListToLines,
  serializeRequestBlacklistDocument,
} from './request-blacklist'

test('empty request blacklist document uses disabled defaults', () => {
  assert.deepEqual(parseRequestBlacklistDocument(''), {
    mode: 'off',
    enabledGroups: [],
    contentWords: '',
    domains: '',
    blockMessage: '',
    blockStatusCode: 403,
  })
})

test('missing and zero block status codes display as 403', () => {
  assert.equal(
    parseRequestBlacklistDocument('{"mode":"observe"}').blockStatusCode,
    403
  )
  assert.equal(
    parseRequestBlacklistDocument('{"block_status_code":0}').blockStatusCode,
    403
  )
})

test('invalid block status code is preserved for the backend to reject', () => {
  const values = parseRequestBlacklistDocument(
    '{"mode":"enforce","block_status_code":500}'
  )
  assert.equal(values.blockStatusCode, 500)

  const serialized = JSON.parse(serializeRequestBlacklistDocument(values))
  assert.equal(serialized.block_status_code, 500)
})

test('empty enabled groups round-trip as all groups', () => {
  const values = parseRequestBlacklistDocument(
    '{"mode":"observe","enabled_groups":[]}'
  )
  assert.deepEqual(values.enabledGroups, [])

  const serialized = JSON.parse(serializeRequestBlacklistDocument(values))
  assert.deepEqual(serialized.enabled_groups, [])
})

test('multiline fields trim entries and ignore blank CRLF lines', () => {
  const list = requestBlacklistLinesToList(
    '  first  \r\n\r\n second\n\t\nthird  '
  )
  assert.deepEqual(list, ['first', 'second', 'third'])
  assert.equal(requestBlacklistListToLines(list), 'first\nsecond\nthird')

  const serialized = JSON.parse(
    serializeRequestBlacklistDocument({
      mode: 'enforce',
      enabledGroups: [],
      contentWords: ' alpha \r\n\r\n beta ',
      domains: ' example.com\r\n\r\n api.example.com ',
      blockMessage: '',
      blockStatusCode: 403,
    })
  )
  assert.deepEqual(serialized.content_words, ['alpha', 'beta'])
  assert.deepEqual(serialized.domains, ['example.com', 'api.example.com'])
})
