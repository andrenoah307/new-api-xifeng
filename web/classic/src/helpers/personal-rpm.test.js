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
  normalizePersonalRPMItems,
  personalRPMDisplayState,
} from './personal-rpm.js';

describe('personal RPM presentation contract', () => {
  test('sorts by current usage, breaks ties by model, and preserves zero usage', () => {
    const zero = {
      model: 'zero',
      current: 0,
      limit: 20,
      utilization: 0,
      available: true,
      unlimited: false,
      over_limit: false,
    };
    const z = {
      model: 'z',
      current: 2,
      limit: 20,
      utilization: 0.1,
      available: true,
      unlimited: false,
      over_limit: false,
    };
    const a = { ...z, model: 'a' };
    const one = { ...z, model: 'one', current: 1, utilization: 0.05 };

    assert.deepEqual(normalizePersonalRPMItems([z, zero, one, a]), [
      a,
      z,
      one,
      zero,
    ]);
  });

  test('keeps unavailable metrics without letting their counters affect sorting', () => {
    const available = {
      model: 'available',
      current: 1,
      limit: 20,
      utilization: 0.05,
      available: true,
      unlimited: false,
      over_limit: false,
    };
    const unavailableZ = {
      ...available,
      model: 'z-unavailable',
      current: 999,
      utilization: null,
      available: false,
    };
    const unavailableA = {
      ...available,
      model: 'a-unavailable',
      current: null,
      utilization: null,
      available: false,
    };

    assert.deepEqual(
      normalizePersonalRPMItems([unavailableZ, available, unavailableA]),
      [available, unavailableA, unavailableZ],
    );
  });

  test('distinguishes empty from unavailable', () => {
    assert.equal(personalRPMDisplayState('empty', []), 'empty');
    assert.equal(personalRPMDisplayState('ok', []), 'empty');
    assert.equal(
      personalRPMDisplayState('unavailable', [
        {
          model: 'hidden',
          current: null,
          limit: 20,
          utilization: null,
          available: false,
          unlimited: false,
          over_limit: false,
        },
      ]),
      'unavailable',
    );
  });
});
