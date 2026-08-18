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

import { requestUptimeStatus } from './uptime.js';

describe('Classic Uptime request gate', () => {
  for (const [label, status] of [
    ['status is not ready', undefined],
    ['status is null', null],
    ['Uptime is explicitly disabled', { uptime_kuma_enabled: false }],
    ['the Uptime flag is missing', {}],
  ]) {
    test(`does not request when ${label}`, async () => {
      let requestCount = 0;

      const result = await requestUptimeStatus(status, async () => {
        requestCount += 1;
        return { data: { success: true } };
      });

      assert.equal(requestCount, 0);
      assert.equal(result, null);
    });
  }

  test('requests exactly once when Uptime is explicitly enabled', async () => {
    let requestCount = 0;
    const response = { data: { success: true, data: [] } };

    const result = await requestUptimeStatus(
      { uptime_kuma_enabled: true },
      async () => {
        requestCount += 1;
        return response;
      },
    );

    assert.equal(requestCount, 1);
    assert.equal(result, response);
  });
});
