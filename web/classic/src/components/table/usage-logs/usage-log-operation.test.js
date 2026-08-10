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
  getUsageLogOperationText,
  isUsageLogOperationAction,
} from './usage-log-operation';

const translate = (template, params = {}) =>
  template.replace(/{{(\w+)}}/g, (placeholder, name) =>
    Object.prototype.hasOwnProperty.call(params, name)
      ? String(params[name])
      : placeholder,
  );

function quotaFormatter(calls) {
  return (value, digits) => {
    calls.push([value, digits]);
    return `formatted:${value}`;
  };
}

describe('usage log quota operation details', () => {
  test('renders numeric add and subtract quotas with six digits', () => {
    const calls = [];
    const formatQuota = quotaFormatter(calls);

    assert.equal(
      getUsageLogOperationText(
        { op: { action: 'user.quota_add', params: { quota: 5000000 } } },
        'raw content',
        translate,
        formatQuota,
      ),
      '管理员增加账户额度 formatted:5000000',
    );
    assert.equal(
      getUsageLogOperationText(
        { op: { action: 'user.quota_subtract', params: { quota: 125000 } } },
        'raw content',
        translate,
        formatQuota,
      ),
      '管理员减少账户额度 formatted:125000',
    );
    assert.deepEqual(calls, [
      [5000000, 6],
      [125000, 6],
    ]);
  });

  test('renders numeric override endpoints with six digits', () => {
    const calls = [];

    assert.equal(
      getUsageLogOperationText(
        {
          op: {
            action: 'user.quota_override',
            params: { from: 500000, to: 5000000 },
          },
        },
        'raw content',
        translate,
        quotaFormatter(calls),
      ),
      '管理员将账户额度从 formatted:500000 调整为 formatted:5000000',
    );
    assert.deepEqual(calls, [
      [500000, 6],
      [5000000, 6],
    ]);
  });

  test('preserves historical string quota parameters verbatim', () => {
    const calls = [];
    const quota = '¥10.000000 额度';
    const from = '$1.250000 quota';
    const to = '10 credits';

    assert.equal(
      getUsageLogOperationText(
        { op: { action: 'user.quota_add', params: { quota } } },
        'raw content',
        translate,
        quotaFormatter(calls),
      ),
      `管理员增加账户额度 ${quota}`,
    );
    assert.equal(
      getUsageLogOperationText(
        { op: { action: 'user.quota_override', params: { from, to } } },
        'raw content',
        translate,
        quotaFormatter(calls),
      ),
      `管理员将账户额度从 ${from} 调整为 ${to}`,
    );
    assert.deepEqual(calls, []);
  });

  test('falls back to the original content for unknown actions and missing operations', () => {
    const content = '原始英文 content';

    assert.equal(
      getUsageLogOperationText(
        { op: { action: 'user.quota_unknown', params: { quota: 5000000 } } },
        content,
        translate,
        quotaFormatter([]),
      ),
      content,
    );
    assert.equal(
      getUsageLogOperationText({}, content, translate, quotaFormatter([])),
      content,
    );
    assert.equal(isUsageLogOperationAction('user.quota_unknown'), false);
    assert.equal(isUsageLogOperationAction('user.quota_add'), true);
  });
});
