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
  GROUP_RATE_LIMIT_MAX_COUNT,
  GROUP_RATE_LIMIT_MIN_SUCCESS_COUNT,
  deleteGroupRateLimitRule,
  parseGroupRateLimitConfig,
  upsertGroupRateLimitRule,
  validateGroupRateLimitRule,
} from './group-rate-limit.js';

describe('Classic group-rate-limit helper', () => {
  test('uses the same user-facing validation codes as the Default helper', () => {
    assert.deepEqual(
      validateGroupRateLimitRule({
        groupName: '',
        totalCount: -1,
        successCount: 0,
      }),
      {
        ok: false,
        errors: [
          'group-name-required',
          'total-count-range',
          'success-count-range',
        ],
      },
    );
  });

  test('does not impose a frontend-only group-name length limit', () => {
    assert.deepEqual(
      validateGroupRateLimitRule({
        groupName: 'x'.repeat(4096),
        totalCount: GROUP_RATE_LIMIT_MAX_COUNT,
        successCount: GROUP_RATE_LIMIT_MIN_SUCCESS_COUNT,
      }),
      { ok: true, errors: [] },
    );
  });

  test('keeps the exact error message used to refuse unsafe mutations', () => {
    const raw = '{"broken":[1]}';
    assert.equal(
      upsertGroupRateLimitRule(
        raw,
        { groupName: 'vip', totalCount: 1, successCount: 1 },
        null,
      ).error,
      'Invalid group rate limit document',
    );
    assert.equal(
      deleteGroupRateLimitRule(raw, 'broken').error,
      'Invalid group rate limit document',
    );
  });

  test('retains the parsed doc for a valid decimal integer literal', () => {
    assert.deepEqual(parseGroupRateLimitConfig('{"vip":[1.0,2]}'), {
      ok: true,
      doc: { vip: [1, 2] },
      rules: [{ groupName: 'vip', totalCount: 1, successCount: 2 }],
    });
    assert.equal(
      upsertGroupRateLimitRule(
        '{"vip":[1.0,2.0]}',
        { groupName: 'vip', totalCount: 1, successCount: 2 },
        'vip',
      ).json,
      '{\n  "vip": [\n    1,\n    2\n  ]\n}',
    );
  });
});
