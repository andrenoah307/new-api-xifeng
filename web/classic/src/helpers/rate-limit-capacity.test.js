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
  isRateLimitCapacityEnabled,
  shouldRequestRateLimitCapacity,
} from './rate-limit-capacity.js';

describe('rate-limit capacity dashboard hard gate', () => {
  const cases = [
    ['explicit true', { rate_limit_capacity_enabled: true }, true],
    ['explicit false', { rate_limit_capacity_enabled: false }, false],
    ['undefined field', { rate_limit_capacity_enabled: undefined }, false],
    ['missing field', {}, false],
    ['null status', null, false],
  ];

  for (const [label, status, expected] of cases) {
    test(`is ${expected ? 'open' : 'closed'} for ${label}`, () => {
      assert.equal(isRateLimitCapacityEnabled(status), expected);
    });
  }
});

describe('rate-limit capacity request decision', () => {
  const base = {
    force: false,
    loaded: false,
    loadedAt: 0,
    now: 10_000,
    inFlight: false,
    retryAfterAt: 0,
    staleTime: 5_000,
  };
  const cases = [
    [
      'rejects an in-flight forced request',
      { ...base, force: true, inFlight: true },
      false,
    ],
    [
      'lets force bypass fresh data',
      { ...base, force: true, loaded: true, loadedAt: 9_000 },
      true,
    ],
    [
      'lets force bypass the error cooldown',
      { ...base, force: true, retryAfterAt: 11_000 },
      true,
    ],
    [
      'rejects a non-forced request with fresh data',
      { ...base, loaded: true, loadedAt: 9_000 },
      false,
    ],
    [
      'rejects a non-forced request during the error cooldown',
      { ...base, retryAfterAt: 11_000 },
      false,
    ],
    ['allows the initial request', base, true],
    [
      'allows a request when loaded data has expired',
      { ...base, loaded: true, loadedAt: 5_000 },
      true,
    ],
  ];

  for (const [label, input, expected] of cases) {
    test(label, () => {
      assert.equal(shouldRequestRateLimitCapacity(input), expected);
    });
  }
});
