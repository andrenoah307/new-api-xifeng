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
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  createEmptyRule,
  deduplicateErrors,
  hasCondition,
  normalizeRule,
  normalizeStatusCodes,
  normalizeStringList,
  parseErrorLog,
  parseRules,
  stripErrorContentPrefix,
} from './error-filter'

describe('error filter rule normalization', () => {
  test('fills omitted persisted fields with safe defaults', () => {
    assert.deepEqual(normalizeRule({ action: 'rewrite' }), {
      status_codes: [],
      message_contains: [],
      error_codes: [],
      action: 'rewrite',
      rewrite_message: '',
      replace_status_code: 200,
      replace_message: '',
    })
  })

  test('creates independent empty rules', () => {
    const first = createEmptyRule()
    const second = createEmptyRule()

    first.status_codes.push(429)

    assert.deepEqual(second, {
      status_codes: [],
      message_contains: [],
      error_codes: [],
      action: 'retry',
      rewrite_message: '',
      replace_status_code: 200,
      replace_message: '',
    })
  })

  test('trims, removes empty values, and preserves first duplicate', () => {
    assert.deepEqual(
      normalizeStringList(['  rate limit  ', '', 'rate limit', ' timeout ']),
      ['rate limit', 'timeout']
    )
  })

  test('normalizes delimited status code input and rejects invalid values', () => {
    assert.deepEqual(normalizeStatusCodes('200, 404，500'), [200, 404, 500])
    assert.deepEqual(
      normalizeStatusCodes(['99', '600', 'abc', '4.5', '404.5']),
      []
    )
  })

  test('parses only arrays and never throws for malformed persisted JSON', () => {
    for (const value of [undefined, '', 'null', '{}', '{bad json']) {
      assert.doesNotThrow(() => parseRules(value))
      assert.deepEqual(parseRules(value), [])
    }

    assert.deepEqual(parseRules('[1,2]'), [
      createEmptyRule(),
      createEmptyRule(),
    ])

    assert.deepEqual(parseRules('[{"action":"replace"}]'), [
      {
        status_codes: [],
        message_contains: [],
        error_codes: [],
        action: 'replace',
        rewrite_message: '',
        replace_status_code: 200,
        replace_message: '',
      },
    ])
  })

  test('uses backend no-condition semantics', () => {
    assert.equal(hasCondition(createEmptyRule()), false)
    assert.equal(
      hasCondition({
        ...createEmptyRule(),
        message_contains: ['  upstream unavailable  '],
      }),
      true
    )
    assert.equal(
      hasCondition({ ...createEmptyRule(), status_codes: [503] }),
      true
    )
    assert.equal(
      hasCondition({ ...createEmptyRule(), error_codes: ['upstream_error'] }),
      true
    )
    assert.equal(
      hasCondition({
        ...createEmptyRule(),
        message_contains: ['   '],
        error_codes: [''],
      }),
      false
    )
  })
})

describe('error log helpers', () => {
  test('strips status prefix and all proxy id suffix formats', () => {
    assert.equal(
      stripErrorContentPrefix(
        'status_code=429, upstream failed (request id: abc) (request_ori_id: def) （traceid: ghi）'
      ),
      'upstream failed'
    )
    assert.equal(stripErrorContentPrefix(undefined), '')
  })

  test('parses error metadata while tolerating malformed other JSON', () => {
    assert.deepEqual(
      parseErrorLog({
        id: 7,
        created_at: 123,
        content: 'status_code=500, failed',
        model_name: 'gpt-test',
        other: JSON.stringify({
          status_code: '500',
          error_code: 'server_error',
          error_type: 'upstream',
        }),
      }),
      {
        id: 7,
        createdAt: 123,
        content: 'status_code=500, failed',
        modelName: 'gpt-test',
        statusCode: 500,
        errorCode: 'server_error',
        errorType: 'upstream',
      }
    )
    assert.deepEqual(
      parseErrorLog({ id: 8, other: '{bad json', content: null }),
      {
        id: 8,
        createdAt: undefined,
        content: '',
        modelName: '',
        statusCode: null,
        errorCode: '',
        errorType: '',
      }
    )
    assert.equal(
      parseErrorLog({
        other: { status_code: 503, error_code: 'overloaded' },
      }).statusCode,
      503
    )
    assert.equal(parseErrorLog(null).statusCode, null)
  })

  test('deduplicates by status, error code, and content while keeping first item', () => {
    const first = parseErrorLog({
      id: 1,
      content: 'same',
      other: JSON.stringify({ status_code: 500, error_code: 'E' }),
    })
    const duplicate = { ...first, id: 2 }
    const distinctStatus = { ...first, id: 3, statusCode: 502 }

    assert.deepEqual(deduplicateErrors([first, duplicate, distinctStatus]), [
      first,
      distinctStatus,
    ])
  })
})
