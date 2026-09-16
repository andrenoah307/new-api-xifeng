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

import type { ChannelInflight, ChannelInflightRuntime } from '../types'
import {
  countAwaitingFirstByte,
  formatInflightAge,
  indexInflightByChannel,
} from './inflight-utils'

function inflight(overrides: Partial<ChannelInflight> = {}): ChannelInflight {
  return {
    channel_id: 1,
    in_flight: 0,
    awaiting_headers: 0,
    awaiting_first_chunk: 0,
    oldest_age_ms: 0,
    cancellable: 0,
    by_reason: {},
    ...overrides,
  }
}

describe('formatInflightAge', () => {
  test('renders an age the way a table cell can hold it', () => {
    const cases: Array<{ input: number; expected: string }> = [
      { input: 0, expected: '0s' },
      { input: -1, expected: '0s' },
      { input: Number.NaN, expected: '0s' },
      { input: Number.POSITIVE_INFINITY, expected: '0s' },
      { input: 999, expected: '0s' },
      { input: 1000, expected: '1s' },
      { input: 59_999, expected: '59s' },
      { input: 60_000, expected: '1m0s' },
      { input: 305_000, expected: '5m5s' },
      { input: 3_599_000, expected: '59m59s' },
      { input: 3_600_000, expected: '1h0m' },
      { input: 7_530_000, expected: '2h5m' },
    ]
    for (const { input, expected } of cases) {
      assert.equal(formatInflightAge(input), expected, `age ${input}`)
    }
  })
})

describe('countAwaitingFirstByte', () => {
  test('sums both pre-first-byte classes', () => {
    assert.equal(
      countAwaitingFirstByte(
        inflight({ awaiting_headers: 3, awaiting_first_chunk: 4 })
      ),
      7
    )
  })

  test('never lets a negative counter subtract from the total', () => {
    assert.equal(
      countAwaitingFirstByte(
        inflight({ awaiting_headers: -5, awaiting_first_chunk: 2 })
      ),
      2
    )
  })
})

describe('indexInflightByChannel', () => {
  test('keys the bulk runtime payload by channel id', () => {
    const runtime: ChannelInflightRuntime = {
      channels: [
        inflight({ channel_id: 7, in_flight: 2 }),
        inflight({ channel_id: 11, in_flight: 5 }),
      ],
      thresholds: {
        stream_response_header_s: 300,
        streaming_s: 300,
        non_stream_s: 900,
      },
      threshold_enabled: {
        stream_response_header: true,
        streaming: true,
        non_stream: true,
      },
      scope: 'local_instance',
      tracking_degraded: false,
    }

    const indexed = indexInflightByChannel(runtime)
    assert.deepEqual(Object.keys(indexed).sort(), ['11', '7'])
    assert.equal(indexed['7'].in_flight, 2)
    assert.equal(indexed['11'].in_flight, 5)
  })

  test('an absent or empty payload indexes to nothing rather than throwing', () => {
    assert.deepEqual(indexInflightByChannel(undefined), {})
    assert.deepEqual(
      indexInflightByChannel({
        channels: [],
        thresholds: {
          stream_response_header_s: 0,
          streaming_s: 0,
          non_stream_s: 0,
        },
        threshold_enabled: {
          stream_response_header: false,
          streaming: false,
          non_stream: false,
        },
        scope: 'local_instance',
        tracking_degraded: true,
      }),
      {}
    )
  })
})
