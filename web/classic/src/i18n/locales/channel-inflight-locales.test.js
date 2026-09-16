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
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const locales = ['en', 'fr', 'ja', 'ru', 'vi', 'zh', 'zh-CN', 'zh-TW'];
const requiredKeys = [
  '清理卡住的连接',
  '确认清理',
  '获取连接状态失败',
  '仅关闭已超过网关超时时限的连接；仍在超时时限内的连接不会被触碰。',
  '在途',
  '未收首字',
  '最老',
  '可清理',
  '超时时限',
  '没有可清理的连接。可清理为 0 是正常状态。',
  '未配置超时时限的类别没有越界阈值，因此该类别的连接永远不可清理。',
  '本实例的连接跟踪已降级，计数可能不完整。',
  '计数仅覆盖响应本次请求的实例。',
  '已清理 {{total}} 条卡住的连接',
  // Reused by the confirmation dialog; a locale that lost it would render the
  // unconfigured timeout classes as blanks.
  '未配置',
];

const readTranslation = (locale) =>
  JSON.parse(readFileSync(new URL(`./${locale}.json`, import.meta.url), 'utf8'))
    .translation ?? {};

for (const locale of locales) {
  test(`${locale} has complete stalled connection cleanup copy`, () => {
    const translation = readTranslation(locale);

    for (const key of requiredKeys) {
      const value = translation[key];
      assert.equal(typeof value, 'string', `${locale} is missing ${key}`);
      assert.notEqual(value.trim(), '');
    }
  });
}

test('the cleared-connections toast avoids the i18next plural trigger', () => {
  // An interpolation named `count` makes i18next resolve `key_one`/`key_other`
  // first, which these flat locale files do not define.
  const key = '已清理 {{total}} 条卡住的连接';
  assert.equal(key.includes('{{count}}'), false);

  for (const locale of locales) {
    const value = readTranslation(locale)[key];
    assert.equal(
      value.includes('{{total}}'),
      true,
      `${locale} dropped the {{total}} interpolation`,
    );
    assert.equal(
      value.includes('{{count}}'),
      false,
      `${locale} reintroduced the i18next plural trigger`,
    );
  }
});
