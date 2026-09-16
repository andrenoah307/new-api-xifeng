/*
Copyright (C) 2025 QuantumNous

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

import assert from 'node:assert/strict';
import { describe, test } from 'node:test';

import {
  countAwaitingFirstByte,
  formatInflightAge,
  formatInflightThreshold,
  indexInflightByChannel,
} from './channel-inflight.js';

describe('formatInflightAge', () => {
  test('renders an age the way a table cell can hold it', () => {
    const cases = [
      [0, '0s'],
      [-1, '0s'],
      [Number.NaN, '0s'],
      [Number.POSITIVE_INFINITY, '0s'],
      [undefined, '0s'],
      [999, '0s'],
      [1000, '1s'],
      [59999, '59s'],
      [60000, '1m0s'],
      [305000, '5m5s'],
      [3599000, '59m59s'],
      [3600000, '1h0m'],
      [7530000, '2h5m'],
    ];
    for (const [input, expected] of cases) {
      assert.equal(formatInflightAge(input), expected, `age ${input}`);
    }
  });
});

describe('countAwaitingFirstByte', () => {
  test('sums both pre-first-byte classes', () => {
    assert.equal(
      countAwaitingFirstByte({
        awaiting_headers: 3,
        awaiting_first_chunk: 4,
      }),
      7,
    );
  });

  test('never lets a negative or missing counter subtract from the total', () => {
    assert.equal(
      countAwaitingFirstByte({ awaiting_headers: -5, awaiting_first_chunk: 2 }),
      2,
    );
    assert.equal(countAwaitingFirstByte({ awaiting_headers: 2 }), 2);
    assert.equal(countAwaitingFirstByte(null), 0);
    assert.equal(countAwaitingFirstByte(undefined), 0);
  });
});

describe('indexInflightByChannel', () => {
  test('keys the bulk runtime payload by channel id', () => {
    const indexed = indexInflightByChannel({
      channels: [
        { channel_id: 7, in_flight: 2 },
        { channel_id: 11, in_flight: 5 },
      ],
    });
    assert.deepEqual(Object.keys(indexed).sort(), ['11', '7']);
    assert.equal(indexed['7'].in_flight, 2);
    assert.equal(indexed['11'].in_flight, 5);
  });

  test('a malformed or absent payload indexes to nothing rather than throwing', () => {
    assert.deepEqual(indexInflightByChannel(undefined), {});
    assert.deepEqual(indexInflightByChannel({}), {});
    assert.deepEqual(indexInflightByChannel({ channels: 'nope' }), {});
    assert.deepEqual(
      indexInflightByChannel({
        channels: [null, { in_flight: 1 }, { channel_id: 'x' }],
      }),
      {},
    );
  });
});

describe('formatInflightThreshold', () => {
  test('an unconfigured class reads as unconfigured, never as 0s', () => {
    assert.equal(formatInflightThreshold(300, true, '未配置'), '300s');
    assert.equal(formatInflightThreshold(0, false, '未配置'), '未配置');
    assert.equal(formatInflightThreshold(300, false, '未配置'), '未配置');
    assert.equal(formatInflightThreshold(-1, true, '未配置'), '0s');
  });
});
