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
  buildTokenFormValues,
  getTokenFormInitValues,
  isCurrentTokenLoadRequest,
} from './token-form-values';

const conversion = { displayRate: 7.3, quotaPerUnit: 500000, symbol: '¥' };

const enabledToken = {
  name: 'monthly-token',
  remain_quota: 123456,
  expired_time: -1,
  unlimited_quota: true,
  model_limits_enabled: true,
  model_limits: 'gpt-4o,claude-3-5-sonnet',
  allow_ips: '127.0.0.1',
  group: 'default',
  cross_group_retry: true,
  period_type: 'month',
  period_quota_limit: 684932,
  period_limit_unit: 'cny',
  period_reset_at: 1800000000,
  period_anchor_at: 1799000000,
};

describe('edit token form hydration', () => {
  // The modal hydrates with { isOverride: true }, which replaces the whole
  // value store: a key missing here would wipe a registered field, and an extra
  // key would pollute the form contract.
  test('produces exactly the init value key set', () => {
    const values = buildTokenFormValues(enabledToken, conversion, {
      expiredTime: -1,
      remainAmount: 0.25,
    });

    assert.deepEqual(
      Object.keys(values).sort(),
      Object.keys(getTokenFormInitValues()).sort(),
    );
    assert.equal('canonicalQuota' in values, false);
    assert.equal('period_anchor_at' in values, false);
  });

  test('hydrates an enabled amount limit instead of falling back to the seed', () => {
    const values = buildTokenFormValues(enabledToken, conversion, {
      expiredTime: '2026-08-10 00:00:00',
      remainAmount: 0.25,
    });

    assert.equal(values.period_enabled, true);
    assert.equal(values.period_type, 'month');
    assert.equal(values.period_days, 0);
    assert.equal(values.period_limit_unit, 'cny');
    assert.equal(values.period_limit_value, '10');
    assert.equal(values.period_reset_at, 1800000000);
    assert.equal(values.expired_time, '2026-08-10 00:00:00');
    assert.equal(values.remain_amount, 0.25);
    assert.deepEqual(values.model_limits, ['gpt-4o', 'claude-3-5-sonnet']);
    assert.equal(values.tokenCount, 1);
  });

  test('hydrates a disabled token into the closed period shape', () => {
    const values = buildTokenFormValues(
      {
        ...enabledToken,
        period_type: '',
        period_quota_limit: 0,
        period_limit_unit: 'quota',
      },
      conversion,
      { expiredTime: -1, remainAmount: 0 },
    );

    assert.equal(values.period_enabled, false);
    assert.equal(values.period_type, '');
    assert.equal(values.period_days, 0);
    assert.equal(values.period_limit_unit, 'cny');
    assert.equal(values.period_limit_value, '0');
    assert.equal(values.period_reset_at, 0);
  });

  test('quota unit tokens keep the native limit string', () => {
    const values = buildTokenFormValues(
      { ...enabledToken, period_limit_unit: 'quota' },
      conversion,
      { expiredTime: -1, remainAmount: 0 },
    );

    assert.equal(values.period_limit_unit, 'quota');
    assert.equal(values.period_limit_value, '684932');
  });
});

describe('token load request sequencing', () => {
  test('accepts the latest response and discards superseded ones', () => {
    assert.equal(isCurrentTokenLoadRequest(2, 2), true);
    assert.equal(isCurrentTokenLoadRequest(1, 2), false);
  });
});
