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

import { normalizeEnabledOptionValue } from './boolean.js';

describe('Classic enabled option parsing', () => {
  test('keeps the new card option disabled when its stored value is false', () => {
    assert.equal(
      normalizeEnabledOptionValue('RateLimitCapacityCardEnabled', 'false'),
      false,
    );
    assert.equal(
      normalizeEnabledOptionValue('RateLimitCapacityCardEnabled', undefined),
      false,
    );
  });

  test('converts enabled option strings while preserving unrelated values', () => {
    assert.equal(
      normalizeEnabledOptionValue('RateLimitCapacityCardEnabled', 'true'),
      true,
    );
    assert.equal(
      normalizeEnabledOptionValue('ModelNameRPMRateLimit', 'false'),
      'false',
    );
  });
});
