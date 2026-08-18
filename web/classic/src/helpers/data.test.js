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

import { refreshStatusAfterOptionUpdate } from './data.js';

const statusRelatedKeys = [
  'RateLimitCapacityCardEnabled',
  'ModelNameRPMRateLimit',
  'console_setting.api_info_enabled',
  'console_setting.uptime_kuma_enabled',
  'console_setting.announcements_enabled',
  'console_setting.faq_enabled',
];

describe('Classic status refresh after option updates', () => {
  for (const key of statusRelatedKeys) {
    test(`refreshes and publishes status once after saving ${key}`, async () => {
      const nextStatus = { marker: key };
      const requestedUrls = [];
      const actions = [];
      const persisted = [];
      const api = {
        get: async (url) => {
          requestedUrls.push(url);
          return { data: { success: true, data: nextStatus } };
        },
      };

      await refreshStatusAfterOptionUpdate(
        key,
        (action) => actions.push(action),
        api,
        (status) => persisted.push(status),
      );

      assert.deepEqual(requestedUrls, ['/api/status']);
      assert.deepEqual(actions, [{ type: 'set', payload: nextStatus }]);
      assert.deepEqual(persisted, [nextStatus]);
    });
  }

  test('does not overwrite old status when the GET fails', async () => {
    const actions = [];
    const persisted = [];
    const api = {
      get: async () => {
        throw new Error('offline');
      },
    };

    await refreshStatusAfterOptionUpdate(
      'RateLimitCapacityCardEnabled',
      (action) => actions.push(action),
      api,
      (status) => persisted.push(status),
    );

    assert.deepEqual(actions, []);
    assert.deepEqual(persisted, []);
  });

  test('does not request status for an unrelated option', async () => {
    let requestCount = 0;

    await refreshStatusAfterOptionUpdate(
      'UnrelatedOption',
      () => {},
      { get: async () => requestCount++ },
      () => {},
    );

    assert.equal(requestCount, 0);
  });
});
